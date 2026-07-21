<template>
  <div class="traffic-page">
    <div class="k2-page-header">
      <div>
        <h3>流量分析</h3>
        <p class="sub">
          聚合洞察优先 · 明细按需加载 ·
          <span class="hint">千级用户下避免全表扫明细</span>
        </p>
      </div>
      <div class="header-actions">
        <el-select v-model="hours" size="large" style="width: 140px" @change="onRangeChange">
          <el-option :value="1" label="近 1 小时" />
          <el-option :value="6" label="近 6 小时" />
          <el-option :value="24" label="近 24 小时" />
          <el-option :value="48" label="近 48 小时" />
          <el-option :value="168" label="近 7 天" />
          <el-option :value="720" label="近 30 天" />
        </el-select>
        <el-button size="large" :loading="loadingStats" @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <!-- Filters -->
    <div class="filter-bar">
      <el-input
        v-model="filters.email"
        clearable
        size="large"
        placeholder="按邮箱筛选（明细 / 聚焦）"
        class="f-email"
        @keyup.enter="applyFilters"
        @clear="applyFilters"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-input
        v-model="filters.userId"
        clearable
        size="large"
        placeholder="用户 ID"
        class="f-uid"
        @keyup.enter="applyFilters"
        @clear="applyFilters"
      />
      <el-select
        v-model="filters.nodeId"
        clearable
        filterable
        size="large"
        placeholder="全部节点"
        class="f-node"
        @change="applyFilters"
      >
        <el-option
          v-for="n in nodeOptions"
          :key="n.id"
          :label="n.name"
          :value="n.id"
        />
      </el-select>
      <el-button type="primary" size="large" @click="applyFilters">应用筛选</el-button>
      <el-button size="large" text @click="clearFilters" v-if="hasFilters">清除</el-button>
    </div>

    <!-- KPI -->
    <div class="stat-grid">
      <div class="stat-card c1">
        <div class="s-top">
          <span class="s-label">窗口合计</span>
          <span class="s-chip">{{ hoursLabel }}</span>
        </div>
        <div class="s-val">{{ formatBytes(stats.total || 0) }}</div>
        <div class="s-sub">
          ↑ {{ formatBytes(stats.total_upload || 0) }}
          · ↓ {{ formatBytes(stats.total_download || 0) }}
        </div>
      </div>
      <div class="stat-card c2">
        <div class="s-top"><span class="s-label">活跃用户</span></div>
        <div class="s-val">{{ stats.active_users ?? 0 }}</div>
        <div class="s-sub">窗口内产生流量的去重用户</div>
      </div>
      <div class="stat-card c3">
        <div class="s-top"><span class="s-label">人均流量</span></div>
        <div class="s-val">{{ formatBytes(stats.avg_per_user || 0) }}</div>
        <div class="s-sub">合计 ÷ 活跃用户</div>
      </div>
      <div class="stat-card c4">
        <div class="s-top"><span class="s-label">峰值时段</span></div>
        <div class="s-val sm">{{ stats.peak_bucket || '—' }}</div>
        <div class="s-sub">{{ stats.peak_total ? formatBytes(stats.peak_total) : '暂无峰值' }}</div>
      </div>
    </div>

    <div v-if="stats.log_rows != null" class="scale-note">
      <el-icon><InfoFilled /></el-icon>
      本窗口原始刷盘记录约 <b>{{ formatNum(stats.log_rows) }}</b> 行
      · 图表与排行均为 SQL 聚合 Top {{ stats.rank_limit || 30 }}
      · 明细日志请使用下方筛选后分页查看
    </div>

    <!-- Trend -->
    <div class="chart-panel">
      <div class="panel-head">
        <div>
          <h4>流量趋势</h4>
          <p>
            {{ stats.granularity === 'day' ? '按日聚合（长窗口）' : '按小时聚合' }}
            · 适合观察尖峰与增长
          </p>
        </div>
        <div class="legend-pills">
          <span class="pill up">上传</span>
          <span class="pill down">下载</span>
        </div>
      </div>
      <div ref="seriesChart" class="chart-box trend" v-loading="loadingStats" />
    </div>

    <!-- Node + Ranking -->
    <div class="split-grid">
      <div class="chart-panel">
        <div class="panel-head">
          <div>
            <h4>节点分布</h4>
            <p>Top 节点 · 点击可筛选明细</p>
          </div>
        </div>
        <div ref="nodeChart" class="chart-box mid" v-loading="loadingStats" />
        <div v-if="(stats.nodes || []).length" class="mini-table">
          <div
            v-for="(n, i) in (stats.nodes || []).slice(0, 8)"
            :key="n.node_id"
            class="mini-row"
            @click="focusNode(n.node_id)"
          >
            <span class="rank">{{ i + 1 }}</span>
            <span class="name" :title="n.name">{{ n.name }}</span>
            <div class="bar-wrap">
              <div class="bar" :style="{ width: Math.min(100, n.share || 0) + '%' }" />
            </div>
            <span class="bytes">{{ formatBytes(n.total || n.upload + n.download) }}</span>
            <span class="share">{{ (n.share || 0).toFixed(1) }}%</span>
          </div>
        </div>
      </div>

      <div class="chart-panel">
        <div class="panel-head">
          <div>
            <h4>用户排行</h4>
            <p>Top {{ stats.rank_limit || 30 }} · 点击行查看该用户明细</p>
          </div>
          <el-input
            v-model="rankSearch"
            clearable
            size="small"
            placeholder="过滤本页排行…"
            style="width: 160px"
          />
        </div>
        <div class="rank-table-wrap" v-loading="loadingStats">
          <table class="rank-table" v-if="filteredRanking.length">
            <thead>
              <tr>
                <th style="width:40px">#</th>
                <th>用户</th>
                <th style="width:28%">占比</th>
                <th style="width:100px" class="r">流量</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(u, i) in filteredRanking"
                :key="u.user_id"
                @click="focusUser(u)"
                :class="{ active: String(filters.userId) === String(u.user_id) }"
              >
                <td>
                  <span class="badge" :class="'b' + Math.min(i + 1, 4)">{{ i + 1 }}</span>
                </td>
                <td>
                  <div class="u-cell">
                    <span class="avatar">{{ (u.email || '?').charAt(0).toUpperCase() }}</span>
                    <div>
                      <div class="email">{{ u.email }}</div>
                      <div class="uid">ID {{ u.user_id }}</div>
                    </div>
                  </div>
                </td>
                <td>
                  <div class="share-cell">
                    <div class="bar-wrap">
                      <div class="bar user" :style="{ width: Math.min(100, u.share || 0) + '%' }" />
                    </div>
                    <span>{{ (u.share || 0).toFixed(1) }}%</span>
                  </div>
                </td>
                <td class="r">
                  <div class="bytes-main">{{ formatBytes(u.total || u.upload + u.download) }}</div>
                  <div class="bytes-sub">↑{{ formatBytes(u.upload) }} ↓{{ formatBytes(u.download) }}</div>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty">该窗口暂无用户流量</div>
        </div>
      </div>
    </div>

    <!-- Detail logs -->
    <div class="chart-panel logs-panel">
      <div class="panel-head">
        <div>
          <h4>刷盘明细</h4>
          <p>
            每次调度 flush 的 (user × node) 记录 ·
            <template v-if="logsFiltered">已筛选 · 适合下钻</template>
            <template v-else>
              <span class="warn-text">未筛选时仅展示最近一页，千级用户请先选用户/节点</span>
            </template>
          </p>
        </div>
        <div class="log-meta">
          共 <b>{{ logTotal }}</b> 条
          <el-button size="small" text type="primary" @click="fetchLogs" :loading="loadingLogs">
            重新加载
          </el-button>
        </div>
      </div>

      <el-table
        :data="logs"
        v-loading="loadingLogs"
        class="log-table"
        empty-text="暂无明细（可放宽时间窗口或清除筛选）"
        @row-click="onLogRowClick"
      >
        <el-table-column prop="id" label="ID" width="80">
          <template #default="{ row }">
            <span class="id-pill">#{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="用户" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="u-cell compact">
              <span class="avatar sm">{{ (row.email || '?').charAt(0).toUpperCase() }}</span>
              <div>
                <div class="email">{{ row.email }}</div>
                <div class="uid">ID {{ row.user_id }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="节点" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="node-chip">{{ row.node_name || '#' + row.node_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="上传" width="110" align="right">
          <template #default="{ row }">
            <span class="up">{{ formatBytes(row.upload) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="下载" width="110" align="right">
          <template #default="{ row }">
            <span class="down">{{ formatBytes(row.download) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="合计" width="110" align="right">
          <template #default="{ row }">
            <b>{{ formatBytes((row.upload || 0) + (row.download || 0)) }}</b>
          </template>
        </el-table-column>
        <el-table-column label="记录时间" width="170">
          <template #default="{ row }">
            {{ formatTime(row.recorded_at) }}
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="logPage"
          v-model:page-size="logPageSize"
          :total="logTotal"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          background
          @current-change="fetchLogs"
          @size-change="onPageSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import * as echarts from 'echarts'
import { Refresh, Search, InfoFilled } from '@element-plus/icons-vue'
import request from '@/api/request'
import { formatBytes } from '@/utils/format'

const hours = ref(24)
const loadingStats = ref(false)
const loadingLogs = ref(false)
const stats = reactive<Record<string, any>>({})
const rankSearch = ref('')

const filters = reactive({
  email: '',
  userId: '' as string | number,
  nodeId: undefined as number | undefined,
})

const nodeOptions = ref<{ id: number; name: string }[]>([])
const logs = ref<any[]>([])
const logTotal = ref(0)
const logPage = ref(1)
const logPageSize = ref(20)
const logsFiltered = ref(false)

const seriesChart = ref<HTMLDivElement>()
const nodeChart = ref<HTMLDivElement>()
let sc: echarts.ECharts | null = null
let nc: echarts.ECharts | null = null

const hoursLabel = computed(() => {
  const m: Record<number, string> = {
    1: '1 小时', 6: '6 小时', 24: '24 小时', 48: '48 小时', 168: '7 天', 720: '30 天',
  }
  return m[hours.value] || `${hours.value}h`
})

const hasFilters = computed(() =>
  !!(filters.email || filters.userId || filters.nodeId),
)

const filteredRanking = computed(() => {
  const list = (stats.ranking || []) as any[]
  const q = rankSearch.value.trim().toLowerCase()
  if (!q) return list
  return list.filter((u) =>
    String(u.email || '').toLowerCase().includes(q) ||
    String(u.user_id).includes(q),
  )
})

function formatNum(n: number) {
  if (n == null) return '0'
  return Number(n).toLocaleString('zh-CN')
}

function formatTime(t: string) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

function statsParams() {
  const p: Record<string, any> = { hours: hours.value }
  if (filters.userId) p.user_id = Number(filters.userId)
  if (filters.nodeId) p.node_id = filters.nodeId
  return p
}

function logsParams() {
  const p: Record<string, any> = {
    hours: hours.value,
    page: logPage.value,
    page_size: logPageSize.value,
  }
  if (filters.userId) p.user_id = Number(filters.userId)
  if (filters.nodeId) p.node_id = filters.nodeId
  if (filters.email.trim()) p.email = filters.email.trim()
  return p
}

function initCharts() {
  if (seriesChart.value && !sc) {
    sc = echarts.init(seriesChart.value)
  }
  if (nodeChart.value && !nc) {
    nc = echarts.init(nodeChart.value)
  }
}

function renderSeries(series: any[]) {
  if (!sc) return
  if (!series?.length) {
    sc.clear()
    sc.setOption({
      title: { text: '暂无趋势数据', left: 'center', top: 'center', textStyle: { color: '#94a3b8', fontSize: 14 } },
    })
    return
  }
  const buckets = series.map((s) => s.bucket)
  const ups = series.map((s) => s.upload)
  const downs = series.map((s) => s.download)
  sc.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'cross', crossStyle: { color: '#cbd5e1' } },
      formatter: (params: any) => {
        const lines = [params[0]?.axisValue || '']
        for (const p of params) {
          lines.push(`${p.marker}${p.seriesName}: ${formatBytes(p.value)}`)
        }
        const sum = params.reduce((a: number, p: any) => a + (p.value || 0), 0)
        lines.push(`合计: ${formatBytes(sum)}`)
        return lines.join('<br/>')
      },
    },
    legend: { show: false },
    grid: { left: 56, right: 24, top: 24, bottom: 36 },
    xAxis: {
      type: 'category',
      data: buckets,
      boundaryGap: false,
      axisLabel: {
        fontSize: 10,
        color: '#94a3b8',
        formatter: (v: string) => {
          // shorten labels
          if (v?.length > 13) return v.slice(5) // MM-DD HH:00
          return v
        },
      },
      axisLine: { lineStyle: { color: '#e2e8f0' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: { fontSize: 11, color: '#94a3b8', formatter: (v: number) => formatBytes(v) },
      splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } },
    },
    series: [
      {
        name: '上传',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: ups,
        lineStyle: { width: 2, color: '#4f46e5' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(79,70,229,0.28)' },
            { offset: 1, color: 'rgba(79,70,229,0.02)' },
          ]),
        },
      },
      {
        name: '下载',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: downs,
        lineStyle: { width: 2, color: '#10b981' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(16,185,129,0.22)' },
            { offset: 1, color: 'rgba(16,185,129,0.02)' },
          ]),
        },
      },
    ],
  }, true)
}

