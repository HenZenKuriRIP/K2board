<template>
  <div class="page" v-loading="loading && !order">
    <div class="page-head">
      <el-button text class="back" @click="$router.push('/user/orders')">← 返回订单</el-button>
      <h2>订单详情</h2>
      <p v-if="tradeNo">{{ tradeNo }}</p>
    </div>

    <EmptyState
      v-if="!order && !loading"
      variant="notfound"
      title="未找到订单"
      description="订单不存在或无权查看，请从「我的订单」进入。"
      class="glass"
    >
      <el-button type="primary" @click="$router.push('/user/orders')">我的订单</el-button>
      <el-button @click="$router.push('/user')">回仪表盘</el-button>
    </EmptyState>

    <template v-else-if="order">
      <section class="glass card status-card" :class="order.status">
        <div class="status-row">
          <span class="pill" :class="order.status">{{ orderStatusLabel(order.status) }}</span>
          <span v-if="order.status === 'cancelled' && cancelReasonLabel(order.remark)" class="sub-pill">
            {{ cancelReasonLabel(order.remark) }}
          </span>
        </div>
        <div class="plan">{{ order.plan_name }}</div>
        <div class="amt">{{ formatPrice(order.total_amount, order.currency) }}</div>

        <div v-if="order.status === 'pending'" class="countdown-box">
          <span class="cd-label">支付剩余</span>
          <span class="cd-val" :class="{ danger: countdown.remaining.value <= 60 }">
            {{ countdown.expired.value ? '已到期' : countdown.label.value }}
          </span>
          <span class="cd-deadline" v-if="order.expired_at">截止 {{ formatISODate(order.expired_at) }}</span>
        </div>

        <BenefitChips :order="order" class="benefit-block" />

        <p v-if="order.status_hint" class="hint" :class="{ ok: order.status === 'paid', warn: order.status === 'cancelled' }">
          {{ order.status_hint }}
        </p>
      </section>

      <!-- Paid: entitlement summary -->
      <section v-if="order.status === 'paid'" class="glass card benefit-card">
        <h3>开通权益</h3>
        <div class="benefit-grid">
          <div class="b-item">
            <span class="b-k">套餐</span>
            <span class="b-v">{{ order.benefits?.plan_name || order.plan_name }}</span>
          </div>
          <div class="b-item">
            <span class="b-k">时长</span>
            <span class="b-v">{{ order.benefits?.duration_text || '—' }}</span>
          </div>
          <div class="b-item">
            <span class="b-k">{{ order.benefits?.traffic_label || '流量' }}</span>
            <span class="b-v">{{ order.benefits?.traffic_text || '—' }}</span>
          </div>
          <div class="b-item">
            <span class="b-k">速率</span>
            <span class="b-v">{{ order.benefits?.speed_text || '—' }}</span>
          </div>
          <div class="b-item">
            <span class="b-k">设备</span>
            <span class="b-v">{{ order.benefits?.device_text || '—' }}</span>
          </div>
          <div class="b-item" v-if="order.paid_at">
            <span class="b-k">支付时间</span>
            <span class="b-v">{{ formatISODate(order.paid_at) }}</span>
          </div>
          <div class="b-item" v-if="order.fulfilled_at">
            <span class="b-k">开通时间</span>
            <span class="b-v">{{ formatISODate(order.fulfilled_at) }}</span>
          </div>
        </div>
        <el-button type="primary" class="full" @click="copySubscribe">复制订阅链接</el-button>
      </section>

      <section class="glass card">
        <div class="kv"><span>订单号</span><b class="mono">{{ order.trade_no }}</b></div>
        <div class="kv"><span>创建时间</span><b>{{ formatISODate(order.created_at) }}</b></div>
        <div class="kv" v-if="order.payment_method">
          <span>支付方式</span><b>{{ methodLabel(order.payment_method, methods) }}</b>
        </div>
        <div class="kv" v-if="order.crypto_amount">
          <span>链上金额</span>
          <b>{{ order.crypto_amount }} {{ (order.crypto_token || '').toUpperCase() }}
            <template v-if="order.crypto_network">· {{ order.crypto_network }}</template>
          </b>
        </div>
        <div class="kv" v-if="order.pay_address">
          <span>收款地址</span><b class="mono sm">{{ order.pay_address }}</b>
        </div>
      </section>

      <!-- Pending pay -->
      <section v-if="order.status === 'pending'" class="glass card pay-card">
        <div class="pay-head">
          <h3>支付</h3>
          <span v-if="canReopenCashier(order)" class="ready-tag">可重开收银台</span>
        </div>

        <el-button
          v-if="canReopenCashier(order)"
          type="warning"
          size="large"
          class="full reopen"
          @click="reopenCashier"
        >
          打开已有收银台
        </el-button>
        <p v-if="canReopenCashier(order)" class="ck-hint center">
          若刚才已跳转过，可直接打开同一收款页，无需重新下单。
        </p>

        <div class="divider" v-if="canReopenCashier(order)"><span>或重新选择支付方式</span></div>

        <label class="field-label">支付方式</label>
        <div v-if="displayMethods.length" class="method-grid" role="listbox" aria-label="支付方式">
          <button
            v-for="m in displayMethods"
            :key="m.code"
            type="button"
            class="method-card"
            :class="[userFacingMethodTone(m.code, m.name), { active: selectedMethod === m.code }]"
            role="option"
            :aria-selected="selectedMethod === m.code"
            @click="selectedMethod = m.code"
          >
            <span class="mc-ico" aria-hidden="true">{{ userFacingMethodIcon(m.code, m.name) }}</span>
            <span class="mc-body">
              <span class="mc-name">{{ userFacingMethodName(m.code, m.name) }}</span>
            </span>
            <span class="mc-check" aria-hidden="true">{{ selectedMethod === m.code ? '✓' : '' }}</span>
          </button>
        </div>
        <p v-else class="ck-hint">暂无可用支付方式，请联系管理员。</p>
        <p v-if="selectedHint" class="ck-hint">
          {{ selectedHint }}
        </p>

        <!-- 第三方回跳失败时的用户须知 -->
        <div
          v-if="selectedMethod"
          class="pay-notice"
        >
          <div class="pay-notice-title">付款后请注意</div>
          <ul>
            <li>部分支付渠道付款成功后<strong>不会自动跳回本站</strong>（可能停在支付页或其它页面）。</li>
            <li>只要付款成功，系统会在后台自动开通套餐；请<strong>重新打开本站并登录</strong>，仪表盘即可看到已生效套餐。</li>
            <li>也可返回本页点击「刷新」或稍候再查；「我的订单」中状态变为「已支付」即表示开通成功。</li>
          </ul>
        </div>

        <div class="pay-actions">
          <el-button
            type="primary"
            size="large"
            class="full"
            :loading="paying"
            :disabled="!selectedMethod || countdown.expired.value"
            @click="doPay"
          >
            {{ payButtonText }}
          </el-button>
          <el-button size="large" class="full ghost" :loading="cancelling" @click="onCancel">
            取消订单
          </el-button>
        </div>

        <div class="policy" v-if="order.cancel_hint">
          <div class="policy-title">取消与迟到到账说明</div>
          <p>{{ order.cancel_hint }}</p>
        </div>
      </section>

      <section v-else class="actions-row">
        <el-button type="primary" @click="$router.push('/user')">返回仪表盘</el-button>
        <el-button @click="$router.push('/user/orders')">全部订单</el-button>
        <el-button v-if="order.status === 'cancelled'" type="warning" plain @click="$router.push('/user')">
          重新购买
        </el-button>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import {
  getOrder,
  syncOrder,
  getPaymentMethods,
  checkoutOrder,
  cancelOrder,
  buildOrderReturnUrl,
  type OrderInfo,
  type PaymentMethodInfo,
} from '@/api/userApi'
import { formatPrice, formatISODate, orderStatusLabel } from '@/utils/format'
import {
  CANCEL_POLICY_HTML,
  canReopenCashier,
  cancelReasonLabel,
  methodLabel,
  userFacingMethodHint,
  userFacingMethodIcon,
  userFacingMethodName,
  userFacingMethodTone,
} from '@/utils/order'
import { useOrderCountdown } from '@/composables/useOrderCountdown'
import EmptyState from '@/components/EmptyState.vue'
import BenefitChips from '@/components/BenefitChips.vue'

