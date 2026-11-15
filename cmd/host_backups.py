#!/usr/bin/env python3
# cc-backups: local backups (ZFS snapshots) for CaptainCore site containers.
# Installed on each node as /usr/local/sbin/cc-backups by `captaincore host
# backup`, which keeps it current; a systemd timer runs `cc-backups daily`.
# Python 3 standard library only. Every command prints one JSON line.
#
#   cc-backups create  <container> [daily|manual|pre-restore|pre-deploy]
#   cc-backups list    <container>
#   cc-backups restore <container> <snapshot> [both|files|database]
#   cc-backups delete  <container> <snapshot>
#   cc-backups daily                 snapshot every site container, then prune
#   cc-backups prune
#   cc-backups clone   <src> <dst> <ip> <ssh_port>        new container from src's image + volumes
#   cc-backups sync    <src> <dst> <both|files|database> <src_host> <dst_host>
#   cc-backups container <container> [1h|24h|7d]   programs, traffic, Caddyfiles, metrics
#   cc-backups restart <container> <php|database|cron|app-<name>|container>
#   cc-backups guard-setup                 sign-in protection: fail2ban jails that read the edge's sign-in log
#   cc-backups guard-status                addresses blocked from signing in right now
#   cc-backups guard-unban <address>       let an address sign in again
#   cc-backups guard-ban|guard-unbanned <jail> <address>   (called by fail2ban)
#   cc-backups image-build <8.3|8.4|8.5>   build cc-wp-php8x (the image script on stdin)
#   cc-backups image-list                  site images, and how many containers run each
#   cc-backups image-of <container>        the image a container runs, and whether a newer build exists
#   cc-backups rebuild <container> <image> same site on another image (a PHP switch, or a refreshed build)
#   cc-backups cache-status <container>  saved pages: how many, how big, how old
#   cc-backups cache-clear  <container>  delete every saved page
#   cc-backups site-park <container>     stop, rename <ct>-removed-<date>, unpublish (purged after 7 days)
#   cc-backups site-purge <parked>       delete one parked container and its volumes now
#   cc-backups site-restore <container> <host> <on|off> [restic|tar]   a long-term backup into a new site (restic: credentials
#                                        JSON on stdin, restored on this server; tar: a restic dump stream on stdin); cron on or off
#
# A backup is one atomic `zfs snapshot` of the container's files volume
# (/srv/site) and database volume (/var/lib/mysql), so the two always match.
# Restores copy out of the snapshot with rsync instead of rolling back, so
# every other backup survives, and a pre-restore snapshot is taken first.
import datetime
import json
import os
import re
import subprocess
import sys
import time

# Retention, per site: the newest 30 daily backups (by count, so a node that
# missed days still keeps 30), every pre-deploy and pre-restore safety backup
# younger than 30 days, and the newest 10 manual backups.
KEEP_COUNT = {"daily": 30, "manual": 10}
KEEP_DAYS = {"pre-deploy": 30, "pre-restore": 30}
TYPES = ("daily", "manual", "pre-restore", "pre-deploy")
NAME_RE = re.compile(r"^cc-(\d{8}T\d{6})-(daily|manual|pre-restore|pre-deploy)$")


def out(obj, code=0):
    print(json.dumps(obj))
    sys.exit(code)


def run(*args, check=True, inp=None):
    # Never hand our stdin to a child: `incus exec` forwards it into the
    # container, and site-restore's stdin is the backup.
    if inp is None:
        p = subprocess.run(list(args), capture_output=True, text=True, stdin=subprocess.DEVNULL)
    else:
        p = subprocess.run(list(args), capture_output=True, text=True, input=inp)
    if check and p.returncode != 0:
        raise RuntimeError("%s: %s" % (" ".join(args[:4]), (p.stderr or p.stdout).strip()[:400]))
    return p


def volumes(ct):
    """The container's files and database volumes: dataset + host mount."""
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct):
        raise RuntimeError("invalid container name")
    inst = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout)
    vols = {}
    for dev in inst.get("devices", {}).values():
        if dev.get("type") != "disk" or not dev.get("source") or not dev.get("pool"):
            continue
        role = {"/srv/site": "files", "/var/lib/mysql": "database"}.get(dev.get("path"))
        if not role:
            continue
        pool = dev["pool"]
        src = run("incus", "storage", "get", pool, "source").stdout.strip()
        vols[role] = {
            "dataset": "%s/custom/default_%s" % (src, dev["source"]),
            "mount": "/var/lib/incus/storage-pools/%s/custom/default_%s" % (pool, dev["source"]),
        }
    if set(vols) != {"files", "database"}:
        raise RuntimeError("container %s has no files + database volumes" % ct)
    return vols


def snapshots(ct):
    vols = volumes(ct)
    rows = run("zfs", "list", "-H", "-p", "-t", "snapshot", "-o", "name,used,referenced",
               vols["files"]["dataset"], vols["database"]["dataset"]).stdout.splitlines()
    by = {}
    for line in rows:
        full, used, ref = line.split("\t")
        ds, snap = full.split("@", 1)
        m = NAME_RE.match(snap)
        if not m:
            continue
        b = by.setdefault(snap, {"name": snap, "type": m.group(2), "used_bytes": 0, "size_bytes": 0, "parts": []})
        b["used_bytes"] += int(used)
        b["size_bytes"] += int(ref)
        b["parts"].append("files" if ds == vols["files"]["dataset"] else "database")
        b["created"] = datetime.datetime.strptime(m.group(1), "%Y%m%dT%H%M%S").replace(
            tzinfo=datetime.timezone.utc).isoformat()
    return vols, sorted(by.values(), key=lambda b: b["name"].split("-")[1], reverse=True)


def create(ct, kind):
    if kind not in TYPES:
        raise RuntimeError("unknown backup type")
    vols = volumes(ct)
    name = "cc-%s-%s" % (datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S"), kind)
    # One command, so both snapshots are taken at the same instant.
    run("zfs", "snapshot", "%s@%s" % (vols["files"]["dataset"], name), "%s@%s" % (vols["database"]["dataset"], name))
    return name


def health(ct):
    ip = ""
    state = json.loads(run("incus", "query", "/1.0/instances/%s/state" % ct).stdout)
    for a in (state.get("network", {}).get("eth0", {}).get("addresses") or []):
        if a.get("family") == "inet":
            ip = a["address"]
    if not ip:
        return "no-ip"
    return run("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20",
               "-H", "X-Forwarded-Proto: https", "http://%s/" % ip, check=False).stdout


# A plain site (HTML files, no WordPress) has the same volumes as any other,
# so backups, restores, rebuilds and parking work unchanged, but MariaDB and
# the WordPress cron timer stay masked. The mark is the container's
# user.cc_stack, which a rebuild carries over with the other user.* keys.
def is_plain(ct):
    return run("incus", "config", "get", ct, "user.cc_stack", check=False).stdout.strip() == "plain"


def db_off(ct):
    run("incus", "exec", ct, "--", "sh", "-c",
        # mask refuses wp-cron.timer (its unit file lives in /etc); disabled is
        # enough there, and the timer needs wp-config.php anyway. MariaDB may
        # have failed at first boot on the empty volume: clear that state.
        "systemctl disable --now mariadb wp-cron.timer >/dev/null 2>&1; systemctl mask mariadb >/dev/null 2>&1; "
        "systemctl reset-failed mariadb >/dev/null 2>&1; true", check=False)


# ── Page cache ─────────────────────────────────────────────────────────────
# Pages saved by the container's PHP (the prepend file `host apply` installs)
# and served by its web server. Clearing is always safe: the next visitor's
# request saves the page again.
PAGE_CACHE = "/var/cache/captaincore/page"


def cache_status(ct):
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct):
        raise RuntimeError("invalid container name")
    sh = ("R=%s; grep -q '\"enabled\": true' /etc/captaincore/page-cache.json 2>/dev/null && echo on || echo off; "
          "if [ -d \"$R\" ]; then find \"$R\" -type f -name '*.html' -printf '%%T@\\n' | "
          "awk 'BEGIN{n=0;a=0;b=0}{n++; if(a==0||$1<a)a=$1; if($1>b)b=$1}END{printf \"%%d %%d %%d\\n\", n, a, b}'; "
          "du -sk \"$R\" | cut -f1; else echo '0 0 0'; echo 0; fi") % PAGE_CACHE
    lines = run("incus", "exec", ct, "--", "sh", "-c", sh).stdout.split("\n")
    n, oldest, newest = (lines[1].split() + ["0", "0", "0"])[:3]
    return {"enabled": lines[0].strip() == "on", "pages": int(n), "bytes": int(lines[2].strip() or 0) * 1024 if int(n) else 0,
            "oldest": int(oldest), "newest": int(newest), "now": int(time.time())}


def cache_clear(ct):
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct):
        raise RuntimeError("invalid container name")
    run("incus", "exec", ct, "--", "sh", "-c",
        "[ -d '%s' ] && find '%s' -mindepth 1 -maxdepth 1 -exec rm -rf {} + || true" % (PAGE_CACHE, PAGE_CACHE), check=False)


def restore(ct, snap, what):
    if what not in ("both", "files", "database"):
        raise RuntimeError("restore what: both, files or database")
    vols, snaps = snapshots(ct)
    found = next((s for s in snaps if s["name"] == snap), None)
    if not found or set(found["parts"]) != {"files", "database"}:
        raise RuntimeError("backup %s not found" % snap)
    safety = create(ct, "pre-restore")
    done = []
    if what in ("both", "files"):
        src = "%s/.zfs/snapshot/%s/" % (vols["files"]["mount"], snap)
        # SSH keys are managed separately (ssh-keys deploy); keep the current ones.
        run("rsync", "-aHAX", "--numeric-ids", "--delete", "--exclude=/.ssh/", src, vols["files"]["mount"] + "/")
        done.append("files")
    if what in ("both", "database") and not is_plain(ct):
        run("incus", "exec", ct, "--", "systemctl", "stop", "mariadb")
        try:
            src = "%s/.zfs/snapshot/%s/" % (vols["database"]["mount"], snap)
            run("rsync", "-aHAX", "--numeric-ids", "--delete", src, vols["database"]["mount"] + "/")
        finally:
            run("incus", "exec", ct, "--", "systemctl", "start", "mariadb")
        done.append("database")
    cache_clear(ct)
    run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)
    time.sleep(1.5)
    return {"restored": done, "from": snap, "safety_backup": safety, "health": health(ct)}


def delete(ct, snap):
    vols, snaps = snapshots(ct)
    found = next((s for s in snaps if s["name"] == snap), None)
    if not found:
        raise RuntimeError("backup %s not found" % snap)
    for role in found["parts"]:
        run("zfs", "destroy", "%s@%s" % (vols[role]["dataset"], snap))


def site_containers():
    names = run("incus", "list", "-f", "csv", "-c", "n").stdout.split()
    out_ = []
    for n in names:
        if PARKED_RE.match(n) or OLDROOT_RE.match(n):
            continue
        try:
            volumes(n)
            out_.append(n)
        except Exception:
            pass
    return out_


def prune():
    removed = []
    now = datetime.datetime.now(datetime.timezone.utc)
    for ct in site_containers():
        _, snaps = snapshots(ct)
        by_type = {}
        for s in snaps:   # newest first
            by_type.setdefault(s["type"], []).append(s)
        for s in snaps:
            age = (now - datetime.datetime.fromisoformat(s["created"])).days
            t = s["type"]
            if (t in KEEP_DAYS and age >= KEEP_DAYS[t]) or (t in KEEP_COUNT and s in by_type[t][KEEP_COUNT[t]:]):
                delete(ct, s["name"])
                removed.append("%s@%s" % (ct, s["name"]))
    return removed


HOST_RE = re.compile(r"^(?=.{1,253}$)([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$")


def wp(ct, *args, check=True):
    cmd = "cd /srv/site/public && wp " + " ".join("'%s'" % a.replace("'", "") for a in args)
    return run("incus", "exec", ct, "--", "su", "-s", "/bin/bash", "site", "-c", cmd, check=check)


