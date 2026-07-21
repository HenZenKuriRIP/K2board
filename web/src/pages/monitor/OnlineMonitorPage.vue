<template>
  <div class="monitor">
    <!-- HUD header -->
    <div class="hud-top">
      <div class="hud-title">
        <span class="live-dot" :class="{ on: !paused }" />
        <div>
          <h3>用户状态监控</h3>
          <p>实时在线 · IP 地理映射 · 节点撮合视图</p>
        </div>
      </div>
      <div class="hud-actions">
        <el-button size="small" :type="paused ? 'primary' : 'default'" @click="togglePause">
          {{ paused ? '继续轮询' : '暂停' }}
        </el-button>
        <el-button size="small" :loading="loading" @click="refresh(true)">刷新</el-button>
        <span class="tick">{{ lastTick }}</span>
      </div>
    </div>

    <!-- KPI strip -->
    <div class="kpi-row">
      <div class="kpi k1"><span>在线用户</span><b>{{ snap?.totalOnlineUsers ?? 0 }}</b></div>
      <div class="kpi k2"><span>在线 IP</span><b>{{ snap?.totalOnlineIPs ?? 0 }}</b></div>
      <div class="kpi k3"><span>国家/地区</span><b>{{ countryCount }}</b></div>
      <div class="kpi k4"><span>节点在线合计</span><b>{{ snap?.nodeOnlineSum ?? 0 }}</b></div>
      <div class="kpi k5"><span>活跃节点</span><b>{{ activeNodes }}</b></div>
    </div>

    <div class="stage">
      <!-- Globe -->
      <div ref="globeEl" class="globe-host" />
      <div class="globe-vignette" />
      <div class="globe-hint">拖拽旋转 · 滚轮缩放 · 点击光点查看</div>

      <!-- Left panel: live feed -->
      <aside class="panel left glass">
        <div class="panel-h">
          <span>实时会话</span>
          <em>{{ endpointsView.length }}</em>
        </div>
        <div class="feed">
          <div
            v-for="(e, i) in endpointsView"
            :key="e.ip + e.userId + i"
            class="feed-item"
            :class="{ active: selected?.ip === e.ip && selected?.userId === e.userId, lan: e.geo?.private }"
            @click="focusEndpoint(e)"
          >
            <div class="fi-top">
              <span class="email">{{ e.email }}</span>
              <span class="flag">{{ e.geo?.countryCode || '…' }}</span>
            </div>
            <div class="fi-ip">{{ e.ip }}</div>
            <div class="fi-loc">
              {{ locationText(e) }}
            </div>
          </div>
          <div v-if="!endpointsView.length && !loading" class="empty">
            暂无在线 IP<br />
            <small>等待节点上报 /UniProxy/alive</small>
          </div>
        </div>
      </aside>

      <!-- Right panel: detail + nodes + countries -->
      <aside class="panel right glass">
        <div class="panel-h"><span>选中详情</span></div>
        <div v-if="selected" class="detail">
          <div class="d-row"><label>用户</label><b>{{ selected.email }}</b></div>
          <div class="d-row"><label>UID</label><b>#{{ selected.userId }}</b></div>
          <div class="d-row"><label>IP</label><code>{{ selected.ip }}</code></div>
          <div class="d-row"><label>位置</label><b>{{ locationText(selected) }}</b></div>
          <div class="d-row"><label>ASN/ISP</label><b>{{ selected.geo?.org || '-' }}</b></div>
          <div class="d-row"><label>设备</label><b>{{ selected.deviceCount }}<template v-if="selected.deviceLimit"> / {{ selected.deviceLimit }}</template></b></div>
          <div class="d-row"><label>流量</label><b>{{ formatBytes(selected.trafficUsed) }} / {{ selected.trafficLimit ? formatBytes(selected.trafficLimit) : '∞' }}</b></div>
          <div class="d-row"><label>最后活跃</label><b>{{ formatActive(selected.lastActiveAt) }}</b></div>
        </div>
        <div v-else class="empty sm">点击地球光点或左侧列表</div>

        <div class="panel-h mt"><span>节点热力</span></div>
        <div class="node-list">
          <div v-for="n in hotNodes" :key="n.id" class="node-row">
            <span class="ndot" :class="n.status || 'offline'" />
            <span class="nname">{{ n.name }}</span>
            <span class="ncount">{{ n.online_count || 0 }}</span>
          </div>
          <div v-if="!hotNodes.length" class="empty sm">无节点在线数据</div>
        </div>

        <div class="panel-h mt"><span>国家分布</span></div>
        <div class="country-bars">
          <div v-for="c in topCountries" :key="c.code" class="cbar">
            <span class="cname">{{ c.name }}</span>
            <div class="bar"><i :style="{ width: c.pct + '%' }" /></div>
            <span class="cnum">{{ c.count }}</span>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, shallowRef } from 'vue'
