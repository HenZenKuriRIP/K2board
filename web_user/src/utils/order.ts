import type { OrderInfo } from '@/api/userApi'
import { parseExpireMs } from '@/utils/format'

/** Cancel policy HTML for MessageBox */
export const CANCEL_POLICY_HTML = `
<div class="u-cancel-policy">
  <p class="u-cp-lead"><strong>取消后将无法再支付此订单</strong></p>
  <ul>
    <li>若<strong>尚未转账</strong>：可安全取消，稍后再下新单。</li>
    <li>若<strong>已向收款地址转账</strong>：请勿取消，等待到账。手动取消后系统<strong>不会</strong>因迟到到账自动开通，需联系管理员人工补单。</li>
    <li>超时自动关闭的订单：若支付迟到，系统<strong>可能</strong>自动补开通。</li>
    <li>取消<strong>不会</strong>同步关闭第三方收银台（如 USDT 平台可能仍显示「待付款」直至过期）。</li>
  </ul>
</div>
`

export function cancelReasonLabel(remark?: string): string {
  switch (remark) {
    case 'closed by user':
      return '您已手动取消'
    case 'auto-expired':
      return '超时自动关闭'
    case 'closed by admin':
      return '管理员关闭'
    default:
      return ''
  }
}

export function canReopenCashier(o?: OrderInfo | null): boolean {
  if (!o || o.status !== 'pending') return false
  if (o.can_reopen_cashier === true && o.payment_url) return true
  if (!o.payment_url) return false
  const end = parseExpireMs(o.expired_at)
  return end > Date.now()
}

export function benefitChips(o?: OrderInfo | null): string[] {
  if (!o) return []
  const b = o.benefits
  if (b) {
    const trafficChip =
      b.traffic_limit > 0 && b.traffic_label
        ? `${b.traffic_label} ${b.traffic_text}`
        : b.traffic_text
    return [b.duration_text, trafficChip, b.speed_text, b.device_text].filter(Boolean)
  }
  // fallback from raw fields
  const chips: string[] = []
  if (o.duration) {
    const d = o.duration / 86400
    chips.push(d >= 1 ? `${Math.floor(d)} 天` : '短期')
  }
  if (o.traffic_limit != null) {
    if (o.traffic_limit <= 0) {
      chips.push('不限流量')
    } else {
      const gb = (o.traffic_limit / 1073741824).toFixed(0)
      const monthly = (o.duration || 0) > 31 * 86400
      chips.push(monthly ? `每月流量 ${gb} GB` : `流量 ${gb} GB`)
    }
  }
  if (o.speed_limit != null) {
    chips.push(o.speed_limit > 0 ? `${o.speed_limit} Mbps` : '不限速')
  }
  if (o.device_limit != null) {
    chips.push(o.device_limit > 0 ? `${o.device_limit} 台` : '不限设备')
  }
  return chips
}

/** User-facing pay brand: hide frog/epay/platform noise. */
export type PayBrand = 'wechat' | 'alipay' | 'usdt' | 'online' | 'other'

/**
 * Classify a payment method for user UI (icon / title).
 * Prefer admin display name hints, then method code suffixes (frog_wx, epay_alipay).
 * Bare epay/frog (聚合收银台) → online，仍展示在用户端，不暴露平台名。
 */
export function payBrand(code?: string, name?: string): PayBrand {
  const n = String(name || '')
  const c = String(code || '').toLowerCase()
  // Admin name is strongest signal (e.g. 显示名「支付宝」但 code 为 frog_xxx)
  if (/微信|wechat|weixin|wxpay/i.test(n)) return 'wechat'
  if (/支付宝|alipay/i.test(n)) return 'alipay'
  if (/usdt|泰达|tether/i.test(n)) return 'usdt'

  if (c === 'alipay' || c.includes('alipay') || c.endsWith('_ali') || c.includes('_zfb')) return 'alipay'
  if (
    c.includes('wx') ||
    c.includes('wechat') ||
    c.includes('weixin') ||
    c.includes('wxpay')
  ) {
    return 'wechat'
  }
  // giftcard 中转多为支付宝
  if (c === 'giftcard' || c.startsWith('giftcard')) return 'alipay'
  if (c === 'epusdt' || c === 'bepusdt' || c.includes('usdt')) return 'usdt'
  // 易支付 / 青蛙 裸 code 或多实例未带 wx/ali：聚合收银台
  if (c === 'epay' || c.startsWith('epay_') || c === 'frog' || c.startsWith('frog_')) {
    return 'online'
  }
  return 'other'
}

