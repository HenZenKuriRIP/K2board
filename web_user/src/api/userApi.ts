import request from './request'

/** 当前可续费套餐摘要（停售后仍允许持有者续费时返回） */
export interface RenewPlanInfo {
  id: number
  name: string
  group_id?: number
  duration: number
  traffic_limit: number
  speed_limit: number
  device_limit: number
  price: number
  currency: string
  show_on_shop?: boolean
  allow_renew?: boolean
}

export interface UserInfo {
  id: number; email: string; uuid: string
  /** Session token is only from login/register (localStorage), not echoed by /info */
  token?: string
  group_name: string
  plan_id?: number
  plan_name?: string
  /** Backend: plan/group bound and not expired */
  has_service?: boolean
  traffic_used: number; traffic_limit: number
  usage_percent: number
  traffic_reset_day?: number
  last_traffic_reset_at?: number
  speed_limit: number; device_limit: number
  expire_at: number; expire_text: string; expired: boolean; subscribe_url: string
  /** 是否可续费当前套餐（仪表盘入口） */
  can_renew?: boolean
  renew_plan?: RenewPlanInfo | null
}

export interface PlanInfo {
  id: number; name: string; group_id: number
  duration: number; traffic_limit: number; speed_limit: number
  device_limit: number; enable: boolean; sort: number
  price?: number; currency?: string; show_on_shop?: boolean
  /** 1–28 monthly traffic reset day; 0 = no calendar reset */
  reset_day?: number
  allow_renew?: boolean
}

export interface OrderBenefits {
  plan_name: string
  duration: number
  duration_text: string
  traffic_limit: number
  traffic_text: string
  /** 流量 | 每月流量 */
  traffic_label?: string
  speed_limit: number
  speed_text: string
  device_limit: number
  device_text: string
}

export interface OrderInfo {
  id: number
  trade_no: string
  plan_id: number
  plan_name: string
  group_id?: number
  duration?: number
  traffic_limit?: number
  speed_limit?: number
  device_limit?: number
  total_amount: number
  currency: string
  status: string
  payment_method?: string
  created_at: string
  paid_at?: string
  expired_at?: string
  fulfilled_at?: string
  remark?: string
  /** Safe checkout fields (server strips full meta) */
  payment_url?: string
  pay_address?: string
  crypto_amount?: string
  crypto_token?: string
  crypto_network?: string
  remaining_seconds?: number
  benefits?: OrderBenefits
  can_reopen_cashier?: boolean
  cancel_hint?: string
  status_hint?: string
}

export interface PaymentMethodInfo {
  code: string
  name: string
  sort: number
}

export function getUserInfo(token: string): Promise<{ data: UserInfo }> {
  return request.get('/user/info', { params: { token } })
}

export function getPlans(): Promise<{ data: PlanInfo[] }> {
  return request.get('/user/plans')
}

export function userLogin(email: string, password: string): Promise<{ data: { token: string; uuid: string; email: string; subscribe_url: string } }> {
  return request.post('/user/login', {
    email: String(email || '').trim().toLowerCase(),
    password: String(password ?? '').trim(),
  })
}

export function userRegister(
  email: string,
  password: string,
  code: string,
  inviteCode?: string,
): Promise<{ data: { token: string; uuid: string; email: string; subscribe_url: string } }> {
  return request.post('/user/register', {
    email,
    password,
    code,
    invite_code: inviteCode || undefined,
  })
}

// ── Referral ──────────────────────────────────────

export interface PayoutMethod {
  code: string
  name: string
}

export interface ReferralOverview {
  enable: boolean
  invite_code: string
  invite_url: string
  rate_percent: number
  min_withdraw: number // cents
  balance: number
  commission_total: number
  invitee_count: number
  pending_withdraw: number
  payout_methods: PayoutMethod[]
}

export interface CommissionLedger {
  id: number
  user_id: number
  from_user_id: number
  from_user_email?: string
  order_id: number
  trade_no: string
  order_amount: number
  rate_percent: number
  amount: number
  status: string
  remark?: string
  created_at: string
}

