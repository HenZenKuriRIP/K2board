<template>
  <div class="dash" v-loading="store.loading">
    <!-- 未订阅：强提示 -->
    <section v-if="!hasActivePlan" class="need-plan-banner glass-panel">
      <div class="npb-icon" aria-hidden="true">!</div>
      <div class="npb-main">
        <h2>尚未购买套餐，无法使用加速服务</h2>
        <p>
          注册账号后需要<strong>先选择下方套餐并完成支付</strong>，才会分配节点权限与订阅流量。
          未开通前，客户端即使导入链接也<strong>没有可用节点</strong>。
        </p>
        <el-button type="primary" size="large" class="npb-cta" @click="scrollToShop">
          立即选择套餐
        </el-button>
      </div>
    </section>

    <!-- 已过期：强提示 -->
    <section v-else-if="store.info?.expired" class="need-plan-banner warn glass-panel">
      <div class="npb-icon" aria-hidden="true">!</div>
      <div class="npb-main">
        <h2>套餐已过期，请续费或重新购买</h2>
        <p>
          当前「{{ activePlanName }}」已到期，节点将不可用。请续费本套餐或选购新套餐以恢复服务。
        </p>
      </div>
    </section>

    <!-- 当前生效套餐：最醒目 -->
    <section
      class="plan-hero glass-panel"
      :class="{
        expired: store.info?.expired,
        none: !hasActivePlan,
        active: hasActivePlan && !store.info?.expired,
      }"
    >
      <div class="plan-hero-glow" aria-hidden="true" />
      <div class="plan-hero-main">
        <div class="plan-status-row">
          <span class="status-pill" :class="statusClass">{{ statusLabel }}</span>
          <span v-if="hasActivePlan && !store.info?.expired" class="live-tag">当前生效套餐</span>
          <span v-if="store.info?.group_name && hasActivePlan" class="group-pill">{{ store.info.group_name }}</span>
        </div>
        <h1 class="plan-title">{{ activePlanName }}</h1>
        <p class="plan-sub">
          <template v-if="!hasActivePlan">
            {{ greet() }}，{{ displayName }} · 请先购买套餐后再复制订阅、连接节点
          </template>
          <template v-else-if="store.info?.expired">
            {{ greet() }}，{{ displayName }} · 套餐已过期，续费或新购后恢复
          </template>
          <template v-else>
            {{ greet() }}，{{ displayName }} · 以下为<strong>当前账户正在使用</strong>的套餐权益
          </template>
        </p>

        <div class="plan-stats" v-if="hasActivePlan">
          <div class="stat">
            <span class="stat-label">到期时间</span>
            <span class="stat-val">{{ store.info?.expire_text || '永久' }}</span>
          </div>
          <div class="stat">
            <span class="stat-label">{{ activeTrafficLabel }}</span>
            <span class="stat-val">
              {{ formatBytes(store.info?.traffic_used || 0) }}
              <em>/ {{ (store.info?.traffic_limit ?? 0) > 0 ? formatBytes(store.info!.traffic_limit) : '∞' }}</em>
            </span>
          </div>
          <div class="stat">
            <span class="stat-label">速率</span>
            <span class="stat-val">{{ store.info?.speed_limit ? store.info.speed_limit + ' Mbps' : '不限速' }}</span>
          </div>
          <div class="stat">
            <span class="stat-label">设备</span>
            <span class="stat-val">{{ store.info?.device_limit || '不限' }} 台</span>
          </div>
        </div>
        <div v-else class="no-plan-stats">
          <span>流量额度</span><b>未开通</b>
          <span>到期时间</span><b>未开通</b>
          <span>节点权限</span><b>无</b>
        </div>

        <div class="bar-row" v-if="hasActivePlan || (store.info?.traffic_limit ?? 0) > 0">
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: barWidth, background: barColor }" />
          </div>
          <span class="pct">{{ Math.min(store.info?.usage_percent || 0, 100).toFixed(1) }}%</span>
        </div>
        <p class="traffic-hint" v-if="hasActivePlan">{{ trafficCycleText }}</p>
      </div>

      <div class="plan-hero-actions">
        <el-button
          type="primary"
          size="large"
          class="cta"
          :disabled="!hasActivePlan || !!store.info?.expired"
          @click="copyUrl"
        >
          复制订阅链接
        </el-button>
        <el-button size="large" class="ghost-btn" @click="$router.push('/user/subscribe')">订阅管理</el-button>
        <el-button
          v-if="!hasActivePlan"
          type="warning"
          size="large"
          @click="scrollToShop"
        >
          去购买套餐
        </el-button>
        <el-button
          v-if="store.info?.can_renew && store.info?.renew_plan && store.info.renew_plan.show_on_shop"
          size="large"
          class="ghost-btn"
          :loading="renewing"
          @click="renewCurrentPlan"
        >
          {{ (store.info.renew_plan.price || 0) <= 0 ? '免费续期' : '续费本套餐' }}
        </el-button>
      </div>
    </section>

    <!-- Pending order -->
    <section v-if="pendingOrder" class="pending-banner glass-panel" @click="goPendingPay">
      <div class="pb-main">
        <span class="pb-tag">待支付</span>
        <div>
          <strong>{{ pendingOrder.plan_name }}</strong>
          <span class="pb-sub">
            {{ formatPrice(pendingOrder.total_amount, pendingOrder.currency) }}
            · 剩余 {{ pendingCdLabel }}
          </span>
          <BenefitChips :order="pendingOrder" compact class="pb-chips" />
        </div>
      </div>
      <div class="pb-actions" @click.stop>
        <el-button
          v-if="canReopenCashier(pendingOrder)"
          type="warning"
          size="small"
          @click="reopenPending"
        >
          打开收银台
        </el-button>
        <el-button type="primary" size="small" @click="goPendingPay">去支付</el-button>
      </div>
    </section>

    <!-- 导入订阅：仅已开通有效套餐时展示完整引导 -->
    <section class="glass-panel import-guide">
      <div class="block-head">
        <div>
          <h3>导入订阅 · 快速配置</h3>
          <p v-if="hasUnexpiredPlan">三步完成客户端接入，无需手动填写节点</p>
          <p v-else>购买套餐并支付成功后，再复制订阅链接导入客户端</p>
        </div>
        <div class="import-actions">
          <el-button
            type="primary"
            plain
            size="small"
            :disabled="!hasUnexpiredPlan"
            @click="copyUrl"
          >
            复制链接
          </el-button>
          <el-button text type="primary" size="small" @click="$router.push('/user/docs')">详细教程</el-button>
        </div>
      </div>
      <div v-if="hasUnexpiredPlan" class="import-steps">
        <div class="istep" v-for="(s, i) in steps" :key="i">
          <div class="istep-num">{{ i + 1 }}</div>
          <div>
            <strong>{{ s.t }}</strong>
            <p>{{ s.d }}</p>
          </div>
        </div>
      </div>
      <p v-else class="import-locked">未开通套餐时订阅链接无可用节点，购买后即可在此复制。</p>
    </section>

    <!-- 套餐商城：续费弱化整合在此 -->
    <section ref="shopRef" class="glass-panel block shop">
      <div class="block-head">
        <div>
          <h3>套餐商城</h3>
          <p>
            <template v-if="!hasActivePlan">选择套餐并支付后即可使用节点 · <b>未购买无法使用服务</b></template>
            <template v-else-if="store.info?.expired">续费或新购后恢复服务</template>
            <template v-else>可续费本套餐；购买其他套餐将按新套餐<strong>覆盖权益</strong>（见确认提示）</template>
          </p>
        </div>
        <el-button text type="primary" @click="$router.push('/user/orders')">我的订单</el-button>
      </div>

      <!-- 下架套餐续费：弱化提示，整合在商城内 -->
      <div
        v-if="store.info?.can_renew && store.info?.renew_plan && !store.info.renew_plan.show_on_shop"
        class="renew-soft"
      >
        <span class="renew-soft-text">
          当前套餐「{{ store.info.renew_plan.name }}」已停止商城销售，仍可续费
          <em>
            {{ formatPrice(store.info.renew_plan.price, store.info.renew_plan.currency) }}
            · {{ formatDur(store.info.renew_plan.duration) }}
          </em>
        </span>
        <el-button
          size="small"
          text
          type="primary"
          :loading="renewing"
          @click="renewCurrentPlan"
        >
          {{ (store.info.renew_plan.price || 0) <= 0 ? '免费续期' : '续费' }}
        </el-button>
      </div>

      <div class="plan-grid" v-if="plans.length > 0">
        <div
          v-for="p in plans"
          :key="p.id"
          class="plan u-lift"
          :class="{
            current: store.info?.plan_id === p.id && !store.info?.expired,
            other: hasActivePlan && store.info?.plan_id !== p.id,
          }"
        >
          <div class="plan-top">
            <span class="plan-name">{{ p.name }}</span>
            <span v-if="store.info?.plan_id === p.id && !store.info?.expired" class="cur-tag">当前生效</span>
            <span v-else-if="store.info?.plan_id === p.id && store.info?.expired" class="cur-tag expired-tag">已过期</span>
            <span v-else-if="hasActivePlan && !store.info?.expired" class="cur-tag other-tag">其他套餐</span>
          </div>
          <div class="plan-price">{{ formatPrice(p.price, p.currency) }}</div>
          <div class="plan-specs">
            <span>{{ formatPlanTrafficLine(p.traffic_limit, p.duration, p.reset_day) }}</span>
            <span>速率 {{ p.speed_limit > 0 ? p.speed_limit + 'M' : '不限' }}</span>
            <span>设备 {{ p.device_limit > 0 ? p.device_limit + '台' : '不限' }}</span>
            <span>周期 {{ formatDur(p.duration) }}</span>
          </div>
          <el-button
            :type="store.info?.plan_id === p.id ? 'success' : 'primary'"
            class="buy-btn"
            size="small"
            :loading="buyingId === p.id"
            @click="buyPlan(p)"
          >
            {{ buyButtonLabel(p) }}
          </el-button>
        </div>
      </div>
      <p v-else class="empty-shop">
        暂无上架套餐。
        <template v-if="store.info?.can_renew && store.info?.renew_plan && !store.info.renew_plan.show_on_shop">
          您仍可通过上方续费入口续订当前套餐。
        </template>
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import {
  getPlans, createOrder, checkoutOrder, listOrders,
  type PlanInfo, type OrderInfo,
} from '@/api/userApi'
import {
  formatBytes,
  formatPrice,
  parseExpireMs,
  formatCountdown,
  formatPlanTrafficLine,
  planTrafficLabel,
} from '@/utils/format'
import { userHasBoundPlan, userHasActiveService } from '@/utils/userService'
import { canReopenCashier } from '@/utils/order'
import BenefitChips from '@/components/BenefitChips.vue'

