<template>
  <div class="plan-page">
    <div class="k2-page-header">
      <div>
        <h3>订阅计划</h3>
        <p class="sub">
          定义时长、流量、限速与设备策略 ·
          <strong>用户端商城按「排序」升序显示</strong>（数字越小越靠前）
        </p>
      </div>
      <el-button type="primary" size="large" @click="showCreate">
        <el-icon><Plus /></el-icon>
        创建计划
      </el-button>
    </div>

    <div class="sort-banner">
      调整卡片上的 <b>↑ ↓</b> 或编辑里的「用户端显示排序」即可改变商城顺序；保存后用户刷新即可看到。
    </div>

    <div class="plan-grid" v-loading="loading">
      <div v-for="(p, idx) in plans" :key="p.id" class="plan-card" :class="{ off: !p.enable }">
        <div class="plan-top">
          <div>
            <div class="plan-name">{{ p.name }}</div>
            <div class="plan-group">{{ getGroupName(p.group_id) }}</div>
          </div>
          <div class="plan-top-right">
            <span class="sort-chip" title="用户端商城排序值">序 {{ p.sort }}</span>
            <span class="k2-enable-pill" :class="p.enable ? 'yes' : 'no'">
              {{ p.enable ? '启用' : '禁用' }}
            </span>
          </div>
        </div>
        <div class="plan-price" v-if="p.show_on_shop || p.allow_renew || (p.price != null && p.price > 0)">
          <span class="price-val">{{ formatPrice(p.price, p.currency) }}</span>
          <span class="shop-tag" :class="p.show_on_shop ? 'on' : 'off'">
            {{ p.show_on_shop ? '商城上架' : '未上架' }}
          </span>
          <span class="shop-tag" :class="p.allow_renew !== false ? 'on' : 'off'">
            {{ p.allow_renew !== false ? '可续费' : '禁续费' }}
          </span>
        </div>
        <div class="plan-specs">
          <div class="spec"><span>时长</span><b>{{ formatDuration(p.duration) }}</b></div>
          <div class="spec"><span>流量</span><b>{{ p.traffic_limit > 0 ? formatBytes(p.traffic_limit) : '不限' }}</b></div>
          <div class="spec"><span>速度</span><b>{{ p.speed_limit > 0 ? p.speed_limit + ' Mbps' : '不限' }}</b></div>
          <div class="spec"><span>设备</span><b>{{ p.device_limit > 0 ? p.device_limit + ' 台' : '不限' }}</b></div>
          <div class="spec"><span>重置</span><b>{{ (p.reset_day ?? 0) > 0 ? '每月 ' + p.reset_day + ' 号' : '无' }}</b></div>
          <div class="spec"><span>显示序</span><b>{{ p.sort }}</b></div>
        </div>
        <div class="plan-actions">
          <span class="k2-id-pill">#{{ p.id }}</span>
          <div class="act-btns">
            <el-button class="k2-action-btn" size="small" :disabled="idx === 0" @click="movePlan(idx, -1)">↑</el-button>
            <el-button class="k2-action-btn" size="small" :disabled="idx >= plans.length - 1" @click="movePlan(idx, 1)">↓</el-button>
            <el-button class="k2-action-btn" size="small" @click="showEdit(p)">编辑</el-button>
            <el-popconfirm
              title="确认删除该计划？若仍有用户绑定该套餐将无法删除"
              @confirm="handleDelete(p.id)"
            >
              <template #reference>
                <el-button class="k2-action-btn danger" size="small">删除</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </div>

      <button v-if="!loading" type="button" class="plan-card add-card" @click="showCreate">
        <el-icon :size="28"><Plus /></el-icon>
        <span>新建订阅计划</span>
      </button>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑计划' : '创建计划'"
      width="560px"
      align-center
      destroy-on-close
    >
      <el-form :model="form" label-position="top" class="plan-form">
        <el-form-item label="计划名称">
          <el-input v-model="form.name" size="large" placeholder="例：VIP 月付" />
        </el-form-item>
        <el-form-item label="所属权限组">
          <el-select v-model="form.group_id" style="width:100%" clearable placeholder="不绑定" size="large">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="订阅时长">
          <el-select v-model="durationPreset" style="width:100%" size="large" @change="onDurationPreset">
            <el-option label="月付 (30天)" value="month" />
            <el-option label="季付 (90天)" value="quarter" />
            <el-option label="半年付 (180天)" value="half" />
            <el-option label="年付 (365天)" value="year" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="durationPreset === 'custom'" label="时长（天）">
          <el-input v-model.number="customDurationDays" type="number" size="large" placeholder="0" />
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="流量限制 (GB)">
            <el-input v-model.number="form.traffic_limit" type="number" size="large" placeholder="0 = 不限" />
          </el-form-item>
          <el-form-item label="速度限制 (Mbps)">
            <el-input v-model.number="form.speed_limit" type="number" size="large" placeholder="0 = 不限" />
          </el-form-item>
          <el-form-item label="设备限制">
            <el-input v-model.number="form.device_limit" type="number" size="large" placeholder="0 = 不限" />
          </el-form-item>
          <el-form-item label="用户端显示排序">
            <el-input v-model.number="form.sort" type="number" size="large" placeholder="0" />
            <div class="field-hint">数字越小，用户商城越靠前。也可在列表用 ↑↓ 快速调整。</div>
          </el-form-item>
        </div>
        <el-form-item label="每月流量重置日">
          <el-select v-model="form.reset_day" style="width:100%" clearable placeholder="不自动重置" size="large">
            <el-option :value="0" label="不自动重置（整段订阅累计已用流量）" />
            <el-option v-for="d in 28" :key="d" :value="d" :label="'每月 ' + d + ' 号清零已用流量'" />
          </el-select>
          <div class="field-hint">
            流量额度是套餐的 traffic_limit，不是“自动每月包”。只有设置了重置日，系统才会在服务器本地日历的该日将已用流量清零；为 0 时在整个有效期内累计直到到期或再次购买。
          </div>
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="售价（元）">
            <el-input v-model.number="form.price_yuan" type="number" size="large" placeholder="0 = 免费" />
          </el-form-item>
          <el-form-item label="币种">
            <el-select v-model="form.currency" style="width:100%" size="large">
              <el-option value="CNY" label="CNY 人民币" />
              <el-option value="USD" label="USD 美元" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="用户商城上架（新客）">
          <el-switch v-model="form.show_on_shop" active-text="上架可购" inactive-text="不在商城展示" />
          <div class="field-hint">控制新用户是否在商城看到并购买。上架要求：已选权限组，且该组至少有 1 个启用中的节点</div>
        </el-form-item>
        <el-form-item label="已购用户续费">
          <el-switch v-model="form.allow_renew" active-text="允许续费" inactive-text="禁止续费" />
          <div class="field-hint">
            停止商城销售后，当前仍绑定本套餐的用户是否可在「仪表盘 / 我的套餐」续费。与上架开关独立；开启时同样需要权限组有节点。
          </div>
        </el-form-item>
        <el-form-item v-if="editing" label="启用">
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
import { formatBytes, bytesToGB, gbToBytes } from '@/utils/format'

