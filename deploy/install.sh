#!/usr/bin/env bash
# =============================================================================
# K2Board one-click installer
#
# Interactive:     bash deploy/install.sh
# Non-interactive: bash deploy/install.sh panel.example.com 1
# Cloudflare:      bash deploy/install.sh panel.example.com 1 --cloudflare
#                  bash deploy/install.sh panel.example.com 1 --no-cloudflare
#                  bash deploy/install.sh panel.example.com 1 --cloudflare=auto
# Uninstall:       bash deploy/install.sh --uninstall
#                  bash deploy/install.sh --uninstall --purge   # also drop nginx + Real IP
#                  bash deploy/install.sh --uninstall --keep-nginx
# =============================================================================

set -euo pipefail

# ── colors / UI ──────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
  BLUE=$'\033[0;34m'; CYAN=$'\033[0;36m'; BOLD=$'\033[1m'; DIM=$'\033[2m'
  NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BLUE=""; CYAN=""; BOLD=""; DIM=""; NC=""
fi

INSTALL_DIR="/opt/k2board"
USER_WEB_DIR="/var/www/k2board-user"
STEP=0
TOTAL_STEPS=10
ADMIN_PATH_FILE="$INSTALL_DIR/.admin_path"
CF_REALIP_CONF="/etc/nginx/conf.d/cloudflare-realip.conf"
CF_FLAG_FILE="$INSTALL_DIR/.cloudflare"
# Survives app uninstall when nginx/TLS kept (reinstall-friendly CF marker)
CF_META_DIR="/etc/k2board"
CF_META_FILE="$CF_META_DIR/cloudflare.meta"
NGINX_SITE_AVAILABLE="/etc/nginx/sites-available/k2board"
NGINX_SITE_ENABLED="/etc/nginx/sites-enabled/k2board"
SSL_DIR="/etc/nginx/ssl"
# CF_MODE: auto | yes | no | "" (empty = will prompt / default auto)
CF_MODE=""
# After resolution: 0/1
CF_ENABLE_REALIP=0
# TLS strategy: le | cf_origin | cf_origin_api | existing | skip
TLS_STRATEGY=""
# Detection: none | dns | proxy
CF_DETECT="none"
CF_DETECT_DETAIL=""
# Cloudflare API token (memory only; also from env CF_API_TOKEN / --cf-token)
CF_API_TOKEN="${CF_API_TOKEN:-}"
CF_ORIGIN_API_OK=0
# Uninstall flags
DO_UNINSTALL=0
UNINSTALL_PURGE=0          # 1 = remove nginx site + Real IP even for CF
UNINSTALL_KEEP_NGINX=""    # ""=auto, 1=keep, 0=remove

# read from tty even when script is piped from curl
read_tty() {
  local __var="$1"
  local __prompt="${2:-}"
  local __def="${3:-}"
  local __input=""
  if [[ -n "$__prompt" ]]; then
    if [[ -n "$__def" ]]; then
      printf "%s [%s]: " "$__prompt" "$__def" >/dev/tty
    else
      printf "%s: " "$__prompt" >/dev/tty
    fi
  fi
  IFS= read -r __input </dev/tty || true
  if [[ -z "$__input" && -n "$__def" ]]; then
    __input="$__def"
  fi
  printf -v "$__var" '%s' "$__input"
}

confirm_yes() {
  local ans=""
  read_tty ans "$1" "Y"
  case "${ans,,}" in
    n|no) return 1 ;;
    *) return 0 ;;
  esac
}

banner() {
  echo ""
  echo -e "${CYAN}${BOLD}"
  cat <<'EOF'
  ╔══════════════════════════════════════════════════════╗
  ║              K2Board  ·  One-Click Setup             ║
  ║         Go panel · XrayR4u UniProxy compatible       ║
  ╚══════════════════════════════════════════════════════╝
EOF
  echo -e "${NC}"
}

step_begin() {
  STEP=$((STEP + 1))
  echo ""
  echo -e "${BLUE}${BOLD}[$STEP/$TOTAL_STEPS]${NC} ${BOLD}$1${NC}"
}

ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
info() { echo -e "  ${DIM}→${NC} $1"; }
warn() { echo -e "  ${YELLOW}!${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; }
die()  { fail "$1"; exit 1; }

run_quiet() {
  # Run command, swallow noise; return real exit code
  "$@" >/dev/null 2>&1
}

rand_hex() {
  # $1 = bytes
  head -c "$1" /dev/urandom 2>/dev/null | od -A n -t x1 | tr -d ' \n' || \
    openssl rand -hex "$1" 2>/dev/null || date +%s%N | sha256sum | head -c $(($1 * 2))
}

rand_alnum() {
  local n="${1:-12}"
  head -c 64 /dev/urandom 2>/dev/null | base64 | tr -d '/+=\n' | head -c "$n"
}

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    die "Please run as root (sudo)."
  fi
}

# DNS-over-HTTPS helpers (work when dig/host missing; need curl)
doh_type() {
  local d="$1" t="$2"
  command -v curl >/dev/null 2>&1 || return 0
  curl -fsSL --connect-timeout 4 --max-time 10 \
    -H 'accept: application/dns-json' \
    "https://cloudflare-dns.com/dns-query?name=${d}&type=${t}" 2>/dev/null \
    | tr ',' '\n' | sed -n 's/.*"data":"\([^"]*\)".*/\1/p' | tr -d '.' || true
}

# Resolve domain A/AAAA records (best-effort)
dns_a_records() {
  local d="$1"
  local out=""
  if command -v dig >/dev/null 2>&1; then
    out="$(dig +short A "$d" 2>/dev/null; dig +short AAAA "$d" 2>/dev/null)"
  elif command -v host >/dev/null 2>&1; then
    out="$(host -t A "$d" 2>/dev/null | awk '/has address/{print $NF}'; host -t AAAA "$d" 2>/dev/null | awk '/IPv6/{print $NF}')"
  elif command -v getent >/dev/null 2>&1; then
    out="$(getent ahosts "$d" 2>/dev/null | awk '{print $1}' | sort -u)"
  fi
  if [[ -z "$(echo "$out" | tr -d '[:space:]')" ]]; then
    out="$(doh_type "$d" A; doh_type "$d" AAAA)"
  fi
  echo "$out" | grep -E '^[0-9a-fA-F.:]+$' || true
}

dns_ns_records() {
  local d="$1"
  local out=""
  if command -v dig >/dev/null 2>&1; then
    out="$(dig +short NS "$d" 2>/dev/null)"
  elif command -v host >/dev/null 2>&1; then
    out="$(host -t NS "$d" 2>/dev/null | awk '{print $NF}' | tr -d '.')"
  fi
  if [[ -z "$(echo "$out" | tr -d '[:space:]')" ]]; then
    out="$(doh_type "$d" NS)"
  fi
  echo "$out"
}

# Parent zone for subdomain (panel.example.com → example.com)
dns_parent_zone() {
  local d="$1"
  if [[ "$d" == *.* && "$d" != *.*.* ]]; then
    echo "$d"
    return
  fi
  echo "${d#*.}"
}

# Return 0 if IPv4 is in dotted CIDR (pure bash, no ipcalc)
ipv4_in_cidr() {
  local ip="$1" cidr="$2"
  local net mask
  local a b c d na nb nc nd
  net="${cidr%/*}"
  mask="${cidr#*/}"
  [[ "$ip" == *.* && "$net" == *.* ]] || return 1
  [[ "$mask" =~ ^[0-9]+$ && "$mask" -ge 0 && "$mask" -le 32 ]] || return 1
  IFS=. read -r a b c d <<<"$ip"
  IFS=. read -r na nb nc nd <<<"$net"
  [[ -n "$a" && -n "$d" && -n "$na" && -n "$nd" ]] || return 1
  local ip_n=$(( (a<<24) + (b<<16) + (c<<8) + d ))
  local net_n=$(( (na<<24) + (nb<<16) + (nc<<8) + nd ))
  local mask_n=$(( (0xFFFFFFFF << (32 - mask)) & 0xFFFFFFFF ))
  (( (ip_n & mask_n) == (net_n & mask_n) ))
}

# Fallback CF IPv4 ranges (updated occasionally; live fetch preferred)
cf_fallback_ipv4() {
  cat <<'EOF'
173.245.48.0/20
103.21.244.0/22
103.22.200.0/22
103.31.4.0/22
141.101.64.0/18
108.162.192.0/18
190.93.240.0/20
188.114.96.0/20
197.234.240.0/22
198.41.128.0/17
162.158.0.0/15
104.16.0.0/13
104.24.0.0/14
172.64.0.0/13
131.0.72.0/22
EOF
}

