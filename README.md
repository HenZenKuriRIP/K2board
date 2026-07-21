# K2Board

Go implementation of a multi-tenant proxy **panel**, inspired by [v2board](https://github.com/v2board/v2board), designed for **XrayR4u UniProxy** backends.

- Single static binary (admin UI embedded)
- PostgreSQL or MySQL
- Optional Redis traffic buffer
- Separate user portal (static SPA)

---

## Quick install (Linux)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh)
```

The installer walks you through domain, database engine, and admin credentials, then sets up:

1. OS packages (nginx, curl, …)
2. PostgreSQL **or** MySQL (idempotent on reinstall)
3. Redis (falls back to memory if unavailable)
4. Latest release binary
5. `config.yml` + `.env` secrets
6. User portal static files
7. Nginx reverse proxy (+ optional TLS via acme.sh)
8. systemd service + health check

**Uninstall:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh) --uninstall
```

- **Cloudflare installs** (Real IP / Origin cert path): default uninstall **keeps** nginx site, `cloudflare-realip.conf`, and `/etc/nginx/ssl/*` for easy reinstall (no need to re-paste API Token if certs remain).
- Full wipe of proxy + certs: add `--purge`. Force keep/remove nginx: `--keep-nginx` / `--remove-nginx`. See [DEPLOY.md](DEPLOY.md#uninstall--reinstall).

Non-interactive example:

```bash
bash install.sh panel.example.com 1 admin@example.com 'YourStrongPass'
# args: domain  db(1=postgres|2=mysql)  admin_email  admin_password
```

---

## Cloudflare + API Token install (optional)

Use this when the panel domain is on **Cloudflare** (especially **orange-cloud / Proxied**). The installer can:

- enable **Cloudflare Real IP** (correct visitor IP for rate limits / logs)
- issue a **Cloudflare Origin CA** certificate via **API Token** (no manual cert paste)

### Prerequisites (do these in Cloudflare **before** running the script)

1. **Add the site to Cloudflare**  
   Dashboard → add domain → set nameservers at your registrar to Cloudflare’s NS.

2. **Create a DNS record for the panel host**  
   e.g. `panel.example.com` → **A/AAAA** to your server public IP.  
   - Turn the cloud **orange (Proxied)** if you want CF in front of the panel.  
   - The installer does **not** create or edit DNS records.

3. **Create an API Token** (recommended; do **not** use Global API Key)  
   Open [API Tokens](https://dash.cloudflare.com/profile/api-tokens) → **Create Token** → **Create Custom Token**:

   | Setting | Value |
   |---------|--------|
   | **Permissions** | `Zone` → `Zone` → **Read** |
   | | `Zone` → `SSL and Certificates` → **Edit** |
   | **Zone Resources** | `Include` → **Specific zone** → the zone that owns the panel domain |
   | **Client IP Filtering** | Optional (e.g. lock to your VPS IP). Skip if the server IP is not fixed. |

   **DNS → Edit is not required** for this project’s installer.

   Copy the token when shown (it is only displayed once).

4. **Account / membership**  
   The Cloudflare user must be allowed to use the API for that zone (API Access enabled for the member if your org restricts it).

5. **SSL mode**  
   Prefer **Full (strict)** after Origin cert is installed. The installer tries to set this via API; you can also set it under **SSL/TLS → Overview**.  
   Do **not** use **Flexible**.

### Run the installer

Interactive (detect CF → enable Real IP → paste Token, or Enter to skip):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh)
```

Non-interactive (Real IP + Origin CA via Token):

```bash
CF_API_TOKEN='your-token' bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh) \
  panel.example.com 1 admin@example.com 'YourStrongPass' --cloudflare

# or:
# bash deploy/install.sh panel.example.com 1 --cloudflare --cf-token='your-token'
```

| Flag / env | Meaning |
|------------|---------|
| `--cloudflare` | Force Cloudflare Real IP on |
| `--no-cloudflare` | Force Real IP off |
| `--cloudflare=auto` | Detect (default interactive) |
| `CF_API_TOKEN` / `--cf-token=` | Issue Origin CA automatically |

- Token is used **in memory only** and is **not** saved to disk.  
- **Enter without a token** → fallback: place certs under `/etc/nginx/ssl/` yourself, or Let's Encrypt, or skip HTTPS.  
- More detail: [DEPLOY.md](DEPLOY.md#cloudflare-optional).

---

## Standalone user portal (split deploy / shadow domains)

Run the **user SPA only** on a separate server or extra domains.  
The **panel API, subscribe URL, payment notify, admin, DB, Redis stay on www**.  
Clients keep existing subscription links (do not change `subscribe_url`).

### One-click on the shadow host

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install-user-portal.sh) \
  user.example.com https://www.example.com
```

| Arg / flag | Meaning |
|------------|---------|
| `user.example.com` | This host’s public domain (shadow entry) |
| `https://www.example.com` | Panel API base (`site_url` / www) |
| `--version=v1.4.27` | Pin release asset (default: latest) |
| `--skip-nginx` | Only extract static files + `config.js` |
| `--cloudflare` | Install Cloudflare Real IP snippet |
| `--uninstall` | Remove site + web root |

The installer:

1. Downloads `k2board-user-dist.tar.gz` from GitHub Releases  
2. Writes `/var/www/k2board-user/config.js` → `window.__K2_API_BASE__ = 'https://www…'`  
3. Installs Nginx from [`deploy/nginx-user-portal.conf`](deploy/nginx-user-portal.conf) (static only; `/api` returns 404 on this host)

### Required on the panel (www)

1. **站点 URL** / **订阅域名** = `https://www.example.com` (pin; never point subscribe at the shadow host)  
2. **允许的用户端域名** add: `https://user.example.com`  
3. Save settings (CORS + payment return_url allow-list)

### Security notes

- Shadow host: **no** PostgreSQL, Redis, panel binary, or admin path  
- Prefer Cloudflare orange-cloud + **Full (strict)** + Origin cert  
- Firewall: 443 public; SSH restricted  
- Verify in browser Network: API host is **www**, not the shadow domain  
- Full guide: [docs/MULTI_ORIGIN.md](docs/MULTI_ORIGIN.md)

### Manual Nginx

```bash
# After placing dist under /var/www/k2board-user and config.js
sed -e 's|__SERVER_NAME__|user.example.com|g' \
    -e 's|__ROOT__|/var/www/k2board-user|g' \
    -e 's|__SSL_CERT__|/etc/nginx/ssl/fullchain.pem|g' \
    -e 's|__SSL_KEY__|/etc/nginx/ssl/privkey.pem|g' \
    deploy/nginx-user-portal.conf \
  > /etc/nginx/sites-available/k2board-user
ln -sfn /etc/nginx/sites-available/k2board-user /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

---

## Features

| Area | Highlights |
|------|------------|
| **Admin** | Dashboard, users, nodes, groups, plans, traffic, queue metrics, audit, settings |
| **Node API** | XrayR4u-compatible UniProxy (`/config`, `/user`, `/push`, `/alive`, `/info`, …) |
| **Access control** | Multi-group node mapping; subscription visibility matches node user pull |
| **Traffic** | Buffered flush per `(user, node)`; monthly plan-based reset |
| **Devices** | Online IP aggregation; `device_limit` exposed / enforced on pull |
| **Subscribe** | V2Ray / Clash / Surge / Shadowrocket |
| **Mail** | SMTP (587/465/25) for user registration codes + admin test mail |
| **Security** | JWT admin auth, panel token (SHA-256), rate limits, secret admin URL path |

---

## Architecture (short)

```
XrayR4u  ──►  /api/v1/server/UniProxy/*   (token + node_id)
Browsers ──►  /api/v1/admin/*             (JWT)
Clients  ──►  /api/v1/client/subscribe
Users    ──►  /api/v1/user/*              (register/login/profile)
```

Full design notes: [ARCHITECTURE.md](ARCHITECTURE.md)  
Deploy & config details: [DEPLOY.md](DEPLOY.md)

---

## Development

```bash
# Backend
go run ./cmd/server

# Admin UI (proxies /api → :8080)
cd web && npm install && npm run dev

# User portal
cd web_user && npm install && npm run dev
```

Production binary (embed admin UI):

```bash
cd web && npm run build && cd ..
go build -o k2board ./cmd/server
```

Cross-compile release artifacts:

```bash
./build_release.sh v1.x.x
```

---

## Configuration

| File | Purpose |
|------|---------|
| `config.yml` | Server, DB, Redis, scheduler |
| `.env` | `jwt.secret`, `admin.password` (not committed) |

Default listen (installer): `127.0.0.1:8080` behind Nginx.

---

## XrayR4u

```yaml
PanelType: "V2board"
ApiConfig:
  ApiHost: "https://your-domain"
  ApiKey:  "<Panel Token from Admin → Settings>"
  NodeID:  1
  NodeType: "Vless"   # or AnyTLS
```

---

## License

MIT