function stripPlatformNoise(raw: string): string {
  return String(raw || '')
    .replace(/青蛙\s*(四方|支付|系统)?/gi, '')
    .replace(/彩虹\s*/gi, '')
    .replace(/易支付/gi, '')
    .replace(/四方|通道\s*\d*/gi, '')
    .replace(/[-_·|/]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

/** Plain title: 支付宝 / 微信支付 / 在线支付… 不展示青蛙、易支付等平台品牌。 */
export function userFacingMethodName(code?: string, name?: string): string {
  if (!code && !name) return '—'
  const brand = payBrand(code, name)
  switch (brand) {
    case 'wechat':
      return '微信支付'
    case 'alipay':
      return '支付宝'
    case 'usdt':
      return 'USDT'
    case 'online': {
      // 管理端若配置了干净显示名（如「备用支付」）则保留；否则统一「在线支付」
      const cleaned = stripPlatformNoise(String(name || ''))
      if (cleaned && !/^(epay|frog)(_|$)/i.test(cleaned)) return cleaned
      return '在线支付'
    }
    default: {
      let s = stripPlatformNoise(String(name || code || ''))
      if (/支付宝|alipay/i.test(s)) return '支付宝'
      if (/微信|wechat|weixin/i.test(s)) return '微信支付'
      return s || String(name || code)
    }
  }
}

export function userFacingMethodIcon(code?: string, name?: string): string {
  switch (payBrand(code, name)) {
    case 'wechat':
      return '微'
    case 'alipay':
      return '支'
    case 'usdt':
      return '₮'
    case 'online':
      return '付'
    default:
      return '付'
  }
}

export function userFacingMethodTone(code?: string, name?: string): string {
  const b = payBrand(code, name)
  if (b === 'wechat') return 'tone-wechat'
  if (b === 'alipay') return 'tone-alipay'
  if (b === 'usdt') return 'tone-usdt'
  if (b === 'online') return 'tone-online'
  return 'tone-other'
}

export function userFacingMethodHint(code?: string, name?: string): string {
  switch (payBrand(code, name)) {
    case 'wechat':
      return '将跳转微信完成支付。部分环境支付成功后可能不会自动跳回本站。'
    case 'alipay':
      return '将跳转支付宝完成支付。'
    case 'usdt':
      return '将跳转 USDT 收银台，请选择网络并完成链上转账。'
    case 'online':
      return '将跳转支付页，可在收银台选择微信或支付宝完成付款。'
    default:
      return '将跳转支付页完成付款。支付成功后请留意下方说明。'
  }
}

export function methodLabel(code?: string, methods?: { code: string; name: string }[]): string {
  if (!code) return '—'
  const m = methods?.find((x) => x.code === code)
  return userFacingMethodName(code, m?.name)
}

/** Pick first non-empty query value; if array (duplicate keys), prefer K2 merchant trade_no. */
function firstQueryValue(v: unknown): string {
  if (v == null || v === '') return ''
  if (Array.isArray(v)) {
    const list = v.map((x) => String(x ?? '').trim()).filter(Boolean)
    if (!list.length) return ''
    // Epay appends its platform trade_no alongside our param; K2 orders start with "K2"
    const k2 = list.find((s) => /^K2/i.test(s))
    return k2 || list[0]
  }
  const s = String(v).trim()
  // Vue may stringify duplicates as "a,b"
  if (s.includes(',')) {
    const parts = s.split(',').map((x) => x.trim()).filter(Boolean)
    const k2 = parts.find((p) => /^K2/i.test(p))
    if (k2) return k2
  }
  return s
}

/**
 * Resolve merchant order id from payment return query.
 * 易支付等会同时带回：out_trade_no=商户单号、trade_no=平台流水；切勿用平台流水查本站订单。
 * 优先：out_trade_no → tn（本站写入）→ trade_no（兼容旧链接，数组时偏 K2*）
 */
export function resolveOrderTradeNoFromQuery(query: Record<string, unknown> | { [key: string]: unknown }): string {
  const out = firstQueryValue(query.out_trade_no)
  if (out) return out
  const tn = firstQueryValue(query.tn)
  if (tn) return tn
  return firstQueryValue(query.trade_no)
}
