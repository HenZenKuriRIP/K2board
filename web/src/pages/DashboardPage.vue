<template>
  <div class="dashboard">
    <div class="k2-page-header">
      <div>
        <h3>仪表盘</h3>
        <p class="sub">实时掌握业务与面板主机运行态势</p>
      </div>
      <div class="header-actions">
        <span class="live-badge" :class="hostStatusClass">
          <i />
          {{ hostStatusLabel }}
        </span>
        <el-button text type="primary" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <div class="stat-grid">
      <div class="stat-card c-indigo">
        <div class="stat-top">
          <div class="stat-icon"><el-icon :size="22"><UserFilled /></el-icon></div>
          <span class="stat-chip">Users</span>
        </div>
        <div class="stat-value">{{ stats.total_users }}</div>
        <div class="stat-label">总用户</div>
        <div class="stat-meta">
          <span>活跃 <b>{{ stats.active_users }}</b></span>
          <span class="online">在线 <b>{{ stats.online_users }}</b></span>
        </div>
        <div class="stat-wave" />
      </div>

      <div class="stat-card c-emerald">
        <div class="stat-top">
          <div class="stat-icon"><el-icon :size="22"><Monitor /></el-icon></div>
          <span class="stat-chip">Nodes</span>
        </div>
        <div class="stat-value">{{ stats.total_nodes }}</div>
        <div class="stat-label">节点总数</div>
        <div class="stat-meta">
          <span>启用 <b>{{ stats.active_nodes }}</b></span>
        </div>
        <div class="stat-wave" />
      </div>

      <div class="stat-card c-amber">
        <div class="stat-top">
          <div class="stat-icon"><el-icon :size="22"><Top /></el-icon></div>
          <span class="stat-chip">Upload</span>
        </div>
        <div class="stat-value sm">{{ formatBytes(stats.total_upload) }}</div>
        <div class="stat-label">累计上传</div>
        <div class="stat-wave" />
      </div>

      <div class="stat-card c-violet">
        <div class="stat-top">
          <div class="stat-icon"><el-icon :size="22"><Bottom /></el-icon></div>
          <span class="stat-chip">Download</span>
        </div>
        <div class="stat-value sm">{{ formatBytes(stats.total_download) }}</div>
        <div class="stat-label">累计下载</div>
        <div class="stat-wave" />
      </div>
    </div>

    <!-- Host health -->
    <div class="panel health-panel">
      <div class="panel-head">
        <div>
          <h4>面板服务器健康</h4>
          <p>
            {{ host.hostname || '—' }}
            · {{ host.os }}/{{ host.arch }}
            · 运行 {{ formatUptime(host.uptime_sec) }}
          </p>
        </div>
        <div class="health-status" :class="host.status || 'healthy'">
          <span class="hs-dot" />
          <span>{{ host.message || '采集中…' }}</span>
        </div>
      </div>

      <div class="metric-grid">
        <div class="metric-card">
          <div class="mc-top">
            <span class="mc-label">CPU</span>
            <span class="mc-val" :class="levelClass(host.cpu_percent)">
              {{ host.cpu_percent < 0 ? '采样中' : host.cpu_percent.toFixed(1) + '%' }}
            </span>
          </div>
          <div class="bar-track">
            <div
              class="bar-fill cpu"
              :style="{ width: barWidth(host.cpu_percent) }"
            />
          </div>
          <div class="mc-sub">
            {{ host.num_cpu }} 核 · 负载
            {{ host.load1.toFixed(2) }} / {{ host.load5.toFixed(2) }} / {{ host.load15.toFixed(2) }}
          </div>
        </div>

        <div class="metric-card">
          <div class="mc-top">
            <span class="mc-label">内存</span>
            <span class="mc-val" :class="levelClass(host.mem_used_pct)">
              {{ host.mem_used_pct.toFixed(1) }}%
            </span>
          </div>
          <div class="bar-track">
            <div class="bar-fill mem" :style="{ width: clampPct(host.mem_used_pct) + '%' }" />
          </div>
          <div class="mc-sub">
            {{ formatBytes(host.mem_used_bytes) }} / {{ formatBytes(host.mem_total_bytes) }}
          </div>
        </div>

        <div class="metric-card">
          <div class="mc-top">
            <span class="mc-label">磁盘 /</span>
            <span class="mc-val" :class="levelClass(host.disk_used_pct)">
              {{ host.disk_total_bytes ? host.disk_used_pct.toFixed(1) + '%' : '—' }}
            </span>
          </div>
          <div class="bar-track">
            <div class="bar-fill disk" :style="{ width: (host.disk_total_bytes ? clampPct(host.disk_used_pct) : 0) + '%' }" />
          </div>
          <div class="mc-sub">
            <template v-if="host.disk_total_bytes">
              {{ formatBytes(host.disk_used_bytes) }} / {{ formatBytes(host.disk_total_bytes) }}
            </template>
            <template v-else>暂无磁盘数据</template>
          </div>
        </div>

        <div class="metric-card">
          <div class="mc-top">
            <span class="mc-label">进程内存</span>
            <span class="mc-val muted">{{ formatBytes(host.alloc_bytes) }}</span>
          </div>
          <div class="proc-stats">
            <div class="ps">
              <span>堆分配</span>
              <b>{{ formatBytes(host.alloc_bytes) }}</b>
            </div>
            <div class="ps">
              <span>运行时</span>
              <b>{{ formatBytes(host.sys_bytes) }}</b>
            </div>
            <div class="ps">
              <span>协程</span>
              <b>{{ host.goroutines }}</b>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="bottom-grid">
      <div class="panel traffic-panel">
        <div class="panel-head">
          <div>
            <h4>流量概览</h4>
            <p>用户累计消耗与上下行合计</p>
          </div>
          <el-icon class="panel-ico" :size="20"><TrendCharts /></el-icon>
        </div>
        <div class="traffic-hero">
          <div class="th-label">用户已用流量</div>
          <div class="th-value">{{ formatBytes(stats.total_traffic_used) }}</div>
        </div>
        <div class="traffic-rows">
          <div class="tr-row">
            <span class="dot up" />
            <span class="tr-label">总上传</span>
            <span class="tr-val up">{{ formatBytes(stats.total_upload) }}</span>
          </div>
          <div class="tr-row">
            <span class="dot down" />
            <span class="tr-label">总下载</span>
            <span class="tr-val down">{{ formatBytes(stats.total_download) }}</span>
          </div>
        </div>
      </div>

      <div class="panel sys-panel">
        <div class="panel-head">
          <div>
            <h4>系统信息</h4>
            <p>面板能力与协议支持</p>
          </div>
          <el-icon class="panel-ico" :size="20"><InfoFilled /></el-icon>
        </div>
        <div class="sys-list">
          <div class="sys-item">
            <span class="k">面板版本</span>
            <span class="pill primary">K2Board {{ stats.panel_version || 'v1.4+' }}</span>
          </div>
          <div class="sys-item">
            <span class="k">运行时</span>
            <span class="pill soft">{{ host.go_version || 'Go' }} · {{ host.os }}/{{ host.arch }}</span>
          </div>
          <div class="sys-item">
            <span class="k">数据库</span>
            <span class="pill success">PostgreSQL / MySQL</span>
          </div>
          <div class="sys-item protocols">
            <span class="k">节点协议</span>
            <div class="tags">
              <span class="tag t1">VLESS</span>
              <span class="tag t2">AnyTLS</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted, onUnmounted } from 'vue'
