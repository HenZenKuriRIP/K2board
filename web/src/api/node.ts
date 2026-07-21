import request from './request'

export interface NodeToken {
  id: number
  node_id: number
  token: string
  created_at: string
}

export interface Node {
  id: number
  name: string
  group_id: number
  group_ids: number[]
  node_type: string
  host: string
  port: number
  network: string
  tls: number
  tls_type: string
  path: string
  sni: string
  service_name: string
  cipher: string
  flow: string
  speed_limit: number
  reality_settings: any
  vless_decryption?: string
  vless_encryption?: string
  status: string; cpu: number; mem: number; disk: number; uptime: number; active_conns: number
  online_count: number
  enable: boolean
  created_at: string
  updated_at: string
  node_tokens?: NodeToken[]
}

export interface CreateNodeParams {
  name: string
  group_id?: number
  group_ids?: number[]
  node_type: string
  cipher?: string
  host?: string
  port?: number
  network?: string
  tls?: number
  tls_type?: string
  path?: string
  sni?: string
  service_name?: string
  flow?: string
  speed_limit?: number
  reality_settings?: any
  vless_decryption?: string
  vless_encryption?: string
}

export interface UpdateNodeParams {
  name?: string
  group_id?: number
  group_ids?: number[]
  node_type?: string
  cipher?: string
  host?: string
  port?: number
  network?: string
  tls?: number
  tls_type?: string
  path?: string
  sni?: string
  service_name?: string
  flow?: string
  speed_limit?: number
  reality_settings?: any
  vless_decryption?: string
  vless_encryption?: string
  enable?: boolean
}

export function getNodeList(params?: { node_type?: string }): Promise<{ data: Node[] }> {
  return request.get('/admin/nodes', { params })
}

export function getNode(id: number): Promise<{ data: Node }> {
  return request.get(`/admin/nodes/${id}`)
}

export function createNode(params: CreateNodeParams): Promise<{ data: Node }> {
  return request.post('/admin/nodes', params)
}

export function updateNode(id: number, params: UpdateNodeParams): Promise<{ data: Node }> {
  return request.put(`/admin/nodes/${id}`, params)
}

export function deleteNode(id: number): Promise<any> {
  return request.delete(`/admin/nodes/${id}`)
}

export function generateNodeToken(id: number): Promise<{ data: NodeToken }> {
  return request.post(`/admin/nodes/${id}/token`)
}
