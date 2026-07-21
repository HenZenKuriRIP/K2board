<template>
  <div class="auth-page">
    <AuthScene />

    <main class="stage stage-card-only">
      <section class="panel">
        <div class="card card-tall">
          <div class="card-head">
            <div class="card-logo" aria-hidden="true">
              <AuthLogoMark />
            </div>
            <div class="card-titles">
              <h2>创建账户</h2>
              <p>填写邮箱与密码完成注册</p>
            </div>
          </div>

          <el-form ref="ff" :model="m" :rules="r" class="form" @submit.prevent="go">
            <el-form-item prop="email">
              <label class="field" style="width: 100%">
                <span class="label">邮箱</span>
                <div class="input-wrap">
                  <span class="input-ico" aria-hidden="true">✉</span>
                  <el-input v-model="m.email" placeholder="name@example.com" size="large" />
                </div>
              </label>
            </el-form-item>

            <el-form-item prop="code">
              <label class="field" style="width: 100%">
                <span class="label">验证码</span>
                <div class="code-row">
                  <div class="input-wrap" style="flex: 1; min-width: 0">
                    <span class="input-ico" aria-hidden="true">#</span>
                    <el-input v-model="m.code" placeholder="6 位验证码" size="large" maxlength="6" />
                  </div>
                  <el-button class="code-btn" size="large" :loading="sd" :disabled="cd > 0" @click="send">
                    {{ cd > 0 ? cd + 's' : '发送验证码' }}
                  </el-button>
                </div>
              </label>
            </el-form-item>

            <el-form-item prop="password">
              <label class="field" style="width: 100%">
                <span class="label">密码</span>
                <div class="input-wrap">
                  <span class="input-ico" aria-hidden="true">🔒</span>
                  <el-input
                    v-model="m.password"
                    type="password"
                    show-password
                    placeholder="至少 6 位"
                    size="large"
                  />
                </div>
              </label>
            </el-form-item>

            <el-form-item prop="confirm">
              <label class="field" style="width: 100%">
                <span class="label">确认密码</span>
                <div class="input-wrap">
                  <span class="input-ico" aria-hidden="true">🔒</span>
                  <el-input
                    v-model="m.confirm"
                    type="password"
                    show-password
                    placeholder="再次输入密码"
                    size="large"
                  />
                </div>
              </label>
            </el-form-item>

            <el-form-item prop="invite_code">
              <label class="field" style="width: 100%">
                <span class="label">邀请码（可选）</span>
                <div class="input-wrap">
                  <span class="input-ico" aria-hidden="true">↗</span>
                  <el-input
                    v-model="m.invite_code"
                    placeholder="有邀请码可填写"
                    size="large"
                    maxlength="16"
                  />
                </div>
              </label>
            </el-form-item>

            <button class="submit" type="button" :disabled="sb" @click="go">
              {{ sb ? '注册中…' : '注 册' }}
            </button>
          </el-form>

          <div class="divider"><span>或</span></div>

          <p class="foot">
            已有账号？
            <router-link to="/user/login">立即登录</router-link>
          </p>
        </div>
      </section>
    </main>

    <footer class="bottom">
      <span>© {{ year }}</span>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { userRegister, sendVerificationCode } from '@/api/userApi'
import { useUserAuthStore } from '@/stores/userAuth'
import AuthScene from '@/components/AuthScene.vue'
import AuthLogoMark from '@/components/AuthLogoMark.vue'
import '@/styles/auth.css'

const router = useRouter()
const route = useRoute()
const store = useUserAuthStore()
const ff = ref<FormInstance>()
const sb = ref(false)
const sd = ref(false)
const cd = ref(0)
const year = new Date().getFullYear()
const m = reactive({ email: '', code: '', password: '', confirm: '', invite_code: '' })

onMounted(() => {
  const inv = String(route.query.invite || route.query.invite_code || '').trim()
  if (inv) m.invite_code = inv.toUpperCase()
})

const r: FormRules = {
  email: [{ required: true, type: 'email', message: '请输入有效邮箱' }],
  code: [{ required: true, len: 6, message: '6位验证码' }],
  password: [{ required: true, min: 6, message: '至少6位' }],
  confirm: [
    { required: true, message: '请再次输入密码' },
    {
      validator: (_rule, v, cb) => {
        if (v !== m.password) cb(new Error('两次密码不一致'))
        else cb()
      },
    },
  ],
}
let t: ReturnType<typeof setInterval> | null = null

async function send() {
  const email = m.email.trim().toLowerCase()
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    ElMessage.warning('请先输入有效邮箱')
    return
  }
  m.email = email
  sd.value = true
  try {
    const res: any = await sendVerificationCode(email)
    ElMessage.success(res?.message || '验证码已发送')
    cd.value = 60
    if (t) clearInterval(t)
    t = setInterval(() => {
      cd.value--
      if (cd.value <= 0 && t) clearInterval(t)
    }, 1000)
  } catch (e: any) {
    ElMessage.error(e?.message || '发送失败，请检查邮件服务是否已配置')
  }
  sd.value = false
}

async function go() {
  if (!ff.value) return
  const v = await ff.value.validate().catch(() => false)
  if (!v) return
  sb.value = true
  try {
    const email = m.email.trim().toLowerCase()
    const res = await userRegister(email, m.password, m.code.trim(), m.invite_code.trim())
    store.setSession(res.data.token, res.data.email)
    await store.fetchInfo()
    await store.refreshPendingCount()
    ElMessage.success('注册成功')
    router.replace('/user')
  } catch (e: any) {
    ElMessage.error(e?.message || '注册失败')
  }
  sb.value = false
}
</script>