const route = useRoute()
const router = useRouter()
const store = useUserAuthStore()

const tradeNo = computed(() => String(route.params.trade_no || ''))
const order = ref<OrderInfo | null>(null)
const methods = ref<PaymentMethodInfo[]>([])
const selectedMethod = ref('')
const loading = ref(false)
const paying = ref(false)
const cancelling = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null
let paidToastShown = false

const isPending = computed(() => order.value?.status === 'pending')
const expiredAtRef = computed(() => order.value?.expired_at)

const countdown = useOrderCountdown(expiredAtRef, {
  active: isPending,
  onExpire: () => {
    refresh(true)
  },
})

const payButtonText = computed(() => {
  if (!order.value) return '去支付'
  if (order.value.total_amount <= 0) return '确认开通'
  if (canReopenCashier(order.value) && order.value.payment_method === selectedMethod.value) {
    return '重新发起支付'
  }
  return '去支付'
})

/** 全部启用方式按 sort 展示；名称仅做品牌净化（不隐藏易支付等备用通道） */
const displayMethods = computed(() => {
  return [...methods.value].sort((a, b) => {
    const sa = Number(a.sort ?? 0)
    const sb = Number(b.sort ?? 0)
    if (sa !== sb) return sa - sb
    return String(a.code).localeCompare(String(b.code))
  })
})

