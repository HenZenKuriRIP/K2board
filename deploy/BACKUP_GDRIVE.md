# K2Board 全库备份到 Google Drive（每 4 小时）

备份对象：**整个 PostgreSQL 数据库 `k2board`**（用户、节点、套餐、订单、设置、流量等全部表），不是只备份用户表。

工具：**pg_dump + gzip + rclone → Google Drive**

---

## 一、你需要准备什么（Google 侧）

### 1. 一个 Google 账号

- 个人 Gmail 即可（或 Workspace）
- 建议单独建「备份专用」账号，或主账号下专用文件夹
- 确认 Drive 剩余空间 ≥ 备份体积 × 保留份数（例如单份 50MB～几 GB 不等）

### 2. 是否需要「Google Cloud 项目」？

| 方式 | 是否需要 Cloud 项目 | 推荐场景 |
|------|---------------------|----------|
| **rclone 交互授权（推荐入门）** | **不强制**先建复杂项目；按 rclone 提示浏览器登录即可 | 单机面板、管理员本人操作 |
| 服务账号 JSON | 要建 GCP 项目、启用 Drive API、共享文件夹给 SA | 多机、无人值守、企业 |

下面按 **rclone + 浏览器登录（最常见）** 写。

### 3. 准备清单（动手前打勾）

