<template>
  <div class="queue-page">
    <div class="k2-page-header">
      <div>
        <h3>后台调度监控</h3>
        <p class="sub">
          进程内周期任务 · 非消息队列 ·
          <span class="hint">指标进程重启后清零 · 当前实例 {{ (data.queue.store_type || 'memory').toUpperCase() }}</span>
        </p>
      </div>
      <div class="header-actions">
        <span class="auto-badge" :class="{ on: autoRefresh }">
          {{ autoRefresh ? `自动刷新 ${countdown}s` : '已暂停自动刷新' }}
        </span>
        <el-button size="large" @click="autoRefresh = !autoRefresh">
          {{ autoRefresh ? '暂停' : '开启自动' }}
        </el-button>
        <el-button size="large" type="primary" :loading="loading" @click="fetchData(true)">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <!-- Top KPIs -->
    <div class="stat-grid">
      <div class="stat-card" :class="storeTypeClass">
        <div class="icon-wrap"><el-icon :size="20"><Coin /></el-icon></div>
        <div class="body">
          <div class="val">{{ (data.queue.store_type || 'memory').toUpperCase() }}</div>
          <div class="label">流量缓冲引擎</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="icon-wrap i2"><el-icon :size="20"><Clock /></el-icon></div>
        <div class="body">
          <div class="val">{{ data.queue.buffer_size ?? 0 }}</div>
          <div class="label">待刷盘条目 (user×node)</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="icon-wrap i3"><el-icon :size="20"><Finished /></el-icon></div>
        <div class="body">
          <div class="val">{{ data.queue.total_entries || 0 }}</div>
          <div class="label">累计成功落库</div>
          <div class="delta" v-if="delta.entries !== null" :class="deltaClass(delta.entries)">
            {{ formatDelta(delta.entries) }} 较上次
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="icon-wrap i4"><el-icon :size="20"><Timer /></el-icon></div>
        <div class="body">
          <div class="val">{{ formatUptime(data.scheduler.uptime_seconds || 0) }}</div>
          <div class="label">调度器运行时长</div>
        </div>
      </div>
    </div>

    <!-- Four pipelines -->
    <div class="pipe-grid">
      <div
        v-for="p in pipelineCards"
        :key="p.id"
        class="pipe-card"
        :class="[p.tone, { stale: p.isStale, hot: p.hasRecentWork }]"
      >
        <div class="pipe-head">
          <div class="pipe-title">
            <span class="pipe-dot" />
            {{ p.name }}
          </div>
          <el-tag size="small" effect="plain" round>{{ p.intervalLabel }}</el-tag>
        </div>

        <div class="pipe-writes">
          <div class="w-label">写入</div>
          <ul>
            <li v-for="(w, i) in p.writes" :key="i">{{ w }}</li>
          </ul>
          <div v-if="p.sideEffects?.length" class="side">
            顺带：{{ p.sideEffects.join(' · ') }}
          </div>
        </div>

        <div class="pipe-metrics">
          <div class="m">
            <span>上次执行</span>
            <b>{{ formatTime(p.lastAt) }}</b>
          </div>
          <div class="m">
            <span>距下次约</span>
            <b :class="{ warn: p.nextSec !== null && p.nextSec < 15 }">{{ p.nextLabel }}</b>
          </div>
          <div class="m result">
            <span>上次结果</span>
            <b>{{ p.lastResult }}</b>
          </div>
          <div class="m">
            <span>累计</span>
            <b>{{ p.totals }}</b>
          </div>
        </div>
      </div>
    </div>

    <div class="mid-grid">
      <!-- Activity feed -->
      <div class="panel feed-panel">
        <div class="panel-title">
          <el-icon><List /></el-icon>
          最近执行流水
          <span class="muted">本进程内存 · 最多 50 条 · 重启清空</span>
        </div>
        <div v-if="recentEvents.length" class="feed">
          <div
            v-for="(ev, idx) in recentEvents"
            :key="idx"
            class="feed-item"
            :class="[ev.job, { empty: ev.empty, fail: (ev.failed || 0) > 0 }]"
          >
            <div class="feed-left">
              <span class="job-tag">{{ jobLabel(ev.job) }}</span>
              <div class="feed-msg">{{ ev.message }}</div>
            </div>
            <div class="feed-time">{{ formatTime(ev.at) }}</div>
          </div>
        </div>
        <div v-else class="empty">暂无执行记录，等待调度器下一次 tick…</div>
      </div>

      <!-- Context -->
      <div class="panel side-panel">
        <div class="panel-title">
          <el-icon><DataAnalysis /></el-icon>
          关联快照
        </div>
        <p class="note">{{ data.stats.today_note || '今日流量来自日统计聚合表，不含缓冲中未刷盘部分。' }}</p>
        <div class="today-grid">
          <div class="t-item">
            <span>今日上传</span>
            <b>{{ formatBytes(data.stats.today_upload || 0) }}</b>
          </div>
          <div class="t-item">
            <span>今日下载</span>
            <b>{{ formatBytes(data.stats.today_download || 0) }}</b>
          </div>
          <div class="t-item">
            <span>启用用户</span>
            <b>{{ data.stats.enabled_users || 0 }} <small>/ {{ data.stats.total_users || 0 }}</small></b>
          </div>
          <div class="t-item">
            <span>启用节点</span>
            <b>{{ data.stats.enabled_nodes || 0 }} <small>/ {{ data.stats.total_nodes || 0 }}</small></b>
          </div>
        </div>

        <div class="panel-title mt">
          <el-icon><Timer /></el-icon>
          调度配置
        </div>
        <div class="kv-list">
          <div class="kv"><span>刷盘间隔</span><b>{{ cfg.flush_interval }} 秒</b></div>
          <div class="kv"><span>日统计间隔</span><b>{{ cfg.stats_interval }} 秒</b></div>
          <div class="kv"><span>账号维护间隔</span><b>{{ cfg.auto_disable_interval }} 秒</b></div>
          <div class="kv"><span>流量重置间隔</span><b>{{ Math.round((cfg.reset_interval || 3600) / 60) }} 分钟</b></div>
          <div class="kv"><span>启动时间</span><b>{{ formatTime(data.scheduler.started_at) }}</b></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, onMounted, onUnmounted, watch } from 'vue'