import { UserFilled, Monitor, Top, Bottom, TrendCharts, InfoFilled } from '@element-plus/icons-vue'
import { getDashboard, type DashboardStats, type HostHealth } from '@/api/admin'
import { formatBytes } from '@/utils/format'

const loading = ref(false)
const stats = reactive<DashboardStats>({
  total_users: 0, active_users: 0, online_users: 0,
  total_nodes: 0, active_nodes: 0,
  total_upload: 0, total_download: 0, total_traffic_used: 0,
  panel_version: '',
})

const host = reactive<HostHealth>({
  hostname: '',
  os: '',
  arch: '',
  go_version: '',
  num_cpu: 0,
  goroutines: 0,
  uptime_sec: 0,
  alloc_bytes: 0,
  sys_bytes: 0,
  load1: 0,
  load5: 0,
  load15: 0,
  mem_total_bytes: 0,
  mem_used_bytes: 0,
  mem_used_pct: 0,
  disk_total_bytes: 0,
  disk_used_bytes: 0,
  disk_used_pct: 0,
  cpu_percent: -1,
  status: 'healthy',
  message: '',
})

let timer: ReturnType<typeof setInterval> | null = null

const hostStatusClass = computed(() => {
  const s = host.status
  if (s === 'critical') return 'crit'
  if (s === 'warn') return 'warn'
  return ''
})

