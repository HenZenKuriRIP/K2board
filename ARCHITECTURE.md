# K2Board Architecture

## Overview

K2Board is a **Go** rewrite of a v2board-style control plane for proxy fleets. It exposes an **XrayR4u UniProxy-compatible** server API, embeds a Vue 3 admin console, and optionally serves a separate user portal.

| Layer | Stack |
|-------|--------|
| API | Go, Gin, GORM |
| Admin UI | Vue 3, Element Plus, TypeScript (embedded via `//go:embed`) |
| User UI | Vue 3 SPA (static files, e.g. `/var/www/k2board-user`) |
| Data | PostgreSQL or MySQL |
| Buffer | In-memory or Redis |

---

## Repository layout

```
K2board/
├── cmd/server/main.go       # process entry
├── frontend.go              # embed web/dist
├── config.yml               # runtime config sample
├── build_release.sh         # multi-arch static builds
├── deploy/
│   ├── install.sh           # one-click installer
│   ├── k2board.nginx.conf
│   ├── k2board.service
│   ├── Dockerfile
│   └── docker-compose.yml
├── internal/
│   ├── config/              # Viper + .env secrets
│   ├── database/            # GORM init, migrate, seeds
│   ├── models/              # User, Node, Group, Plan, traffic, …
│   ├── handlers/
│   │   ├── admin/           # JWT admin API
│   │   ├── server/          # UniProxy node API
│   │   ├── client/          # subscription export
│   │   └── user/            # user register/login/profile
│   ├── services/            # business logic
│   ├── queue/               # traffic buffer + scheduler
│   ├── middleware/          # JWT, node auth, CORS, limits
│   ├── router/
│   └── utils/               # JWT, bcrypt, SMTP, response helpers
├── web/                     # admin frontend
└── web_user/                # user portal frontend
```

---

## Core domain model

```
Group (enable)  ←──many-to-many──→  Node (enable, protocol, TLS/REALITY, …)
   │                                    │
   │ group_id                           │ node_group_mappings
   ▼                                    ▼
 Plan (limits, duration, reset_day)   Node online / traffic logs
   │
   │ plan_id / group_id
   ▼
 User (uuid, token, traffic_used/limit, device_limit, expire_at)
```

### Access rules (subscription ↔ node user list) — **strict**

| Node / user | Who appears in `/UniProxy/user` | Who sees node in subscribe |
|-------------|----------------------------------|----------------------------|
| **Unmapped** (no `node_group_mappings`) | Nobody | Nobody |
| **Grouped** | Users in enabled mapped groups | Same (enabled group only) |
| User `group_id = 0` or disabled group | Never (empty list) | No nodes |

`device_limit > 0`: users currently over the limit (distinct IPs in online window) are omitted from node user pull; API also returns `device_limit` for backends that enforce locally.

---

## Node authentication

Query: `?token=<plaintext>&node_id=<id>`

1. Match **panel token** (SHA-256 of plaintext vs `settings.panel_token`)
2. Else match **per-node** token row
3. Reject disabled / missing nodes; update heartbeat

---

## Traffic pipeline

```
Node POST /UniProxy/push
        │
        ▼
 DefaultStore.Add(userID, nodeID, up, down)   # key = (user, node)
        │
        │  every flush_interval (default 60s)
        ▼
 traffic_logs  +  users.traffic_used += up+down
        │
        │  every stats_interval
        ▼
 daily rollups: stat_servers / stat_users
```

Failed flushes re-queue (memory) or restore Redis keys so counters are not dropped silently.

---

## Background scheduler

| Job | Default | Config key |
|-----|---------|------------|
| Traffic flush | 60s | `scheduler.flush_interval` |
| Daily aggregation | 300s | `scheduler.stats_interval` |
| Expire users + purge online + refresh `config_version` | 60s | `scheduler.auto_disable_interval` |
| Monthly traffic reset by `users.plan_id` / `plans.reset_day` | 1h | fixed ticker |

`config_version` is stored in `settings` so multi-instance panels can converge; nodes read it from `/info`.

---

## HTTP API map

### Admin — `/api/v1/admin` (Bearer JWT, admin only)

| Methods | Path | Purpose |
|---------|------|---------|
| POST | `/login` | Admin login |
| GET | `/dashboard`, `/dashboard/trend` | Stats |
| CRUD | `/users`, `/nodes`, `/groups`, `/plans` | Resources |
| POST | `/users/:id/reset-{uuid,token,traffic}` | Resets |
| GET | `/traffic-logs`, `/traffic-stats` | Traffic |
| GET | `/queue/stats` | Buffer / scheduler metrics |
| GET/PUT | `/settings` | Site, panel token, SMTP, registration |
| POST | `/settings/test-email` | SMTP probe (saved or form override) |

Admin UI is often exposed only under a **secret Nginx prefix** (e.g. `/a1b2c3d4/`); the Go app always sees `/api/v1/...` and `/` assets.

### Node — `/api/v1/server/UniProxy/*` (token + node_id)

`config`, `user`, `push`, `alive`, `status`/`info`, `rule`, `illegal`.

### Client — `/api/v1/client`

`GET /subscribe?token=&flag=v2ray|clash|surge|shadowrocket`

### User — `/api/v1/user`

`send-code`, `register`, `login`, `info`, `plans`, `change-password`  
Registration requires `allow_register=true` and working SMTP settings.

---

## Mail & registration

```
Admin Settings → smtp_* + allow_register
        │
User POST /send-code → SMTP send → store 6-digit code (memory, 10m TTL)
        │
User POST /register → verify code → CreateUser (email lowercased)
```

SMTP: port **465** = implicit TLS; **587** = STARTTLS required; **25** = STARTTLS if advertised.

---

## Security notes

- Admin password and JWT secret live in `.env` (file mode 600).
- Panel token stored as SHA-256 hex; plaintext shown once at generation.
- Login / node / subscribe rate limits are in-process (per instance).
- Verification codes are **in-memory** (not shared across multi-instance without sticky sessions or future Redis store).

---

## Related docs

- [README.md](README.md) — install & feature overview  
- [DEPLOY.md](DEPLOY.md) — config reference & ops  