import { Coin, Refresh, Finished, Clock, Timer, List, DataAnalysis } from '@element-plus/icons-vue'
import request from '@/api/request'
import { formatBytes } from '@/utils/format'

const REFRESH_SEC = 15

const data = reactive({
  queue: {} as Record<string, any>,
  scheduler: {} as Record<string, any>,
  scheduler_config: {} as Record<string, any>,
  pipelines: {} as Record<string, any>,
  stats: {} as Record<string, any>,
  recent: [] as any[],
})

const loading = ref(false)
const autoRefresh = ref(true)
const countdown = ref(REFRESH_SEC)
const nowTick = ref(Date.now())
let pollTimer: ReturnType<typeof setInterval> | null = null
let clockTimer: ReturnType<typeof setInterval> | null = null

const prevSnapshot = ref<{ entries: number; flushes: number } | null>(null)
const delta = reactive<{ entries: number | null; flushes: number | null }>({
  entries: null,
  flushes: null,
})

const cfg = computed(() => ({
  flush_interval: data.scheduler_config?.flush_interval || 60,
  stats_interval: data.scheduler_config?.stats_interval || 300,
  auto_disable_interval: data.scheduler_config?.auto_disable_interval || 60,
  reset_interval: data.scheduler_config?.reset_interval || 3600,
}))

const storeTypeClass = computed(() => (data.queue.store_type === 'redis' ? 'redis' : 'memory'))

const recentEvents = computed(() => {
  if (Array.isArray(data.recent) && data.recent.length) return data.recent
  return []
})

function isValidTime(t: any): boolean {
  if (!t) return false
  if (typeof t === 'string' && t.startsWith('0001-')) return false
  return true
}

function formatUptime(s: number): string {
  if (!s || s <= 0) return '刚启动'
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return `${d > 0 ? d + '天 ' : ''}${h}时 ${m}分`
}