def clone(src, dst, ip, port):
    """New container dst from src's image and a copy of src's volumes."""
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", dst) or not re.match(r"^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip) \
            or not (1024 < int(port) < 65535):
        raise RuntimeError("invalid clone target")
    if run("incus", "info", dst, check=False).returncode == 0:
        raise RuntimeError("container %s already exists" % dst)
    inst = json.loads(run("incus", "query", "/1.0/instances/" + src).stdout)
    image = inst.get("config", {}).get("volatile.base_image", "")
    if not image:
        raise RuntimeError("cannot tell which image %s runs" % src)
    devs = {d.get("path"): d for d in inst.get("devices", {}).values() if d.get("type") == "disk" and d.get("source")}
    run("incus", "init", image, dst)
    run("incus", "config", "set", dst, "limits.memory=" + inst.get("config", {}).get("limits.memory", "1GiB"))
    run("incus", "config", "device", "override", dst, "eth0", "ipv4.address=" + ip)
    for path, suffix in (("/srv/site", "site"), ("/var/lib/mysql", "db")):
        d = devs[path]
        name = "%s-%s" % (dst, suffix)
        run("incus", "storage", "volume", "copy", "%s/%s" % (d["pool"], d["source"]), "%s/%s" % (d["pool"], name))
        run("incus", "storage", "volume", "attach", d["pool"], name, dst, path)
    run("incus", "config", "device", "add", dst, "ssh", "proxy", "listen=tcp:0.0.0.0:%d" % int(port), "connect=tcp:127.0.0.1:22")
    run("ufw", "allow", "%d/tcp" % int(port), check=False)
    run("incus", "start", dst)
    for _ in range(40):
        if run("incus", "exec", dst, "--", "systemctl", "is-active", "mariadb", check=False).stdout.strip() == "active":
            break
        time.sleep(1)
    return {"container": dst, "ip": ip, "ssh_port": int(port), "health": health(dst)}


# ── Sign-in protection ──────────────────────────────────────────────────────
# The edge logs every POST to wp-login.php and xmlrpc.php, on any site, to
# /var/log/caddy/auth.log. fail2ban reads that log: an address with too many
# failed sign-ins is blocked from signing in on every site of this server
# for an hour, longer each time it comes back. Blocked means the edge
# answers its requests to wp-login.php and xmlrpc.php with a "too many
# attempts" page (429); the rest of every site keeps working for it, so a
# customer who mistyped a password is not looking at a dead site. The list
# is one snippet, /etc/caddy/guard.caddyfile, that every site block imports.
GUARD_STATE = "/var/lib/captaincore/guard.json"
GUARD_FILE = "/etc/caddy/guard.caddyfile"
GUARD_LOG = "/var/log/caddy/auth.log"
GUARD_JAILS = ("cc-wp-login", "cc-xmlrpc")
ADDR_RE = re.compile(r"^[0-9A-Fa-f:.]{3,45}$")
# Jetpack's servers talk to xmlrpc.php all day (jetpack.com/ips-v4.json).
JETPACK = "122.248.245.244/32 54.217.201.243/32 54.232.116.4/32 192.0.64.0/18 195.234.108.0/22"
GUARD_CONF = {
    "/etc/fail2ban/filter.d/cc-wp-login.conf": """# Managed by CaptainCore. A failed WordPress sign-in: the form comes back (200).
# A good one is a redirect (302) and is not counted.
[Definition]
datepattern = "ts":{EPOCH}
failregex = ^.*"client_ip":"<ADDR>".*"method":"POST".*"uri":"[^"]*/wp-login\\.php[^"]*".*"status":200\\b
ignoreregex =
""",
    "/etc/fail2ban/filter.d/cc-xmlrpc.conf": """# Managed by CaptainCore. Any POST to xmlrpc.php: password guessing through it
# looks like every other call, so only the volume tells.
[Definition]
datepattern = "ts":{EPOCH}
failregex = ^.*"client_ip":"<ADDR>".*"method":"POST".*"uri":"[^"]*/xmlrpc\\.php[^"]*".*"status":200\\b
ignoreregex =
""",
    "/etc/fail2ban/action.d/cc-guard.conf": """# Managed by CaptainCore. Blocks sign-in at the edge instead of the firewall.
[Definition]
actionstart =
actionstop =
actioncheck =
actionban = /usr/local/sbin/cc-backups guard-ban <name> <ip>
actionunban = /usr/local/sbin/cc-backups guard-unbanned <name> <ip>
""",
    "/etc/fail2ban/jail.d/cc-guard.local": """# Managed by CaptainCore (sign-in protection for sites on this server).
[cc-wp-login]
enabled = true
filter = cc-wp-login
backend = auto
logpath = %s
banaction = cc-guard
maxretry = 10
findtime = 5m
bantime = 1h
bantime.increment = true
bantime.maxtime = 1d

[cc-xmlrpc]
enabled = true
filter = cc-xmlrpc
backend = auto
logpath = %s
banaction = cc-guard
maxretry = 60
findtime = 5m
bantime = 1h
bantime.increment = true
bantime.maxtime = 1d
ignoreip = 127.0.0.1/8 ::1 10.77.0.0/24 %s
""" % (GUARD_LOG, GUARD_LOG, JETPACK),
}


def _is_address(a):
    import ipaddress
    try:
        ipaddress.ip_address(a)
        return True
    except ValueError:
        return False


def _guard_snippet(addresses):
    # 192.0.2.1 is a documentation address nobody has: it keeps the matcher valid when the list is empty.
    return ("# Managed by CaptainCore (cc-backups guard-*). Addresses that may not sign in for now.\n"
            "(cc_guard) {\n\t@cc_guarded {\n\t\tpath */wp-login.php */xmlrpc.php\n\t\tclient_ip %s\n\t}\n\terror @cc_guarded 429\n}\n"
            % " ".join(sorted(addresses) or ["192.0.2.1"]))


def _guard_change(jail, address, banned):
    import fcntl
    os.makedirs(os.path.dirname(GUARD_STATE), exist_ok=True)
    with open(GUARD_STATE + ".lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            state = json.load(open(GUARD_STATE))
        except Exception:
            state = {}
        if jail:
            have = set(state.get(jail, []))
            have.add(address) if banned else have.discard(address)
            state[jail] = sorted(have)
            with open(GUARD_STATE + ".new", "w") as f:
                json.dump(state, f)
            os.replace(GUARD_STATE + ".new", GUARD_STATE)
        new = _guard_snippet({a for ips in state.values() for a in ips if _is_address(a)})
        old = open(GUARD_FILE).read() if os.path.exists(GUARD_FILE) else None
        if new == old:
            return False
        with open(GUARD_FILE + ".new", "w") as f:
            f.write(new)
        # Caddy runs as its own user and must be able to read this (fail2ban
        # creates files for root only; an unreadable import would also stop
        # the edge from starting at all).
        os.chmod(GUARD_FILE + ".new", 0o644)
        os.replace(GUARD_FILE + ".new", GUARD_FILE)
        if old is not None:
            # The edge reloads three seconds from now, once, however many
            # addresses change in between (a wave of blocks is one reload; a
            # reload keeps open connections and, measured under load, costs
            # about one brand-new connection). If one is already waiting it
            # will read this list, so a second request is simply refused.
            run("systemd-run", "--on-active=3", "--timer-property=AccuracySec=1s", "--unit=cc-guard-reload", "--collect", "--quiet",
                "systemctl", "reload", "caddy", check=False)
        return True


def guard_setup():
    changed = []
    if not os.path.isdir("/etc/fail2ban"):
        return {"installed": False, "changed": []}
    if not os.path.exists(GUARD_LOG):
        open(GUARD_LOG, "a").close()
        run("chown", "caddy:caddy", GUARD_LOG, check=False)
    for path, content in GUARD_CONF.items():
        if not os.path.exists(path) or open(path).read() != content:
            with open(path, "w") as f:
                f.write(content)
            changed.append(os.path.basename(path))
    _guard_change("", "", False)   # make sure the snippet exists
    for path in (GUARD_FILE, "/etc/caddy/Caddyfile"):
        if os.path.exists(path):
            os.chmod(path, 0o644)
    active = all(run("fail2ban-client", "status", j, check=False).returncode == 0 for j in GUARD_JAILS)
    if changed or not active:
        run("fail2ban-client", "reload", check=False)
    return {"installed": True, "changed": changed}


def guard_status():
    out_ = {"blocked": [], "jails": {}, "now": int(time.time())}
    for jail in GUARD_JAILS:
        st = run("fail2ban-client", "status", jail, check=False)
        if st.returncode != 0:
            out_["jails"][jail] = {"active": False}
            continue
        num = lambda label: int((re.search(label + r":\s+(\d+)", st.stdout) or [0, 0])[1])
        out_["jails"][jail] = {"active": True, "failed_now": num("Currently failed"), "failed_total": num("Total failed"),
                               "blocked_now": num("Currently banned"), "blocked_total": num("Total banned")}
        for line in run("fail2ban-client", "get", jail, "banip", "--with-time", check=False).stdout.splitlines():
            m = re.match(r"^(\S+)\s+(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \+ (\d+) = (\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)", line.strip())
            if m and ADDR_RE.match(m.group(1)):
                until = int(time.mktime(time.strptime(m.group(4), "%Y-%m-%d %H:%M:%S")))
                since = int(time.mktime(time.strptime(m.group(2), "%Y-%m-%d %H:%M:%S")))
                out_["blocked"].append({"address": m.group(1), "why": "sign-in" if jail == "cc-wp-login" else "xmlrpc", "since": since, "until": until})
    out_["blocked"].sort(key=lambda b: -b["since"])
    return out_


def guard_unban(address):
    if not _is_address(address):
        raise RuntimeError("not an address")
    run("fail2ban-client", "unban", address, check=False)
    for jail in GUARD_JAILS:   # in case fail2ban had already forgotten it
        _guard_change(jail, address, False)
    return guard_status()


# ── Images and rebuilds ─────────────────────────────────────────────────────
# One image per PHP branch (cc-wp-php83, cc-wp-php84, cc-wp-php85), built by
# the script the CLI carries (cmd/host_image.sh). A site's files and database
# live on their own volumes and everything else in its container is written
# by `host apply`, so changing PHP is not an upgrade in place: the site gets a
# new container from the other image, with the same volumes, address, SSH
# port and limits. The same step moves a site onto a refreshed build of its
# image (a new PHP or FrankenPHP release).
IMAGE_RE = re.compile(r"^cc-wp-php8[3-5]$")
OLDROOT_RE = re.compile(r"^([a-z0-9][a-z0-9-]*)-oldroot-(\d{8})\d{6}$")


def _images():
    """alias -> image, for the site images on this server."""
    found = {}
    for img in json.loads(run("incus", "image", "list", "-f", "json").stdout):
        for a in img.get("aliases") or []:
            if IMAGE_RE.match(a.get("name", "")):
                found[a["name"]] = img
    return found


def image_of(ct):
    if not CT_NAME.match(ct):
        raise RuntimeError("invalid container name")
    cfg = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout).get("config", {})
    images = _images()
    base = cfg.get("volatile.base_image", "")
    alias = cfg.get("user.cc_image", "")
    if not alias:
        # Containers from before rebuilds existed: note which image they came
        # from while that build is still here to ask.
        alias = next((a for a, img in images.items() if img.get("fingerprint") == base), "")
        if alias:
            run("incus", "config", "set", ct, "user.cc_image=" + alias, check=False)
    latest = (images.get(alias) or {}).get("fingerprint", "")
    return {"image": alias, "fingerprint": base[:12], "latest": latest[:12], "outdated": bool(latest) and latest != base,
            "available": sorted(images)}


def image_list():
    images = _images()
    using = {}
    for inst in json.loads(run("incus", "list", "-f", "json").stdout):
        if PARKED_RE.match(inst["name"]) or OLDROOT_RE.match(inst["name"]):
            continue
        fp = (inst.get("config") or {}).get("volatile.base_image", "")
        using.setdefault(fp, []).append(inst["name"])
    rows = []
    for alias, img in sorted(images.items()):
        fp = img.get("fingerprint", "")
        rows.append({"image": alias, "php": "8." + alias[-1], "fingerprint": fp[:12], "built": img.get("created_at", ""),
                     "size_bytes": img.get("size", 0), "description": (img.get("properties") or {}).get("description", ""),
                     "containers": sorted(using.get(fp, []))})
    return rows


