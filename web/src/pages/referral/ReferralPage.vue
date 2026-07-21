<template>
  <div class="ref-page">
    <div class="k2-page-header">
      <div>
        <h3>推广管理</h3>
        <p class="sub">提现审核 · 佣金流水 · 返佣规则见系统设置</p>
      </div>
      <div class="header-actions">
        <el-button size="large" @click="goSettings">推广设置</el-button>
        <el-button size="large" :loading="loading" @click="reload">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <div class="cfg-bar" v-if="cfg">
      <span class="pill" :class="cfg.enable ? 'on' : 'off'">{{ cfg.enable ? '推广已开启' : '推广已关闭' }}</span>
      <span class="pill mute">返佣 {{ cfg.rate_percent }}%</span>
      <span class="pill mute">最低提现 ¥{{ (cfg.min_withdraw / 100).toFixed(2) }}</span>
    </div>

    <el-tabs v-model="tab" @tab-change="onTab">
      <el-tab-pane label="提现审核" name="withdraws">
        <div class="toolbar">
          <el-select v-model="wStatus" clearable placeholder="全部状态" style="width: 140px" @change="loadWithdraws">
            <el-option value="pending" label="待审核" />
            <el-option value="paid" label="已打款" />
            <el-option value="rejected" label="已驳回" />
          </el-select>
        </div>
        <div class="table-shell">
          <el-table :data="withdraws" v-loading="loading" class="ref-table">
            <el-table-column label="ID" prop="id" width="70" />
            <el-table-column label="用户" min-width="160" show-overflow-tooltip>
              <template #default="{ row }">
                <div>{{ row.user_email || '—' }}</div>
                <div class="muted">UID {{ row.user_id }}</div>
              </template>
            </el-table-column>
            <el-table-column label="金额" width="110" align="right">
              <template #default="{ row }">
                <b>{{ formatPrice(row.amount) }}</b>
              </template>
            </el-table-column>
            <el-table-column label="收款" min-width="180">
              <template #default="{ row }">
                <div>{{ methodName(row.method) }}</div>
                <div class="muted">{{ row.account }}</div>
                <div v-if="row.account_name" class="muted">{{ row.account_name }}</div>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="100" align="center">
              <template #default="{ row }">
                <span class="st" :class="row.status">{{ statusLabel(row.status) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="申请时间" width="160">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="备注" prop="admin_remark" min-width="100" show-overflow-tooltip />
            <el-table-column label="操作" width="200" fixed="right">
              <template #default="{ row }">
                <template v-if="row.status === 'pending'">
                  <el-button size="small" type="primary" @click="approve(row)">通过打款</el-button>
                  <el-button size="small" type="danger" plain @click="reject(row)">驳回</el-button>
                </template>
                <span v-else class="muted">—</span>
              </template>
            </el-table-column>
          </el-table>
          <div class="pager">
            <el-pagination
              layout="total, prev, pager, next"
              :total="wTotal"
              :page-size="20"
              v-model:current-page="wPage"
              @current-change="loadWithdraws"
            />
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="佣金流水" name="ledgers">
        <div class="table-shell">
          <el-table :data="ledgers" v-loading="loading" class="ref-table">
            <el-table-column label="ID" prop="id" width="70" />
            <el-table-column label="推广人" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <div>{{ row.user_email || '—' }}</div>
                <div class="muted">UID {{ row.user_id }}</div>
              </template>
            </el-table-column>
            <el-table-column label="付费用户" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <div>{{ row.from_user_email || '—' }}</div>
                <div class="muted">UID {{ row.from_user_id }}</div>
              </template>
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
            <el-table-column label="时间" width="160">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
          <div class="pager">
            <el-pagination
              layout="total, prev, pager, next"
              :total="lTotal"
              :page-size="20"
              v-model:current-page="lPage"
              @current-change="loadLedgers"
            />
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import request from '@/api/request'

const router = useRouter()
const loading = ref(false)
const tab = ref('withdraws')
const cfg = ref<{ enable: boolean; rate_percent: number; min_withdraw: number; methods: { code: string; name: string }[] } | null>(null)

const withdraws = ref<any[]>([])
const wTotal = ref(0)
const wPage = ref(1)
const wStatus = ref('pending')

const ledgers = ref<any[]>([])
const lTotal = ref(0)
const lPage = ref(1)

function formatPrice(cents: number) {
  return `¥${((cents || 0) / 100).toFixed(2)}`
}
function formatTime(iso: string) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-CN')
}
function statusLabel(s: string) {
  return ({ pending: '待审核', paid: '已打款', approved: '已通过', rejected: '已驳回' } as Record<string, string>)[s] || s
}
function methodName(code: string) {
  const m = cfg.value?.methods?.find((x) => x.code === code)
  return m?.name || code
}

async function loadCfg() {
  try {
    const res = await request.get('/admin/referral/config')
    cfg.value = res.data
  } catch { /* */ }
}

async function loadWithdraws() {
  loading.value = true
  try {
    const res = await request.get('/admin/referral/withdrawals', {
      params: { page: wPage.value, page_size: 20, status: wStatus.value || undefined },
    })
    withdraws.value = res.data?.list || []
    wTotal.value = res.data?.total || 0
  } catch { /* */ }
  loading.value = false
}

async function loadLedgers() {
  loading.value = true
  try {
    const res = await request.get('/admin/referral/ledgers', {
      params: { page: lPage.value, page_size: 20 },
    })
    ledgers.value = res.data?.list || []
    lTotal.value = res.data?.total || 0
  } catch { /* */ }
  loading.value = false
}

async function approve(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认已向 ${row.user_email || row.user_id} 打款 ${formatPrice(row.amount)}？\n收款：${methodName(row.method)} ${row.account}`,
      '通过并标记已打款',
      { type: 'warning' },
    )
  } catch { return }
  try {
    await request.post(`/admin/referral/withdrawals/${row.id}/approve`, { remark: '已打款' })
    ElMessage.success('已标记为已打款')
    await loadWithdraws()
  } catch { /* */ }
}

async function reject(row: any) {
  let remark = ''
  try {
    const { value } = await ElMessageBox.prompt('驳回原因（将退回用户余额）', '驳回提现', {
      confirmButtonText: '驳回',
      cancelButtonText: '取消',
      inputPlaceholder: '可选备注',
    })
    remark = value || ''
  } catch { return }
  try {
    await request.post(`/admin/referral/withdrawals/${row.id}/reject`, { remark })
    ElMessage.success('已驳回并退回余额')
    await loadWithdraws()
  } catch { /* */ }
}

function onTab(name: string | number) {
  if (name === 'withdraws') loadWithdraws()
  else loadLedgers()
}

function reload() {
  loadCfg()
  if (tab.value === 'withdraws') loadWithdraws()
  else loadLedgers()
}

function goSettings() {
  router.push('/settings#referral')
}

onMounted(() => {
  loadCfg()
  loadWithdraws()
})
</script>

<style scoped>
.ref-page { max-width: 1200px; }
.header-actions { display: flex; gap: 10px; align-items: center; }
.cfg-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  background: #f1f5f9;
  color: #475569;
}
.pill.on { background: #ecfdf5; color: #047857; }
.pill.off { background: #fef2f2; color: #b91c1c; }
.pill.mute { font-weight: 600; color: #64748b; }
.toolbar { margin-bottom: 12px; }
.table-shell {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 14px;
  padding: 8px;
  overflow: hidden;
}
.muted { font-size: 12px; color: #94a3b8; }
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
.st.rejected { background: #fef2f2; color: #b91c1c; }
.pager {
  display: flex;
  justify-content: flex-end;
  padding: 12px 8px 4px;
}
</style>