const store = useUserAuthStore()
const router = useRouter()
const plans = ref<PlanInfo[]>([])
const buyingId = ref(0)
const renewing = ref(false)
const pendingOrder = ref<OrderInfo | null>(null)
const nowTick = ref(Date.now())
const shopRef = ref<HTMLElement | null>(null)
let clock: ReturnType<typeof setInterval> | null = null

const barWidth = computed(() => Math.min(store.info?.usage_percent || 0, 100) + '%')
const barColor = computed(() => {
  const p = store.info?.usage_percent || 0
  if (p >= 90) return 'linear-gradient(90deg,#f87171,#ef4444)'
  if (p >= 70) return 'linear-gradient(90deg,#fbbf24,#f59e0b)'
  // primary bar follows theme (dark = rose/cyan like login)
  const dark = document.documentElement.getAttribute('data-theme') !== 'light'
  return dark
    ? 'linear-gradient(90deg,#f43f5e,#22d3ee)'
    : 'linear-gradient(90deg,#6366f1,#22d3ee)'
})
const displayName = computed(() => store.info?.email?.split('@')[0] || '用户')

/** 已绑定套餐（含过期）；新注册无套餐为 false */
const hasActivePlan = computed(() => userHasBoundPlan(store.info))

/** 仍在有效期内（未过期）——加购时需二次确认覆盖规则 */
const hasUnexpiredPlan = computed(() => userHasActiveService(store.info))

