<template>
  <div class="user-page">
    <div class="k2-page-header">
      <div>
        <h3>用户管理</h3>
        <p class="sub">管理账号、权限组、流量与订阅分发</p>
      </div>
      <el-button type="primary" size="large" @click="showCreate">
        <el-icon><Plus /></el-icon>
        创建用户
      </el-button>
    </div>

    <transition name="slide">
      <div v-if="selectedIds.length > 0" class="batch-bar">
        <div class="batch-left">
          <span class="batch-count">{{ selectedIds.length }}</span>
          <span>已选择用户</span>
        </div>
        <div class="batch-actions">
          <el-select v-model="batchGroupId" placeholder="移动到权限组" clearable style="width: 180px">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
          <el-button type="primary" :disabled="!batchGroupId" @click="handleBatchGroup">批量移动</el-button>
          <el-button type="danger" plain @click="handleBatchDelete">批量删除</el-button>
          <el-button text @click="clearSelection">取消</el-button>
        </div>
      </div>
    </transition>

    <div class="table-shell">
      <div class="table-toolbar">
        <div class="search-wrap">
          <el-input
            v-model="searchForm.search"
            placeholder="搜索邮箱…"
            clearable
            size="large"
            class="search-input"
            @clear="fetchList"
            @keyup.enter="fetchList"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-button size="large" @click="fetchList">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
        <div class="toolbar-meta">共 <b>{{ pagination.total }}</b> 位用户</div>
      </div>

      <el-table
        ref="tableRef"
        :data="users"
        v-loading="loading"
        class="user-table"
        row-class-name="user-row"
        :default-sort="{ prop: 'id', order: 'descending' }"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column prop="id" label="ID" width="72" sortable="custom">
          <template #default="{ row }">
            <span class="id-pill">#{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip sortable="custom">
          <template #default="{ row }">
            <div class="email-cell">
              <span class="avatar-sm">{{ row.email?.charAt(0)?.toUpperCase() }}</span>
              <span class="email-text">{{ row.email }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="88" align="center">
          <template #default="{ row }">
            <el-tooltip :content="tooltipText(row)" placement="top">
              <span class="status-dot" :class="isRecentlyOnline(row) ? 'on' : 'off'">
                <i />
                {{ isRecentlyOnline(row) ? '在线' : '离线' }}
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="设备" width="100" align="center">
          <template #default="{ row }">
            <el-tooltip v-if="row.online_ips?.length" placement="top" :show-after="150">
              <template #content>
                <div v-for="ip in row.online_ips" :key="ip" class="ip-tip">{{ ip }}</div>
              </template>
              <span class="device-badge" :class="deviceClass(row)">
                <el-icon><Iphone /></el-icon>
                {{ row.online_ips.length }}<template v-if="row.device_limit > 0">/{{ row.device_limit }}</template>
              </span>
            </el-tooltip>
            <span v-else class="device-badge muted">
              0<template v-if="row.device_limit > 0">/{{ row.device_limit }}</template>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="权限组" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span
              class="group-tag"
              :class="{ empty: !row.group_id }"
              :title="row.group_id ? `权限组：${getGroupName(row.group_id)}` : '未分组'"
            >{{ row.group_id ? getGroupName(row.group_id) : '未分组' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="订阅计划" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span
              class="group-tag plan-tag"
              :class="{ empty: !row.plan_id }"
              :title="row.plan_id ? `套餐：${getPlanName(row.plan_id)}` : '无套餐'"
            >{{ getPlanName(row.plan_id || 0) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="traffic_used" label="流量" min-width="200" sortable="custom">
          <template #default="{ row }">
            <!-- 无套餐且无权限组 = 未开通：勿显示 ∞（0 限额在开通用户上才表示不限） -->
            <div v-if="!hasServiceBinding(row)" class="traffic-cell none">
              <span class="na-text">未开通</span>
            </div>
            <div v-else class="traffic-cell">
              <div class="traffic-top">
                <span>{{ formatBytes(row.traffic_used) }}</span>
                <span class="sep">/</span>
                <span class="lim">{{ row.traffic_limit > 0 ? formatBytes(row.traffic_limit) : '∞' }}</span>
              </div>
              <div class="progress-track" v-if="row.traffic_limit > 0">
                <div
                  class="progress-fill"
                  :class="{ danger: row.traffic_used >= row.traffic_limit, warn: pct(row) >= 80 && row.traffic_used < row.traffic_limit }"
                  :style="{ width: Math.min(100, pct(row)) + '%' }"
                />
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订阅" width="96" align="center">
          <template #default="{ row }">
            <el-tooltip
              :disabled="!!row.group_id"
              content="未分组：订阅拉不到任何节点"
              placement="top"
            >
              <el-button
                class="copy-btn"
                size="small"
                :class="{ muted: !row.group_id }"
                @click="copySubUrl(row, '')"
              >
                <el-icon><Link /></el-icon>
                复制
              </el-button>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="expire_at" label="到期" width="140" sortable="custom">
          <template #default="{ row }">
            <!-- 未开通：expire_at=0 不是「永久」，是未设到期 -->
            <span v-if="!hasServiceBinding(row)" class="expire none">未开通</span>
            <span v-else-if="row.expire_at > 0" class="expire" :class="{ expired: row.expire_at < nowTs }">
              {{ formatDate(row.expire_at) }}
            </span>
            <span v-else class="expire forever">永久</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="注册时间" width="150" sortable="custom">
          <template #default="{ row }">
            <span class="reg-time">{{ formatDateTime(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <span class="enable-pill" :class="row.enable ? 'yes' : 'no'">{{ row.enable ? '正常' : '封禁' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="88" fixed="right" align="center">
          <template #default="{ row }">
            <el-dropdown trigger="click" @command="(cmd: string) => handleCommand(cmd, row)">
              <el-button class="more-btn" size="small">
                操作 <el-icon><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit"><el-icon><Edit /></el-icon> 编辑</el-dropdown-item>
                  <el-dropdown-item command="sub-v2ray" divided><el-icon><Link /></el-icon> V2Ray 订阅</el-dropdown-item>
                  <el-dropdown-item command="sub-clash"><el-icon><Link /></el-icon> Clash 订阅</el-dropdown-item>
                  <el-dropdown-item command="sub-surge"><el-icon><Link /></el-icon> Surge 订阅</el-dropdown-item>
                  <el-dropdown-item command="sub-shadowrocket"><el-icon><Link /></el-icon> Shadowrocket</el-dropdown-item>
                  <el-dropdown-item command="reset-uuid" divided><el-icon><RefreshRight /></el-icon> 重置 UUID</el-dropdown-item>
                  <el-dropdown-item command="reset-traffic"><el-icon><DeleteFilled /></el-icon> 重置流量</el-dropdown-item>
                  <el-dropdown-item command="delete" divided class="danger-item">
                    <el-icon><Delete /></el-icon> 删除
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          :page-sizes="[10, 20, 50, 100]"
          background
          @change="fetchList"
        />
      </div>
    </div>

    <UserEditDialog v-model:visible="dialogVisible" :user="editingUser" @saved="fetchList" />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Search, Refresh, ArrowDown, Edit, Link, RefreshRight,
  DeleteFilled, Delete, Iphone,
} from '@element-plus/icons-vue'
import { getUserList, deleteUser, resetUserUUID, type User } from '@/api/user'
import request from '@/api/request'
import { formatBytes, formatDate, formatDateTime } from '@/utils/format'
import UserEditDialog from './UserEditDialog.vue'

interface Group { id: number; name: string }
interface Plan { id: number; name: string; group_id?: number }

const users = ref<User[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editingUser = ref<User | null>(null)
const nowTs = Math.floor(Date.now() / 1000)
const groups = ref<Group[]>([])
const plans = ref<Plan[]>([])
const selectedIds = ref<number[]>([])
const batchGroupId = ref<number | null>(null)
const searchForm = reactive({ search: '' })
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
/** Full-table sort (server-side) */
const sortState = reactive({ sort_by: 'id', sort_order: 'desc' as 'asc' | 'desc' })

function onSortChange(payload: { prop: string; order: string | null }) {
  // order: ascending | descending | null (clear)
  if (!payload.prop || !payload.order) {
    sortState.sort_by = 'id'
    sortState.sort_order = 'desc'
  } else {
    sortState.sort_by = payload.prop
    sortState.sort_order = payload.order === 'ascending' ? 'asc' : 'desc'
  }
  pagination.page = 1
  fetchList()
}

async function fetchList() {
  loading.value = true
  try {
    const res = await getUserList({
      page: pagination.page,
      page_size: pagination.pageSize,
      search: searchForm.search,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order,
    })
    users.value = res.data.list
    pagination.total = res.data.total
  } catch { /* ignore */ }
  loading.value = false
}

async function fetchMeta() {
  try {
    const [gr, pr] = await Promise.all([
      request.get('/admin/groups'),
      request.get('/admin/plans'),
    ])
    groups.value = gr.data || []
    plans.value = pr.data || []
  } catch { /* ignore */ }
}

/** 有套餐或权限组才算已绑定服务；新注册用户两者皆 0 → 未开通 */
function hasServiceBinding(row: User): boolean {
  return (row.plan_id || 0) > 0 || (row.group_id || 0) > 0
}

function pct(row: User) {
  if (!row.traffic_limit) return 0
  return Math.round((row.traffic_used / row.traffic_limit) * 100)
}
function deviceClass(row: User) {
  if (row.device_limit > 0 && row.online_ips && row.online_ips.length > row.device_limit) return 'over'
  return 'ok'
}
function isRecentlyOnline(row: User) {
  if (!row.last_active_at) return false
  return Date.now() - new Date(row.last_active_at).getTime() < 5 * 60 * 1000
}
function tooltipText(row: User) {
  if (!row.last_active_at) return '从未使用'
  return '最后在线: ' + new Date(row.last_active_at).toLocaleString('zh-CN')
}
function getGroupName(id: number) {
  if (!id) return '未分组'
  return groups.value.find(g => g.id === id)?.name || `#${id}`
}
function getPlanName(id: number) {
  if (!id) return '无套餐'
  return plans.value.find(p => p.id === id)?.name || `#${id}`
}
function showCreate() {
  editingUser.value = null
  dialogVisible.value = true
}
function showEdit(user: User) {
  editingUser.value = { ...user }
  dialogVisible.value = true
}
function onSelectionChange(rows: User[]) {
  selectedIds.value = rows.map(r => r.id)
}
function clearSelection() {
  selectedIds.value = []
  batchGroupId.value = null
}
function getSubUrl(user: User, flag: string) {
  return `${window.location.origin}/api/v1/client/subscribe?token=${user.token}${flag ? '&flag=' + flag : ''}`
}
function copySubUrl(user: User, flag: string) {
  navigator.clipboard.writeText(getSubUrl(user, flag))
  ElMessage.success(flag ? `已复制 ${flag.toUpperCase()} 订阅` : '订阅链接已复制')
}

async function handleBatchDelete() {
  try {
    await ElMessageBox.confirm(`确认删除 ${selectedIds.value.length} 个用户？`, '批量删除', { type: 'warning' })
    await request.post('/admin/users/batch-delete', { ids: selectedIds.value })
    ElMessage.success('已批量删除')
    clearSelection()
    fetchList()
  } catch { /* cancel */ }
}

async function handleBatchGroup() {
  if (!batchGroupId.value) return
  try {
    await request.post('/admin/users/batch-group', { ids: selectedIds.value, group_id: batchGroupId.value })
    ElMessage.success('已批量移动')
    clearSelection()
    fetchList()
  } catch {
    ElMessage.error('操作失败')
  }
}

async function handleCommand(cmd: string, user: User) {
  switch (cmd) {
    case 'edit':
      showEdit(user)
      break
    case 'sub-v2ray':
    case 'sub-clash':
    case 'sub-surge':
    case 'sub-shadowrocket':
      copySubUrl(user, cmd.replace('sub-', ''))
      break
    case 'reset-uuid':
      try {
        await resetUserUUID(user.id)
        ElMessage.success('UUID 已重置')
        fetchList()
      } catch {
        ElMessage.error('失败')
      }
      break
    case 'reset-traffic':
      try {
        await request.post(`/admin/users/${user.id}/reset-traffic`)
        ElMessage.success('流量已重置')
        fetchList()
      } catch {
        ElMessage.error('失败')
      }
      break
    case 'delete':
      try {
        await ElMessageBox.confirm(`确认删除 ${user.email}？`, '确认删除', {
          type: 'warning',
          confirmButtonText: '删除',
        })
        await deleteUser(user.id)
        ElMessage.success('已删除')
        fetchList()
      } catch { /* cancel */ }
      break
  }
}

onMounted(() => {
  fetchList()
  fetchMeta()
})
</script>

<style scoped>
.batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 18px;
  margin-bottom: 16px;
  border-radius: 14px;
  background: linear-gradient(90deg, #eef2ff, #ecfeff);
  border: 1px solid #c7d2fe;
  box-shadow: 0 8px 24px rgba(79, 70, 229, 0.08);
}
.batch-left {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  color: #334155;
  font-weight: 500;
}
.batch-count {
  min-width: 28px;
  height: 28px;
  padding: 0 8px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  background: #4f46e5;
  color: #fff;
  font-weight: 800;
  font-size: 13px;
}
.batch-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.table-shell {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  box-shadow: var(--k2-shadow-sm);
  padding: 18px 18px 8px;
  overflow: hidden;
}
.table-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.search-wrap {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.search-input {
  width: 280px;
}
.toolbar-meta {
  font-size: 13px;
  color: var(--k2-text-muted);
}
.toolbar-meta b {
  color: var(--k2-text);
  font-weight: 700;
}

.user-table {
  width: 100%;
  --el-table-header-bg-color: #f8fafc;
}
.id-pill {
  font-size: 12px;
  font-weight: 700;
  color: #64748b;
  background: #f1f5f9;
  padding: 3px 8px;
  border-radius: 6px;
}
.email-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.avatar-sm {
  width: 32px;
  height: 32px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 700;
  color: #fff;
  background: var(--k2-gradient);
  flex-shrink: 0;
}
.email-text {
  font-weight: 600;
  color: #0f172a;
  font-size: 13px;
}

.status-dot {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 8px;
  border-radius: 999px;
}
.status-dot i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.status-dot.on {
  color: #059669;
  background: #ecfdf5;
}
.status-dot.on i {
  background: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
}
.status-dot.off {
  color: #94a3b8;
  background: #f8fafc;
}
.status-dot.off i {
  background: #cbd5e1;
}

.device-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 700;
  padding: 4px 8px;
  border-radius: 8px;
  cursor: default;
}
.device-badge.ok {
  color: #4f46e5;
  background: #eef2ff;
}
.device-badge.over {
  color: #dc2626;
  background: #fef2f2;
}
.device-badge.muted {
  color: #94a3b8;
  background: #f8fafc;
}
.ip-tip {
  line-height: 1.7;
  font-family: ui-monospace, monospace;
  font-size: 12px;
}

.group-tag {
  display: inline-block;
  font-size: 12px;
  font-weight: 600;
  color: #4338ca;
  background: #eef2ff;
  padding: 4px 10px;
  border-radius: 8px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-tag.empty {
  color: #94a3b8;
  background: #f1f5f9;
}
.group-tag.plan-tag {
  color: #0f766e;
  background: #ecfdf5;
}
.group-tag.plan-tag.empty {
  color: #94a3b8;
  background: #f1f5f9;
}

.traffic-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 0;
}
.traffic-cell.none {
  flex-direction: row;
  align-items: center;
}
.na-text {
  color: #94a3b8;
  font-size: 12px;
  font-weight: 600;
}
.traffic-top {
  font-size: 12px;
  font-weight: 600;
  color: #334155;
}
.traffic-top .sep {
  color: #cbd5e1;
  margin: 0 4px;
}
.traffic-top .lim {
  color: #94a3b8;
}
.progress-track {
  height: 6px;
  border-radius: 999px;
  background: #f1f5f9;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #6366f1, #22d3ee);
  transition: width 0.4s ease;
}
.progress-fill.warn {
  background: linear-gradient(90deg, #f59e0b, #fbbf24);
}
.progress-fill.danger {
  background: linear-gradient(90deg, #ef4444, #f87171);
}

.copy-btn {
  border-radius: 8px !important;
  border: 1px solid #e0e7ff !important;
  background: #eef2ff !important;
  color: #4f46e5 !important;
  font-weight: 600;
}
.copy-btn.muted {
  opacity: 0.55;
}
.more-btn {
  border-radius: 8px !important;
  font-weight: 600;
}

.expire {
  font-size: 12px;
  font-weight: 600;
  color: #64748b;
}
.expire.expired {
  color: #ef4444;
}
.expire.forever {
  color: #059669;
}
.expire.none {
  color: #94a3b8;
}

.reg-time {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  white-space: nowrap;
}

.enable-pill {
  font-size: 11px;
  font-weight: 700;
  padding: 4px 10px;
  border-radius: 999px;
}
.enable-pill.yes {
  background: #ecfdf5;
  color: #059669;
}
.enable-pill.no {
  background: #fef2f2;
  color: #dc2626;
}

.pagination-wrap {
  margin-top: 16px;
  padding: 8px 4px 12px;
  display: flex;
  justify-content: flex-end;
}

.slide-enter-active,
.slide-leave-active {
  transition: all 0.22s ease;
}
.slide-enter-from,
.slide-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

:deep(.danger-item) {
  color: #ef4444 !important;
}
:deep(.el-table .user-row td.el-table__cell) {
  padding: 14px 0;
}
:deep(.el-table__inner-wrapper::before) {
  display: none;
}

@media (max-width: 640px) {
  .search-input {
    width: 100%;
  }
  .search-wrap {
    width: 100%;
  }
}
</style>
