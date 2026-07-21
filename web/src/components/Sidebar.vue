<template>
  <div class="sidebar">
    <div class="sidebar-bg" aria-hidden="true">
      <div class="glow g1" />
      <div class="glow g2" />
      <div class="grid-fade" />
    </div>

    <!-- Brand -->
    <div class="brand">
      <div class="brand-mark">
        <svg viewBox="0 0 32 32" width="20" height="20" fill="none" aria-hidden="true">
          <path d="M6 18c4-10 16-12 20-4 1 2 1 5-1 7-4 4-12 5-17 1" stroke="url(#sg)" stroke-width="2.4" stroke-linecap="round"/>
          <circle cx="22" cy="11" r="3" fill="url(#sg)"/>
          <defs>
            <linearGradient id="sg" x1="6" y1="8" x2="26" y2="24" gradientUnits="userSpaceOnUse">
              <stop stop-color="#4f46e5"/><stop offset="1" stop-color="#0891b2"/>
            </linearGradient>
          </defs>
        </svg>
      </div>
      <div class="brand-text">
        <span class="brand-name">K2Board</span>
        <span class="brand-sub">Admin Console</span>
      </div>
    </div>

    <!-- Nav groups -->
    <nav class="nav-scroll" aria-label="主导航">
      <div v-for="group in navGroups" :key="group.id" class="nav-group">
        <div class="group-head">
          <span class="group-title">{{ group.title }}</span>
          <span class="group-line" />
        </div>
        <div class="group-items">
          <router-link
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            class="nav-item"
            :class="{
              active: isActive(item.path),
              primary: item.primary,
            }"
            @click="$emit('nav')"
          >
            <span class="nav-icon" :class="'tone-' + (item.tone || 'slate')">
              <el-icon :size="16"><component :is="item.icon" /></el-icon>
            </span>
            <span class="nav-meta">
              <span class="nav-title">{{ item.title }}</span>
              <span v-if="item.desc" class="nav-desc">{{ item.desc }}</span>
            </span>
            <span v-if="isActive(item.path)" class="nav-rail" />
          </router-link>
        </div>
      </div>
    </nav>

    <!-- Footer -->
    <div class="sidebar-foot">
      <div class="foot-status">
        <span class="pulse" />
        <div class="foot-copy">
          <span class="foot-label">服务状态</span>
          <span class="foot-value">运行中</span>
        </div>
      </div>
      <div class="foot-ver">Aurora · v1.4</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import {
  Odometer, UserFilled, Monitor, Collection, TrendCharts,
  Cpu, Notebook, Setting, Grid, Place, Wallet, CreditCard, Share,
} from '@element-plus/icons-vue'

defineEmits<{ nav: [] }>()
const route = useRoute()

/**
 * Navigation grouped by domain — primary items first within each group.
 * tone: icon accent color key
 */
const navGroups = [
  {
    id: 'overview',
    title: '概览',
    items: [
      { path: '/', title: '仪表盘', desc: '核心指标', icon: Odometer, primary: true, tone: 'indigo' },
    ],
  },
  {
    id: 'users',
    title: '用户与订阅',
    items: [
      { path: '/users', title: '用户管理', desc: '账号与权益', icon: UserFilled, primary: true, tone: 'violet' },
      { path: '/groups', title: '权限组', desc: '节点可见性', icon: Grid, tone: 'slate' },
      { path: '/plans', title: '订阅计划', desc: '套餐与定价', icon: Collection, tone: 'cyan' },
    ],
  },
  {
    id: 'commerce',
    title: '商业支付',
    items: [
      { path: '/orders', title: '订单管理', desc: '成交与补单', icon: Wallet, primary: true, tone: 'amber' },
      { path: '/payment-methods', title: '支付方式', desc: '渠道配置', icon: CreditCard, tone: 'slate' },
      { path: '/referral', title: '推广管理', desc: '返佣与提现', icon: Share, primary: true, tone: 'violet' },
    ],
  },
  {
    id: 'fleet',
    title: '节点运维',
    items: [
      { path: '/nodes', title: '节点管理', desc: '协议与接入', icon: Monitor, primary: true, tone: 'emerald' },
      { path: '/nodes?tab=monitor', title: '节点监控', desc: '负载与健康', icon: TrendCharts, primary: true, tone: 'cyan' },
      { path: '/monitor', title: '在线监控', desc: '用户在线地图', icon: Place, tone: 'sky' },
    ],
  },
  {
    id: 'insights',
    title: '数据与任务',
    items: [
      { path: '/traffic', title: '流量分析', desc: '消耗洞察', icon: TrendCharts, primary: true, tone: 'blue' },
      { path: '/queue', title: '后台调度', desc: '刷盘与定时', icon: Cpu, tone: 'slate' },
    ],
  },
  {
    id: 'system',
    title: '系统',
    items: [
      { path: '/logs', title: '系统日志', desc: '审计记录', icon: Notebook, tone: 'slate' },
      { path: '/settings', title: '系统设置', desc: '站点与邮件', icon: Setting, tone: 'rose' },
    ],
  },
]

function isActive(path: string) {
  if (path === '/') return route.path === '/'
  // Nodes list vs nodes monitor share /nodes path; distinguish by query tab
  if (path === '/nodes') {
    return route.path === '/nodes' && route.query.tab !== 'monitor'
  }
  if (path === '/nodes?tab=monitor') {
    return route.path === '/nodes' && route.query.tab === 'monitor'
  }
  const base = path.split('?')[0]
  return route.path === base || route.path.startsWith(base + '/')
}
</script>

<style scoped>
.sidebar {
  position: relative;
  height: 100%;
  display: flex;
  flex-direction: column;
  color: var(--k2-text);
  overflow: hidden;
  background: var(--k2-sidebar);
  border-right: 1px solid var(--k2-sidebar-border);
}