const activePlanName = computed(() => {
  if (!hasActivePlan.value) return '尚未开通套餐'
  const name = (store.info?.plan_name || '').trim()
  if (name) return name
  const g = (store.info?.group_name || '').trim()
  if (g && g !== '未分组' && g !== '-') return g
  return '已绑定套餐'
})

const statusLabel = computed(() => {
  if (!hasActivePlan.value) return '未订阅 · 请先购买'
  if (store.info?.expired) return '已过期 · 请续费'
  if ((store.info?.expire_at || 0) > 0) return '生效中'
  return '永久生效'
})

const statusClass = computed(() => {
  if (!hasActivePlan.value) return 'muted'
  if (store.info?.expired) return 'danger'
  return 'ok'
})

function scrollToShop() {
  shopRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function buyButtonLabel(p: PlanInfo) {
  const free = (p.price || 0) <= 0
  const myPlanId = Number(store.info?.plan_id) || 0
  // 新用户 / 无套餐：只显示立即购买，不出现「覆盖」
  if (myPlanId > 0 && myPlanId === p.id) {
    return free ? '免费续期' : '续费本套餐'
  }
  if (hasUnexpiredPlan.value) {
    return free ? '免费开通（将覆盖）' : '购买（将覆盖当前套餐）'
  }
  return free ? '免费开通' : '立即购买'
}

/** 未到期加购 / 续费前的说明（与后端 fulfill 覆盖逻辑一致） */
async function confirmPurchaseIfNeeded(p: PlanInfo, isRenewSame: boolean): Promise<boolean> {
  if (!hasUnexpiredPlan.value) return true
  if (isRenewSame) {
    try {
      await ElMessageBox.confirm(
        `您正在续费当前套餐「${p.name}」。\n\n` +
          `支付成功后将：\n` +
          `· 在原到期时间基础上延长本套餐时长\n` +
          `· 清零已用流量，并按本套餐额度重新计算\n` +
          `· 限速 / 设备数等与本套餐保持一致\n\n` +
          `是否继续？`,
        '确认续费',
        {
          type: 'info',
          confirmButtonText: '确认续费',
          cancelButtonText: '取消',
          dangerouslyUseHTMLString: false,
        },
      )
      return true
    } catch {
      return false
    }
  }
  try {
    await ElMessageBox.confirm(
      `您当前套餐「${activePlanName.value}」尚未到期。\n\n` +
        `若现在购买「${p.name}」，支付成功后将立即生效，并：\n` +
        `· 切换为新套餐（套餐名 / 权限组 / 流量额度 / 限速 / 设备数将被覆盖）\n` +
        `· 已用流量清零，按新套餐额度重新计算\n` +
        `· 到期时间在原到期基础上叠加新套餐时长（不是等旧套餐结束后再排队）\n\n` +
        `这与「只叠加时长、保留旧套餐流量规则」不同。是否仍要新购？`,
      '套餐尚未到期 · 新购将覆盖权益',
      {
        type: 'warning',
        confirmButtonText: '仍要新购',
        cancelButtonText: '再想想',
        dangerouslyUseHTMLString: false,
      },
    )
    return true
  } catch {
    return false
  }
}

const trafficCycleText = computed(() => {
  const d = store.info?.traffic_reset_day || 0
  if (d > 0) {
    return `每月流量配额 · 每月 ${d} 号自动清零已用流量`
  }
  // Long-period plans without reset_day still show as monthly in shop; usage is cumulative until expire
  const renewDur = store.info?.renew_plan?.duration || 0
  if (renewDur > 31 * 86400) {
    return '每月流量配额 · 以套餐周期内统计为准（见套餐说明）'
  }
  return '套餐配额 · 有效期内累计已用'
})

/** Hero stat label: 每月流量 vs 流量 */
const activeTrafficLabel = computed(() => {
  if ((store.info?.traffic_reset_day || 0) > 0) return '每月流量'
  const dur = store.info?.renew_plan?.duration
  return planTrafficLabel(dur, store.info?.traffic_reset_day)
})

const pendingCdLabel = computed(() => {
  const o = pendingOrder.value
  if (!o?.expired_at) return '—'
  const end = parseExpireMs(o.expired_at)
  const sec = end ? Math.max(0, Math.floor((end - nowTick.value) / 1000)) : 0
  if (sec <= 0) return '已到期'
  return formatCountdown(sec)
})

const steps = [
  { t: '复制订阅链接', d: '点击「复制订阅链接」按钮' },
  { t: '导入客户端', d: 'FlClash / 小火箭 / Surge 粘贴订阅（FlClash 用默认 Clash 链接）' },
  { t: '选择节点连接', d: '客户端自动更新节点，选择开启即可' },
]

function greet() {
  const h = new Date().getHours()
  return h < 12 ? '早上好' : h < 18 ? '下午好' : '晚上好'
}
function formatDur(sec: number) {
  if (!sec) return '-'
  const d = sec / 86400
  return d >= 365 ? (d / 365).toFixed(0) + '年' : Math.floor(d) + '天'
}

function goPendingPay() {
  if (!pendingOrder.value) return
  router.push({ name: 'UserOrderPay', params: { trade_no: pendingOrder.value.trade_no } })
}

function reopenPending() {
  const o = pendingOrder.value
  if (!o || !canReopenCashier(o) || !o.payment_url) {
    goPendingPay()
    return
  }
  window.location.href = o.payment_url
}

async function loadPending() {
  if (!store.token) return
  try {
    const r = await listOrders(store.token)
    const list = r.data || []
    pendingOrder.value = list.find((o) => o.status === 'pending') || null
    store.pendingOrderCount = list.filter((o) => o.status === 'pending').length
  } catch {
    pendingOrder.value = null
  }
}

async function placeOrder(planId: number, price: number, successFreeMsg: string) {
  if (!store.token) {
    ElMessage.warning('请先登录')
    return
  }
  const r = await createOrder(store.token, planId)
  if ((price || 0) <= 0) {
    // Free plans fulfill via Checkout amount=0 path (no payment gateway / no mock)
    await checkoutOrder(store.token, r.data.trade_no, 'free')
    ElMessage.success(successFreeMsg)
    await store.fetchInfo()
    await loadPending()
    return
  }
  router.push({ name: 'UserOrderPay', params: { trade_no: r.data.trade_no } })
}

async function buyPlan(p: PlanInfo) {
  const isCurrent = store.info?.plan_id === p.id
  const ok = await confirmPurchaseIfNeeded(p, isCurrent)
  if (!ok) return
  buyingId.value = p.id
  try {
    await placeOrder(
      p.id,
      p.price || 0,
      isCurrent ? '免费套餐已续期' : '免费套餐已开通',
    )
  } catch (e: any) {
    ElMessage.error(e?.message || '下单失败')
  } finally {
    buyingId.value = 0
  }
}

async function renewCurrentPlan() {
  const rp = store.info?.renew_plan
  if (!rp || !store.token) return
  const ok = await confirmPurchaseIfNeeded(rp as PlanInfo, true)
  if (!ok) return
  renewing.value = true
  try {
    await placeOrder(rp.id, rp.price || 0, '免费套餐已续期')
  } catch (e: any) {
    ElMessage.error(e?.message || '续费下单失败')
  } finally {
    renewing.value = false
  }
}

async function copyUrl() {
  if (!hasActivePlan.value || store.info?.expired) {
    ElMessage.warning('请先购买或续费有效套餐后再复制订阅链接')
    scrollToShop()
    return
  }
  if (!store.info?.subscribe_url) {
    ElMessage.warning('暂无订阅链接')
    return
  }
  let u = store.info.subscribe_url
  u = u.replace(/([?&])flag=[^&]*/g, '$1').replace(/[?&]$/, '').replace(/\?&/, '?')
  const sep = u.includes('?') ? '&' : '?'
  await navigator.clipboard.writeText(`${u}${sep}flag=clash`)
  ElMessage.success('已复制 FlClash/Clash 订阅链接')
}

onMounted(async () => {
  if (store.isLoggedIn) {
    await store.fetchInfo()
    try {
      const pr = await getPlans()
      plans.value = pr.data || []
    } catch { /* ignore */ }
    await loadPending()
    clock = setInterval(() => {
      nowTick.value = Date.now()
    }, 1000)
  }
})

onUnmounted(() => {
  if (clock) clearInterval(clock)
})
</script>

<style scoped>
.dash {
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: 100%;
}

.glass-panel { background: var(--u-surface); border: 1px solid var(--u-border); border-radius: 16px; box-shadow: 0 1px 2px rgba(15,23,42,0.05); position: relative; overflow: hidden; }
.glass-panel::before { display: none; content: none; }
.glass-panel > * {
  position: relative;
  z-index: 1;
}

.u-lift {
  transition: transform 0.22s cubic-bezier(0.22, 1, 0.36, 1),
    box-shadow 0.22s ease, border-color 0.2s ease;
}
.u-lift:hover {
  transform: translateY(-3px);
  border-color: rgba(165, 180, 252, 0.32);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
}

/* ── 未订阅 / 过期强提示 ── */
.need-plan-banner {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  padding: 20px 22px;
  border-color: #fbbf24 !important;
  background: linear-gradient(135deg, #fffbeb, #fff7ed) !important;
}
.need-plan-banner.warn {
  border-color: #f87171 !important;
  background: linear-gradient(135deg, #fef2f2, #fff1f2) !important;
}
.npb-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: #f59e0b;
  color: var(--u-text-inv);
  font-weight: 900;
  font-size: 20px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.need-plan-banner.warn .npb-icon {
  background: #ef4444;
}
.npb-main h2 {
  margin: 0 0 8px;
  font-size: 17px;
  font-weight: 800;
  color: #9a3412;
}
.need-plan-banner.warn .npb-main h2 {
  color: #b91c1c;
}
.npb-main p {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.55;
  color: #78350f;
}
.need-plan-banner.warn .npb-main p {
  color: #7f1d1d;
}
.npb-cta {
  font-weight: 700;
}
.live-tag {
  font-size: 11px;
  font-weight: 800;
  padding: 4px 10px;
  border-radius: 999px;
  background: #dcfce7;
  color: #15803d;
  border: 1px solid #bbf7d0;
}
.no-plan-stats {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 14px;
  font-size: 13px;
  color: var(--u-text-3);
  margin-bottom: 8px;
}
.no-plan-stats b {
  color: var(--u-text-3);
  font-weight: 700;
}

/* ── Plan Hero ── */
.plan-hero {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 24px;
  flex-wrap: wrap;
  padding: 28px 30px;
  border-color: rgba(99, 102, 241, 0.35) !important;
}
.plan-hero.active {
  border-color: rgba(22, 163, 74, 0.4) !important;
  box-shadow: 0 0 0 1px rgba(22, 163, 74, 0.08), 0 4px 20px rgba(22, 163, 74, 0.06);
}
.plan-hero.expired {
  border-color: rgba(248, 113, 113, 0.4) !important;
}
.plan-hero.none {
  border-color: var(--u-border) !important;
  opacity: 0.95;
}
.plan-hero-glow { display: none; }
.plan-hero.expired .plan-hero-glow { display: none; }
.plan-hero.none .plan-hero-glow { display: none; }
.plan-status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}
.status-pill {
  font-size: 12px;
  font-weight: 800;
  padding: 5px 12px;
  border-radius: 999px;
  letter-spacing: 0.02em;
}
.status-pill.ok {
  background: rgba(52, 211, 153, 0.18);
  color: #15803d;
  border: 1px solid rgba(52, 211, 153, 0.35);
  box-shadow: none;
}
.status-pill.danger {
  background: rgba(248, 113, 113, 0.18);
  color: #b91c1c;
  border: 1px solid rgba(248, 113, 113, 0.35);
}
.status-pill.muted {
  background: var(--u-surface-2);
  color: var(--u-text-3);
  border: 1px solid var(--u-border);
}
.group-pill {
  font-size: 12px;
  font-weight: 700;
  padding: 5px 12px;
  border-radius: 999px;
  background: var(--u-surface-2);
  color: var(--u-text-2);
  border: 1px solid var(--u-border);
}
.plan-title {
  margin: 0 0 6px;
  font-size: clamp(26px, 3.2vw, 36px);
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--u-text);
  text-shadow: none;
}
.plan-sub {
  margin: 0 0 18px;
  font-size: 13px;
  color: var(--u-text-3);
}
.plan-stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 18px;
  margin-bottom: 16px;
}
@media (min-width: 720px) {
  .plan-stats {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.stat-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--u-text-3);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.stat-val {
  font-size: 15px;
  font-weight: 800;
  color: var(--u-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.stat-val em {
  font-style: normal;
  font-weight: 600;
  color: var(--u-text-3);
  font-size: 13px;
}
.bar-row {
  display: flex;
  align-items: center;
  gap: 14px;
  max-width: 520px;
}
.bar-track {
  flex: 1;
  height: 12px;
  border-radius: 999px;
  background: var(--u-surface-2);
  overflow: hidden;
  border: 1px solid var(--u-border);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
}
.bar-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.7s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 0 22px rgba(99, 102, 241, 0.5);
}
.pct {
  min-width: 52px;
  text-align: right;
  font-size: 13px;
  font-weight: 700;
  color: var(--u-text-2);
}
.traffic-hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
}
.import-locked {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--u-text-3);
  line-height: 1.5;
}
.plan-hero-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  flex-shrink: 0;
}
.cta {
  min-width: 140px;
  box-shadow: 0 12px 32px rgba(99, 102, 241, 0.4) !important;
}
.ghost-btn {
  background: var(--u-surface) !important;
  border: 1px solid var(--u-border) !important;
  color: var(--u-text-2) !important;
  box-shadow: none !important;
}