fetch_cf_ipv4_list() {
  local tmp
  tmp="$(curl -fsSL --connect-timeout 5 --max-time 15 https://www.cloudflare.com/ips-v4 2>/dev/null || true)"
  if [[ -n "$tmp" ]] && echo "$tmp" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+'; then
    echo "$tmp"
  else
    cf_fallback_ipv4
  fi
}

fetch_cf_ipv6_list() {
  curl -fsSL --connect-timeout 5 --max-time 15 https://www.cloudflare.com/ips-v6 2>/dev/null \
    | grep -E '^[0-9a-fA-F:]+/[0-9]+' || true
}

# Detect whether domain is on Cloudflare (DNS and/or proxied A in CF ranges)
detect_cloudflare() {
  local domain="$1"
  CF_DETECT="none"
  CF_DETECT_DETAIL=""

  local ns parent ns_all
  parent="$(dns_parent_zone "$domain")"
  ns="$(dns_ns_records "$domain")"
  ns_all="$ns"
  if [[ -n "$parent" && "$parent" != "$domain" ]]; then
    ns_all="$ns_all"$'\n'"$(dns_ns_records "$parent")"
  fi
  if echo "$ns_all" | grep -qiE 'cloudflare|ns\.cloudflare'; then
    CF_DETECT="dns"
    CF_DETECT_DETAIL="NS points to Cloudflare"
  fi

  local ips ip ranges match=0
  ips="$(dns_a_records "$domain")"
  ranges="$(fetch_cf_ipv4_list)"
  while IFS= read -r ip; do
    [[ -z "$ip" || "$ip" == *:* ]] && continue
    while IFS= read -r cidr; do
      [[ -z "$cidr" ]] && continue
      if ipv4_in_cidr "$ip" "$cidr" 2>/dev/null; then
        match=1
        break 2
      fi
    done <<<"$ranges"
  done <<<"$ips"

  if [[ "$match" -eq 1 ]]; then
    CF_DETECT="proxy"
    CF_DETECT_DETAIL="A record is in Cloudflare anycast ranges (likely orange-cloud proxy)"
  elif [[ "$CF_DETECT" == "dns" ]]; then
    CF_DETECT_DETAIL="Cloudflare DNS (may be grey-cloud / DNS-only)"
  fi
}

write_cloudflare_realip_conf() {
  local v4 v6
  v4="$(fetch_cf_ipv4_list)"
  v6="$(fetch_cf_ipv6_list)"
  mkdir -p /etc/nginx/conf.d
  {
    echo "# Managed by K2Board install.sh — Cloudflare Real IP restoration"
    echo "# Safe when not behind CF: only rewrites when peer is in set_real_ip_from ranges."
    echo "# Regenerated on install when Cloudflare Real IP is enabled."
    echo ""
    echo "real_ip_header CF-Connecting-IP;"
    echo "real_ip_recursive on;"
    echo ""
    echo "# IPv4"
    while IFS= read -r cidr; do
      [[ -z "$cidr" ]] && continue
      echo "set_real_ip_from $cidr;"
    done <<<"$v4"
    if [[ -n "$v6" ]]; then
      echo ""
      echo "# IPv6"
      while IFS= read -r cidr; do
        [[ -z "$cidr" ]] && continue
        echo "set_real_ip_from $cidr;"
      done <<<"$v6"
    fi
  } > "$CF_REALIP_CONF"
}

remove_cloudflare_realip_conf() {
  rm -f "$CF_REALIP_CONF" 2>/dev/null || true
}

print_cf_origin_cert_help() {
  echo ""
  echo -e "  ${CYAN}${BOLD}Cloudflare Origin Certificate (manual)${NC}"
  echo -e "  ${DIM}────────────────────────────────────────${NC}"
  echo -e "  1. Dashboard → SSL/TLS → ${BOLD}Origin Server${NC} → Create Certificate"
  echo -e "     host: ${BOLD}$DOMAIN${NC}"
  echo -e "  2. Save as:"
  echo -e "       ${BOLD}/etc/nginx/ssl/fullchain.pem${NC}  ${DIM}(certificate)${NC}"
  echo -e "       ${BOLD}/etc/nginx/ssl/privkey.pem${NC}    ${DIM}(private key)${NC}"
  echo -e "  3. SSL/TLS mode → ${BOLD}Full (strict)${NC}"
  echo -e "  ${DIM}Or re-run with API Token to auto-issue (preferred).${NC}"
  echo ""
}

print_cf_token_help() {
  echo ""
  echo -e "  ${CYAN}${BOLD}Cloudflare API Token (Origin CA)${NC}"
  echo -e "  ${DIM}────────────────────────────────────────${NC}"
  echo -e "  Create Token: My Profile → API Tokens → Create Token"
  echo -e "  Permissions: ${BOLD}Zone → SSL and Certificates → Edit${NC}"
  echo -e "             + ${BOLD}Zone → Zone → Read${NC}  ${DIM}(to resolve zone / set Full strict)${NC}"
  echo -e "  Zone Resources: include the zone that owns ${BOLD}$DOMAIN${NC}"
  echo -e "  ${DIM}Token is used once in memory and not written to disk.${NC}"
  echo ""
}

# Secret prompt (no echo)
read_tty_secret() {
  local __var="$1"
  local __prompt="${2:-}"
  local __input=""
  if [[ -n "$__prompt" ]]; then
    printf "%s: " "$__prompt" >/dev/tty
  fi
  # -s hide paste; still works for long tokens
  IFS= read -rs __input </dev/tty || true
  printf "\n" >/dev/tty
  printf -v "$__var" '%s' "$__input"
}

# Parent registered zone guess: a.b.c.d → try progressively shorter
cf_zone_candidates() {
  local d="$1"
  local parts i n
  IFS='.' read -r -a parts <<<"$d"
  n=${#parts[@]}
  if [[ "$n" -lt 2 ]]; then
    echo "$d"
    return
  fi
  for ((i = 0; i <= n - 2; i++)); do
    local z=""
    local j
    for ((j = i; j < n; j++)); do
      [[ -n "$z" ]] && z+="."
      z+="${parts[j]}"
    done
    echo "$z"
  done
}

# Issue Origin CA via API: local ECC key + CSR → POST /certificates
# Writes /etc/nginx/ssl/{privkey,fullchain}.pem  Returns 0 on success.
issue_cloudflare_origin_api() {
  local domain="$1"
  local token="$2"
  local tmpdir key_file csr_file csr_raw body resp cert_pem zone_id z

  if ! command -v openssl >/dev/null 2>&1; then
    fail "openssl not found (required for Origin API CSR)"
    return 1
  fi
  if ! command -v curl >/dev/null 2>&1; then
    fail "curl not found"
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 not found (required to build/parse CF API JSON)"
    return 1
  fi

  token="$(echo -n "$token" | tr -d '\r\n ')"
  [[ -n "$token" ]] || return 1

  tmpdir="$(mktemp -d /tmp/k2board-cf-origin.XXXXXX)"
  key_file="$tmpdir/privkey.pem"
  csr_file="$tmpdir/req.csr"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmpdir'" RETURN

  info "Generating local ECC private key + CSR for $domain..."
  local req_type="origin-ecc"
  if ! openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
      -nodes -keyout "$key_file" -out "$csr_file" \
      -subj "/CN=${domain}" >/dev/null 2>&1; then
    # Fallback RSA if EC params unavailable
    req_type="origin-rsa"
    openssl req -new -newkey rsa:2048 -nodes \
      -keyout "$key_file" -out "$csr_file" \
      -subj "/CN=${domain}" >/dev/null 2>&1 || {
      fail "openssl CSR generation failed"
      return 1
    }
  fi

  csr_raw="$(cat "$csr_file")"
  body="$(DOMAIN_JSON="$domain" CSR_JSON="$csr_raw" REQ_TYPE="$req_type" python3 - <<'PY'
import json, os
print(json.dumps({
    "hostnames": [os.environ["DOMAIN_JSON"]],
    "requested_validity": 5475,
    "request_type": os.environ.get("REQ_TYPE") or "origin-ecc",
    "csr": os.environ["CSR_JSON"],
}))
PY
)"

  info "Requesting Cloudflare Origin CA certificate..."
  # Do not use curl -f so we can surface API error messages
  resp="$(curl -sS --connect-timeout 15 --max-time 60 \
    -X POST "https://api.cloudflare.com/client/v4/certificates" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "$body" 2>&1)" || {
    warn "Cloudflare API request failed (network)"
    echo "$resp" | head -c 500
    echo ""
    return 1
  }

  cert_pem="$(RESP_JSON="$resp" python3 - <<'PY'
