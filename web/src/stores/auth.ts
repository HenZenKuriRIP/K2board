import { defineStore } from 'pinia'
import { ref } from 'vue'
import { login as loginApi, type LoginParams } from '@/api/admin'
import type { User } from '@/api/user'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const email = ref<string>(localStorage.getItem('email') || '')

  function isLoggedIn(): boolean {
    return !!token.value
  }

  async function login(params: LoginParams): Promise<boolean> {
    try {
      const res = await loginApi({
        email: params.email.trim().toLowerCase(),
        password: params.password,
      })
      const data = res.data
      if (!data?.token) {
        return false
      }
      token.value = data.token
      email.value = data.email
      localStorage.setItem('token', data.token)
      localStorage.setItem('email', data.email)
      return true
    } catch {
      return false
    }
  }

  function logout() {
    token.value = ''
    email.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('email')
    window.location.href = '/login'
  }

  return { token, email, isLoggedIn, login, logout }
})
