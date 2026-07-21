#!/usr/bin/env bash
# =============================================================================
# K2Board — standalone user portal installer (shadow domain / split deploy)
#
# Installs ONLY the static user SPA (web_user). No Go panel, no DB, no Redis.
# API / subscribe / payment / admin stay on the panel host (www).
#
# Usage:
#   Interactive:
#     bash deploy/install-user-portal.sh
#     bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install-user-portal.sh)
#
#   Non-interactive:
#     bash deploy/install-user-portal.sh user.example.com https://www.panel.com
#
#   Options:
#     --version=v1.4.27     release tag (default: latest)
#     --dir=/var/www/...    web root (default: /var/www/k2board-user)
#     --skip-nginx          only download/extract dist + write config.js
#     --skip-tls            HTTP only (not recommended)
#     --cloudflare          install CF Real IP snippets (optional)
#     --uninstall           remove site + web root (keeps ssl by default)
#     --purge               with --uninstall: also remove ssl files for this site
#
# After install (required on panel):
#   Admin → 系统设置 → 允许的用户端域名 → add https://user.example.com
#   site_url / subscribe_url stay on www (do not point subscribe at this host)
# =============================================================================

set -euo pipefail

REPO="${K2BOARD_REPO:-HenZenKuriRIP/K2board}"
RELEASE_BASE="https://github.com/${REPO}/releases"
INSTALL_NAME="k2board-user"
WEB_DIR="${K2BOARD_USER_DIR:-/var/www/k2board-user}"
NGINX_AVAIL="/etc/nginx/sites-available/${INSTALL_NAME}"
NGINX_ENAB="/etc/nginx/sites-enabled/${INSTALL_NAME}"
SSL_DIR="/etc/nginx/ssl"
META_DIR="/etc/k2board"
META_FILE="${META_DIR}/user-portal.meta"
CF_REALIP="/etc/nginx/conf.d/cloudflare-realip.conf"

DOMAIN=""
API_BASE=""
VERSION="latest"
SKIP_NGINX=0
SKIP_TLS=0
CF_REALIP_ON=0
DO_UNINSTALL=0
PURGE=0

if [[ -t 1 ]]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
  BOLD=$'\033[1m'; DIM=$'\033[2m'; NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BOLD=""; DIM=""; NC=""
fi

info()  { printf "%s%s%s\n" "${DIM}" "$*" "${NC}"; }
ok()    { printf "%s✓%s %s\n" "${GREEN}" "${NC}" "$*"; }
warn()  { printf "%s!%s %s\n" "${YELLOW}" "${NC}" "$*"; }
err()   { printf "%s✗%s %s\n" "${RED}" "${NC}" "$*" >&2; }
die()   { err "$*"; exit 1; }

need_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    die "Please run as root (sudo)."
  fi
}

read_tty() {
  local __var="$1" __prompt="${2:-}" __def="${3:-}" __input=""
  if [[ -n "$__prompt" ]]; then
    if [[ -n "$__def" ]]; then
      printf "%s [%s]: " "$__prompt" "$__def" >/dev/tty
    else
      printf "%s: " "$__prompt" >/dev/tty
    fi
  fi
  IFS= read -r __input </dev/tty || true
  [[ -z "$__input" && -n "$__def" ]] && __input="$__def"
  printf -v "$__var" '%s' "$__input"
}

usage() {
  sed -n '2,40p' "$0" | sed 's/^# \?//'
  exit 0
}

parse_args() {
  local a
  for a in "$@"; do
    case "$a" in
      -h|--help) usage ;;
      --uninstall) DO_UNINSTALL=1 ;;
      --purge) PURGE=1 ;;
      --skip-nginx) SKIP_NGINX=1 ;;
      --skip-tls) SKIP_TLS=1 ;;
      --cloudflare) CF_REALIP_ON=1 ;;
      --version=*) VERSION="${a#*=}" ;;
      --dir=*) WEB_DIR="${a#*=}" ;;
      --api=*) API_BASE="${a#*=}" ;;
      --domain=*) DOMAIN="${a#*=}" ;;
      http://*|https://*)
        # bare API base as second positional is handled below
        if [[ -z "$API_BASE" && -n "$DOMAIN" ]]; then
          API_BASE="$a"
        elif [[ -z "$DOMAIN" ]]; then
          DOMAIN="${a#*://}"
          DOMAIN="${DOMAIN%%/*}"
        else
          API_BASE="$a"
        fi
        ;;
      -*)
        die "Unknown option: $a"
        ;;
      *)
        if [[ -z "$DOMAIN" ]]; then
          DOMAIN="$a"
        elif [[ -z "$API_BASE" ]]; then
          API_BASE="$a"
        else
          die "Unexpected argument: $a"
        fi
        ;;
    esac
  done
}

