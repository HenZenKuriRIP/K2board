/**
 * Online monitoring snapshot — built only from existing admin APIs.
 * Does NOT introduce new backend endpoints.
 */
import { getUserList, type User } from './user'
import { getNodeList, type Node } from './node'
import request from './request'

export interface OnlineEndpoint {
  userId: number
  email: string
  ip: string
  nodeId?: number
  lastActiveAt: string | null
  deviceLimit: number
  deviceCount: number
  trafficUsed: number
  trafficLimit: number
  groupId: number
}

export interface OnlineSnapshot {
  fetchedAt: number
  onlineUserIds: number[]
  users: User[]
  nodes: Node[]
  endpoints: OnlineEndpoint[]
  totalOnlineUsers: number
  totalOnlineIPs: number
  nodeOnlineSum: number
}

export async function getOnlineUserIds(): Promise<number[]> {
  const res = await request.get('/admin/users/online')
  const data = res.data
  return Array.isArray(data) ? data : []
}

/** Pull online users + nodes using existing list endpoints only. */
export async function fetchOnlineSnapshot(): Promise<OnlineSnapshot> {
  const [onlineIds, nodesRes] = await Promise.all([
    getOnlineUserIds().catch(() => [] as number[]),
    getNodeList().catch(() => ({ data: [] as Node[] })),
  ])
  const onlineSet = new Set(onlineIds)
  const nodes = nodesRes.data || []

  // Scan user pages until we collect all online users (page_size max 100 per existing API)
  const found = new Map<number, User>()
  let page = 1
  const maxPages = 30
  while (page <= maxPages) {
    const res = await getUserList({ page, page_size: 100 })
    const list = res.data?.list || []
    for (const u of list) {
      const hasOnline = (u.online_ips && u.online_ips.length > 0) || onlineSet.has(u.id)
      if (hasOnline) found.set(u.id, u)
    }
    // Early stop if we have every id from /users/online and no more pages
    if (list.length < 100) break
    if (onlineSet.size > 0 && [...onlineSet].every(id => found.has(id))) break
    page++
  }

  // Also include online IDs that might have empty online_ips due to race
  for (const id of onlineSet) {
    if (!found.has(id)) {
      // minimal stub — detail missing
      found.set(id, {
        id,
        email: `#${id}`,
        uuid: '',
        token: '',
        group_id: 0,
        plan_id: 0,
        balance: 0,
        traffic_limit: 0,
        traffic_used: 0,
        speed_limit: 0,
        device_limit: 0,
        device_count: 0,
        online_ips: [],
        last_active_at: null,
        enable: true,
        expire_at: 0,
        created_at: '',
        updated_at: '',
      })
    }
  }

  const users = [...found.values()]
  const endpoints: OnlineEndpoint[] = []
  const ipSet = new Set<string>()

  for (const u of users) {
    const ips = u.online_ips?.length ? u.online_ips : []
    if (ips.length === 0 && onlineSet.has(u.id)) {
      // online id without IP detail — still count user
      continue
    }
    for (const ip of ips) {
      ipSet.add(ip)
      endpoints.push({
        userId: u.id,
        email: u.email,
        ip,
        lastActiveAt: u.last_active_at,
        deviceLimit: u.device_limit,
        deviceCount: u.device_count || ips.length,
        trafficUsed: u.traffic_used,
        trafficLimit: u.traffic_limit,
        groupId: u.group_id,
      })
    }
  }

  const nodeOnlineSum = nodes.reduce((s, n) => s + (n.online_count || 0), 0)

  return {
    fetchedAt: Date.now(),
    onlineUserIds: onlineIds,
    users,
    nodes,
    endpoints,
    totalOnlineUsers: onlineSet.size || users.filter(u => u.online_ips?.length).length,
    totalOnlineIPs: ipSet.size,
    nodeOnlineSum,
  }
}
