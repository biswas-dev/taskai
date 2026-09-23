#!/bin/bash
# Converge an existing nginx vhost onto the current deployment policy:
# - Custom error page for 502/503/504 (friendly "Updating..." page)
# - proxy_next_upstream directives for retry on upstream failure
# - Workable rate limits, keyed on the real client IP, answering 429 not 503
#
# Idempotent: skips whatever is already configured.
# Usage: sudo ./ensure-zero-downtime.sh <domain>
# Example: sudo ./ensure-zero-downtime.sh staging.taskai.cc
set -e

DOMAIN="${1:?Usage: ensure-zero-downtime.sh <domain>}"
CONF="/etc/nginx/sites-available/$DOMAIN"

if [ ! -f "$CONF" ]; then
    echo "No nginx config found at $CONF, skipping"
    exit 0
fi

CHANGED=0

# Snapshot the config so a failed `nginx -t` below can be rolled back.
BACKUP="${CONF}.bak.$(date +%Y%m%d%H%M%S)"
cp "$CONF" "$BACKUP"

# 1. Add error_page and /50x.html location if not present
if grep -q 'error_page 502 503 504 /50x.html' "$CONF"; then
    echo "error_page already configured for $DOMAIN"
else
    echo "Adding error_page and /50x.html to $DOMAIN..."
    python3 -c "
import sys

with open('${CONF}') as f:
    content = f.read()

# Build the error page block
error_block = '''
    # Friendly maintenance page during deployments
    error_page 502 503 504 /50x.html;
    location = /50x.html {
        internal;
        default_type text/html;
        return 503 '<!DOCTYPE html><html><head><title>Updating...</title><meta http-equiv=\"refresh\" content=\"5\"></head><body style=\"font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f8fafc\"><div style=\"text-align:center\"><h1 style=\"color:#1e293b\">Updating TaskAI</h1><p style=\"color:#64748b\">Please wait a moment, this page will refresh automatically.</p></div></body></html>';
    }
'''

# Insert before the first location block (after security headers/logging)
marker = '    # Auth endpoints'
if marker not in content:
    marker = '    # API endpoints'
if marker not in content:
    marker = '    location'

idx = content.find(marker)
if idx > 0:
    content = content[:idx] + error_block.strip() + '\n\n    ' + content[idx:]
else:
    print('Could not find insertion point for error_page', file=sys.stderr)
    sys.exit(1)

with open('${CONF}', 'w') as f:
    f.write(content)
"
    CHANGED=1
fi

# 2. Add proxy_next_upstream to location blocks that proxy_pass but lack it
if grep -q 'proxy_next_upstream' "$CONF"; then
    echo "proxy_next_upstream already configured for $DOMAIN"
else
    echo "Adding proxy_next_upstream to proxy locations in $DOMAIN..."
    python3 -c "
import re

with open('${CONF}') as f:
    content = f.read()

# Directive to add after the last proxy_ line in each location block
next_upstream = '''
        proxy_next_upstream error timeout http_502 http_503;
        proxy_next_upstream_timeout 5s;
        proxy_next_upstream_tries 2;'''

# Find all location blocks that have proxy_pass but not proxy_next_upstream
# Strategy: find each proxy_read_timeout or last proxy_ directive and append after it
lines = content.split('\n')
result = []
in_location = False
has_proxy_pass = False
last_proxy_idx = -1
inserted_indices = set()

# First pass: identify insertion points
for i, line in enumerate(lines):
    stripped = line.strip()
    if stripped.startswith('location') and '{' in stripped:
        in_location = True
        has_proxy_pass = False
        last_proxy_idx = -1
    elif in_location and stripped.startswith('proxy_'):
        has_proxy_pass = True
        last_proxy_idx = i
    elif in_location and stripped == '}':
        if has_proxy_pass and last_proxy_idx >= 0:
            inserted_indices.add(last_proxy_idx)
        in_location = False

# Second pass: build result with insertions
for i, line in enumerate(lines):
    result.append(line)
    if i in inserted_indices:
        result.append('')
        for directive_line in next_upstream.strip().split('\n'):
            result.append(directive_line)

with open('${CONF}', 'w') as f:
    f.write('\n'.join(result))