import json, os, sys
try:
    d = json.loads(os.environ["RESP_JSON"])
except Exception as e:
    print("json parse error:", e, file=sys.stderr)
    sys.exit(1)
if not d.get("success"):
    errs = d.get("errors") or []
    msg = "; ".join(str(e.get("message", e)) for e in errs) or "unknown error"
    print(msg, file=sys.stderr)
    sys.exit(2)
cert = (d.get("result") or {}).get("certificate") or ""
if not cert.strip():
    print("empty certificate in API response", file=sys.stderr)
    sys.exit(3)
print(cert)
PY
)" || {
    warn "Origin CA API rejected request or returned no certificate"
    return 1
  }

  mkdir -p /etc/nginx/ssl
  umask 077
  cp "$key_file" /etc/nginx/ssl/privkey.pem
  printf '%s\n' "$cert_pem" > /etc/nginx/ssl/fullchain.pem
  chmod 600 /etc/nginx/ssl/privkey.pem
  chmod 644 /etc/nginx/ssl/fullchain.pem
  ok "Origin certificate written to /etc/nginx/ssl/{fullchain,privkey}.pem"

  # Best-effort: set SSL mode Full (strict) on the zone
  for z in $(cf_zone_candidates "$domain"); do
    local zjson
    zjson="$(curl -fsS --connect-timeout 10 --max-time 30 \
      -H "Authorization: Bearer ${token}" \
      -H "Content-Type: application/json" \
      "https://api.cloudflare.com/client/v4/zones?name=${z}" 2>/dev/null || true)"
    zone_id="$(ZONE_JSON="$zjson" ZONE_NAME="$z" python3 -c '
import json, os
raw = os.environ.get("ZONE_JSON") or ""
want = os.environ.get("ZONE_NAME") or ""
if not raw.strip():
    raise SystemExit(0)
try:
    d = json.loads(raw)
except Exception:
    raise SystemExit(0)
if not d.get("success"):
    raise SystemExit(0)
for row in d.get("result") or []:
    if row.get("name") == want:
        print(row.get("id") or "")
        break
' 2>/dev/null || true)"
    if [[ -n "$zone_id" ]]; then
      if curl -fsS --connect-timeout 10 --max-time 30 \
          -X PATCH "https://api.cloudflare.com/client/v4/zones/${zone_id}/settings/ssl" \
          -H "Authorization: Bearer ${token}" \
          -H "Content-Type: application/json" \
          -d '{"value":"strict"}' >/dev/null 2>&1; then
        ok "Cloudflare SSL mode set to Full (strict) on zone ${z}"
      else
        info "Could not set SSL mode automatically — set Full (strict) in the dashboard"
      fi
      break
    fi
  done

  return 0
}

# Fallback TLS choices when no API token / API failed
prompt_cf_tls_fallback() {
  echo ""
  echo -e "  ${BOLD}TLS fallback${NC}  ${DIM}(no API token or Origin API skipped)${NC}"
  echo -e "    ${BOLD}[1]${NC} Manual Origin cert files  ${DIM}/etc/nginx/ssl/{fullchain,privkey}.pem${NC}"
  echo -e "    ${BOLD}[2]${NC} Let's Encrypt standalone  ${DIM}prefer grey-cloud / direct A${NC}"
  echo -e "    ${BOLD}[3]${NC} Skip TLS for now"
  local tls_choice=""
  if [[ -e /dev/tty ]]; then
    read_tty tls_choice "  Select" "1"
  else
    tls_choice="3"
  fi
  case "$tls_choice" in
    2|le|letsencrypt|acme) TLS_STRATEGY="le" ;;
    3|skip|none|http)      TLS_STRATEGY="skip" ;;
    *)                     TLS_STRATEGY="cf_origin" ;;
  esac
  if [[ "$TLS_STRATEGY" == "cf_origin" ]]; then
    print_cf_origin_cert_help
    if [[ -e /dev/tty ]]; then
      echo -e "  Place Origin cert/key under ${BOLD}/etc/nginx/ssl/${NC}, then continue."
      if confirm_yes "  Wait — I will place certs now (retry check)"; then
        for _try in 1 2 3 4 5 6; do
          if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
            ok "Found /etc/nginx/ssl/{fullchain,privkey}.pem"
            TLS_STRATEGY="existing"
            return
          fi
          warn "Cert files not found yet (attempt $_try/6)..."
          sleep 3
        done
        warn "Origin cert still missing — will stay on HTTP until certs are added"
        TLS_STRATEGY="skip"
      else
        TLS_STRATEGY="skip"
      fi
    else
      TLS_STRATEGY="skip"
    fi
  fi
}

# Persist CF install markers (for smart uninstall / reinstall)
write_cf_meta() {
  local real_ip="${1:-0}" origin="${2:-0}" domain="${3:-}"
  mkdir -p "$INSTALL_DIR" "$CF_META_DIR"
  {
    echo "enabled"
    echo "real_ip=${real_ip}"
    echo "origin_cert=${origin}"
    echo "domain=${domain}"
    echo "updated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date)"
  } > "$CF_FLAG_FILE"
  cp -f "$CF_FLAG_FILE" "$CF_META_FILE" 2>/dev/null || true
  chmod 644 "$CF_META_FILE" 2>/dev/null || true
}

clear_cf_meta() {
  rm -f "$CF_FLAG_FILE" "$CF_META_FILE" 2>/dev/null || true
}

# Detect prior Cloudflare-oriented deploy (Real IP / Origin / meta marker)
is_cf_install() {
  if [[ -f "$CF_FLAG_FILE" ]] && grep -qiE '^(enabled|real_ip=1)' "$CF_FLAG_FILE" 2>/dev/null; then
    return 0
  fi
  if [[ -f "$CF_META_FILE" ]] && grep -qiE '^(enabled|real_ip=1)' "$CF_META_FILE" 2>/dev/null; then
    return 0
  fi
  if [[ -f "$CF_REALIP_CONF" ]]; then
    return 0
  fi
  return 1
}

usage_install() {
  cat <<EOF
Usage:
  bash deploy/install.sh [domain] [db:1|2] [admin_email] [admin_password] [flags]
  bash deploy/install.sh --uninstall [uninstall-flags]

Install flags:
  --cloudflare           Enable Cloudflare Real IP (orange-cloud recommended)
  --no-cloudflare        Disable Cloudflare Real IP
  --cloudflare=auto      Detect domain and decide (default when interactive)
  --cf-token=TOKEN       Cloudflare API Token for Origin CA (or env CF_API_TOKEN)
  -h, --help             Show this help

Uninstall flags:
  --uninstall, -u        Remove panel app (service, /opt/k2board, …)
  --keep-nginx           Keep nginx site + CF Real IP + TLS certs (for reinstall)
  --remove-nginx         Remove nginx k2board site + CF Real IP conf
  --purge                Same as --remove-nginx and also drop SSL files under /etc/nginx/ssl

Default uninstall behavior:
  • Cloudflare install (detected): KEEP nginx site, Real IP conf, and TLS certs
    so reinstall can reuse Origin CA without re-tokening.
  • Non-CF install: remove nginx k2board site; TLS files under /etc/nginx/ssl kept
    unless --purge.

TLS (when Cloudflare is enabled/detected):
  Prefer Origin CA via API Token (local key + CSR → CF signs).
  Empty token → fallback: manual cert files / Let's Encrypt / skip.

Examples:
  bash deploy/install.sh
  bash deploy/install.sh panel.example.com 1
  CF_API_TOKEN=xxx bash deploy/install.sh panel.example.com 1 --cloudflare
  bash deploy/install.sh panel.example.com 1 --cloudflare --cf-token=xxx
  bash deploy/install.sh --uninstall                 # CF: keep nginx+certs
  bash deploy/install.sh --uninstall --purge         # full proxy wipe
EOF
}

