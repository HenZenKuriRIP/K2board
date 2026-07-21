<template>
  <div class="node-page">
    <div class="k2-page-header">
      <div>
        <h3>{{ tab === 'monitor' ? '节点监控' : '节点管理' }}</h3>
        <p class="sub">
          {{ tab === 'monitor' ? '负载、在线与健康态势 · 节点运维' : '配置协议节点与权限可见范围 · 节点运维' }}
        </p>
      </div>
      <div class="header-actions">
        <el-button size="large" @click="tab === 'manage' ? fetchList() : fetchNodes()">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
        <el-button v-if="tab === 'manage'" type="primary" size="large" @click="showCreate">
          <el-icon><Plus /></el-icon>
          创建节点
        </el-button>
      </div>
    </div>

    <!-- Manage (sidebar: 节点管理) — no in-page sub-tabs -->
    <div v-show="tab === 'manage'" class="k2-table-shell">
      <div class="k2-table-toolbar">
        <el-radio-group v-model="filterType" size="large" @change="fetchList">
          <el-radio-button value="">全部</el-radio-button>
          <el-radio-button value="vless">VLESS</el-radio-button>
          <el-radio-button value="anytls">AnyTLS</el-radio-button>
        </el-radio-group>
        <div class="toolbar-meta">共 <b>{{ nodes.length }}</b> 个节点</div>
      </div>

      <el-table :data="nodes" v-loading="loading" class="aurora-table">
        <el-table-column prop="id" label="ID" width="72">
          <template #default="{ row }"><span class="k2-id-pill">#{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column prop="name" label="节点名称" min-width="160">
          <template #default="{ row }">
            <div class="name-cell">
              <span class="node-dot" :class="row.status || (row.enable ? 'online' : 'offline')" />
              <div>
                <div class="n-name">{{ row.name }}</div>
                <div class="n-host">{{ row.host }}:{{ row.port }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="线路类型" min-width="150">
          <template #default="{ row }">
            <div class="line-type-cell">
              <span class="proto-tag" :class="lineTypeClass(row)">{{ lineTypeLabel(row) }}</span>
              <span class="line-sub">{{ typeLabel(row.node_type) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="可见范围" min-width="140">
          <template #default="{ row }">
            <span v-if="isUnassignedNode(row)" class="scope-tag unassigned" title="未绑定权限组：不对任何用户开放">未分配</span>
            <span v-else class="scope-tag">{{ getGroupNames(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="服务人数" width="110" align="center">
          <template #default="{ row }">
            <el-tooltip
              placement="top"
              :content="isUnassignedNode(row)
                ? '未绑定权限组，无用户可见该节点'
                : `权限组内可使用该节点的用户数（非实时在线）· 当前在线 ${row.online_count || 0}`"
            >
              <span class="user-count-chip" :class="{ empty: !(row.user_count > 0) }">
                {{ row.user_count ?? 0 }} 人
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="传输 / 安全" width="150">
          <template #default="{ row }">
            <div class="tx-cell">
              <span class="k2-muted-chip">{{ row.network || 'tcp' }}</span>
              <span v-if="row.tls === 2" class="tls-tag reality">REALITY</span>
              <span v-else-if="row.tls === 1" class="tls-tag tls">TLS</span>
              <span v-else class="tls-tag none">无TLS</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="150">
          <template #default="{ row }">
            <el-tooltip
              :content="`CPU ${(row.cpu || 0).toFixed(0)}% · MEM ${(row.mem || 0).toFixed(0)}% · DISK ${(row.disk || 0).toFixed(0)}% · CONN ${row.active_conns || 0}`"
              placement="top"
              :disabled="row.status === 'offline' || row.status === 'disabled'"
            >
              <span class="status-pill" :class="row.status || 'offline'">
                <i />{{ statusLabel(row) }}
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right" align="center">
          <template #default="{ row }">
            <el-dropdown trigger="click" @command="(c: string) => onCmd(c, row)">
              <el-button class="k2-action-btn" size="small">
                操作 <el-icon><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit"><el-icon><Edit /></el-icon> 编辑</el-dropdown-item>
                  <el-dropdown-item command="delete" class="danger-item">
                    <el-icon><Delete /></el-icon> 删除
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- Monitor — command-center style -->
    <div v-show="tab === 'monitor'" class="mon-wrap" v-loading="loading">
      <div class="mon-kpi">
        <div class="mk mk-on">
          <span>在线节点</span>
          <b>{{ monStats.online }}<small>/{{ monStats.total }}</small></b>
        </div>
        <div class="mk mk-warn">
          <span>告警 / 异常</span>
          <b>{{ monStats.warning }}</b>
        </div>
        <div class="mk mk-off">
          <span>离线</span>
          <b>{{ monStats.offline }}</b>
        </div>
        <div class="mk mk-user">
          <span>在线用户合计</span>
          <b>{{ monStats.users }}</b>
        </div>
        <div class="mk mk-conn">
          <span>活跃连接</span>
          <b>{{ monStats.conns }}</b>
        </div>
        <div class="mk mk-cpu">
          <span>平均 CPU</span>
          <b>{{ monStats.avgCpu }}<small>%</small></b>
        </div>
      </div>

      <div class="mon-toolbar">
        <div class="mon-filters">
          <button
            v-for="f in monFilters"
            :key="f.key"
            type="button"
            class="mf-btn"
            :class="{ active: monFilter === f.key }"
            @click="monFilter = f.key"
          >{{ f.label }}</button>
        </div>
        <div class="mon-sort">
          <span class="dim">排序</span>
          <el-select v-model="monSort" size="small" style="width:130px">
            <el-option value="status" label="状态优先" />
            <el-option value="online" label="在线用户" />
            <el-option value="cpu" label="CPU 负载" />
            <el-option value="conn" label="连接数" />
            <el-option value="name" label="名称" />
          </el-select>
          <el-switch v-model="monAuto" active-text="自动刷新" inline-prompt />
        </div>
      </div>

      <div class="mon-stage">
        <div class="mon-grid">
          <div
            v-for="n in filteredMonNodes"
            :key="n.id"
            class="rack-card"
            :class="[n.status || 'offline', { selected: selId === n.id, hot: loadLevel(n) >= 2 }]"
            @click="selectMon(n)"
          >
            <div class="rack-glow" />
            <div class="rack-head">
              <div class="rack-id">
                <span class="pulse-ring" :class="n.status || 'offline'" />
                <div>
                  <div class="rack-name">{{ n.name }}</div>
                  <div class="rack-host">{{ n.host }}:{{ n.port }}</div>
                </div>
              </div>
              <div class="rack-tags">
                <span class="proto-tag sm" :class="n.node_type">{{ typeLabel(n.node_type) }}</span>
                <span class="st-chip" :class="n.status || 'offline'">{{ statusLabel(n) }}</span>
              </div>
            </div>

            <div class="rack-metrics">
              <div class="ring-box">
                <svg viewBox="0 0 72 72" class="ring">
                  <circle cx="36" cy="36" r="28" class="track" />
                  <circle
                    cx="36" cy="36" r="28" class="arc cpu"
                    :style="ringStyle(n.cpu || 0, cpuColor(n.cpu))"
                  />
                </svg>
                <div class="ring-txt">
                  <b :style="{ color: cpuColor(n.cpu) }">{{ Math.round(n.cpu || 0) }}</b>
                  <span>CPU</span>
                </div>
              </div>
              <div class="ring-box">
                <svg viewBox="0 0 72 72" class="ring">
                  <circle cx="36" cy="36" r="28" class="track" />
                  <circle
                    cx="36" cy="36" r="28" class="arc"
                    :style="ringStyle(n.mem || 0, memColor(n.mem))"
                  />
                </svg>
                <div class="ring-txt">
                  <b :style="{ color: memColor(n.mem) }">{{ Math.round(n.mem || 0) }}</b>
                  <span>MEM</span>
                </div>
              </div>
              <div class="ring-box">
                <svg viewBox="0 0 72 72" class="ring">
                  <circle cx="36" cy="36" r="28" class="track" />
                  <circle
                    cx="36" cy="36" r="28" class="arc"
                    :style="ringStyle(n.disk || 0, diskColor(n.disk))"
                  />
                </svg>
                <div class="ring-txt">
                  <b :style="{ color: diskColor(n.disk) }">{{ Math.round(n.disk || 0) }}</b>
                  <span>DISK</span>
                </div>
              </div>
            </div>

            <div class="load-bars">
              <div class="lb"><i class="cpu" :style="{ width: clampPct(n.cpu) + '%' }" /></div>
              <div class="lb"><i class="mem" :style="{ width: clampPct(n.mem) + '%' }" /></div>
              <div class="lb"><i class="disk" :style="{ width: clampPct(n.disk) + '%' }" /></div>
            </div>

            <div class="rack-foot">
              <div class="stat">
                <span>用户</span>
                <b>{{ n.online_count || 0 }}</b>
              </div>
              <div class="stat">
                <span>连接</span>
                <b>{{ n.active_conns || 0 }}</b>
              </div>
              <div class="stat">
                <span>运行</span>
                <b>{{ fmtUp(n.uptime) }}</b>
              </div>
              <div class="load-badge" :class="'lv' + loadLevel(n)">
                {{ loadText(n) }}
              </div>
            </div>
          </div>

          <div v-if="!filteredMonNodes.length && !loading" class="mon-empty">
            当前筛选下没有节点
          </div>
        </div>

        <transition name="slide-panel">
          <div v-if="selId" class="trend-console">
            <div class="tc-head">
              <div class="tc-title">
                <span class="pulse-ring sm" :class="selNode?.status || 'offline'" />
                <div>
                  <h4>{{ selName }}</h4>
                  <p>{{ selNode?.host }} · {{ typeLabel(selNode?.node_type || '') }} · 点击卡片可取消选中</p>
                </div>
              </div>
              <div class="tc-actions">
                <el-radio-group v-model="th" size="small" @change="loadTrend">
                  <el-radio-button :value="1">1h</el-radio-button>
                  <el-radio-button :value="6">6h</el-radio-button>
                  <el-radio-button :value="24">24h</el-radio-button>
                </el-radio-group>
                <el-button size="small" text @click="selId = 0">关闭</el-button>
              </div>
            </div>
            <div class="tc-quick">
              <div class="tq"><span>CPU</span><b>{{ Math.round(selNode?.cpu || 0) }}%</b></div>
              <div class="tq"><span>内存</span><b>{{ Math.round(selNode?.mem || 0) }}%</b></div>
              <div class="tq"><span>磁盘</span><b>{{ Math.round(selNode?.disk || 0) }}%</b></div>
              <div class="tq"><span>连接</span><b>{{ selNode?.active_conns || 0 }}</b></div>
              <div class="tq"><span>在线用户</span><b>{{ selNode?.online_count || 0 }}</b></div>
              <div class="tq"><span>Uptime</span><b>{{ fmtUp(selNode?.uptime || 0) }}</b></div>
            </div>
            <div ref="tcDom" class="trend-chart" />
          </div>
        </transition>
      </div>
    </div>

    <NodeEditDialog v-model:visible="dialogVisible" :node="editingNode" @saved="fetchList" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, ArrowDown, Edit, Delete } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import request from '@/api/request'
import { getNodeList, deleteNode, type Node } from '@/api/node'
import NodeEditDialog from './NodeEditDialog.vue'

interface Group { id: number; name: string }
const route = useRoute()
// View driven only by sidebar route: /nodes vs /nodes?tab=monitor
const tab = ref<'manage' | 'monitor'>(route.query.tab === 'monitor' ? 'monitor' : 'manage')

watch(() => route.query.tab, (v) => {
  const next = v === 'monitor' ? 'monitor' : 'manage'
  if (tab.value !== next) {
    tab.value = next
    if (next === 'monitor') fetchNodes()
    else fetchList()
  }
})
const groups = ref<Group[]>([])
const nodes = ref<Node[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editingNode = ref<Node | null>(null)
const filterType = ref('')
const selId = ref(0)
const selName = ref('')
const th = ref(6)
const tcDom = ref<HTMLDivElement>()
const monFilter = ref<'all' | 'online' | 'warning' | 'offline'>('all')
const monSort = ref('status')
const monAuto = ref(true)
let tc: echarts.ECharts | null = null
let monTimer: ReturnType<typeof setInterval> | null = null

const monFilters = [
  { key: 'all' as const, label: '全部' },
  { key: 'online' as const, label: '在线' },
  { key: 'warning' as const, label: '告警' },
  { key: 'offline' as const, label: '离线' },
]

const RING = 2 * Math.PI * 28

const monStats = computed(() => {
  const list = nodes.value
  const total = list.length
  let online = 0, warning = 0, offline = 0, users = 0, conns = 0, cpuSum = 0, cpuN = 0
  for (const n of list) {
    const st = n.status || (n.enable ? 'offline' : 'disabled')
    if (st === 'online') online++
    else if (st === 'warning') warning++
    else offline++
    users += n.online_count || 0
    conns += n.active_conns || 0
    if (st === 'online' || st === 'warning') {
      cpuSum += n.cpu || 0
      cpuN++
    }
  }
  return {
    total, online, warning, offline, users, conns,
    avgCpu: cpuN ? Math.round(cpuSum / cpuN) : 0,
  }
})

const selNode = computed(() => nodes.value.find(n => n.id === selId.value) || null)

const filteredMonNodes = computed(() => {
  let list = [...nodes.value]
  if (monFilter.value !== 'all') {
    list = list.filter(n => {
      const st = n.status || (n.enable ? 'offline' : 'disabled')
      if (monFilter.value === 'offline') return st === 'offline' || st === 'disabled'
      return st === monFilter.value
    })
  }
  const rank: Record<string, number> = { online: 0, warning: 1, offline: 2, disabled: 3 }
  list.sort((a, b) => {
    if (monSort.value === 'online') return (b.online_count || 0) - (a.online_count || 0)
    if (monSort.value === 'cpu') return (b.cpu || 0) - (a.cpu || 0)
    if (monSort.value === 'conn') return (b.active_conns || 0) - (a.active_conns || 0)
    if (monSort.value === 'name') return (a.name || '').localeCompare(b.name || '')
    const sa = rank[a.status || 'offline'] ?? 9
    const sb = rank[b.status || 'offline'] ?? 9
    if (sa !== sb) return sa - sb
    return (b.online_count || 0) - (a.online_count || 0)
  })
  return list
})

function fmtUp(s: number) {
  if (!s) return '-'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h}h${m}m` : `${m}m`
}
function cpuColor(v: number) { return v > 80 ? '#ef4444' : v > 60 ? '#f59e0b' : '#6366f1' }
function memColor(v: number) { return v > 85 ? '#ef4444' : v > 65 ? '#f59e0b' : '#10b981' }
function diskColor(v: number) { return v > 85 ? '#ef4444' : v > 70 ? '#f59e0b' : '#a78bfa' }
function clampPct(v?: number) { return Math.max(0, Math.min(100, Math.round(v || 0))) }
function ringStyle(pct: number, color: string) {
  const p = clampPct(pct)
  const offset = RING * (1 - p / 100)
  return {
    stroke: color,
    strokeDasharray: `${RING}`,
    strokeDashoffset: `${offset}`,
  }
}
function loadLevel(n: Node) {
  const m = Math.max(n.cpu || 0, n.mem || 0, n.disk || 0)
  if (m >= 85) return 3
  if (m >= 65) return 2
  if (m >= 35) return 1
  return 0
}
function loadText(n: Node) {
  return ['空闲', '正常', '偏高', '过载'][loadLevel(n)]
}
function typeLabel(t: string) {
  const m: Record<string, string> = { v2ray: 'VMess', vless: 'VLESS', trojan: 'Trojan', shadowsocks: 'SS', anytls: 'AnyTLS' }
  return m[t] || t || '-'
}
/** Product-facing line shape (A/B/C) */
function lineTypeLabel(row: any) {
  if (!row) return '-'
  if (row.node_type === 'anytls') return 'C · AnyTLS'
  const net = String(row.network || '').toLowerCase()
  if (row.node_type === 'vless' && row.tls === 2) return 'A · 直连 REALITY'
  if (row.node_type === 'vless' && row.tls === 1 && (net === 'xhttp' || net === 'splithttp')) return 'B · CDN XHTTP'
  if (row.node_type === 'vless' && row.tls === 1) return 'VLESS · TLS'
  return typeLabel(row.node_type)
}
function lineTypeClass(row: any) {
  if (row?.node_type === 'anytls') return 'anytls'
  const net = String(row?.network || '').toLowerCase()
  if (row?.tls === 2) return 'reality-line'
  if (net === 'xhttp' || net === 'splithttp') return 'cdn-line'
  return row?.node_type || ''
}
function statusLabel(r: any) {
  const m: Record<string, string> = { online: '运行正常', warning: '心跳异常', offline: '未运行', disabled: '已禁用' }
  return m[r?.status] || (r?.enable ? '未知' : '已禁用')
}
async function fetchGroups() {
  try {
    const r = await request.get('/admin/groups')
    groups.value = r.data || []
  } catch { /* ignore */ }
}
function isUnassignedNode(row: any): boolean {
  if (Array.isArray(row.group_ids)) return row.group_ids.length === 0
  return !row.group_id
}
function getGroupNames(row: any): string {
  const ids: number[] = Array.isArray(row.group_ids) ? row.group_ids : (row.group_id ? [row.group_id] : [])
  if (!ids.length) return '未分配'
  return ids.map((id: number) => groups.value.find(g => g.id === id)?.name || '#' + id).join(', ')
}
async function fetchList() {
  loading.value = true
  try {
    const r = await getNodeList({ node_type: filterType.value || undefined })
    nodes.value = r.data
  } catch { /* ignore */ }
  loading.value = false
}
async function fetchNodes(silent = false) {
  if (!silent) loading.value = true
  try {
    const r = await request.get('/admin/nodes')
    nodes.value = r.data || []
    if (selId.value) nextTick(() => loadTrend())
  } catch { /* ignore */ }
  if (!silent) loading.value = false
}
function showCreate() {
  editingNode.value = null
  dialogVisible.value = true
}
function showEdit(n: Node) {
  editingNode.value = { ...n, group_ids: n.group_ids || [] }
  dialogVisible.value = true
}
async function onCmd(c: string, row: Node) {
  if (c === 'edit') showEdit(row)
  if (c === 'delete') {
    try {
      await ElMessageBox.confirm(`确认删除节点 ${row.name}？`, '删除节点', { type: 'warning' })
      await deleteNode(row.id)
      ElMessage.success('已删除')
      fetchList()
    } catch { /* cancel */ }
  }
}

function selectMon(n: Node) {
  if (selId.value === n.id) {
    selId.value = 0
    return
  }
  selId.value = n.id
  selName.value = n.name
  nextTick(() => loadTrend())
}

async function loadTrend() {
  if (!tcDom.value || !selId.value) return
  if (!tc) {
    tc = echarts.init(tcDom.value, undefined, { renderer: 'canvas' })
  }
  try {
    const r = await request.get(`/admin/nodes/${selId.value}/metrics`, { params: { hours: th.value } })
    const d = r.data || []
    if (!d.length) {
      tc.setOption({
        backgroundColor: 'transparent',
        title: { text: '暂无历史指标', left: 'center', top: 'center', textStyle: { color: '#64748b', fontSize: 14, fontWeight: 600 } },
        series: [],
      }, true)
      return
    }
    tc.setOption({
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(15,23,42,0.92)',
        borderColor: 'rgba(99,102,241,0.35)',
        textStyle: { color: '#e2e8f0', fontSize: 12 },
      },
      legend: {
        data: ['CPU%', '内存%', '磁盘%', '连接数'],
        top: 4,
        textStyle: { color: '#94a3b8' },
        icon: 'roundRect',
      },
      grid: { left: 48, right: 48, top: 44, bottom: 28 },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: d.map((x: any) => new Date(x.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
        axisLabel: { fontSize: 10, color: '#64748b' },
        axisLine: { lineStyle: { color: 'rgba(148,163,184,0.25)' } },
        axisTick: { show: false },
      },
      yAxis: [
        {
          type: 'value', max: 100,
          axisLabel: { formatter: '{value}%', color: '#64748b', fontSize: 10 },
          splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } },
        },
        {
          type: 'value', name: '连接',
          nameTextStyle: { color: '#64748b', fontSize: 10 },
          axisLabel: { fontSize: 10, color: '#64748b' },
          splitLine: { show: false },
        },
      ],
      series: [
        {
          name: 'CPU%', type: 'line', data: d.map((x: any) => x.cpu), smooth: true, symbol: 'none',
          lineStyle: { width: 2.2, color: '#818cf8' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(129,140,248,0.35)' },
              { offset: 1, color: 'rgba(129,140,248,0.02)' },
            ]),
          },
        },
        {
          name: '内存%', type: 'line', data: d.map((x: any) => x.mem), smooth: true, symbol: 'none',
          lineStyle: { width: 2.2, color: '#34d399' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(52,211,153,0.28)' },
              { offset: 1, color: 'rgba(52,211,153,0.02)' },
            ]),
          },
        },
        {
          name: '磁盘%', type: 'line', data: d.map((x: any) => x.disk), smooth: true, symbol: 'none',
          lineStyle: { width: 2, color: '#fbbf24' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(251,191,36,0.22)' },
              { offset: 1, color: 'rgba(251,191,36,0.02)' },
            ]),
          },
        },
        {
          name: '连接数', type: 'line', yAxisIndex: 1,
          data: d.map((x: any) => x.active_conns || 0),
          smooth: true, symbol: 'none',
          lineStyle: { width: 2, type: 'dashed', color: '#c084fc' },
        },
      ],
    }, true)
    tc.resize()
  } catch { /* ignore */ }
}

function startMonAuto() {
  if (monTimer) clearInterval(monTimer)
  monTimer = setInterval(() => {
    if (tab.value === 'monitor' && monAuto.value) fetchNodes(true)
  }, 10000)
}

watch(tab, (v) => {
  if (v === 'monitor') {
    fetchNodes()
    nextTick(() => { if (selId.value) loadTrend() })
  }
})

onMounted(() => {
  fetchList()
  fetchGroups()
  startMonAuto()
  window.addEventListener('resize', () => tc?.resize())
})

onUnmounted(() => {
  if (monTimer) clearInterval(monTimer)
  tc?.dispose()
  tc = null
})
</script>

<style scoped>
.header-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.toolbar-meta {
  font-size: 13px;
  color: var(--k2-text-muted);
}
.toolbar-meta b {
  color: var(--k2-text);
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.node-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
.node-dot.online { background: #10b981; box-shadow: 0 0 0 4px rgba(16, 185, 129, 0.18); }
.node-dot.warning { background: #f59e0b; box-shadow: 0 0 0 4px rgba(245, 158, 11, 0.18); }
.node-dot.offline, .node-dot.disabled { background: #cbd5e1; }
.n-name { font-weight: 700; color: #0f172a; font-size: 13px; }
.n-host { font-size: 11px; color: #94a3b8; margin-top: 2px; }

.proto-tag {
  display: inline-block;
  font-size: 11px;
  font-weight: 800;
  padding: 4px 10px;
  border-radius: 8px;
  letter-spacing: 0.02em;
}
.proto-tag.vless, .proto-tag.sm.vless { background: #fff7ed; color: #c2410c; }
.proto-tag.anytls, .proto-tag.sm.anytls { background: #eef2ff; color: #4f46e5; }
.proto-tag.v2ray { background: #eff6ff; color: #1d4ed8; }
.proto-tag.trojan { background: #fef2f2; color: #dc2626; }
.proto-tag.shadowsocks { background: #ecfdf5; color: #059669; }
.proto-tag.reality-line { background: #eef2ff; color: #4338ca; }
.proto-tag.cdn-line { background: #ecfeff; color: #0e7490; }
.proto-tag.sm { font-size: 10px; padding: 2px 8px; }
.line-type-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
}
.line-sub {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 600;
}
.tx-cell {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.scope-tag {
  font-size: 12px;
  font-weight: 600;
  color: #4338ca;
  background: #eef2ff;
  padding: 4px 10px;
  border-radius: 8px;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.scope-tag.unassigned {
  color: #c2410c;
  background: #fff7ed;
}

.user-count-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 3.25rem;
  font-size: 12px;
  font-weight: 700;
  color: #0f766e;
  background: #ecfdf5;
  padding: 4px 10px;
  border-radius: 999px;
  cursor: default;
}
.user-count-chip.empty {
  color: #94a3b8;
  background: #f1f5f9;
}

.tls-tag {
  font-size: 11px;
  font-weight: 800;
  padding: 4px 10px;
  border-radius: 8px;
}
.tls-tag.reality { background: #fef3c7; color: #b45309; }
.tls-tag.tls { background: #ecfdf5; color: #059669; }
.tls-tag.none { background: #f8fafc; color: #94a3b8; }

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  padding: 5px 10px;
  border-radius: 999px;
}
.status-pill i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.status-pill.online { color: #059669; background: #ecfdf5; }
.status-pill.online i { background: #10b981; }
.status-pill.warning { color: #d97706; background: #fffbeb; }
.status-pill.warning i { background: #f59e0b; }
.status-pill.offline, .status-pill.disabled { color: #94a3b8; background: #f8fafc; }
.status-pill.offline i, .status-pill.disabled i { background: #cbd5e1; }
.status-pill.sm { font-size: 11px; padding: 3px 8px; }

/* ── Monitor command center ── */
.mon-wrap { display: flex; flex-direction: column; gap: 14px; }
.mon-kpi {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 10px;
}
.mk {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 14px;
  padding: 12px 14px;
  box-shadow: var(--k2-shadow-sm);
  position: relative;
  overflow: hidden;
}
.mk::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
}
.mk-on::before { background: #10b981; }
.mk-warn::before { background: #f59e0b; }
.mk-off::before { background: #94a3b8; }
.mk-user::before { background: #06b6d4; }
.mk-conn::before { background: #8b5cf6; }
.mk-cpu::before { background: #6366f1; }
.mk span {
  display: block;
  font-size: 11px;
  font-weight: 700;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.mk b {
  display: block;
  margin-top: 4px;
  font-size: 22px;
  font-weight: 800;
  color: #0f172a;
  letter-spacing: -0.03em;
}
.mk small { font-size: 13px; color: #94a3b8; font-weight: 600; margin-left: 2px; }

.mon-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.mon-filters {
  display: inline-flex;
  gap: 4px;
  padding: 4px;
  background: #f1f5f9;
  border-radius: 12px;
}
.mf-btn {
  border: none;
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  font-weight: 700;
  color: #64748b;
  padding: 7px 14px;
  border-radius: 9px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.mf-btn.active {
  background: #fff;
  color: #4f46e5;
  box-shadow: 0 2px 8px rgba(79, 70, 229, 0.15);
}
.mon-sort {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mon-sort .dim { font-size: 12px; color: #94a3b8; font-weight: 600; }

.mon-stage { display: flex; flex-direction: column; gap: 14px; }
.mon-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}
.mon-empty {
  grid-column: 1 / -1;
  text-align: center;
  padding: 48px;
  color: #94a3b8;
  font-weight: 600;
}

.rack-card {
  position: relative;
  border-radius: 18px;
  padding: 16px 16px 14px;
  cursor: pointer;
  background: linear-gradient(165deg, #0f172a 0%, #111827 55%, #0b1220 100%);
  border: 1px solid rgba(148, 163, 184, 0.12);
  color: #e2e8f0;
  overflow: hidden;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}
.rack-card:hover {
  transform: translateY(-3px);
  border-color: rgba(129, 140, 248, 0.35);
  box-shadow: 0 16px 40px rgba(15, 23, 42, 0.35);
}
.rack-card.selected {
  border-color: rgba(34, 211, 238, 0.55);
  box-shadow: 0 0 0 1px rgba(34, 211, 238, 0.25), 0 20px 50px rgba(6, 182, 212, 0.18);
}
.rack-card.online { box-shadow: inset 0 0 0 1px rgba(16, 185, 129, 0.12); }
.rack-card.warning { box-shadow: inset 0 0 0 1px rgba(245, 158, 11, 0.18); }
.rack-card.hot .rack-glow {
  opacity: 0.55;
}
.rack-glow {
  position: absolute;
  width: 180px;
  height: 180px;
  right: -40px;
  top: -60px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.35), transparent 70%);
  pointer-events: none;
  opacity: 0.35;
  transition: opacity 0.2s ease;
}
.rack-card.online .rack-glow {
  background: radial-gradient(circle, rgba(16, 185, 129, 0.3), transparent 70%);
}
.rack-card.warning .rack-glow {
  background: radial-gradient(circle, rgba(245, 158, 11, 0.35), transparent 70%);
}

.rack-head {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 14px;
  position: relative;
  z-index: 1;
}
.rack-id {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  min-width: 0;
}
.pulse-ring {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  margin-top: 4px;
  flex-shrink: 0;
  background: #64748b;
}
.pulse-ring.online {
  background: #10b981;
  box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.5);
  animation: monPulse 1.8s infinite;
}
.pulse-ring.warning {
  background: #f59e0b;
  box-shadow: 0 0 0 0 rgba(245, 158, 11, 0.45);
  animation: monPulse 1.8s infinite;
}
.pulse-ring.sm { width: 10px; height: 10px; margin-top: 6px; }
@keyframes monPulse {
  0% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.5); }
  70% { box-shadow: 0 0 0 10px rgba(16, 185, 129, 0); }
  100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}
.rack-name {
  font-size: 14px;
  font-weight: 800;
  color: #f8fafc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 160px;
}
.rack-host {
  font-size: 11px;
  color: #64748b;
  margin-top: 2px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.rack-tags {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}
.st-chip {
  font-size: 10px;
  font-weight: 800;
  padding: 3px 8px;
  border-radius: 999px;
  letter-spacing: 0.02em;
}
.st-chip.online { color: #6ee7b7; background: rgba(16, 185, 129, 0.15); }
.st-chip.warning { color: #fcd34d; background: rgba(245, 158, 11, 0.15); }
.st-chip.offline, .st-chip.disabled { color: #94a3b8; background: rgba(148, 163, 184, 0.12); }

.rack-metrics {
  display: flex;
  justify-content: space-around;
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
}
.ring-box {
  position: relative;
  width: 72px;
  height: 72px;
}
.ring {
  width: 72px;
  height: 72px;
  transform: rotate(-90deg);
}
.ring .track {
  fill: none;
  stroke: rgba(148, 163, 184, 0.12);
  stroke-width: 6;
}
.ring .arc {
  fill: none;
  stroke-width: 6;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.6s ease;
}
.ring-txt {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.ring-txt b {
  font-size: 14px;
  font-weight: 800;
  line-height: 1;
}
.ring-txt span {
  font-size: 9px;
  font-weight: 700;
  color: #64748b;
  margin-top: 2px;
  letter-spacing: 0.06em;
}

.load-bars {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 12px;
  position: relative;
  z-index: 1;
}
.lb {
  height: 4px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}
.lb i {
  display: block;
  height: 100%;
  border-radius: 999px;
  transition: width 0.5s ease;
}
.lb i.cpu { background: linear-gradient(90deg, #6366f1, #a5b4fc); }
.lb i.mem { background: linear-gradient(90deg, #059669, #34d399); }
.lb i.disk { background: linear-gradient(90deg, #d97706, #fbbf24); }

.rack-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  position: relative;
  z-index: 1;
  padding-top: 10px;
  border-top: 1px solid rgba(148, 163, 184, 0.1);
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 44px;
}
.stat span {
  font-size: 9px;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.stat b {
  font-size: 13px;
  font-weight: 800;
  color: #e2e8f0;
}
.load-badge {
  margin-left: auto;
  font-size: 10px;
  font-weight: 800;
  padding: 4px 10px;
  border-radius: 999px;
  letter-spacing: 0.04em;
}
.load-badge.lv0 { color: #94a3b8; background: rgba(148, 163, 184, 0.12); }
.load-badge.lv1 { color: #6ee7b7; background: rgba(16, 185, 129, 0.15); }
.load-badge.lv2 { color: #fcd34d; background: rgba(245, 158, 11, 0.15); }
.load-badge.lv3 { color: #fca5a5; background: rgba(239, 68, 68, 0.18); }

.trend-console {
  border-radius: 18px;
  padding: 16px 18px 12px;
  background: linear-gradient(180deg, #0f172a 0%, #0b1220 100%);
  border: 1px solid rgba(99, 102, 241, 0.25);
  box-shadow: 0 16px 48px rgba(15, 23, 42, 0.35);
}
.tc-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.tc-title {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.tc-title h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: #f8fafc;
}
.tc-title p {
  margin: 3px 0 0;
  font-size: 12px;
  color: #64748b;
}
.tc-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tc-quick {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 8px;
  margin-bottom: 10px;
}
.tq {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(148, 163, 184, 0.1);
  border-radius: 12px;
  padding: 8px 10px;
}
.tq span {
  display: block;
  font-size: 10px;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
}
.tq b {
  display: block;
  margin-top: 2px;
  font-size: 15px;
  font-weight: 800;
  color: #e2e8f0;
}
.trend-chart { height: 300px; }

.slide-panel-enter-active,
.slide-panel-leave-active {
  transition: all 0.28s ease;
}
.slide-panel-enter-from,
.slide-panel-leave-to {
  opacity: 0;
  transform: translateY(12px);
}

:deep(.danger-item) { color: #ef4444 !important; }
:deep(.aurora-table .el-table__cell) { padding: 14px 0; }

@media (max-width: 1100px) {
  .mon-kpi { grid-template-columns: repeat(3, 1fr); }
  .tc-quick { grid-template-columns: repeat(3, 1fr); }
}
@media (max-width: 640px) {
  .mon-kpi { grid-template-columns: repeat(2, 1fr); }
  .tc-quick { grid-template-columns: repeat(2, 1fr); }
  .rack-name { max-width: 120px; }
}
</style>