"
    CHANGED=1
fi

# 3. Rate limiting: real client IPs, limits that fit the app, 429 not 503.
#
# Background: nginx sees every request arriving from a Cloudflare edge address,
# so without the real-ip snippet the per-IP buckets were shared by everyone
# behind that edge. Combined with rate=100r/m (below what a single open tab
# costs) that throttled ordinary traffic, and nginx's default 503 made the
# result look like an outage — it tripped the maintenance page, which
# auto-refreshed into another burst, and matched proxy_next_upstream http_503.
SNIPPET_DIR="/etc/nginx/snippets"
SNIPPET="$SNIPPET_DIR/cloudflare-realip.conf"

mkdir -p "$SNIPPET_DIR"
# Rewritten every run so the Cloudflare ranges stay in step with the repo.
cat > "$SNIPPET" <<'SNIPEOF'
# Restore the originating client IP for traffic proxied by Cloudflare.
#
# Every request reaches nginx from a Cloudflare edge node. Without this, all
# per-IP logic — rate limiting above all — keys on the edge address, so every
# visitor arriving through the same edge shares one bucket and normal traffic
# looks like a single abusive client.
#
# Included from inside each server block (see nginx-ssl.conf.j2), never at http
# level: these directives may only be declared once per context.
#
# Source: https://www.cloudflare.com/ips/

# IPv4
set_real_ip_from 173.245.48.0/20;
set_real_ip_from 103.21.244.0/22;
set_real_ip_from 103.22.200.0/22;
set_real_ip_from 103.31.4.0/22;
set_real_ip_from 141.101.64.0/18;
set_real_ip_from 108.162.192.0/18;
set_real_ip_from 190.93.240.0/20;
set_real_ip_from 188.114.96.0/20;
set_real_ip_from 197.234.240.0/22;
set_real_ip_from 198.41.128.0/17;
set_real_ip_from 162.158.0.0/15;
set_real_ip_from 104.16.0.0/13;
set_real_ip_from 104.24.0.0/14;
set_real_ip_from 172.64.0.0/13;
set_real_ip_from 131.0.72.0/22;

# IPv6
set_real_ip_from 2400:cb00::/32;
set_real_ip_from 2606:4700::/32;
set_real_ip_from 2803:f800::/32;
set_real_ip_from 2405:b500::/32;
set_real_ip_from 2405:8100::/32;
set_real_ip_from 2a06:98c0::/29;
set_real_ip_from 2c0f:f248::/32;

real_ip_header CF-Connecting-IP;
real_ip_recursive on;
SNIPEOF

# `set -e` is suspended inside an if-condition, so capture the status explicitly.
RATE_STATUS=0
RATE_OUT=$(python3 "$(dirname "$0")/nginx-rate-limit-patch.py" "$CONF") || RATE_STATUS=$?

case "$RATE_STATUS" in
    0)
        echo "Rate limiting updated for $DOMAIN: $RATE_OUT"
        CHANGED=1
        ;;
    10)
        # Nothing to change.
        echo "Rate limiting already current for $DOMAIN"
        ;;
    *)
        echo "Rate-limit patch failed for $DOMAIN (exit $RATE_STATUS): $RATE_OUT" >&2
        cp "$BACKUP" "$CONF"
        exit 1
        ;;
esac

if [ "$CHANGED" -eq 1 ]; then
    # Validate before reloading, and put the previous config back if it fails.
    # nginx keeps serving the old config until a successful reload, so restoring
    # the file here is what stops a later restart picking up a broken one.
    if nginx -t; then
        systemctl reload nginx
        echo "nginx configuration updated and reloaded for $DOMAIN"
        # Keep only the most recent few backups.
        ls -1t "${CONF}".bak.* 2>/dev/null | tail -n +6 | xargs -r rm -f
    else
        echo "nginx -t failed for $DOMAIN, restoring previous config" >&2
        cp "$BACKUP" "$CONF"
        if nginx -t; then
            echo "Previous config restored and valid." >&2
        else
            echo "WARNING: restored config still fails nginx -t." >&2
        fi
        exit 1
    fi
else
    echo "No changes needed for $DOMAIN"
    rm -f "$BACKUP"
fi
