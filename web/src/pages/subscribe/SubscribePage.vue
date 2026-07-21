<template>
  <div class="subscribe-page">
    <div class="page-header">
      <h3>订阅管理</h3>
    </div>

    <el-card shadow="never" class="table-card">
      <div class="table-toolbar">
        <el-input
          v-model="searchForm.search" placeholder="搜索邮箱..." clearable
          style="width:260px" @clear="fetchList" @keyup.enter="fetchList"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <div class="toolbar-right">
          <span class="format-label">默认格式：</span>
          <el-select v-model="defaultFormat" size="small" style="width:130px">
            <el-option label="V2Ray" value="v2ray" />
            <el-option label="Clash" value="clash" />
            <el-option label="Surge" value="surge" />
            <el-option label="Shadowrocket" value="shadowrocket" />
          </el-select>
          <el-button @click="fetchList"><el-icon><Refresh /></el-icon> 刷新</el-button>
        </div>
      </div>

      <el-table :data="users" stripe v-loading="loading" style="width:100%">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="email" label="邮箱" width="200" show-overflow-tooltip />
        <el-table-column label="流量(已用/总额)" width="180">
          <template #default="{ row }">
            <div v-if="!hasServiceBinding(row)" class="traffic-cell">
              <span class="t-text na">未开通</span>
            </div>
            <div v-else class="traffic-cell">
              <span class="t-text">
                {{ formatBytes(row.traffic_used) }}
                <template v-if="row.traffic_limit > 0"> / {{ formatBytes(row.traffic_limit) }}</template>
                <template v-else> / ∞</template>
              </span>
              <el-progress
                v-if="row.traffic_limit > 0"
                :percentage="Math.min(100, Math.round(row.traffic_used / row.traffic_limit * 100))"
                :stroke-width="5" :show-text="false"
                :status="row.traffic_used >= row.traffic_limit ? 'exception' : undefined"
              />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="到期" width="110">
          <template #default="{ row }">
            <span v-if="!hasServiceBinding(row)" class="exp na">未开通</span>
            <span v-else-if="row.expire_at > 0" class="exp" :class="{ expired: row.expire_at < nowTs }">
              {{ formatDate(row.expire_at) }}
            </span>
            <span v-else class="exp forever">永久</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.enable ? 'success' : 'danger'" size="small" effect="dark">
              {{ row.enable ? '正常' : '封禁' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="订阅链接 (V2Ray)" min-width="220">
          <template #default="{ row }">
            <div class="url-cell">
              <span class="url-text">{{ getSubscribeUrl(row, 'v2ray') }}</span>
              <el-button link type="primary" size="small" @click="copyUrl(row, 'v2ray')">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-dropdown @command="(flag: string) => copyUrl(row, flag)">
              <el-button size="small" type="primary">
                复制链接<el-icon style="margin-left:4px"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="v2ray">📡 V2Ray 格式</el-dropdown-item>
                  <el-dropdown-item command="clash">🎯 Clash 格式</el-dropdown-item>
                  <el-dropdown-item command="surge">⚡ Surge 格式</el-dropdown-item>
                  <el-dropdown-item command="shadowrocket">🚀 Shadowrocket</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button size="small" @click="openQrCode(row)">二维码</el-button>
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
          @change="fetchList"
        />
      </div>
    </el-card>

    <!-- QR Code dialog -->
    <el-dialog v-model="qrVisible" title="订阅二维码" width="400px" center>
      <div style="text-align:center">
        <div ref="qrRef" style="display:inline-block;padding:16px;background:#fff;border-radius:8px" />
        <p style="margin-top:12px;color:#666;font-size:13px">
          {{ qrEmail }} · {{ qrFormat.toUpperCase() }}
        </p>
        <p style="color:#999;font-size:11px;word-break:break-all;margin-top:8px">
          {{ qrUrl }}
        </p>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, Refresh, CopyDocument, ArrowDown } from '@element-plus/icons-vue'
import request from '@/api/request'
import { formatBytes, formatDate } from '@/utils/format'

interface SubUser {
  id: number; email: string; token: string; uuid: string
  plan_id?: number; group_id?: number
  traffic_used: number; traffic_limit: number
  enable: boolean; expire_at: number
}

const users = ref<SubUser[]>([])
const loading = ref(false)
const defaultFormat = ref('v2ray')
const qrVisible = ref(false)
const qrUrl = ref('')
const qrEmail = ref('')
const qrFormat = ref('v2ray')
const nowTs = Math.floor(Date.now() / 1000)

const searchForm = reactive({ search: '' })
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

/** 无套餐且无权限组 = 未开通（traffic_limit/expire_at 为 0 不表示无限/永久） */
function hasServiceBinding(row: SubUser): boolean {
  return (row.plan_id || 0) > 0 || (row.group_id || 0) > 0
}

function getSubscribeUrl(user: SubUser, flag: string) {
  return `${window.location.origin}/api/v1/client/subscribe?token=${user.token}&flag=${flag}`
}

async function fetchList() {
  loading.value = true
  try {
    const res = await request.get('/admin/subscribe/users', {
      params: { page: pagination.page, page_size: pagination.pageSize, search: searchForm.search },
    })
    users.value = res.data.list || []
    pagination.total = res.data.total || 0
  } catch {}
  loading.value = false
}

function copyUrl(user: SubUser, flag: string) {
  const url = getSubscribeUrl(user, flag)
  navigator.clipboard.writeText(url)
  ElMessage.success(`${flag.toUpperCase()} 订阅链接已复制`)
}

function openQrCode(user: SubUser) {
  qrEmail.value = user.email
  qrFormat.value = defaultFormat.value
  qrUrl.value = getSubscribeUrl(user, defaultFormat.value)
  qrVisible.value = true
}

onMounted(fetchList)
</script>

<style scoped>
.page-header { margin-bottom: 20px; }
.page-header h3 { font-size: 20px; font-weight: 600; color: #1a1a1a; margin: 0; }
.table-card { border-radius: 10px; }
.table-toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.toolbar-right { display: flex; align-items: center; gap: 8px; }
.format-label { color: #666; font-size: 13px; }
.traffic-cell { display: flex; flex-direction: column; gap: 4px; }
.t-text { font-size: 12px; color: #666; }
.t-text.na, .exp.na { color: #94a3b8; font-weight: 600; }
.exp { font-size: 12px; color: #64748b; font-weight: 600; }
.exp.expired { color: #ef4444; }
.exp.forever { color: #059669; }
.url-cell { display: flex; align-items: center; gap: 4px; }
.url-text {
  font-size: 11px; color: #999; font-family: monospace;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 180px;
}
.pagination-wrap { margin-top: 16px; display: flex; justify-content: flex-end; }
</style>