- [ ] Google 账号能登录 [drive.google.com](https://drive.google.com)
- [ ] 服务器能访问外网（`curl -I https://www.google.com`）
- [ ] 服务器是 root 或有 sudo
- [ ] 知道 K2 配置路径，一般是 `/opt/k2board/config.yml`
- [ ] 确认库是 PostgreSQL（安装脚本默认）
- [ ] （可选）想加密备份：准备一串 **长口令** 作 GPG 密码，离线记在密码管理器

---

## 二、服务器前置安装

SSH 登录面板机：

```bash
# Debian / Ubuntu
apt-get update
apt-get install -y postgresql-client gzip curl unzip

# 安装 rclone（官方脚本）
curl https://rclone.org/install.sh | bash
rclone version
```

确认 `pg_dump` 可用：

```bash
which pg_dump
pg_dump --version
```

---

## 三、配置 rclone 连接 Google Drive（重要）

在**服务器**上执行（需要能弹出链接，你在**自己电脑浏览器**完成登录）：

```bash
rclone config
```

按提示大致选择：

1. `n` → 新建 remote  
2. 名字输入：`gdrive`（后面脚本默认用这个名字）  
3. 存储类型：找到 **Google Drive**（列表里数字选项，以当前 rclone 提示为准）  
4. `client_id` / `client_secret`：  
   - **直接回车留空** → 使用 rclone 默认客户端（个人备份够用）  
   - 若 Google 限制默认客户端，再按 [rclone Google Drive 文档](https://rclone.org/drive/) 自建 OAuth 客户端  
5. `scope`：建议选 **完整 Drive 访问**（选项说明里类似 “Full access”），以便写入备份目录  
6. `root_folder_id`：回车默认  
7. `service_account_file`：回车（不用 SA）  
8. `Edit advanced config?` → `n`  
9. `Use auto config?`  
   - 服务器无桌面时选 **`n`**（headless）  
   - rclone 会给出一个链接 → **复制到你自己电脑浏览器**打开  
   - 用 Google 账号登录并允许访问  
   - 把浏览器得到的 **验证码** 粘贴回服务器  
10. `Configure as a Shared Drive?` → 一般 `n`  
11. 确认保存 `y`，退出 `q`

### 测试是否通

```bash
# 列出网盘根目录
rclone lsd gdrive:

# 创建备份目录
rclone mkdir gdrive:K2board-Backups

# 上传一个测试文件
echo "k2board backup test $(date)" > /tmp/k2-backup-test.txt
rclone copy /tmp/k2-backup-test.txt gdrive:K2board-Backups/
rclone ls gdrive:K2board-Backups/
```

在浏览器打开 Google Drive，应能看到文件夹 **K2board-Backups** 和测试文件。

### 凭证保存在哪（勿泄露、勿提交 git）

```text
/root/.config/rclone/rclone.conf
```

权限建议：

```bash
chmod 600 /root/.config/rclone/rclone.conf
```

---

## 四、确认数据库连接信息

查看（不要把密码发到公开地方）：

```bash
grep -A3 '^database:' /opt/k2board/config.yml
```

常见类似：

```yaml
database:
  driver: "postgres"
  dsn: "host=localhost user=k2board password=xxxx dbname=k2board port=5432 ..."
```

手动试一次导出（密码用 DSN 里的）：

```bash
export PGPASSWORD='你的密码'
pg_dump -h 127.0.0.1 -U k2board -d k2board --format=custom -f /tmp/test.dump
ls -lh /tmp/test.dump
rm -f /tmp/test.dump
```

成功即说明备份账号权限够用。

---

## 五、安装备份脚本

把仓库里的脚本拷到服务器（或手动创建同内容）：

```bash
mkdir -p /opt/k2board/scripts
# 若已 scp 项目：
# cp deploy/backup-k2board-gdrive.sh /opt/k2board/scripts/
chmod 700 /opt/k2board/scripts/backup-k2board-gdrive.sh
```

脚本默认：

| 项 | 默认值 |
|----|--------|
| 配置 | `/opt/k2board/config.yml` |
| 本地备份目录 | `/var/backups/k2board` |
| 本地保留 | 最近 6 份 |
| rclone remote | `gdrive` |
| 网盘目录 | `K2board-Backups` |

手动跑一次：

```bash
/opt/k2board/scripts/backup-k2board-gdrive.sh
```

成功日志类似：

```text
开始全库备份...
本地备份完成: /var/backups/k2board/k2board_full_YYYYMMDD_HHMMSS.dump.gz
上传到 gdrive:K2board-Backups/
全部完成
```

检查：

```bash
ls -lh /var/backups/k2board/
rclone ls gdrive:K2board-Backups/
```

---

## 六、每 4 小时自动执行（cron）

```bash
crontab -e
```

加入（root）：

```cron
# K2Board 全库备份 → Google Drive，每 4 小时
0 */4 * * * /opt/k2board/scripts/backup-k2board-gdrive.sh >> /var/log/k2board-backup.log 2>&1
```

含义：`00:00、04:00、08:00、12:00、16:00、20:00` 各一次。

查看日志：

```bash
tail -50 /var/log/k2board-backup.log
```

---

## 七、（强烈建议）加密后再上传

备份含邮箱、密码哈希、订阅 token 等，建议加密。

安装：

```bash
apt-get install -y gnupg
```

在 cron 或手动前设置环境变量（示例，请换成自己的长密码）：

```bash
# /etc/k2board-backup.env  （chmod 600）
BACKUP_GPG_PASSPHRASE='请换成很长的随机密码'
```

cron：

```cron
0 */4 * * * set -a; . /etc/k2board-backup.env; set +a; /opt/k2board/scripts/backup-k2board-gdrive.sh >> /var/log/k2board-backup.log 2>&1
```

生成文件将是 `*.dump.gz.gpg`，网盘上是密文。

---

## 八、恢复（了解即可）

```bash
# 从网盘拉回
rclone copy gdrive:K2board-Backups/k2board_full_XXXX.dump.gz /tmp/

# 若加密
# gpg -d /tmp/k2board_full_XXXX.dump.gz.gpg > /tmp/k2board_full_XXXX.dump.gz
gunzip -c /tmp/k2board_full_XXXX.dump.gz > /tmp/k2board_full_XXXX.dump

# 恢复到库（会覆盖同名对象，生产务必先停服务、先再备份）
systemctl stop k2board
export PGPASSWORD='...'
# 危险操作：按需使用 --clean 等，建议先恢复到新库测试
pg_restore -h 127.0.0.1 -U k2board -d k2board --no-owner --no-acl /tmp/k2board_full_XXXX.dump
systemctl start k2board
```

---

## 九、常见问题

| 现象 | 处理 |
|------|------|
| rclone 授权失败 | 用 headless 模式，本机浏览器拿 token；或自建 Google OAuth 客户端 |
| pg_dump 认证失败 | 检查 config.yml 密码；或配置 `~/.pgpass` |
| 上传很慢 | 正常；可只保留 gzip，或换时段 |
| Drive 空间满 | 删旧备份：`rclone delete gdrive:K2board-Backups/ --min-age 30d` |
| cron 没跑 | `grep CRON /var/log/syslog`；确认路径与可执行权限 |

---

## 十、步骤总览（最短路径）

1. Google 账号能开 Drive  
2. 服务器安装 `postgresql-client`、`rclone`  
3. `rclone config` 建 `gdrive` 并测通  
4. 安装 `backup-k2board-gdrive.sh` 并手动跑通  
5. cron：`0 */4 * * *`  
6. （建议）GPG 加密环境变量  

完成后：**每 4 小时整库快照 → 本地 + Google 网盘 `K2board-Backups/`**。
