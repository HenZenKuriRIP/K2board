import request from './request'

export interface LoginParams {
  email: string
  password: string
}

export interface LoginResult {
  token: string
  email: string
}

export interface HostHealth {
  hostname: string
  os: string
  arch: string
  go_version: string
  num_cpu: number
  goroutines: number
  uptime_sec: number
  alloc_bytes: number
  sys_bytes: number
  load1: number
  load5: number
  load15: number
  mem_total_bytes: number
  mem_used_bytes: number
  mem_used_pct: number
  disk_total_bytes: number
  disk_used_bytes: number
  disk_used_pct: number
  cpu_percent: number
  status: 'healthy' | 'warn' | 'critical' | string
  message: string
}

export interface DashboardStats {
  total_users: number
  active_users: number
  online_users: number
  total_nodes: number
  active_nodes: number
  total_upload: number
  total_download: number
  total_traffic_used: number
  panel_version?: string
  host?: HostHealth
}

export function login(params: LoginParams): Promise<{ data: LoginResult }> {
  return request.post('/admin/login', params)
}

export function getDashboard(): Promise<{ data: DashboardStats }> {
  return request.get('/admin/dashboard')
}