normalize_api_base() {
  local b="$1"
  b="${b%%/}"
  if [[ -z "$b" ]]; then
    echo ""
    return
  fi
  if [[ "$b" != http://* && "$b" != https://* ]]; then
    b="https://${b}"
  fi
  # strip path
  b="$(printf '%s' "$b" | sed -E 's#(https?://[^/]+).*#\1#')"
  echo "$b"
}

uninstall() {
  need_root
  info "Uninstalling user portal site…"
  rm -f "$NGINX_ENAB" "$NGINX_AVAIL"
  if [[ -d "$WEB_DIR" ]]; then
    rm -rf "$WEB_DIR"
    ok "Removed $WEB_DIR"
  fi
  if [[ "$PURGE" -eq 1 ]]; then
    rm -f "${SSL_DIR}/user-portal-fullchain.pem" "${SSL_DIR}/user-portal-privkey.pem" 2>/dev/null || true
    # only remove generic names if meta says they belong to us
    if [[ -f "$META_FILE" ]]; then
      # shellcheck disable=SC1090
      source "$META_FILE" 2>/dev/null || true
      if [[ -n "${SSL_CERT:-}" && -f "$SSL_CERT" ]]; then
        warn "Leaving shared SSL files intact (use --purge carefully): $SSL_CERT"
      fi
    fi
    rm -f "$META_FILE"
    ok "Purged meta"
  fi
  if command -v nginx >/dev/null 2>&1; then
    nginx -t 2>/dev/null && systemctl reload nginx || true
  fi
  ok "User portal uninstalled"
  exit 0
}

ensure_packages() {
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates tar nginx >/dev/null
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y curl ca-certificates tar nginx >/dev/null
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl ca-certificates tar nginx >/dev/null
  else
    warn "Unknown package manager — ensure curl, tar, nginx are installed"
  fi
  ok "Packages ready"
}

download_dist() {
  local url tmp
  mkdir -p "$WEB_DIR"
  tmp="$(mktemp /tmp/k2board-user-XXXXXX.tar.gz)"

  if [[ "$VERSION" == "latest" ]]; then
    url="${RELEASE_BASE}/latest/download/k2board-user-dist.tar.gz"
  else
    url="${RELEASE_BASE}/download/${VERSION}/k2board-user-dist.tar.gz"
  fi

  info "Downloading $url"
  if ! curl -fSL --retry 3 --connect-timeout 20 -o "$tmp" "$url"; then
    rm -f "$tmp"
    die "Failed to download user portal dist. Check release assets / version."
  fi
  [[ -s "$tmp" ]] || die "Downloaded archive is empty"

  find "$WEB_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
  tar -xzf "$tmp" -C "$WEB_DIR/"
  rm -f "$tmp"

  [[ -f "$WEB_DIR/index.html" ]] || die "index.html missing after extract — bad archive layout"
  ok "Extracted to $WEB_DIR"
}

write_config_js() {
  local api
  api="$(normalize_api_base "$API_BASE")"
  [[ -n "$api" ]] || die "API base (panel www URL) is required"

  cat > "${WEB_DIR}/config.js" <<EOF
// Generated by install-user-portal.sh — do not put secrets here.
// Panel API host (www). Subscribe links still use panel subscribe_url.
window.__K2_API_BASE__ = '${api}';
EOF
  chmod 644 "${WEB_DIR}/config.js"
  ok "Wrote config.js → ${api}"
}

fetch_nginx_template() {
  local tpl="${1:-}"
  if [[ -n "$tpl" && -f "$tpl" ]]; then
    cat "$tpl"
    return
  fi
  # Prefer local repo file when running from git checkout
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
  if [[ -f "${here}/nginx-user-portal.conf" ]]; then
    cat "${here}/nginx-user-portal.conf"
    return
  fi
  curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/deploy/nginx-user-portal.conf"
}

install_nginx_site() {
  local cert key conf
  mkdir -p "$SSL_DIR" /var/www/html

  cert="${SSL_DIR}/fullchain.pem"
  key="${SSL_DIR}/privkey.pem"

  if [[ "$SKIP_TLS" -eq 1 ]]; then
    warn "TLS skipped — generating temporary self-signed cert (replace for production)"
    if [[ ! -f "$cert" || ! -f "$key" ]]; then
      openssl req -x509 -nodes -newkey rsa:2048 -days 30 \
        -keyout "$key" -out "$cert" \
        -subj "/CN=${DOMAIN}" 2>/dev/null || die "openssl failed"
    fi
  else
    if [[ ! -f "$cert" || ! -f "$key" ]]; then
      warn "No certs at $cert — generating self-signed placeholder."
      warn "For Cloudflare: upload Origin cert to $SSL_DIR or use Full (strict) with CF Origin CA."
      openssl req -x509 -nodes -newkey rsa:2048 -days 30 \
        -keyout "$key" -out "$cert" \
        -subj "/CN=${DOMAIN}" 2>/dev/null || die "openssl failed"
    else
      ok "Using existing TLS certs in $SSL_DIR"
    fi
  fi

  conf="$(fetch_nginx_template | sed \
    -e "s|__SERVER_NAME__|${DOMAIN}|g" \
    -e "s|__ROOT__|${WEB_DIR}|g" \
    -e "s|__SSL_CERT__|${cert}|g" \
    -e "s|__SSL_KEY__|${key}|g")"

  printf '%s\n' "$conf" > "$NGINX_AVAIL"
  ln -sfn "$NGINX_AVAIL" "$NGINX_ENAB"

  # Avoid default site stealing the name
  rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true

  if [[ "$CF_REALIP_ON" -eq 1 ]]; then
    install_cf_realip || warn "CF Real IP install skipped/failed"
  fi

  nginx -t || die "nginx -t failed"
  systemctl enable nginx >/dev/null 2>&1 || true
  systemctl reload nginx || systemctl restart nginx
  ok "Nginx site enabled: $NGINX_ENAB"
}

install_cf_realip() {
  # Minimal Cloudflare Real IP — full list may be refreshed by panel installer
  if [[ -f "$CF_REALIP" ]]; then
    ok "CF Real IP conf already present"
    return 0
  fi
  cat > "$CF_REALIP" <<'EOF'
# Cloudflare Real IP (user portal) — refresh periodically from https://www.cloudflare.com/ips/
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
set_real_ip_from 2400:cb00::/32;
set_real_ip_from 2606:4700::/32;
set_real_ip_from 2803:f800::/32;
set_real_ip_from 2405:b500::/32;
set_real_ip_from 2405:8100::/32;
set_real_ip_from 2a06:98c0::/29;
set_real_ip_from 2c0f:f248::/32;
real_ip_header CF-Connecting-IP;
EOF
  ok "Wrote $CF_REALIP"
}

write_meta() {
  mkdir -p "$META_DIR"
  cat > "$META_FILE" <<EOF
DOMAIN=${DOMAIN}
API_BASE=$(normalize_api_base "$API_BASE")
WEB_DIR=${WEB_DIR}
VERSION=${VERSION}
INSTALLED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
  chmod 644 "$META_FILE"
}

harden_perms() {
  # Static files: world-readable, not world-writable
  chown -R root:www-data "$WEB_DIR" 2>/dev/null || chown -R root:nginx "$WEB_DIR" 2>/dev/null || true
  find "$WEB_DIR" -type d -exec chmod 755 {} +
  find "$WEB_DIR" -type f -exec chmod 644 {} +
  ok "Permissions hardened (root-owned, web-readable)"
}

print_next_steps() {
  local origin api
  origin="https://${DOMAIN}"
  api="$(normalize_api_base "$API_BASE")"
  cat <<EOF

${BOLD}=== User portal installed ===${NC}
  Domain:     ${origin}
  Web root:   ${WEB_DIR}
  API base:   ${api}
  Nginx:      ${NGINX_ENAB}

${BOLD}Required on the panel (www) server:${NC}
  1. Admin → 系统设置 → 站点 URL / 订阅域名 = ${api}  (钉死，勿改订阅域)
  2. Admin → 系统设置 → 允许的用户端域名 增加一行:
       ${origin}
  3. Save settings

${BOLD}Cloudflare (recommended):${NC}
  - DNS A/AAAA for ${DOMAIN} → this server, Proxied (orange)
  - SSL/TLS mode: Full (strict)
  - Origin Certificate → ${SSL_DIR}/fullchain.pem + privkey.pem
  - Do NOT point subscribe/API DNS to this host

${BOLD}Security checklist:${NC}
  - This host must NOT run PostgreSQL/Redis/panel binary
  - Firewall: allow 443 (and 22 from ops IP only)
  - Verify: browser Network → API host is ${api}, not ${DOMAIN}
  - Verify: subscription links still use ${api}/api/v1/client/subscribe

Docs: https://github.com/${REPO}/blob/main/docs/MULTI_ORIGIN.md

EOF
}

main() {
  parse_args "$@"

  if [[ "$DO_UNINSTALL" -eq 1 ]]; then
    uninstall
  fi

  need_root

  if [[ -z "$DOMAIN" ]]; then
    read_tty DOMAIN "Shadow user domain (e.g. user.example.com)" ""
  fi
  [[ -n "$DOMAIN" ]] || die "Domain is required"
  DOMAIN="${DOMAIN#http://}"
  DOMAIN="${DOMAIN#https://}"
  DOMAIN="${DOMAIN%%/*}"

  if [[ -z "$API_BASE" ]]; then
    read_tty API_BASE "Panel API base URL (www, e.g. https://www.example.com)" ""
  fi
  API_BASE="$(normalize_api_base "$API_BASE")"
  [[ -n "$API_BASE" ]] || die "Panel API base is required"
  if [[ "$API_BASE" == "https://${DOMAIN}" || "$API_BASE" == "http://${DOMAIN}" ]]; then
    warn "API base equals this shadow domain — only OK if you reverse-proxy /api (not recommended)."
  fi

  info "Installing standalone user portal"
  info "  domain=${DOMAIN}  api=${API_BASE}  version=${VERSION}  dir=${WEB_DIR}"

  ensure_packages
  download_dist
  write_config_js
  harden_perms

  if [[ "$SKIP_NGINX" -eq 0 ]]; then
    install_nginx_site
  else
    warn "Skipped nginx (--skip-nginx)"
  fi

  write_meta
  print_next_steps
  ok "Done"
}

main "$@"
