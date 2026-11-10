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


SUSPENDED_HTML = ("<!doctype html><html lang=en><head><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'>"
                  "<title>Temporarily unavailable</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;"
                  "font:16px/1.6 -apple-system,Segoe UI,sans-serif;background:#f6f6f7;color:#1a1a1f}main{max-width:420px;padding:32px;text-align:center}"
                  "h1{font-size:22px;margin:0 0 8px}p{margin:0;color:#5e5e69}@media(prefers-color-scheme:dark){body{background:#0b0b0d;color:#ececed}p{color:#9d9da7}}"
                  "</style></head><body><main><h1>This site is temporarily unavailable</h1><p>Please check back later.</p></main></body></html>")


def site_body():
    # A suspended site answers every request (apps too) with a holding page.
    if cfg.get("suspended"):
        return ["\troute {", "\t\theader Retry-After 3600", "\t\theader Content-Type \"text/html; charset=utf-8\"",
                "\t\theader Cache-Control no-store", "\t\trespond \"%s\" 503" % SUSPENDED_HTML, "\t}"]
    # route keeps the written order: deny, redirects, auth, then the proxy.
    b = ["\troute {", "\t\tencode zstd gzip"]
    if deny:
        b += ["\t\t@cc_deny remote_ip %s" % " ".join(deny), "\t\trespond @cc_deny 403"]
    # Apps at a path of the site: public (before redirects and the password).
    for a in apps:
        if a.get("route") != "path":
            continue
        b += ["\t\t@cc_app_%s path %s %s/*" % (a["name"].replace("-", "_"), a["path"], a["path"]),
              "\t\troute @cc_app_%s {" % a["name"].replace("-", "_")]
        if a.get("strip_prefix"):
            b.append("\t\t\turi strip_prefix %s" % a["path"])
        b += ["\t\t\treverse_proxy %s:%d {" % (ip, int(a["port"])), "\t\t\t\tlb_try_duration 5s", "\t\t\t}", "\t\t}"]
    for r in redirects:
        b.append("\t\tredir %s %s %d" % (r["from"], r["to"], int(r["code"])))
    if pp_on:
        import base64
        b += ["\t\tbasicauth {", "\t\t\t%s %s" % (pp["username"], base64.b64encode(pp["password_hash"].encode()).decode()), "\t\t}"]
    b += ["\t\treverse_proxy %s:80 {" % ip, "\t\t\tlb_try_duration 15s", "\t\t\tlb_try_interval 250ms", "\t\t}", "\t}"]
    return b


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
        edge += ["", "%s {" % addrs([a["hostname"]])] + NOINDEX_LINE + ["\troute {", "\t\tencode zstd gzip"]
        if deny:
            edge += ["\t\t@cc_deny remote_ip %s" % " ".join(deny), "\t\trespond @cc_deny 403"]
        edge += ["\t\treverse_proxy %s:%d {" % (ip, int(a["port"])), "\t\t\tlb_try_duration 5s", "\t\t}", "\t}", "}"]
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
pc_on = bool(pc.get("enabled")) and pc_php.startswith("<?php")
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
opcache.memory_consumption = 128
opcache.interned_strings_buffer = 16
opcache.max_accelerated_files = 20000
opcache.revalidate_freq = 2
""" % (mem_limit, upload, upload, max_exec)
if pc_on:
    ini_new += "auto_prepend_file = %s\n" % PC_PHP

fcaddy_new = """# CaptainCore site container. TLS ends at the node edge; this serves plain
# HTTP on :80 to the bridge only. Managed by CaptainCore (hosting configuration).
{
	frankenphp {
		num_threads %d
		max_threads %d
		max_wait_time 15s
	}
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
""" % (workers, max_workers, static_block, page_blocks)

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

want_debug = bool(cfg.get("debug"))
cur_debug = wp("config", "get", "WP_DEBUG", check=False).stdout.strip().lower() in ("1", "true")
debug_changed = want_debug != cur_debug

cur_mem = run("incus", "config", "get", ct, "limits.memory").stdout.strip()
want_mem = "%dMiB" % memory_mb
mem_changed = cur_mem != want_mem

blog_public_fix = noindex and wp("option", "get", "blog_public", check=False).stdout.strip() != "0"

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

# Outgoing mail: the container's sendmail (msmtp) hands mail to the server's
# relay on 10.77.0.1:2525, which sends it through Mailgun with credentials the
# container never sees. The relay knows each container by its address: a tag
# for Mailgun's logs, or a refusal when mail is turned off for the site.
mail_on = cfg.get("mail", "relay") != "off"
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
    res["would_change"] = [n for n, c in (("cache", pc_changed), ("edge", edge_new != edge_old), ("php", php_changed), ("memory", mem_changed), ("debug", debug_changed), ("blog_public", blog_public_fix), ("hardening", bool(harden_changed)), ("cpu", cpu_changed), ("mail", msmtp_changed or relay_changed)) if c] \
        + ["app:" + n for n in app_changes] + ["remove:" + u for u in removed_units]
    res["edge"] = edge_new
    done("dry-run")


def health():
    code_ct = run("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20",
                  "-H", "Host: " + primary, "-H", "X-Forwarded-Proto: https", "http://%s/" % ip, check=False).stdout
    code_edge = run("curl", "-sk", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20",
                    "--resolve", "%s:443:127.0.0.1" % primary, "https://%s/" % primary, check=False).stdout
    return code_ct, code_edge


def ok(code):
    return code.isdigit() and 200 <= int(code) < 400


try:
    if edge_new != edge_old:
        if edge_old is not None:
            open(edge_prev_path, "w").write(edge_old)
        open(edge_path, "w").write(edge_new)
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
        if PC_JSON in pc_diff:
            # Other hostnames or exclusions: saved pages may no longer be right.
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

    if msmtp_changed:
        ct_write_any(MSMTP_PATH, msmtp_new)
    if relay_changed:
        tmp = CC_CLIENTS + ".cc-new"
        with open(tmp, "w") as f:
            f.write(relay_new)
        os.replace(tmp, CC_CLIENTS)
        run("postfix", "reload", check=False)
    if msmtp_changed or relay_changed:
        res["changes"].append("mail")
    res["mail"] = {"relay": mail_on, "sendmail": has_msmtp}

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
