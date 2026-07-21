<template>
  <div class="page" v-loading="loading">
    <div class="page-head">
      <div>
        <h2>我的订单</h2>
        <p>待支付可倒计时续付或取消 · 已支付可查看开通权益</p>
      </div>
      <el-button type="primary" @click="$router.push('/user')">去商城</el-button>
    </div>

    <div class="tabs glass">
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        class="tab"
        :class="{ active: filter === t.key }"
        @click="filter = t.key"
      >
        {{ t.label }}
        <span v-if="t.key === 'pending' && pendingCount" class="badge">{{ pendingCount }}</span>
        <span v-else-if="t.key !== 'pending' && countMap[t.key]" class="badge mute">{{ countMap[t.key] }}</span>
      </button>
    </div>

    <EmptyState
      v-if="filtered.length === 0 && !loading"
      :variant="emptyVariant"
      :title="emptyTitle"
      :description="emptyText"
      class="glass"
    >
      <el-button type="primary" @click="$router.push('/user')">浏览套餐</el-button>
      <el-button v-if="filter !== 'all'" @click="filter = 'all'">查看全部</el-button>
    </EmptyState>

    <div class="list">
      <article
        v-for="o in filtered"
        :key="o.trade_no"
        class="card glass"
        :class="o.status"
        @click="goPay(o.trade_no)"
      >
        <div class="card-top">
          <div class="left">
            <div class="plan">{{ o.plan_name }}</div>
            <div class="trade">{{ o.trade_no }}</div>
          </div>
          <div class="right">
            <span class="st" :class="o.status">{{ orderStatusLabel(o.status) }}</span>
            <div class="amt">{{ formatPrice(o.total_amount, o.currency) }}</div>
          </div>
        </div>

        <BenefitChips v-if="o.status === 'paid' || o.status === 'pending'" :order="o" compact class="chips-row" />

        <div class="meta">
          <div class="row">
            <span>创建</span>
            <b>{{ formatISODate(o.created_at) }}</b>
          </div>
          <div class="row" v-if="o.payment_method">
            <span>支付方式</span>
            <b>{{ userFacingMethodName(o.payment_method) }}</b>
          </div>
          <div class="row" v-if="o.status === 'pending'">
            <span>剩余时间</span>
            <b class="cd" :class="{ danger: (remainMap[o.trade_no] ?? 0) <= 60 }">
              {{ countdownLabel(o) }}
            </b>
          </div>
          <div class="row" v-if="o.status === 'paid' && o.paid_at">
            <span>支付时间</span>
            <b>{{ formatISODate(o.paid_at) }}</b>
          </div>
          <div class="row" v-if="o.status === 'cancelled' && cancelReasonLabel(o.remark)">
            <span>原因</span>
            <b>{{ cancelReasonLabel(o.remark) }}</b>
          </div>
        </div>

        <div class="actions" @click.stop>
          <template v-if="o.status === 'pending'">
            <el-button
              v-if="canReopenCashier(o)"
              type="warning"
              size="small"
              @click="reopenCashier(o)"
            >
              打开收银台
            </el-button>
            <el-button type="primary" size="small" @click="goPay(o.trade_no)">继续支付</el-button>
            <el-button size="small" :loading="cancelling === o.trade_no" @click="onCancel(o)">
              取消
            </el-button>
          </template>
          <template v-else>
            <el-button size="small" type="primary" plain @click="goPay(o.trade_no)">
              {{ o.status === 'paid' ? '查看权益' : '查看详情' }}
            </el-button>
          </template>
        </div>
      </article>
    </div>

    <p class="footnote">
      手动取消后迟到到账不会自动开通；超时关闭订单在到账后可能自动补开通。第三方收银台状态可能延迟更新。
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import { listOrders, cancelOrder, type OrderInfo } from '@/api/userApi'
import {
  formatPrice,
  formatISODate,
  orderStatusLabel,
  parseExpireMs,
  formatCountdown,
} from '@/utils/format'
import {
  CANCEL_POLICY_HTML,
  canReopenCashier,
  cancelReasonLabel,
  userFacingMethodName,
} from '@/utils/order'
import EmptyState from '@/components/EmptyState.vue'
import BenefitChips from '@/components/BenefitChips.vue'