# ── uninstall ────────────────────────────────────────────────────────────────
do_uninstall() {
  banner
  local keep_nginx=0
  local remove_ssl=0
  local cf_detected=0
  if is_cf_install; then
    cf_detected=1
  fi

  # Resolve nginx keep policy
  if [[ "$UNINSTALL_PURGE" -eq 1 ]]; then
    keep_nginx=0
    remove_ssl=1
  elif [[ "$UNINSTALL_KEEP_NGINX" == "1" ]]; then
    keep_nginx=1
  elif [[ "$UNINSTALL_KEEP_NGINX" == "0" ]]; then
    keep_nginx=0
  elif [[ "$cf_detected" -eq 1 ]]; then
    keep_nginx=1
  else
    keep_nginx=0
  fi

  echo -e "${YELLOW}${BOLD}  Uninstall K2Board${NC}"
  echo -e "  Removes: systemd service, ${INSTALL_DIR} (binary/config), system user."
  if [[ "$cf_detected" -eq 1 ]]; then
    echo -e "  Detected: ${BOLD}Cloudflare-oriented install${NC} (Real IP / CF meta / Origin path)."
  fi
  if [[ "$keep_nginx" -eq 1 ]]; then
    echo -e "  Nginx:    ${GREEN}KEEP${NC} site + CF Real IP  ${DIM}(reinstall-friendly)${NC}"
    echo -e "  TLS:      ${GREEN}KEEP${NC} ${SSL_DIR}/{fullchain,privkey}.pem"
  else
    echo -e "  Nginx:    ${YELLOW}REMOVE${NC} k2board site + CF Real IP conf"
    if [[ "$remove_ssl" -eq 1 ]]; then
      echo -e "  TLS:      ${YELLOW}REMOVE${NC} ${SSL_DIR}  ${DIM}(--purge)${NC}"
    else
      echo -e "  TLS:      ${GREEN}KEEP${NC} ${SSL_DIR}  ${DIM}(use --purge to delete)${NC}"
    fi
  fi
  echo -e "  Database / Redis packages and data: ${BOLD}not${NC} removed."
  echo ""

  if [[ "$cf_detected" -eq 1 && "$keep_nginx" -eq 1 && -e /dev/tty && "$UNINSTALL_PURGE" -eq 0 && -z "$UNINSTALL_KEEP_NGINX" ]]; then
    info "CF reinstall tip: keeping nginx + Origin certs avoids re-pasting API Token."
    if ! confirm_yes "  Proceed (keep nginx + certs)"; then
      echo "  Cancelled. Use --purge to also remove nginx/Real IP/certs."
      exit 0
    fi
  else
    if ! confirm_yes "  Proceed with uninstall"; then
      echo "  Cancelled."
      exit 0
    fi
  fi

  TOTAL_STEPS=3
  STEP=0

  step_begin "Stop service"
  systemctl stop k2board 2>/dev/null || true
  systemctl disable k2board 2>/dev/null || true
  rm -f /etc/systemd/system/k2board.service
  systemctl daemon-reload 2>/dev/null || true
  ok "Service removed"

  step_begin "Nginx / TLS"
  if [[ "$keep_nginx" -eq 1 ]]; then
    info "Preserved: $NGINX_SITE_AVAILABLE (and enabled link if present)"
    info "Preserved: $CF_REALIP_CONF (if present)"
    info "Preserved: $SSL_DIR"
    # Ensure durable CF marker remains for next uninstall/reinstall detection
    if [[ ! -f "$CF_META_FILE" ]] && [[ -f "$CF_FLAG_FILE" || -f "$CF_REALIP_CONF" ]]; then
      mkdir -p "$CF_META_DIR"
      if [[ -f "$CF_FLAG_FILE" ]]; then
        cp -f "$CF_FLAG_FILE" "$CF_META_FILE" 2>/dev/null || true
      else
        {
          echo "enabled"
          echo "real_ip=1"
          echo "origin_cert=$([[ -f $SSL_DIR/fullchain.pem ]] && echo 1 || echo 0)"
          echo "note=preserved_on_uninstall"
        } > "$CF_META_FILE"
      fi
    fi
    ok "Nginx site + CF Real IP + TLS kept for reinstall"
  else
    rm -f "$NGINX_SITE_ENABLED" \
          "$NGINX_SITE_AVAILABLE" \
          /etc/nginx/conf.d/k2board.conf \
          "$CF_REALIP_CONF" 2>/dev/null || true
    if [[ "$remove_ssl" -eq 1 ]]; then
      rm -f "$SSL_DIR/fullchain.pem" "$SSL_DIR/privkey.pem" 2>/dev/null || true
      # only remove empty ssl dir
      rmdir "$SSL_DIR" 2>/dev/null || true
      ok "Nginx site, CF Real IP, and TLS files removed"
    else
      ok "Nginx site + CF Real IP removed (TLS files kept under $SSL_DIR)"
    fi
    clear_cf_meta
    if command -v nginx >/dev/null 2>&1; then
      nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
    fi
  fi

  step_begin "Remove application files"
  # If keeping nginx for reinstall, drop durable meta under /etc/k2board already written above
  rm -rf "$INSTALL_DIR" /tmp/k2board-build 2>/dev/null || true
  if [[ "$keep_nginx" -eq 0 ]]; then
    rm -rf "$CF_META_DIR" 2>/dev/null || true
  fi
  if [[ -d "$USER_WEB_DIR" ]]; then
    if [[ -e /dev/tty ]]; then
      if confirm_yes "  Also delete user frontend at $USER_WEB_DIR"; then
        rm -rf "$USER_WEB_DIR"
        ok "User frontend removed"
      else
        info "Kept $USER_WEB_DIR"
      fi
    else
      info "Kept $USER_WEB_DIR (non-interactive)"
    fi
  fi
  userdel k2board 2>/dev/null || true
  ok "Application directory cleaned"

  echo ""
  echo -e "${GREEN}${BOLD}  Uninstall complete.${NC}"
  if [[ "$keep_nginx" -eq 1 ]]; then
    echo -e "  ${CYAN}Reinstall:${NC} run install again with the same domain."
    echo -e "  Nginx + Origin certs were kept — installer should detect existing TLS."
    echo -e "  Optional: ${BOLD}--cloudflare${NC} again for Real IP (file kept if already present)."
  fi
  echo -e "  ${DIM}PostgreSQL/MySQL/Redis packages and DB data were not removed.${NC}"
  echo ""
  exit 0
}

# ── parse flags + positionals ──────────────────────────────────────────────────
POSITIONAL=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --uninstall|-u)
      DO_UNINSTALL=1
      shift
      ;;
    --purge)
      UNINSTALL_PURGE=1
      shift
      ;;
    --keep-nginx)
      UNINSTALL_KEEP_NGINX="1"
      shift
      ;;
    --remove-nginx)
      UNINSTALL_KEEP_NGINX="0"
      shift
      ;;
    --help|-h)
      usage_install
      exit 0
      ;;
    --cloudflare)
      CF_MODE="yes"
      shift
      ;;
    --no-cloudflare)
      CF_MODE="no"
      shift
      ;;
    --cloudflare=*)
      CF_MODE="${1#*=}"
      CF_MODE="$(echo "$CF_MODE" | tr '[:upper:]' '[:lower:]')"
      case "$CF_MODE" in
        yes|true|1|on) CF_MODE="yes" ;;
        no|false|0|off) CF_MODE="no" ;;
        auto) CF_MODE="auto" ;;
        *) die "Invalid --cloudflare value: use auto|yes|no" ;;
      esac
      shift
      ;;
    --cf-token=*)
      CF_API_TOKEN="${1#*=}"
      shift
      ;;
    --cf-token)
      if [[ -n "${2:-}" && "${2:0:1}" != "-" ]]; then
        CF_API_TOKEN="$2"
        shift 2
      else
        die "--cf-token requires a value (or use --cf-token=TOKEN / CF_API_TOKEN=)"
      fi
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do POSITIONAL+=("$1"); shift; done
      break
      ;;
    -*)
      die "Unknown option: $1 (try --help)"
      ;;
    *)
      POSITIONAL+=("$1")
      shift
      ;;
  esac
done

if [[ "$DO_UNINSTALL" -eq 1 ]]; then
  need_root
  do_uninstall
fi

# ── install ──────────────────────────────────────────────────────────────────
need_root
banner

DOMAIN="${POSITIONAL[0]:-}"
DB_CHOICE="${POSITIONAL[1]:-}"
ADMIN_EMAIL="${POSITIONAL[2]:-}"
ADMIN_PASS="${POSITIONAL[3]:-}"
REINSTALL=0
[[ -d "$INSTALL_DIR" && -f "$INSTALL_DIR/k2board" ]] && REINSTALL=1

if [[ "$REINSTALL" -eq 1 ]]; then
  echo -e "  ${YELLOW}Existing installation detected at $INSTALL_DIR${NC}"
  echo -e "  Config and secrets will be preserved when possible."
  echo ""
fi

# Collect inputs
if [[ -z "$DOMAIN" ]]; then
  echo -e "${BOLD}  Configuration${NC}"
  echo -e "  ${DIM}────────────────────────────────────────${NC}"
  while [[ -z "$DOMAIN" ]]; do
    read_tty DOMAIN "  Domain (e.g. panel.example.com)"
    [[ -z "$DOMAIN" ]] && fail "Domain is required"
  done