/* Pending */
.pending-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  cursor: pointer;
  border-color: rgba(251, 191, 36, 0.35) !important;
  background: var(--u-primary-soft) !important;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}
.pending-banner:hover {
  transform: translateY(-2px);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.1) inset,
    0 16px 40px rgba(245, 158, 11, 0.15) !important;
}
.pb-main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}
.pb-tag {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 800;
  color: var(--u-text);
  background: var(--u-surface-2);
  padding: 3px 8px;
  border-radius: 999px;
  margin-top: 2px;
  box-shadow: 0 4px 12px rgba(245, 158, 11, 0.35);
}
.pb-main strong {
  display: block;
  color: var(--u-text);
  font-size: 14px;
}
.pb-sub {
  display: block;
  margin-top: 2px;
  font-size: 12px;
  color: #fbbf24;
  font-variant-numeric: tabular-nums;
}
.pb-chips {
  margin-top: 8px;
}
.pb-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}

/* Import guide */
.import-guide {
  padding: 22px 24px;
}
.block-head {
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}
.block-head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text);
}
.block-head p {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
}
.import-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.import-steps {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
}
@media (min-width: 800px) {
  .import-steps {
    grid-template-columns: repeat(3, 1fr);
  }
}
.istep {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 14px 16px;
  border-radius: 14px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
}
.istep-num {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: var(--u-text);
  background: var(--u-surface-2);
  flex-shrink: 0;
  box-shadow: 0 8px 20px rgba(99, 102, 241, 0.35);
}
.istep strong {
  font-size: 14px;
  color: var(--u-text);
}
.istep p {
  margin: 3px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.45;
}