interface Plan {
  id: number; name: string; group_id: number; duration: number
  traffic_limit: number; speed_limit: number; device_limit: number
  sort: number; enable: boolean; reset_day?: number
  price?: number; currency?: string; show_on_shop?: boolean; allow_renew?: boolean
}
interface Group { id: number; name: string }

const plans = ref<Plan[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref(false)
const editId = ref(0)
const saving = ref(false)
const durationPreset = ref('')
const form = reactive({
  name: '', group_id: 0, duration: 0, traffic_limit: 0,
  speed_limit: 0, device_limit: 0, sort: 0, reset_day: 0, enable: true,
  price_yuan: 0 as number, currency: 'CNY', show_on_shop: false, allow_renew: true,
})

function formatPrice(cents?: number, currency = 'CNY') {
  const n = (cents || 0) / 100
  const sym = currency === 'USD' ? '$' : '¥'
  return `${sym}${n.toFixed(2)}`
}

async function fetchPlans() {
  try {
    const r = await request.get('/admin/plans')
    plans.value = r.data || []
  } catch { /* ignore */ }
}
async function fetchGroups() {
  try {
    const r = await request.get('/admin/groups')
    groups.value = r.data || []
  } catch { /* ignore */ }
}
async function fetchAll() {
  loading.value = true
  await Promise.all([fetchPlans(), fetchGroups()])
  loading.value = false
}

function getGroupName(id: number) {
  const g = groups.value.find(x => x.id === id)
  return g ? g.name : '未绑定组'
}

function formatDuration(s: number) {
  if (!s) return '-'
  const d = s / 86400
  if (d === 30) return '月付'
  if (d === 90) return '季付'
  if (d === 180) return '半年'
  if (d === 365) return '年付'
  return `${d} 天`
}

const durMap: Record<string, number> = {
  month: 2592000, quarter: 7776000, half: 15552000, year: 31536000,
}
function onDurationPreset(v: string) {
  if (durMap[v]) form.duration = durMap[v]
  else if (v === 'custom') form.duration = 0
}
const customDurationDays = computed({
  get: () => form.duration / 86400 || 0,
  set: (v: number) => { form.duration = (v || 0) * 86400 },
})

function showCreate() {
  editing.value = false
  editId.value = 0
  resetForm()
  durationPreset.value = ''
  dialogVisible.value = true
}
function showEdit(p: Plan) {
  editing.value = true
  editId.value = p.id
  Object.assign(form, {
    name: p.name,
    group_id: p.group_id,
    duration: p.duration,
    traffic_limit: bytesToGB(p.traffic_limit),
    speed_limit: p.speed_limit,
    device_limit: p.device_limit,
    sort: p.sort,
    reset_day: p.reset_day || 0,
    enable: p.enable,
    price_yuan: Math.round(((p.price || 0) / 100) * 100) / 100,
    currency: p.currency || 'CNY',
    show_on_shop: !!p.show_on_shop,
    allow_renew: p.allow_renew !== false,
  })
  const presets: Record<number, string> = {
    2592000: 'month', 7776000: 'quarter', 15552000: 'half', 31536000: 'year',
  }
  durationPreset.value = presets[p.duration] || (p.duration > 0 ? 'custom' : '')
  dialogVisible.value = true
}
function resetForm() {
  Object.assign(form, {
    name: '', group_id: 0, duration: 0, traffic_limit: 0,
    speed_limit: 0, device_limit: 0, sort: 0, reset_day: 0, enable: true,
    price_yuan: 0, currency: 'CNY', show_on_shop: false, allow_renew: true,
  })
}

async function handleSave() {
  if (!form.name) {
    ElMessage.warning('请输入名称')
    return
  }
  if ((form.show_on_shop || form.allow_renew) && !form.group_id) {
    ElMessage.warning('上架或开启续费前请先选择权限组')
    return
  }
  saving.value = true
  try {
    const priceCents = Math.round((Number(form.price_yuan) || 0) * 100)
    const payload = {
      name: form.name,
      group_id: form.group_id,
      duration: form.duration,
      traffic_limit: gbToBytes(form.traffic_limit),
      speed_limit: form.speed_limit,
      device_limit: form.device_limit,
      sort: form.sort,
      reset_day: form.reset_day,
      enable: form.enable,
      price: priceCents,
      currency: form.currency || 'CNY',
      show_on_shop: form.show_on_shop,
      allow_renew: form.allow_renew,
    }
    if (editing.value) await request.put(`/admin/plans/${editId.value}`, payload)
    else await request.post('/admin/plans', payload)
    ElMessage.success(editing.value ? '已更新' : '已创建')
    dialogVisible.value = false
    fetchAll()
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || '操作失败'
    ElMessage.error(msg)
  }
  saving.value = false
}

/** Swap sort with neighbor so user shop order (sort ASC) changes. */
async function movePlan(idx: number, dir: -1 | 1) {
  const j = idx + dir
  if (j < 0 || j >= plans.value.length) return
  const a = plans.value[idx]
  const b = plans.value[j]
  const sa = a.sort
  const sb = b.sort
  // If sort equal, assign sequential so order sticks
  let newA = sb
  let newB = sa
  if (sa === sb) {
    newA = sa + dir
    newB = sa
  }
  try {
    await Promise.all([
      request.put(`/admin/plans/${a.id}`, { sort: newA }),
      request.put(`/admin/plans/${b.id}`, { sort: newB }),
    ])
    ElMessage.success('显示顺序已更新')
    await fetchPlans()
  } catch (e: any) {
    ElMessage.error(e?.message || '调整顺序失败')
  }
}

async function handleDelete(id: number) {
  try {
    await request.delete(`/admin/plans/${id}`)
    ElMessage.success('已删除')
    fetchAll()
  } catch (e: any) {
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
.sort-banner {
  margin: -8px 0 16px;
  padding: 10px 14px;
  font-size: 13px;
  color: #4338ca;
  background: #eef2ff;
  border: 1px solid #c7d2fe;
  border-radius: 12px;
  line-height: 1.5;
}
.plan-top-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}
.sort-chip {
  font-size: 11px;
  font-weight: 800;
  color: #4f46e5;
  background: #eef2ff;
  padding: 2px 8px;
  border-radius: 999px;
}
.act-btns {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
}
.plan-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.plan-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 20px;
  box-shadow: var(--k2-shadow-sm);
  display: flex;
  flex-direction: column;
  gap: 14px;
  transition: transform 0.18s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}
.plan-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--k2-shadow);
  border-color: rgba(79, 70, 229, 0.2);
}
.plan-card.off {
  opacity: 0.72;
}
.plan-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 10px;
}
.plan-name {
  font-size: 17px;
  font-weight: 800;
  color: #0f172a;
  letter-spacing: -0.02em;
}
.plan-group {
  margin-top: 4px;
  font-size: 12px;
  font-weight: 600;
  color: #6366f1;
  background: #eef2ff;
  display: inline-block;
  padding: 2px 8px;
  border-radius: 6px;
}
.plan-price {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  background: linear-gradient(135deg, #eef2ff, #ecfeff);
  border-radius: 12px;
}
.price-val {
  font-size: 20px;
  font-weight: 800;
  color: #4f46e5;
  letter-spacing: -0.02em;
}
.shop-tag {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 999px;
}
.shop-tag.on { background: #d1fae5; color: #059669; }
.shop-tag.off { background: #f1f5f9; color: #94a3b8; }
.plan-specs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.spec {
  background: #f8fafc;
  border-radius: 12px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.spec span {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.spec b {
  font-size: 13px;
  color: #0f172a;
  font-weight: 700;
}
.plan-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 4px;
  border-top: 1px solid #f1f5f9;
}
.k2-action-btn.danger {
  color: #dc2626 !important;
  border-color: #fecaca !important;
  background: #fef2f2 !important;
}
.add-card {
  min-height: 220px;
  border: 2px dashed #c7d2fe;
  background: linear-gradient(160deg, #eef2ff, #f8fafc);
  color: #4f46e5;
  cursor: pointer;
  align-items: center;
  justify-content: center;
  gap: 10px;
  font-weight: 700;
  font-size: 14px;
  font-family: inherit;
}
.add-card:hover {
  border-color: #818cf8;
  background: #eef2ff;
}
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 12px;
}
.plan-form :deep(.el-form-item__label) {
  font-weight: 600;
  color: #475569;
}
.field-hint {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.45;
}
@media (max-width: 560px) {
  .form-grid { grid-template-columns: 1fr; }
}
</style>
