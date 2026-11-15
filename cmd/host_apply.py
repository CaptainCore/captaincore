# Runs on a CaptainCore node as root (sudo python3 -c ...), reading the
# Manager's host-config reply on stdin. Prints one JSON line as the result.
# Embedded into the CLI by cmd/host.go; keep it Python 3 standard library only.
import json
import os
import re
import subprocess
import sys
import time

DRY = "--dry-run" in sys.argv
res = {"changes": [], "health": {}, "certs": {}}


def done(status, error=None):
    res["status"] = status
    if error:
        res["error"] = error
    print(json.dumps(res))
    sys.exit(0)


def run(*args, inp=None, check=True):
    p = subprocess.run(list(args), input=inp, capture_output=True, text=True)
    if check and p.returncode != 0:
        raise RuntimeError("%s: %s" % (" ".join(args[:4]), (p.stderr or p.stdout).strip()[:400]))
    return p


try:
    hc = json.load(sys.stdin)
except Exception as e:
    done("failed", "bad input: %s" % e)

cfg = hc.get("config") or {}
ct = hc.get("container", "")
ip = hc.get("ip", "")
rev = hc.get("revision", 0)
hosts = hc.get("hostnames") or []
primary = hc.get("primary", "")
# A plain site (HTML files, no WordPress or database): no page cache, no
# WordPress debug constants, no blog_public to keep at 0.
plain = hc.get("stack") == "plain"
APP_NAME_RE = re.compile(r"^[a-z][a-z0-9-]{0,19}$")
apps = [a for a in (cfg.get("apps") or []) if APP_NAME_RE.match(str(a.get("name", "")))
        and 3000 <= int(a.get("port", 0)) <= 3999 and a.get("enabled", True)]
app_hosts = [a["hostname"] for a in apps if a.get("route") == "hostname" and a.get("hostname")]
site_hosts = []
for d in (cfg.get("domains") or []):
    site_hosts.append(d.get("name", ""))
    if d.get("www"):
        site_hosts.append("www." + d.get("name", ""))
res["revision"] = rev

# Everything below lands in config files or argv, so check it all again here.
HOST_RE = re.compile(r"^(?=.{1,253}$)([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$")
if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct):
    done("failed", "invalid container name")
