<template>
  <div class="result-page" v-loading="loading && !order">
    <section class="glass card">
      <h2>支付结果</h2>
      <p class="sub">订单 {{ tradeNo || '—' }}</p>

      <div v-if="order" class="status-block" :class="order.status">
        <div class="label">{{ statusText }}</div>
        <div class="plan">{{ order.plan_name }}</div>
        <div class="amt">{{ formatPrice(order.total_amount, order.currency) }}</div>

        <div v-if="order.status === 'pending'" class="countdown-box">
          <span class="cd-label">支付剩余</span>
          <span class="cd-val" :class="{ danger: countdown.remaining.value <= 60 }">
            {{ countdown.expired.value ? '已到期' : countdown.label.value }}
          </span>
        </div>

        <BenefitChips v-if="order.status === 'paid' || order.status === 'pending'" :order="order" class="chips" />
      </div>

      <EmptyState
        v-else-if="!loading"
        variant="notfound"
        title="未找到订单"
        description="链接可能无效，或订单不属于当前账号。"
      />

      <p v-if="order?.status_hint" class="hint" :class="{ ok: order.status === 'paid', warn: order.status === 'cancelled' }">
        {{ order.status_hint }}
      </p>
      <p v-else-if="order?.status === 'pending'" class="hint">
        若已完成链上支付，请稍候；本页会自动刷新。也可打开收银台继续支付。
      </p>

      <!-- 支付核实说明：第三方可能不回跳本站 -->
      <div class="verify-notice" v-if="order">
        <div class="vn-title">支付核实说明</div>
        <ul>
          <li>
            部分第三方支付完成后<strong>不会自动跳回本站</strong>（可能停在支付成功页或其它页面，如 Google 等）。
          </li>
          <li>
            只要付款成功，套餐会在后台<strong>自动开通</strong>。若未自动跳转，请<strong>重新打开本站并登录</strong>，仪表盘即可看到已生效套餐。
          </li>
          <li>
            也可在本页点击「刷新状态」，或到「我的订单」查看是否已变为「已支付」。
          </li>
        </ul>
      </div>

      <div class="actions" v-if="order">
        <el-button
          v-if="order.status === 'pending' && canReopenCashier(order)"
          type="warning"
          @click="reopen"
        >
          打开收银台
        </el-button>
        <el-button v-if="order.status === 'pending'" type="primary" @click="goPay">
          继续支付
        </el-button>
        <el-button type="primary" v-if="order.status === 'paid'" @click="$router.push('/user')">
          返回仪表盘
        </el-button>
        <el-button v-if="order.status !== 'paid'" @click="$router.push('/user')">仪表盘</el-button>
        <el-button @click="refresh" :loading="loading">刷新状态</el-button>
        <el-button @click="$router.push('/user/orders')">我的订单</el-button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import { getOrder, syncOrder, type OrderInfo } from '@/api/userApi'
import { formatPrice, orderStatusLabel } from '@/utils/format'
import { canReopenCashier, resolveOrderTradeNoFromQuery } from '@/utils/order'
import { useOrderCountdown } from '@/composables/useOrderCountdown'
import EmptyState from '@/components/EmptyState.vue'
import BenefitChips from '@/components/BenefitChips.vue'

const route = useRoute()
const router = useRouter()
const store = useUserAuthStore()
/** 兼容 tn / out_trade_no / trade_no；易支付平台流水 trade_no 不会盖掉商户单号 */
const tradeNo = computed(() => resolveOrderTradeNoFromQuery(route.query as Record<string, unknown>))
const order = ref<OrderInfo | null>(null)
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | null = null
let paidToast = false

const statusText = computed(() => orderStatusLabel(order.value?.status))
const isPending = computed(() => order.value?.status === 'pending')
const expiredAtRef = computed(() => order.value?.expired_at)

const countdown = useOrderCountdown(expiredAtRef, {
  active: isPending,
  onExpire: () => refresh(),
})

function goPay() {
  if (!tradeNo.value) return
  router.push({ name: 'UserOrderPay', params: { trade_no: tradeNo.value } })
}

function reopen() {
  if (!canReopenCashier(order.value) || !order.value?.payment_url) {
    goPay()
    return
  }
  window.location.href = order.value.payment_url
}

