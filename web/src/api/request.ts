import axios from 'axios'
import { ElMessage } from 'element-plus'
import type { AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios'

const request: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 20000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor: attach JWT token
request.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

function extractErrorMessage(error: any): string {
  const data = error?.response?.data
  if (data) {
    if (typeof data === 'string' && data.trim()) return data
    if (data.message) return String(data.message)
    if (data.error) return String(data.error)
  }
  if (error?.message) return error.message
  return '网络错误'
}

function isAuthPage(): boolean {
  const hash = window.location.hash || ''
  const path = window.location.pathname || ''
  return hash.includes('/login') || path.endsWith('/login')
}

// Response interceptor: unwrap business envelope + surface real errors
request.interceptors.response.use(
  (response: AxiosResponse) => {
    const data = response.data
    // Standard K2Board envelope: { code, message, data }
    if (data && typeof data === 'object' && data.code !== undefined && data.code !== 0) {
      const msg = data.message || '请求失败'
      ElMessage.error(msg)
      return Promise.reject(new Error(msg))
    }
    return data
  },
  (error) => {
    const status = error.response?.status
    const msg = extractErrorMessage(error)

    // Expired/invalid session: only force logout outside login page
    if (status === 401 && !isAuthPage()) {
      localStorage.removeItem('token')
      localStorage.removeItem('email')
      ElMessage.error(msg || '登录已过期，请重新登录')
      // Hash-mode admin path
      const base = window.location.pathname.replace(/\/?$/, '/')
      window.location.href = `${base}#/login`
      return Promise.reject(error)
    }

    ElMessage.error(msg)
    return Promise.reject(error)
  },
)

export default request