function formatTime(t: any): string {
  if (!isValidTime(t)) return '尚未执行'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

function secondsUntilNext(lastAt: any, intervalSec: number): number | null {
  if (!intervalSec || intervalSec <= 0) return null
  // no last yet: still "about interval" from process start if known
  const base = isValidTime(lastAt)
    ? new Date(lastAt).getTime()
    : (isValidTime(data.scheduler.started_at) ? new Date(data.scheduler.started_at).getTime() : null)
  if (base == null) return null
  const elapsed = (nowTick.value - base) / 1000
  const rem = intervalSec - (elapsed % intervalSec)
  return Math.max(0, Math.ceil(rem))
}

function formatNext(sec: number | null): string {
  if (sec === null) return '—'
  if (sec <= 0) return '即将'
  if (sec < 60) return `${sec} 秒`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return s ? `${m} 分 ${s} 秒` : `${m} 分`
}

function formatDelta(n: number): string {
  if (n > 0) return `+${n}`
  if (n < 0) return `${n}`
  return '0'
}

function deltaClass(n: number) {
  if (n > 0) return 'up'
  if (n < 0) return 'down'
  return ''
}

function jobLabel(job: string): string {
  const map: Record<string, string> = {
    flush: '刷盘',
    aggregate: '聚合',
    // job id stays "disable" for API/metrics compatibility; meaning is account maintenance
    disable: '账号维护',
    reset: '重置',
    purge: '清理',
  }
  return map[job] || job
}

function isStale(lastAt: any, intervalSec: number): boolean {
  if (!isValidTime(lastAt) || !intervalSec) return false
  const age = (nowTick.value - new Date(lastAt).getTime()) / 1000
  return age > intervalSec * 3
}

const pipelineCards = computed(() => {
  const p = data.pipelines || {}
  const flush = p.flush || {}
  const agg = p.aggregate || {}
  const dis = p.disable || {}
  const reset = p.reset || {}

  const flushInt = Number(flush.interval || cfg.value.flush_interval)
  const aggInt = Number(agg.interval || cfg.value.stats_interval)
  const disInt = Number(dis.interval || cfg.value.auto_disable_interval)
  const resetInt = Number(reset.interval || cfg.value.reset_interval)

  const flushNext = secondsUntilNext(flush.last_at, flushInt)
  const aggNext = secondsUntilNext(agg.last_at, aggInt)
  const disNext = secondsUntilNext(dis.last_at, disInt)
  const resetNext = secondsUntilNext(reset.last_at, resetInt)

  const lastFlushOk = flush.last_success ?? data.queue.last_flush_success ?? 0
  const lastFlushFail = flush.last_failed ?? data.queue.last_flush_failed ?? 0

  return [
    {
      id: 'flush',
      name: flush.name || '流量缓冲刷盘',
      tone: 'tone-flush',
      writes: flush.writes || ['traffic_logs (insert)', 'users.traffic_used (+=)'],
      sideEffects: [] as string[],
      intervalLabel: `每 ${flushInt}s`,
      lastAt: flush.last_at || data.queue.last_flush_at,
      nextSec: flushNext,
      nextLabel: formatNext(flushNext),
      lastResult:
        !isValidTime(flush.last_at || data.queue.last_flush_at)
          ? '—'
          : lastFlushFail > 0
            ? `成功 ${lastFlushOk} · 失败 ${lastFlushFail}`
            : lastFlushOk > 0
              ? `成功写入 ${lastFlushOk} 条`
              : '空缓冲（无写入）',
      totals: `运行 ${flush.total_runs ?? data.queue.total_flushes ?? 0} 次 · 成功 ${flush.total_ok ?? data.queue.total_entries ?? 0} 条 · 失败 ${flush.total_fail ?? data.queue.total_flush_failed ?? 0}`,
      isStale: isStale(flush.last_at || data.queue.last_flush_at, flushInt),
      hasRecentWork: lastFlushOk > 0 || lastFlushFail > 0,
    },
    {
      id: 'aggregate',
      name: agg.name || '日统计聚合',
      tone: 'tone-agg',
      writes: agg.writes || ['stat_servers (upsert)', 'stat_users (upsert)'],
      sideEffects: [] as string[],
      intervalLabel: `每 ${aggInt}s`,
      lastAt: agg.last_at || data.scheduler.last_aggregation_at,
      nextSec: aggNext,
      nextLabel: formatNext(aggNext),
      lastResult: !isValidTime(agg.last_at || data.scheduler.last_aggregation_at)
        ? '—'
        : `节点 ${agg.last_nodes ?? data.scheduler.last_aggregation_nodes ?? 0} · 用户 ${agg.last_users ?? data.scheduler.last_aggregation_users ?? 0}`,
      totals: `运行 ${agg.total_runs ?? data.scheduler.total_aggregations ?? 0} 次`,
      isStale: isStale(agg.last_at || data.scheduler.last_aggregation_at, aggInt),
      hasRecentWork: (agg.last_nodes || 0) + (agg.last_users || 0) > 0,
    },
    {
      id: 'disable',
      name: dis.name || '账号维护（过期不改 enable）',
      tone: 'tone-dis',
      writes: dis.writes || ['users.enable = true (one-time legacy repair only)', 'config_version++ (if repair)'],
      sideEffects: dis.side_effects || ['RefreshConfigVersion', 'PurgeStaleOnline'],
      intervalLabel: `每 ${disInt}s`,
      lastAt: dis.last_at || data.scheduler.last_disable_at,
      nextSec: disNext,
      nextLabel: formatNext(disNext),
      // last_affected = one-time legacy re-enable count (not "disabled users")
      lastResult: !isValidTime(dis.last_at || data.scheduler.last_disable_at)
        ? '—'
        : (dis.last_affected ?? data.scheduler.last_disabled ?? 0) > 0
          ? `历史修复恢复 ${dis.last_affected ?? data.scheduler.last_disabled} 人` +
            ((dis.last_purged ?? data.scheduler.last_purged ?? 0) > 0
              ? ` · 清理在线 ${dis.last_purged ?? data.scheduler.last_purged}`
              : '')
          : (dis.last_purged ?? data.scheduler.last_purged ?? 0) > 0
            ? `空跑（过期不改 enable）· 清理在线 ${dis.last_purged ?? data.scheduler.last_purged}`
            : '空跑（过期不改 enable · 仅维护/清理在线）',
      totals: `运行 ${dis.total_runs ?? data.scheduler.total_disable_runs ?? 0} 次 · 累计历史修复 ${dis.total_affected ?? data.scheduler.total_disabled ?? 0} 人 · 清理在线 ${dis.total_purged ?? data.scheduler.total_purged ?? 0}`,
      isStale: isStale(dis.last_at || data.scheduler.last_disable_at, disInt),
      hasRecentWork:
        (dis.last_affected ?? data.scheduler.last_disabled ?? 0) > 0 ||
        (dis.last_purged ?? data.scheduler.last_purged ?? 0) > 0,
    },
    {
      id: 'reset',
      name: reset.name || '月流量自动重置',
      tone: 'tone-reset',
      writes: reset.writes || ['users.traffic_used = 0'],
      sideEffects: [] as string[],
      intervalLabel: `每 ${Math.round(resetInt / 60)} 分`,
      lastAt: reset.last_at || data.scheduler.last_reset_at,
      nextSec: resetNext,
      nextLabel: formatNext(resetNext),
      lastResult: !isValidTime(reset.last_at || data.scheduler.last_reset_at)
        ? '—'
        : (reset.last_affected ?? data.scheduler.last_reset ?? 0) > 0
          ? `重置 ${reset.last_affected ?? data.scheduler.last_reset} 人`
          : '无命中用户',
      totals: `运行 ${reset.total_runs ?? data.scheduler.total_reset_runs ?? 0} 次 · 累计重置 ${reset.total_affected ?? data.scheduler.total_reset ?? 0} 人`,
      isStale: isStale(reset.last_at || data.scheduler.last_reset_at, resetInt),
      hasRecentWork: (reset.last_affected ?? data.scheduler.last_reset ?? 0) > 0,
    },
  ]
})

async function fetchData(manual = false) {
  if (manual) loading.value = true
  try {
    const r = await request.get('/admin/queue/stats')
    const body = r.data || {}

    const nextEntries = body.queue?.total_entries ?? 0
    const nextFlushes = body.queue?.total_flushes ?? 0
    if (prevSnapshot.value) {
      delta.entries = nextEntries - prevSnapshot.value.entries
      delta.flushes = nextFlushes - prevSnapshot.value.flushes
    }
    prevSnapshot.value = { entries: nextEntries, flushes: nextFlushes }

    data.queue = body.queue || {}
    data.scheduler = body.scheduler || {}
    data.scheduler_config = body.scheduler_config || {}
    data.pipelines = body.pipelines || {}
    data.stats = body.stats || {}
    data.recent = body.recent || []
    countdown.value = REFRESH_SEC
  } catch (e) {
    console.error('Failed to fetch queue stats', e)
  } finally {
    loading.value = false
  }
}

function startTimers() {
  stopTimers()
  clockTimer = setInterval(() => {
    nowTick.value = Date.now()
  }, 1000)
  pollTimer = setInterval(() => {
    if (!autoRefresh.value) return
    countdown.value -= 1
    if (countdown.value <= 0) {
      fetchData(false)
    }
  }, 1000)
}

function stopTimers() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  if (clockTimer) {
    clearInterval(clockTimer)
    clockTimer = null
  }
}

