# K2Board Deployment Guide

## One-click (recommended)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh)
```

The script is interactive (domain, database, admin email/password), prints step progress (`[1/10]…[10/10]`), and is **safe to re-run**: existing `config.yml` / `.env` are kept; database roles are created/updated without noisy “already exists” failures.

### Cloudflare (optional)

When the panel domain is on Cloudflare (especially **orange-cloud proxy**):

1. Installer **detects** CF (NS / A in CF ranges) and defaults **Cloudflare Real IP** on.
2. You can force the choice:
   - `--cloudflare` — enable Real IP (`/etc/nginx/conf.d/cloudflare-realip.conf`)
   - `--no-cloudflare` — disable
   - `--cloudflare=auto` — detect (default interactive path)
3. **TLS (preferred when CF is on):**
   - Paste a **Cloudflare API Token** → installer generates a local ECC key + CSR, calls Origin CA API, writes certs, and best-effort sets SSL mode **Full (strict)**.
   - Token is **not** written to disk (memory only).
   - **Enter to skip token** → fallback: manual Origin files / Let's Encrypt standalone / skip HTTPS.
4. Token permissions (minimum):
   - `Zone → SSL and Certificates → Edit`
   - `Zone → Zone → Read` (resolve zone + set Full strict)
   - Zone Resources: the zone that owns the panel domain

Non-interactive examples:

```bash
# Orange cloud: Real IP + Origin CA via API Token
CF_API_TOKEN='your-token' bash deploy/install.sh panel.example.com 1 admin@example.com 'YourPass' --cloudflare

# Same via flag
bash deploy/install.sh panel.example.com 1 --cloudflare --cf-token='your-token'

# Direct DNS, no CF
bash deploy/install.sh panel.example.com 1 --no-cloudflare
```

Origin cert paths used by Nginx:

- `/etc/nginx/ssl/fullchain.pem`
- `/etc/nginx/ssl/privkey.pem`

**Do not use Flexible SSL** with this stack. Payment notify and subscribe stay on the same domain; Real IP only affects rate-limit/logging accuracy, not payment signature verification.

### Uninstall / reinstall

```bash
# Default uninstall
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install.sh) --uninstall
```

| Mode | Behavior |
|------|----------|
| **Cloudflare install detected** (Real IP conf / `/etc/k2board/cloudflare.meta`) | **Keeps** nginx `k2board` site, `cloudflare-realip.conf`, and `/etc/nginx/ssl/*` so reinstall can reuse Origin certs without re-pasting API Token |
| **Non-CF install** | Removes nginx k2board site; keeps SSL files unless purged |
| `--keep-nginx` | Force keep proxy + Real IP + TLS |
| `--remove-nginx` | Force remove site + Real IP (TLS files still kept) |
| `--purge` | Remove site + Real IP + TLS files under `/etc/nginx/ssl` |

App files (`/opt/k2board`, systemd unit) are always removed. DB/Redis packages and data are never removed by the script.

Reinstall after CF uninstall (nginx kept):

```bash
# Origin certs still on disk → installer reuses them
bash deploy/install.sh panel.example.com 1 --cloudflare
# optional: CF_API_TOKEN=... only if you need to re-issue certs
```

---

## Manual / development

```bash
# Terminal 1 — API
cp config.yml.example config.yml   # or edit config.yml
go run ./cmd/server

# Terminal 2 — Admin UI (Vite proxies /api → :8080)
cd web && npm install && npm run dev

# Terminal 3 — User portal (optional)
cd web_user && npm install && npm run dev
```

Production single binary (admin UI embedded):

```bash
cd web && npm run build && cd ..
go build -ldflags="-s -w" -o k2board ./cmd/server
./k2board
```

---

## Configuration

### `config.yml`

```yaml
server:
  host: "127.0.0.1"          # installer binds localhost; Nginx terminates TLS
  port: 8080
  mode: "release"            # or debug
  node_rate_limit: 50        # per IP / 5 min (0 = off)

database:
  driver: "postgres"         # postgres | mysql
  dsn: "host=localhost user=k2board password=SECRET dbname=k2board port=5432 sslmode=disable TimeZone=UTC"
  # MySQL:
  # driver: "mysql"
  # dsn: "k2board:SECRET@tcp(127.0.0.1:3306)/k2board?charset=utf8mb4&parseTime=true&loc=Local"

redis:
  enabled: true
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
  email: "admin@k2board.com"
```

### `.env` (secrets, not committed)

```bash
jwt.secret=<long-random>
admin.password=<initial-or-rotated-password>
```

On startup, admin password is synced from `.env` when the hash marker changes.

---

## Nginx sketch

- `/api/` → Go `:8080` (must win over catch-all)
- `/<secret>/` → rewrite strip → Go (admin SPA + assets via embed)
- `/` → user portal static root
- `/assets/` → user dist first, then `@go` fallback for admin hashed assets

See `deploy/k2board.nginx.conf` and the templates inside `deploy/install.sh`.

---

## systemd

Installer writes `/etc/systemd/system/k2board.service` with:

- `WorkingDirectory=/opt/k2board`
- `User=k2board`
- `Restart=always`

```bash
systemctl status k2board
journalctl -u k2board -f
```

---

## Docker (optional)

```bash
cd deploy
docker compose up -d
```

Adjust `Dockerfile` / compose env for your DB and secrets. Prefer the host installer for production Nginx + TLS.

---

## Post-install checklist

1. Open **admin URL** printed by the installer (secret path).
2. Log in with admin email / password.
3. **Settings → Panel Token** — copy for XrayR4u `ApiKey`.
4. Create **groups / plans / nodes** and assign mappings.
5. To allow self-registration: enable **Open registration** and configure **SMTP**, then “Send test”.
6. Point XrayR4u `ApiHost` at the public origin (no path).

---

## Releases

Binaries: [GitHub Releases](https://github.com/HenZenKuriRIP/K2board/releases)

| Asset | Use |
|-------|-----|
| `k2board-linux-amd64` | x86_64 server |
| `k2board-linux-arm64` | ARM64 server |
| `k2board-user-dist.tar.gz` | User portal static files |

---

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Admin 404 | Use full secret path with trailing slash |
| API 302/HTML | Nginx missing `location /api/` |
| Login fails | `journalctl -u k2board`; verify `.env` `admin.password` |
| DB errors on reinstall | Installer keeps old DSN; drop DB only if you intend a wipe |
| Registration “mail not configured” | Settings → SMTP host/user/pass; test email first |
| Node offline | Panel token, `node_id`, firewall to `/api/v1/server/` |

More design detail: [ARCHITECTURE.md](ARCHITECTURE.md).