const store = useUserAuthStore()
const router = useRouter()
const route = useRoute()
const loading = ref(false)
const orders = ref<OrderInfo[]>([])
const filter = ref<'all' | 'pending' | 'paid' | 'cancelled'>('all')
const cancelling = ref('')
const nowTick = ref(Date.now())
const refreshedExpired = new Set<string>()
let clock: ReturnType<typeof setInterval> | null = null

const tabs = [
  { key: 'all' as const, label: '全部' },
  { key: 'pending' as const, label: '待支付' },
  { key: 'paid' as const, label: '已支付' },
  { key: 'cancelled' as const, label: '已取消' },
]

const pendingCount = computed(() => orders.value.filter((o) => o.status === 'pending').length)

const countMap = computed(() => ({
  all: orders.value.length,
  pending: pendingCount.value,
  paid: orders.value.filter((o) => o.status === 'paid').length,
  cancelled: orders.value.filter((o) => o.status === 'cancelled' || o.status === 'failed').length,
}))

const filtered = computed(() => {
  if (filter.value === 'all') return orders.value
  if (filter.value === 'cancelled') {
    return orders.value.filter((o) => o.status === 'cancelled' || o.status === 'failed')
  }
  return orders.value.filter((o) => o.status === filter.value)
})

const emptyVariant = computed(() => {
  if (filter.value === 'pending') return 'pending' as const
  if (filter.value === 'paid') return 'paid' as const
  if (filter.value === 'cancelled') return 'cancelled' as const
  return 'all' as const
})

const emptyTitle = computed(() => {
  if (filter.value === 'pending') return '没有待支付订单'
  if (filter.value === 'paid') return '还没有已支付订单'
  if (filter.value === 'cancelled') return '没有已取消订单'
  return '订单空空如也'
})

const emptyText = computed(() => {
  if (filter.value === 'pending') return '去商城选购套餐后，待支付订单会出现在这里，可倒计时续付。'
  if (filter.value === 'paid') return '支付成功后，这里会展示套餐与开通权益摘要。'
  if (filter.value === 'cancelled') return '超时或手动取消的订单会归到这里。'
  return '选购套餐后，订单进度、倒计时与支付结果都会在这里汇总。'
})

const remainMap = computed(() => {
  const m: Record<string, number> = {}
  const now = nowTick.value
  for (const o of orders.value) {
    if (o.status !== 'pending') continue
    const end = parseExpireMs(o.expired_at)
    m[o.trade_no] = end ? Math.max(0, Math.floor((end - now) / 1000)) : 0
  }
  return m
})

function countdownLabel(o: OrderInfo) {
  const sec = remainMap.value[o.trade_no]
  if (sec == null) return '—'
  if (sec <= 0) return '已到期'
  return formatCountdown(sec)
}

function goPay(tradeNo: string) {
  router.push({ name: 'UserOrderPay', params: { trade_no: tradeNo } })
}

function reopenCashier(o: OrderInfo) {
  if (!canReopenCashier(o) || !o.payment_url) {
    ElMessage.warning('收银台链接不可用，请进入详情重新发起支付')
    goPay(o.trade_no)
    return
  }
  window.location.href = o.payment_url
}

async function load(silent = false) {
  if (!store.token) return
  if (!silent) loading.value = true
  try {
    const r = await listOrders(store.token)
    orders.value = r.data || []
    store.pendingOrderCount = pendingCount.value
  } catch (e: any) {
    if (!silent) {
      ElMessage.error(e?.message || '加载订单失败')
      orders.value = []
    }
  } finally {
    if (!silent) loading.value = false
  }
}

async function onCancel(o: OrderInfo) {
  if (!store.token) return
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
  cancelling.value = o.trade_no
  try {
    await cancelOrder(store.token, o.trade_no)
    ElMessage.success('订单已取消')
    await load()
    await store.refreshPendingCount()
  } catch (e: any) {
    ElMessage.error(e?.message || '取消失败')
    await load()
  } finally {
    cancelling.value = ''
  }
}

// Deep-link ?tab=pending
watch(
  () => route.query.tab,
  (t) => {
    if (t === 'pending' || t === 'paid' || t === 'cancelled' || t === 'all') {
      filter.value = t
    }
  },
  { immediate: true },
)