watch(autoRefresh, (on) => {
  if (on) countdown.value = REFRESH_SEC
})

onMounted(() => {
  fetchData(true)
  startTimers()
})

onUnmounted(stopTimers)
</script>

<style scoped>
.queue-page {
  --flush: #4f46e5;
  --agg: #059669;
  --dis: #d97706;
  --reset: #7c3aed;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.auto-badge {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  padding: 6px 12px;
  border-radius: 999px;
  background: #f1f5f9;
}
.auto-badge.on {
  color: #059669;
  background: #ecfdf5;
}
.hint {
  color: #94a3b8;
  font-weight: 500;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 18px;
}
.stat-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 16px;
  padding: 16px 18px;
  display: flex;
  align-items: center;
  gap: 14px;
  box-shadow: var(--k2-shadow-sm);
}
.stat-card.memory { box-shadow: inset 4px 0 0 #10b981, var(--k2-shadow-sm); }
.stat-card.redis { box-shadow: inset 4px 0 0 #7c3aed, var(--k2-shadow-sm); }
.icon-wrap {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  color: #fff;
  background: linear-gradient(135deg, #64748b, #475569);
  flex-shrink: 0;
}
.icon-wrap.i2 { background: linear-gradient(135deg, #fbbf24, #d97706); }
.icon-wrap.i3 { background: linear-gradient(135deg, #34d399, #059669); }
.icon-wrap.i4 { background: linear-gradient(135deg, #6366f1, #4f46e5); }
.val {
  font-size: 20px;
  font-weight: 800;
  color: #0f172a;
  letter-spacing: -0.02em;
  word-break: break-word;
}
.label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 600;
  margin-top: 2px;
}
.delta {
  font-size: 11px;
  font-weight: 700;
  color: #94a3b8;
  margin-top: 2px;
}
.delta.up { color: #059669; }
.delta.down { color: #dc2626; }

.pipe-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 18px;
}
.pipe-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 16px;
  box-shadow: var(--k2-shadow-sm);
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: relative;
  overflow: hidden;
}
.pipe-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: #94a3b8;
}
.pipe-card.tone-flush::before { background: var(--flush); }
.pipe-card.tone-agg::before { background: var(--agg); }
.pipe-card.tone-dis::before { background: var(--dis); }
.pipe-card.tone-reset::before { background: var(--reset); }
.pipe-card.hot {
  box-shadow: 0 0 0 1px rgba(16, 185, 129, 0.25), var(--k2-shadow-sm);
}
.pipe-card.stale {
  opacity: 0.85;
  outline: 1px dashed #f59e0b;
}
.pipe-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.pipe-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 800;
  color: #0f172a;
}
.pipe-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}
.tone-flush .pipe-dot { background: var(--flush); box-shadow: 0 0 0 3px rgba(79, 70, 229, 0.15); }
.tone-agg .pipe-dot { background: var(--agg); box-shadow: 0 0 0 3px rgba(5, 150, 105, 0.15); }
.tone-dis .pipe-dot { background: var(--dis); box-shadow: 0 0 0 3px rgba(217, 119, 6, 0.15); }
.tone-reset .pipe-dot { background: var(--reset); box-shadow: 0 0 0 3px rgba(124, 58, 237, 0.15); }

.pipe-writes {
  background: #f8fafc;
  border-radius: 12px;
  padding: 10px 12px;
  font-size: 12px;
}
.w-label {
  font-size: 10px;
  font-weight: 700;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: 6px;
}
.pipe-writes ul {
  margin: 0;
  padding-left: 16px;
  color: #334155;
  font-weight: 600;
  line-height: 1.55;
}
.side {
  margin-top: 6px;
  color: #64748b;
  font-size: 11px;
  font-weight: 500;
}

.pipe-metrics {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pipe-metrics .m {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
}
.pipe-metrics .m span { color: #94a3b8; font-weight: 500; flex-shrink: 0; }
.pipe-metrics .m b {
  color: #0f172a;
  font-weight: 700;
  text-align: right;
  word-break: break-word;
}
.pipe-metrics .m b.warn { color: #d97706; }
.pipe-metrics .m.result b { color: #1e293b; font-size: 12.5px; }

.mid-grid {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 16px;
}
.panel {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 20px;
  box-shadow: var(--k2-shadow-sm);
}
.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 800;
  color: #0f172a;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.panel-title.mt { margin-top: 20px; }
.muted {
  font-size: 11px;
  font-weight: 500;
  color: #94a3b8;
  margin-left: 4px;
}

.feed {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 420px;
  overflow-y: auto;
}
.feed-item {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: #f8fafc;
  border-left: 3px solid #94a3b8;
}
.feed-item.flush { border-left-color: var(--flush); }
.feed-item.aggregate { border-left-color: var(--agg); }
.feed-item.disable { border-left-color: var(--dis); }
.feed-item.reset { border-left-color: var(--reset); }
.feed-item.purge { border-left-color: #0ea5e9; }
.feed-item.empty { opacity: 0.72; }
.feed-item.fail { background: #fff7ed; }
.feed-left { min-width: 0; }
.job-tag {
  display: inline-block;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  color: #64748b;
  background: #e2e8f0;
  border-radius: 6px;
  padding: 2px 6px;
  margin-bottom: 4px;
}
.feed-msg {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  line-height: 1.4;
}
.feed-time {
  font-size: 11px;
  color: #94a3b8;
  white-space: nowrap;
  padding-top: 2px;
}

.empty {
  text-align: center;
  color: #94a3b8;
  padding: 36px 12px;
  font-size: 13px;
}

.note {
  font-size: 12px;
  color: #64748b;
  line-height: 1.5;
  margin: 0 0 12px;
  padding: 10px 12px;
  background: #f8fafc;
  border-radius: 10px;
}

.today-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.t-item {
  background: #f8fafc;
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.t-item span {
  font-size: 11px;
  font-weight: 600;
  color: #94a3b8;
}
.t-item b {
  font-size: 15px;
  font-weight: 800;
  color: #0f172a;
}
.t-item small {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
}

.kv-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.kv {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: #f8fafc;
  border-radius: 10px;
  font-size: 13px;
}
.kv span { color: #64748b; font-weight: 500; }
.kv b { color: #0f172a; font-weight: 700; }

@media (max-width: 1200px) {
  .pipe-grid { grid-template-columns: repeat(2, 1fr); }
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
  .mid-grid { grid-template-columns: 1fr; }
}
@media (max-width: 560px) {
  .pipe-grid, .stat-grid, .today-grid { grid-template-columns: 1fr; }
}
</style>