async function refresh() {
  if (!store.token || !tradeNo.value) return
  loading.value = true
  try {
    // Prefer gateway sync for pending (alipay query / epusdt) so lost notify still opens plan
    try {
      const sr = await syncOrder(store.token, tradeNo.value)
      if (sr.data?.order) order.value = sr.data.order
    } catch {
      const r = await getOrder(store.token, tradeNo.value)
      order.value = r.data
    }
    if (!order.value) {
      const r = await getOrder(store.token, tradeNo.value)
      order.value = r.data
    }
    if (order.value?.status === 'paid' || order.value?.status === 'cancelled') {
      stopPoll()
      if (order.value.status === 'paid') {
        await store.fetchInfo()
        await store.refreshPendingCount()
        // 回跳时 notify 往往已先 paid：首屏即 paid 也要提示成功（不仅 pending→paid）
        if (!paidToast) {
          paidToast = true
          ElMessage.success('支付成功，套餐已开通')
        }
      } else {
        await store.refreshPendingCount()
      }
    }
  } catch {
    order.value = null
  } finally {
    loading.value = false
  }
}

function startPoll() {
  stopPoll()
  timer = setInterval(() => {
    if (order.value?.status === 'pending') refresh()
  }, 4000)
}

function stopPoll() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

onMounted(async () => {
  if (!store.isLoggedIn) {
    // 尽量带上单号，登录后用户可从订单页继续看
    const tn = tradeNo.value
    window.location.hash = tn
      ? `#/user/login?from=order-result&tn=${encodeURIComponent(tn)}`
      : '#/user/login'
    return
  }
  if (!tradeNo.value) {
    order.value = null
    return
  }
  await refresh()
  if (order.value?.status === 'pending') startPoll()
})

onUnmounted(stopPoll)
</script>

<style scoped>
.result-page {
  width: 100%;
  max-width: 560px;
  margin: 0;
  padding: 4px 0 24px;
}
.glass {
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  border-radius: 24px;
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
.card {
  padding: 32px 28px;
  text-align: center;
}
.card > * {
  position: relative;
  z-index: 1;
}
.card h2 {
  margin: 0;
  color: var(--u-text);
  font-size: 20px;
  font-weight: 800;
}
.sub {
  margin: 8px 0 20px;
  font-size: 12px;
  color: var(--u-text-3);
  word-break: break-all;
}
.status-block {
  padding: 22px;
  border-radius: 16px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  margin-bottom: 16px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
}
.status-block.paid {
  border: 1px solid rgba(52, 211, 153, 0.35);
  background: var(--u-surface-2);
}
.status-block.pending {
  border: 1px solid rgba(251, 191, 36, 0.35);
}
.label {
  font-size: 14px;
  font-weight: 700;
  color: var(--u-primary);
  margin-bottom: 8px;
}
.plan {
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text);
}
.amt {
  margin-top: 8px;
  font-size: 22px;
  font-weight: 800;
  color: var(--u-primary-strong);
}
.countdown-box {
  margin: 16px auto 0;
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 10px 24px;
  border-radius: 12px;
  background: var(--u-surface-2);
}
.cd-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--u-text-3);
}
.cd-val {
  font-size: 28px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: #fbbf24;
}
.cd-val.danger {
  color: #f87171;
}
.chips {
  margin-top: 14px;
  justify-content: center;
}
.hint {
  font-size: 13px;
  color: var(--u-text-3);
  line-height: 1.5;
  margin: 0 0 20px;
  text-align: left;
}
.hint.ok {
  color: #15803d;
  text-align: center;
}
.hint.warn {
  color: #fcd34d;
}
.verify-notice {
  margin: 0 0 20px;
  padding: 14px 16px;
  border-radius: 14px;
  text-align: left;
  background: rgba(244, 63, 94, 0.08);
  border: 1px solid rgba(244, 63, 94, 0.22);
}
.verify-notice .vn-title {
  font-size: 13px;
  font-weight: 800;
  color: var(--u-primary);
  margin-bottom: 8px;
}
.verify-notice ul {
  margin: 0;
  padding-left: 1.15em;
}
.verify-notice li {
  font-size: 12.5px;
  color: var(--u-text-2);
  line-height: 1.55;
  margin-bottom: 6px;
}
.verify-notice li:last-child {
  margin-bottom: 0;
}
.verify-notice strong {
  color: var(--u-text);
  font-weight: 700;
}
.actions {
  display: flex;
  gap: 10px;
  justify-content: center;
  flex-wrap: wrap;
}
</style>