if not re.match(r"^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip):
    done("failed", "invalid container ip")
if not site_hosts or not all(HOST_RE.match(h) for h in site_hosts + app_hosts) or primary not in site_hosts:
    done("failed", "invalid hostnames")
PATH_RE = re.compile(r"^(/[A-Za-z0-9._~-]+)+$")
apps = [a for a in apps if (a.get("route") == "hostname" and HOST_RE.match(a.get("hostname", "")))
        or (a.get("route") == "path" and PATH_RE.match(a.get("path", "")))]
php = cfg.get("php") or {}
try:
    workers = int(php["workers"]); max_workers = int(php["max_workers"])
    mem_limit = int(php["memory_limit"]); upload = int(php["upload_max_filesize"])
    max_exec = int(php["max_execution_time"]); memory_mb = int(cfg["memory_mb"])
except Exception:
    done("failed", "invalid php settings")
if not (1 <= workers <= max_workers <= 16 and 64 <= mem_limit <= 1024 and 2 <= upload <= 512
        and 30 <= max_exec <= 300 and 512 <= memory_mb <= 4096):
    done("failed", "php settings out of range")

# ── 1. Edge: /etc/caddy/sites/<container>.caddyfile ────────────────────────
def addrs(names):
    out = []
    for n in names:
        out.append(n)
        if not cfg.get("force_https", True):
            out.append("http://" + n)
    return ", ".join(out)

REDIR_FROM = re.compile(r"^/[A-Za-z0-9._~%/+-]*\*?$")
REDIR_TO = re.compile(r"^(/[A-Za-z0-9._~%/+?=&#-]*|https?://[A-Za-z0-9.-]+(:\d+)?(/[A-Za-z0-9._~%/+?=&#-]*)?)$")
IP_RE = re.compile(r"^[0-9a-fA-F:.]+(/\d{1,3})?$")
redirects = [r for r in (cfg.get("redirects") or [])
             if REDIR_FROM.match(r.get("from", "")) and REDIR_TO.match(r.get("to", "")) and int(r.get("code", 301)) in (301, 302, 307, 308)]
deny = [d for d in (cfg.get("ip_deny") or []) if IP_RE.match(d)]
pp = cfg.get("password_protect") or {}
pp_on = bool(pp.get("enabled")) and re.match(r"^[A-Za-z0-9._-]{1,64}$", pp.get("username", "")) and \
    re.match(r"^\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}$", pp.get("password_hash", ""))


# ── What visitors see when something is wrong ───────────────────────────────
# The edge answers its own failures (the site's container is down or slow, an
# address is denied, nothing is at an address) and a suspended site with these
# pages. They say what a visitor can do and nothing about the server: no
# hostnames, addresses, software names or versions. /etc/caddy/errors/ is
# shared by every site on the server and written again when a page changes.
ERR_DIR = "/etc/caddy/errors"
# Sign-in protection (the block list every site imports, and the sign-in log
# fail2ban reads) needs Caddy 2.8 or newer at the edge; the node tool's
# `guard-setup` owns the fail2ban side.
_cv = re.search(r"v(\d+)\.(\d+)", run("caddy", "version", check=False).stdout or "")
CADDY_NEW = bool(_cv) and (int(_cv.group(1)), int(_cv.group(2))) >= (2, 8)
GUARD_FILE = "/etc/caddy/guard.caddyfile"
MAIN_CADDYFILE = """# Node edge. Managed by CaptainCore (`captaincore host apply`): changes here
# are overwritten. Each site's block is /etc/caddy/sites/<container>.caddyfile,
# written from the site's hosting configuration.
import guard.caddyfile
import sites/*.caddyfile
"""
GUARD_LINES = ["\t\timport cc_guard"] if CADDY_NEW else []
# Only sign-in attempts are logged at the edge (the site's own access log is
# in its container): what fail2ban needs to spot password guessing.
AUTH_LOG = [
    "\tlog {", "\t\toutput file /var/log/caddy/auth.log {", "\t\t\troll_size 20MiB", "\t\t\troll_keep 3", "\t\t}",
    "\t\tformat filter {", "\t\t\twrap json", "\t\t\tfields {", "\t\t\t\trequest>headers delete", "\t\t\t\trequest>tls delete",
    "\t\t\t\tresp_headers delete", "\t\t\t}", "\t\t}", "\t}",
    "\t@cc_not_signin {", "\t\tnot {", "\t\t\tmethod POST", "\t\t\tpath */wp-login.php */xmlrpc.php", "\t\t}", "\t}",
    "\tlog_skip @cc_not_signin"] if CADDY_NEW else []
ERR_ICONS = {
    "pulse": "<path d='M3 12h4l2-5 4 10 2-5h6'/>",
    "clock": "<circle cx='12' cy='12' r='8.5'/><path d='M12 7.5V12l3 2'/>",
    "lock": "<rect x='5.5' y='10.5' width='13' height='9' rx='2'/><path d='M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5'/>",
    "search": "<circle cx='11' cy='11' r='6'/><path d='M15.5 15.5 20 20'/>",
    "pause": "<circle cx='12' cy='12' r='8.5'/><path d='M10 9v6M14 9v6'/>",
}


def error_page(title, message, icon, code="", button=None):
    btn = "<a class='btn' href='%s'>%s</a>" % button if button else ""
    foot = "<small>%s</small>" % code if code else ""
    return ("<!doctype html><html lang='en'><head><meta charset='utf-8'><meta name='viewport' content='width=device-width,initial-scale=1'>"
            "<meta name='robots' content='noindex'><title>%s</title><style>"
            ":root{color-scheme:light dark;--bg:#f5f5f7;--card:#fff;--ink:#16161a;--dim:#62626e;--rule:#e4e4ea;--accent:#2f6fde}"
            "@media(prefers-color-scheme:dark){:root{--bg:#0c0c0f;--card:#17171c;--ink:#ededf0;--dim:#9a9aa6;--rule:#26262e;--accent:#7ba8ff}}"
            "*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px;background:var(--bg);color:var(--ink);"
            "font:16px/1.6 system-ui,-apple-system,'Segoe UI',Roboto,sans-serif}main{width:100%%;max-width:440px;text-align:center}"
            ".mark{width:72px;height:72px;margin:0 auto 22px;border-radius:20px;background:var(--card);border:1px solid var(--rule);display:grid;place-items:center;color:var(--accent)}"
            "h1{font-size:24px;line-height:1.25;margin:0 0 10px;letter-spacing:-.01em}p{margin:0;color:var(--dim)}"
            "a.btn{display:inline-block;margin-top:24px;padding:10px 20px;border-radius:10px;background:var(--ink);color:var(--bg);text-decoration:none;font-weight:600;font-size:14px}"
            "small{display:block;margin-top:28px;font-size:12px;color:var(--dim);letter-spacing:.04em}"
            "</style></head><body><main><div class='mark'><svg viewBox='0 0 24 24' width='34' height='34' fill='none' stroke='currentColor' stroke-width='1.6' "
            "stroke-linecap='round' stroke-linejoin='round' aria-hidden='true'>%s</svg></div><h1>%s</h1><p>%s</p>%s%s</main></body></html>\n"
            % (title, ERR_ICONS[icon], title, message, btn, foot))


AGAIN = ("", "Try again")   # an empty link reloads the same address
ERR_PAGES = {
    "403.html": error_page("You don't have access to this page", "The owner of this site limits who can see it.", "lock", "Error 403"),
    "404.html": error_page("Page not found", "Nothing is at this address. Check the link, or start from the home page.", "search", "Error 404", ("/", "Go to the home page")),
    "429.html": error_page("Too many sign-in attempts", "Signing in from your network is paused for a while after repeated failed attempts. The rest of the site still works. Please try again later.", "lock", "Error 429"),
    "502.html": error_page("This site isn't responding", "We couldn't reach the site just now. It is usually back within a minute.", "pulse", "Error 502", AGAIN),
    "503.html": error_page("This site is busy right now", "It has more visitors than it can answer at the moment. Please try again shortly.", "pulse", "Error 503", AGAIN),
    "504.html": error_page("This page took too long", "The site didn't answer in time. Trying again often works.", "clock", "Error 504", AGAIN),
    "error.html": error_page("Something went wrong", "Please try again in a moment.", "pulse", "", AGAIN),
    "suspended.html": error_page("This site is temporarily unavailable", "Please check back later.", "pause"),
}
ERR_KNOWN = " || ".join("{http.error.status_code} == %s" % c for c in ("403", "404", "429", "502", "503", "504"))
# Errors raised at the edge end here, whatever block they came from.
ERRORS_BLOCK = [
    "\thandle_errors {", "\t\theader -Server", "\t\theader Cache-Control no-store", "\t\troot * %s" % ERR_DIR,
    "\t\t@cc_known `%s`" % ERR_KNOWN,
    "\t\thandle @cc_known {", "\t\t\trewrite * /{http.error.status_code}.html", "\t\t\tfile_server", "\t\t}",
    "\t\thandle {", "\t\t\trewrite * /error.html", "\t\t\tfile_server", "\t\t}", "\t}"]


def proxy(indent, upstream, try_for, site=False):
    """reverse_proxy to a container. Nothing about the software behind the
    edge goes out, a container that stops answering is a 504, and for the
    site itself the bare answers the container's web server gives (a plain
    text 502/503/504 when PHP is saturated, an empty 404) become the pages
    above. Pages WordPress or an app made are passed through as they are."""
    t = "\t" * indent
    b = [t + "reverse_proxy %s {" % upstream, t + "\tlb_try_duration %s" % try_for, t + "\tlb_try_interval 250ms",
         t + "\theader_down -Server", t + "\theader_down -X-Powered-By",
         t + "\ttransport http {", t + "\t\tdial_timeout 3s", t + "\t\tresponse_header_timeout %ds" % (max_exec + 30), t + "\t}"]
    if site:
        for code in ("502", "503", "504"):
            b += [t + "\t@cc_plain%s {" % code, t + "\t\tstatus %s" % code, t + "\t\theader Content-Type text/plain*", t + "\t}",
                  t + "\thandle_response @cc_plain%s {" % code, t + "\t\terror %s" % code, t + "\t}"]
        # A plain site has no index.php to answer a missing .php path, so
        # PHP says "No input file specified."; every 404 there is the edge's.
        b += [t + "\t@cc_empty404 {", t + "\t\tstatus 404"] + ([] if plain else [t + "\t\theader Content-Length 0"]) + [t + "\t}",
              t + "\thandle_response @cc_empty404 {", t + "\t\terror 404", t + "\t}"]
    return b + [t + "}"]


def site_body():
    # A suspended site answers every request (apps too) with a holding page.
    if cfg.get("suspended"):
        return ["\theader -Server", "\troute {", "\t\theader Retry-After 3600", "\t\theader Cache-Control no-store",
                "\t\troot * %s" % ERR_DIR, "\t\trewrite * /suspended.html", "\t\tfile_server {", "\t\t\tstatus 503", "\t\t}", "\t}"] + ERRORS_BLOCK
    # route keeps the written order: deny, redirects, auth, then the proxy.
    b = AUTH_LOG + ["\theader -Server", "\troute {", "\t\tencode zstd gzip"] + GUARD_LINES
    if deny:
        b += ["\t\t@cc_deny remote_ip %s" % " ".join(deny), "\t\terror @cc_deny 403"]
    # Apps at a path of the site: public (before redirects and the password).
    for a in apps:
        if a.get("route") != "path":
            continue
        b += ["\t\t@cc_app_%s path %s %s/*" % (a["name"].replace("-", "_"), a["path"], a["path"]),
              "\t\troute @cc_app_%s {" % a["name"].replace("-", "_")]
        if a.get("strip_prefix"):
            b.append("\t\t\turi strip_prefix %s" % a["path"])
        b += proxy(3, "%s:%d" % (ip, int(a["port"])), "5s") + ["\t\t}"]
    for r in redirects:
        b.append("\t\tredir %s %s %d" % (r["from"], r["to"], int(r["code"])))
    if pp_on:
        import base64
        b += ["\t\t%s {" % ("basic_auth" if CADDY_NEW else "basicauth"), "\t\t\t%s %s" % (pp["username"], base64.b64encode(pp["password_hash"].encode()).decode()), "\t\t}"]
    b += proxy(2, "%s:80" % ip, "15s", site=True) + ["\t}"]
    return b + ERRORS_BLOCK


# Search engines: noindex (the default; staging always) puts X-Robots-Tag on
# every response of every block, whatever answers it (WordPress, static
# files, apps, redirects), and keeps WordPress's blog_public at 0.
noindex = cfg.get("noindex", True) is not False
NOINDEX_LINE = ['\theader X-Robots-Tag "noindex, nofollow"'] if noindex else []

edge = ["# Managed by CaptainCore: %s, revision %s. Changes here are overwritten." % (ct, rev)]
others = [h for h in hosts if h != primary]
others = [h for h in site_hosts if h != primary]
if cfg.get("redirect_to_primary") and others:
    edge += ["%s {" % addrs([primary])] + NOINDEX_LINE + site_body() + ["}", "",
             "%s {" % addrs(others)] + NOINDEX_LINE + ["\tredir https://%s{uri} permanent" % primary, "}"]
else:
    edge += ["%s {" % addrs(site_hosts)] + NOINDEX_LINE + site_body() + ["}"]
for a in apps:
    if a.get("route") == "hostname" and cfg.get("suspended"):
        edge += ["", "%s {" % addrs([a["hostname"]])] + NOINDEX_LINE + site_body() + ["}"]
    elif a.get("route") == "hostname":
        edge += ["", "%s {" % addrs([a["hostname"]])] + NOINDEX_LINE + ["\theader -Server", "\troute {", "\t\tencode zstd gzip"]
        if deny:
            edge += ["\t\t@cc_deny remote_ip %s" % " ".join(deny), "\t\terror @cc_deny 403"]
        edge += proxy(2, "%s:%d" % (ip, int(a["port"])), "5s") + ["\t}"] + ERRORS_BLOCK + ["}"]
edge_new = "\n".join(edge) + "\n"
edge_path = "/etc/caddy/sites/%s.caddyfile" % ct
edge_prev_path = "/etc/caddy/sites/.%s.prev" % ct
edge_old = open(edge_path).read() if os.path.exists(edge_path) else None


def caddy_reload():
    run("caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
    run("systemctl", "reload", "caddy")


# ── Page cache and static files ────────────────────────────────────────────
# Static files that exist on disk go out with a one-year browser cache.
# Pages: PHP saves what anonymous visitors request as static files (the
# prepend file below, no plugin in WordPress) and the container's web server
# answers later requests from those files without running PHP. The cookie
# and query rules here are the same ones the prepend file applies.
pc = cfg.get("page_cache") if isinstance(cfg.get("page_cache"), dict) else {}
pc_php = hc.get("page_cache_php") or ""
pc_on = bool(pc.get("enabled")) and pc_php.startswith("<?php") and not plain
try:
    pc_ttl = max(5, min(int(pc.get("ttl") or 60), 10080))
except (TypeError, ValueError):
    pc_ttl = 60
pc_exclude = [p for p in (pc.get("exclude") or []) if isinstance(p, str) and PATH_RE.match(p)][:50]
PC_ROOT = "/var/cache/captaincore/page"
PC_PHP = "/etc/captaincore/page-cache.php"
PC_JSON = "/etc/captaincore/page-cache.json"
PC_GC = "/etc/captaincore/page-cache-gc.sh"
PC_UNITS = ["/etc/systemd/system/cc-page-cache-gc.service", "/etc/systemd/system/cc-page-cache-gc.timer"]
PC_MAX_MB = 1024
PC_COOKIES = "wordpress_logged_in_|wp-postpass_|comment_author_|woocommerce_items_in_cart|woocommerce_cart_hash|wp_woocommerce_session_|edd_items_in_cart|wordpress_no_cache"
PC_QUERY = "utm_[a-z0-9_]+|fbclid|gclid|gclsrc|gad_source|gad_campaignid|gbraid|wbraid|dclid|msclkid|mc_cid|mc_eid|_ga|_gl|srsltid|ttclid|twclid|li_fat_id|igshid|yclid|_hsenc|_hsmi|mkt_tok"
STATIC_EXT = "css js mjs woff woff2 ttf otf eot svg png jpg jpeg gif webp avif ico bmp mp4 webm ogg mp3 wav"

static_block = """	# Static files: a year in the browser (theme and plugin files carry a
	# version in their address, so an update is a new address).
	@cc_static {
		path %s
		file
	}
	header @cc_static Cache-Control "public, max-age=31536000"
""" % " ".join("*." + e for e in STATIC_EXT.split())


def page_block(name, slash, try_file):
    return """	@cc_page_%s {
		method GET HEAD
		header X-Forwarded-Proto https
		%spath */
		not header_regexp Cookie (%s)
		not header_regexp Authorization .+
		expression `{query} == "" || {query}.matches("^((%s)(=[^&]*)?(&|$))+$")`
		file {
			root %s
			try_files %s
		}
	}
	handle @cc_page_%s {
		root * %s
		rewrite * {file_match.relative}
		header X-Cache HIT
		file_server
	}
""" % (name, "" if slash else "not ", PC_COOKIES, PC_QUERY, PC_ROOT, try_file, name, PC_ROOT)


page_blocks = ("""	# Page cache: a saved copy answers anonymous page views; anything with a
	# login, cart or comment cookie, a form post or a real query string goes
	# to PHP. /about/ and /about are saved apart, as WordPress treats them.
""" + page_block("slash", True, "/{host}{path}index.html") + page_block("bare", False, "/{host}{path}/index-noslash.html")) if pc_on else ""

pc_json = json.dumps({"enabled": pc_on, "hosts": sorted(set(site_hosts)), "exclude": pc_exclude}, sort_keys=True) + "\n"
pc_gc = """#!/bin/sh
# Managed by CaptainCore (hosting configuration). Saved pages older than
# %d minutes go; so does everything if the cache passes %d MB.
R=%s
[ -d "$R" ] || exit 0
find "$R" -type f -name '*.html' -mmin +%d -delete
find "$R" -type f -name '.tmp-*' -mmin +10 -delete
find "$R" -mindepth 1 -maxdepth 1 -name '.trash-*' -mmin +10 -exec rm -rf {} +
if [ "$(du -sm "$R" | cut -f1)" -gt %d ]; then find "$R" -mindepth 1 -maxdepth 1 -exec rm -rf {} +; fi
find "$R" -mindepth 2 -type d -empty -delete
exit 0
""" % (pc_ttl, PC_MAX_MB, PC_ROOT, pc_ttl, PC_MAX_MB)
pc_unit_files = {
    PC_UNITS[0]: """[Unit]
Description=CaptainCore page cache: drop expired pages

[Service]
Type=oneshot
User=site
Group=site
ExecStart=/bin/sh %s
Nice=10
""" % PC_GC,
    PC_UNITS[1]: """[Unit]
Description=CaptainCore page cache: drop expired pages

[Timer]
OnCalendar=*:0/5
RandomizedDelaySec=45
AccuracySec=30

[Install]
WantedBy=timers.target
""",
}

# ── 2. PHP inside the container ────────────────────────────────────────────
ini_new = """; Managed by CaptainCore (hosting configuration). Changes here are overwritten.
memory_limit = %dM
upload_max_filesize = %dM
post_max_size = %dM
max_execution_time = %d
max_input_vars = 5000
log_errors = On
error_log = /srv/site/logs/php-error.log
display_errors = Off
display_startup_errors = Off
expose_php = Off
opcache.memory_consumption = 128
opcache.interned_strings_buffer = 16
opcache.max_accelerated_files = 20000
opcache.revalidate_freq = 2
""" % (mem_limit, upload, upload, max_exec)
# PHP loads one file before every request and command (auto_prepend_file):
# a small one written here that pulls in what the site needs, the page cache
# and, with outgoing mail off, the mail block. Nothing of it shows in WordPress.
mail_on = cfg.get("mail", "relay") != "off"
MAIL_OFF_PHP = "/etc/captaincore/mail-off.php"
PREPEND_PHP = "/etc/captaincore/prepend.php"
mail_block = not mail_on and not plain
# WordPress defines wp_mail only when nothing has yet, and so do the SMTP
# plugins that replace it (Gravity SMTP, FluentSMTP, WP Mail SMTP): defined
# here first, no mail leaves the site by any of them, whatever port or API
# they would use. The relay switch alone only stops sendmail.
MAIL_OFF_SRC = r"""<?php
// Managed by CaptainCore (hosting configuration): outgoing email is turned
// off for this site. Every attempt is noted in ~/logs/mail.log.
if ( ! function_exists( 'wp_mail' ) ) {
	function wp_mail( $to, $subject, $message, $headers = '', $attachments = array() ) {
		$to   = is_array( $to ) ? implode( ', ', $to ) : (string) $to;
		$line = gmdate( 'Y-m-d H:i:s' ) . ' not sent, outgoing email is off: to=' . substr( $to, 0, 200 )
			. ' subject=' . substr( preg_replace( '/\s+/', ' ', (string) $subject ), 0, 200 ) . "\n";
		@file_put_contents( '/srv/site/logs/mail.log', $line, FILE_APPEND );
		if ( function_exists( 'do_action' ) && class_exists( 'WP_Error' ) ) {
			do_action( 'wp_mail_failed', new WP_Error( 'wp_mail_failed', 'Outgoing email is turned off for this site.' ) );
		}
		return false;
	}
}
"""
prepend_new = "<?php\n// Managed by CaptainCore (hosting configuration): loaded before every request and command.\n" \
    + ("require '%s';\n" % MAIL_OFF_PHP if mail_block else "") + ("require '%s';\n" % PC_PHP if pc_on else "")
if pc_on or mail_block:
    ini_new += "auto_prepend_file = %s\n" % PREPEND_PHP

# PHP's shell commands are off for web requests unless the site turns them
# on: they are how an uploaded web shell runs anything, and WordPress itself
# never calls them. This is set on the web server only, so WP-CLI and
# WP-Cron (which run as commands, not requests) keep them.
SHELL_FUNCTIONS = "exec,passthru,shell_exec,system,proc_open,popen,pcntl_exec"
shell_line = "" if php.get("shell") else "\t\tphp_ini disable_functions %s\n" % SHELL_FUNCTIONS

fcaddy_new = """# CaptainCore site container. TLS ends at the node edge; this serves plain
# HTTP on :80 to the bridge only. Managed by CaptainCore (hosting configuration).
{
	frankenphp {
		num_threads %d
		max_threads %d
		max_wait_time 15s
%s	}
	auto_https off
	servers {
		trusted_proxies static 10.77.0.1/32
	}
}

:80 {
	root /srv/site/public
	# No Brotli here: its encoder holds several MB per response in flight, so
	# 50 visitors at once on cached pages cost 650 MB and got PHP killed in a
	# load test. zstd and gzip stay under 200 MB and answer three times faster.
	encode zstd gzip
	# Never for the public, wherever they sit: hidden files and folders (.env,
	# .git, .htpasswd), logs (wp-content/debug.log), database dumps and
	# editor or backup copies (wp-config.php.bak would be sent as plain text).
	# /.well-known/ stays open: certificates and app links live there.
	@cc_private {
		path */.* *.log *.sql *.sql.gz *.bak *.old *.orig *.save *.swp *.swo *~
		not path /.well-known/*
	}
	respond @cc_private 404
	# PHP never runs from the folders WordPress writes to at runtime. An
	# uploaded script is the usual first step of a break-in, and nothing a
	# visitor should reach lives there as PHP (plugins that keep PHP files in
	# uploads include them from their own code, which this does not touch).
	# Also caught: other PHP extensions, any letter case, and the path-info
	# form (/uploads/x.php/y.jpg) that would still run x.php.
	@cc_upload_php path_regexp (?i)/wp-content/(uploads|upgrade|blogs\\.dir)/.*\\.(php\\d?|phtml|phar|pht|phps)(/|$)
	respond @cc_upload_php 404
%s%s	php_server
	# ~/logs/access.log is what CaptainCore's Logs view lists. client_ip is
	# the visitor (trusted_proxies takes it from the edge's X-Forwarded-For).
	log {
		output file /srv/site/logs/access.log {
			roll_size 25MiB
			roll_keep 4
		}
		format filter {
			wrap json
			fields {
				request>headers delete
				request>tls delete
				resp_headers delete
			}
		}
	}
}
""" % (workers, max_workers, shell_line, static_block, page_blocks)

# The web server half of FrankenPHP (Go) sizes its garbage collector from the
# container's whole memory limit by default, as if PHP and the database were
# not in there with it. Give it a quarter, at least 128 MB.
GOMEM_PATH = "/etc/systemd/system/frankenphp.service.d/20-captaincore-memory.conf"
gomem_new = "# Managed by CaptainCore (hosting configuration).\n[Service]\nEnvironment=GOMEMLIMIT=%dMiB\n" % max(128, memory_mb // 4)

PHP_FILES = [("/etc/php-zts/conf.d/99-captaincore.ini", ini_new), ("/etc/frankenphp/Caddyfile", fcaddy_new), (GOMEM_PATH, gomem_new)]


def ct_read(path):
    p = run("incus", "exec", ct, "--", "cat", path, check=False)
    return p.stdout if p.returncode == 0 else None


def ct_write(path, content):
    # path is one of the constants in PHP_FILES, never input.
    run("incus", "exec", ct, "--", "sh", "-c", "mkdir -p \"$(dirname '%s')\" && cat > '%s.cc-new' && chmod 0644 '%s.cc-new' && mv '%s.cc-new' '%s'" % (path, path, path, path, path), inp=content)


# The logs directory the Logs view reads (~/logs), owned by the site user.
if not DRY:
    run("incus", "exec", ct, "--", "install", "-d", "-o", "site", "-g", "site", "-m", "0750", "/srv/site/logs", check=False)
    run("incus", "exec", ct, "--", "sh", "-c",
        "[ -d /etc/logrotate.d ] && printf '%s\\n' '/srv/site/logs/php-error.log /srv/site/logs/mail.log {' '  size 25M' '  rotate 4' '  compress' '  missingok' '  notifempty' '  copytruncate' '}' > /etc/logrotate.d/captaincore || true",
        check=False)

php_old = {path: ct_read(path) for path, _ in PHP_FILES}
php_changed = any(php_old[path] != content for path, content in PHP_FILES)

def wp(*args, check=True):
    cmd = "cd /srv/site/public && wp " + " ".join("'%s'" % a.replace("'", "") for a in args)
    return run("incus", "exec", ct, "--", "su", "-s", "/bin/bash", "site", "-c", cmd, check=check)


# ── Apps: one systemd service each, as the site user ──────────────────────
def app_mem(a):
    try:
        return max(32, min(int(a.get("memory_mb") or 256), 16384))
    except (TypeError, ValueError):
        return 256


def app_files(a):
    n = a["name"]
    env_lines = ["# Managed by CaptainCore (hosting configuration)."]
    for k, v in sorted((a.get("env") or {}).items()):
        if re.match(r"^[A-Z_][A-Z0-9_]{0,63}$", k) and "\n" not in str(v):
            env_lines.append('%s="%s"' % (k, str(v).replace("\\", "\\\\").replace('"', '\\"')))
    return {
        "/etc/captaincore/apps/%s.sh" % n: "#!/bin/sh\n# Managed by CaptainCore (hosting configuration).\ncd /srv/site/apps/%s || exit 1\nexec %s\n" % (n, a["command"]),
        "/etc/captaincore/apps/%s.env" % n: "\n".join(env_lines) + "\n",
        "/etc/systemd/system/cc-app-%s.service" % n: """[Unit]
Description=CaptainCore app %s
After=network-online.target mariadb.service

[Service]
User=site
Group=site
WorkingDirectory=/srv/site/apps/%s
Environment=PORT=%d
EnvironmentFile=-/etc/captaincore/apps/%s.env
ExecStart=/bin/sh /etc/captaincore/apps/%s.sh
Restart=always
RestartSec=3
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full
MemoryHigh=%dM
MemoryMax=%dM
TasksMax=256

[Install]
WantedBy=multi-user.target
""" % (n, n, int(a["port"]), n, n, app_mem(a) * 9 // 10, app_mem(a)),
    }


def ct_write_any(path, content):
    # App env files hold secrets: root only (systemd reads them as root).
    mode = "0600" if path.endswith(".env") else "0755" if path.endswith(".sh") else "0644"
    run("incus", "exec", ct, "--", "sh", "-c", "mkdir -p \"$(dirname '%s')\" && cat > '%s.cc-new' && chmod %s '%s.cc-new' && mv '%s.cc-new' '%s'" % (path, path, mode, path, path, path), inp=content)


existing_units = run("incus", "exec", ct, "--", "sh", "-c", "ls /etc/systemd/system/ | grep -E '^cc-app-[a-z0-9-]+\\.service$' || true", check=False).stdout.split()
want_units = ["cc-app-%s.service" % a["name"] for a in apps]
app_changes = {}
for a in apps:
    for path, content in app_files(a).items():
        if ct_read(path) != content:
            app_changes.setdefault(a["name"], {})[path] = content
removed_units = [u for u in existing_units if u not in want_units]

want_debug = bool(cfg.get("debug")) and not plain
cur_debug = not plain and wp("config", "get", "WP_DEBUG", check=False).stdout.strip().lower() in ("1", "true")
debug_changed = want_debug != cur_debug

cur_mem = run("incus", "config", "get", ct, "limits.memory").stdout.strip()
want_mem = "%dMiB" % memory_mb
mem_changed = cur_mem != want_mem

blog_public_fix = noindex and not plain and wp("option", "get", "blog_public", check=False).stdout.strip() != "0"

# Container hardening, kept by every apply: customers SSH in only as site,
# tunnel only to their own database (127.0.0.1:3306), no agent, X11 or tun
# forwarding; no LLMNR/mDNS listener; at most `cpus` cores (default 4, the
# PHP thread ceiling), so one busy site can't take the whole server.
HARDEN_FILES = [
    ("/etc/ssh/sshd_config.d/10-captaincore.conf", "\n".join([
        "# Managed by CaptainCore (hosting configuration).",
        "PermitRootLogin no", "AllowUsers site", "PasswordAuthentication no", "KbdInteractiveAuthentication no",
        "X11Forwarding no", "AllowAgentForwarding no", "PermitTunnel no", "GatewayPorts no",
        "AllowTcpForwarding local", "PermitOpen 127.0.0.1:3306 localhost:3306", "MaxAuthTries 3"]) + "\n"),
    ("/etc/systemd/resolved.conf.d/10-captaincore.conf", "# Managed by CaptainCore (hosting configuration).\n[Resolve]\nLLMNR=no\nMulticastDNS=no\n"),
]
harden_old = {path: ct_read(path) for path, _ in HARDEN_FILES}
harden_changed = [path for path, content in HARDEN_FILES if harden_old[path] != content]
try:
    want_cpus = str(max(1, min(int(cfg.get("cpus") or 4), os.cpu_count() or 4)))
except (TypeError, ValueError):
    want_cpus = "4"
cpu_changed = run("incus", "config", "get", ct, "limits.cpu").stdout.strip() != want_cpus

# Limits the site never sees:
#  - root disk: the container's system disk is capped (the site's files and
#    database have their own volumes; /tmp is memory). Without a cap a
#    runaway log could fill the server for every site on it.
#  - files and database volumes count live data only (refquota): our local
#    backups (ZFS snapshots) never use up a site's space.
#  - network: each container's bandwidth capped both ways (bandwidth_mbit).
ROOT_SIZE = "5GiB"
try:
    want_bw = "%dMbit" % max(10, min(int(cfg.get("bandwidth_mbit") or 250), 1000))
except (TypeError, ValueError):
    want_bw = "250Mbit"
_inst = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout)
_local = _inst.get("devices") or {}
root_changed = (_local.get("root") or {}).get("size") != ROOT_SIZE
_nic = _local.get("eth0") or {}
bw_changed = _nic.get("limits.egress") != want_bw or _nic.get("limits.ingress") != want_bw
_vols = [d["source"] for d in _local.values() if d.get("type") == "disk" and d.get("source") and d.get("pool") == "default"]
refq_changed = [v for v in _vols if run("incus", "storage", "volume", "get", "default", v, "zfs.use_refquota", check=False).stdout.strip() != "true"]

# Programs that run with root's rights for whoever starts them; a site needs
# none of them, and each is a step towards root inside the container. Their
# setuid bit goes through dpkg-statoverride, so package updates keep it off.
SETUID_OFF = ["/usr/bin/pkexec", "/usr/lib/polkit-1/polkit-agent-helper-1", "/usr/lib/dbus-1.0/dbus-daemon-launch-helper",
              "/usr/bin/mount", "/usr/bin/umount", "/usr/bin/chfn", "/usr/bin/chsh", "/usr/bin/gpasswd", "/usr/bin/newgrp",
              "/usr/bin/su", "/usr/bin/passwd", "/usr/lib/openssh/ssh-keysign", "/usr/bin/chage", "/usr/bin/expiry"]
suid_on = run("incus", "exec", ct, "--", "sh", "-c",
              "for f in %s; do [ -u \"$f\" ] && echo \"$f\"; done; true" % " ".join(SETUID_OFF), check=False).stdout.split()

# Database over an SSH tunnel: sshd lets the site user forward only to
# 127.0.0.1:3306, and MariaDB (skip_name_resolve) sees those connections as
# 127.0.0.1, which never matches a user@localhost. Keep a 127.0.0.1 twin of
# each site database user, same password hash and grants, in step with it
# (a deploy between environments changes the password).
def db_sql(sql):
    return run("incus", "exec", ct, "--", "mariadb", "-N", "-B", "-e", sql, check=False)

db_twin_sql = []
if not plain:
    _users = db_sql("SELECT User FROM mysql.global_priv WHERE Host = 'localhost' AND User NOT IN ('root', 'mysql', 'mariadb.sys', '')").stdout.split()
    for _u in _users:
        if not re.match(r"^[A-Za-z0-9_]{1,32}$", _u):
            continue
        _want = [l.replace("`@`localhost`", "`@`127.0.0.1`") for l in db_sql("SHOW GRANTS FOR '%s'@'localhost'" % _u).stdout.splitlines() if l.strip()]
        _have = [l for l in db_sql("SHOW GRANTS FOR '%s'@'127.0.0.1'" % _u).stdout.splitlines() if l.strip()]
        if _want and sorted(_want) != sorted(_have):
            db_twin_sql += ["DROP USER IF EXISTS '%s'@'127.0.0.1'" % _u, "CREATE USER '%s'@'127.0.0.1'" % _u] + _want
db_twin_changed = bool(db_twin_sql)

# Outgoing mail: the container's sendmail (msmtp) hands mail to the server's
# relay on 10.77.0.1:2525, which sends it through Mailgun with credentials the
# container never sees. The relay knows each container by its address: a tag
# for Mailgun's logs, or a refusal when mail is turned off for the site.
MSMTP_PATH = "/etc/msmtprc"
msmtp_new = "\n".join([
    "# Managed by CaptainCore (hosting configuration).",
    "# Mail goes to this server's relay, which sends it through Mailgun.",
    "defaults", "auth off", "tls off", "syslog off", "logfile /srv/site/logs/mail.log",
    "account relay", "host 10.77.0.1", "port 2525", "from wordpress@%s" % primary,
    "allow_from_override on", "set_from_header auto", "account default : relay"]) + "\n"
has_msmtp = run("incus", "exec", ct, "--", "test", "-x", "/usr/bin/msmtp", check=False).returncode == 0
msmtp_changed = has_msmtp and ct_read(MSMTP_PATH) != msmtp_new
CC_CLIENTS = "/etc/postfix/cc_clients"
relay_line = ("%s PREPEND X-Mailgun-Tag: %s" % (ip, ct)) if mail_on else ("%s REJECT Outgoing mail is turned off for this site" % ip)
relay_old = open(CC_CLIENTS).read() if os.path.exists(CC_CLIENTS) else None
relay_new = None
if relay_old is not None and ip:
    keep = [l for l in relay_old.splitlines() if l.strip() and not l.startswith("#") and l.split()[0] != ip]
    relay_new = "# Managed by CaptainCore: one line per site container (host apply).\n" + "\n".join(sorted(keep + [relay_line])) + "\n"
relay_changed = relay_new is not None and relay_new != relay_old
# With mail off, the container cannot reach other mail servers either (SMTP
# submission ports), so a script on the site cannot send around the relay.
# Port 25 is closed for every container already.
MAIL_PORTS = "465,587,2525"
mail_ports_blocked = bool(ip) and re.search(r"^%s/tcp\s+DENY FWD\s+%s(\s|$)" % (re.escape(MAIL_PORTS), re.escape(ip)),
                                            run("ufw", "status", check=False).stdout, re.M) is not None
mail_ports_changed = bool(ip) and mail_on == mail_ports_blocked
prepend_want = {PREPEND_PHP: prepend_new} if (pc_on or mail_block) else {}
if mail_block:
    prepend_want[MAIL_OFF_PHP] = MAIL_OFF_SRC
prepend_diff = [path for path, content in prepend_want.items() if ct_read(path) != content]

# WordPress cron: the container's wp-cron.timer runs due events every minute.
# Off for a copy that must not act on its own (a restored backup can hold live
# payment keys and subscription renewals). Kept in the config so a rebuild,
# which starts from the image with the timer on, gets it turned off again.
cron_on = cfg.get("cron", True) is not False

# ── Scheduled tasks (hosting setting cron_jobs) ────────────────────────────
# One command each on a fixed schedule, run as the site user in ~/public by a
# systemd timer + oneshot service (an hour at most, memory capped). They run
# only while the site's cron switch is on, so a restored copy or a new
# environment never acts on its own. Checked again here: name, preset, time.
CRON_NAME_RE = re.compile(r"^[a-z][a-z0-9-]{0,19}$")
CRON_AT_RE = re.compile(r"^([01]\d|2[0-3]):[0-5]\d$")
def cron_calendar(j):
    at = j.get("at") if CRON_AT_RE.match(str(j.get("at", ""))) else "03:00"
    return {"every-5-minutes": "*:0/5", "every-15-minutes": "*:0/15", "hourly": "*:00",
            "daily": "*-*-* %s:00" % at, "weekly": "Mon *-*-* %s:00" % at}.get(j.get("schedule"))

cron_jobs = [j for j in (cfg.get("cron_jobs") or []) if CRON_NAME_RE.match(str(j.get("name", ""))) and cron_calendar(j)
             and 0 < len(str(j.get("command", ""))) <= 500 and not re.search(r"[\x00-\x1f\x7f]", str(j.get("command", "")))]
cron_mem = max(128, min(memory_mb - 512, 1024))

def cron_files(j):
    n = j["name"]
    return {
        "/etc/captaincore/cron/%s.sh" % n: "#!/bin/bash\n# Managed by CaptainCore (hosting configuration).\ncd /srv/site/public || exit 1\n%s\n" % j["command"],
        "/etc/systemd/system/cc-cron-%s.service" % n: """[Unit]
Description=CaptainCore scheduled task %s
After=network-online.target mariadb.service

[Service]
Type=oneshot
User=site
Group=site
WorkingDirectory=/srv/site/public
ExecStart=/bin/bash /etc/captaincore/cron/%s.sh
TimeoutStartSec=3600
Nice=10
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full
MemoryHigh=%dM
MemoryMax=%dM
TasksMax=128
""" % (n, n, cron_mem * 9 // 10, cron_mem),
        "/etc/systemd/system/cc-cron-%s.timer" % n: """[Unit]
Description=CaptainCore scheduled task %s

[Timer]
OnCalendar=%s
AccuracySec=10s
RandomizedDelaySec=20

[Install]
WantedBy=timers.target
""" % (n, cron_calendar(j)),
    }

cron_existing = run("incus", "exec", ct, "--", "sh", "-c", "ls /etc/systemd/system/ | grep -E '^cc-cron-[a-z0-9-]+\\.timer$' || true", check=False).stdout.split()
cron_want = ["cc-cron-%s.timer" % j["name"] for j in cron_jobs]
cron_file_changes = {}
for j in cron_jobs:
    for path, content in cron_files(j).items():
        if ct_read(path) != content:
            cron_file_changes.setdefault(j["name"], {})[path] = content
cron_removed = [u for u in cron_existing if u not in cron_want]
cron_active = set(run("incus", "exec", ct, "--", "sh", "-c", "systemctl list-unit-files 'cc-cron-*.timer' --state=enabled --no-legend 2>/dev/null | cut -d' ' -f1; true", check=False).stdout.split())
cron_should = set("cc-cron-%s.timer" % j["name"] for j in cron_jobs if cron_on and j.get("enabled", True) is not False)
cron_toggle = (cron_should ^ (cron_active & set(cron_want)))
cron_jobs_changed = bool(cron_file_changes or cron_removed or cron_toggle)
cron_state = "" if plain else run("incus", "exec", ct, "--", "systemctl", "is-enabled", "wp-cron.timer", check=False).stdout.strip()
cron_changed = cron_state in ("enabled", "disabled") and (cron_state == "enabled") != cron_on

# Page cache files in the container. The prepend file must exist before the
# PHP settings name it (a missing auto_prepend_file is a fatal error), so it
# is written first and never removed.
pc_want = {PC_PHP: pc_php, PC_JSON: pc_json, PC_GC: pc_gc}
pc_want.update(pc_unit_files)
pc_have = {path: ct_read(path) for path in pc_want}
if pc_on:
    pc_diff = [path for path, content in pc_want.items() if pc_have[path] != content]
else:
    # Off: the config says so (if the prepend file is there to read it) and the expiry timer goes.
    pc_diff = ([PC_JSON] if pc_have[PC_PHP] is not None and pc_have[PC_JSON] != pc_json else []) + [u for u in PC_UNITS if pc_have[u] is not None]
pc_changed = bool(pc_diff)

if DRY:
    res["would_change"] = [n for n, c in (("cache", pc_changed), ("edge", edge_new != edge_old), ("php", php_changed), ("memory", mem_changed), ("debug", debug_changed), ("blog_public", blog_public_fix), ("hardening", bool(harden_changed)), ("cpu", cpu_changed), ("mail", msmtp_changed or relay_changed or bool(prepend_diff) or mail_ports_changed), ("cron", cron_changed), ("root disk", root_changed), ("volume quotas", bool(refq_changed)),
        ("bandwidth", bw_changed), ("setuid", bool(suid_on)), ("database tunnel", db_twin_changed), ("scheduled tasks", cron_jobs_changed)) if c] \
        + ["app:" + n for n in app_changes] + ["remove:" + u for u in removed_units]
    res["edge"] = edge_new
    done("dry-run")


KERNEL_REBOOT_SH = """#!/bin/bash
# Managed by CaptainCore (host apply). Reboots this server into a newer
# installed kernel (cc-kernel-reboot.timer, 09:00 UTC); does nothing while the
# running kernel is the newest. kexec goes straight into the new kernel (seconds
# offline); when the last kexec into a kernel did not take, a normal reboot.
# --force reboots now even without a newer kernel (to test the window).
set -u
force="${1:-}"
running="$(uname -r)"
newest="$(ls /boot/vmlinuz-* 2>/dev/null | sed 's|/boot/vmlinuz-||' | sort -V | tail -1)"
state=/var/lib/captaincore/kernel-reboot
mkdir -p /var/lib/captaincore
if [ "$force" != "--force" ] && { [ -z "$newest" ] || [ "$newest" = "$running" ]; }; then
  echo "running $running, the newest installed kernel"; rm -f "$state"; exit 0
fi
target="${newest:-$running}"
if [ "$force" != "--force" ] && [ -f "$state" ] && [ "$(cat "$state")" = "$target" ]; then
  logger -t cc-kernel-reboot "kexec into $target did not take last time; normal reboot"
  rm -f "$state"; systemctl reboot; exit 0
fi
echo "$target" > "$state"
if command -v kexec >/dev/null && kexec -l "/boot/vmlinuz-$target" --initrd="/boot/initrd.img-$target" --reuse-cmdline; then
  logger -t cc-kernel-reboot "kexec into $target (running $running)"
  systemctl kexec
else
  logger -t cc-kernel-reboot "kexec could not load $target; normal reboot"
  systemctl reboot
fi
"""


def health():
    code_ct = run("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20",
                  "-H", "Host: " + primary, "-H", "X-Forwarded-Proto: https", "http://%s/" % ip, check=False).stdout
    code_edge = run("curl", "-sk", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20",
                    "--resolve", "%s:443:127.0.0.1" % primary, "https://%s/" % primary, check=False).stdout
    return code_ct, code_edge


def ok(code):
    return code.isdigit() and 200 <= int(code) < 400


try:
    os.makedirs(ERR_DIR, exist_ok=True)
    for name, page in ERR_PAGES.items():
        path = os.path.join(ERR_DIR, name)
        if not os.path.exists(path) or open(path).read() != page:
            with open(path + ".cc-new", "w") as f:
                f.write(page)
            os.chmod(path + ".cc-new", 0o644)
            os.replace(path + ".cc-new", path)
            if "error pages" not in res["changes"]:
                res["changes"].append("error pages")

    # The server's own updates. Security updates (and Caddy's) install daily
    # (Ubuntu's unattended-upgrades); needrestart then restarts whatever runs
    # an old library, so most updates need no reboot. Only a newer kernel
    # does: cc-kernel-reboot.timer checks at 09:00 UTC and reboots into it
    # with kexec (straight into the new kernel, skipping the firmware's
    # checks: seconds offline instead of minutes); if a kexec ever does not
    # take, the next run reboots normally. Containers start with the server.
    NODE_FILES = {
        "/etc/apt/apt.conf.d/52captaincore": "\n".join([
            "// Managed by CaptainCore (host apply).",
            'Unattended-Upgrade::Origins-Pattern { "origin=cloudsmith/caddy/stable"; };',
            "// Kernel reboots are cc-kernel-reboot's (09:00 UTC, kexec); other updates restart services (needrestart).",
            'Unattended-Upgrade::Automatic-Reboot "false";', ""]),
        "/etc/needrestart/conf.d/50-captaincore.conf": "# Managed by CaptainCore (host apply): restart services after updates, no prompts.\n$nrconf{restart} = 'a';\n$nrconf{kernelhints} = 0;\n",
        "/usr/local/sbin/cc-kernel-reboot": KERNEL_REBOOT_SH,
        "/etc/systemd/system/cc-kernel-reboot.service": "[Unit]\nDescription=CaptainCore: reboot into a newer kernel (kexec)\n\n[Service]\nType=oneshot\nExecStart=/usr/local/sbin/cc-kernel-reboot\n",
        "/etc/systemd/system/cc-kernel-reboot.timer": "[Unit]\nDescription=CaptainCore: kernel reboot window (09:00 UTC)\n\n[Timer]\nOnCalendar=*-*-* 09:00:00 UTC\n\n[Install]\nWantedBy=timers.target\n",
    }
    node_changed = []
    for path, content in NODE_FILES.items():
        if not os.path.exists(path) or open(path).read() != content:
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path + ".cc-new", "w") as f:
                f.write(content)
            os.chmod(path + ".cc-new", 0o755 if path.startswith("/usr/local/sbin/") else 0o644)
            os.replace(path + ".cc-new", path)
            node_changed.append(path)
    if run("bash", "-c", "command -v kexec", check=False).returncode != 0:
        # kexec-tools asks whether it should take over every reboot: no, only ours.
        run("bash", "-c", "echo 'kexec-tools kexec-tools/load_kexec boolean false' | debconf-set-selections && "
            "DEBIAN_FRONTEND=noninteractive apt-get install -y -q kexec-tools", check=False)
        node_changed.append("kexec-tools")
    if node_changed:
        run("systemctl", "daemon-reload", check=False)
        run("systemctl", "enable", "--now", "cc-kernel-reboot.timer", check=False)
        res["changes"].append("server updates")

    if CADDY_NEW:
        # The block list and its fail2ban jails (idempotent), then the node's
        # main file, which must name the list before any site imports it.
        if os.path.exists("/usr/local/sbin/cc-backups"):
            run("/usr/local/sbin/cc-backups", "guard-setup", check=False)
        if not os.path.exists(GUARD_FILE):
            with open(GUARD_FILE, "w") as f:
                f.write("(cc_guard) {\n\t@cc_guarded {\n\t\tpath */wp-login.php */xmlrpc.php\n\t\tclient_ip 192.0.2.1\n\t}\n\terror @cc_guarded 429\n}\n")
        os.chmod(GUARD_FILE, 0o644)
        main_path = "/etc/caddy/Caddyfile"
        main_old = open(main_path).read() if os.path.exists(main_path) else ""
        if main_old != MAIN_CADDYFILE:
            if main_old and not os.path.exists(main_path + ".before-captaincore"):
                open(main_path + ".before-captaincore", "w").write(main_old)
            open(main_path, "w").write(MAIN_CADDYFILE)
            os.chmod(main_path, 0o644)
            try:
                run("caddy", "validate", "--config", main_path, "--adapter", "caddyfile")
            except Exception as e:
                open(main_path, "w").write(main_old)
                done("failed", "the node's main edge file was rejected: %s" % e)
            res["changes"].append("edge main file")

    if edge_new != edge_old:
        if edge_old is not None:
            open(edge_prev_path, "w").write(edge_old)
        open(edge_path, "w").write(edge_new)
        os.chmod(edge_path, 0o644)
        try:
            caddy_reload()
        except Exception as e:
            if edge_old is not None:
                open(edge_path, "w").write(edge_old)
            else:
                os.remove(edge_path)
            caddy_reload()
            done("failed", "edge config rejected: %s" % e)
        res["changes"].append("edge")
    elif "edge main file" in res["changes"]:
        caddy_reload()

    if pc_changed and pc_on:
        if PC_PHP in pc_diff:
            # Lint before it replaces the file PHP loads on every request.
            run("incus", "exec", ct, "--", "sh", "-c", "mkdir -p /etc/captaincore && cat > '%s.cc-check'" % PC_PHP, inp=pc_php)
            lint = run("incus", "exec", ct, "--", "php", "-l", PC_PHP + ".cc-check", check=False)
            if lint.returncode != 0:
                run("incus", "exec", ct, "--", "rm", "-f", PC_PHP + ".cc-check", check=False)
                done("failed", "page cache file rejected by PHP: %s" % (lint.stdout or lint.stderr).strip()[:300])
            run("incus", "exec", ct, "--", "sh", "-c", "chmod 0644 '%s.cc-check' && mv '%s.cc-check' '%s'" % (PC_PHP, PC_PHP, PC_PHP))
        for path in pc_diff:
            if path != PC_PHP:
                ct_write_any(path, pc_want[path])
        run("incus", "exec", ct, "--", "sh", "-c",
            "install -d -m 0755 /var/cache/captaincore && install -d -o site -g site -m 0750 '%s'" % PC_ROOT)
        if PC_JSON in pc_diff or PC_PHP in pc_diff:
            # Other hostnames, exclusions or saving rules: saved pages may no longer be right.
            run("incus", "exec", ct, "--", "find", PC_ROOT, "-mindepth", "1", "-maxdepth", "1", "-exec", "rm", "-rf", "{}", "+", check=False)
        run("incus", "exec", ct, "--", "systemctl", "daemon-reload", check=False)
        run("incus", "exec", ct, "--", "systemctl", "enable", "--now", "cc-page-cache-gc.timer", check=False)
        res["changes"].append("cache")
    elif pc_changed:
        if PC_JSON in pc_diff:
            ct_write_any(PC_JSON, pc_json)
        run("incus", "exec", ct, "--", "systemctl", "disable", "--now", "cc-page-cache-gc.timer", check=False)
        run("incus", "exec", ct, "--", "rm", "-f", *PC_UNITS, check=False)
        run("incus", "exec", ct, "--", "systemctl", "daemon-reload", check=False)
        res["changes"].append("cache")
    if not pc_on and (pc_changed or php_changed):
        # Nothing serves saved pages once the rules below are gone; drop them.
        run("incus", "exec", ct, "--", "sh", "-c", "[ -d '%s' ] && find '%s' -mindepth 1 -maxdepth 1 -exec rm -rf {} + || true" % (PC_ROOT, PC_ROOT), check=False)

    # The prepend file and what it pulls in exist before PHP is told to load
    # them (a missing auto_prepend_file stops every request).
    for path in sorted(prepend_diff, key=lambda x: x == PREPEND_PHP):
        run("incus", "exec", ct, "--", "sh", "-c", "mkdir -p /etc/captaincore && cat > '%s.cc-check'" % path, inp=prepend_want[path])
        lint = run("incus", "exec", ct, "--", "php", "-n", "-l", path + ".cc-check", check=False)
        if lint.returncode != 0:
            run("incus", "exec", ct, "--", "rm", "-f", path + ".cc-check", check=False)
            done("failed", "%s rejected by PHP: %s" % (path, (lint.stdout or lint.stderr).strip()[:300]))
        run("incus", "exec", ct, "--", "sh", "-c", "chmod 0644 '%s.cc-check' && mv '%s.cc-check' '%s'" % (path, path, path))
    if prepend_diff and not php_changed:
        run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)
    if prepend_diff:
        res["changes"].append("prepend")

    if php_changed:
        for path, content in PHP_FILES:
            if php_old[path] != content:
                ct_write(path, content)
        # Validate as the site user: validating opens the log files, and a
        # root-owned access.log would stop PHP (running as site) from starting.
        v = run("incus", "exec", ct, "--", "su", "-s", "/bin/sh", "site", "-c",
                "cd /srv/site && frankenphp validate --config /etc/frankenphp/Caddyfile", check=False)
        run("incus", "exec", ct, "--", "chown", "-R", "site:site", "/srv/site/logs", check=False)

        def restore_php():
            for path, _ in PHP_FILES:
                if php_old[path] is not None:
                    ct_write(path, php_old[path])
            run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)

        if v.returncode != 0:
            restore_php()
            done("failed", "php config rejected: %s" % (v.stderr or v.stdout).strip()[:300])
        run("incus", "exec", ct, "--", "systemctl", "daemon-reload", check=False)
        r = run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)
        if r.returncode != 0:
            why = run("incus", "exec", ct, "--", "journalctl", "-u", "frankenphp", "-n", "3", "--no-pager", "-o", "cat", check=False).stdout.strip()[-300:]
            restore_php()
            done("failed", "PHP would not start with the new config, put the previous one back: %s" % why)
        res["changes"].append("php")

    if mem_changed:
        run("incus", "config", "set", ct, "limits.memory=" + want_mem)
        res["changes"].append("memory")

    if cpu_changed:
        run("incus", "config", "set", ct, "limits.cpu=" + want_cpus)
        res["changes"].append("cpu")

    if root_changed:
        r = run("incus", "config", "device", "set", ct, "root", "size=" + ROOT_SIZE, check=False)
        if r.returncode != 0:
            r = run("incus", "config", "device", "override", ct, "root", "size=" + ROOT_SIZE, check=False)
        if r.returncode == 0:
            res["changes"].append("root disk")
        else:
            res["warnings"] = res.get("warnings", []) + ["root disk not capped: %s" % (r.stderr or r.stdout).strip()[:200]]

    for v in refq_changed:
        run("incus", "storage", "volume", "set", "default", v, "zfs.use_refquota=true", check=False)
    if refq_changed:
        res["changes"].append("volume quotas")

    if bw_changed:
        run("incus", "config", "device", "set", ct, "eth0", "limits.egress=" + want_bw, "limits.ingress=" + want_bw)
        res["changes"].append("bandwidth")

    if suid_on:
        run("incus", "exec", ct, "--", "sh", "-c",
            "for f in %s; do [ -u \"$f\" ] || continue; m=$(stat -c %%a \"$f\"); "
            "dpkg-statoverride --list \"$f\" >/dev/null 2>&1 && dpkg-statoverride --remove \"$f\" >/dev/null 2>&1; "
            "dpkg-statoverride --update --add \"$(stat -c %%U \"$f\")\" \"$(stat -c %%G \"$f\")\" $(printf %%o $((0$m & 03777))) \"$f\"; done" % " ".join(suid_on), check=False)
        res["changes"].append("setuid")

    if harden_changed:
        for path, content in HARDEN_FILES:
            if path in harden_changed:
                ct_write_any(path, content)
        if any("sshd" in p for p in harden_changed):
            if run("incus", "exec", ct, "--", "sshd", "-t", check=False).returncode != 0:
                # Never leave SSH unable to start: put the old drop-in back.
                path = HARDEN_FILES[0][0]
                if harden_old[path] is None:
                    run("incus", "exec", ct, "--", "rm", "-f", path, check=False)
                else:
                    ct_write_any(path, harden_old[path])
                res["warnings"] = res.get("warnings", []) + ["sshd rejected the hardening drop-in; left as it was"]
            else:
                run("incus", "exec", ct, "--", "systemctl", "reload", "ssh", check=False)
        if any("resolved" in p for p in harden_changed):
            run("incus", "exec", ct, "--", "systemctl", "restart", "systemd-resolved", check=False)
        res["changes"].append("hardening")

    if db_twin_changed:
        r = db_sql("; ".join(db_twin_sql) + ";")
        if r.returncode == 0:
            res["changes"].append("database tunnel")
        else:
            res["warnings"] = res.get("warnings", []) + ["database tunnel user not updated: " + (r.stderr or "").strip()[:200]]

    if msmtp_changed:
        ct_write_any(MSMTP_PATH, msmtp_new)
    if relay_changed:
        tmp = CC_CLIENTS + ".cc-new"
        with open(tmp, "w") as f:
            f.write(relay_new)
        os.replace(tmp, CC_CLIENTS)
        run("postfix", "reload", check=False)
    if mail_ports_changed:
        if mail_on:
            run("ufw", "route", "delete", "deny", "from", ip, "to", "any", "port", MAIL_PORTS, "proto", "tcp", check=False)
        else:
            run("ufw", "route", "insert", "1", "deny", "from", ip, "to", "any", "port", MAIL_PORTS, "proto", "tcp",
                "comment", "cc-mail-off " + ct, check=False)
    if msmtp_changed or relay_changed or mail_ports_changed:
        res["changes"].append("mail")
    res["mail"] = {"relay": mail_on, "sendmail": has_msmtp}
    if cron_changed:
        run("incus", "exec", ct, "--", "systemctl", "enable" if cron_on else "disable", "--now", "wp-cron.timer", check=False)
        res["changes"].append("cron")
    if not plain:
        res["cron"] = cron_on
    if mail_on and not has_msmtp:
        # Seen for real: a container on a build of its image from before mail
        # support sends nothing, and nothing else says so.
        res["warnings"] = res.get("warnings", []) + ["This container has no mail program, so WordPress cannot send email. Its image build is older than mail support: run `captaincore host rebuild` for the site."]

    for name, files in app_changes.items():
        run("incus", "exec", ct, "--", "install", "-d", "-o", "site", "-g", "site", "/srv/site/apps/" + name, check=False)
        for path, content in files.items():
            ct_write_any(path, content)
        res["changes"].append("app:" + name)
    if apps:   # files written before env files were root only
        run("incus", "exec", ct, "--", "sh", "-c", "chmod 0600 /etc/captaincore/apps/*.env 2>/dev/null || true", check=False)
    for unit in removed_units:
        n = unit[len("cc-app-"):-len(".service")]
        run("incus", "exec", ct, "--", "systemctl", "disable", "--now", unit, check=False)
        run("incus", "exec", ct, "--", "rm", "-f", "/etc/systemd/system/" + unit, "/etc/captaincore/apps/%s.sh" % n, "/etc/captaincore/apps/%s.env" % n, check=False)
        res["changes"].append("removed:" + n)
    if app_changes or removed_units:
        run("incus", "exec", ct, "--", "systemctl", "daemon-reload", check=False)
    for a in apps:
        unit = "cc-app-%s.service" % a["name"]
        if a["name"] in app_changes:
            run("incus", "exec", ct, "--", "systemctl", "enable", unit, check=False)
            run("incus", "exec", ct, "--", "systemctl", "restart", unit, check=False)
        else:
            run("incus", "exec", ct, "--", "systemctl", "enable", "--now", unit, check=False)

    if cron_jobs_changed:
        for name, files in cron_file_changes.items():
            for path, content in files.items():
                ct_write_any(path, content)
        for unit in cron_removed:
            n = unit[len("cc-cron-"):-len(".timer")]
            run("incus", "exec", ct, "--", "systemctl", "disable", "--now", unit, check=False)
            run("incus", "exec", ct, "--", "rm", "-f", "/etc/systemd/system/" + unit, "/etc/systemd/system/cc-cron-%s.service" % n, "/etc/captaincore/cron/%s.sh" % n, check=False)
        run("incus", "exec", ct, "--", "systemctl", "daemon-reload", check=False)
        for j in cron_jobs:
            unit = "cc-cron-%s.timer" % j["name"]
            if unit in cron_should:
                run("incus", "exec", ct, "--", "systemctl", "enable", unit, check=False)
                run("incus", "exec", ct, "--", "systemctl", "restart", unit, check=False)
            else:
                run("incus", "exec", ct, "--", "systemctl", "disable", "--now", unit, check=False)
        res["changes"].append("scheduled tasks")
    # Last run of each task, for the Hosting tab.
    if cron_jobs:
        _names = ["cc-cron-%s.service" % j["name"] for j in cron_jobs]
        _show = run("incus", "exec", ct, "--", "systemctl", "show", *_names, "-p", "Id,Result,ExecMainStatus,ExecMainExitTimestampMonotonic,ExecMainExitTimestamp", check=False).stdout
        res["cron_jobs"] = {}
        for block in _show.strip().split("\n\n"):
            kv = dict(l.split("=", 1) for l in block.splitlines() if "=" in l)
            n = kv.get("Id", "")[len("cc-cron-"):-len(".service")]
            if n:
                ran = kv.get("ExecMainExitTimestampMonotonic", "0") not in ("", "0")
                res["cron_jobs"][n] = {"last_run": kv.get("ExecMainExitTimestamp", "") if ran else "",
                                       "result": (kv.get("Result", "") if ran else ""), "exit": kv.get("ExecMainStatus", "") if ran else ""}

    if debug_changed:
        flag = "true" if want_debug else "false"
        wp("config", "set", "WP_DEBUG", flag, "--raw", "--type=constant")
        wp("config", "set", "WP_DEBUG_LOG", flag, "--raw", "--type=constant")
        wp("config", "set", "WP_DEBUG_DISPLAY", "false", "--raw", "--type=constant")
        res["changes"].append("debug")

    if blog_public_fix:
        wp("option", "update", "blog_public", "0", "--quiet", check=False)
        res["changes"].append("blog_public")

    time.sleep(1.5)
    code_ct, code_edge = health()
    res["health"] = {"container": code_ct, "edge": code_edge}
    res["page_cache"] = {"enabled": pc_on, "ttl": pc_ttl}
    if pc_on and ok(code_ct):
        # The request above saved the home page if it can be saved; this one
        # says whether the web server now answers it without PHP.
        head = run("curl", "-s", "-o", "/dev/null", "-D", "-", "--max-time", "20", "-H", "Host: " + primary,
                   "-H", "X-Forwarded-Proto: https", "http://%s/" % ip, check=False).stdout
        m = re.search(r"(?im)^x-cache:\s*(\S+)", head)
        res["page_cache"]["home"] = m.group(1).upper() if m else "NONE"
    if not ok(code_ct):
        # Roll back what this run changed, then report.
        if php_changed:
            for path, _ in PHP_FILES:
                if php_old[path] is not None:
                    ct_write(path, php_old[path])
            run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)
        if "edge" in res["changes"] and edge_old is not None:
            open(edge_path, "w").write(edge_old)
            caddy_reload()
        if mem_changed and cur_mem:
            run("incus", "config", "set", ct, "limits.memory=" + cur_mem, check=False)
        res["rolled_back"] = res["changes"]
        res["changes"] = []
        done("failed", "site answered %s after the change, rolled back" % code_ct)