import Globe from 'globe.gl'
import { fetchOnlineSnapshot, type OnlineSnapshot, type OnlineEndpoint } from '@/api/monitor'
import { resolveIPs, maybeIP, type GeoResult } from '@/utils/geoip'
import { formatBytes } from '@/utils/format'
import type { Node } from '@/api/node'

interface EndpointView extends OnlineEndpoint {
  geo?: GeoResult
}

const globeEl = ref<HTMLElement | null>(null)
const globe = shallowRef<ReturnType<typeof Globe> | null>(null)
const snap = ref<OnlineSnapshot | null>(null)
const endpointsView = ref<EndpointView[]>([])
const selected = ref<EndpointView | null>(null)
const loading = ref(false)
const paused = ref(false)
const lastTick = ref('--:--:--')
let timer: ReturnType<typeof setInterval> | null = null
let resizeObs: ResizeObserver | null = null

const countryCount = computed(() => {
  const s = new Set(endpointsView.value.map(e => e.geo?.countryCode).filter(Boolean))
  return s.size
})

const activeNodes = computed(() =>
  (snap.value?.nodes || []).filter(n => (n.online_count || 0) > 0 || n.status === 'online').length,
)

const hotNodes = computed(() =>
  [...(snap.value?.nodes || [])]
    .sort((a, b) => (b.online_count || 0) - (a.online_count || 0))
    .slice(0, 8),
)

const topCountries = computed(() => {
  const m = new Map<string, { code: string; name: string; count: number }>()
  for (const e of endpointsView.value) {
    const code = e.geo?.countryCode || '??'
    const name = e.geo?.private ? '内网' : (e.geo?.country || 'Unknown')
    const cur = m.get(code) || { code, name, count: 0 }
    cur.count++
    m.set(code, cur)
  }
  const arr = [...m.values()].sort((a, b) => b.count - a.count).slice(0, 6)
  const max = arr[0]?.count || 1
  return arr.map(c => ({ ...c, pct: Math.round((c.count / max) * 100) }))
})

function locationText(e: EndpointView) {
  if (!e.geo) return '定位中…'
  if (e.geo.private) return '内网 / 私有地址'
  const parts = [e.geo.city, e.geo.region, e.geo.country].filter(Boolean)
  return parts.join(' · ') || e.geo.countryCode
}

function formatActive(iso: string | null) {
  if (!iso) return '-'
  try {
    return new Date(iso).toLocaleString('zh-CN')
  } catch {
    return iso
  }
}

function togglePause() {
  paused.value = !paused.value
}

