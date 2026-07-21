<template>
  <div class="login-root">
    <div class="bg">
      <div class="orb o1" />
      <div class="orb o2" />
      <div class="orb o3" />
      <div class="grid" />
    </div>

    <div class="login-shell">
      <section class="brand-panel">
        <div class="brand-inner">
          <div class="logo-row">
            <div class="logo-mark">
              <svg viewBox="0 0 40 40" width="28" height="28" fill="none">
                <path d="M8 22c5-12 19-14 24-5 1 3 1 6-1 8-5 5-15 6-21 1" stroke="url(#lg)" stroke-width="2.6" stroke-linecap="round"/>
                <circle cx="28" cy="14" r="3.5" fill="url(#lg)"/>
                <defs>
                  <linearGradient id="lg" x1="8" y1="10" x2="32" y2="30">
                    <stop stop-color="#4f46e5"/><stop offset="1" stop-color="#0891b2"/>
                  </linearGradient>
                </defs>
              </svg>
            </div>
            <span class="logo-name">K2Board</span>
          </div>
          <h1>下一代<br />代理面板体验</h1>
          <p>高性能 · 多协议 · 与 XrayR4u UniProxy 深度兼容。用极简运维掌控全局节点与用户体系。</p>
          <div class="feature-row">
            <span>VLESS / AnyTLS</span>
            <span>实时流量</span>
            <span>权限组调度</span>
          </div>
        </div>
      </section>

      <section class="form-panel">
        <div class="form-card">
          <div class="form-head">
            <h2>欢迎回来</h2>
            <p>登录管理员控制台</p>
          </div>
          <el-form ref="formRef" :model="form" :rules="rules" @keyup.enter="handleLogin" class="form">
            <el-form-item prop="email">
              <el-input
                v-model="form.email"
                placeholder="管理员邮箱"
                size="large"
                :prefix-icon="User"
                clearable
              />
            </el-form-item>
            <el-form-item prop="password">
              <el-input
                v-model="form.password"
                type="password"
                placeholder="密码"
                size="large"
                :prefix-icon="Lock"
                show-password
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" size="large" class="submit" :loading="loading" @click="handleLogin">
                进入控制台
              </el-button>
            </el-form-item>
          </el-form>
          <div class="form-foot">K2Board Aurora · Secure Admin</div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { User, Lock } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ email: '', password: '' })
