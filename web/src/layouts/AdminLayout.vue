<template>
  <el-container class="layout-root" :class="{ 'is-mobile': isMobile, 'drawer-open': mobileOpen }">
    <div v-if="isMobile && mobileOpen" class="mobile-overlay" @click="mobileOpen = false" />

    <el-aside
      :width="asideWidth"
      class="layout-aside"
      :class="{ 'is-drawer': isMobile, open: mobileOpen }"
    >
      <Sidebar @nav="mobileOpen = false" />
    </el-aside>

    <el-container class="layout-body">
      <el-header class="layout-header">
        <div class="header-left">
          <button
            v-if="isMobile"
            class="icon-btn"
            type="button"
            @click="mobileOpen = !mobileOpen"
            aria-label="打开菜单"
          >
            <el-icon :size="20"><Menu /></el-icon>
          </button>
          <div class="crumb-wrap">
            <span class="crumb-root">控制台</span>
            <template v-if="sectionLabel">
              <span class="crumb-sep">/</span>
              <span class="crumb-sec">{{ sectionLabel }}</span>
            </template>
            <span v-if="pageTitle" class="crumb-sep">/</span>
            <span v-if="pageTitle" class="crumb-cur">{{ pageTitle }}</span>
          </div>
        </div>
        <AppHeader />
      </el-header>

      <el-main class="layout-main">
        <div class="main-mesh" aria-hidden="true" />
        <div class="main-content">
          <router-view v-slot="{ Component, route: r }">
            <transition name="page" mode="out-in">
              <component :is="Component" :key="r.path" />
            </transition>
          </router-view>
        </div>
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Menu } from '@element-plus/icons-vue'
import Sidebar from '@/components/Sidebar.vue'
import AppHeader from '@/components/AppHeader.vue'

const route = useRoute()
const isMobile = ref(false)
const mobileOpen = ref(false)

const sectionMap: Record<string, string> = {
  '/': '概览',
  '/users': '用户与订阅',
  '/groups': '用户与订阅',
  '/plans': '用户与订阅',
  '/orders': '商业支付',
  '/payment-methods': '商业支付',
  '/nodes': '节点运维',
  '/monitor': '节点运维',
  '/traffic': '数据与任务',
  '/queue': '数据与任务',
  '/logs': '系统',
  '/settings': '系统',
  '/referral': '商业支付',
}
const sectionLabel = computed(() => {
  const p = route.path
  if (p === '/') return sectionMap['/']
  for (const [key, label] of Object.entries(sectionMap)) {
    if (key !== '/' && (p === key || p.startsWith(key + '/'))) return label
  }
  return ''
})

const pageTitle = computed(() => {
  if (route.path === '/nodes' && route.query.tab === 'monitor') return '节点监控'
  return (route.meta.title as string) || ''
})

// Always report full width to el-aside; mobile hide is CSS translate (avoids width:0 collapse)
const asideWidth = '268px'

let mq: MediaQueryList | null = null
function onMqChange() {
  const next = mq ? mq.matches : window.innerWidth < 900
  isMobile.value = next
  if (!next) mobileOpen.value = false
}
watch(() => route.fullPath, () => { mobileOpen.value = false })

onMounted(() => {
  mq = window.matchMedia('(max-width: 899.98px)')
  onMqChange()
  mq.addEventListener('change', onMqChange)
})
onUnmounted(() => {
  mq?.removeEventListener('change', onMqChange)
})
</script>

