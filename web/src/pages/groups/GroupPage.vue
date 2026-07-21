<template>
  <div class="group-page">
    <div class="k2-page-header">
      <div>
        <h3>权限组管理</h3>
        <p class="sub">组织节点可见范围与订阅计划归属</p>
      </div>
      <el-button type="primary" size="large" @click="showCreate">
        <el-icon><Plus /></el-icon>
        创建权限组
      </el-button>
    </div>

    <div class="summary-row">
      <div class="sum-card">
        <span class="sum-label">权限组</span>
        <span class="sum-val">{{ groups.length }}</span>
      </div>
      <div class="sum-card">
        <span class="sum-label">已启用</span>
        <span class="sum-val ok">{{ enabledCount }}</span>
      </div>
      <div class="sum-card">
        <span class="sum-label">组内用户合计</span>
        <span class="sum-val">{{ totalUsers }}</span>
      </div>
      <div class="sum-card">
        <span class="sum-label">关联节点合计</span>
        <span class="sum-val">{{ totalNodes }}</span>
      </div>
      <div class="sum-card">
        <span class="sum-label">订阅计划合计</span>
        <span class="sum-val">{{ totalPlans }}</span>
      </div>
    </div>

    <div class="k2-table-shell" v-loading="loading">
      <el-table :data="groups" class="aurora-table">
        <el-table-column prop="id" label="ID" width="80">
          <template #default="{ row }"><span class="k2-id-pill">#{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column prop="name" label="组名" min-width="200">
          <template #default="{ row }">
            <div class="name-cell">
              <span class="g-avatar">{{ row.name?.charAt(0)?.toUpperCase() || 'G' }}</span>
              <span class="g-name">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="用户数" width="120">
          <template #default="{ row }">
            <span class="k2-count-chip users" :class="{ empty: !(row.user_count > 0) }">
              {{ row.user_count ?? 0 }} 人
            </span>
          </template>
        </el-table-column>
        <el-table-column label="关联节点" width="120">
          <template #default="{ row }">
            <span class="k2-count-chip">{{ nodeCounts[row.id] || 0 }} 节点</span>
          </template>
        </el-table-column>
        <el-table-column label="订阅计划" width="120">
          <template #default="{ row }">
            <span class="k2-count-chip plan">{{ planCounts[row.id] || 0 }} 计划</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <span class="k2-enable-pill" :class="row.enable ? 'yes' : 'no'">
              {{ row.enable ? '启用' : '禁用' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" align="right">
          <template #default="{ row }">
            <el-button class="k2-action-btn" size="small" @click="showEdit(row)">编辑</el-button>
            <el-popconfirm
              :title="(row.user_count > 0)
                ? `无法删除：仍有 ${row.user_count} 名用户在使用，请先调整用户分组`
                : '确认删除该权限组？将解除节点映射及计划上的组绑定（无用户时）'"
              :disabled="row.user_count > 0"
              @confirm="handleDelete(row.id)"
            >
              <template #reference>
                <el-button class="k2-action-btn danger" size="small" :disabled="row.user_count > 0">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑权限组' : '创建权限组'"
      width="440px"
      align-center
      destroy-on-close
    >
      <el-form :model="form" label-position="top">
        <el-form-item label="组名">
          <el-input v-model="form.name" size="large" placeholder="例如：VIP、标准、试用" />
        </el-form-item>
        <el-form-item v-if="editing" label="启用状态">
          <el-switch v-model="form.enable" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import request from '@/api/request'

interface Group { id: number; name: string; enable: boolean; user_count?: number }
const groups = ref<Group[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref(false)
const editId = ref(0)
const saving = ref(false)
const nodeCounts = ref<Record<number, number>>({})
const planCounts = ref<Record<number, number>>({})
const form = reactive({ name: '', enable: true })

const enabledCount = computed(() => groups.value.filter(g => g.enable).length)
const totalUsers = computed(() => groups.value.reduce((a, g) => a + (Number(g.user_count) || 0), 0))
const totalNodes = computed(() => Object.values(nodeCounts.value).reduce((a, b) => a + b, 0))
const totalPlans = computed(() => Object.values(planCounts.value).reduce((a, b) => a + b, 0))

async function fetchGroups() {
  try {
    const r = await request.get('/admin/groups')
    groups.value = r.data || []
  } catch { /* ignore */ }
}

async function fetchCounts() {
  try {
    const [nr, pr] = await Promise.all([
      request.get('/admin/nodes'),
      request.get('/admin/plans'),
    ])
    const nc: Record<number, number> = {}
    const pc: Record<number, number> = {}
    for (const n of (nr.data || [])) {
      const ids: number[] = Array.isArray(n.group_ids)
        ? n.group_ids
        : (n.group_id ? [n.group_id] : [])
      for (const gid of ids) {
        if (gid) nc[gid] = (nc[gid] || 0) + 1
      }
    }
    for (const p of (pr.data || [])) {
      if (p.group_id) pc[p.group_id] = (pc[p.group_id] || 0) + 1
    }
    nodeCounts.value = nc
    planCounts.value = pc
  } catch { /* ignore */ }
}

async function fetchAll() {
  loading.value = true
  await fetchGroups()
  await fetchCounts()
  loading.value = false
}

function showCreate() {
  editing.value = false
  editId.value = 0
  form.name = ''
  form.enable = true
  dialogVisible.value = true
}

function showEdit(g: Group) {
  editing.value = true
  editId.value = g.id
  form.name = g.name
  form.enable = g.enable
  dialogVisible.value = true
}

async function handleSave() {
  if (!form.name) {
    ElMessage.warning('请输入组名')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await request.put(`/admin/groups/${editId.value}`, { name: form.name, enable: form.enable })
    } else {
      await request.post('/admin/groups', { name: form.name })
    }
    ElMessage.success(editing.value ? '已更新' : '已创建')
    dialogVisible.value = false
    fetchAll()
  } catch {
    ElMessage.error('操作失败')
  }
  saving.value = false
}

async function handleDelete(id: number) {
  try {
    await request.delete(`/admin/groups/${id}`)
    ElMessage.success('已删除')
    fetchAll()
  } catch (e: any) {
    // request 拦截器通常已 toast message；此处兜底
    const msg = e?.response?.data?.message || e?.message
    if (msg && !String(msg).includes('无法删除')) {
      ElMessage.error(msg || '删除失败')
    }
    fetchAll()
  }
}

onMounted(fetchAll)
</script>

<style scoped>
.summary-row {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 12px;
  margin-bottom: 18px;
}
.sum-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 16px;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  box-shadow: var(--k2-shadow-sm);
}
.sum-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--k2-text-muted);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.sum-val {
  font-size: 26px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--k2-text);
}
.sum-val.ok { color: #059669; }

.name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.g-avatar {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: #fff;
  background: var(--k2-gradient);
  flex-shrink: 0;
}
.g-name {
  font-weight: 700;
  color: #0f172a;
}
.k2-count-chip.plan {
  color: #0e7490;
  background: #ecfeff;
}
.k2-count-chip.users {
  color: #5b21b6;
  background: #f5f3ff;
  font-weight: 700;
}
.k2-count-chip.users.empty {
  color: #94a3b8;
  background: #f8fafc;
  font-weight: 600;
}
.k2-action-btn.danger {
  color: #dc2626 !important;
  border-color: #fecaca !important;
  background: #fef2f2 !important;
}
:deep(.aurora-table .el-table__cell) {
  padding: 14px 0;
}
@media (max-width: 1100px) {
  .summary-row { grid-template-columns: repeat(3, 1fr); }
}
@media (max-width: 700px) {
  .summary-row { grid-template-columns: 1fr 1fr; }
}
@media (max-width: 520px) {
  .summary-row { grid-template-columns: 1fr; }
}
</style>