const hostStatusLabel = computed(() => {
  if (host.status === 'critical') return '资源告警'
  if (host.status === 'warn') return '需关注'
  return '运行正常'
})

function clampPct(n: number) {
  if (!Number.isFinite(n) || n < 0) return 0
  return Math.min(100, n)
}

function barWidth(cpu: number) {
  if (cpu < 0) return '8%'
  return clampPct(cpu) + '%'
}

function levelClass(pct: number) {
  if (pct < 0) return 'muted'
  if (pct >= 90) return 'crit'
  if (pct >= 75) return 'warn'
  return 'ok'
}

function formatUptime(sec: number) {
  if (!sec || sec < 0) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分钟`
}

async function load() {
  loading.value = true
  try {
    const res = await getDashboard()
    const d = res.data
    Object.assign(stats, d)
    if (d.host) Object.assign(host, d.host)
  } catch { /* ignore */ }
  finally {
    loading.value = false
  }
}

onMounted(async () => {
  await load()
  // Second tick so CPU % can be computed from /proc/stat delta
  setTimeout(() => load(), 1200)
  timer = setInterval(load, 15000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.live-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  color: #059669;
  background: #ecfdf5;
  border: 1px solid #a7f3d0;
  padding: 8px 14px;
  border-radius: 999px;
}
.live-badge i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.5);
  animation: live 1.8s infinite;
}
.live-badge.warn {
  color: #d97706;
  background: #fffbeb;
  border-color: #fde68a;
}
.live-badge.warn i { background: #f59e0b; }
.live-badge.crit {
  color: #dc2626;
  background: #fef2f2;
  border-color: #fecaca;
}
.live-badge.crit i { background: #ef4444; }
@keyframes live {
  0% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.5); }
  70% { box-shadow: 0 0 0 8px rgba(16, 185, 129, 0); }
  100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}
.stat-card {
  position: relative;
  overflow: hidden;
  border-radius: 18px;
  padding: 20px 20px 16px;
  background: #fff;
  border: 1px solid var(--k2-border);
  box-shadow: var(--k2-shadow-sm);
  transition: transform 0.2s ease, box-shadow 0.25s ease;
  min-height: 168px;
}
.stat-card:hover {
  transform: translateY(-3px);
  box-shadow: var(--k2-shadow);
}
.stat-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 14px;
  display: grid;
  place-items: center;
  color: #fff;
}
.c-indigo .stat-icon { background: linear-gradient(135deg, #6366f1, #4f46e5); box-shadow: 0 8px 20px rgba(79, 70, 229, 0.3); }
.c-emerald .stat-icon { background: linear-gradient(135deg, #34d399, #059669); box-shadow: 0 8px 20px rgba(16, 185, 129, 0.28); }
.c-amber .stat-icon { background: linear-gradient(135deg, #fbbf24, #d97706); box-shadow: 0 8px 20px rgba(245, 158, 11, 0.28); }
.c-violet .stat-icon { background: linear-gradient(135deg, #a78bfa, #7c3aed); box-shadow: 0 8px 20px rgba(124, 58, 237, 0.28); }
.stat-chip {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--k2-text-muted);
  background: #f8fafc;
  border-radius: 999px;
  padding: 4px 10px;
}
.stat-value {
  font-size: 34px;
  font-weight: 800;
  letter-spacing: -0.04em;
  color: var(--k2-text);
  line-height: 1.1;
}
.stat-value.sm { font-size: 26px; }
.stat-label {
  margin-top: 4px;
  font-size: 13px;
  color: var(--k2-text-secondary);
  font-weight: 500;
}
.stat-meta {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: var(--k2-text-muted);
}
.stat-meta b { color: var(--k2-text); font-weight: 700; }
.stat-meta .online b { color: #10b981; }
.stat-wave {
  position: absolute;
  right: -20px;
  bottom: -30px;
  width: 120px;
  height: 120px;
  border-radius: 50%;
  opacity: 0.08;
  pointer-events: none;
}
.c-indigo .stat-wave { background: #4f46e5; }
.c-emerald .stat-wave { background: #10b981; }
.c-amber .stat-wave { background: #f59e0b; }
.c-violet .stat-wave { background: #7c3aed; }

/* Health panel */
.health-panel {
  margin-bottom: 16px;
  background:
    radial-gradient(900px 200px at 100% 0%, rgba(99, 102, 241, 0.08), transparent 55%),
    #fff;
}
.health-status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 700;
  padding: 8px 14px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #059669;
  border: 1px solid #a7f3d0;
}
.health-status.warn {
  background: #fffbeb;
  color: #d97706;
  border-color: #fde68a;
}
.health-status.critical {
  background: #fef2f2;
  color: #dc2626;
  border-color: #fecaca;
}
.hs-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}
.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
}
.metric-card {
  background: #f8fafc;
  border: 1px solid #eef2f7;
  border-radius: 14px;
  padding: 14px 16px;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.metric-card:hover {
  border-color: rgba(99, 102, 241, 0.25);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.04);
}
.mc-top {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 10px;
  gap: 8px;
}
.mc-label {
  font-size: 12px;
  font-weight: 700;
  color: var(--k2-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.mc-val {
  font-size: 20px;
  font-weight: 800;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums;
}
.mc-val.ok { color: #4f46e5; }
.mc-val.warn { color: #d97706; }
.mc-val.crit { color: #dc2626; }
.mc-val.muted { color: var(--k2-text); font-size: 18px; }
.bar-track {
  height: 8px;
  border-radius: 999px;
  background: #e2e8f0;
  overflow: hidden;
  margin-bottom: 10px;
}
.bar-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.45s ease;
}
.bar-fill.cpu { background: linear-gradient(90deg, #6366f1, #22d3ee); }
.bar-fill.mem { background: linear-gradient(90deg, #8b5cf6, #6366f1); }
.bar-fill.disk { background: linear-gradient(90deg, #10b981, #34d399); }
.mc-sub {
  font-size: 11px;
  color: var(--k2-text-muted);
  font-variant-numeric: tabular-nums;
  line-height: 1.4;
}
.proc-stats {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 4px;
}
.ps {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--k2-text-muted);
}
.ps b {
  color: var(--k2-text);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.bottom-grid {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 16px;
}
.panel {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 22px 24px;
  box-shadow: var(--k2-shadow-sm);
}
.panel-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
  gap: 12px;
}
.panel-head h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: var(--k2-text);
}
.panel-head p {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--k2-text-muted);
}
.panel-ico {
  color: var(--k2-primary);
  background: var(--k2-primary-soft);
  padding: 10px;
  border-radius: 12px;
  box-sizing: content-box;
  flex-shrink: 0;
}
.traffic-hero {
  background: var(--k2-gradient-soft);
  border: 1px solid rgba(79, 70, 229, 0.1);
  border-radius: 14px;
  padding: 18px 20px;
  margin-bottom: 16px;
}
.th-label {
  font-size: 12px;
  color: var(--k2-text-secondary);
  font-weight: 600;
}
.th-value {
  margin-top: 6px;
  font-size: 28px;
  font-weight: 800;
  letter-spacing: -0.03em;
  background: var(--k2-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.traffic-rows {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.tr-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  background: #f8fafc;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.dot.up { background: #4f46e5; box-shadow: 0 0 0 4px rgba(79, 70, 229, 0.12); }
.dot.down { background: #10b981; box-shadow: 0 0 0 4px rgba(16, 185, 129, 0.12); }
.tr-label { flex: 1; font-size: 13px; color: var(--k2-text-secondary); }
.tr-val { font-weight: 700; font-size: 14px; }
.tr-val.up { color: #4f46e5; }
.tr-val.down { color: #059669; }

.sys-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.sys-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.sys-item .k {
  font-size: 13px;
  color: var(--k2-text-secondary);
  font-weight: 500;
}
.pill {
  font-size: 12px;
  font-weight: 700;
  padding: 5px 12px;
  border-radius: 999px;
}
.pill.primary { background: #eef2ff; color: #4f46e5; }
.pill.success { background: #ecfdf5; color: #059669; }
.pill.soft { background: #f1f5f9; color: #475569; }
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
}
.tag {
  font-size: 11px;
  font-weight: 700;
  padding: 4px 10px;
  border-radius: 8px;
}
.t1 { background: #eef2ff; color: #4f46e5; }
.t2 { background: #ecfeff; color: #0891b2; }

@media (max-width: 1100px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
  .metric-grid { grid-template-columns: repeat(2, 1fr); }
  .bottom-grid { grid-template-columns: 1fr; }
}
@media (max-width: 560px) {
  .stat-grid { grid-template-columns: 1fr; }
  .metric-grid { grid-template-columns: 1fr; }
  .stat-value { font-size: 28px; }
}
</style>
