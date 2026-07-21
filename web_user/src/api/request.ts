import axios from 'axios'
import type { AxiosInstance, AxiosResponse } from 'axios'

/**
 * Resolve panel API origin for multi-domain (shadow) user portals.
 *
 * Priority:
 * 1. VITE_API_BASE at build time (e.g. https://www.example.com)
 * 2. window.__K2_API_BASE__ from /config.js (same dist, different shadow hosts)
 * 3. Empty → same-origin relative /api/v1 (when reverse-proxied on www)
 *
 * Subscribe links always come from backend subscribe_url/site_url (www) —
 * they are independent of this API base.
 */
function resolveApiBase(): string {
  const trim = (s: string) => s.replace(/\/+$/, '').trim()

  const fromEnv = trim(String(import.meta.env.VITE_API_BASE || ''))
  if (fromEnv) return fromEnv

  try {
    const w = (window as unknown as { __K2_API_BASE__?: string }).__K2_API_BASE__
    if (typeof w === 'string' && trim(w)) return trim(w)
  } catch {
    /* ignore */
  }

  return ''
}

const apiBase = resolveApiBase()

const request: AxiosInstance = axios.create({
  baseURL: apiBase + '/api/v1',
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
  // Shadow portal → www API is cross-origin; cookies not used (token in body/query).
  withCredentials: false,
})

function extractError(error: any): string {
  const data = error?.response?.data
  if (data?.message) return String(data.message)
  if (typeof data === 'string' && data.trim()) return data
  if (error?.message) return error.message
  return '网络错误'
}

function isLoginPage(): boolean {
  const h = window.location.hash || ''
  return h.includes('/user/login') || h.includes('/user/register')
}

request.interceptors.response.use(
  (response: AxiosResponse) => {
    const data = response.data
    if (data && typeof data === 'object' && data.code !== undefined && data.code !== 0) {
      return Promise.reject(new Error(data.message || '请求失败'))
    }
    return data
  },
  (error) => {
    const status = error?.response?.status
    const msg = extractError(error)
    if (status === 401 && !isLoginPage()) {
      localStorage.removeItem('user_token')
      localStorage.removeItem('user_email')
      window.location.hash = '#/user/login'
    }
    return Promise.reject(new Error(msg))
  },
)

export default request
export { resolveApiBase, apiBase }