else
  echo -e "  Domain: ${BOLD}$DOMAIN${NC}"
fi
DOMAIN="$(echo "$DOMAIN" | tr '[:upper:]' '[:lower:]' | xargs)"
DOMAIN="${DOMAIN#http://}"
DOMAIN="${DOMAIN#https://}"
DOMAIN="${DOMAIN%%/*}"

if [[ -z "$DB_CHOICE" ]]; then
  echo ""
  echo -e "  Database engine:"
  echo -e "    ${BOLD}[1]${NC} PostgreSQL  ${DIM}(recommended)${NC}"
  echo -e "    ${BOLD}[2]${NC} MySQL / MariaDB"
  read_tty DB_CHOICE "  Select" "1"
fi
case "$DB_CHOICE" in
  2|mysql|mysql*|Maria*|maria*) DB_CHOICE=2; DB_LABEL="MySQL/MariaDB" ;;
  *) DB_CHOICE=1; DB_LABEL="PostgreSQL" ;;
esac

if [[ -z "$ADMIN_EMAIL" ]]; then
  read_tty ADMIN_EMAIL "  Admin email" "admin@k2board.com"
fi
ADMIN_EMAIL=$(echo "$ADMIN_EMAIL" | tr '[:upper:]' '[:lower:]' | xargs)

if [[ -z "$ADMIN_PASS" ]]; then
  # Only prompt if we are interactive (no password positional)
  if [[ ${#POSITIONAL[@]} -lt 4 ]]; then
    read_tty ADMIN_PASS "  Admin password (empty = random)"
  fi
fi
if [[ -z "$ADMIN_PASS" ]]; then
  ADMIN_PASS="$(rand_alnum 14)"
  GENERATED_PASS=1
else
  GENERATED_PASS=0
fi

# Preserve admin path on reinstall if present
if [[ -f "$ADMIN_PATH_FILE" ]]; then
  ADMIN_PATH="$(tr -d '[:space:]' < "$ADMIN_PATH_FILE")"
fi
if [[ -z "${ADMIN_PATH:-}" ]]; then
  ADMIN_PATH="$(rand_hex 4)"
fi

# ── Cloudflare Real IP: detect + option C (prompt / flags) ───────────────────
info "Detecting Cloudflare for ${DOMAIN}..."
detect_cloudflare "$DOMAIN"
case "$CF_DETECT" in
  proxy)
    ok "Cloudflare detected: $CF_DETECT_DETAIL"
    ;;
  dns)
    warn "Cloudflare DNS detected: $CF_DETECT_DETAIL"
    info "If the record is orange-cloud (proxied), enable Real IP and use Origin Certificate."
    ;;
  *)
    info "No Cloudflare signal on DNS/A (direct or other CDN)."
    ;;
esac

# Resolve CF_MODE default
if [[ -z "$CF_MODE" ]]; then
  if [[ -t 0 || -t 1 ]] && [[ -e /dev/tty ]]; then
    # interactive default from detection
    if [[ "$CF_DETECT" == "proxy" || "$CF_DETECT" == "dns" ]]; then
      CF_MODE="auto" # will map to yes after prompt default
    else
      CF_MODE="auto"
    fi
  else
    # non-interactive without flag: only enable when clearly proxied
    if [[ "$CF_DETECT" == "proxy" ]]; then
      CF_MODE="auto"
    else
      CF_MODE="no"
    fi
  fi
fi

# Interactive prompt when mode is auto (and we can talk to the user)
if [[ "$CF_MODE" == "auto" ]]; then
  local_default="n"
  [[ "$CF_DETECT" == "proxy" || "$CF_DETECT" == "dns" ]] && local_default="Y"
  echo ""
  echo -e "  ${BOLD}Cloudflare Real IP${NC}  ${DIM}(restores visitor IP for rate-limit / logs)${NC}"
  echo -e "    ${BOLD}[Y]${NC} Enable  ${DIM}— recommended when domain is orange-cloud proxied${NC}"
  echo -e "    ${BOLD}[n]${NC} Disable ${DIM}— direct IP / grey-cloud only${NC}"
  if [[ -e /dev/tty ]]; then
    ans=""
    read_tty ans "  Enable Cloudflare Real IP" "$local_default"
    case "${ans,,}" in
      n|no|0) CF_MODE="no" ;;
      *) CF_MODE="yes" ;;
    esac
  else
    # no tty: auto means enable on CF detect
    if [[ "$CF_DETECT" == "proxy" || "$CF_DETECT" == "dns" ]]; then
      CF_MODE="yes"
    else
      CF_MODE="no"
    fi
  fi
fi

case "$CF_MODE" in
  yes) CF_ENABLE_REALIP=1 ;;
  *)   CF_ENABLE_REALIP=0 ;;
esac

# TLS strategy — CF path: Token → Origin API (preferred); empty token → manual / LE / skip
if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
  TLS_STRATEGY="existing"
elif [[ "$CF_ENABLE_REALIP" -eq 1 || "$CF_DETECT" == "proxy" || "$CF_DETECT" == "dns" ]]; then
  echo ""
  echo -e "  ${YELLOW}${BOLD}TLS behind Cloudflare${NC}"
  echo -e "  Prefer ${BOLD}Origin CA via API Token${NC} (auto-issue; set SSL to Full strict)."
  echo -e "  Press Enter without a token to fall back to manual cert files or Let's Encrypt."
  print_cf_token_help

  # Prefer env / --cf-token; otherwise interactive paste
  if [[ -z "$CF_API_TOKEN" && -e /dev/tty ]]; then
    tok=""
    read_tty_secret tok "  Paste Cloudflare API Token (Enter to skip)"
    CF_API_TOKEN="$(echo -n "$tok" | tr -d '\r\n ')"
  fi
  CF_API_TOKEN="$(echo -n "${CF_API_TOKEN:-}" | tr -d '\r\n ')"

  if [[ -n "$CF_API_TOKEN" ]]; then
    TLS_STRATEGY="cf_origin_api"
    info "Will issue Origin certificate via Cloudflare API during Nginx step"
  else
    info "No API Token — choosing fallback TLS method"
    prompt_cf_tls_fallback
  fi
else
  TLS_STRATEGY="le"
fi

echo ""
echo -e "${BOLD}  Summary${NC}"
echo -e "  ${DIM}────────────────────────────────────────${NC}"
echo -e "  Domain          : ${BOLD}$DOMAIN${NC}"
echo -e "  Database        : ${BOLD}$DB_LABEL${NC}"
echo -e "  Admin email     : ${BOLD}$ADMIN_EMAIL${NC}"
echo -e "  Admin password  : ${BOLD}$ADMIN_PASS${NC}$([[ $GENERATED_PASS -eq 1 ]] && echo "  ${DIM}(generated)${NC}" || true)"
echo -e "  Admin panel URL : ${BOLD}https://$DOMAIN/$ADMIN_PATH/${NC}"
echo -e "  Mode            : ${BOLD}$([[ $REINSTALL -eq 1 ]] && echo reinstall || echo fresh)${NC}"
echo -e "  CF detect       : ${BOLD}$CF_DETECT${NC}$([[ -n "$CF_DETECT_DETAIL" ]] && echo "  ${DIM}($CF_DETECT_DETAIL)${NC}" || true)"
echo -e "  CF Real IP      : ${BOLD}$([[ $CF_ENABLE_REALIP -eq 1 ]] && echo enabled || echo disabled)${NC}"
echo -e "  TLS strategy    : ${BOLD}$TLS_STRATEGY${NC}"
if [[ "$TLS_STRATEGY" == "cf_origin_api" ]]; then
  echo -e "  CF API Token    : ${BOLD}provided${NC}  ${DIM}(not saved to disk)${NC}"
fi
echo -e "  ${DIM}────────────────────────────────────────${NC}"
echo ""

if ! confirm_yes "  Start installation"; then
  echo "  Cancelled."
  exit 0
fi

