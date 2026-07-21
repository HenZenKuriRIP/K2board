# 用户端 / 管理端拆分 + PostgreSQL 迁到新机

目标架构（`example.com` 示例，按实际替换）：

| 角色 | 域名 | 机器 | 运行内容 |
|------|------|------|----------|
| 用户端 | `www.example.com` | **旧机 A** | Nginx + 用户静态 + `/downloads/` + **反代 `/api/` → B** |
| 管理端 + API + 库 | `admin.example.com` | **新机 B** | Nginx + k2board + PostgreSQL + Redis |
| 证书 | 通配 Origin | A/B 共用 | `*.example.com` + `example.com` |

**原则：**

1. 用户订阅 URL、节点 UniProxy、支付回调继续走 **www + `/api/`**（A 反代到 B），尽量不改客户端配置。  
2. `site_url` / `subscribe_url` **保持** `https://www.example.com`（先不要改成 admin）。  
3. **同一时刻只允许一台机器跑 k2board**（迁库切换窗口后只留 B）。  
4. JWT / 管理员配置从 A **原样复制**，勿重新随机生成。

维护窗口预估：迁库 + 切换约 **15～40 分钟**（库越大越久）。

---

## 阶段 0 — 旧机 A 备份与信息收集

在 **A** 上执行，全部保存到安全处：

```bash
# 服务状态
systemctl status k2board postgresql redis-server nginx --no-pager | head -40

# 管理路径
cat /opt/k2board/.admin_path 2>/dev/null

# 配置（勿外传）
cp -a /opt/k2board/config.yml /root/k2board-config-backup.yml
cp -a /opt/k2board/.env /root/k2board-env-backup 2>/dev/null || true

# 全库备份（custom 格式）
export PGPASSWORD='从config.yml的dsn里取'
pg_dump -h 127.0.0.1 -U k2board -d k2board --format=custom \
  -f /root/k2board_pre_migrate_$(date +%Y%m%d_%H%M%S).dump
ls -lh /root/k2board_pre_migrate_*.dump

# 证书（通配已替换则备份现用）
cp -a /etc/nginx/ssl /root/ssl-backup-pre-split
```

记下：

- A/B 公网 IP、是否同机房内网 IP  
- DB 用户/密码/库名（一般 `k2board`）  
- `.admin_path`（如 `44f67dd0`）  
- 管理后台里的 `site_url`、`subscribe_url`

**Cloudflare DNS（开始前可先加）：**

| 名称 | 类型 | 内容 | 代理 |
|------|------|------|------|
| `www` | A | **A 公网 IP** | 橙云 |
| `admin` | A | **B 公网 IP** | 橙云 |

---

## 阶段 1 — 新机 B：基础环境（可与业务并行，先不切流量）

### 1.1 软件

```bash
# Debian/Ubuntu
apt-get update
apt-get install -y nginx curl ca-certificates redis-server \
  postgresql postgresql-contrib postgresql-client
systemctl enable --now redis-server postgresql
```

### 1.2 证书（复用通配）

把 A 上正在用的通配证书拷到 B：

```bash
# 在 A 上
scp /etc/nginx/ssl/fullchain.pem /etc/nginx/ssl/privkey.pem root@B公网IP:/root/

# 在 B 上
mkdir -p /etc/nginx/ssl
cp /root/fullchain.pem /etc/nginx/ssl/fullchain.pem
cp /root/privkey.pem  /etc/nginx/ssl/privkey.pem
chmod 600 /etc/nginx/ssl/privkey.pem
chmod 644 /etc/nginx/ssl/fullchain.pem
```

### 1.3 创建空库用户（密码建议与 A 相同，省事）

```bash
# B 上，把 PASS 换成与 A 相同的数据库密码
sudo -u postgres psql <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'k2board') THEN
    CREATE USER k2board WITH PASSWORD 'PASS';
  ELSE
    ALTER USER k2board WITH PASSWORD 'PASS';
  END IF;
END
$$;
SELECT 'ok' FROM pg_database WHERE datname = 'k2board';
SQL

# 若库不存在再创建
sudo -u postgres psql -c "CREATE DATABASE k2board OWNER k2board;" 2>/dev/null || true
sudo -u postgres psql -d k2board -c "GRANT ALL ON SCHEMA public TO k2board;"
```

