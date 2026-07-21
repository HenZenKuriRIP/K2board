/**
 * Format bytes to human-readable string. Shows GB with 2 decimals for values >= 1GB.
 */
export function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B'
  const gb = bytes / 1073741824
  if (gb >= 1) return gb.toFixed(2) + ' GB'
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(0) + ' KB'
  return bytes + ' B'
}

/** Convert GB to bytes for API calls. */
export function gbToBytes(gb: number): number {
  return Math.round(gb * 1073741824)
}

/** Convert bytes to GB for display in inputs. */
export function bytesToGB(bytes: number): number {
  if (!bytes || bytes === 0) return 0
  return Math.round((bytes / 1073741824) * 100) / 100
}

/**
 * Format a Unix timestamp (seconds) to date string.
 */
export function formatDate(ts: number): string {
  if (!ts || ts === 0) return '-'
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`
}

/**
 * Format ISO date string to locale string.
 */
export function formatISODate(iso: string): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN')
}

/** Format plan/order price in cents. */
export function formatPrice(cents?: number, currency = 'CNY'): string {
  const n = (cents || 0) / 100
  if (n <= 0) return '免费'
  const sym = currency === 'USD' ? '$' : '¥'
  return `${sym}${n.toFixed(2)}`
}

/** ~31 days in seconds — longer plans show traffic as monthly quota. */
export const PLAN_MONTH_SEC = 31 * 86400

/** True when plan duration is longer than one month. */
export function isMonthlyTrafficPlan(durationSec?: number, resetDay?: number): boolean {
  if (resetDay && resetDay > 0) return true
  return (durationSec || 0) > PLAN_MONTH_SEC
}

/**
 * Shop / hero traffic line.
 * e.g. "每月流量 80.00 GB" | "流量 80.00 GB" | "流量 不限"
 */
export function formatPlanTrafficLine(
  trafficLimit: number,
  durationSec?: number,
  resetDay?: number,
): string {
  const monthly = isMonthlyTrafficPlan(durationSec, resetDay)
  const label = monthly ? '每月流量' : '流量'
  if (!trafficLimit || trafficLimit <= 0) return `${label} 不限`
  return `${label} ${formatBytes(trafficLimit)}`
}

/** Short label only: 每月流量 / 流量 */
export function planTrafficLabel(durationSec?: number, resetDay?: number): string {
  return isMonthlyTrafficPlan(durationSec, resetDay) ? '每月流量' : '流量'
}

/** Order status label for user portal. */
export function orderStatusLabel(status?: string): string {
  const m: Record<string, string> = {
    pending: '待支付',
    paid: '已支付',
    cancelled: '已取消',
    failed: '失败',
  }
  return m[status || ''] || status || '—'
}

/** Parse expired_at (ISO or unix seconds/ms) → Date ms, or 0. */
export function parseExpireMs(expiredAt?: string | number | null): number {
  if (expiredAt == null || expiredAt === '') return 0
  if (typeof expiredAt === 'number') {
    return expiredAt < 1e12 ? expiredAt * 1000 : expiredAt
  }
  const t = Date.parse(String(expiredAt))
  return Number.isFinite(t) ? t : 0
}

/** Format remaining seconds as mm:ss or h:mm:ss. */
export function formatCountdown(totalSec: number): string {
  const s = Math.max(0, Math.floor(totalSec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  if (h > 0) return `${h}:${pad(m)}:${pad(sec)}`
  return `${pad(m)}:${pad(sec)}`
}
