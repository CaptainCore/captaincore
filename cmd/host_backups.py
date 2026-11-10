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
    if what in ("both", "database"):
        run("incus", "exec", ct, "--", "systemctl", "stop", "mariadb")
        try:
            src = "%s/.zfs/snapshot/%s/" % (vols["database"]["mount"], snap)
            run("rsync", "-aHAX", "--numeric-ids", "--delete", src, vols["database"]["mount"] + "/")
        finally:
            run("incus", "exec", ct, "--", "systemctl", "start", "mariadb")
        done.append("database")
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


def create_site(ct, image, ip, port, disk_gib, memory_mib):
    """A new site container from an image: its own files and database volumes,
    a fixed IP and SSH port, the site defaults (own host user ID range, no
    traffic to other containers, a process limit), MariaDB with a fresh
    database and user. Prints the database password once."""
    import secrets
    if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct) or not re.match(r"^[a-z0-9][a-z0-9.-]{0,63}$", image) \
            or not re.match(r"^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip) or not (1024 < int(port) < 65535) \
            or not (1 <= int(disk_gib) <= 200) or not (512 <= int(memory_mib) <= 8192):
        raise RuntimeError("invalid create arguments")
    if run("incus", "info", ct, check=False).returncode == 0:
        raise RuntimeError("container %s already exists" % ct)
    pool = "default"
    src = run("incus", "storage", "get", pool, "source").stdout.strip()
    run("incus", "storage", "volume", "create", pool, ct + "-site", "size=%dGiB" % int(disk_gib))
    run("incus", "storage", "volume", "create", pool, ct + "-db", "size=5GiB")
    run("zfs", "set", "recordsize=16K", "%s/custom/default_%s-db" % (src, ct), check=False)
    run("incus", "init", image, ct)
    for kv in ("limits.memory=%dMiB" % int(memory_mib), "limits.processes=1000", "security.idmap.isolated=true"):
        run("incus", "config", "set", ct, kv)
    run("incus", "config", "device", "override", ct, "eth0", "ipv4.address=" + ip, "security.port_isolation=true")
    run("incus", "storage", "volume", "attach", pool, ct + "-site", ct, "/srv/site")
    run("incus", "storage", "volume", "attach", pool, ct + "-db", ct, "/var/lib/mysql")
    run("incus", "config", "device", "add", ct, "ssh", "proxy", "listen=tcp:0.0.0.0:%d" % int(port), "connect=tcp:127.0.0.1:22")
    run("ufw", "allow", "%d/tcp" % int(port), check=False)
    run("incus", "start", ct)
    for _ in range(30):
        if run("incus", "exec", ct, "--", "true", check=False).returncode == 0:
            break
        time.sleep(1)
    dbpass = secrets.token_hex(16)
    setup = """set -e
install -d -o site -g site -m 0755 /srv/site /srv/site/public /srv/site/apps
install -d -o site -g site -m 0750 /srv/site/logs
install -d -o site -g site -m 0700 /srv/site/.ssh
chown mysql:mysql /var/lib/mysql
[ -d /var/lib/mysql/mysql ] || mariadb-install-db --user=mysql --datadir=/var/lib/mysql --skip-test-db >/dev/null 2>&1
systemctl restart mariadb
mariadb -e "CREATE DATABASE IF NOT EXISTS wordpress CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE USER IF NOT EXISTS 'wordpress'@'localhost' IDENTIFIED BY '$DBPASS'; GRANT ALL ON wordpress.* TO 'wordpress'@'localhost';"
[ -n "$(ls -A /srv/site/public)" ] || { printf '%s\n' '<!doctype html><meta charset="utf-8"><meta name="robots" content="noindex"><title>Coming soon</title><style>body{font:16px/1.5 system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;color:#444}</style><p>This site is being set up.</p>' > /srv/site/public/index.html; chown site:site /srv/site/public/index.html; }
systemctl restart frankenphp || true
"""
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--", "bash", "-c", setup)
    return {"container": ct, "ip": ip, "ssh_port": int(port), "db_name": "wordpress", "db_user": "wordpress", "db_password": dbpass,
            "idmap": run("incus", "config", "get", ct, "volatile.idmap.base", check=False).stdout.strip()}


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
"""
    run("incus", "exec", ct, "--env", "DBPASS=" + dbpass, "--env", "HOST=" + url_host, "--env", "TITLE=" + title,
        "--env", "AUSER=" + admin_user, "--env", "AEMAIL=" + admin_email, "--env", "APASS=" + adminpass,
        "--", "su", "-s", "/bin/bash", "site", "-c", script)
    return {"url": "https://" + url_host, "admin_user": admin_user}


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
        done.append("files")
    if what in ("both", "database"):
        run("incus", "exec", dst, "--", "systemctl", "stop", "mariadb")
        try:
            run("rsync", "-aHAX", "--numeric-ids", "--delete",
                "%s/.zfs/snapshot/%s/" % (sv["database"]["mount"], point), dv["database"]["mount"] + "/")
        finally:
            run("incus", "exec", dst, "--", "systemctl", "start", "mariadb")
        # The copy carries the source's URLs and search engine setting; give
        # the destination back its own.
        if src_host != dst_host:
            wp(dst, "search-replace", "https://" + src_host, "https://" + dst_host, "--all-tables", "--precise", "--quiet")
            wp(dst, "search-replace", src_host, dst_host, "--all-tables", "--precise", "--quiet")
        if keep_public in ("0", "1"):
            wp(dst, "option", "update", "blog_public", keep_public, "--quiet", check=False)
        wp(dst, "cache", "flush", "--quiet", check=False)
        done.append("database")
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
    return rec


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
    hist = _ct_history(ct, seconds)
    info["cpu"] = hist[-1]["cpu"] if hist else None

    res = {"container": info, "history": hist, "range": range_, "versions": {}, "units": [], "listening": [],
           "db": {}, "caddyfile": "", "edge_caddyfile": "", "traffic": {}}
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
        out({"status": "ok", "created": made, "pruned": prune()})
    elif cmd == "clone":
        out(dict(status="ok", **clone(sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5]))))
    elif cmd == "sync":
        out(dict(status="ok", **sync(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6])))
    elif cmd == "site-create":
        out(dict(status="ok", **create_site(sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5]), int(sys.argv[6]), int(sys.argv[7]))))
    elif cmd == "authorize":
        ct, key = sys.argv[2], sys.argv[3]
        if not re.match(r"^[a-z0-9][a-z0-9-]{0,62}$", ct) or not re.match(r"^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)) [A-Za-z0-9+/]+={0,3}$", key):
            raise RuntimeError("invalid authorize arguments")
        run("incus", "exec", ct, "--env", "KEY=" + key + " captaincore-management", "--", "sh", "-c",
            "install -d -o site -g site -m 0700 /srv/site/.ssh && touch /srv/site/.ssh/authorized_keys && "
            "grep -qF \"$KEY\" /srv/site/.ssh/authorized_keys || printf '%s\\n' \"$KEY\" >> /srv/site/.ssh/authorized_keys; "
            "chown site:site /srv/site/.ssh/authorized_keys && chmod 600 /srv/site/.ssh/authorized_keys")
        out({"status": "ok"})
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