function renderNodeChart(nodes: any[]) {
  if (!nc) return
  if (!nodes?.length) {
    nc.clear()
    nc.setOption({
      title: { text: '暂无节点流量', left: 'center', top: 'center', textStyle: { color: '#94a3b8', fontSize: 14 } },
    })
    return
  }
  const top = nodes.slice(0, 12)
  const names = top.map((n) => n.name || '#' + n.node_id).reverse()
  const ups = top.map((n) => n.upload).reverse()
  const downs = top.map((n) => n.download).reverse()
  nc.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: any) => {
        const name = params[0]?.name || ''
        const lines = [name]
        let sum = 0
        for (const p of params) {
          lines.push(`${p.marker}${p.seriesName}: ${formatBytes(p.value)}`)
          sum += p.value || 0
        }
        lines.push(`合计: ${formatBytes(sum)}`)
        return lines.join('<br/>')
      },
    },
    legend: { data: ['上传', '下载'], top: 0, right: 0, textStyle: { color: '#64748b', fontSize: 11 } },
    grid: { left: 100, right: 16, top: 28, bottom: 8 },
    xAxis: {
      type: 'value',
      axisLabel: { fontSize: 10, color: '#94a3b8', formatter: (v: number) => formatBytes(v) },
      splitLine: { lineStyle: { color: '#f1f5f9' } },
    },
    yAxis: {
      type: 'category',
      data: names,
      axisLabel: { fontSize: 11, width: 90, overflow: 'truncate', color: '#475569' },
      axisTick: { show: false },
      axisLine: { show: false },
    },
    series: [
      {
        name: '上传',
        type: 'bar',
        stack: 't',
        data: ups,
        itemStyle: { color: '#4f46e5', borderRadius: [0, 0, 0, 0] },
        barMaxWidth: 14,
      },
      {
        name: '下载',
        type: 'bar',
        stack: 't',
        data: downs,
        itemStyle: { color: '#34d399', borderRadius: [0, 4, 4, 0] },
        barMaxWidth: 14,
      },
    ],
  }, true)
}

