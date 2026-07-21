<template>
  <div class="page" v-loading="store.loading">
    <div class="page-head">
      <div>
        <h2>个人中心</h2>
        <p>账号资料与安全设置</p>
      </div>
    </div>

    <div class="layout">
      <section class="glass-panel card profile-card">
        <div class="profile-hero">
          <div class="avatar">{{ (store.info?.email || 'U').charAt(0).toUpperCase() }}</div>
          <div>
            <div class="email">{{ store.info?.email }}</div>
            <div class="meta">
              {{ profileGroup }} · {{ store.info?.expire_text || '未开通' }}
            </div>
          </div>
        </div>
        <div class="info-grid">
          <div class="info-item">
            <span class="k">邮箱</span>
            <span class="v">{{ store.info?.email }}</span>
          </div>
          <div class="info-item">
            <span class="k">套餐组</span>
            <span class="v">{{ profileGroup }}</span>
          </div>
          <div class="info-item">
            <span class="k">到期</span>
            <span class="v" :class="{ exp: store.info?.expired }">{{ store.info?.expire_text || '永久有效' }}</span>
          </div>
        </div>
      </section>

      <section class="glass-panel card security-card">
        <h3>修改密码</h3>
        <p class="desc">修改后需要重新登录</p>
        <el-form ref="formRef" :model="form" :rules="rules" class="form" label-width="0">
          <el-form-item prop="oldPassword">
            <el-input v-model="form.oldPassword" type="password" show-password placeholder="原密码" size="large" />
          </el-form-item>
          <el-form-item prop="newPassword">
            <el-input v-model="form.newPassword" type="password" show-password placeholder="新密码（至少6位）" size="large" />
          </el-form-item>
          <el-button type="primary" size="large" :loading="saving" @click="handleChange">保存新密码</el-button>
        </el-form>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import { changeUserPassword } from '@/api/userApi'

const store = useUserAuthStore()
const formRef = ref<FormInstance>()
const saving = ref(false)

const profileGroup = computed(() => {
  const g = (store.info?.group_name || '').trim()
  if (!g || g === '-' || g === '—') return '未分组'
  return g
})
const form = reactive({ oldPassword: '', newPassword: '' })
const rules: FormRules = {
  oldPassword: [{ required: true, message: '请输入原密码' }],
  newPassword: [{ required: true, min: 6, message: '至少6位' }],
}

async function handleChange() {
  if (!formRef.value) return
  const v = await formRef.value.validate().catch(() => false)
  if (!v) return
  saving.value = true
  try {
    await changeUserPassword(store.token, form.oldPassword, form.newPassword)
    ElMessage.success('密码已修改，请重新登录')
    store.logout()
  } catch {
    ElMessage.error('修改失败')
  }
  saving.value = false
}

onMounted(() => {
  if (store.isLoggedIn) store.fetchInfo()
})
</script>

<style scoped>
.page {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.page-head h2 {
  margin: 0;
  font-size: clamp(22px, 2.4vw, 28px);
  font-weight: 800;
  color: var(--u-text);
  letter-spacing: -0.03em;
}
.page-head p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--u-text-3);
}

.layout {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
  align-items: start;
}

.glass-panel { background: var(--u-surface); border: 1px solid var(--u-border); border-radius: 16px; box-shadow: 0 1px 2px rgba(15,23,42,0.05); position: relative; overflow: hidden; }
.glass-panel::before { display: none; content: none; }
.glass-panel > * {
  position: relative;
  z-index: 1;
}

.card {
  padding: 24px 26px;
}
.profile-hero {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
  padding-bottom: 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.avatar {
  width: 64px;
  height: 64px;
  border-radius: 20px;
  display: grid;
  place-items: center;
  font-size: 24px;
  font-weight: 800;
  color: var(--u-text);
  background: var(--u-gradient);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.2) inset,
    0 14px 32px rgba(99, 102, 241, 0.45);
}
.email {
  font-size: 18px;
  font-weight: 800;
  color: var(--u-text);
}
.meta {
  margin-top: 4px;
  font-size: 12px;
  color: var(--u-text-3);
}
.info-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 10px;
}
.info-item {
  display: flex;
  gap: 16px;
  padding: 14px 16px;
  border-radius: 14px;
  background: var(--u-surface-2);
  border: 1px solid var(--u-border);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
}
.k {
  width: 56px;
  font-size: 12px;
  color: var(--u-text-3);
  font-weight: 600;
  flex-shrink: 0;
}
.v {
  font-size: 14px;
  color: var(--u-text-2);
  font-weight: 600;
  word-break: break-all;
}
.v.exp {
  color: #b91c1c;
}
h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text);
}
.desc {
  margin: 6px 0 16px;
  font-size: 12px;
  color: var(--u-text-3);
}
.form {
  max-width: 420px;
}

@media (min-width: 900px) {
  .layout {
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
  }
  .info-grid {
    grid-template-columns: 1fr;
  }
}
@media (min-width: 1100px) {
  .info-grid {
    grid-template-columns: 1fr 1fr 1fr;
  }
  .info-item {
    flex-direction: column;
    gap: 6px;
  }
  .k {
    width: auto;
  }
}
@media (max-width: 560px) {
  .card { padding: 18px; }
}
</style>
