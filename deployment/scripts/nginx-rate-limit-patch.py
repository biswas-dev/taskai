#!/usr/bin/env python3
"""Bring an existing nginx vhost in line with the current rate-limiting policy.

Idempotent and conservative: every edit is guarded, and anything whose anchor is
missing is skipped rather than guessed at. Prints the edits it made.
"""
import re
import sys

SNIPPET_INCLUDE = "include /etc/nginx/snippets/cloudflare-realip.conf;"

path = sys.argv[1]
with open(path) as f:
    original = f.read()
content = original
edits = []

# 1. Raise the zone rates. The old values were below what one idle browser tab
#    costs, so ordinary use tripped the limiter.
def bump_rate(text, zone_suffix, old, new):
    pattern = re.compile(
        r"(limit_req_zone\s+\$binary_remote_addr\s+zone=\S*%s:\S+\s+rate=)%s;" % (zone_suffix, old))
    return pattern.subn(r"\g<1>%s;" % new, text)

for suffix, old, new in (("_api", "100r/m", "600r/m"),
                         ("_auth", "20r/m", "30r/m"),
                         ("_mcp", "30r/m", "300r/m")):
    content, n = bump_rate(content, suffix, old, new)
    if n:
        edits.append("rate %s: %s -> %s" % (suffix, old, new))

# 2. Raise the burst allowances to cover a page load (~20 API calls) with headroom.
for suffix, old, new in (("_api", "20", "100"),
                         ("_auth", "3", "10"),
                         ("_mcp", "10", "60")):
    pattern = re.compile(r"(limit_req\s+zone=\S*%s\s+burst=)%s(\s+nodelay;)" % (suffix, old))
    content, n = pattern.subn(r"\g<1>%s\g<2>" % new, content)
    if n:
        edits.append("burst %s: %s -> %s" % (suffix, old, new))

# 3. Per-server directives: real client IPs, and 429 rather than nginx's default
#    503 for throttling. Inserted into each TLS server block.
def split_server_blocks(text):
    """Yield (start, end) spans of top-level `server { ... }` blocks."""
    for m in re.finditer(r"^server\s*\{", text, re.M):
        depth, i = 0, m.start()
        while i < len(text):
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
                if depth == 0:
                    yield (m.start(), i + 1)
                    break
            i += 1

out, last = [], 0
added_realip = added_status = 0
for start, end in split_server_blocks(content):
    block = content[start:end]
    if "listen 443" not in block:
        continue
    inject = []
    if SNIPPET_INCLUDE not in block:
        inject.append("    # Real client IPs (Cloudflare): without this every visitor behind the\n"
                      "    # same edge node shares one rate-limit bucket.\n"
                      "    " + SNIPPET_INCLUDE)
        added_realip += 1
    if "limit_req_status" not in block and "limit_req" in block:
        inject.append("    # Throttled requests answer 429, not 503: a 503 trips the maintenance\n"
                      "    # page (which auto-refreshes into another burst) and proxy_next_upstream.\n"
                      "    limit_req_status 429;")
        added_status += 1
    if not inject:
        continue
    brace = block.index("{") + 1
    new_block = block[:brace] + "\n" + "\n\n".join(inject) + "\n" + block[brace:]
    out.append(content[last:start] + new_block)
    last = end
out.append(content[last:])
content = "".join(out)
if added_realip:
    edits.append("cloudflare real-ip include x%d" % added_realip)
if added_status:
    edits.append("limit_req_status 429 x%d" % added_status)

# 4. index.html names content-hashed assets that a deploy deletes, so a cached
#    copy points the browser at files that no longer exist.
#
#    Target the frontend `location /` specifically: the port-80 server block has
#    a `location /` too, but it only issues a 301 redirect.
if "no-cache, must-revalidate" not in content:
    target = None
    for start, end in split_server_blocks(content):
        block = content[start:end]
        if "listen 443" not in block:
            continue
        for m in re.finditer(r"^(\s*)location / \{\s*\n", block, re.M):
            # The frontend location proxies; the redirect location does not.
            tail = block[m.end():m.end() + 600]
            if "proxy_pass" in tail.split("\n    }")[0]:
                target = (start + m.end(), m.group(1))
                break
        if target:
            break
    if target:
        pos, indent = target
        insert = (indent + "    # index.html names hash-versioned assets that a deploy removes, so a\n"
                  + indent + "    # cached copy leaves the tab on a build whose files are gone.\n"
                  + indent + '    add_header Cache-Control "no-cache, must-revalidate" always;\n')
        content = content[:pos] + insert + content[pos:]
        edits.append("index.html no-cache")
    else:
        edits.append("index.html no-cache SKIPPED (frontend location not found)")

if content != original:
    with open(path, "w") as f:
        f.write(content)

print("; ".join(edits) if edits else "already current")
sys.exit(0 if edits else 10)
