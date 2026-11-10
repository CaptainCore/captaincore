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

KEEP = {"daily": 14, "pre-restore": 7, "pre-deploy": 7}   # days
MAX_MANUAL = 10
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
        manual = [s for s in snaps if s["type"] == "manual"]
        for s in snaps:
            age = (now - datetime.datetime.fromisoformat(s["created"])).days
            if (s["type"] in KEEP and age >= KEEP[s["type"]]) or (s["type"] == "manual" and s in manual[MAX_MANUAL:]):
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
    elif cmd == "prune":
        out({"status": "ok", "pruned": prune()})
    else:
        out({"status": "error", "error": "usage: cc-backups create|list|restore|delete|daily|prune"}, 2)
except IndexError:
    out({"status": "error", "error": "missing arguments"}, 2)
except Exception as e:
    out({"status": "error", "error": str(e)[:400]}, 1)