def image_build(version, script):
    if version not in ("8.3", "8.4", "8.5"):
        raise RuntimeError("PHP version must be 8.3, 8.4 or 8.5")
    if "IMAGE-SETUP-OK" not in script:
        raise RuntimeError("no image script on stdin")
    branch = version.replace(".", "")
    alias, build = "cc-wp-php" + branch, "build-php" + branch
    run("incus", "delete", build, "--force", check=False)
    run("incus", "launch", "images:debian/13", build)
    try:
        for _ in range(40):
            if run("incus", "exec", build, "--", "getent", "hosts", "deb.debian.org", check=False).returncode == 0:
                break
            time.sleep(1)
        p = run("incus", "exec", build, "--env", "PHP_BRANCH=" + branch, "--", "bash", "-s", inp=script, check=False)
        if p.returncode != 0 or "IMAGE-SETUP-OK" not in p.stdout:
            raise RuntimeError("image script failed: %s" % (p.stderr or p.stdout).strip()[-400:])
        m = re.search(r"FrankenPHP v([\d.]+)", run("incus", "exec", build, "--", "sh", "-c", "frankenphp version | head -1", check=False).stdout)
        php = run("incus", "exec", build, "--", "php", "-r", "echo PHP_VERSION;", check=False).stdout.strip()
        if not m or not php.startswith(version + "."):
            raise RuntimeError("the build runs PHP %r, not %s" % (php[:40], version))
        maria = re.search(r"(\d+\.\d+)\.\d+-MariaDB", run("incus", "exec", build, "--", "mariadbd", "--version", check=False).stdout)
        desc = "CaptainCore WordPress site: Debian 13, FrankenPHP %s, PHP %s, MariaDB %s" % (m.group(1), php, maria.group(1) if maria else "?")
        run("incus", "stop", build, "--timeout", "60")
        old = _images().get(alias)
        # The new build takes the name in one step; containers on the old
        # build keep running and are noted as that image, so they show as
        # outdated rather than unknown.
        run("incus", "image", "alias", "delete", alias + "-new", check=False)
        run("incus", "publish", build, "--alias", alias + "-new", "description=" + desc)
        new_fp = json.loads(run("incus", "image", "list", alias + "-new", "-f", "json").stdout)[0]["fingerprint"]
        if old:
            for inst in json.loads(run("incus", "list", "-f", "json").stdout):
                c = inst.get("config") or {}
                if c.get("volatile.base_image") == old["fingerprint"] and not c.get("user.cc_image"):
                    run("incus", "config", "set", inst["name"], "user.cc_image=" + alias, check=False)
            run("incus", "image", "alias", "delete", alias)
        run("incus", "image", "alias", "create", alias, new_fp)
        run("incus", "image", "alias", "delete", alias + "-new", check=False)
        if old and old["fingerprint"] != new_fp:
            run("incus", "image", "delete", old["fingerprint"], check=False)   # kept on disk while containers still use it
        return {"image": alias, "fingerprint": new_fp[:12], "description": desc, "replaced": (old or {}).get("fingerprint", "")[:12]}
    finally:
        run("incus", "delete", build, "--force", check=False)


def _site_devices(ct, devs, add):
    """The devices that make a container this site: its volumes, its address, its SSH port."""
    for name, d in sorted(devs.items()):
        kind = d.get("type")
        if kind == "disk" and d.get("pool") and d.get("source"):
            if add:
                run("incus", "storage", "volume", "attach", d["pool"], d["source"], ct, name, d["path"])
            else:
                run("incus", "config", "device", "remove", ct, name)
        elif kind == "nic":
            keys = ["%s=%s" % (k, v) for k, v in d.items() if k not in ("type", "name", "network", "nictype", "parent")]
            if add and keys:
                run("incus", "config", "device", "override", ct, name, *keys)
            elif not add:
                run("incus", "config", "device", "remove", ct, name)   # also drops its DHCP lease
        elif kind == "proxy":
            if add:
                run("incus", "config", "device", "add", ct, name, "proxy", *["%s=%s" % (k, v) for k, v in d.items() if k != "type"])
            else:
                run("incus", "config", "device", "remove", ct, name)


def _wait_ready(ct, ip):
    for attempt in range(2):
        for _ in range(25):
            if not ip or ip in run("incus", "list", "^%s$" % ct, "-f", "csv", "-c", "4", check=False).stdout:
                break
            time.sleep(1)
        else:
            if attempt == 0:
                run("incus", "restart", ct, "--timeout", "30", check=False)
                continue
            raise RuntimeError("the container came up without its address %s" % ip)
        break
    if is_plain(ct):
        # A fresh root from the image has MariaDB enabled again.
        for _ in range(30):
            if run("incus", "exec", ct, "--", "true", check=False).returncode == 0:
                break
            time.sleep(1)
        db_off(ct)
        return
    for _ in range(40):
        if run("incus", "exec", ct, "--", "systemctl", "is-active", "mariadb", check=False).stdout.strip() == "active":
            return
        time.sleep(1)
    raise RuntimeError("the database did not start")


def rebuild(ct, alias):
    if not CT_NAME.match(ct) or PARKED_RE.match(ct) or OLDROOT_RE.match(ct):
        raise RuntimeError("invalid container name")
    if not IMAGE_RE.match(alias) or alias not in _images():
        raise RuntimeError("image %s is not on this server (build it with: captaincore node image build <server-id> --php=8.%s)" % (alias, alias[-1:]))
    volumes(ct)   # only a site container (files + database volumes) is rebuilt
    inst = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout)
    devs = inst.get("devices") or {}
    keep = {k: v for k, v in (inst.get("config") or {}).items() if k.split(".")[0] in ("limits", "security", "user", "raw", "boot")}
    ip = next((d.get("ipv4.address", "") for d in devs.values() if d.get("type") == "nic"), "")
    was = image_of(ct)
    was_running = inst.get("status") == "Running"
    safety = create(ct, "pre-deploy")
    # SSH host keys travel, or every key holder gets a "host key changed" warning.
    keys = run("incus", "exec", ct, "--", "sh", "-c", "cd /etc/ssh && tar -cf - ssh_host_* | base64 -w0", check=False).stdout.strip() if was_running else ""
    old = "%s-oldroot-%s" % (ct, time.strftime("%Y%m%d%H%M%S", time.gmtime()))
    started = time.time()
    if was_running:
        run("incus", "stop", ct, "--timeout", "60", check=False)
        if json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout).get("status") != "Stopped":
            run("incus", "stop", ct, "--force")
    _site_devices(ct, devs, add=False)
    run("incus", "rename", ct, old)
    try:
        run("incus", "init", alias, ct)
        for k, v in sorted(keep.items()):
            run("incus", "config", "set", ct, "%s=%s" % (k, v))
        run("incus", "config", "set", ct, "user.cc_image=" + alias)
        _site_devices(ct, devs, add=True)
        run("incus", "start", ct)
        _wait_ready(ct, ip)
        if keys:
            run("incus", "exec", ct, "--", "sh", "-c", "cd /etc/ssh && rm -f ssh_host_* && base64 -d | tar -xf - && systemctl restart ssh", inp=keys)
        # No answer yet means PHP is still starting: wait for it. An error
        # page is an answer: the site does not run on this image, and every
        # second spent re-asking is a second visitors see that error.
        code, errors = "000", 0
        for _ in range(20):
            code = health(ct)
            if code.isdigit() and 200 <= int(code) < 400:
                break
            if code.isdigit() and int(code) >= 400:
                errors += 1
                if errors >= 2:
                    raise RuntimeError("the site answered %s on %s" % (code, alias))
            time.sleep(1)
        else:
            raise RuntimeError("the site did not answer on %s" % alias)
    except Exception as e:
        # The old container has not been touched beyond its devices: put it back as it was.
        run("incus", "delete", ct, "--force", check=False)
        run("incus", "rename", old, ct)
        _site_devices(ct, devs, add=True)
        if was_running:
            run("incus", "start", ct, check=False)
        raise RuntimeError("rebuild failed and the previous container is back: %s" % str(e)[:300])
    offline = round(time.time() - started, 1)
    # One replaced container per site is enough to go back to: drop earlier ones.
    for n in run("incus", "list", "-f", "csv", "-c", "n").stdout.split():
        m = OLDROOT_RE.match(n)
        if m and m.group(1) == ct and n != old:
            run("incus", "delete", n, "--force", check=False)
    php = "PHP " + run("incus", "exec", ct, "--", "php", "-r", "echo PHP_VERSION;", check=False).stdout.strip()
    return {"container": ct, "image": alias, "was": was.get("image", ""), "offline_seconds": offline, "health": code,
            "safety_backup": safety, "old_root": old, "runs": php, "host_keys_kept": bool(keys)}


# ── Removing a site: park first, purge after a week ─────────────────────────
# A removed site's container is stopped and renamed <ct>-removed-<YYYYMMDD>;
# its edge block, firewall port and relay line go at once, so nothing serves
# it. The container, its volumes and their snapshots stay seven days (an
# operator can rename it back and re-apply), then `daily` purges them.
PARKED_RE = re.compile(r"^([a-z0-9][a-z0-9-]*)-removed-(\d{8})$")
PARK_DAYS = 7


def site_park(ct):
    if not CT_NAME.match(ct) or PARKED_RE.match(ct):
        raise RuntimeError("invalid container name")
    info = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout)
    # A mail block for this address would otherwise catch the next site given it.
    _addr = ((info.get("devices") or {}).get("eth0") or {}).get("ipv4.address", "")
    if _addr:
        run("ufw", "route", "delete", "deny", "from", _addr, "to", "any", "port", "465,587,2525", "proto", "tcp", check=False)
    devs = info.get("devices") or {}
    port = next((d.get("listen", "").rsplit(":", 1)[-1] for d in devs.values() if d.get("type") == "proxy"), "")
    ip = ((info.get("expanded_devices") or {}).get("eth0") or {}).get("ipv4.address", "")
    run("incus", "stop", ct, "--timeout", "60", check=False)
    # Give the address and the SSH port back now: the next new site is handed
    # the lowest free ones. Incus refuses an address a parked container still
    # claims, and the DHCP lease outlives a stop (the next container then
    # boots on a random address); only removing the network device drops the
    # lease. Both are noted on the container for whoever restores it:
    #   incus config device override <ct> eth0 ipv4.address=<ip> security.port_isolation=true
    #   incus config device add <ct> ssh proxy listen=tcp:0.0.0.0:<port> connect=tcp:127.0.0.1:22
    run("incus", "config", "set", ct, "user.cc_parked_ip=%s" % ip, "user.cc_parked_ssh_port=%s" % port, check=False)
    for name, d in devs.items():
        if d.get("type") == "proxy" or (d.get("type") == "nic" and d.get("ipv4.address")):
            run("incus", "config", "device", "remove", ct, name, check=False)
    parked = "%s-removed-%s" % (ct, time.strftime("%Y%m%d", time.gmtime()))
    run("incus", "rename", ct, parked)
    for path in ("/etc/caddy/sites/%s.caddyfile" % ct, "/etc/caddy/sites/.%s.prev" % ct):
        if os.path.exists(path):
            os.remove(path)
    run("systemctl", "reload", "caddy", check=False)
    if port.isdigit():
        run("ufw", "delete", "allow", "%s/tcp" % port, check=False)
    cc = "/etc/postfix/cc_clients"
    if ip and os.path.exists(cc):
        lines = [l for l in open(cc).read().splitlines() if not (l.split()[:1] == [ip])]
        open(cc, "w").write("\n".join(lines) + "\n")
        run("postfix", "reload", check=False)
    return {"parked": parked, "ssh_port": port, "ip": ip, "purge_after_days": PARK_DAYS}


def purge_parked(force_name=None):
    """Delete parked containers older than PARK_DAYS (or one by name), with their volumes."""
    purged = []
    for n in run("incus", "list", "-f", "csv", "-c", "n").stdout.split():
        m = PARKED_RE.match(n)
        if not m:
            continue
        age = (datetime.datetime.now(datetime.timezone.utc) - datetime.datetime.strptime(m.group(2), "%Y%m%d").replace(tzinfo=datetime.timezone.utc)).days
        if n != force_name and age < PARK_DAYS:
            continue
        info = json.loads(run("incus", "query", "/1.0/instances/" + n).stdout)
        vols = [d["source"] for d in (info.get("devices") or {}).values() if d.get("type") == "disk" and d.get("source") and d.get("pool")]
        run("incus", "delete", n, "--force")
        src = run("incus", "storage", "get", "default", "source").stdout.strip()
        for v in vols:
            if run("incus", "storage", "volume", "delete", "default", v, check=False).returncode != 0:
                # Our own zfs snapshots (local backups) block the delete: drop them, try again.
                run("zfs", "destroy", "-r", "%s/custom/default_%s" % (src, v), check=False)
                run("incus", "storage", "volume", "delete", "default", v, check=False)
        purged.append(n)
    # Containers a rebuild replaced: only the old root filesystem is left in
    # them (the volumes went to the new container), so only that is deleted.
    for n in run("incus", "list", "-f", "csv", "-c", "n").stdout.split():
        m = OLDROOT_RE.match(n)
        if not m:
            continue
        age = (datetime.datetime.now(datetime.timezone.utc) - datetime.datetime.strptime(m.group(2), "%Y%m%d").replace(tzinfo=datetime.timezone.utc)).days
        if n == force_name or age >= PARK_DAYS:
            info = json.loads(run("incus", "query", "/1.0/instances/" + n).stdout)
            if any(d.get("type") == "disk" and d.get("pool") and d.get("source") for d in (info.get("devices") or {}).values()):
                continue   # never delete something that still holds a site's volumes
            run("incus", "delete", n, "--force", check=False)
            purged.append(n)
    return purged


# MariaDB's data directory on the database volume, the wordpress database and
# its user ($DBPASS). Idempotent; wp-install sets the password it keeps.
DB_SETUP = """chown mysql:mysql /var/lib/mysql
[ -d /var/lib/mysql/mysql ] || mariadb-install-db --user=mysql --datadir=/var/lib/mysql --skip-test-db >/dev/null 2>&1
systemctl restart mariadb
mariadb -e "CREATE DATABASE IF NOT EXISTS wordpress CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE USER IF NOT EXISTS 'wordpress'@'localhost' IDENTIFIED BY '$DBPASS'; GRANT ALL ON wordpress.* TO 'wordpress'@'localhost';"
"""