async function fetchStats() {
  loadingStats.value = true
  try {
    const r = await request.get('/admin/traffic-stats', { params: statsParams() })
    Object.keys(stats).forEach((k) => delete stats[k])
    Object.assign(stats, r.data || {})
    await nextTick()
    initCharts()
    renderSeries(stats.series || stats.hourly || [])
    renderNodeChart(stats.nodes || [])
  } catch {
    /* interceptor handles */
  } finally {
    loadingStats.value = false
  }
}

async function fetchLogs() {
  loadingLogs.value = true
  try {
    const r = await request.get('/admin/traffic-logs', { params: logsParams() })
    const body = r.data || {}
    logs.value = body.list || []
    logTotal.value = body.total || 0
    logsFiltered.value = !!body.filtered
  } catch {
    logs.value = []
  } finally {
    loadingLogs.value = false
  }
}

async function fetchNodes() {
  try {
    const r = await request.get('/admin/nodes')
    // API returns node array directly in data
    const list = Array.isArray(r.data) ? r.data : (r.data?.list || [])
    nodeOptions.value = list.map((n: any) => ({
      id: n.id,
      name: n.name || `节点 #${n.id}`,
    }))
  } catch {
    nodeOptions.value = []
  }
}

function onRangeChange() {
  logPage.value = 1
  refreshAll()
}