const selectedHint = computed(() => {
  if (!selectedMethod.value) return ''
  const m = methods.value.find((x) => x.code === selectedMethod.value)
  return userFacingMethodHint(selectedMethod.value, m?.name)
})

async function refresh(silent = false) {
  if (!store.token || !tradeNo.value) return
  if (!silent) loading.value = true
  try {
    const prev = order.value?.status
    // Gateway sync when pending (covers lost async notify)
    const pm = order.value?.payment_method || ''
    const isFrog = pm === 'frog' || pm.startsWith('frog_')
    if (!order.value || order.value.status === 'pending' || order.value.payment_method === 'alipay' || order.value.payment_method === 'epusdt' || order.value.payment_method === 'epay' || isFrog) {
      try {
        const sr = await syncOrder(store.token, tradeNo.value)
        if (sr.data?.order) order.value = sr.data.order
      } catch {
        const r = await getOrder(store.token, tradeNo.value)
        order.value = r.data
      }
    } else {
      const r = await getOrder(store.token, tradeNo.value)
      order.value = r.data
    }
    if (order.value?.status === 'paid') {
      stopPoll()
      await store.fetchInfo()
      await store.refreshPendingCount()
      if (prev === 'pending' && !paidToastShown) {
        paidToastShown = true
        ElMessage.success('支付成功，套餐已开通')
      }
    }
    if (order.value?.status === 'cancelled' || order.value?.status === 'failed') {
      stopPoll()
      await store.refreshPendingCount()
    }
    // prefer existing payment method on re-entry
    if (order.value?.payment_method && methods.value.some((m) => m.code === order.value!.payment_method)) {
      selectedMethod.value = order.value.payment_method
    }
  } catch {
    if (!silent) order.value = null
  } finally {
    if (!silent) loading.value = false
  }
}

function startPoll() {
  stopPoll()
  pollTimer = setInterval(() => {
    if (order.value?.status === 'pending') refresh(true)
  }, 4000)
}

function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function reopenCashier() {
  const url = order.value?.payment_url
  if (!url || !canReopenCashier(order.value)) {
    ElMessage.warning('收银台链接已失效，请重新发起支付')
    return
  }
  window.location.href = url
}

async function doPay() {
  if (!store.token || !order.value || !selectedMethod.value) return
  if (order.value.status !== 'pending') return
  paying.value = true
  try {
    const tn = order.value.trade_no
    const method = selectedMethod.value
    const returnUrl = buildOrderReturnUrl(tn)
    const r = await checkoutOrder(store.token, tn, method, returnUrl)
    const intent = r.data?.intent
    if (r.data?.order) order.value = r.data.order

    if (intent?.type === 'completed') {
      ElMessage.success(intent.message || '已开通')
      await refresh()
      await store.fetchInfo()
      await store.refreshPendingCount()
      return
    }
    if (intent?.type === 'redirect' && intent.url) {
      ElMessage.success(intent.message || '正在跳转收银台…')
      // keep payment_url on order after redirect return
      window.location.href = intent.url
      return
    }
    ElMessage.info(intent?.message || '请按提示完成支付')
    startPoll()
  } catch (e: any) {
    ElMessage.error(e?.message || '支付失败')
    await refresh()
  } finally {
    paying.value = false
  }
}

async function onCancel() {
  if (!store.token || !order.value) return
  try {
    await ElMessageBox.confirm(CANCEL_POLICY_HTML, '取消订单', {
      type: 'warning',
      dangerouslyUseHTMLString: true,
      confirmButtonText: '确认取消',
      cancelButtonText: '再想想',
      customClass: 'u-cancel-box',
    })
  } catch {
    return
  }
  cancelling.value = true
  try {
    const r = await cancelOrder(store.token, order.value.trade_no)
    order.value = r.data
    ElMessage.success('订单已取消')
    stopPoll()
    await store.refreshPendingCount()
  } catch (e: any) {
    ElMessage.error(e?.message || '取消失败')
    await refresh()
  } finally {
    cancelling.value = false
  }
}