def wp_enable(ct):
    """Make a plain site ready for WordPress (Install WordPress, before
    wp-install): a local backup, MariaDB unmasked and running with the
    wordpress database, the cron timer on, and whatever is in ~/public moved
    to ~/html-before-wordpress-<UTC datetime> so WordPress starts from an
    empty folder. Refuses a container that already has WordPress."""
    import secrets
    if not CT_NAME.match(ct) or PARKED_RE.match(ct) or OLDROOT_RE.match(ct):
        raise RuntimeError("invalid container name")
    if run("incus", "exec", ct, "--", "test", "-f", "/srv/site/public/wp-config.php", check=False).returncode == 0:
        raise RuntimeError("WordPress is already installed here (wp-config.php exists)")
    backup = create(ct, "pre-deploy")
    moved = "html-before-wordpress-" + time.strftime("%Y%m%d%H%M%S", time.gmtime())
    script = """set -e
systemctl unmask mariadb >/dev/null 2>&1 || true
systemctl enable mariadb wp-cron.timer >/dev/null 2>&1
""" + DB_SETUP + """systemctl start wp-cron.timer
if [ -n "$(ls -A /srv/site/public)" ]; then
  install -d -o site -g site -m 0755 "/srv/site/$MOVED"
  find /srv/site/public -mindepth 1 -maxdepth 1 -exec mv -t "/srv/site/$MOVED" {} +
  echo moved
fi
"""
    p = run("incus", "exec", ct, "--env", "DBPASS=" + secrets.token_hex(16), "--env", "MOVED=" + moved, "--", "bash", "-c", script)
    run("incus", "config", "unset", ct, "user.cc_stack", check=False)
    return {"container": ct, "backup": backup, "moved_to": ("~/" + moved) if "moved" in p.stdout else ""}


def create_site(ct, image, ip, port, disk_gib, memory_mib, stack="wordpress"):
    """A new site container from an image: its own files and database volumes,
    a fixed IP and SSH port, the site defaults (own host user ID range, no
    traffic to other containers, a process limit), MariaDB with a fresh
    database and user. Prints the database password once. A plain site
    (stack "plain") gets a small database volume and no MariaDB at all."""
    import secrets
    if stack not in ("wordpress", "plain"):
        raise RuntimeError("invalid stack")
    plain = stack == "plain"
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct) or not re.match(r"^[a-z0-9][a-z0-9.-]{0,63}$", image) \
            or not re.match(r"^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip) or not (1024 < int(port) < 65535) \
            or not (1 <= int(disk_gib) <= 200) or not (512 <= int(memory_mib) <= 8192):
        raise RuntimeError("invalid create arguments")
    if run("incus", "info", ct, check=False).returncode == 0:
        raise RuntimeError("container %s already exists" % ct)
    # The Manager picks the address and SSH port from the sites it knows; a
    # container it does not know about (made from another Manager, or not
    # imported yet) can already hold them.
    for other in json.loads(run("incus", "list", "-f", "json").stdout):
        nic = (other.get("devices") or {}).get("eth0") or {}
        if nic.get("ipv4.address") == ip:
            raise RuntimeError("address %s is already used by container %s" % (ip, other.get("name")))
        for dev in (other.get("devices") or {}).values():
            if dev.get("type") == "proxy" and dev.get("listen", "").endswith(":%d" % int(port)):
                raise RuntimeError("SSH port %d is already used by container %s" % (int(port), other.get("name")))
    pool = "default"
    # Caps count live data only (our snapshots never use up a site's space).
    run("incus", "storage", "set", pool, "volume.zfs.use_refquota=true", check=False)
    src = run("incus", "storage", "get", pool, "source").stdout.strip()
    for suffix in ("-site", "-db"):
        if run("incus", "storage", "volume", "show", pool, ct + suffix, check=False).returncode == 0:
            raise RuntimeError("volume %s%s already exists" % (ct, suffix))
    try:
        run("incus", "storage", "volume", "create", pool, ct + "-site", "size=%dGiB" % int(disk_gib))
        run("incus", "storage", "volume", "create", pool, ct + "-db", "size=%s" % ("1GiB" if plain else "10GiB"))
        run("zfs", "set", "recordsize=16K", "%s/custom/default_%s-db" % (src, ct), check=False)
        run("incus", "init", image, ct)
        run("incus", "config", "device", "override", ct, "root", "size=5GiB", check=False)
        for kv in ("limits.memory=%dMiB" % int(memory_mib), "limits.processes=1000", "security.idmap.isolated=true") \
                + (("user.cc_stack=plain",) if plain else ()):
            run("incus", "config", "set", ct, kv)
        run("incus", "config", "device", "override", ct, "eth0", "ipv4.address=" + ip, "security.port_isolation=true")
        run("incus", "storage", "volume", "attach", pool, ct + "-site", ct, "/srv/site")
        run("incus", "storage", "volume", "attach", pool, ct + "-db", ct, "/var/lib/mysql")
        run("incus", "config", "device", "add", ct, "ssh", "proxy", "listen=tcp:0.0.0.0:%d" % int(port), "connect=tcp:127.0.0.1:22")
        run("incus", "start", ct)
        # The edge and the mail relay know the site by this address, so a
        # container that came up on another one (a stale DHCP lease on the
        # address) is not a site yet. One restart asks again.
        for attempt in range(2):
            for _ in range(20):
                if ip in run("incus", "list", "^%s$" % ct, "-f", "csv", "-c", "4", check=False).stdout:
                    break
                time.sleep(1)
            else:
                if attempt == 0:
                    run("incus", "restart", ct, "--timeout", "30", check=False)
                    continue
                raise RuntimeError("container came up without its address %s (is it still leased to another container?)" % ip)
            break
    except Exception:
        # Nothing of this name existed a moment ago, so all of it is this
        # run's: take it away, or the next try would build on a half-made site.
        run("incus", "delete", ct, "--force", check=False)
        for suffix in ("-site", "-db"):
            run("incus", "storage", "volume", "delete", pool, ct + suffix, check=False)
        raise
    run("ufw", "allow", "%d/tcp" % int(port), check=False)
    for _ in range(30):
        if run("incus", "exec", ct, "--", "true", check=False).returncode == 0:
            break
        time.sleep(1)
    dbpass = secrets.token_hex(16)
    database = DB_SETUP
    if plain:
        db_off(ct)
        database = ""
    setup = """set -e
install -d -o site -g site -m 0755 /srv/site /srv/site/public /srv/site/apps
install -d -o site -g site -m 0750 /srv/site/logs
install -d -o site -g site -m 0700 /srv/site/.ssh
# The image carries the build container's SSH host keys: every site would
# share one key. Each site gets its own (a rebuild keeps these).
rm -f /etc/ssh/ssh_host_*
ssh-keygen -A >/dev/null
systemctl restart ssh
""" + database + """[ -n "$(ls -A /srv/site/public)" ] || { printf '%s\n' '<!doctype html><meta charset="utf-8"><meta name="robots" content="noindex"><title>Coming soon</title><style>body{font:16px/1.5 system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;color:#444}</style><p>This site is being set up.</p>' > /srv/site/public/index.html; chown site:site /srv/site/public/index.html; }
systemctl restart frankenphp || true
"""
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--", "bash", "-c", setup)
    idmap = run("incus", "config", "get", ct, "volatile.idmap.base", check=False).stdout.strip()
    if plain:
        return {"container": ct, "ip": ip, "ssh_port": int(port), "stack": "plain", "idmap": idmap}
    return {"container": ct, "ip": ip, "ssh_port": int(port), "db_name": "wordpress", "db_user": "wordpress", "db_password": dbpass,
            "idmap": idmap}


def wp_install(ct, url_host, title, admin_user, admin_email):
    """Fresh WordPress in a new container (after site-create). Reads the
    database password from MariaDB's grant (never passed around), writes
    wp-config with the edge's HTTPS header and the cron timer, installs."""
    import secrets
    if not HOST_RE.match(url_host) or not re.match(r"^[A-Za-z0-9 ._-]{1,60}$", title) \
            or not re.match(r"^[A-Za-z0-9._@-]{1,60}$", admin_user) or not re.match(r"^[^\s@'\"]+@[^\s@'\"]+$", admin_email):
        raise RuntimeError("invalid install arguments")
    dbpass = secrets.token_hex(16)
    adminpass = secrets.token_urlsafe(18)
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--", "bash", "-c",
        "mariadb -e \"ALTER USER 'wordpress'@'localhost' IDENTIFIED BY '$DBPASS'\"")
    script = """set -e
cd /srv/site/public
wp core download --quiet
wp config create --dbname=wordpress --dbuser=wordpress --dbpass="$DBPASS" --dbhost=localhost --skip-check --quiet --extra-php <<'PHP'
// TLS ends at the node edge; trust its forwarded scheme.
if ( ( $_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '' ) === 'https' ) { $_SERVER['HTTPS'] = 'on'; }
define( 'DISABLE_WP_CRON', true );
PHP
wp core install --url="https://$HOST" --title="$TITLE" --admin_user="$AUSER" --admin_email="$AEMAIL" --admin_password="$APASS" --skip-email --quiet
wp option update blog_public 0 --quiet
# Addresses people can read (/about/), as the web installer would set. With
# plain ones WordPress answers the home page at any path.
wp rewrite structure '/%postname%/' --quiet
"""
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--env", "HOST=" + url_host, "--env", "TITLE=" + title,
        "--env", "AUSER=" + admin_user, "--env", "AEMAIL=" + admin_email, "--env", "APASS=" + adminpass,
        "--", "su", "-s", "/bin/bash", "site", "-c", script)
    return {"url": "https://" + url_host, "admin_user": admin_user}


# Restoring a long-term (restic) backup into a new container. The CLI streams
# `restic dump <snapshot> / --archive tar` on stdin: the site's files with its
# database dump (database-backup.sql) at the top. Files a host put there for
# itself are moved aside, not deleted: an object cache drop-in pointing at the
# old host's Redis, and Kinsta's and WP Engine's must-use plugins.
HOST_LEFTOVERS = ("wp-content/object-cache.php", "wp-content/mu-plugins/kinsta-mu-plugins.php", "wp-content/mu-plugins/kinsta-mu-plugins",
                  "wp-content/mu-plugins/mu-plugin.php", "wp-content/mu-plugins/wpengine-common",
                  "wp-content/mu-plugins/wpengine-security-auditor.php", "wp-content/mu-plugins/wpe-wp-sign-on-plugin.php",
                  "wp-content/mu-plugins/wpe-wp-sign-on-plugin", "wp-content/mu-plugins/wpe-elasticpress-autosuggest-logger.php",
                  "wp-content/mu-plugins/force-strong-passwords", "wp-content/mu-plugins/slt-force-strong-passwords.php")

# Runs in the container with the new database password in $DBPASS: the edge's
# HTTPS header, the container's database, and the cron timer (not WP-Cron on
# page views), whatever the copied wp-config.php said.
WP_CONFIG_FIX = r"""<?php
$f = '/srv/site/public/wp-config.php';
$c = file_get_contents( $f );
if ( $c === false ) { fwrite( STDERR, "no wp-config.php\n" ); exit( 1 ); }
$set = function ( $name, $value ) use ( &$c ) {
	$re = '/define\s*\(\s*([\'"])' . $name . '\1\s*,\s*(?:[^;]|\n)*?\)\s*;/';
	$line = "define( '" . $name . "', " . var_export( $value, true ) . " );";
	if ( preg_match( $re, $c ) ) { $c = preg_replace( $re, addcslashes( $line, '\\$' ), $c, 1 ); }
	else { $c = preg_replace( '/<\?php/', "<?php\n" . addcslashes( $line, '\\$' ), $c, 1 ); }
};
$set( 'DB_NAME', 'wordpress' );
$set( 'DB_USER', 'wordpress' );
$set( 'DB_PASSWORD', getenv( 'DBPASS' ) );
$set( 'DB_HOST', 'localhost' );
$set( 'DISABLE_WP_CRON', true );
if ( strpos( $c, 'HTTP_X_FORWARDED_PROTO' ) === false ) {
	$c = preg_replace( '/<\?php/', "<?php\n// TLS ends at the node edge; trust its forwarded scheme.\nif ( ( \$_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '' ) === 'https' ) { \$_SERVER['HTTPS'] = 'on'; }", $c, 1 );
}
file_put_contents( $f, $c );
$multi = preg_match( '/define\s*\(\s*[\'"]MULTISITE[\'"]\s*,\s*true/i', $c ) ? 1 : 0;
$sub   = preg_match( '/define\s*\(\s*[\'"]SUBDOMAIN_INSTALL[\'"]\s*,\s*true/i', $c ) ? 1 : 0;
echo json_encode( [ 'multisite' => $multi, 'subdomain' => $sub ] );
"""