# ── [1] OS detect ────────────────────────────────────────────────────────────
step_begin "Detect operating system"
if command -v apt-get &>/dev/null; then
  PKG_MGR="apt"
  pkg_install() { DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" >/dev/null; }
  pkg_update()  { DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null 2>&1 || true; }
elif command -v dnf &>/dev/null; then
  PKG_MGR="dnf"
  pkg_install() { dnf install -y -q "$@" >/dev/null; }
  pkg_update()  { true; }
elif command -v yum &>/dev/null; then
  PKG_MGR="yum"
  pkg_install() { yum install -y -q "$@" >/dev/null; }
  pkg_update()  { true; }
else
  die "Unsupported package manager (need apt / dnf / yum)"
fi
ok "Package manager: $PKG_MGR"
info "Architecture: $(uname -m) · Kernel: $(uname -r)"

# ── [2] System packages ──────────────────────────────────────────────────────
step_begin "Install system packages"
pkg_update
info "Installing curl, wget, nginx, git, openssl, dns tools..."
if [[ "$PKG_MGR" == "apt" ]]; then
  pkg_install curl wget nginx git openssl ca-certificates dnsutils python3 || \
    pkg_install curl wget nginx git openssl ca-certificates python3 || true
else
  pkg_install curl wget nginx git openssl bind-utils python3 || \
    pkg_install curl wget nginx git openssl python3 || true
fi
for bin in curl wget nginx openssl python3; do
  command -v "$bin" >/dev/null 2>&1 && ok "$bin ready" || warn "$bin not found"
done

# ── [3] Database ─────────────────────────────────────────────────────────────
step_begin "Configure database ($DB_LABEL)"
JWT_SECRET="$(rand_alnum 32)"
DB_PASS="$(rand_alnum 18)"
DB_NAME="k2board"
DB_USER="k2board"

if [[ "$DB_CHOICE" -eq 1 ]]; then
  info "Installing PostgreSQL..."
  if [[ "$PKG_MGR" == "apt" ]]; then
    pkg_install postgresql postgresql-contrib || true
  else
    pkg_install postgresql-server postgresql || pkg_install postgresql || true
    # RHEL first init
    if command -v postgresql-setup &>/dev/null; then
      run_quiet postgresql-setup --initdb || run_quiet postgresql-setup initdb || true
    fi
  fi

  systemctl enable postgresql >/dev/null 2>&1 || systemctl enable postgresql-14 >/dev/null 2>&1 || true
  systemctl start postgresql  >/dev/null 2>&1 || systemctl start postgresql-14 >/dev/null 2>&1 || true
  sleep 1

  # Idempotent role + database (no noisy errors on reinstall)
  info "Ensuring role and database exist..."
  su - postgres -c "psql -v ON_ERROR_STOP=0 -tAc \"SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'\"" 2>/dev/null | grep -q 1 || \
    su - postgres -c "psql -v ON_ERROR_STOP=1 -c \"CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';\"" >/dev/null 2>&1 || true

  # Always reset password so DSN we write matches (only when creating new config later we may keep old)
  NEW_DB_PASS="$DB_PASS"
  # If reinstall and config exists, try to keep existing DSN password
  KEEP_EXISTING_DSN=0
  if [[ -f "$INSTALL_DIR/config.yml" ]]; then
    KEEP_EXISTING_DSN=1
    info "Existing config.yml found — keeping previous database DSN"
  else
    su - postgres -c "psql -v ON_ERROR_STOP=0 -c \"ALTER USER ${DB_USER} WITH PASSWORD '${NEW_DB_PASS}';\"" >/dev/null 2>&1 || true
    su - postgres -c "psql -v ON_ERROR_STOP=0 -tAc \"SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'\"" 2>/dev/null | grep -q 1 || \
      su - postgres -c "psql -v ON_ERROR_STOP=1 -c \"CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};\"" >/dev/null 2>&1 || true
    su - postgres -c "psql -v ON_ERROR_STOP=0 -c \"GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};\"" >/dev/null 2>&1 || true
    # PG15+ schema privileges
    su - postgres -c "psql -d ${DB_NAME} -v ON_ERROR_STOP=0 -c \"GRANT ALL ON SCHEMA public TO ${DB_USER};\"" >/dev/null 2>&1 || true
  fi

  DB_DRIVER="postgres"
  if [[ "$KEEP_EXISTING_DSN" -eq 0 ]]; then
    DB_DSN="host=localhost user=${DB_USER} password=${NEW_DB_PASS} dbname=${DB_NAME} port=5432 sslmode=disable TimeZone=UTC"
  else
    DB_DSN=""  # will not overwrite
  fi

  # Connectivity probe (only for fresh credentials)
  if [[ "$KEEP_EXISTING_DSN" -eq 0 ]]; then
    if command -v psql >/dev/null 2>&1; then
      if PGPASSWORD="$NEW_DB_PASS" psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT 1" >/dev/null 2>&1; then
        ok "PostgreSQL connection verified"
      else
        warn "Could not verify app-user login yet (peer/hba). Service may still work via local socket rules."
        ok "PostgreSQL role/database prepared"
      fi
    else
      ok "PostgreSQL packages installed"
    fi
  else
    ok "PostgreSQL left unchanged (reinstall)"
  fi
else
  info "Installing MySQL/MariaDB..."
  pkg_install mysql-server 2>/dev/null || pkg_install mariadb-server 2>/dev/null || true
  systemctl enable mysql >/dev/null 2>&1 || systemctl enable mariadb >/dev/null 2>&1 || true
  systemctl start mysql  >/dev/null 2>&1 || systemctl start mariadb  >/dev/null 2>&1 || true
  sleep 1

  MYSQL_BIN="mysql"
  command -v mysql >/dev/null 2>&1 || MYSQL_BIN="mariadb"

  KEEP_EXISTING_DSN=0
  if [[ -f "$INSTALL_DIR/config.yml" ]]; then
    KEEP_EXISTING_DSN=1
    info "Existing config.yml found — keeping previous database DSN"
  else
    $MYSQL_BIN -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" >/dev/null 2>&1 || true
    # Drop+recreate user is noisy; use CREATE IF NOT EXISTS + ALTER
    $MYSQL_BIN -e "CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';" >/dev/null 2>&1 || \
      $MYSQL_BIN -e "GRANT USAGE ON *.* TO '${DB_USER}'@'localhost';" >/dev/null 2>&1 || true
    $MYSQL_BIN -e "ALTER USER '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';" >/dev/null 2>&1 || true
    $MYSQL_BIN -e "GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost'; FLUSH PRIVILEGES;" >/dev/null 2>&1 || true
  fi

  DB_DRIVER="mysql"
  if [[ "$KEEP_EXISTING_DSN" -eq 0 ]]; then
    DB_DSN="${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?charset=utf8mb4&parseTime=true&loc=Local"
    if $MYSQL_BIN -u"$DB_USER" -p"$DB_PASS" -e "SELECT 1" "$DB_NAME" >/dev/null 2>&1; then
      ok "MySQL connection verified"
    else
      warn "MySQL user probe failed; check root auth plugin if service cannot start"
      ok "MySQL database prepared"
    fi
  else
    DB_DSN=""
    ok "MySQL left unchanged (reinstall)"
  fi
fi

# ── [4] Redis ────────────────────────────────────────────────────────────────
step_begin "Configure Redis"
pkg_install redis-server 2>/dev/null || pkg_install redis 2>/dev/null || true
systemctl enable redis-server >/dev/null 2>&1 || systemctl enable redis >/dev/null 2>&1 || true
systemctl start redis-server  >/dev/null 2>&1 || systemctl start redis  >/dev/null 2>&1 || true
REDIS_ENABLED="true"
if command -v redis-cli >/dev/null 2>&1 && redis-cli ping 2>/dev/null | grep -qi pong; then
  ok "Redis is running (PONG)"
else
  warn "Redis not responding — panel will fall back to in-memory traffic buffer"
  REDIS_ENABLED="false"
fi

# ── [5] Download binary ──────────────────────────────────────────────────────
step_begin "Install K2Board binary"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Unsupported CPU arch: $ARCH" ;;
esac

mkdir -p "$INSTALL_DIR"
RELEASE_URL="https://github.com/HenZenKuriRIP/K2board/releases/latest/download/k2board-linux-${ARCH}"
TMP_BIN="$(mktemp)"
info "Downloading $RELEASE_URL"
if curl -fSL --retry 3 --connect-timeout 15 -o "$TMP_BIN" "$RELEASE_URL" && [[ -s "$TMP_BIN" ]]; then
  systemctl stop k2board 2>/dev/null || true
  mv -f "$TMP_BIN" "$INSTALL_DIR/k2board"
  chmod +x "$INSTALL_DIR/k2board"
  ok "Binary installed ($(du -h "$INSTALL_DIR/k2board" | awk '{print $1}'), linux-${ARCH})"
else
  rm -f "$TMP_BIN"
  warn "Release download failed — building from source (API-only if frontend missing)"
  pkg_install golang-go 2>/dev/null || pkg_install golang 2>/dev/null || die "Go toolchain install failed"
  rm -rf /tmp/k2board-build
  git clone --depth 1 https://github.com/HenZenKuriRIP/K2board /tmp/k2board-build
  (
    cd /tmp/k2board-build
    mkdir -p web/dist && touch web/dist/.gitkeep
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/k2board" ./cmd/server
  )
  chmod +x "$INSTALL_DIR/k2board"
  ok "Built from source"
fi

# ── [6] Config & secrets ─────────────────────────────────────────────────────
step_begin "Write configuration"
mkdir -p "$INSTALL_DIR"
echo -n "$ADMIN_PATH" > "$ADMIN_PATH_FILE"
chmod 600 "$ADMIN_PATH_FILE"

if [[ -f "$INSTALL_DIR/config.yml" ]]; then
  ok "Kept existing config.yml"
  # Ensure auto_disable_interval exists (best-effort, non-destructive)
  if ! grep -q "auto_disable_interval" "$INSTALL_DIR/config.yml" 2>/dev/null; then
    info "Note: you may add scheduler.auto_disable_interval manually if missing"
  fi
else
  cat > "$INSTALL_DIR/config.yml" <<YEOF
server:
  host: "127.0.0.1"
  port: 8080
  mode: "release"
  node_rate_limit: 50

database:
  driver: "${DB_DRIVER}"
  dsn: "${DB_DSN}"

redis:
  enabled: ${REDIS_ENABLED}
  addr: "localhost:6379"
  password: ""
  db: 0

scheduler:
  flush_interval: 60
  stats_interval: 300
  auto_disable_interval: 60

jwt:
  expire_hours: 24

admin:
  email: "${ADMIN_EMAIL}"
YEOF
  chmod 600 "$INSTALL_DIR/config.yml"
  ok "Created config.yml"
fi

if [[ -f "$INSTALL_DIR/.env" ]]; then
  ok "Kept existing .env secrets"
  # Optionally update admin.password if user provided one interactively and wants?
  # Keep as-is for safety on reinstall.
else
  cat > "$INSTALL_DIR/.env" <<EOF
jwt.secret=${JWT_SECRET}
admin.password=${ADMIN_PASS}
EOF
  chmod 600 "$INSTALL_DIR/.env"
  ok "Created .env (jwt + admin password)"
fi

# ── [7] System user ──────────────────────────────────────────────────────────
step_begin "Create system user"
if id k2board &>/dev/null; then
  ok "User k2board already exists"
else
  useradd -r -s /usr/sbin/nologin -d "$INSTALL_DIR" k2board 2>/dev/null || \
    useradd -r -s /bin/false -d "$INSTALL_DIR" k2board
  ok "Created system user k2board"
fi
chown -R k2board:k2board "$INSTALL_DIR"
ok "Permissions applied on $INSTALL_DIR"

# ── [8] User frontend ────────────────────────────────────────────────────────
step_begin "Deploy user portal frontend"
mkdir -p "$USER_WEB_DIR"
USER_DIST="https://github.com/HenZenKuriRIP/K2board/releases/latest/download/k2board-user-dist.tar.gz"
info "Downloading user portal assets..."
if curl -fSL --retry 2 --connect-timeout 15 -o /tmp/k2board-user.tar.gz "$USER_DIST" \
   && [[ -s /tmp/k2board-user.tar.gz ]]; then
  # clear only contents, keep dir
  find "$USER_WEB_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
  tar -xzf /tmp/k2board-user.tar.gz -C "$USER_WEB_DIR/" 2>/dev/null || true
  rm -f /tmp/k2board-user.tar.gz
  if [[ -f "$USER_WEB_DIR/index.html" ]]; then
    ok "User portal deployed to $USER_WEB_DIR"
  else
    warn "Archive extracted but index.html missing"
  fi
else
  warn "User portal download failed — admin panel still works; user site may be empty"
fi

# ── [9] Nginx + TLS ──────────────────────────────────────────────────────────
step_begin "Configure Nginx reverse proxy"
write_nginx_http() {
  cat > /etc/nginx/sites-available/k2board <<NEOF
server {
    listen 80;
    server_name ${DOMAIN};
    client_max_body_size 10m;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
    }

    location /${ADMIN_PATH}/ {
        rewrite ^/${ADMIN_PATH}/(.*) /\$1 break;
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
    }
    location = /${ADMIN_PATH} { return 302 /${ADMIN_PATH}/; }

    location /assets/ {
        root ${USER_WEB_DIR};
        try_files \$uri @go;
        access_log off;
        expires 7d;
    }
    location /favicon.ico {
        root ${USER_WEB_DIR};
        try_files \$uri @go;
        access_log off;
    }
    # Login background / config.js — must be before catch-all 302 /
    location ~* ^/(background\.(png|jpe?g|webp)|config\.js|config\.example\.js|robots\.txt)\$ {
        root ${USER_WEB_DIR};
        try_files \$uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }

    location = / {
        root ${USER_WEB_DIR};
        try_files /index.html =404;
    }
    location / { return 302 /; }

    location @go {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
    }
}
NEOF
}