.sidebar-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}
.glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(48px);
  opacity: 0.55;
}
.g1 {
  width: 180px;
  height: 180px;
  top: -70px;
  left: -50px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.22), transparent 70%);
}
.g2 {
  width: 140px;
  height: 140px;
  bottom: 100px;
  right: -50px;
  background: radial-gradient(circle, rgba(8, 145, 178, 0.14), transparent 70%);
}
.grid-fade {
  display: none;
}

/* Brand */
.brand {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 16px 14px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--k2-border);
  margin: 0 8px 4px;
}
.brand-mark {
  width: 38px;
  height: 38px;
  border-radius: 11px;
  display: grid;
  place-items: center;
  background: var(--k2-primary-soft);
  border: 1px solid #c7d2fe;
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.12);
}
.brand-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.brand-name {
  font-size: 16px;
  font-weight: 800;
  letter-spacing: 0.01em;
  color: var(--k2-text);
  line-height: 1.2;
}
.brand-sub {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--k2-text-muted);
}

/* Scroll nav */
.nav-scroll {
  position: relative;
  z-index: 1;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.35) transparent;
}
.nav-scroll::-webkit-scrollbar {
  width: 4px;
}
.nav-scroll::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.35);
  border-radius: 4px;
}

.nav-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.group-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px 6px;
}
.group-title {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #94a3b8;
  white-space: nowrap;
  flex-shrink: 0;
}
.group-line {
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, #e2e8f0, transparent);
}

.group-items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 11px;
  text-decoration: none;
  color: var(--k2-text-secondary);
  transition: background 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}
.nav-item:hover {
  color: var(--k2-text);
  background: var(--k2-bg-soft);
}
.nav-item.primary:not(.active) .nav-title {
  font-weight: 600;
  color: var(--k2-text);
}
.nav-item.active {
  color: var(--k2-primary);
  background: var(--k2-primary-soft);
  box-shadow: inset 0 0 0 1px #c7d2fe;
}
.nav-item.active .nav-title {
  font-weight: 700;
  color: var(--k2-primary);
}
.nav-item.active .nav-desc {
  color: #818cf8;
}

.nav-icon {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
  border: 1px solid transparent;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

/* Icon tone accents — lively pastels on light sidebar */
.tone-indigo { color: #4f46e5; background: #eef2ff; border-color: #c7d2fe; }
.tone-violet { color: #7c3aed; background: #f3e8ff; border-color: #e9d5ff; }
.tone-cyan { color: #0891b2; background: #ecfeff; border-color: #a5f3fc; }
.tone-amber { color: #d97706; background: #fffbeb; border-color: #fde68a; }
.tone-emerald { color: #059669; background: #ecfdf5; border-color: #a7f3d0; }
.tone-sky { color: #0284c7; background: #e0f2fe; border-color: #bae6fd; }
.tone-blue { color: #2563eb; background: #eff6ff; border-color: #bfdbfe; }
.tone-rose { color: #e11d48; background: #fff1f2; border-color: #fecdd3; }
.tone-slate { color: #64748b; background: #f1f5f9; border-color: #e2e8f0; }

.nav-item.active .nav-icon {
  box-shadow: 0 2px 8px rgba(79, 70, 229, 0.12);
}
.nav-item.active .tone-indigo { background: #e0e7ff; color: #4338ca; }
.nav-item.active .tone-violet { background: #ede9fe; color: #6d28d9; }
.nav-item.active .tone-cyan { background: #cffafe; color: #0e7490; }
.nav-item.active .tone-amber { background: #fef3c7; color: #b45309; }
.nav-item.active .tone-emerald { background: #d1fae5; color: #047857; }
.nav-item.active .tone-sky { background: #bae6fd; color: #0369a1; }
.nav-item.active .tone-blue { background: #dbeafe; color: #1d4ed8; }
.nav-item.active .tone-rose { background: #ffe4e6; color: #be123c; }
.nav-item.active .tone-slate { background: #e2e8f0; color: #475569; }

.nav-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.nav-title {
  font-size: 13.5px;
  font-weight: 500;
  line-height: 1.25;
  letter-spacing: -0.01em;
}
.nav-desc {
  font-size: 10.5px;
  font-weight: 500;
  color: #94a3b8;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nav-item:hover .nav-desc {
  color: #64748b;
}

.nav-rail {
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 18px;
  border-radius: 0 3px 3px 0;
  background: linear-gradient(180deg, #6366f1, #22d3ee);
  box-shadow: 0 0 8px rgba(99, 102, 241, 0.35);
}

/* Footer */
.sidebar-foot {
  position: relative;
  z-index: 1;
  flex-shrink: 0;
  padding: 12px 14px 16px;
  border-top: 1px solid var(--k2-border);
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: linear-gradient(180deg, transparent, #f8fafc);
}
.foot-status {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 12px;
  background: var(--k2-success-soft);
  border: 1px solid #bbf7d0;
}
.pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.45);
  animation: pulse 2s infinite;
  flex-shrink: 0;
}
@keyframes pulse {
  0% { box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.45); }
  70% { box-shadow: 0 0 0 8px rgba(34, 197, 94, 0); }
  100% { box-shadow: 0 0 0 0 rgba(34, 197, 94, 0); }
}
.foot-copy {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.foot-label {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: #059669;
}
.foot-value {
  font-size: 12px;
  font-weight: 700;
  color: #047857;
}
.foot-ver {
  font-size: 10px;
  font-weight: 500;
  color: #94a3b8;
  padding-left: 4px;
  letter-spacing: 0.04em;
}
</style>