async function refresh(manual = false) {
  if (loading.value) return
  loading.value = true
  try {
    const s = await fetchOnlineSnapshot()
    snap.value = s
    lastTick.value = new Date().toLocaleTimeString('zh-CN')

    const ips = s.endpoints.map(e => e.ip)
    // also resolve node hosts that look like IPs for arcs
    const nodeIPs = s.nodes.map(n => maybeIP(n.host)).filter(Boolean) as string[]
    const geoMap = await resolveIPs([...ips, ...nodeIPs], 5)

    const views: EndpointView[] = s.endpoints.map(e => ({
      ...e,
      geo: geoMap.get(e.ip),
    }))
    endpointsView.value = views

    // keep selection fresh
    if (selected.value) {
      const again = views.find(v => v.userId === selected.value!.userId && v.ip === selected.value!.ip)
      selected.value = again || selected.value
    }

    await nextTick()
    updateGlobe(views, s.nodes, geoMap)
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function focusEndpoint(e: EndpointView) {
  selected.value = e
  if (e.geo && globe.value) {
    globe.value.pointOfView({ lat: e.geo.lat, lng: e.geo.lng, altitude: 1.8 }, 800)
  }
}

function updateGlobe(views: EndpointView[], nodes: Node[], geoMap: Map<string, GeoResult>) {
  if (!globe.value) return

  const points = views
    .filter(v => v.geo && Number.isFinite(v.geo.lat) && Number.isFinite(v.geo.lng))
    .map(v => ({
      ...v,
      lat: v.geo!.lat,
      lng: v.geo!.lng,
      color: v.geo!.private ? '#fbbf24' : '#22d3ee',
      size: v.geo!.private ? 0.35 : 0.55,
    }))

  // Node “hubs”
  const hubs: { lat: number; lng: number; name: string; online: number }[] = []
  for (const n of nodes) {
    const ip = maybeIP(n.host)
    if (!ip) continue
    const g = geoMap.get(ip)
    if (!g || g.private) continue
    hubs.push({ lat: g.lat, lng: g.lng, name: n.name, online: n.online_count || 0 })
  }

  // Arcs: from node hub to user points (cap 40 for performance)
  const arcs: any[] = []
  if (hubs.length && points.length) {
    const hub = hubs.sort((a, b) => b.online - a.online)[0]
    for (const p of points.slice(0, 40)) {
      if (p.geo?.private) continue
      arcs.push({
        startLat: hub.lat,
        startLng: hub.lng,
        endLat: p.lat,
        endLng: p.lng,
        color: ['rgba(99,102,241,0.15)', 'rgba(34,211,238,0.85)'],
      })
    }
  }

  const rings = points.slice(0, 12).map(p => ({
    lat: p.lat,
    lng: p.lng,
    color: p.color,
  }))

  const labels = [
    ...hubs.map(h => ({
      lat: h.lat,
      lng: h.lng,
      text: h.name,
      color: '#a5b4fc',
      size: 0.8,
    })),
    ...points.slice(0, 15).map(p => ({
      lat: p.lat,
      lng: p.lng,
      text: p.geo?.city || p.geo?.countryCode || '',
      color: 'rgba(255,255,255,0.75)',
      size: 0.45,
    })),
  ].filter(l => l.text)

  globe.value
    .pointsData(points)
    .pointLat('lat')
    .pointLng('lng')
    .pointColor('color')
    .pointAltitude(0.02)
    .pointRadius('size')
    .pointLabel((d: any) =>
      `<div style="font-family:Inter,sans-serif;padding:6px 10px">
        <b>${d.email}</b><br/>
        <span style="opacity:.85">${d.ip}</span><br/>
        <span style="opacity:.7">${locationText(d)}</span>
      </div>`,
    )
    .arcsData(arcs)
    .arcColor('color')
    .arcAltitude(0.22)
    .arcStroke(0.4)
    .arcDashLength(0.35)
    .arcDashGap(0.2)
    .arcDashAnimateTime(2200)
    .ringsData(rings)
    .ringColor((d: any) => d.color)
    .ringMaxRadius(3)
    .ringPropagationSpeed(1.2)
    .ringRepeatPeriod(1400)
    .labelsData(labels)
    .labelLat('lat')
    .labelLng('lng')
    .labelText((d: any) => d.text)
    .labelSize((d: any) => d.size || 0.5)
    .labelColor((d: any) => d.color)
    .labelDotRadius(0.2)
    .labelAltitude(0.02)
    .labelResolution(2)
}

function initGlobe() {
  if (!globeEl.value) return
  const el = globeEl.value
  const g = Globe()(el)
    .globeImageUrl('https://unpkg.com/three-globe/example/img/earth-blue-marble.jpg')
    .bumpImageUrl('https://unpkg.com/three-globe/example/img/earth-topology.png')
    .backgroundImageUrl('https://unpkg.com/three-globe/example/img/night-sky.png')
    .showAtmosphere(true)
    .atmosphereColor('#4f46e5')
    .atmosphereAltitude(0.22)
    .backgroundColor('rgba(0,0,0,0)')
    .width(el.clientWidth)
    .height(el.clientHeight)
    .onPointClick((p: any) => {
      selected.value = p
      g.pointOfView({ lat: p.lat, lng: p.lng, altitude: 1.7 }, 700)
    })

  const controls = g.controls()
  controls.autoRotate = true
  controls.autoRotateSpeed = 0.55
  controls.enableZoom = true
  g.pointOfView({ lat: 25, lng: 105, altitude: 2.2 })

  try {
    g.renderer().setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
  } catch { /* */ }

  globe.value = g

  resizeObs = new ResizeObserver(() => {
    if (!globeEl.value || !globe.value) return
    globe.value.width(globeEl.value.clientWidth).height(globeEl.value.clientHeight)
  })
  resizeObs.observe(el)

  // Pause auto-rotate while hovering stage for exploration
  el.addEventListener('pointerenter', () => {
    if (globe.value) globe.value.controls().autoRotate = false
  })
  el.addEventListener('pointerleave', () => {
    if (globe.value && !paused.value) globe.value.controls().autoRotate = true
  })
}

function tickLoop() {
  if (timer) clearInterval(timer)
  timer = setInterval(() => {
    if (!paused.value) refresh(false)
  }, 8000)
}

onMounted(async () => {
  await nextTick()
  initGlobe()
  await refresh(true)
  tickLoop()
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  resizeObs?.disconnect()
  try {
    globe.value?._destructor()
  } catch { /* */ }
  globe.value = null
})
</script>

<style scoped>
.monitor {
  position: relative;
  margin: -8px -8px 0;
  min-height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
  gap: 12px;
  color: #e2e8f0;
}

.hud-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 4px 4px 0;
}
.hud-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.hud-title h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: #0f172a;
}
.hud-title p {
  margin: 2px 0 0;
  font-size: 12px;
  color: #64748b;
}
.live-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #94a3b8;
  box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.4);
}
.live-dot.on {
  background: #10b981;
  animation: pulse 1.6s infinite;
}
@keyframes pulse {
  0% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.55); }
  70% { box-shadow: 0 0 0 12px rgba(16, 185, 129, 0); }
  100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}
