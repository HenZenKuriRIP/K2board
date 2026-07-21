import type { UserInfo } from '@/api/userApi'

/** Placeholder group names that must not count as an active subscription. */
const EMPTY_GROUP = new Set(['', '未分组', '-', '—', '无', 'none', 'null'])

/**
 * Whether the user currently has (or had) a bound plan/group entitlement.
 * Aligns with backend UserHasActiveService for “usable now”, but also true
 * when plan is bound and only expired (so UI can show renew paths).
 */
export function userHasBoundPlan(info?: UserInfo | null): boolean {
  if (!info) return false
  const planId = Number(info.plan_id) || 0
  if (planId > 0) return true
  const name = (info.plan_name || '').trim()
  if (name) return true
  // Never treat placeholder group as service (bug: backend used to send "-")
  const g = (info.group_name || '').trim()
  if (g && !EMPTY_GROUP.has(g)) return true
  // Explicit flag from API when present
  if (info.has_service === true) return true
  return false
}

/** Usable service right now (bound + not expired). */
export function userHasActiveService(info?: UserInfo | null): boolean {
  if (!userHasBoundPlan(info)) return false
  if (info?.expired) return false
  return true
}
