import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { userLogin as loginApi, getUserInfo, listOrders, type UserInfo } from '@/api/userApi'
import { formatBytes } from '@/utils/format'
import { identifyCrisp, unloadCrisp } from '@/utils/crisp'

export const useUserAuthStore = defineStore('userAuth', () => {
  const token = ref<string>(localStorage.getItem('user_token') || '')
  const email = ref<string>(localStorage.getItem('user_email') || '')
  const info = ref<UserInfo | null>(null)
  const loading = ref(false)
  /** Pending orders badge for nav */
  const pendingOrderCount = ref(0)

  /** Reactive login flag — templates must use this (not a plain function) for auto-updates */
  const isLoggedIn = computed(() => !!token.value)

  function setSession(newToken: string, newEmail: string) {
    token.value = newToken
    email.value = newEmail
    localStorage.setItem('user_token', newToken)
    localStorage.setItem('user_email', newEmail)
  }

  function clearSession() {
    token.value = ''
    email.value = ''
    info.value = null
    pendingOrderCount.value = 0
    localStorage.removeItem('user_token')
    localStorage.removeItem('user_email')
  }

  async function login(emailVal: string, password: string): Promise<void> {
    // Drop stale session first so failed login cannot keep old token
    clearSession()
    const res = await loginApi(String(emailVal || '').trim(), password)
    const data = res.data
    if (!data?.token) {
      throw new Error('登录响应异常，请刷新后重试')
    }
    setSession(data.token, data.email)
    await fetchInfo()
    await refreshPendingCount()
  }

  function syncCrispIdentity(d: UserInfo | null) {
    if (!d) return
    identifyCrisp({
      email: d.email,
      id: d.id,
      plan_name: d.plan_name,
      group_name: d.group_name,
      expire_text: d.expire_text,
    })
  }

  async function fetchInfo() {
    if (!token.value) return
    loading.value = true
    try {
      const res = await getUserInfo(token.value)
      const d = res.data
      // Prepend origin if URL is relative
      if (d.subscribe_url && d.subscribe_url.startsWith('/')) {
        d.subscribe_url = window.location.origin + d.subscribe_url
      }
      info.value = d
      syncCrispIdentity(d)
    } catch {
      info.value = null
    }
    loading.value = false
  }

  async function refreshPendingCount() {
    if (!token.value) {
      pendingOrderCount.value = 0
      return
    }
    try {
      const r = await listOrders(token.value)
      const list = r.data || []
      pendingOrderCount.value = list.filter((o) => o.status === 'pending').length
    } catch {
      /* keep last value */
    }
  }

  function logout() {
    // Remove Crisp bubble before navigating to login
    unloadCrisp()
    clearSession()
    window.location.href = '/#/user/login'
  }

  function formatTraffic(bytes: number): string {
    return formatBytes(bytes)
  }

  return {
    token,
    email,
    info,
    loading,
    pendingOrderCount,
    isLoggedIn,
    setSession,
    clearSession,
    login,
    fetchInfo,
    refreshPendingCount,
    logout,
    formatTraffic,
  }
})