async function copySubscribe() {
  const url = store.info?.subscribe_url
  if (!url) {
    await store.fetchInfo()
  }
  const u = store.info?.subscribe_url
  if (!u) {
    ElMessage.warning('暂无订阅链接')
    return
  }
  await navigator.clipboard.writeText(u)
  ElMessage.success('订阅链接已复制')
}

watch(tradeNo, async () => {
  paidToastShown = false
  await refresh()
  if (order.value?.status === 'pending') startPoll()
})

onMounted(async () => {
  if (!store.isLoggedIn) {
    router.replace('/user/login')
    return
  }
  try {
    const mr = await getPaymentMethods()
    methods.value = mr.data || []
    const sorted = [...methods.value].sort(
      (a, b) => Number(a.sort ?? 0) - Number(b.sort ?? 0) || String(a.code).localeCompare(String(b.code)),
    )
    selectedMethod.value = sorted[0]?.code || ''
  } catch {
    methods.value = []
  }
  await refresh()
  if (order.value?.status === 'pending') startPoll()
})

onUnmounted(stopPoll)
</script>

<style scoped>
.page {
  width: 100%;
  max-width: 720px;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-head h2 {
  margin: 4px 0 0;
  font-size: clamp(22px, 2.4vw, 26px);
  font-weight: 800;
  color: var(--u-text);
}
.page-head p {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
  word-break: break-all;
}
.back {
  color: var(--u-text-3) !important;
  padding-left: 0 !important;
}
.glass {
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  border-radius: 20px;
  backdrop-filter: none;
  -webkit-backdrop-filter: none;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
  position: relative;
  overflow: hidden;
}
.glass::before {
  content: "";
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
  background: var(--u-surface-2);
  z-index: 0;
}
.glass > * {
  position: relative;
  z-index: 1;
}
.card {
  padding: 22px;
}
.status-card {
  text-align: center;
}
.status-card.paid {
  border-color: rgba(52, 211, 153, 0.35);
  background: var(--u-surface-2);
}
.status-card.pending {
  border-color: rgba(251, 191, 36, 0.35);
  background: var(--u-surface-2);
}
.status-card.cancelled {
  border-color: rgba(148, 163, 184, 0.25);
}
.status-row {
  display: flex;
  justify-content: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.pill {
  font-size: 12px;
  font-weight: 700;
  padding: 4px 12px;
  border-radius: 999px;
}
.pill.pending {
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.14);
}
.pill.paid {
  color: #34d399;
  background: rgba(52, 211, 153, 0.14);
}
.pill.cancelled,
.pill.failed {
  color: var(--u-text-3);
  background: rgba(148, 163, 184, 0.14);
}
.sub-pill {
  font-size: 11px;
  font-weight: 600;
  color: var(--u-text-3);
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--u-surface-2);
}
.plan {
  font-size: 18px;
  font-weight: 800;
  color: var(--u-text);
}
.amt {
  margin-top: 8px;
  font-size: 28px;
  font-weight: 800;
  color: var(--u-primary-strong);
}
.countdown-box {
  margin: 18px auto 0;
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 12px 28px;
  border-radius: 14px;
  background: var(--u-surface-2);
  border: 1px solid rgba(251, 191, 36, 0.25);
}
.cd-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--u-text-3);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.cd-val {
  font-size: 32px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: #fbbf24;
  letter-spacing: 0.04em;
}
.cd-val.danger {
  color: #f87171;
}
.cd-deadline {
  font-size: 11px;
  color: var(--u-text-3);
}
.benefit-block {
  margin-top: 14px;
  justify-content: center;
}
.hint {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.55;
  text-align: left;
}
.hint.ok {
  color: #15803d;
  text-align: center;
}
.hint.warn {
  color: #fcd34d;
}
.benefit-card h3,
.pay-card h3 {
  margin: 0 0 14px;
  font-size: 15px;
  font-weight: 800;
  color: var(--u-text);
}
.benefit-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}
.b-item {
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  border-radius: 12px;
  padding: 10px 12px;
  text-align: left;
}
.b-k {
  display: block;
  font-size: 11px;
  color: var(--u-text-3);
  font-weight: 600;
  margin-bottom: 4px;
}
.b-v {
  font-size: 13px;
  font-weight: 700;
  color: var(--u-text-2);
}
.kv {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: var(--u-text-3);
  margin-bottom: 10px;
}
.kv:last-child {
  margin-bottom: 0;
}
.kv b {
  color: var(--u-text-2);
  font-weight: 600;
  text-align: right;
  word-break: break-all;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}