const rules: FormRules = {
  email: [{ required: true, message: '请输入邮箱', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleLogin() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  const ok = await auth.login(form)
  loading.value = false
  if (ok) {
    ElMessage.success('登录成功')
    router.push('/')
  }
  // failures already toast via request interceptor
}
</script>

<style scoped>
.login-root {
  min-height: 100vh;
  min-height: 100dvh;
  position: relative;
  overflow-x: hidden;
  overflow-y: auto;
  background: var(--k2-bg);
  font-family: var(--k2-font);
  width: 100%;
}
.bg {
  position: absolute;
  inset: 0;
  overflow: hidden;
}
.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.35;
}
.o1 {
  width: 520px;
  height: 520px;
  background: #a5b4fc;
  top: -160px;
  right: 10%;
  animation: drift 14s ease-in-out infinite;
}
.o2 {
  width: 420px;
  height: 420px;
  background: #67e8f9;
  bottom: -140px;
  left: -80px;
  animation: drift 12s ease-in-out infinite reverse;
}
.o3 {
  width: 280px;
  height: 280px;
  background: #c4b5fd;
  top: 45%;
  left: 40%;
  opacity: 0.28;
  animation: drift 16s ease-in-out infinite;
}
.grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(15, 23, 42, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(15, 23, 42, 0.03) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(ellipse at center, black 20%, transparent 75%);
}
@keyframes drift {
  0%, 100% { transform: translate(0, 0) scale(1); }
  50% { transform: translate(24px, -18px) scale(1.05); }
}

.login-shell {
  position: relative;
  z-index: 1;
  min-height: 100vh;
  min-height: 100dvh;
  display: grid;
  grid-template-columns: 1.1fr 0.9fr;
  max-width: 1180px;
  width: 100%;
  margin: 0 auto;
  padding: 40px 28px;
  padding-bottom: max(40px, env(safe-area-inset-bottom));
  align-items: center;
  gap: 32px;
  box-sizing: border-box;
}

.brand-panel {
  color: var(--k2-text);
  padding: 24px;
}
.logo-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 36px;
}
.logo-mark {
  width: 48px;
  height: 48px;
  border-radius: 14px;
  display: grid;
  place-items: center;
  background: var(--k2-primary-soft);
  border: 1px solid #c7d2fe;
  box-shadow: 0 8px 24px rgba(79, 70, 229, 0.15);
}
.logo-name {
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0.04em;
  color: var(--k2-text);
}
.brand-panel h1 {
  margin: 0 0 16px;
  font-size: clamp(32px, 5vw, 48px);
  font-weight: 800;
  line-height: 1.15;
  letter-spacing: -0.03em;
  background: linear-gradient(120deg, #0f172a 10%, #4f46e5 55%, #0891b2 100%);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.brand-panel p {
  margin: 0 0 28px;
  max-width: 420px;
  font-size: 15px;
  line-height: 1.7;
  color: var(--k2-text-secondary);
}
.feature-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.feature-row span {
  font-size: 12px;
  font-weight: 600;
  color: var(--k2-primary);
  padding: 7px 14px;
  border-radius: 999px;
  border: 1px solid #c7d2fe;
  background: var(--k2-primary-soft);
}
.feature-row span:nth-child(2) {
  color: #0891b2;
  border-color: #a5f3fc;
  background: #ecfeff;
}
.feature-row span:nth-child(3) {
  color: #7c3aed;
  border-color: #e9d5ff;
  background: #f3e8ff;
}

.form-panel {
  display: flex;
  justify-content: center;
}
.form-card {
  width: 100%;
  max-width: 420px;
  background: #fff;
  border-radius: 20px;
  padding: 40px 36px 28px;
  box-shadow: var(--k2-shadow);
  border: 1px solid var(--k2-border);
}
.form-head {
  margin-bottom: 28px;
}
.form-head h2 {
  margin: 0;
  font-size: 28px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: #0f172a;
}
.form-head p {
  margin: 8px 0 0;
  color: #94a3b8;
  font-size: 14px;
}
.submit {
  width: 100%;
  height: 48px !important;
  font-size: 15px !important;
  letter-spacing: 0.04em;
  border-radius: 12px !important;
}
.form-foot {
  text-align: center;
  margin-top: 8px;
  font-size: 12px;
  color: #94a3b8;
  letter-spacing: 0.04em;
}

@media (max-width: 900px) {
  .login-shell {
    grid-template-columns: 1fr;
    padding: 20px 16px 32px;
    align-content: start;
    gap: 16px;
  }
  .brand-panel {
    text-align: center;
    padding: 8px 4px 0;
  }
  .brand-panel h1 {
    font-size: clamp(26px, 7vw, 34px);
  }
  .logo-row {
    justify-content: center;
    margin-bottom: 14px;
  }
  .brand-panel p {
    margin-left: auto;
    margin-right: auto;
    font-size: 14px;
    margin-bottom: 16px;
  }
  .feature-row {
    justify-content: center;
  }
  .form-panel {
    width: 100%;
  }
  .form-card {
    padding: 28px 20px 22px;
    max-width: 100%;
    border-radius: 16px;
  }
  .form-head h2 {
    font-size: 22px;
  }
  .orb {
    opacity: 0.22;
    animation: none;
  }
}
@media (max-width: 480px) {
  .feature-row span:nth-child(3) {
    display: none;
  }
  .brand-panel p {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
}
@media (prefers-reduced-motion: reduce) {
  .orb {
    animation: none;
  }
}
</style>