/* Shop */
.block {
  padding: 24px 26px;
}
.renew-soft {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
  padding: 10px 14px;
  border-radius: 12px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
}
.renew-soft-text {
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.5;
}
.renew-soft-text em {
  font-style: normal;
  color: var(--u-primary);
  margin-left: 6px;
}
.empty-shop {
  margin: 0;
  font-size: 13px;
  color: var(--u-text-3);
  line-height: 1.55;
}
.plan-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
}
.plan {
  padding: 18px;
  border-radius: 16px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.05) inset,
    0 8px 20px rgba(0, 0, 0, 0.18);
  transition: border-color 0.2s ease, transform 0.2s ease, box-shadow 0.2s ease;
}
.plan:hover {
  transform: translateY(-3px);
  border-color: rgba(129, 140, 248, 0.4);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
}
.plan.current {
  border-color: rgba(22, 163, 74, 0.55);
  background: linear-gradient(165deg, #f0fdf4, var(--u-surface-2));
  box-shadow:
    0 0 0 1px rgba(22, 163, 74, 0.12),
    0 12px 28px rgba(52, 211, 153, 0.12);
}
.plan.other {
  border-style: dashed;
  opacity: 0.92;
}
.plan-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  gap: 8px;
  flex-wrap: wrap;
}
.plan-name {
  font-size: 15px;
  font-weight: 800;
  color: var(--u-text);
}
.cur-tag {
  font-size: 11px;
  font-weight: 700;
  color: #15803d;
  background: rgba(52, 211, 153, 0.2);
  padding: 3px 8px;
  border-radius: 999px;
  border: 1px solid rgba(52, 211, 153, 0.3);
}
.cur-tag.other-tag {
  color: var(--u-text-3);
  background: var(--u-bg-soft);
  border-color: var(--u-border);
}
.cur-tag.expired-tag {
  color: #b91c1c;
  background: #fee2e2;
  border-color: #fecaca;
}
.plan-price {
  font-size: 22px;
  font-weight: 800;
  color: var(--u-primary);
  margin-bottom: 10px;
  letter-spacing: -0.02em;
}
.plan-specs {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12px;
  color: var(--u-text-3);
  margin-bottom: 14px;
}
.buy-btn {
  width: 100%;
}

@media (max-width: 560px) {
  .plan-hero { padding: 20px 16px; }
  .block, .import-guide { padding: 18px; }
  .pending-banner {
    flex-direction: column;
    align-items: stretch;
  }
  .plan-hero-actions {
    width: 100%;
  }
  .plan-hero-actions .el-button {
    flex: 1;
  }
}
</style>
