import request from './request'

export interface User {
  id: number
  email: string
  uuid: string
  token: string
  group_id: number
  plan_id: number
  balance: number
  traffic_limit: number
  traffic_used: number
  speed_limit: number
  device_limit: number
  device_count: number
  online_ips: string[]
  last_active_at: string | null
  enable: boolean
  expire_at: number
  created_at: string
  updated_at: string
}

export interface UserListResult {
  list: User[]
  total: number
  page: number
  page_size: number
}

export interface CreateUserParams {
  email: string
  password: string
  group_id?: number
  plan_id?: number
  traffic_limit?: number
  speed_limit?: number
  device_limit?: number
  expire_at?: number
}

export interface UpdateUserParams {
  email?: string
  password?: string
  group_id?: number
  plan_id?: number
  traffic_limit?: number
  speed_limit?: number
  device_limit?: number
  enable?: boolean
  expire_at?: number
}

export function getUserList(params: {
  page: number
  page_size: number
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc' | string
}): Promise<{ data: UserListResult }> {
  return request.get('/admin/users', { params })
}

export function getUser(id: number): Promise<{ data: User }> {
  return request.get(`/admin/users/${id}`)
}

export function createUser(params: CreateUserParams): Promise<{ data: User }> {
  return request.post('/admin/users', params)
}

export function updateUser(id: number, params: UpdateUserParams): Promise<{ data: User }> {
  return request.put(`/admin/users/${id}`, params)
}

export function deleteUser(id: number): Promise<any> {
  return request.delete(`/admin/users/${id}`)
}

export function resetUserUUID(id: number): Promise<{ data: { uuid: string } }> {
  return request.post(`/admin/users/${id}/reset-uuid`)
}

export function resetUserToken(id: number): Promise<{ data: { token: string } }> {
  return request.post(`/admin/users/${id}/reset-token`)
}
