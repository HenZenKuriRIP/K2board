<template>
  <div class="page" v-loading="loading">
    <div class="page-head">
      <div>
        <h2>推广返佣</h2>
        <p>分享邀请链接，好友付费后按比例获得佣金</p>
      </div>
    </div>

    <el-alert
      v-if="overview && !overview.enable"
      type="warning"
      :closable="false"
      show-icon
      title="推广功能暂未开启"
      description="请稍后再试或联系管理员"
      class="mb"
    />

    <template v-if="overview">
      <div class="stats">
        <div class="stat glass">
          <span class="k">可提现余额</span>
          <b class="v money">{{ formatPrice(overview.balance) }}</b>
        </div>
        <div class="stat glass">
          <span class="k">累计佣金</span>
          <b class="v">{{ formatPrice(overview.commission_total) }}</b>
        </div>
        <div class="stat glass">
          <span class="k">邀请人数</span>
          <b class="v">{{ overview.invitee_count }}</b>
        </div>
        <div class="stat glass">
          <span class="k">返佣比例</span>
          <b class="v">{{ overview.rate_percent }}%</b>
        </div>
      </div>

      <section class="glass card">
        <h3>我的邀请</h3>
        <p class="desc">好友通过以下链接或邀请码注册后，其每笔实付订单将按 {{ overview.rate_percent }}% 返佣给你</p>
        <div class="invite-row">
          <div class="field">
            <span class="label">邀请码</span>
            <div class="value-row">
              <code class="code">{{ overview.invite_code }}</code>
              <el-button size="small" @click="copy(overview.invite_code)">复制</el-button>
            </div>
          </div>
          <div class="field">
            <span class="label">邀请链接</span>
            <div class="value-row">
              <el-input :model-value="overview.invite_url" readonly size="large" />
              <el-button type="primary" size="large" @click="copy(overview.invite_url)">复制链接</el-button>
            </div>
          </div>
        </div>
      </section>

      <section class="glass card" v-if="overview.balance > 0 || overview.pending_withdraw > 0 || overview.enable">
        <h3>申请提现</h3>
        <p class="desc">
          最低提现 {{ formatPrice(overview.min_withdraw) }}
          <span v-if="overview.pending_withdraw > 0">
            · 审核中冻结 {{ formatPrice(overview.pending_withdraw) }}
          </span>
          <span v-if="!overview.enable"> · 推广已关闭，仍可提现已有余额</span>
        </p>
        <el-form label-position="top" class="w-form" @submit.prevent>
          <div class="form-grid">
            <el-form-item label="提现金额（元）">
              <el-input-number
                v-model="wYuan"
                :min="0"
                :precision="2"
                :step="10"
                controls-position="right"
                style="width: 100%"
              />
            </el-form-item>
            <el-form-item label="收款方式">
              <el-select v-model="wMethod" placeholder="选择方式" style="width: 100%">
                <el-option
                  v-for="m in overview.payout_methods || []"
                  :key="m.code"
                  :label="m.name"
                  :value="m.code"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="收款账号">
              <el-input v-model="wAccount" placeholder="支付宝/微信账号、USDT 地址或银行卡号" />
            </el-form-item>
            <el-form-item label="收款姓名（可选）">
              <el-input v-model="wName" placeholder="真实姓名，便于核对" />
            </el-form-item>
          </div>
          <el-button type="primary" size="large" :loading="submitting" :disabled="!canWithdraw" @click="submitWithdraw">
            提交提现申请
          </el-button>
        </el-form>
      </section>

      <el-tabs v-model="tab" class="tabs">
        <el-tab-pane label="佣金明细" name="ledgers">
          <el-table :data="ledgers" v-loading="listLoading" empty-text="暂无佣金记录" class="tbl">
            <el-table-column label="时间" min-width="150">
              <template #default="{ row }">{{ formatISODate(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="来源用户" min-width="120" show-overflow-tooltip>
              <template #default="{ row }">{{ row.from_user_email || ('UID ' + row.from_user_id) }}</template>
            </el-table-column>
            <el-table-column prop="trade_no" label="订单号" min-width="160" show-overflow-tooltip />
            <el-table-column label="订单金额" width="100" align="right">
              <template #default="{ row }">{{ formatPrice(row.order_amount) }}</template>
            </el-table-column>
            <el-table-column label="比例" width="70" align="center">
              <template #default="{ row }">{{ row.rate_percent }}%</template>
            </el-table-column>
            <el-table-column label="佣金" width="100" align="right">
              <template #default="{ row }"><b class="pos">{{ formatPrice(row.amount) }}</b></template>
            </el-table-column>
          </el-table>
          <div class="pager" v-if="ledgerTotal > 20">
            <el-pagination
              layout="prev, pager, next"
              :total="ledgerTotal"
              :page-size="20"
              v-model:current-page="ledgerPage"
              @current-change="loadLedgers"
            />
          </div>
        </el-tab-pane>

        <el-tab-pane label="提现记录" name="withdraws">
          <el-table :data="withdraws" v-loading="listLoading" empty-text="暂无提现记录" class="tbl">
            <el-table-column label="申请时间" min-width="150">
              <template #default="{ row }">{{ formatISODate(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="金额" width="100" align="right">
              <template #default="{ row }">{{ formatPrice(row.amount) }}</template>
            </el-table-column>
            <el-table-column label="方式" width="100">
              <template #default="{ row }">{{ methodLabel(row.method) }}</template>
            </el-table-column>
            <el-table-column prop="account" label="账号" min-width="140" show-overflow-tooltip />
            <el-table-column label="状态" width="100" align="center">
              <template #default="{ row }">
                <span class="st" :class="row.status">{{ withdrawStatusLabel(row.status) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="admin_remark" label="备注" min-width="120" show-overflow-tooltip />
          </el-table>
          <div class="pager" v-if="withdrawTotal > 20">
            <el-pagination
              layout="prev, pager, next"
              :total="withdrawTotal"
              :page-size="20"
              v-model:current-page="withdrawPage"
              @current-change="loadWithdraws"
            />
          </div>
        </el-tab-pane>

        <el-tab-pane label="邀请的用户" name="invitees">
          <el-table :data="invitees" v-loading="listLoading" empty-text="还没有邀请到用户" class="tbl">
            <el-table-column prop="email" label="邮箱" min-width="160" />
            <el-table-column label="注册时间" min-width="150">
              <template #default="{ row }">{{ formatISODate(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90" align="center">
              <template #default="{ row }">
                <span class="st" :class="row.enable ? 'paid' : 'cancelled'">{{ row.enable ? '正常' : '禁用' }}</span>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import {
  getReferral,
  listReferralLedgers,
  listReferralWithdrawals,
  listInvitees,
  createWithdraw,
  type ReferralOverview,
  type CommissionLedger,
  type CommissionWithdraw,
} from '@/api/userApi'
import { formatPrice, formatISODate } from '@/utils/format'

const store = useUserAuthStore()
const loading = ref(true)
const listLoading = ref(false)
const overview = ref<ReferralOverview | null>(null)
const tab = ref('ledgers')

const ledgers = ref<CommissionLedger[]>([])
const ledgerTotal = ref(0)
const ledgerPage = ref(1)
const withdraws = ref<CommissionWithdraw[]>([])
const withdrawTotal = ref(0)
const withdrawPage = ref(1)
const invitees = ref<{ id: number; email: string; created_at: string; plan_id: number; enable: boolean }[]>([])

const wYuan = ref(0)
const wMethod = ref('')
const wAccount = ref('')
const wName = ref('')
const submitting = ref(false)

function yuanToCents(yuan: number): number {
  // Avoid float artifacts (e.g. 19.99 * 100)
  return Math.round((Number(yuan) || 0) * 100 + Number.EPSILON)
}

const canWithdraw = computed(() => {
  if (!overview.value) return false
  // Allow cash-out even when referral is disabled (accrued balance)
  const cents = yuanToCents(wYuan.value)
  return (
    cents >= overview.value.min_withdraw &&
    cents <= overview.value.balance &&
    !!wMethod.value &&
    !!wAccount.value.trim()
  )
})

function methodLabel(code: string) {
  const m = overview.value?.payout_methods?.find((x) => x.code === code)
  return m?.name || code
}

function withdrawStatusLabel(s: string) {
  const map: Record<string, string> = {
    pending: '待审核',
    paid: '已打款',
    approved: '已通过',
    rejected: '已驳回',
  }
  return map[s] || s
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

async function loadOverview() {
  loading.value = true
  try {
    const res = await getReferral(store.token)
    overview.value = res.data
    if (!wMethod.value && res.data.payout_methods?.length) {
      wMethod.value = res.data.payout_methods[0].code
    }
    // suggest min withdraw if balance enough
    if (res.data.balance >= res.data.min_withdraw) {
      wYuan.value = res.data.min_withdraw / 100
    }
  } catch {
    ElMessage.error('加载推广信息失败')
  }
  loading.value = false
}

async function loadLedgers() {
  listLoading.value = true
  try {
    const res = await listReferralLedgers(store.token, ledgerPage.value)
    ledgers.value = res.data.list || []
    ledgerTotal.value = res.data.total || 0
  } catch { /* */ }
  listLoading.value = false
}

async function loadWithdraws() {
  listLoading.value = true
  try {
    const res = await listReferralWithdrawals(store.token, withdrawPage.value)
    withdraws.value = res.data.list || []
    withdrawTotal.value = res.data.total || 0
  } catch { /* */ }
  listLoading.value = false
}

async function loadInvitees() {
  listLoading.value = true
  try {
    const res = await listInvitees(store.token)
    invitees.value = res.data.list || []
  } catch { /* */ }
  listLoading.value = false
}

async function submitWithdraw() {
  if (!canWithdraw.value || !overview.value) return
  const cents = yuanToCents(wYuan.value)
  submitting.value = true
  try {
    await createWithdraw(store.token, cents, wMethod.value, wAccount.value.trim(), wName.value.trim())
    ElMessage.success('提现申请已提交，请等待审核')
    wAccount.value = ''
    wName.value = ''
    await loadOverview()
    await loadWithdraws()
    tab.value = 'withdraws'
  } catch (e: any) {
    ElMessage.error(e?.message || '提交失败')
  }
  submitting.value = false
}

watch(tab, (t) => {
  if (t === 'ledgers') loadLedgers()
  else if (t === 'withdraws') loadWithdraws()
  else if (t === 'invitees') loadInvitees()
})

onMounted(async () => {
  await loadOverview()
  await loadLedgers()
})
</script>

<style scoped>
.page { max-width: 960px; margin: 0 auto; }
.page-head { margin-bottom: 18px; }
.page-head h2 { margin: 0; font-size: 22px; font-weight: 800; color: var(--u-text); }
.page-head p { margin: 6px 0 0; font-size: 13px; color: var(--u-text-3); }
.mb { margin-bottom: 16px; }
.stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  margin-bottom: 16px;
}
@media (min-width: 720px) {
  .stats { grid-template-columns: repeat(4, 1fr); }
}
.stat {
  padding: 16px;
  border-radius: 14px;
  border: 1px solid var(--u-border);
  background: var(--u-surface);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.stat .k { font-size: 12px; color: var(--u-text-3); font-weight: 600; }
.stat .v { font-size: 20px; font-weight: 800; color: var(--u-text); letter-spacing: -0.02em; }
.stat .money { color: #059669; }
.card {
  padding: 20px;
  border-radius: 14px;
  border: 1px solid var(--u-border);
  background: var(--u-surface);
  margin-bottom: 16px;
}
.card h3 { margin: 0 0 6px; font-size: 16px; font-weight: 800; color: var(--u-text); }
.desc { margin: 0 0 16px; font-size: 13px; color: var(--u-text-3); line-height: 1.5; }
.invite-row { display: flex; flex-direction: column; gap: 14px; }
.field .label { display: block; font-size: 12px; font-weight: 600; color: var(--u-text-3); margin-bottom: 6px; }
.value-row { display: flex; gap: 8px; align-items: center; }
.code {
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0.12em;
  color: var(--u-primary);
  background: var(--u-primary-soft);
  border: 1px solid var(--u-border-glow);
  padding: 8px 14px;
  border-radius: 10px;
}
.form-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 4px 16px;
}
@media (min-width: 640px) {
  .form-grid { grid-template-columns: 1fr 1fr; }
}
.tabs { margin-top: 8px; }
.tbl { width: 100%; }
.pager { display: flex; justify-content: center; margin-top: 12px; }
.pos { color: #059669; }
.st {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}
.st.pending { background: #fffbeb; color: #b45309; }
.st.paid, .st.approved { background: #ecfdf5; color: #047857; }
.st.rejected, .st.cancelled { background: #fef2f2; color: #b91c1c; }
.glass { /* inherit shell glass if any */ }
</style>