**此时先不要 restore**，等维护窗口。

### 1.4 部署 k2board 二进制与配置

```bash
mkdir -p /opt/k2board
# 上传与现网一致或更新后的 k2board 二进制
# scp k2board-linux-amd64 root@B:/opt/k2board/k2board
chmod +x /opt/k2board/k2board
useradd -r -s /usr/sbin/nologin k2board 2>/dev/null || true
chown -R k2board:k2board /opt/k2board
```

从 A 复制配置后改 DSN 为本地：

```bash
# 在 A
scp /opt/k2board/config.yml /opt/k2board/.env root@B:/opt/k2board/ 2>/dev/null
scp /opt/k2board/.admin_path root@B:/opt/k2board/ 2>/dev/null
```

编辑 B 的 `/opt/k2board/config.yml`：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  driver: "postgres"
  dsn: "host=127.0.0.1 user=k2board password=原密码 dbname=k2board port=5432 sslmode=disable TimeZone=Asia/Shanghai"

redis:
  enabled: true
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

# jwt / admin 等其余字段保持与 A 完全一致，不要重新生成密钥
```

systemd（可参考仓库 `deploy/k2board.service`）：

```bash
cat > /etc/systemd/system/k2board.service <<'EOF'
[Unit]
Description=K2Board
After=network.target postgresql.service redis-server.service

[Service]
Type=simple
User=k2board
WorkingDirectory=/opt/k2board
ExecStart=/opt/k2board/k2board
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
# 等库 restore 完成后再 start
```

### 1.5 B 的 Nginx

使用仓库模板：

- `deploy/nginx-admin.example.conf`

把其中 `ADMIN_PATH` 换成 A 上 `.admin_path` 内容，启用：

```bash
# 示例
cp nginx-admin.example.conf /etc/nginx/sites-available/k2board-admin
# 编辑 server_name、ADMIN_PATH
ln -sf /etc/nginx/sites-available/k2board-admin /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx
```

防火墙：开放 80/443；**不要**对公网开 5432。

---

## 阶段 2 — 维护窗口：迁库并只在 B 启动 API

### 2.1 冻结写入（A）

```bash
# A：停止面板，避免迁库期间有新订单/流量写入
systemctl stop k2board
```

可选：临时在 A 的 nginx 给 `/api/` 返回 503 维护页（非必须）。

### 2.2 最终 dump（A）→ 传到 B

```bash
# A
export PGPASSWORD='...'
pg_dump -h 127.0.0.1 -U k2board -d k2board --format=custom \
  -f /root/k2board_final.dump
scp /root/k2board_final.dump root@B:/root/
```

### 2.3 restore（B）

```bash
# B
export PGPASSWORD='...'
# 清空目标库后恢复（新库应为空或仅有空库）
sudo -u postgres psql -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='k2board' AND pid <> pg_backend_pid();" 2>/dev/null || true
# 使用 pg_restore；--clean 在空库上可忽略错误
pg_restore -h 127.0.0.1 -U k2board -d k2board --no-owner --no-acl \
  /root/k2board_final.dump
# 若权限报错可：
# sudo -u postgres pg_restore -d k2board --no-owner --role=k2board /root/k2board_final.dump

# 抽查
export PGPASSWORD='...'
psql -h 127.0.0.1 -U k2board -d k2board -c 'SELECT count(*) AS users FROM users;'
psql -h 127.0.0.1 -U k2board -d k2board -c "SELECT key, left(value,40) FROM settings WHERE key IN ('site_url','subscribe_url');"
```

确认 `site_url` / `subscribe_url` 仍是 `https://www.example.com`（或你原来的值）。

### 2.4 启动 B 上的 k2board