function applyFilters() {
  logPage.value = 1
  refreshAll()
}

function clearFilters() {
  filters.email = ''
  filters.userId = ''
  filters.nodeId = undefined
  rankSearch.value = ''
  logPage.value = 1
  refreshAll()
}

function focusUser(u: any) {
  filters.userId = u.user_id
  filters.email = ''
  logPage.value = 1
  refreshAll()
  // scroll to logs
  nextTick(() => {
    document.querySelector('.logs-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

function focusNode(nodeId: number) {
  filters.nodeId = nodeId
  logPage.value = 1
  refreshAll()
  nextTick(() => {
    document.querySelector('.logs-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

function onLogRowClick(row: any) {
  if (row?.user_id) {
    filters.userId = row.user_id
    applyFilters()
  }
}

function onPageSizeChange() {
  logPage.value = 1
  fetchLogs()
}

function refreshAll() {
  fetchStats()
  fetchLogs()
}

function onResize() {
  sc?.resize()
  nc?.resize()
}

onMounted(() => {
  initCharts()
  fetchNodes()
  refreshAll()
  window.addEventListener('resize', onResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  sc?.dispose()
  nc?.dispose()
  sc = null
  nc = null
})

// keep charts responsive when ranking filter changes height slightly
watch(filteredRanking, () => nextTick(onResize))
</script>

<style scoped>
.hint { color: #94a3b8; font-weight: 500; }
.header-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 16px;
  padding: 14px 16px;
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 16px;
  box-shadow: var(--k2-shadow-sm);
  align-items: center;
}
.f-email { width: 240px; }
.f-uid { width: 120px; }
.f-node { width: 180px; }

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 12px;
}
.stat-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 16px;
  padding: 16px 18px;
  box-shadow: var(--k2-shadow-sm);
  position: relative;
  overflow: hidden;
}
.stat-card::after {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
}
.c1::after { background: #4f46e5; }
.c2::after { background: #0ea5e9; }
.c3::after { background: #10b981; }
.c4::after { background: #f59e0b; }
.s-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.s-label {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.s-chip {
  font-size: 11px;
  font-weight: 700;
  color: #6366f1;
  background: #eef2ff;
  padding: 2px 8px;
  border-radius: 999px;
}
.s-val {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: #0f172a;
  word-break: break-word;
}
.s-val.sm { font-size: 16px; line-height: 1.35; }
.s-sub {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
  font-weight: 500;
}

.scale-note {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #64748b;
  background: #f8fafc;
  border: 1px solid var(--k2-border);
  border-radius: 12px;
  padding: 10px 14px;
  margin-bottom: 16px;
}
.scale-note b { color: #0f172a; }

.chart-panel {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 18px 20px;
  margin-bottom: 16px;
  box-shadow: var(--k2-shadow-sm);
}
.panel-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.panel-head h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
  color: #0f172a;
}
.panel-head p {
  margin: 4px 0 0;
  font-size: 12px;
  color: #94a3b8;
}
.warn-text { color: #d97706; font-weight: 600; }
.legend-pills { display: flex; gap: 8px; }
.pill {
  font-size: 11px;
  font-weight: 700;
  padding: 4px 10px;
  border-radius: 999px;
}
.pill.up { background: #eef2ff; color: #4f46e5; }
.pill.down { background: #ecfdf5; color: #059669; }

.chart-box.trend { height: 280px; }
.chart-box.mid { height: 260px; }

.split-grid {
  display: grid;
  grid-template-columns: 1fr 1.15fr;
  gap: 16px;
  margin-bottom: 0;
}
.split-grid .chart-panel { margin-bottom: 16px; }

.mini-table { margin-top: 8px; }
.mini-row {
  display: grid;
  grid-template-columns: 28px 1fr 80px 72px 48px;
  gap: 8px;
  align-items: center;
  padding: 8px 4px;
  border-radius: 10px;
  cursor: pointer;
  font-size: 12px;
}
.mini-row:hover { background: #f8fafc; }
.mini-row .rank {
  width: 22px;
  height: 22px;
  border-radius: 7px;
  background: #f1f5f9;
  display: grid;
  place-items: center;
  font-weight: 800;
  color: #64748b;
  font-size: 11px;
}
.mini-row .name {
  font-weight: 700;
  color: #334155;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bar-wrap {
  height: 6px;
  background: #f1f5f9;
  border-radius: 99px;
  overflow: hidden;
}
.bar {
  height: 100%;
  background: linear-gradient(90deg, #6366f1, #22d3ee);
  border-radius: 99px;
  min-width: 2px;
}
.bar.user {
  background: linear-gradient(90deg, #4f46e5, #a78bfa);
}
.mini-row .bytes { font-weight: 700; color: #0f172a; text-align: right; }
.mini-row .share { color: #94a3b8; text-align: right; font-weight: 600; }

.rank-table-wrap {
  max-height: 480px;
  overflow: auto;
}
.rank-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.rank-table th {
  text-align: left;
  font-size: 11px;
  color: #94a3b8;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  padding: 8px 6px;
  position: sticky;
  top: 0;
  background: #fff;
  z-index: 1;
  border-bottom: 1px solid #f1f5f9;
}
.rank-table th.r, .rank-table td.r { text-align: right; }
.rank-table td {
  padding: 10px 6px;
  border-bottom: 1px solid #f8fafc;
  vertical-align: middle;
}
.rank-table tbody tr {
  cursor: pointer;
  transition: background 0.12s;
}
.rank-table tbody tr:hover,
.rank-table tbody tr.active {
  background: #f8fafc;
}
.rank-table tbody tr.active {
  box-shadow: inset 3px 0 0 #4f46e5;
}
.badge {
  display: inline-grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 8px;
  font-size: 11px;
  font-weight: 800;
  background: #f1f5f9;
  color: #64748b;
}
.badge.b1 { background: #fef3c7; color: #b45309; }
.badge.b2 { background: #e2e8f0; color: #475569; }
.badge.b3 { background: #ffedd5; color: #c2410c; }

.u-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.u-cell.compact { gap: 8px; }
.avatar {
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: linear-gradient(135deg, #6366f1, #22d3ee);
  color: #fff;
  font-weight: 800;
  font-size: 13px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.avatar.sm {
  width: 28px;
  height: 28px;
  font-size: 12px;
  border-radius: 8px;
}
.email {
  font-weight: 700;
  color: #0f172a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}
.uid {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 500;
}
.share-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.share-cell .bar-wrap { flex: 1; }
.share-cell span {
  width: 42px;
  text-align: right;
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
}
.bytes-main { font-weight: 800; color: #0f172a; }
.bytes-sub { font-size: 10px; color: #94a3b8; margin-top: 2px; }

.empty {
  text-align: center;
  color: #94a3b8;
  padding: 48px 12px;
  font-size: 13px;
}

.log-meta {
  font-size: 12px;
  color: #64748b;
  display: flex;
  align-items: center;
  gap: 8px;
}
.log-meta b { color: #0f172a; }
.id-pill {
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 6px;
}
.node-chip {
  font-size: 12px;
  font-weight: 700;
  color: #334155;
  background: #f1f5f9;
  padding: 4px 10px;
  border-radius: 8px;
}
.up { color: #4f46e5; font-weight: 700; }
.down { color: #059669; font-weight: 700; }
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}
.log-table :deep(.el-table__row) {
  cursor: pointer;
}

@media (max-width: 1100px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
  .split-grid { grid-template-columns: 1fr; }
  .mini-row {
    grid-template-columns: 28px 1fr 60px 48px;
  }
  .mini-row .bar-wrap { display: none; }
}
@media (max-width: 560px) {
  .stat-grid { grid-template-columns: 1fr; }
  .f-email, .f-uid, .f-node { width: 100%; }
  .chart-box.trend, .chart-box.mid { height: 220px; }
}
</style>
