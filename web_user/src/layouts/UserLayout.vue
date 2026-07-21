<template>
  <div class="shell">
    <aside class="sidebar" v-if="store.isLoggedIn">
      <div class="sidebar-inner">
        <div class="brand" @click="$router.push('/user')">
          <div class="brand-mark">☁</div>
          <div class="brand-text">
            <span class="brand-name">东京热云</span>
            <span class="brand-tag">用户中心</span>
          </div>
        </div>

        <nav class="nav">
          <router-link to="/user" class="nav-link" active-class="" exact-active-class="active">
            <span class="nav-ico">⌂</span>
            <span>仪表盘</span>
          </router-link>
          <router-link to="/user/subscribe" class="nav-link" active-class="active">
            <span class="nav-ico">☰</span>
            <span>订阅</span>
          </router-link>
          <router-link to="/user/docs" class="nav-link" active-class="active">
            <span class="nav-ico">?</span>
            <span>使用教程</span>
          </router-link>
          <router-link to="/user/orders" class="nav-link nav-orders" active-class="active">
            <span class="nav-ico">≡</span>
            <span>我的订单</span>
            <span v-if="store.pendingOrderCount > 0" class="nav-badge">
              {{ store.pendingOrderCount > 9 ? '9+' : store.pendingOrderCount }}
            </span>
          </router-link>
          <router-link to="/user/referral" class="nav-link" active-class="active">
            <span class="nav-ico">↗</span>
            <span>推广返佣</span>
          </router-link>
          <router-link to="/user/profile" class="nav-link" active-class="active">
            <span class="nav-ico">◎</span>
            <span>个人中心</span>
          </router-link>
        </nav>

        <div class="sidebar-foot">
          <div class="user-chip" v-if="store.info || store.email">
            <div class="user-avatar">{{ avatarLetter }}</div>
            <div class="user-meta">
              <span class="user-mail">{{ displayEmail }}</span>
              <span class="user-group">{{ store.info?.group_name || '未分组' }}</span>
            </div>
          </div>
          <button class="logout-btn" type="button" @click="store.logout()">退出登录</button>
          <!-- 左下角主题切换 -->
          <button
            class="theme-toggle"
            type="button"
            :title="isDark ? '切换到亮色主题' : '切换到暗色主题'"
            @click="toggleTheme"
          >
            <span class="tt-ico" aria-hidden="true">{{ isDark ? '☀' : '☾' }}</span>
            <span class="tt-label">{{ isDark ? '亮色模式' : '暗色模式' }}</span>
            <span class="tt-badge">{{ isDark ? 'DARK' : 'LIGHT' }}</span>
          </button>
        </div>
      </div>
    </aside>

    <header class="topbar" v-if="store.isLoggedIn">
      <div class="topbar-inner">
        <div class="brand mobile-brand" @click="$router.push('/user')">
          <div class="brand-mark sm">☁</div>
          <span class="brand-name">东京热云</span>
        </div>
        <div class="topbar-actions">
          <button
            class="topbar-theme"
            type="button"
            :title="isDark ? '切换到亮色' : '切换到暗色'"
            @click="toggleTheme"
          >
            {{ isDark ? '☀' : '☾' }}
          </button>
          <button class="topbar-logout" type="button" @click="store.logout()">退出</button>
        </div>
      </div>
    </header>

    <div class="workspace">
      <main class="main">
        <div class="container">
          <router-view v-slot="{ Component }">
            <transition name="fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </main>
      <footer class="footer">
        <span>© 2026 东京热云</span>
      </footer>
    </div>

    <!-- 移动端底部导航：避免顶部横向滚动挤出/抖动 -->
    <nav v-if="store.isLoggedIn" class="bottom-nav" aria-label="主导航">
      <router-link to="/user" class="bn-item" active-class="" exact-active-class="active">
        <span class="bn-ico">⌂</span>
        <span>首页</span>
      </router-link>
      <router-link to="/user/subscribe" class="bn-item" active-class="active">
        <span class="bn-ico">☰</span>
        <span>订阅</span>
      </router-link>
      <router-link to="/user/orders" class="bn-item bn-orders" active-class="active">
        <span class="bn-ico">≡</span>
        <span>订单</span>
        <i v-if="store.pendingOrderCount > 0" class="bn-dot" />
      </router-link>
      <router-link to="/user/docs" class="bn-item" active-class="active">
        <span class="bn-ico">?</span>
        <span>教程</span>
      </router-link>
      <router-link to="/user/referral" class="bn-item" active-class="active">
        <span class="bn-ico">↗</span>
        <span>推广</span>
      </router-link>
      <router-link to="/user/profile" class="bn-item" active-class="active">
        <span class="bn-ico">◎</span>
        <span>我的</span>
      </router-link>
    </nav>

    <!-- 桌面侧栏已有主题切换；移动端用顶栏按钮，不再用左下 FAB 挡内容 -->

    <!-- 登录后：可拖动客服入口（隐藏 Crisp 默认右下角气泡） -->
    <CrispDraggableLauncher v-if="store.isLoggedIn" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserAuthStore } from '@/stores/userAuth'