```bash
# B
chown -R k2board:k2board /opt/k2board
systemctl enable k2board
systemctl start k2board
systemctl status k2board --no-pager
curl -sS http://127.0.0.1:8080/api/v1/user/plans | head -c 300; echo
```

浏览器测：

- `https://admin.example.com/<ADMIN_PATH>/` 能登录  
- 用户数量与迁库前一致  

**此时 A 的 k2board 仍保持 stop。**

---

## 阶段 3 — 旧机 A：只留用户前端 + 反代 API

### 3.1 改 Nginx（A）

使用模板 `deploy/nginx-www.user-only.conf`：

- `server_name www.example.com;`  
- 证书：现有通配  
- `/` + `/assets/` → `/var/www/k2board-user`  
- `/downloads/` → `/var/www/k2board-downloads`  
- `/api/` → 反代到 B（见模板两种方式）  
- **删除** 管理路径 `location /ADMIN_PATH/`  

推荐反代方式（内网优先）：

```nginx
location /api/ {
    proxy_pass http://B内网IP:8080;   # 或 B 公网 IP:8080（需 B 防火墙仅允许 A）
    proxy_http_version 1.1;
    proxy_set_header Host $host;      # 保持 www，利于日志
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header Connection "";
    proxy_connect_timeout 10s;
    proxy_read_timeout 60s;
}
```

若无内网、B 的 8080 不对外开放，可反代 HTTPS：

```nginx
location /api/ {
    proxy_pass https://admin.example.com;
    proxy_ssl_server_name on;
    proxy_set_header Host admin.example.com;
    ...
}
```

```bash
nginx -t && systemctl reload nginx
```

### 3.2 确认 A 上 k2board 不会再起

```bash
systemctl disable k2board
systemctl stop k2board
# 保留 /opt/k2board 与旧库 1～2 周作回滚，勿删
```

### 3.3 验证清单

| 检查 | 命令/操作 | 期望 |
|------|-----------|------|
| 用户首页 | 浏览器 `https://www.example.com/` | 正常 |
| API 经 www | `curl -sS https://www.example.com/api/v1/user/plans` | JSON |
| 用户登录/套餐/订单 | 实机操作 | 正常 |
| 订阅更新 | 客户端更新订阅 | 仍可用旧链接 |
| 管理端 | `https://admin.example.com/<path>/` | 正常 |
| 节点 | 看在线/流量 | UniProxy 正常 |
| 支付 | 小额或沙箱 | 回调成功 |
| 双实例 | A 上 `systemctl is-active k2board` | `inactive` |

---

## 阶段 4 — 旧机 Postgres（先停服务，勿删数据）

观察 **24～72 小时** 无问题后：

```bash
# A — 仅停止，不 dropdb、不删数据目录
systemctl stop postgresql
systemctl disable postgresql
```

确认 A 磁盘上数据目录仍在，紧急时可再拉起做只读核对。

备份任务：把 `pg_dump`/rclone cron **改到 B** 执行。

---

## 回滚（切流量后发现问题）

1. A：`systemctl start postgresql`（若已停）  
2. A：Nginx `/api/` 改回 `proxy_pass http://127.0.0.1:8080`  
3. A：`systemctl start k2board`  
4. B：`systemctl stop k2board`（避免双写）  
5. `nginx reload`  

说明：若切换后 B 上已有**新写入**，回滚到 A 旧库会丢那一段数据；回滚前先在 B 再 dump 一份。

---

## 不要做的事

- 两台同时 `systemctl start k2board` 连不同库或误连同一逻辑业务  
- 未验证就 `dropdb` / 格式化 A  
- 把 `site_url` 改成 admin 域（支付/订阅易坏）  
- 给用户端改 `VITE_API_BASE` 指 admin（需改 CORS；本次用反代不需要）  
- 公网裸开 5432  

---

## 文件一览

| 文件 | 用途 |
|------|------|
| `deploy/nginx-www.user-only.conf` | 旧机 A：仅用户端 + API 反代 |
| `deploy/nginx-admin.example.conf` | 新机 B：管理端 + 本机 API |
| `deploy/MIGRATE_SPLIT.md` | 本文 |