onMounted(async () => {
  if (!store.isLoggedIn) {
    router.replace('/user/login')
    return
  }
  await load()
  clock = setInterval(() => {
    nowTick.value = Date.now()
    let need = false
    for (const o of orders.value) {
      if (o.status !== 'pending') continue
      const end = parseExpireMs(o.expired_at)
      const sec = end ? Math.max(0, Math.floor((end - Date.now()) / 1000)) : 0
      if (sec <= 0 && !refreshedExpired.has(o.trade_no)) {
        refreshedExpired.add(o.trade_no)
        need = true
      }
    }
    if (need) load(true)
  }, 1000)
})

onUnmounted(() => {
  if (clock) clearInterval(clock)
})
</script>

<style scoped>
.page {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}
.page-head h2 {
  margin: 0;
  font-size: clamp(22px, 2.4vw, 28px);
  font-weight: 800;
  color: var(--u-text);
  letter-spacing: -0.03em;
}
.page-head p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--u-text-3);
}
.glass {
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  border-radius: 18px;
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
.tabs {
  display: flex;
  gap: 4px;
  padding: 6px;
  flex-wrap: wrap;
}
.tab {
  border: none;
  background: transparent;
  color: var(--u-text-3);
  font-size: 13px;
  font-weight: 600;
  padding: 8px 14px;
  border-radius: 12px;
  cursor: pointer;
  font-family: inherit;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
}
.tab:hover {
  color: var(--u-text-2);
  background: var(--u-surface-2);
}
.tab.active {
  color: var(--u-text);
  background: var(--u-surface-2);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.1) inset,
    0 6px 16px rgba(99, 102, 241, 0.22);
}
.badge {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: #f59e0b;
  color: var(--u-text);
  font-size: 11px;
  font-weight: 800;
  display: inline-grid;
  place-items: center;
}
.badge.mute {
  background: rgba(148, 163, 184, 0.25);
  color: var(--u-text-2);
}
.list {
  display: grid;
  grid-template-columns: 1fr;
  gap: 14px;
}
.card {
  padding: 18px 20px;
  cursor: pointer;
  transition: border-color 0.18s ease, transform 0.18s ease, box-shadow 0.18s ease;
}
.card:hover {
  border-color: rgba(129, 140, 248, 0.4);
  transform: translateY(-3px);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
}
.card.pending {
  border-color: rgba(251, 191, 36, 0.28);
}
.card.paid {
  border-color: rgba(52, 211, 153, 0.22);
}
.card-top {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.plan {
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text);
}
.trade {
  margin-top: 4px;
  font-size: 11px;
  color: var(--u-text-3);
  word-break: break-all;
}
.right {
  text-align: right;
  flex-shrink: 0;
}
.st {
  display: inline-block;
  font-size: 12px;
  font-weight: 700;
  padding: 4px 10px;
  border-radius: 999px;
  margin-bottom: 6px;
}
.st.pending {
  color: #fbbf24;
  background: rgba(251, 191, 36, 0.12);
}
.st.paid {
  color: #34d399;
  background: rgba(52, 211, 153, 0.12);
}
.st.cancelled,
.st.failed {
  color: var(--u-text-3);
  background: rgba(148, 163, 184, 0.12);
}
.amt {
  font-size: 18px;
  font-weight: 800;
  color: var(--u-primary-strong);
}
.chips-row {
  margin-bottom: 10px;
}
.meta {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  margin-bottom: 12px;
}
.row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: var(--u-text-3);
}
.row b {
  color: var(--u-text-2);
  font-weight: 600;
  text-align: right;
}
.cd {
  font-variant-numeric: tabular-nums;
  color: #fbbf24 !important;
}
.cd.danger {
  color: #f87171 !important;
}
.actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  padding-top: 12px;
}
.footnote {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--u-text-2);
  line-height: 1.5;
  text-align: center;
}

@media (min-width: 900px) {
  .list {
    grid-template-columns: repeat(2, 1fr);
  }
  .meta {
    grid-template-columns: 1fr 1fr;
  }
}
@media (min-width: 1280px) {
  .list {
    grid-template-columns: repeat(3, 1fr);
  }
}
</style>