import { useTheme } from '@/composables/useTheme'
import { loadCrisp, identifyCrisp } from '@/utils/crisp'
import CrispDraggableLauncher from '@/components/CrispDraggableLauncher.vue'

const store = useUserAuthStore()
const router = useRouter()
const route = useRoute()
const { isDark, toggleTheme } = useTheme()

const displayEmail = computed(() => store.info?.email || store.email || '用户')
const avatarLetter = computed(() => (displayEmail.value || 'U').charAt(0).toUpperCase())

onMounted(async () => {
  if (!store.isLoggedIn) {
    router.replace('/user/login')
    return
  }
  // Crisp only after login — never on login/register pages
  loadCrisp()
  if (!store.info) await store.fetchInfo()
  else {
    identifyCrisp({
      email: store.info.email,
      id: store.info.id,
      plan_name: store.info.plan_name,
      group_name: store.info.group_name,
      expire_text: store.info.expire_text,
    })
  }
  await store.refreshPendingCount()
})

watch(
  () => route.path,
  (p) => {
    if (!store.isLoggedIn) return
    if (p.includes('/user/orders') || p.includes('/order-result') || p === '/user') {
      store.refreshPendingCount()
    }
  },
)
</script>

<style scoped>
/* App shell: viewport-locked; only .workspace scrolls so sidebar/tabs never run off */
.shell {
  height: 100vh;
  height: 100dvh;
  max-height: 100vh;
  max-height: 100dvh;
  display: flex;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  background: var(--u-shell-bg);
  color: var(--u-text);
  transition: background 0.25s ease, color 0.2s ease;
  /* iOS notch / home indicator */
  padding-left: env(safe-area-inset-left);
  padding-right: env(safe-area-inset-right);
}

.sidebar {
  position: relative;
  flex-shrink: 0;
  align-self: stretch;
  width: var(--u-sidebar-w);
  min-width: var(--u-sidebar-w);
  height: 100%;
  max-height: 100%;
  z-index: 30;
  padding: 12px;
  box-sizing: border-box;
  display: none;
  overflow: hidden;
}
.sidebar-inner {
  height: 100%;
  max-height: 100%;
  display: flex;
  flex-direction: column;
  padding: 16px 12px;
  border-radius: var(--u-radius-lg);
  background: var(--u-sidebar-bg);
  border: 1px solid var(--u-border);
  box-shadow: var(--u-shadow-1);
  box-sizing: border-box;
  overflow: hidden;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  user-select: none;
  padding: 4px 4px 14px;
  border-bottom: 1px solid var(--u-border);
  margin-bottom: 12px;
}
.brand-mark {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  background: var(--u-primary-soft);
  border: 1px solid color-mix(in srgb, var(--u-primary) 40%, transparent);
  color: var(--u-primary);
  font-size: 16px;
  flex-shrink: 0;
}
.brand-mark.sm {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  font-size: 14px;
}
.brand-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.brand-name {
  font-size: 15px;
  font-weight: 800;
  color: var(--u-text);
  line-height: 1.2;
}
.brand-tag {
  font-size: 11px;
  color: var(--u-text-3);
  font-weight: 600;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}
.nav-link {
  border: none;
  background: transparent;
  color: var(--u-text-2);
  text-decoration: none;
  font-size: 13.5px;
  font-weight: 600;
  padding: 10px 12px;
  border-radius: 10px;
  cursor: pointer;
  font-family: inherit;
  display: flex;
  align-items: center;
  gap: 10px;
  transition: background 0.12s ease, color 0.12s ease;
}
.nav-ico {
  width: 20px;
  text-align: center;
  opacity: 0.7;
  flex-shrink: 0;
  font-size: 14px;
}
.nav-badge {
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--u-warning);
  color: var(--u-text-inv);
  font-size: 10px;
  font-weight: 800;
  display: inline-grid;
  place-items: center;
}
.nav-link:hover {
  color: var(--u-text);
  background: var(--u-bg-soft);
}
.nav-link.active {
  color: var(--u-primary);
  background: var(--u-primary-soft);
}
.nav-link.active .nav-ico {
  opacity: 1;
}