<style scoped>
.layout-root {
  height: 100vh;
  height: 100dvh;
  max-width: 100%;
  overflow: hidden;
  background: var(--k2-bg);
}
.layout-aside {
  background: transparent;
  overflow: hidden;
  z-index: 40;
  flex-shrink: 0;
  transition: transform 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
.layout-body {
  min-width: 0;
  flex: 1;
  max-width: 100%;
  overflow: hidden;
}
.layout-header {
  height: 56px;
  min-height: 56px;
  padding: 0 20px;
  padding-left: max(16px, env(safe-area-inset-left));
  padding-right: max(16px, env(safe-area-inset-right));
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  background: rgba(255, 255, 255, 0.94);
  backdrop-filter: blur(12px) saturate(1.2);
  -webkit-backdrop-filter: blur(12px) saturate(1.2);
  border-bottom: 1px solid var(--k2-border);
  position: sticky;
  top: 0;
  z-index: 10;
  flex-shrink: 0;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}
.icon-btn {
  display: none;
  border: none;
  background: var(--k2-primary-soft);
  color: var(--k2-primary);
  width: 40px;
  height: 40px;
  border-radius: 10px;
  cursor: pointer;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  -webkit-tap-highlight-color: transparent;
}
.crumb-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  min-width: 0;
  overflow: hidden;
}
.crumb-root {
  color: var(--k2-text-muted);
  font-weight: 500;
  flex-shrink: 0;
}
.crumb-sec {
  color: var(--k2-text-secondary, #64748b);
  font-weight: 600;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.crumb-sep {
  color: #cbd5e1;
  flex-shrink: 0;
}
.crumb-cur {
  color: var(--k2-text);
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.layout-main {
  position: relative;
  padding: 0;
  overflow: auto;
  overflow-x: hidden;
  min-height: 0;
  flex: 1;
  background: transparent;
  -webkit-overflow-scrolling: touch;
}
.main-mesh {
  pointer-events: none;
  position: absolute;
  inset: 0;
  background:
    radial-gradient(900px 400px at 10% -10%, rgba(79, 70, 229, 0.06), transparent 55%),
    radial-gradient(700px 360px at 100% 0%, rgba(8, 145, 178, 0.05), transparent 50%),
    linear-gradient(180deg, #f8fafc 0%, var(--k2-bg) 45%);
  z-index: 0;
}
.main-content {
  position: relative;
  z-index: 1;
  padding: 24px 28px 36px;
  padding-bottom: max(36px, env(safe-area-inset-bottom));
  max-width: 1440px;
  width: 100%;
  margin: 0 auto;
  box-sizing: border-box;
}
.mobile-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.4);
  backdrop-filter: blur(2px);
  z-index: 35;
  animation: fadeIn 0.2s ease;
}
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* Mobile drawer: never collapse width to 0 (that caused cramped layout) */
@media (max-width: 899.98px) {
  .layout-aside.is-drawer {
    position: fixed !important;
    top: 0;
    left: 0;
    bottom: 0;
    width: min(280px, 86vw) !important;
    max-width: 280px;
    height: 100dvh !important;
    transform: translate3d(-105%, 0, 0);
    box-shadow: none;
    will-change: transform;
  }
  .layout-aside.is-drawer.open {
    transform: translate3d(0, 0, 0);
    box-shadow: 12px 0 40px rgba(15, 23, 42, 0.18);
  }
  .icon-btn {
    display: inline-flex;
  }
  .layout-header {
    height: 52px;
    min-height: 52px;
    padding: 0 12px;
  }
  .main-content {
    padding: 14px 12px 28px;
  }
  .crumb-sec {
    display: none;
  }
  .crumb-sep:first-of-type {
    display: none;
  }
}

@media (min-width: 900px) {
  .layout-header {
    height: 64px;
    min-height: 64px;
    padding: 0 28px;
  }
}

/* Lighter page transition on small screens (reduce jitter) */
.page-enter-active,
.page-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.page-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.page-leave-to {
  opacity: 0;
  transform: translateY(-3px);
}
@media (max-width: 899.98px) {
  .page-enter-active,
  .page-leave-active {
    transition: opacity 0.12s ease;
  }
  .page-enter-from,
  .page-leave-to {
    transform: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .page-enter-active,
  .page-leave-active,
  .layout-aside {
    transition: none !important;
  }
  .page-enter-from,
  .page-leave-to {
    transform: none;
  }
}
</style>
