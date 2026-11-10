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
res["revision"] = rev

# Everything below lands in config files or argv, so check it all again here.
HOST_RE = re.compile(r"^(?=.{1,253}$)([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$")
if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct):
    done("failed", "invalid container name")
if not re.match(r"^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip):
    done("failed", "invalid container ip")
if not hosts or not all(HOST_RE.match(h) for h in hosts) or primary not in hosts:
    done("failed", "invalid hostnames")
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


def site_body():
    # route keeps the written order: deny, redirects, auth, then the proxy.
    b = ["\troute {", "\t\tencode zstd gzip"]
    if deny:
        b += ["\t\t@cc_deny remote_ip %s" % " ".join(deny), "\t\trespond @cc_deny 403"]
    for r in redirects:
        b.append("\t\tredir %s %s %d" % (r["from"], r["to"], int(r["code"])))
    if pp_on:
        import base64
        b += ["\t\tbasicauth {", "\t\t\t%s %s" % (pp["username"], base64.b64encode(pp["password_hash"].encode()).decode()), "\t\t}"]
    b += ["\t\treverse_proxy %s:80 {" % ip, "\t\t\tlb_try_duration 15s", "\t\t\tlb_try_interval 250ms", "\t\t}", "\t}"]
    return b


edge = ["# Managed by CaptainCore: %s, revision %s. Changes here are overwritten." % (ct, rev)]
others = [h for h in hosts if h != primary]
if cfg.get("redirect_to_primary") and others:
    edge += ["%s {" % addrs([primary])] + site_body() + ["}", "",
             "%s {" % addrs(others), "\tredir https://%s{uri} permanent" % primary, "}"]
else:
    edge += ["%s {" % addrs(hosts)] + site_body() + ["}"]
edge_new = "\n".join(edge) + "\n"
edge_path = "/etc/caddy/sites/%s.caddyfile" % ct
edge_prev_path = "/etc/caddy/sites/.%s.prev" % ct
edge_old = open(edge_path).read() if os.path.exists(edge_path) else None


def caddy_reload():
    run("caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
    run("systemctl", "reload", "caddy")


# ── 2. PHP inside the container ────────────────────────────────────────────
ini_new = """; Managed by CaptainCore (hosting configuration). Changes here are overwritten.
memory_limit = %dM
upload_max_filesize = %dM
post_max_size = %dM
max_execution_time = %d
max_input_vars = 5000
opcache.memory_consumption = 128
opcache.interned_strings_buffer = 16
opcache.max_accelerated_files = 20000
opcache.revalidate_freq = 2
""" % (mem_limit, upload, upload, max_exec)

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
	encode zstd br gzip
	php_server
}
""" % (workers, max_workers)

PHP_FILES = [("/etc/php-zts/conf.d/99-captaincore.ini", ini_new), ("/etc/frankenphp/Caddyfile", fcaddy_new)]


def ct_read(path):
    p = run("incus", "exec", ct, "--", "cat", path, check=False)
    return p.stdout if p.returncode == 0 else None


def ct_write(path, content):
    # path is one of the constants in PHP_FILES, never input.
    run("incus", "exec", ct, "--", "sh", "-c", "cat > '%s.cc-new' && chmod 0644 '%s.cc-new' && mv '%s.cc-new' '%s'" % (path, path, path, path), inp=content)


php_old = {path: ct_read(path) for path, _ in PHP_FILES}
php_changed = any(php_old[path] != content for path, content in PHP_FILES)

def wp(*args, check=True):
    cmd = "cd /srv/site/public && wp " + " ".join("'%s'" % a.replace("'", "") for a in args)
    return run("incus", "exec", ct, "--", "su", "-s", "/bin/bash", "site", "-c", cmd, check=check)


want_debug = bool(cfg.get("debug"))
cur_debug = wp("config", "get", "WP_DEBUG", check=False).stdout.strip().lower() in ("1", "true")
debug_changed = want_debug != cur_debug

cur_mem = run("incus", "config", "get", ct, "limits.memory").stdout.strip()
want_mem = "%dMiB" % memory_mb
mem_changed = cur_mem != want_mem

if DRY:
    res["would_change"] = [n for n, c in (("edge", edge_new != edge_old), ("php", php_changed), ("memory", mem_changed), ("debug", debug_changed)) if c]
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

    if php_changed:
        for path, content in PHP_FILES:
            if php_old[path] != content:
                ct_write(path, content)
        v = run("incus", "exec", ct, "--", "frankenphp", "validate", "--config", "/etc/frankenphp/Caddyfile", check=False)
        if v.returncode != 0:
            for path, _ in PHP_FILES:
                if php_old[path] is not None:
                    ct_write(path, php_old[path])
            done("failed", "php config rejected: %s" % (v.stderr or v.stdout).strip()[:300])
        run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp")
        res["changes"].append("php")

    if mem_changed:
        run("incus", "config", "set", ct, "limits.memory=" + want_mem)
        res["changes"].append("memory")

    if debug_changed:
        flag = "true" if want_debug else "false"
        wp("config", "set", "WP_DEBUG", flag, "--raw", "--type=constant")
        wp("config", "set", "WP_DEBUG_LOG", flag, "--raw", "--type=constant")
        wp("config", "set", "WP_DEBUG_DISPLAY", "false", "--raw", "--type=constant")
        res["changes"].append("debug")

    time.sleep(1.5)
    code_ct, code_edge = health()
    res["health"] = {"container": code_ct, "edge": code_edge}
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