.sidebar-foot {
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--u-border);
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 12px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
}
.user-avatar {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: var(--u-text-inv);
  background: linear-gradient(135deg, var(--u-primary), var(--u-primary-strong));
  flex-shrink: 0;
}
.user-meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.user-mail {
  font-size: 12px;
  font-weight: 700;
  color: var(--u-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.user-group {
  font-size: 11px;
  color: var(--u-text-3);
}
.logout-btn {
  width: 100%;
  border: 1px solid var(--u-border);
  background: var(--u-surface);
  color: var(--u-text-2);
  font-size: 13px;
  font-weight: 600;
  padding: 9px 12px;
  border-radius: 10px;
  cursor: pointer;
  font-family: inherit;
}
.logout-btn:hover {
  color: var(--u-danger);
  background: var(--u-danger-soft);
  border-color: color-mix(in srgb, var(--u-danger) 35%, transparent);
}

.topbar {
  display: block;
  position: relative;
  flex-shrink: 0;
  z-index: 20;
  width: 100%;
  max-width: 100%;
  background: color-mix(in srgb, var(--u-surface) 92%, transparent);
  border-bottom: 1px solid var(--u-border);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  padding-top: env(safe-area-inset-top);
  box-sizing: border-box;
}
.topbar-inner {
  width: 100%;
  min-height: 52px;
  padding: 10px 14px;
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  box-sizing: border-box;
}
.mobile-brand {
  padding: 0;
  border: none;
  margin: 0;
  min-width: 0;
}
.topbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.topbar-theme,
.topbar-logout {
  border: 1px solid var(--u-border);
  background: var(--u-surface-2);
  color: var(--u-text-2);
  font-family: inherit;
  font-weight: 700;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.topbar-theme {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  font-size: 18px;
  display: grid;
  place-items: center;
}
.topbar-logout {
  height: 40px;
  padding: 0 12px;
  border-radius: 10px;
  font-size: 13px;
}

.workspace {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0; /* critical: allow flex child to scroll */
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 100%;
  height: 100%;
  overflow-x: hidden;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  overscroll-behavior: contain;
}
.main {
  flex: 1 0 auto;
  padding: var(--u-page-pad);
  padding-bottom: 12px;
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
  overflow-x: hidden;
}
.container {
  width: 100%;
  max-width: var(--u-content-max);
  margin: 0 auto;
  min-width: 0;
  box-sizing: border-box;
}
.footer {
  flex-shrink: 0;
  text-align: center;
  padding: 16px;
  padding-bottom: max(16px, env(safe-area-inset-bottom));
  color: var(--u-text-3);
  font-size: 12px;
  box-sizing: border-box;
}

/* Bottom tab bar (mobile only) */
.bottom-nav {
  display: none;
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 50;
  background: color-mix(in srgb, var(--u-surface) 96%, transparent);
  border-top: 1px solid var(--u-border);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  padding: 6px 4px calc(6px + env(safe-area-inset-bottom));
  justify-content: space-around;
  align-items: stretch;
  gap: 2px;
  box-shadow: 0 -4px 20px rgba(15, 23, 42, 0.06);
}
.bn-item {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  padding: 6px 2px;
  border-radius: 10px;
  text-decoration: none;
  color: var(--u-text-3);
  font-size: 10px;
  font-weight: 700;
  position: relative;
  -webkit-tap-highlight-color: transparent;
  transition: color 0.12s ease, background 0.12s ease;
}
.bn-ico {
  font-size: 16px;
  line-height: 1;
  opacity: 0.85;
}
.bn-item.active {
  color: var(--u-primary);
  background: var(--u-primary-soft);
}
.bn-item.active .bn-ico {
  opacity: 1;
}
.bn-dot {
  position: absolute;
  top: 4px;
  right: 18%;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--u-warning);
  box-shadow: 0 0 0 2px var(--u-surface);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.12s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@media (min-width: 960px) {
  .shell {
    flex-direction: row;
    align-items: stretch;
  }
  .sidebar {
    display: block;
  }
  .topbar,
  .bottom-nav {
    display: none !important;
  }
  .main {
    padding: clamp(16px, 2vw, 28px) clamp(16px, 2.5vw, 32px) 16px;
  }
}
@media (max-width: 959.98px) {
  .shell {
    flex-direction: column;
  }
  .bottom-nav {
    display: flex;
  }
  .main {
    /* room for bottom tab bar + safe area */
    padding-bottom: calc(72px + env(safe-area-inset-bottom));
  }
  .footer {
    display: none;
  }
  /* Keep nav visible inside sidebar if ever shown on mid widths */
  .sidebar-inner .nav {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }
}
@media (prefers-reduced-motion: reduce) {
  .fade-enter-active,
  .fade-leave-active {
    transition: none;
  }
}
</style>
