<template>
  <div class="order-page">
    <div class="k2-page-header">
      <div>
        <h3>订单管理</h3>
        <p class="sub">用户购买套餐订单 · 关单 / 手动确认支付</p>
      </div>
      <div class="header-actions">
        <el-select v-model="status" clearable placeholder="全部状态" size="large" style="width:140px" @change="reload">
          <el-option value="pending" label="待支付" />
          <el-option value="paid" label="已支付" />
          <el-option value="cancelled" label="已取消" />
          <el-option value="failed" label="失败" />
        </el-select>
        <el-button size="large" @click="reload" :loading="loading">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <div class="table-shell">
      <el-table :data="list" v-loading="loading" class="order-table">
        <el-table-column prop="trade_no" label="订单号" min-width="180" show-overflow-tooltip />
        <el-table-column label="用户" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ row.user_email || '—' }}</div>
            <div class="muted">UID {{ row.user_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="套餐" min-width="120">
          <template #default="{ row }">
            <b>{{ row.plan_name }}</b>
            <div class="muted">#{{ row.plan_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="金额" width="110" align="right">
          <template #default="{ row }">
            {{ formatPrice(row.total_amount, row.currency) }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <span class="st" :class="row.status">{{ statusLabel(row.status) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="payment_method" label="渠道" width="100" />
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'pending' || (row.status === 'cancelled' && row.remark === 'auto-expired')"
              size="small"
              type="primary"
              @click="markPaid(row)"
            >{{ row.status === 'cancelled' ? '补单开通' : '确认支付' }}</el-button>
            <el-button
              v-if="row.status === 'paid' && !row.fulfilled_at"
              size="small"
              type="warning"
              @click="markPaid(row)"
            >重新履约</el-button>
            <el-button
              v-if="row.status === 'pending' || (row.status === 'cancelled' && row.remark === 'auto-expired')"
              size="small"
              type="success"
              plain
              @click="syncOrder(row)"
            >查单同步</el-button>
            <el-button
              v-if="row.status === 'pending'"
              size="small"
              @click="closeOrder(row)"
            >关单</el-button>
            <span v-if="row.status === 'paid' && row.fulfilled_at" class="muted">已履约</span>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          background
          @current-change="fetchList"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import request from '@/api/request'

const list = ref<any[]>([])
const loading = ref(false)
const status = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

function formatPrice(cents: number, currency = 'CNY') {
  const sym = currency === 'USD' ? '$' : '¥'
  return `${sym}${((cents || 0) / 100).toFixed(2)}`
}
function formatTime(t: string) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}
function statusLabel(s: string) {
  const m: Record<string, string> = {
    pending: '待支付', paid: '已支付', cancelled: '已取消', failed: '失败',
  }
  return m[s] || s
}

async function fetchList() {
  loading.value = true
  try {
    const r = await request.get('/admin/orders', {
      params: { page: page.value, page_size: pageSize.value, status: status.value || undefined },
    })
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  fetchList()
}

async function markPaid(row: any) {
  try {
    await ElMessageBox.confirm(`确认订单 ${row.trade_no} 已支付并开通套餐？`, '手动确认', { type: 'warning' })
    await request.post(`/admin/orders/${row.id}/mark-paid`)
    ElMessage.success('已确认并履约')
    fetchList()
  } catch { /* cancel */ }
}

async function closeOrder(row: any) {
  try {
    await ElMessageBox.confirm(`关闭订单 ${row.trade_no}？`, '关单', { type: 'warning' })
    await request.post(`/admin/orders/${row.id}/close`)
    ElMessage.success('已关单')
    fetchList()
  } catch { /* cancel */ }
}

async function syncOrder(row: any) {
  try {
    const r = await request.post(`/admin/orders/${row.id}/sync`)
    const st = r.data?.status
    if (st === 'paid') {
      ElMessage.success('渠道已支付，订单已同步并开通')
    } else {
      ElMessage.info(`渠道状态：${st || '未支付'}`)
    }
    fetchList()
  } catch (e: any) {
    ElMessage.error(e?.message || '查单失败')
  }
}

onMounted(fetchList)
</script>

<style scoped>
.header-actions { display: flex; gap: 10px; align-items: center; }
.table-shell {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 12px 16px 16px;
  box-shadow: var(--k2-shadow-sm);
}
.muted { font-size: 11px; color: #94a3b8; }
.st {
  font-size: 12px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 999px;
}
.st.pending { background: #fef3c7; color: #b45309; }
.st.paid { background: #d1fae5; color: #059669; }
.st.cancelled { background: #f1f5f9; color: #64748b; }
.st.failed { background: #fee2e2; color: #dc2626; }
.pager { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