except SystemExit:
    raise
except Exception as e:
    done("failed", str(e)[:400])

# Each app answers on its port (any HTTP answer counts; an app that is down
# is reported, never rolled back: the site itself is fine).
if apps:
    time.sleep(2)
    res["apps"] = {}
    for a in apps:
        code = run("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "5",
                   "http://%s:%d/" % (ip, int(a["port"])), check=False).stdout
        active = run("incus", "exec", ct, "--", "systemctl", "is-active", "cc-app-%s.service" % a["name"], check=False).stdout.strip()
        res["apps"][a["name"]] = {"service": active, "http": code}

# Certificates Caddy holds for each hostname (issued once DNS points here).
base = "/var/lib/caddy/.local/share/caddy/certificates"
issued = set()
if os.path.isdir(base):
    for ca in os.listdir(base):
        for name in os.listdir(os.path.join(base, ca)):
            issued.add(name)
res["certs"] = {h: (h in issued) for h in hosts}
# A hostname whose DNS record was created moments ago can fail its first
# ACME attempt (resolvers still cache NXDOMAIN; anchorhost.xyz caches it
# for 180 s) and old Caddy's fallback issuer no longer works. Retry once
# after the negative cache has expired.
if not all(res["certs"].values()):
    run("systemd-run", "--on-active=240", "--unit=cc-caddy-cert-retry-%s" % ct, "--collect",
        "systemctl", "reload", "caddy", check=False)
    res["cert_retry"] = "in 4 minutes"
done("applied")