write_nginx_https() {
  cat > /etc/nginx/sites-available/k2board <<NEOF
server {
    listen 80;
    server_name ${DOMAIN};
    return 301 https://\$host\$request_uri;
}
server {
    listen 443 ssl http2;
    server_name ${DOMAIN};
    client_max_body_size 10m;

    ssl_certificate     /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Connection "";
    }

    location /${ADMIN_PATH}/ {
        rewrite ^/${ADMIN_PATH}/(.*) /\$1 break;
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Connection "";
    }
    location = /${ADMIN_PATH} { return 302 /${ADMIN_PATH}/; }

    location /assets/ {
        root ${USER_WEB_DIR};
        try_files \$uri @go;
        access_log off;
        expires 7d;
    }
    location /favicon.ico {
        root ${USER_WEB_DIR};
        try_files \$uri @go;
        access_log off;
    }
    # Login background / config.js — must be before catch-all 302 /
    location ~* ^/(background\.(png|jpe?g|webp)|config\.js|config\.example\.js|robots\.txt)\$ {
        root ${USER_WEB_DIR};
        try_files \$uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }

    location = / {
        root ${USER_WEB_DIR};
        try_files /index.html =404;
    }
    location / { return 302 /; }

    location @go {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Connection "";
    }
}
NEOF
}

write_nginx_http
mkdir -p /etc/nginx/sites-enabled /etc/nginx/conf.d
ln -sfn /etc/nginx/sites-available/k2board /etc/nginx/sites-enabled/k2board
rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true

# Cloudflare Real IP (optional)
mkdir -p "$INSTALL_DIR"
if [[ "$CF_ENABLE_REALIP" -eq 1 ]]; then
  info "Writing Cloudflare Real IP config → $CF_REALIP_CONF"
  write_cloudflare_realip_conf
  # origin_cert bit refined after TLS step; provisional now
  write_cf_meta 1 0 "${DOMAIN:-}"
  ok "Cloudflare Real IP enabled (CF-Connecting-IP)"
else
  remove_cloudflare_realip_conf
  # Keep meta only if this was a CF Origin path without Real IP (rare)
  if [[ "$TLS_STRATEGY" == "cf_origin_api" || "$TLS_STRATEGY" == "cf_origin" || "$TLS_STRATEGY" == "existing" ]]; then
    if [[ -f "$SSL_DIR/fullchain.pem" ]] || [[ "$CF_DETECT" != "none" ]]; then
      write_cf_meta 0 1 "${DOMAIN:-}"
    else
      clear_cf_meta
      echo "disabled" > "$CF_FLAG_FILE"
    fi
  else
    clear_cf_meta
    echo "disabled" > "$CF_FLAG_FILE"
  fi
  info "Cloudflare Real IP disabled"
fi

if nginx -t >/dev/null 2>&1; then
  systemctl enable nginx >/dev/null 2>&1 || true
  systemctl reload nginx 2>/dev/null || systemctl restart nginx 2>/dev/null || true
  ok "Nginx HTTP config active for $DOMAIN"
else
  warn "nginx -t failed — check /etc/nginx/sites-available/k2board and $CF_REALIP_CONF"
  # Real IP conf might break old nginx without real_ip module — rare on modern packages
  if [[ "$CF_ENABLE_REALIP" -eq 1 ]]; then
    warn "If error mentions real_ip_*, remove $CF_REALIP_CONF and reload nginx"
  fi
fi