export interface CommissionWithdraw {
  id: number
  user_id: number
  amount: number
  status: string
  method: string
  account: string
  account_name?: string
  admin_remark?: string
  processed_at?: string
  created_at: string
}

export function getReferral(token: string): Promise<{ data: ReferralOverview }> {
  return request.get('/user/referral', { params: { token } })
}

export function listReferralLedgers(
  token: string,
  page = 1,
  pageSize = 20,
): Promise<{ data: { list: CommissionLedger[]; total: number } }> {
  return request.get('/user/referral/ledgers', { params: { token, page, page_size: pageSize } })
}

export function listReferralWithdrawals(
  token: string,
  page = 1,
  pageSize = 20,
): Promise<{ data: { list: CommissionWithdraw[]; total: number } }> {
  return request.get('/user/referral/withdrawals', { params: { token, page, page_size: pageSize } })
}

export function listInvitees(
  token: string,
  page = 1,
  pageSize = 20,
): Promise<{ data: { list: { id: number; email: string; created_at: string; plan_id: number; enable: boolean }[]; total: number } }> {
  return request.get('/user/referral/invitees', { params: { token, page, page_size: pageSize } })
}

export function createWithdraw(
  token: string,
  amount: number,
  method: string,
  account: string,
  accountName?: string,
): Promise<{ data: CommissionWithdraw }> {
  return request.post('/user/referral/withdraw', {
    token,
    amount,
    method,
    account,
    account_name: accountName || '',
  })
}

export function sendVerificationCode(email: string): Promise<any> {
  return request.post('/user/send-code', { email })
}

/** 忘记密码：发送重置验证码（邮箱未注册时也返回成功，防枚举） */
export function sendResetPasswordCode(email: string): Promise<any> {
  return request.post('/user/forgot-password/send-code', {
    email: String(email || '').trim().toLowerCase(),
  })
}

/** 忘记密码：邮箱 + 验证码重置 */
export function resetPassword(email: string, code: string, newPassword: string): Promise<any> {
  return request.post('/user/reset-password', {
    email: String(email || '').trim().toLowerCase(),
    code: String(code || '').trim(),
    new_password: newPassword,
  })
}

export function changeUserPassword(token: string, oldPwd: string, newPwd: string): Promise<any> {
  return request.post('/user/change-password', { token, old_password: oldPwd, new_password: newPwd })
}

export function getPaymentMethods(): Promise<{ data: PaymentMethodInfo[] }> {
  return request.get('/user/payment-methods')
}

export function createOrder(token: string, planId: number): Promise<{ data: OrderInfo }> {
  return request.post('/user/orders', { token, plan_id: planId })
}

export function listOrders(token: string): Promise<{ data: OrderInfo[] }> {
  return request.get('/user/orders', { params: { token } })
}

export function getOrder(token: string, tradeNo: string): Promise<{ data: OrderInfo }> {
  return request.get(`/user/orders/${tradeNo}`, { params: { token } })
}

export function checkoutOrder(
  token: string,
  tradeNo: string,
  method: string,
  returnUrl?: string,
): Promise<{ data: any }> {
  return request.post(`/user/orders/${tradeNo}/checkout`, {
    token,
    method,
    return_url: returnUrl || undefined,
  })
}

export function cancelOrder(token: string, tradeNo: string): Promise<{ data: OrderInfo }> {
  return request.post(`/user/orders/${encodeURIComponent(tradeNo)}/cancel`, { token })
}

/** Query payment gateway and sync local order (notify fallback). */
export function syncOrder(token: string, tradeNo: string): Promise<{ data: { order: OrderInfo; synced?: boolean; message?: string } }> {
  return request.post(`/user/orders/${encodeURIComponent(tradeNo)}/sync`, { token })
}

/**
 * Browser return URL after external cashier (hash mode).
 * Uses `tn=` (not `trade_no=`) so 易支付等回跳追加的平台流水 trade_no 不会覆盖商户单号。
 */
export function buildOrderReturnUrl(tradeNo: string): string {
  const base = window.location.origin + window.location.pathname.replace(/\/?$/, '/')
  return `${base}#/user/order-result?tn=${encodeURIComponent(tradeNo)}`
}
