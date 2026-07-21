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

/** Convert GB to bytes for API calls. Never returns NaN. */
export function gbToBytes(gb: number): number {
  const n = Number(gb)
  if (!Number.isFinite(n) || n <= 0) return 0
  return Math.round(n * 1073741824)
}

/** Coerce form number fields to a safe finite number (default 0). */
export function safeNum(v: unknown, fallback = 0): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : fallback
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
 * Format ISO / RFC3339 datetime string (e.g. user.created_at from API) for admin tables.
 */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '-'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/**
 * Format ISO date string to locale string.
 */
export function formatISODate(iso: string): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN')
}
