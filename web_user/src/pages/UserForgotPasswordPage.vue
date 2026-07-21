<template>
  <div class="auth-page">
    <AuthScene />

    <main class="stage stage-card-only">
      <section class="panel">
        <div class="card">
          <div class="card-head">
            <div class="card-logo" aria-hidden="true">
              <AuthLogoMark />
            </div>
            <div class="card-titles">
              <h2>重置密码</h2>
              <p>输入邮箱获取验证码</p>
            </div>
          </div>

          <el-form ref="ff" :model="m" :rules="r" class="form" @submit.prevent="go">
            <el-form-item prop="email">
              <label class="field" style="width: 100%">
                <span class="label">注册邮箱</span>
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
                <span class="label">新密码</span>
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
                <span class="label">确认新密码</span>
                <div class="input-wrap">
                  <span class="input-ico" aria-hidden="true">🔒</span>
                  <el-input
                    v-model="m.confirm"
                    type="password"
                    show-password
                    placeholder="再次输入新密码"
                    size="large"
                  />
                </div>
              </label>
            </el-form-item>

            <button class="submit" type="button" :disabled="sb" @click="go">
              {{ sb ? '提交中…' : '重置密码' }}
            </button>
          </el-form>

          <div class="divider"><span>或</span></div>

          <p class="foot">
            <router-link to="/user/login">返回登录</router-link>
            <span class="sep">·</span>
            <router-link to="/user/register">注册账号</router-link>
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
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { sendResetPasswordCode, resetPassword } from '@/api/userApi'
import AuthScene from '@/components/AuthScene.vue'
import AuthLogoMark from '@/components/AuthLogoMark.vue'
import '@/styles/auth.css'

const router = useRouter()
const ff = ref<FormInstance>()
const sb = ref(false)
const sd = ref(false)
const cd = ref(0)
const year = new Date().getFullYear()
const m = reactive({ email: '', code: '', password: '', confirm: '' })
const r: FormRules = {
  email: [{ required: true, type: 'email', message: '请输入有效邮箱' }],
  code: [{ required: true, len: 6, message: '6位验证码' }],
  password: [{ required: true, min: 6, message: '至少6位' }],
  confirm: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (_r, v, cb) => {
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
    const res: any = await sendResetPasswordCode(email)
    ElMessage.success(res?.message || '若邮箱已注册，验证码将发送至邮箱')
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
    const res: any = await resetPassword(email, m.code.trim(), m.password)
    ElMessage.success(res?.message || '密码已重置，请登录')
    router.replace('/user/login')
  } catch (e: any) {
    ElMessage.error(e?.message || '重置失败')
  }
  sb.value = false
}
</script>
