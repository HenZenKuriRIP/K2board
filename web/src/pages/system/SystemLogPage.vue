<template>
  <div class="log-page">
    <div class="k2-page-header">
      <div>
        <h3>系统日志</h3>
        <p class="sub">审计管理员关键操作与变更记录</p>
      </div>
      <el-button size="large" @click="fetchList">
        <el-icon><Refresh /></el-icon>
        刷新
      </el-button>
    </div>

    <div class="k2-table-shell" v-loading="loading">
      <el-table :data="logs" class="aurora-table">
        <el-table-column prop="id" label="ID" width="80">
          <template #default="{ row }"><span class="k2-id-pill">#{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column prop="admin_id" label="操作人" width="100">
          <template #default="{ row }">
            <span class="admin-chip">UID {{ row.admin_id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="action" label="操作" width="120">
          <template #default="{ row }">
            <span class="action-tag" :class="row.action">{{ row.action }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="target" label="对象" width="140">
          <template #default="{ row }">
            <span class="target-text">{{ row.target }} <em>#{{ row.target_id }}</em></span>
          </template>
        </el-table-column>
        <el-table-column prop="detail" label="详情" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="detail">{{ row.detail || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="180">
          <template #default="{ row }">
            <span class="time">{{ new Date(row.created_at).toLocaleString('zh-CN') }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="k2-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          background
          @change="fetchList"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import request from '@/api/request'

interface Log {
  id: number; admin_id: number; action: string
  target: string; target_id: number; detail: string; created_at: string
}
const logs = ref<Log[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

async function fetchList() {
  loading.value = true
  try {
    const r = await request.get('/admin/audit-logs', {
      params: { page: page.value, page_size: pageSize.value },
    })
    logs.value = r.data.list || []
    total.value = r.data.total || 0
  } catch { /* ignore */ }
  loading.value = false
}

onMounted(fetchList)
</script>

<style scoped>
.admin-chip {
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  background: #f1f5f9;
  padding: 4px 8px;
  border-radius: 8px;
}
.action-tag {
  display: inline-block;
  font-size: 11px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  padding: 4px 10px;
  border-radius: 8px;
  background: #f1f5f9;
  color: #64748b;
}
.action-tag.create { background: #ecfdf5; color: #059669; }
.action-tag.update { background: #fffbeb; color: #d97706; }
.action-tag.delete { background: #fef2f2; color: #dc2626; }
.action-tag.batch { background: #eef2ff; color: #4f46e5; }
.target-text {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
}
.target-text em {
  font-style: normal;
  color: #94a3b8;
  font-weight: 500;
}
.detail {
  font-size: 13px;
  color: #64748b;
}
.time {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
}
:deep(.aurora-table .el-table__cell) {
  padding: 14px 0;
}
</style>
