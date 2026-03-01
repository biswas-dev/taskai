#!/bin/bash
# Ensure the /draw/ nginx route exists (with ^~ to prevent regex override)
# for a given domain. Idempotent: skips if correctly configured.
#
# Usage: sudo ./ensure-draw-route.sh <domain>
# Example: sudo ./ensure-draw-route.sh staging.taskai.cc
set -e

DOMAIN="${1:?Usage: ensure-draw-route.sh <domain>}"
CONF="/etc/nginx/sites-available/$DOMAIN"

if [ ! -f "$CONF" ]; then
    echo "No nginx config found at $CONF, skipping"
    exit 0
fi

# Check if /draw/ route exists with ^~ modifier
if grep -q 'location \^~ /draw/' "$CONF"; then
    echo "/draw/ route already configured for $DOMAIN"
    exit 0
fi

# Upgrade: if /draw/ exists without ^~, add the modifier
if grep -q 'location /draw/' "$CONF"; then
    echo "Upgrading /draw/ route to use ^~ for $DOMAIN..."
    sed -i 's|location /draw/|location ^~ /draw/|' "$CONF"
    nginx -t
    systemctl reload nginx
    echo "/draw/ route upgraded to ^~ and nginx reloaded for $DOMAIN"
    exit 0
fi

# Extract the API proxy_pass from the existing /api/ location block
API_BACKEND=$(grep -m1 -oP 'proxy_pass http://127\.0\.0\.1:\d+' "$CONF" || true)
if [ -z "$API_BACKEND" ]; then
    echo "Cannot detect API backend from $CONF, skipping"
    exit 0
fi

echo "Adding /draw/ route to $DOMAIN ($API_BACKEND)..."

# Build the draw location block in a temp file
DRAW_TMP=$(mktemp)
cat > "$DRAW_TMP" << EOF

    # go-draw canvas editor (served by API backend)
    location ^~ /draw/ {
        ${API_BACKEND};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
EOF

# Insert before the frontend catch-all "location / {" in the HTTPS block.
# Strategy: find "# Frontend" comment or the last "location / {" and insert before it.
python3 -c "
import sys

with open('${CONF}') as f:
    content = f.read()

with open('${DRAW_TMP}') as f:
    draw_block = f.read()

# Try inserting before '# Frontend' comment
marker = '    # Frontend'
if marker in content:
    content = content.replace(marker, draw_block + '\n' + marker, 1)
else:
    # Fallback: insert before the last 'location / {' (frontend catch-all)
    idx = content.rfind('    location / {')
    if idx > 0:
        content = content[:idx] + draw_block + '\n' + content[idx:]
    else:
        print('Could not find insertion point', file=sys.stderr)
        sys.exit(1)

with open('${CONF}', 'w') as f:
    f.write(content)
"

rm -f "$DRAW_TMP"

# Validate and reload
nginx -t
systemctl reload nginx
echo "/draw/ route added and nginx reloaded for $DOMAIN"
