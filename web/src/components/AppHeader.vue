<template>
  <div class="app-header">
    <div class="header-actions">
      <div class="status-pill">
        <span class="dot" />
        Online
      </div>
      <el-dropdown trigger="click" placement="bottom-end">
        <button class="user-chip" type="button">
          <span class="avatar">{{ initials }}</span>
          <span class="meta">
            <span class="name">{{ auth.email || 'Admin' }}</span>
            <span class="role">Administrator</span>
          </span>
          <el-icon class="chev"><ArrowDown /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="auth.logout">
              <el-icon><SwitchButton /></el-icon>
              退出登录
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowDown, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const initials = computed(() => {
  const e = auth.email || 'A'
  return e.slice(0, 1).toUpperCase()
})
</script>

<style scoped>
.app-header {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  width: auto;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.status-pill {
  display: none;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: #059669;
  background: #ecfdf5;
  border: 1px solid #a7f3d0;
  padding: 5px 10px;
  border-radius: 999px;
}
.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
}
@media (min-width: 720px) {
  .status-pill { display: inline-flex; }
}
.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--k2-border);
  background: #fff;
  border-radius: 999px;
  padding: 4px 12px 4px 4px;
  cursor: pointer;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
  font-family: inherit;
  box-shadow: var(--k2-shadow-sm);
}
.user-chip:hover {
  border-color: #a5b4fc;
  box-shadow: 0 4px 14px rgba(79, 70, 229, 0.12);
}
.avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 700;
  color: #fff;
  background: var(--k2-gradient);
  box-shadow: 0 2px 8px rgba(79, 70, 229, 0.25);
}
.meta {
  display: none;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.15;
  text-align: left;
}
@media (min-width: 720px) {
  .meta { display: flex; }
}
.name {
  font-size: 13px;
  font-weight: 600;
  color: var(--k2-text);
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.role {
  font-size: 11px;
  color: var(--k2-text-muted);
}
.chev {
  color: var(--k2-text-muted);
  font-size: 12px;
}
</style>