# TLS
issue_letsencrypt() {
  info "Requesting Let's Encrypt certificate via acme.sh..."
  if [[ "$CF_DETECT" == "proxy" || "$CF_ENABLE_REALIP" -eq 1 ]]; then
    warn "Behind Cloudflare: HTTP-01 may fail unless record is grey-cloud / DNS-only."
  fi
  if curl -fsSL https://get.acme.sh | sh -s email="$ADMIN_EMAIL" >/dev/null 2>&1; then
    systemctl stop nginx >/dev/null 2>&1 || true
    if /root/.acme.sh/acme.sh --issue --standalone -d "$DOMAIN" --keylength ec-256 >/dev/null 2>&1; then
      mkdir -p /etc/nginx/ssl
      /root/.acme.sh/acme.sh --install-cert -d "$DOMAIN" --ecc \
        --key-file /etc/nginx/ssl/privkey.pem \
        --fullchain-file /etc/nginx/ssl/fullchain.pem \
        --reloadcmd "systemctl reload nginx" >/dev/null 2>&1 || true
      write_nginx_https
      systemctl start nginx >/dev/null 2>&1 || true
      nginx -t >/dev/null 2>&1 && systemctl reload nginx >/dev/null 2>&1 || true
      ok "TLS certificate issued for $DOMAIN"
      return 0
    fi
    systemctl start nginx >/dev/null 2>&1 || true
    warn "Certificate issue failed — continuing on HTTP"
    return 1
  fi
  warn "acme.sh install failed — continuing on HTTP"
  return 1
}

case "$TLS_STRATEGY" in
  existing)
    if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
      write_nginx_https
      nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
      if [[ "$CF_ENABLE_REALIP" -eq 1 || "$CF_DETECT" != "none" || -f "$CF_REALIP_CONF" ]]; then
        write_cf_meta "$([[ $CF_ENABLE_REALIP -eq 1 || -f $CF_REALIP_CONF ]] && echo 1 || echo 0)" 1 "${DOMAIN:-}"
        info "CF tip: SSL/TLS mode should be Full (strict) with Origin or public certs on origin."
      fi
      ok "Using TLS certificates at /etc/nginx/ssl/"
    else
      warn "TLS strategy=existing but cert files missing — falling back"
      TLS_STRATEGY="le"
      issue_letsencrypt || true
    fi
    ;;
  cf_origin_api)
    if issue_cloudflare_origin_api "$DOMAIN" "$CF_API_TOKEN"; then
      CF_ORIGIN_API_OK=1
      write_nginx_https
      nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
      write_cf_meta "$([[ $CF_ENABLE_REALIP -eq 1 ]] && echo 1 || echo 0)" 1 "${DOMAIN:-}"
      ok "HTTPS active with Cloudflare Origin CA (API)"
    else
      warn "Origin CA via API failed — falling back"
      prompt_cf_tls_fallback
      case "$TLS_STRATEGY" in
        existing|cf_origin)
          if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
            write_nginx_https
            nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
            write_cf_meta "$([[ $CF_ENABLE_REALIP -eq 1 ]] && echo 1 || echo 0)" 1 "${DOMAIN:-}"
            ok "HTTPS active with certificates on disk"
          fi
          ;;
        le)
          issue_letsencrypt || true
          ;;
        *)
          info "Staying on HTTP for now"
          ;;
      esac
    fi
    # Never leave token in environment after use
    CF_API_TOKEN=""
    unset CF_API_TOKEN || true
    ;;
  cf_origin)
    print_cf_origin_cert_help
    if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
      write_nginx_https
      nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
      ok "Origin certificates installed — HTTPS active"
    else
      warn "Origin certificate not present — staying on HTTP until certs are placed"
    fi
    ;;
  skip)
    info "TLS skipped — site on HTTP. Add certs under /etc/nginx/ssl/ later."
    if [[ "$CF_ENABLE_REALIP" -eq 1 || "$CF_DETECT" != "none" ]]; then
      print_cf_origin_cert_help
    fi
    ;;
  le|*)
    if [[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem ]]; then
      write_nginx_https
      nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true
      ok "Reused existing TLS certificates"
    else
      issue_letsencrypt || true
    fi
    ;;
esac

# Always scrub token after TLS step
CF_API_TOKEN=""
unset CF_API_TOKEN 2>/dev/null || true

SCHEME="http"
[[ -f /etc/nginx/ssl/fullchain.pem && -f /etc/nginx/ssl/privkey.pem && -f /etc/nginx/sites-available/k2board ]] && \
  grep -q 'listen 443' /etc/nginx/sites-available/k2board 2>/dev/null && SCHEME="https"

# ── [10] systemd + start ─────────────────────────────────────────────────────
step_begin "Enable and start K2Board service"
cat > /etc/systemd/system/k2board.service <<EOF
[Unit]
Description=K2Board panel
After=network.target postgresql.service mysql.service mariadb.service redis.service redis-server.service
Wants=network-online.target

[Service]
Type=simple
User=k2board
Group=k2board
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/k2board
Restart=always
RestartSec=3
LimitNOFILE=65535
# Environment secrets loaded from .env by the app

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable k2board >/dev/null 2>&1 || true
systemctl restart k2board

# Wait for health
info "Waiting for API to become ready..."
READY=0
for i in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:8080/api/v1/user/plans" >/dev/null 2>&1 \
     || curl -fsS -o /dev/null -w "%{http_code}" "http://127.0.0.1:8080/" 2>/dev/null | grep -qE '200|301|302|404'; then
    READY=1
    break
  fi
  sleep 0.5
done

if systemctl is-active --quiet k2board; then
  ok "Service k2board is active"
else
  fail "Service failed to start — run: journalctl -u k2board -n 50 --no-pager"
fi
if [[ "$READY" -eq 1 ]]; then
  ok "Local HTTP endpoint responding"
else
  warn "Service is up but health probe timed out — check logs"
fi

# Capture panel token hint from first boot if present
PANEL_HINT=""
if journalctl -u k2board -n 80 --no-pager 2>/dev/null | grep -qi "panel token"; then
  PANEL_HINT="$(journalctl -u k2board -n 80 --no-pager 2>/dev/null | grep -i "panel token" | tail -1 || true)"
fi

# ── finish ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}"
cat <<EOF
  ╔══════════════════════════════════════════════════════╗
  ║                 Installation complete                ║
  ╚══════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"
echo -e "  ${BOLD}User portal${NC}     ${SCHEME}://${DOMAIN}/"
echo -e "  ${BOLD}Admin panel${NC}     ${SCHEME}://${DOMAIN}/${ADMIN_PATH}/"
echo -e "  ${BOLD}Admin email${NC}     ${ADMIN_EMAIL}"
echo -e "  ${BOLD}Admin password${NC}  ${ADMIN_PASS}"
echo ""
echo -e "  ${YELLOW}${BOLD}Bookmark the admin URL${NC} — path /${ADMIN_PATH}/ is random and not linked publicly."
echo ""
echo -e "  ${BOLD}XrayR4u example${NC}"
echo -e "    ApiHost: \"${SCHEME}://${DOMAIN}\""
echo -e "    ApiKey:  (Settings → Panel Token after first login)"
echo -e "    NodeID:  <your node id>"
echo ""
echo -e "  ${BOLD}Useful commands${NC}"
echo -e "    systemctl status k2board"
echo -e "    journalctl -u k2board -f"
echo -e "    nginx -t && systemctl reload nginx"
echo ""
if [[ "$CF_ENABLE_REALIP" -eq 1 ]]; then
  echo -e "  ${BOLD}Cloudflare${NC}"
  echo -e "    Real IP conf:  ${CF_REALIP_CONF}"
  echo -e "    SSL mode:      Full (strict)  ${DIM}— not Flexible${NC}"
  echo -e "    WAF tip:       allow /api/v1/payment/notify/* (payment callbacks)"
  if [[ "$SCHEME" != "https" ]]; then
    echo -e "    ${YELLOW}HTTPS not active yet — install Origin Certificate then re-run installer or enable 443 site.${NC}"
    print_cf_origin_cert_help
  fi
  echo ""
elif [[ "$CF_DETECT" != "none" ]]; then
  echo -e "  ${YELLOW}Domain looks Cloudflare-related but Real IP is disabled.${NC}"
  echo -e "  Re-run with ${BOLD}--cloudflare${NC} if you enable orange-cloud proxy."
  echo ""
fi
if [[ -n "$PANEL_HINT" ]]; then
  echo -e "  ${DIM}$PANEL_HINT${NC}"
  echo ""
fi
echo -e "  Install dir: ${INSTALL_DIR}"
echo -e "  Config:      ${INSTALL_DIR}/config.yml"
echo -e "  Secrets:     ${INSTALL_DIR}/.env"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo ""