.mono.sm {
  font-size: 11px;
}
.pay-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.pay-head h3 {
  margin: 0;
}
.ready-tag {
  font-size: 11px;
  font-weight: 700;
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.12);
  padding: 3px 8px;
  border-radius: 999px;
}
.field-label {
  display: block;
  font-size: 12px;
  font-weight: 700;
  color: var(--u-text-3);
  margin-bottom: 8px;
}
.method-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 10px;
}
@media (min-width: 480px) {
  .method-grid {
    grid-template-columns: 1fr 1fr;
  }
}
.method-card {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  margin: 0;
  padding: 14px 14px 14px 12px;
  border-radius: 14px;
  border: 1.5px solid var(--u-border);
  background: var(--u-surface);
  color: var(--u-text);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
  font: inherit;
}
.method-card:hover {
  border-color: rgba(244, 63, 94, 0.35);
  background: var(--u-surface-2);
}
.method-card.active {
  border-color: var(--u-primary, #f43f5e);
  box-shadow: 0 0 0 3px rgba(244, 63, 94, 0.12);
  background: var(--u-surface-2);
}
.mc-ico {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  font-size: 15px;
  font-weight: 800;
  color: #fff;
  background: #64748b;
}
.tone-wechat .mc-ico {
  background: linear-gradient(135deg, #22c55e, #16a34a);
}
.tone-alipay .mc-ico,
.tone-giftcard .mc-ico {
  background: linear-gradient(135deg, #38bdf8, #2563eb);
}
.tone-usdt .mc-ico {
  background: linear-gradient(135deg, #34d399, #059669);
}
.tone-epay .mc-ico {
  background: linear-gradient(135deg, #a78bfa, #7c3aed);
}
.tone-frog .mc-ico {
  background: linear-gradient(135deg, #fb923c, #ea580c);
}
.tone-online .mc-ico {
  background: linear-gradient(135deg, #818cf8, #4f46e5);
}
.tone-other .mc-ico {
  background: linear-gradient(135deg, #f472b6, #e11d48);
}
.mc-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.mc-name {
  font-size: 15px;
  font-weight: 800;
  color: var(--u-text);
  line-height: 1.3;
  word-break: break-word;
}
.mc-check {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  border: 1.5px solid var(--u-border);
  display: grid;
  place-items: center;
  font-size: 12px;
  font-weight: 800;
  color: #fff;
  background: transparent;
}
.method-card.active .mc-check {
  border-color: var(--u-primary, #f43f5e);
  background: var(--u-primary, #f43f5e);
}
.ck-hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.5;
}
.ck-hint.center {
  text-align: center;
}
.pay-notice {
  margin-top: 14px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(244, 63, 94, 0.08);
  border: 1px solid rgba(244, 63, 94, 0.22);
  text-align: left;
}
.pay-notice-title {
  font-size: 12px;
  font-weight: 800;
  color: var(--u-primary);
  margin-bottom: 8px;
}
.pay-notice ul {
  margin: 0;
  padding-left: 1.15em;
}
.pay-notice li {
  font-size: 12px;
  color: var(--u-text-2);
  line-height: 1.55;
  margin-bottom: 6px;
}
.pay-notice li:last-child {
  margin-bottom: 0;
}
.pay-notice strong {
  color: var(--u-text);
  font-weight: 700;
}
.divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 16px 0;
  color: var(--u-text-2);
  font-size: 11px;
}
.divider::before,
.divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--u-surface-2);
}
.pay-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 18px;
}
.full {
  width: 100%;
}
.ghost {
  background: var(--u-surface) !important;
  border: 1px solid var(--u-border) !important;
  color: var(--u-text-2) !important;
}
.reopen {
  margin-bottom: 4px;
}
.policy {
  margin-top: 16px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(251, 191, 36, 0.06);
  border: 1px solid rgba(251, 191, 36, 0.15);
  text-align: left;
}
.policy-title {
  font-size: 12px;
  font-weight: 800;
  color: #fbbf24;
  margin-bottom: 6px;
}
.policy p {
  margin: 0;
  font-size: 11px;
  color: var(--u-text-3);
  line-height: 1.55;
}
.actions-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
}
</style>