.hud-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tick {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  font-variant-numeric: tabular-nums;
  min-width: 72px;
}

.kpi-row {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 10px;
}
.kpi {
  background: #fff;
  border: 1px solid rgba(15, 23, 42, 0.06);
  border-radius: 14px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.04);
  position: relative;
  overflow: hidden;
}
.kpi::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
}
.k1::before { background: #10b981; }
.k2::before { background: #06b6d4; }
.k3::before { background: #6366f1; }
.k4::before { background: #8b5cf6; }
.k5::before { background: #f59e0b; }
.kpi span {
  font-size: 11px;
  font-weight: 700;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.kpi b {
  font-size: 22px;
  font-weight: 800;
  color: #0f172a;
  letter-spacing: -0.03em;
}

.stage {
  position: relative;
  flex: 1;
  min-height: 560px;
  border-radius: 20px;
  overflow: hidden;
  background: radial-gradient(ellipse at center, #0c1228 0%, #05070f 70%);
  border: 1px solid rgba(99, 102, 241, 0.2);
  box-shadow: 0 20px 60px rgba(15, 23, 42, 0.25), inset 0 0 80px rgba(79, 70, 229, 0.08);
}
.globe-host {
  position: absolute;
  inset: 0;
  z-index: 1;
}
.globe-vignette {
  pointer-events: none;
  position: absolute;
  inset: 0;
  z-index: 2;
  background: radial-gradient(ellipse at center, transparent 40%, rgba(5, 7, 15, 0.55) 100%);
}
.globe-hint {
  position: absolute;
  bottom: 14px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 5;
  font-size: 11px;
  font-weight: 600;
  color: rgba(226, 232, 240, 0.45);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  pointer-events: none;
}

.glass {
  background: rgba(8, 12, 28, 0.72);
  backdrop-filter: blur(16px) saturate(1.3);
  -webkit-backdrop-filter: blur(16px) saturate(1.3);
  border: 1px solid rgba(148, 163, 184, 0.14);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
}

.panel {
  position: absolute;
  top: 16px;
  bottom: 16px;
  width: min(300px, 32vw);
  z-index: 6;
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.panel.left { left: 16px; }
.panel.right { right: 16px; }

.panel-h {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px 8px;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #94a3b8;
}
.panel-h em {
  font-style: normal;
  color: #22d3ee;
  background: rgba(34, 211, 238, 0.12);
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
}
.panel-h.mt {
  margin-top: 8px;
  border-top: 1px solid rgba(148, 163, 184, 0.1);
  padding-top: 12px;
}

.feed {
  flex: 1;
  overflow-y: auto;
  padding: 4px 10px 12px;
}
.feed-item {
  padding: 10px 10px;
  border-radius: 12px;
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 0.15s ease;
  margin-bottom: 6px;
  background: rgba(255, 255, 255, 0.03);
}
.feed-item:hover {
  background: rgba(99, 102, 241, 0.12);
  border-color: rgba(129, 140, 248, 0.25);
}
.feed-item.active {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.28), rgba(34, 211, 238, 0.1));
  border-color: rgba(34, 211, 238, 0.35);
}
.feed-item.lan {
  border-left: 2px solid #fbbf24;
}
.fi-top {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}
.email {
  font-size: 12px;
  font-weight: 700;
  color: #f1f5f9;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.flag {
  font-size: 10px;
  font-weight: 800;
  color: #a5b4fc;
  background: rgba(99, 102, 241, 0.2);
  padding: 1px 6px;
  border-radius: 6px;
  flex-shrink: 0;
}
.fi-ip {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: #22d3ee;
  margin-bottom: 2px;
}
.fi-loc {
  font-size: 11px;
  color: #94a3b8;
}

.detail {
  padding: 0 14px 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.d-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
  align-items: flex-start;
}
.d-row label {
  color: #64748b;
  font-weight: 600;
  flex-shrink: 0;
}
.d-row b {
  color: #e2e8f0;
  font-weight: 700;
  text-align: right;
  word-break: break-all;
}
.d-row code {
  color: #22d3ee;
  font-size: 11px;
  background: rgba(34, 211, 238, 0.1);
  padding: 2px 6px;
  border-radius: 6px;
}

.node-list, .country-bars {
  padding: 0 12px 10px;
  overflow-y: auto;
  max-height: 160px;
}
.node-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  font-size: 12px;
}
.ndot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #64748b;
  flex-shrink: 0;
}
.ndot.online { background: #10b981; box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.25); }
.ndot.warning { background: #f59e0b; }
.nname { flex: 1; color: #cbd5e1; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ncount {
  font-weight: 800;
  color: #a5b4fc;
  background: rgba(99, 102, 241, 0.15);
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
}

.cbar {
  display: grid;
  grid-template-columns: 72px 1fr 28px;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  font-size: 11px;
}
.cname { color: #94a3b8; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bar {
  height: 6px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #6366f1, #22d3ee);
}
.cnum { color: #e2e8f0; font-weight: 800; text-align: right; }

.empty {
  text-align: center;
  color: #64748b;
  font-size: 13px;
  padding: 40px 16px;
  line-height: 1.6;
}
.empty.sm { padding: 12px; font-size: 12px; }
.empty small { color: #475569; }

@media (max-width: 1100px) {
  .kpi-row { grid-template-columns: repeat(3, 1fr); }
  .panel { width: min(260px, 36vw); }
}
@media (max-width: 800px) {
  .kpi-row { grid-template-columns: repeat(2, 1fr); }
  .panel.left, .panel.right {
    position: relative;
    top: auto; left: auto; right: auto; bottom: auto;
    width: 100%;
    max-height: 280px;
    margin: 0;
  }
  .stage {
    display: flex;
    flex-direction: column;
    min-height: auto;
  }
  .globe-host {
    position: relative;
    height: 420px;
  }
  .globe-vignette { display: none; }
}
</style>
