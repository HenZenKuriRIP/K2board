<template>
  <div class="auth-page">
    <AuthScene />
    <!-- 顶栏/中间文案已去掉，避免遮挡背景图 logo 与画面 -->

    <main class="stage stage-card-only">
      <section class="panel">
        <div class="card">
          <div class="card-head">
            <div class="card-logo" aria-hidden="true">
              <AuthLogoMark />
            </div>
            <div class="card-titles">
              <h2>欢迎回来</h2>
              <p>使用邮箱登录</p>
            </div>
          </div>

          <form class="form" @submit.prevent="handleLogin">
            <label class="field">
              <span class="label">邮箱</span>
              <div class="input-wrap">
                <span class="input-ico" aria-hidden="true">✉</span>
                <el-input
                  v-model="email"
                  type="email"
                  autocomplete="username"
                  placeholder="name@example.com"
                  size="large"
                  clearable
                  @keyup.enter="handleLogin"
                />
              </div>
            </label>

            <label class="field">
              <span class="label">密码</span>
              <div class="input-wrap">
                <span class="input-ico" aria-hidden="true">🔒</span>
                <el-input
                  v-model="password"
                  type="password"
                  autocomplete="current-password"
                  show-password
                  placeholder="请输入密码"
                  size="large"
                  @keyup.enter="handleLogin"
                />
              </div>
            </label>

            <div class="form-row">
              <label class="remember">
                <input v-model="remember" type="checkbox" />
                <span>记住邮箱</span>
              </label>
              <router-link class="link-muted" to="/user/forgot-password">忘记密码？</router-link>
            </div>

            <button class="submit" type="submit" :disabled="loading">
              {{ loading ? '登录中…' : '登 录' }}
            </button>
          </form>

          <div class="divider"><span>或</span></div>

          <p class="foot">
            还没有账号？
            <router-link to="/user/register">立即注册</router-link>
          </p>
          <p class="card-note">登录即表示你同意服务条款与隐私政策</p>
        </div>
      </section>
    </main>

    <footer class="bottom">
      <span>© {{ year }}</span>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import AuthScene from '@/components/AuthScene.vue'
import AuthLogoMark from '@/components/AuthLogoMark.vue'
import '@/styles/auth.css'

const REMEMBER_KEY = 'k2_login_email'
const router = useRouter()
const store = useUserAuthStore()
const email = ref('')
const password = ref('')
const remember = ref(true)
const loading = ref(false)
const year = new Date().getFullYear()

onMounted(() => {
  try {
    const saved = localStorage.getItem(REMEMBER_KEY)
    if (saved) {
      email.value = saved
      remember.value = true
    }
  } catch {
    /* ignore */
  }
})

async function handleLogin() {
  const em = email.value.trim()
  const pw = password.value
  if (!em || !pw) {
    ElMessage.warning('请输入邮箱和密码')
    return
  }
  if (!em.includes('@')) {
    ElMessage.warning('请输入有效邮箱')
    return
  }
  if (pw.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  loading.value = true
  try {
    await store.login(em, pw)
    try {
      if (remember.value) localStorage.setItem(REMEMBER_KEY, em.toLowerCase())
      else localStorage.removeItem(REMEMBER_KEY)
    } catch {
      /* ignore */
    }
    router.replace('/user')
  } catch (e: any) {
    ElMessage.error(e?.message || '登录失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>
