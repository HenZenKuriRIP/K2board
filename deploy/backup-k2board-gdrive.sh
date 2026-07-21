#!/usr/bin/env bash
# =============================================================================
# K2Board 全库备份 → 本地 → Google Drive (rclone)
#
# 备份内容: 整个 PostgreSQL 库 k2board（用户/节点/套餐/订单/设置等全部表）
# 推荐 cron: 每 4 小时
#   0 */4 * * * /opt/k2board/scripts/backup-k2board-gdrive.sh >> /var/log/k2board-backup.log 2>&1
#
# 依赖: pg_dump, gzip, rclone（已配置 gdrive remote）
# =============================================================================
set -euo pipefail

# ---------- 可按环境修改 ----------
CONFIG_YML="${CONFIG_YML:-/opt/k2board/config.yml}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/k2board}"
KEEP_LOCAL="${KEEP_LOCAL:-6}"              # 本地保留份数
RCLONE_REMOTE="${RCLONE_REMOTE:-gdrive}"   # rclone config 里的名字
RCLONE_PATH="${RCLONE_PATH:-K2board-Backups}"  # 网盘目录
# 可选：GPG 对称加密（设置后备份为 .gpg，上传密文）
# export BACKUP_GPG_PASSPHRASE='你的长密码'
# --------------------------------

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

need() {
  command -v "$1" >/dev/null 2>&1 || { log "ERROR: 缺少命令 $1"; exit 1; }
}

need pg_dump
need gzip
need rclone

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

# 从 config.yml 解析 postgres DSN（简单提取，适配安装脚本生成的格式）
parse_dsn() {
  local dsn
  dsn="$(grep -E '^\s*dsn:' "$CONFIG_YML" 2>/dev/null | head -1 | sed 's/.*dsn:[[:space:]]*"\{0,1\}//;s/"\{0,1\}[[:space:]]*$//')"
  if [[ -z "$dsn" ]]; then
    log "ERROR: 无法从 $CONFIG_YML 读取 database.dsn"
    exit 1
  fi
  # host=... user=... password=... dbname=... port=...
  export PGHOST="$(echo "$dsn" | sed -n 's/.*host=\([^ ]*\).*/\1/p')"
  export PGUSER="$(echo "$dsn" | sed -n 's/.*user=\([^ ]*\).*/\1/p')"
  export PGPASSWORD="$(echo "$dsn" | sed -n 's/.*password=\([^ ]*\).*/\1/p')"
  export PGDATABASE="$(echo "$dsn" | sed -n 's/.*dbname=\([^ ]*\).*/\1/p')"
  export PGPORT="$(echo "$dsn" | sed -n 's/.*port=\([^ ]*\).*/\1/p')"
  PGHOST="${PGHOST:-127.0.0.1}"
  PGPORT="${PGPORT:-5432}"
  PGDATABASE="${PGDATABASE:-k2board}"
  if [[ -z "${PGUSER:-}" ]]; then
    log "ERROR: DSN 中无 user="
    exit 1
  fi
}

parse_dsn

STAMP="$(date '+%Y%m%d_%H%M%S')"
# 自定义格式 + gzip：体积小、可 pg_restore
OUT_BASE="k2board_full_${STAMP}"
DUMP_FILE="${BACKUP_DIR}/${OUT_BASE}.dump"
GZ_FILE="${DUMP_FILE}.gz"
UPLOAD_FILE="$GZ_FILE"

log "开始全库备份: db=${PGDATABASE} host=${PGHOST} user=${PGUSER}"

pg_dump \
  -h "$PGHOST" \
  -p "$PGPORT" \
  -U "$PGUSER" \
  -d "$PGDATABASE" \
  --format=custom \
  --no-owner \
  --no-acl \
  -f "$DUMP_FILE"

gzip -f -9 "$DUMP_FILE"
log "本地备份完成: $GZ_FILE ($(du -h "$GZ_FILE" | awk '{print $1}'))"

if [[ -n "${BACKUP_GPG_PASSPHRASE:-}" ]]; then
  need gpg
  gpg --batch --yes --pinentry-mode loopback \
    --passphrase "$BACKUP_GPG_PASSPHRASE" \
    -c "$GZ_FILE"
  UPLOAD_FILE="${GZ_FILE}.gpg"
  rm -f "$GZ_FILE"
  log "已 GPG 加密: $UPLOAD_FILE"
fi

# 上传 Google Drive
log "上传到 ${RCLONE_REMOTE}:${RCLONE_PATH}/"
rclone copy "$UPLOAD_FILE" "${RCLONE_REMOTE}:${RCLONE_PATH}/" \
  --retries 3 \
  --low-level-retries 10 \
  --stats 0

log "上传完成"

# 清理本地旧备份
mapfile -t OLD < <(ls -1t "$BACKUP_DIR"/k2board_full_*.dump.gz "$BACKUP_DIR"/k2board_full_*.dump.gz.gpg 2>/dev/null || true)
if ((${#OLD[@]} > KEEP_LOCAL)); then
  for f in "${OLD[@]:KEEP_LOCAL}"; do
    log "删除本地旧备份: $f"
    rm -f "$f"
  done
fi

# 可选：网盘只保留最近 30 个（按修改时间，需 rclone 较新）
# rclone delete "${RCLONE_REMOTE}:${RCLONE_PATH}/" --min-age 60d 2>/dev/null || true

log "全部完成"
exit 0
