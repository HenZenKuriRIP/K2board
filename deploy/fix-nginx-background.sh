#!/usr/bin/env bash
# =============================================================================
# Hotfix for existing single-server installs:
#   /background.png was caught by "location / { return 302 /; }" → login SPA.
# This inserts a static-file location before the catch-all.
#
# Usage (on server as root):
#   bash fix-nginx-background.sh
#   # or:
#   curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/fix-nginx-background.sh | bash
# =============================================================================
set -euo pipefail

CONF="${1:-}"
if [[ -z "$CONF" ]]; then
  for c in /etc/nginx/sites-enabled/k2board /etc/nginx/sites-available/k2board \
           /etc/nginx/conf.d/k2board.conf; do
    if [[ -f "$c" ]]; then CONF="$c"; break; fi
  done
fi
[[ -n "${CONF:-}" && -f "$CONF" ]] || {
  echo "Nginx site config not found. Pass path: $0 /etc/nginx/sites-available/k2board"
  exit 1
}

USER_ROOT="${K2BOARD_USER_DIR:-/var/www/k2board-user}"
if [[ ! -f "$USER_ROOT/background.png" && ! -f "$USER_ROOT/background.jpg" ]]; then
  echo "WARN: no background.png/jpg under $USER_ROOT — deploy user dist first"
fi

if grep -qE 'background\.\(png\|jpe\?g' "$CONF" 2>/dev/null || \
   grep -q 'background\.\(png|jpe?g' "$CONF" 2>/dev/null; then
  echo "Config already has background static location: $CONF"
else
  SNIP=$(cat <<'EOF'

    # [k2board] login background / config.js — before catch-all 302
    location ~* ^/(background\.(png|jpe?g|webp)|config\.js|config\.example\.js|robots\.txt)$ {
        root __USER_ROOT__;
        try_files $uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }
EOF
)
  SNIP="${SNIP//__USER_ROOT__/$USER_ROOT}"
  cp -a "$CONF" "${CONF}.bak.$(date +%Y%m%d%H%M%S)"
  # Insert before the first "location / {" that returns 302, or before last location /
  if grep -q 'location / { return 302' "$CONF"; then
    # awk insert before that line
    awk -v snip="$SNIP" '
      /location \/ \{ return 302/ && !done {
        print snip
        done=1
      }
      { print }
    ' "$CONF" > "${CONF}.new"
    mv "${CONF}.new" "$CONF"
  elif grep -q 'location / {' "$CONF"; then
    awk -v snip="$SNIP" '
      /^[[:space:]]*location \/ \{/ && !done {
        print snip
        done=1
      }
      { print }
    ' "$CONF" > "${CONF}.new"
    mv "${CONF}.new" "$CONF"
  else
    echo "Could not find catch-all location / — append snippet manually"
    printf '%s\n' "$SNIP"
    exit 1
  fi
  echo "Patched $CONF"
fi

nginx -t
systemctl reload nginx
echo "OK — test: curl -sI https://$(hostname -f)/background.png | head -5"
echo "     or: curl -sI http://127.0.0.1/background.png -H 'Host: www.example.com' | head -8"