def _restore_restic(ct, creds):
    """restic restore on the node, straight from B2 into the container's files
    volume (parallel downloads; `restic dump` fetches one piece at a time).
    The credentials come on stdin and only ever live in restic's environment.
    Files land owned by the host's root, then become the site user's."""
    import shutil
    if not shutil.which("restic"):
        for _ in range(12):
            if run("apt-get", "install", "-y", "-q", "restic", check=False).returncode == 0:
                break
            time.sleep(10)   # unattended-upgrades holding the apt lock
        else:
            raise RuntimeError("could not install restic on the server")
    if not re.match(r"^b2:[A-Za-z0-9-]{6,50}:[A-Za-z0-9._/-]{1,400}$", creds.get("repo", "")) or not reResticID.match(creds.get("snapshot", "")):
        raise RuntimeError("invalid repo or snapshot")
    target = volumes(ct)["files"]["mount"] + "/public"
    env = dict(os.environ, B2_ACCOUNT_ID=creds["b2_id"], B2_ACCOUNT_KEY=creds["b2_key"], RESTIC_PASSWORD=creds["password"])
    # A cache for this run only: without one restic fetches every piece of
    # the file tree as its own request, thousands of them one after another.
    cache = "/var/cache/captaincore/restic-%s-%d" % (ct, os.getpid())
    os.makedirs(cache, mode=0o700, exist_ok=True)
    try:
        p = subprocess.run(["restic", "restore", creds["snapshot"], "--target", target, "--repo", creds["repo"], "--cache-dir", cache, "--no-lock",
                            "--exclude-xattr", "*", "-o", "b2.connections=10", "--quiet"], env=env, capture_output=True, text=True, stdin=subprocess.DEVNULL)
    finally:
        shutil.rmtree(cache, ignore_errors=True)
    if p.returncode != 0:
        raise RuntimeError("restic restore failed: " + (p.stderr or p.stdout).strip()[-300:])
    # Older snapshots can hold the site under its full original path; the
    # folder with the database dump is the site's top.
    top = target
    if not os.path.exists(target + "/database-backup.sql") and not os.path.exists(target + "/wp-config.php"):
        found = run("find", target, "-maxdepth", "8", "-name", "database-backup.sql", "-type", "f", "-not", "-path", "*/wp-content/*", check=False).stdout.split("\n")
        found = sorted((f for f in found if f), key=len)
        if found:
            top = os.path.dirname(found[0])
    if top != target:
        hold = volumes(ct)["files"]["mount"] + "/.restore-hoist"
        os.rename(top, hold)
        shutil.rmtree(target)
        os.rename(hold, target)
    uid = run("incus", "exec", ct, "--", "id", "-u", "site").stdout.strip()
    gid = run("incus", "exec", ct, "--", "id", "-g", "site").stdout.strip()
    base = int(run("incus", "config", "get", ct, "volatile.idmap.base").stdout.strip() or 0)
    if not uid.isdigit() or not gid.isdigit() or base <= 0:
        raise RuntimeError("could not read the container's user mapping")
    # -h: change a symlink itself, never what it points to (the host's files).
    run("chown", "-hR", "%d:%d" % (base + int(uid), base + int(gid)), target)
    run("chmod", "0755", target)


def _restore_tar(ct):
    """Unpack a tar stream on stdin into ~/public as the site user."""
    p = subprocess.run(["incus", "exec", "-T", ct, "--", "su", "-s", "/bin/bash", "site", "-c",
                        "tar -x -C /srv/site/public --no-same-owner --delay-directory-restore"],
                       stdin=sys.stdin.buffer, capture_output=True, text=True)
    if p.returncode != 0:
        noise = [l for l in (p.stdout + p.stderr).splitlines() if "Removing leading" not in l]
        raise RuntimeError("unpacking failed: " + "\n".join(noise)[-300:])


reResticID = re.compile(r"^[0-9a-f]{8,64}$")


def site_restore(ct, url_host, cron, mode="tar"):
    """A long-term backup into a new site's container: the files into
    ~/public (mode "restic": restic restore on this server with credentials
    from stdin; "tar": a `restic dump` tar stream on stdin), the dump imported
    into its database, wp-config pointed at that database, the old address
    replaced with this site's, search engines asked to stay away. With cron
    "off" the WordPress cron timer stays stopped so the copy does nothing on
    its own."""
    import secrets
    if not CT_NAME.match(ct) or PARKED_RE.match(ct) or OLDROOT_RE.match(ct) or not HOST_RE.match(url_host) \
            or cron not in ("on", "off") or mode not in ("tar", "restic", "tar-replace", "restic-replace"):
        raise RuntimeError("invalid restore arguments")
    # "-replace": an existing environment is refreshed from another one's
    # backup (host refresh); a local backup of it is taken first.
    replace = mode.endswith("-replace")
    mode = mode.replace("-replace", "")
    if is_plain(ct):
        raise RuntimeError("a plain site has no database to restore into")
    has_wp = run("incus", "exec", ct, "--", "test", "-f", "/srv/site/public/wp-config.php", check=False).returncode == 0
    if has_wp and not replace:
        raise RuntimeError("this site already has WordPress; restore into a new site")
    safety = create(ct, "pre-restore") if replace else ""
    creds = json.loads(sys.stdin.readline()) if mode == "restic" else None
    stamp = time.strftime("%Y%m%d%H%M%S", time.gmtime())
    aside = "/srv/site/restore-" + stamp
    # The placeholder page (or a failed attempt's files, which can belong to
    # the host's root) goes, from the host side; then the files arrive.
    run("find", volumes(ct)["files"]["mount"] + "/public", "-mindepth", "1", "-maxdepth", "1", "-exec", "rm", "-rf", "{}", "+")
    started = time.time()
    if mode == "restic":
        _restore_restic(ct, creds)
        creds = None
    else:
        _restore_tar(ct)
    files_secs = int(time.time() - started)
    check = run("incus", "exec", ct, "--", "bash", "-c",
                "cd /srv/site/public; ls database-backup-*.sql 2>/dev/null | head -1; [ -f database-backup.sql ] && echo DUMP; "
                "[ -f wp-config.php ] && echo CONFIG; du -sb . | cut -f1", check=False).stdout.split()
    if any(x.startswith("database-backup-") for x in check):
        raise RuntimeError("this is a WP Freighter tenant backup (its own database dump and content folder only); not supported yet")
    if "CONFIG" not in check:
        raise RuntimeError("the backup has no wp-config.php at its top; is it a WordPress site?")
    if "DUMP" not in check:
        raise RuntimeError("the backup has no database-backup.sql")
    files_bytes = int(check[-1]) if check and check[-1].isdigit() else 0
    # Out of the web root before anything else runs: the dump, and drop-ins
    # that would load on every request (even WP-CLI's with plugins skipped).
    moved = []
    run("incus", "exec", ct, "--", "install", "-d", "-o", "site", "-g", "site", "-m", "0700", aside)
    run("incus", "exec", ct, "--", "mv", "/srv/site/public/database-backup.sql", aside + "/database-backup.sql")
    for rel in HOST_LEFTOVERS:
        if run("incus", "exec", ct, "--", "test", "-e", "/srv/site/public/" + rel, check=False).returncode == 0:
            dest = aside + "/removed/" + os.path.dirname(rel)
            run("incus", "exec", ct, "--", "su", "-s", "/bin/bash", "site", "-c",
                "mkdir -p '%s' && mv '/srv/site/public/%s' '%s/'" % (dest, rel, dest))
            moved.append(rel)
    dbpass = secrets.token_hex(16)
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--", "bash", "-c",
        "mariadb -e \"ALTER USER 'wordpress'@'localhost' IDENTIFIED BY '$DBPASS'\"")
    if replace:
        # The old database goes whole: the copy may use another table prefix.
        run("incus", "exec", ct, "--", "mariadb", "-e",
            "DROP DATABASE IF EXISTS wordpress; CREATE DATABASE wordpress CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
    fixed = run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--", "su", "-s", "/bin/bash", "site", "-c", "php", inp=WP_CONFIG_FIX)
    flags = json.loads(fixed.stdout.strip().splitlines()[-1])
    if flags.get("subdomain"):
        raise RuntimeError("subdomain multisite can't run on one temporary address; not supported yet")
    started = time.time()
    run("incus", "exec", ct, "--", "bash", "-c", "mariadb wordpress < '%s/database-backup.sql'" % aside)
    db_secs = int(time.time() - started)
    db_bytes = int(run("incus", "exec", ct, "--", "stat", "-c", "%s", aside + "/database-backup.sql").stdout.strip() or 0)
    skip = ("--skip-plugins", "--skip-themes")
    old_home = wp(ct, "option", "get", "home", *skip).stdout.strip()
    old_host = re.sub(r"^https?://", "", old_home).split("/")[0].split(":")[0].lower()
    if old_host and old_host != url_host:
        for scheme in ("https://", "http://"):
            wp(ct, "search-replace", scheme + old_host, "https://" + url_host, "--all-tables", "--precise", "--quiet", *skip)
        wp(ct, "search-replace", old_host, url_host, "--all-tables", "--precise", "--quiet", *skip)
        if flags.get("multisite"):
            wp(ct, "config", "set", "DOMAIN_CURRENT_SITE", url_host, "--type=constant", "--quiet", *skip, check=False)
    wp(ct, "option", "update", "blog_public", "0", "--quiet", *skip, check=False)
    wp(ct, "cache", "flush", "--quiet", *skip, check=False)
    wp(ct, "rewrite", "flush", "--quiet", *skip, check=False)
    # The dump stays beside the files (outside the web root) until the copy
    # is checked: delete ~/restore-<datetime> when it is no longer needed.
    run("incus", "exec", ct, "--", "chown", "-R", "site:site", aside)
    run("incus", "exec", ct, "--", "systemctl", "enable" if cron == "on" else "disable", "--now", "wp-cron.timer", check=False)
    cache_clear(ct)
    run("incus", "exec", ct, "--", "systemctl", "restart", "frankenphp", check=False)
    time.sleep(1.5)
    return {"container": ct, "old_home": old_home, "home": "https://" + url_host, "files_bytes": files_bytes, "files_seconds": files_secs,
            "database_bytes": db_bytes, "database_seconds": db_secs, "moved_aside": moved, "kept_in": aside.replace("/srv/site", "~"),
            "multisite": bool(flags.get("multisite")), "cron": cron, "safety_backup": safety, "health": health(ct)}


def _idmap_base(ct):
    try:
        return int(run("incus", "config", "get", ct, "volatile.idmap.base", check=False).stdout.strip() or 0)
    except ValueError:
        return 0


def shift_owners(path, src_base, dst_base, span=65536):
    """Files copied from one container's volume to another's keep the first
    container's host user ids; each container has its own range (isolated
    idmap), so the copy would belong to nobody in the second one (MariaDB
    refuses to start). Moves every owner in the source range to the same
    place in the target range. lchown: a symlink itself, never its target."""
    if src_base == dst_base or src_base <= 0 or dst_base <= 0:
        return 0
    moved = 0
    for root, dirs, files in os.walk(path):
        for name in [""] + dirs + files:
            p = os.path.join(root, name) if name else root
            if name == "" and root != path:
                continue
            st = os.lstat(p)
            uid, gid = st.st_uid, st.st_gid
            nu = uid - src_base + dst_base if src_base <= uid < src_base + span else uid
            ng = gid - src_base + dst_base if src_base <= gid < src_base + span else gid
            if (nu, ng) != (uid, gid):
                os.lchown(p, nu, ng)
                moved += 1
    return moved


def sync(src, dst, what, src_host, dst_host):
    """Deploy src's files and/or database onto dst (a backup of dst first)."""
    if what not in ("both", "files", "database") or not HOST_RE.match(src_host) or not HOST_RE.match(dst_host):
        raise RuntimeError("invalid deploy arguments")
    sv, dv = volumes(src), volumes(dst)
    safety = create(dst, "pre-deploy")
    point = create(src, "pre-deploy")   # a consistent copy of the source to read from
    keep_public = wp(dst, "option", "get", "blog_public", check=False).stdout.strip()
    done = []
    if what in ("both", "files"):
        run("rsync", "-aHAX", "--numeric-ids", "--delete", "--exclude=/.ssh/", "--exclude=/public/wp-config.php",
            "%s/.zfs/snapshot/%s/" % (sv["files"]["mount"], point), dv["files"]["mount"] + "/")
        shift_owners(dv["files"]["mount"], _idmap_base(src), _idmap_base(dst))
        done.append("files")
    if what in ("both", "database"):
        run("incus", "exec", dst, "--", "systemctl", "stop", "mariadb")
        try:
            run("rsync", "-aHAX", "--numeric-ids", "--delete",
                "%s/.zfs/snapshot/%s/" % (sv["database"]["mount"], point), dv["database"]["mount"] + "/")
            shift_owners(dv["database"]["mount"], _idmap_base(src), _idmap_base(dst))
        finally:
            run("incus", "exec", dst, "--", "systemctl", "start", "mariadb")
        # The copy carries the source's database users; the target keeps its
        # own wp-config.php, so its database user gets the target's password
        # back (two containers built apart have different ones).
        dbpass = wp(dst, "config", "get", "DB_PASSWORD", "--skip-plugins", "--skip-themes", check=False).stdout.strip()
        if dbpass and re.match(r"^[A-Za-z0-9]{8,128}$", dbpass):
            run("incus", "exec", dst, "--env", "DBPASS=" + dbpass, "--", "bash", "-c",
                "mariadb -e \"ALTER USER 'wordpress'@'localhost' IDENTIFIED BY '$DBPASS'\"")
        # The copy carries the source's URLs and search engine setting; give
        # the destination back its own.
        if src_host != dst_host:
            wp(dst, "search-replace", "https://" + src_host, "https://" + dst_host, "--all-tables", "--precise", "--quiet")
            wp(dst, "search-replace", src_host, dst_host, "--all-tables", "--precise", "--quiet")
        if keep_public in ("0", "1"):
            wp(dst, "option", "update", "blog_public", keep_public, "--quiet", check=False)
        wp(dst, "cache", "flush", "--quiet", check=False)
        done.append("database")
    cache_clear(dst)
    run("incus", "exec", dst, "--", "systemctl", "restart", "frankenphp", check=False)
    time.sleep(1.5)
    return {"deployed": done, "from": src, "to": dst, "safety_backup": safety, "health": health(dst)}


# ── Server metrics ─────────────────────────────────────────────────────────
# `sample` runs every minute (cc-metrics.timer) and appends one JSON line to
# /var/lib/captaincore/metrics/<date>.jsonl (kept 14 days); `stats <range>`
# returns a fresh snapshot, system facts, containers and the history.
MDIR = "/var/lib/captaincore/metrics"


def _cpu_times():
    f = open("/proc/stat").readline().split()[1:]
    vals = [int(x) for x in f]
    idle = vals[3] + vals[4]
    return sum(vals), idle


def _meminfo():
    m = {}
    for line in open("/proc/meminfo"):
        k, v = line.split(":", 1)
        m[k] = int(v.split()[0]) * 1024
    arc = 0
    try:
        for line in open("/proc/spl/kstat/zfs/arcstats"):
            p = line.split()
            if p and p[0] == "size":
                arc = int(p[2])
    except OSError:
        pass
    return m, arc


def _uplink():
    best, rx, tx = "", 0, 0
    for line in open("/proc/net/dev").readlines()[2:]:
        name, data = line.split(":", 1)
        name = name.strip()
        if name == "lo" or name.startswith(("veth", "incus", "docker")):
            continue
        f = data.split()
        if int(f[0]) + int(f[8]) > rx + tx:
            best, rx, tx = name, int(f[0]), int(f[8])
    return best, rx, tx


def _pool():
    p = run("zpool", "list", "-H", "-p", "-o", "name,size,alloc,free,health,frag,cap", check=False).stdout.split("\n")[0].split("\t")
    if len(p) < 7:
        return {}
    return {"name": p[0], "size": int(p[1]), "alloc": int(p[2]), "free": int(p[3]), "health": p[4], "frag": p[5], "cap": p[6]}


def _containers():
    try:
        items = json.loads(run("incus", "query", "/1.0/instances?recursion=2").stdout)
    except Exception:
        return []
    out_ = []
    for i in items:
        st = i.get("state") or {}
        ip = ""
        for a in ((st.get("network") or {}).get("eth0") or {}).get("addresses") or []:
            if a.get("family") == "inet":
                ip = a["address"]
        devs = i.get("devices") or {}
        ssh = next((d.get("listen", "").rsplit(":", 1)[-1] for d in devs.values() if d.get("type") == "proxy"), "")
        out_.append({
            "name": i["name"], "status": i.get("status", ""),
            "cpu_ns": (st.get("cpu") or {}).get("usage", 0),
            "mem": (st.get("memory") or {}).get("usage", 0),
            "mem_limit": (st.get("memory") or {}).get("total", 0),
            "ip": ip, "ssh_port": ssh,
            "volumes": [d["source"] for d in devs.values() if d.get("type") == "disk" and d.get("source")],
            "started_at": st.get("started_at", ""),
        })
    return out_


def sample():
    os.makedirs(MDIR, exist_ok=True)
    state_path = os.path.join(MDIR, ".last")
    total, idle = _cpu_times()
    nic, rx, tx = _uplink()
    now = time.time()
    m, arc = _meminfo()
    cts = _containers()
    rec = {"t": int(now), "load": float(open("/proc/loadavg").read().split()[0]),
           "mem_total": m.get("MemTotal", 0), "mem_avail": m.get("MemAvailable", 0), "arc": arc,
           "swap_used": m.get("SwapTotal", 0) - m.get("SwapFree", 0)}
    pool = _pool()
    rec["disk_used"], rec["disk_size"] = pool.get("alloc", 0), pool.get("size", 0)
    try:
        last = json.load(open(state_path))
    except Exception:
        last = None
    if last and now > last["t"]:
        dt = now - last["t"]
        rec["cpu"] = round(100.0 * (1 - (idle - last["idle"]) / max(1, total - last["total"])), 1)
        rec["rx_bps"] = int(max(0, rx - last["rx"]) / dt)
        rec["tx_bps"] = int(max(0, tx - last["tx"]) / dt)
        ncpu = os.cpu_count() or 1
        rec["ct"] = {}
        for c in cts:
            prev = last.get("ct", {}).get(c["name"])
            if prev is not None and c["cpu_ns"] >= prev:
                rec["ct"][c["name"]] = {"cpu": round(100.0 * (c["cpu_ns"] - prev) / (dt * 1e9 * ncpu), 2), "mem": c["mem"]}
    json.dump({"t": now, "total": total, "idle": idle, "rx": rx, "tx": tx,
               "ct": {c["name"]: c["cpu_ns"] for c in cts}}, open(state_path, "w"))
    if "cpu" in rec:
        with open(os.path.join(MDIR, time.strftime("%Y-%m-%d.jsonl", time.gmtime(now))), "a") as f:
            f.write(json.dumps(rec, separators=(",", ":")) + "\n")
    cutoff = time.strftime("%Y-%m-%d", time.gmtime(now - 14 * 86400))
    for name in os.listdir(MDIR):
        if name.endswith(".jsonl") and name < cutoff:
            os.remove(os.path.join(MDIR, name))
    if time.gmtime(now).tm_min % 15 == 0:
        try:
            grow()
        except Exception:
            pass
    return rec


# Disk growth. A site's files and database volumes are capped (refquota:
# live data only, our snapshots never count), but ZFS only takes the space
# that is filled, so the cap is a brake against a runaway, not a size. A
# volume past 80% of its cap grows by half (at least 5 GiB) up to a ceiling;
# at the ceiling it is reported instead. Runs every 15 minutes from `sample`
# and on `cc-backups grow`; every change goes to GROW_LOG. The pool itself
# is watched in `stats` (warning at 70%, critical at 85%).
GROW_AT = 0.80
GROW_CEILING_GIB = {"files": 200, "database": 50}
GROW_LOG = "/var/lib/captaincore/growth.jsonl"


def grow():
    out = []
    for ct in site_containers():
        try:
            vols = volumes(ct)
        except Exception:
            continue
        for role, v in vols.items():
            p = dict(l.split("\t", 1) for l in run("zfs", "get", "-Hp", "-o", "property,value", "referenced,refquota,quota",
                                                       v["dataset"], check=False).stdout.splitlines() if "\t" in l)
            try:
                limit = int(p.get("refquota", "0")) or int(p.get("quota", "0"))
                used = int(p.get("referenced", "0"))
            except ValueError:
                continue
            if not limit or used < GROW_AT * limit:
                continue
            gib, ceiling = limit / 2 ** 30, GROW_CEILING_GIB.get(role, 50)
            rec = {"t": int(time.time()), "container": ct, "volume": role, "used_gib": round(used / 2 ** 30, 1), "from_gib": round(gib)}
            if gib >= ceiling:
                rec["at_ceiling"] = True
            else:
                new = int(min(ceiling, max(-(-gib * 1.5 // 1), -(-gib // 1) + 5)))
                vol = v["dataset"].split("/default_", 1)[1]
                r = run("incus", "storage", "volume", "set", "default", vol, "size=%dGiB" % new, check=False)
                rec["to_gib"] = new
                if r.returncode != 0:
                    rec["error"] = (r.stderr or r.stdout).strip()[:200]
            os.makedirs(os.path.dirname(GROW_LOG), exist_ok=True)
            with open(GROW_LOG, "a") as f:
                f.write(json.dumps(rec, separators=(",", ":")) + "\n")
            out.append(rec)
    return out


def growth_recent(n=20):
    try:
        return [json.loads(l) for l in open(GROW_LOG).read().splitlines()[-n:] if l.strip()]
    except Exception:
        return []


# Health: what needs someone on this server right now, as a complete state
# (the CLI's `node health` posts it to the Manager, which keeps it as Hosting
# signals: open while reported, resolved when gone). One list for the server
# and one per site container; each item is a stable key, a severity
# (critical / high: on the attention list; medium / low: recorded only), a
# title, a detail and the measured value.
OOM_STATE = "/var/lib/captaincore/oom.json"
OOM_WINDOW = 86400


def _item(key, severity, title, detail, value=""):
    return {"key": key, "severity": severity, "title": title, "detail": detail, "value": str(value)}


def server_health():
    srv = []
    pool = _pool()
    cap = int(100 * (pool.get("alloc", 0) or 0) / max(1, pool.get("size", 1) or 1))
    if cap >= 85:
        srv.append(_item("pool", "critical", "Storage pool nearly full", "The pool holding every site is %d%% full; volumes can no longer grow." % cap, "%d%%" % cap))
    elif cap >= 70:
        srv.append(_item("pool", "high", "Storage pool filling up", "The pool holding every site is %d%% full. Plan a move or more disk." % cap, "%d%%" % cap))
    if pool.get("health") and pool.get("health") != "ONLINE":
        srv.append(_item("pool-health", "critical", "Storage pool degraded", "zpool reports %s: a disk may have failed." % pool["health"], pool["health"]))
    st = os.statvfs("/")
    root_pct = int(100 * (1 - st.f_bavail / max(1, st.f_blocks)))
    if root_pct >= 85:
        srv.append(_item("root-disk", "high", "Server system disk filling up", "/ is %d%% full (logs, images, packages)." % root_pct, "%d%%" % root_pct))
    m, _arc = _meminfo()
    avail = 100 * m.get("MemAvailable", 0) / max(1, m.get("MemTotal", 1))
    if avail < 10:
        srv.append(_item("memory", "high", "Server memory nearly used up", "%d%% of memory is available." % avail, "%d%%" % avail))
    for unit, key, sev, title in (("caddy", "edge", "critical", "The edge (Caddy) is not running"),
                                  ("cc-metrics.timer", "metrics-timer", "medium", "Metrics and disk growth timer stopped"),
                                  ("cc-backups-daily.timer", "backups-timer", "high", "Local backups timer stopped"),
                                  ("fail2ban", "guard", "medium", "Sign-in protection (fail2ban) stopped")):
        state = run("systemctl", "is-active", unit, check=False).stdout.strip()
        if state != "active":
            srv.append(_item(key, sev, title, "%s is %s." % (unit, state or "unknown"), state))
    result = run("systemctl", "show", "cc-backups-daily.service", "-p", "Result", "--value", check=False).stdout.strip()
    if result and result != "success":
        srv.append(_item("backups-run", "high", "Last local backup run failed", "cc-backups-daily.service ended with %s; see `node logs backups`." % result, result))
    # A newer kernel than the running one, left for days: the 09:00 UTC
    # window (cc-kernel-reboot) has not taken it. Other updates restart
    # services and need no reboot.
    kernels = sorted((f[len("vmlinuz-"):] for f in os.listdir("/boot") if f.startswith("vmlinuz-")),
                     key=lambda v: [int(x) if x.isdigit() else x for x in re.split(r"[.-]", v)])
    if kernels and kernels[-1] != os.uname().release:
        age = time.time() - os.path.getmtime("/boot/vmlinuz-" + kernels[-1])
        if age > 3 * 86400:
            srv.append(_item("reboot", "medium", "New kernel not running for days", "Kernel %s has been installed for %d days and %s is still running; the 09:00 UTC reboot window has not taken it." % (kernels[-1], age // 86400, os.uname().release), kernels[-1]))

    try:
        oom = json.load(open(OOM_STATE))
    except Exception:
        oom = {}
    now = time.time()
    states = {c["name"]: c for c in json.loads(run("incus", "list", "-f", "json", check=False).stdout or "[]")}
    cts = {}
    usage = {}  # per container, bytes used and the cap of each volume: the Manager's storage figure
    for ct in site_containers():
        items = []
        c = states.get(ct) or {}
        if c.get("status") != "Running":
            items.append(_item("container", "critical", "Container is not running", "%s is %s." % (ct, c.get("status") or "missing"), c.get("status") or "missing"))
            cts[ct] = items
            continue
        units = ["frankenphp"] + ([] if is_plain(ct) else ["mariadb"])
        active = run("incus", "exec", ct, "--", "systemctl", "is-active", *units, check=False).stdout.split()
        for unit, state in zip(units, active):
            if state != "active":
                name = "PHP (FrankenPHP)" if unit == "frankenphp" else "MariaDB"
                items.append(_item(unit, "critical", name + " is not running", "%s in %s is %s." % (unit, ct, state), state))
        try:
            kills = int(next((l.split()[1] for l in open("/sys/fs/cgroup/lxc.payload.%s/memory.events" % ct) if l.startswith("oom_kill ")), "0"))
        except Exception:
            kills = None
        if kills is not None:
            prev = oom.get(ct) or {}
            if kills > prev.get("count", kills):
                prev["last"] = now
            prev["count"] = kills
            oom[ct] = prev
            if prev.get("last") and now - prev["last"] < OOM_WINDOW:
                items.append(_item("oom", "high", "Out of memory", "Something in %s was killed for running out of memory in the last day (container memory limit). Raise the site's memory or find the leak." % ct, kills))
        try:
            vols = volumes(ct)
        except Exception:
            vols = {}
        for role, v in vols.items():
            p = dict(l.split("\t", 1) for l in run("zfs", "get", "-Hp", "-o", "property,value", "referenced,refquota,quota", v["dataset"], check=False).stdout.splitlines() if "\t" in l)
            try:
                limit = int(p.get("refquota", "0")) or int(p.get("quota", "0"))
                used = int(p.get("referenced", "0"))
            except ValueError:
                continue
            usage.setdefault(ct, {})[role] = {"used": used, "limit": limit}
            if limit and used >= 0.95 * limit:
                at_ceiling = limit / 2 ** 30 >= GROW_CEILING_GIB.get(role, 50)
                items.append(_item("disk-" + role, "high", ("Files" if role == "files" else "Database") + " volume full",
                                   "%.1f of %d GiB used%s." % (used / 2 ** 30, round(limit / 2 ** 30), ", at the growth ceiling: raise it by hand" if at_ceiling else ", and it did not grow"),
                                   "%d%%" % (100 * used // limit)))
        try:
            _, snaps = snapshots(ct)
            newest = max((datetime.datetime.strptime(NAME_RE.match(sn["name"]).group(1), "%Y%m%dT%H%M%S")
                          .replace(tzinfo=datetime.timezone.utc).timestamp() for sn in snaps), default=0)
        except Exception:
            newest = 0
        if newest and now - newest > 30 * 3600:
            items.append(_item("local-backup", "high", "No recent local backup", "The newest local backup of %s is %d hours old." % (ct, (now - newest) // 3600), "%dh" % ((now - newest) // 3600)))
        cts[ct] = items
    os.makedirs(os.path.dirname(OOM_STATE), exist_ok=True)
    json.dump(oom, open(OOM_STATE, "w"))
    return {"server": srv, "containers": cts, "usage": usage, "checked_at": int(now)}


def history(seconds, points=240):
    since = time.time() - seconds
    rows = []
    for name in sorted(os.listdir(MDIR)) if os.path.isdir(MDIR) else []:
        if not name.endswith(".jsonl") or name < time.strftime("%Y-%m-%d", time.gmtime(since)):
            continue
        for line in open(os.path.join(MDIR, name)):
            try:
                r = json.loads(line)
            except ValueError:
                continue
            if r["t"] >= since:
                rows.append(r)
    if len(rows) <= points:
        return rows
    # Average into `points` buckets so a week stays light.
    step = seconds / points
    buckets = {}
    for r in rows:
        buckets.setdefault(int((r["t"] - since) // step), []).append(r)
    out_ = []
    for k in sorted(buckets):
        g = buckets[k]
        avg = lambda key: sum(x.get(key, 0) for x in g) / len(g)
        out_.append({"t": int(g[-1]["t"]), "cpu": round(avg("cpu"), 1), "load": round(avg("load"), 2),
                     "mem_total": g[-1]["mem_total"], "mem_avail": int(avg("mem_avail")), "arc": int(avg("arc")),
                     "rx_bps": int(avg("rx_bps")), "tx_bps": int(avg("tx_bps")),
                     "disk_used": g[-1]["disk_used"], "disk_size": g[-1]["disk_size"]})
    return out_


def stats(range_):
    seconds = {"1h": 3600, "24h": 86400, "7d": 7 * 86400}.get(range_, 3600)
    now = sample()  # also refreshes the CPU baseline
    osr = {}
    for line in open("/etc/os-release"):
        if "=" in line:
            k, v = line.rstrip().split("=", 1)
            osr[k] = v.strip('"')
    upd = run("bash", "-c", "apt-get -s -o Debug::NoLocking=1 upgrade 2>/dev/null | grep -c '^Inst' || true", check=False).stdout.strip()
    timers = {}
    for t in ("cc-backups-daily.timer", "cc-metrics.timer", "unattended-upgrades.service"):
        timers[t] = run("systemctl", "is-active", t, check=False).stdout.strip()
    lastrun = run("systemctl", "show", "cc-backups-daily.service", "-p", "ExecMainExitTimestamp", "--value", check=False).stdout.strip()
    cts = _containers()
    used = {}
    for line in run("zfs", "list", "-H", "-p", "-o", "name,used,quota,refquota", "-t", "filesystem", check=False).stdout.splitlines():
        name, u, q, rq = line.split("\t")
        used[name.rsplit("default_", 1)[-1]] = {"used": int(u), "quota": int(rq if rq != "0" else q)}
    snaps = {}
    for line in run("zfs", "list", "-H", "-p", "-t", "snapshot", "-o", "name,used", check=False).stdout.splitlines():
        n, u = line.split("\t")
        vol = n.split("@")[0].rsplit("default_", 1)[-1]
        snaps[vol] = snaps.get(vol, 0) + int(u)
    for c in cts:
        c["disk"] = sum(used.get(v, {}).get("used", 0) for v in c["volumes"])
        c["disk_quota"] = sum(used.get(v, {}).get("quota", 0) for v in c["volumes"])
        c["snapshots_bytes"] = sum(snaps.get(v, 0) for v in c["volumes"])
        c["cpu"] = (now.get("ct") or {}).get(c["name"], {}).get("cpu")
    root = os.statvfs("/")
    return {
        "now": now,
        "system": {
            "hostname": os.uname().nodename, "os": osr.get("PRETTY_NAME", ""), "kernel": os.uname().release,
            "uptime_s": int(float(open("/proc/uptime").read().split()[0])),
            "cpus": os.cpu_count(), "cpu_model": next((l.split(":", 1)[1].strip() for l in open("/proc/cpuinfo") if l.startswith("model name")), ""),
            "incus": run("incus", "version", check=False).stdout.split("Server version:")[-1].strip(),
            "zfs": run("zfs", "version", check=False).stdout.split("\n")[0].strip(),
            "pool": _pool(), "root_fs": {"size": root.f_blocks * root.f_frsize, "free": root.f_bavail * root.f_frsize},
            "updates_pending": int(upd or 0), "reboot_required": os.path.exists("/var/run/reboot-required"),
            "arc_max": int(open("/sys/module/zfs/parameters/zfs_arc_max").read() or 0),
            "timers": timers, "backups_last_run": lastrun,
        },
        "growth": growth_recent(),
        "pool_alert": (lambda c: "critical" if c >= 85 else "warning" if c >= 70 else "")(
            int(100 * (_pool().get("alloc", 0) or 0) / max(1, _pool().get("size", 1) or 1))),
        "containers": cts,
        "history": history(seconds),
        "range": range_,
    }


# ── One container, for its page ────────────────────────────────────────────
# `container <ct> [range]`: what runs inside (each systemd unit: WordPress on
# FrankenPHP, MariaDB, SSH, wp-cron, the apps), versions, database size, the
# last minutes of traffic from ~/logs/access.log, both Caddyfiles (the node
# edge's block and the container's own) and the container's metric history.
CT_NAME = re.compile(r"^[a-z0-9][a-z0-9-]{0,62}$")
CT_PROBE = r"""
echo @@versions
. /etc/os-release; echo "os=$PRETTY_NAME"
echo "frankenphp=$(frankenphp version 2>/dev/null | head -1)"
echo "php=$(php -r 'echo PHP_VERSION;' 2>/dev/null)"
echo "mariadb=$(mariadb --version 2>/dev/null)"
echo "wordpress=$(sed -n "s/^\$wp_version = '\(.*\)';/\1/p" /srv/site/public/wp-includes/version.php 2>/dev/null)"
echo "wp_config=$([ -f /srv/site/public/wp-config.php ] && echo 1 || echo 0)"
echo @@units
apps=$(cd /etc/systemd/system 2>/dev/null && ls cc-app-*.service 2>/dev/null)
systemctl show frankenphp.service mariadb.service ssh.service wp-cron.timer wp-cron.service $apps --no-pager \
  -p Id,ActiveState,SubState,ActiveEnterTimestamp,NRestarts,MemoryCurrent,MainPID,Result,ExecMainExitTimestamp,ExecMainStatus
echo @@listen
ss -Htln 2>/dev/null | awk '{print $4}'
echo @@db
mariadb -N -e "SELECT COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables WHERE table_schema='wordpress'; SHOW GLOBAL STATUS LIKE 'Threads_connected';" 2>/dev/null
echo @@caddyfile
cat /etc/frankenphp/Caddyfile 2>/dev/null
echo @@access
tail -n 4000 /srv/site/logs/access.log 2>/dev/null
"""


def _ct_history(ct, seconds, points=180):
    since = time.time() - seconds
    rows = []
    for name in sorted(os.listdir(MDIR)) if os.path.isdir(MDIR) else []:
        if not name.endswith(".jsonl") or name < time.strftime("%Y-%m-%d", time.gmtime(since)):
            continue
        for line in open(os.path.join(MDIR, name)):
            try:
                r = json.loads(line)
            except ValueError:
                continue
            c = (r.get("ct") or {}).get(ct)
            if r["t"] >= since and c:
                rows.append({"t": r["t"], "cpu": c.get("cpu", 0), "mem": c.get("mem", 0)})
    if len(rows) <= points:
        return rows
    step, buckets = seconds / points, {}
    for r in rows:
        buckets.setdefault(int((r["t"] - since) // step), []).append(r)
    return [{"t": g[-1]["t"], "cpu": round(sum(x["cpu"] for x in g) / len(g), 2), "mem": int(sum(x["mem"] for x in g) / len(g))}
            for g in (buckets[k] for k in sorted(buckets))]


def container(ct, range_):
    if not CT_NAME.match(ct):
        raise RuntimeError("invalid container name")
    seconds = {"1h": 3600, "24h": 86400, "7d": 7 * 86400}.get(range_, 3600)
    info = next((c for c in _containers() if c["name"] == ct), None)
    if not info:
        raise RuntimeError("no container named %s" % ct)
    cfg = json.loads(run("incus", "query", "/1.0/instances/" + ct).stdout)
    conf = cfg.get("config") or {}
    used = {}
    for line in run("zfs", "list", "-H", "-p", "-o", "name,used,quota,refquota", "-t", "filesystem", check=False).stdout.splitlines():
        name, u, q, rq = line.split("\t")
        used[name.rsplit("default_", 1)[-1]] = (int(u), int(rq if rq != "0" else q))
    snaps = 0
    for line in run("zfs", "list", "-H", "-p", "-t", "snapshot", "-o", "name,used", check=False).stdout.splitlines():
        n, u = line.split("\t")
        if n.split("@")[0].rsplit("default_", 1)[-1] in info["volumes"]:
            snaps += int(u)
    info["disk"] = sum(used.get(v, (0, 0))[0] for v in info["volumes"])
    info["disk_quota"] = sum(used.get(v, (0, 0))[1] for v in info["volumes"])
    info["snapshots_bytes"] = snaps
    info["limits"] = {k: conf.get(k, "") for k in ("limits.memory", "limits.processes", "limits.cpu", "limits.cpu.allowance",
                                                    "security.idmap.isolated", "security.nesting", "security.privileged")}
    nic = (cfg.get("expanded_devices") or {}).get("eth0") or {}
    info["limits"]["security.port_isolation"] = nic.get("security.port_isolation", "")
    info["image"] = conf.get("image.description", "")
    info["host_cpus"] = os.cpu_count() or 1
    hist = _ct_history(ct, seconds)
    info["cpu"] = hist[-1]["cpu"] if hist else None

    res = {"container": info, "history": hist, "range": range_, "versions": {}, "units": [], "listening": [],
           "db": {}, "caddyfile": "", "edge_caddyfile": "", "traffic": {},
           "stack": "plain" if conf.get("user.cc_stack") == "plain" else "wordpress"}
    edge_path = "/etc/caddy/sites/%s.caddyfile" % ct
    res["edge_caddyfile"] = open(edge_path).read() if os.path.exists(edge_path) else ""
    if info["status"] != "Running":
        return res
    p = run("incus", "exec", ct, "--", "bash", "-c", CT_PROBE, check=False)
    sec, buf = None, {}
    for line in p.stdout.split("\n"):
        if line.startswith("@@") and line[2:] in ("versions", "units", "listen", "db", "caddyfile", "access"):
            sec = line[2:]
            buf[sec] = []
        elif sec:
            buf[sec].append(line)
    for line in buf.get("versions", []):
        if "=" in line:
            k, v = line.split("=", 1)
            res["versions"][k] = v.strip()
    unit, units = {}, []
    for line in buf.get("units", []) + [""]:
        if not line.strip():
            if unit:
                units.append(unit)
            unit = {}
            continue
        k, _, v = line.partition("=")
        unit[k] = v
    roles = {"frankenphp.service": "php", "mariadb.service": "database", "ssh.service": "ssh", "wp-cron.timer": "cron-timer", "wp-cron.service": "cron"}
    for u in units:
        uid = u.get("Id", "")
        mem = u.get("MemoryCurrent", "")
        res["units"].append({
            "id": uid, "role": "app" if uid.startswith("cc-app-") else roles.get(uid, uid),
            "app": uid[len("cc-app-"):-len(".service")] if uid.startswith("cc-app-") else "",
            "active": u.get("ActiveState", ""), "sub": u.get("SubState", ""), "since": u.get("ActiveEnterTimestamp", ""),
            "restarts": int(u.get("NRestarts") or 0) if (u.get("NRestarts") or "").isdigit() else 0,
            "memory": int(mem) if mem.isdigit() else None, "pid": int(u.get("MainPID") or 0) if (u.get("MainPID") or "").isdigit() else 0,
            "result": u.get("Result", ""), "last_exit": u.get("ExecMainExitTimestamp", ""), "exit_status": u.get("ExecMainStatus", ""),
        })
    res["listening"] = sorted({l.rsplit(":", 1)[-1] for l in buf.get("listen", []) if ":" in l}, key=lambda x: int(x) if x.isdigit() else 0)
    db = [l for l in buf.get("db", []) if l.strip()]
    if db:
        res["db"]["bytes"] = int(db[0]) if db[0].strip().isdigit() else 0
        if len(db) > 1:
            res["db"]["connections"] = int(db[1].split("\t")[-1] or 0)
    res["caddyfile"] = "\n".join(buf.get("caddyfile", [])).rstrip() + "\n"
    # Traffic WordPress served (the edge sends app paths straight to the app,
    # so these are the site's own requests): the last 15 minutes, or what the
    # tail covers on a busy site.
    now, window = time.time(), 900
    hits, codes, slow, oldest = 0, {"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0}, [], None
    for line in buf.get("access", []):
        try:
            r = json.loads(line)
        except ValueError:
            continue
        ts = r.get("ts", 0)
        if ts < now - window:
            continue
        oldest = ts if oldest is None else min(oldest, ts)
        hits += 1
        st = int(r.get("status", 0))
        if 200 <= st < 600:
            codes["%dxx" % (st // 100)] += 1
        slow.append(float(r.get("duration", 0)))
    tail_full = len([l for l in buf.get("access", []) if l.strip()]) >= 3990
    span = max(60, now - oldest) if tail_full and oldest and hits >= 3990 else window
    slow.sort()
    res["traffic"] = {"window_s": int(span), "requests": hits, "per_min": round(hits / span * 60, 1), "codes": codes,
                      "p95_ms": round(slow[int(len(slow) * 0.95) - 1] * 1000) if len(slow) >= 20 else None}
    return res


def restart(ct, what):
    if not CT_NAME.match(ct):
        raise RuntimeError("invalid container name")
    m = APP_UNIT.match(what)
    if what == "container":
        run("incus", "restart", ct, "--timeout", "60")
    elif what in ("php", "database", "cron") or m:
        unit = {"php": "frankenphp", "database": "mariadb", "cron": "wp-cron.timer"}.get(what) or "cc-app-" + m.group(1)
        run("incus", "exec", ct, "--", "systemctl", "restart", unit)
    else:
        raise RuntimeError("restart php, database, cron, app-<name> or container")
    time.sleep(2 if what != "container" else 4)
    return {"restarted": what, "health": health(ct)}


# ── Server logs ────────────────────────────────────────────────────────────
# Fixed sources only; "ct:<container>:<php|database|ssh>" reads a container's
# journal. Returns the last N lines as text.
LOG_SOURCES = {
    "edge": ["journalctl", "-u", "caddy", "--no-pager", "-o", "short-iso"],
    "ssh": ["journalctl", "-u", "ssh", "--no-pager", "-o", "short-iso"],
    "backups": ["journalctl", "-u", "cc-backups-daily", "--no-pager", "-o", "short-iso"],
    "kernel": ["journalctl", "-k", "-p", "warning", "--no-pager", "-o", "short-iso"],
    "incus": ["journalctl", "-u", "incus", "--no-pager", "-o", "short-iso"],
    "updates": ["tail", "/var/log/unattended-upgrades/unattended-upgrades.log"],
}
CT_UNITS = {"php": "frankenphp", "database": "mariadb", "ssh": "ssh", "cron": "wp-cron"}
APP_UNIT = re.compile(r"^app-([a-z][a-z0-9-]{0,19})$")


def logs(source, lines):
    lines = max(10, min(int(lines), 2000))
    if source.startswith("ct:"):
        _, ct, unit = (source.split(":") + ["", ""])[:3]
        m = APP_UNIT.match(unit)
        if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct) or (unit not in CT_UNITS and not m):
            raise RuntimeError("unknown log source")
        name = CT_UNITS[unit] if unit in CT_UNITS else "cc-app-" + m.group(1)
        cmd = ["incus", "exec", ct, "--", "journalctl", "-u", name, "--no-pager", "-o", "short-iso", "-n", str(lines)]
    elif source in LOG_SOURCES:
        cmd = LOG_SOURCES[source] + (["-n", str(lines)] if LOG_SOURCES[source][0] == "journalctl" else [])
        if cmd[0] == "tail":
            cmd = ["tail", "-n", str(lines), cmd[1]]
    else:
        raise RuntimeError("unknown log source")
    p = run(*cmd, check=False)
    return (p.stdout or p.stderr or "").rstrip()


try:
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "create":
        out({"status": "ok", "backup": create(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else "manual")})
    elif cmd == "list":
        _, snaps = snapshots(sys.argv[2])
        out({"status": "ok", "backups": snaps})
    elif cmd == "restore":
        out(dict(status="ok", **restore(sys.argv[2], sys.argv[3], sys.argv[4] if len(sys.argv) > 4 else "both")))
    elif cmd == "delete":
        delete(sys.argv[2], sys.argv[3])
        out({"status": "ok", "deleted": sys.argv[3]})
    elif cmd == "daily":
        made = {ct: create(ct, "daily") for ct in site_containers()}
        out({"status": "ok", "created": made, "pruned": prune(), "purged": purge_parked()})
    elif cmd == "guard-setup":
        out(dict(status="ok", **guard_setup()))
    elif cmd == "guard-status":
        out(dict(status="ok", **guard_status()))
    elif cmd == "guard-unban":
        out(dict(status="ok", **guard_unban(sys.argv[2])))
    elif cmd in ("guard-ban", "guard-unbanned"):
        if sys.argv[2] not in GUARD_JAILS or not _is_address(sys.argv[3]):
            raise RuntimeError("invalid jail or address")
        out({"status": "ok", "changed": _guard_change(sys.argv[2], sys.argv[3], cmd == "guard-ban")})
    elif cmd == "image-build":
        out(dict(status="ok", **image_build(sys.argv[2], sys.stdin.read())))
    elif cmd == "image-list":
        out({"status": "ok", "images": image_list()})
    elif cmd == "image-of":
        out(dict(status="ok", **image_of(sys.argv[2])))
    elif cmd == "rebuild":
        out(dict(status="ok", **rebuild(sys.argv[2], sys.argv[3])))
    elif cmd == "cache-status":
        out(dict(status="ok", **cache_status(sys.argv[2])))
    elif cmd == "cache-clear":
        before = cache_status(sys.argv[2])
        cache_clear(sys.argv[2])
        out(dict(status="ok", cleared=before["pages"], **cache_status(sys.argv[2])))
    elif cmd == "site-park":
        out(dict(status="ok", **site_park(sys.argv[2])))
    elif cmd == "site-purge":
        # An operator purging one parked container before its seven days are up.
        if not PARKED_RE.match(sys.argv[2]) and not OLDROOT_RE.match(sys.argv[2]):
            raise RuntimeError("only a parked container (<name>-removed-<date>) or an old root (<name>-oldroot-<date>) can be purged")
        out({"status": "ok", "purged": purge_parked(sys.argv[2])})
    elif cmd == "clone":
        out(dict(status="ok", **clone(sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5]))))
    elif cmd == "sync":
        out(dict(status="ok", **sync(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6])))
    elif cmd == "site-create":
        out(dict(status="ok", **create_site(sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5]), int(sys.argv[6]), int(sys.argv[7]),
                                            sys.argv[8] if len(sys.argv) > 8 else "wordpress")))
    elif cmd == "authorize":
        ct, key = sys.argv[2], sys.argv[3]
        if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct) or not re.match(r"^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)) [A-Za-z0-9+/]+={0,3}$", key):
            raise RuntimeError("invalid authorize arguments")
        run("incus", "exec", ct, "--env", "KEY=" + key + " captaincore-management", "--", "sh", "-c",
            "install -d -o site -g site -m 0700 /srv/site/.ssh && touch /srv/site/.ssh/authorized_keys && "
            "grep -qF \"$KEY\" /srv/site/.ssh/authorized_keys || printf '%s\\n' \"$KEY\" >> /srv/site/.ssh/authorized_keys; "
            "chown site:site /srv/site/.ssh/authorized_keys && chmod 600 /srv/site/.ssh/authorized_keys")
        out({"status": "ok"})
    elif cmd == "wp-enable":
        out(dict(status="ok", **wp_enable(sys.argv[2])))
    elif cmd == "health":
        out(dict(status="ok", **server_health()))
    elif cmd == "grow":
        out({"status": "ok", "grown": grow()})
    elif cmd == "site-restore":
        out(dict(status="ok", **site_restore(sys.argv[2], sys.argv[3], sys.argv[4] if len(sys.argv) > 4 else "off",
                                             sys.argv[5] if len(sys.argv) > 5 else "tar")))
    elif cmd == "wp-install":
        out(dict(status="ok", **wp_install(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6])))
    elif cmd == "rehost":
        ct, frm, to, public = sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5]
        if not HOST_RE.match(frm) or not HOST_RE.match(to) or public not in ("0", "1"):
            raise RuntimeError("invalid rehost arguments")
        wp(ct, "search-replace", "https://" + frm, "https://" + to, "--all-tables", "--precise", "--quiet")
        wp(ct, "search-replace", frm, to, "--all-tables", "--precise", "--quiet")
        wp(ct, "option", "update", "blog_public", public, "--quiet")
        wp(ct, "cache", "flush", "--quiet", check=False)
        cache_clear(ct)
        out({"status": "ok", "home": wp(ct, "option", "get", "home").stdout.strip()})
    elif cmd == "sample":
        out({"status": "ok", "sample": sample()})
    elif cmd == "stats":
        out(dict(status="ok", **stats(sys.argv[2] if len(sys.argv) > 2 else "1h")))
    elif cmd == "logs":
        out({"status": "ok", "source": sys.argv[2], "text": logs(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else 200)})
    elif cmd == "prune":
        out({"status": "ok", "pruned": prune()})
    elif cmd == "container":
        out(dict(status="ok", **container(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else "1h")))
    elif cmd == "restart":
        out(dict(status="ok", **restart(sys.argv[2], sys.argv[3])))
    else:
        out({"status": "error", "error": "usage: cc-backups create|list|restore|delete|daily|prune"}, 2)
except IndexError:
    out({"status": "error", "error": "missing arguments"}, 2)
except Exception as e:
    out({"status": "error", "error": str(e)[:400]}, 1)
