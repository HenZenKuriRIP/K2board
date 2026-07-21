/** Client-side IP geolocation with localStorage cache. No backend API changes. */

export interface GeoResult {
  ip: string
  lat: number
  lng: number
  city: string
  region: string
  country: string
  countryCode: string
  org: string
  private: boolean
}

const CACHE_KEY = 'k2_geoip_v1'
const CACHE_TTL_MS = 7 * 24 * 3600 * 1000

type CacheStore = Record<string, { t: number; g: GeoResult }>

function loadCache(): CacheStore {
  try {
    return JSON.parse(localStorage.getItem(CACHE_KEY) || '{}')
  } catch {
    return {}
  }
}

function saveCache(c: CacheStore) {
  try {
    // keep last 500 entries
    const keys = Object.keys(c)
    if (keys.length > 500) {
      keys
        .sort((a, b) => (c[a].t || 0) - (c[b].t || 0))
        .slice(0, keys.length - 500)
        .forEach(k => delete c[k])
    }
    localStorage.setItem(CACHE_KEY, JSON.stringify(c))
  } catch { /* quota */ }
}

export function isPrivateIP(ip: string): boolean {
  if (!ip) return true
  if (ip === '127.0.0.1' || ip === '::1' || ip === 'localhost') return true
  if (ip.startsWith('10.')) return true
  if (ip.startsWith('192.168.')) return true
  if (ip.startsWith('169.254.')) return true
  const m = ip.match(/^172\.(\d+)\./)
  if (m) {
    const n = parseInt(m[1], 10)
    if (n >= 16 && n <= 31) return true
  }
  // IPv6 ULA / link-local rough
  if (ip.startsWith('fc') || ip.startsWith('fd') || ip.startsWith('fe80')) return true
  return false
}

function privateGeo(ip: string): GeoResult {
  // Scatter private endpoints near a fixed “lab” locus so they still show on globe
  const h = [...ip].reduce((a, c) => a + c.charCodeAt(0), 0)
  return {
    ip,
    lat: 20 + (h % 40) - 20,
    lng: 100 + (h % 60) - 30,
    city: '内网',
    region: 'LAN',
    country: 'Private Network',
    countryCode: 'LAN',
    org: 'RFC1918',
    private: true,
  }
}

async function fetchGeoRemote(ip: string): Promise<GeoResult | null> {
  // ipwho.is — HTTPS + CORS friendly for browser use
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), 6000)
  try {
    const res = await fetch(`https://ipwho.is/${encodeURIComponent(ip)}`, {
      signal: ctrl.signal,
    })
    if (!res.ok) return null
    const j = await res.json()
    if (!j.success && j.success !== undefined) return null
    if (typeof j.latitude !== 'number' || typeof j.longitude !== 'number') return null
    return {
      ip,
      lat: j.latitude,
      lng: j.longitude,
      city: j.city || '',
      region: j.region || j.region_code || '',
      country: j.country || '',
      countryCode: j.country_code || '',
      org: j.connection?.org || j.connection?.isp || j.org || '',
      private: false,
    }
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

/** Resolve one IP; uses cache; private IPs never hit network. */
export async function resolveIP(ip: string): Promise<GeoResult> {
  const clean = (ip || '').trim()
  if (!clean) return privateGeo('0.0.0.0')
  if (isPrivateIP(clean)) return privateGeo(clean)

  const cache = loadCache()
  const hit = cache[clean]
  if (hit && Date.now() - hit.t < CACHE_TTL_MS) return hit.g

  const remote = await fetchGeoRemote(clean)
  if (remote) {
    cache[clean] = { t: Date.now(), g: remote }
    saveCache(cache)
    return remote
  }

  // soft fallback so UI still works offline
  return {
    ip: clean,
    lat: 0,
    lng: 0,
    city: '未知',
    region: '',
    country: 'Unknown',
    countryCode: '??',
    org: '',
    private: false,
  }
}

/** Resolve many IPs with concurrency limit. */
export async function resolveIPs(ips: string[], concurrency = 4): Promise<Map<string, GeoResult>> {
  const uniq = [...new Set(ips.filter(Boolean))]
  const out = new Map<string, GeoResult>()
  let i = 0
  async function worker() {
    while (i < uniq.length) {
      const idx = i++
      const ip = uniq[idx]
      out.set(ip, await resolveIP(ip))
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, uniq.length) }, () => worker()))
  return out
}

/** Host may be domain or IP — only resolve when it looks like IPv4. */
export function maybeIP(host: string): string | null {
  if (!host) return null
  // strip brackets / port
  let h = host.replace(/^\[|\]$/g, '')
  h = h.split(':')[0]
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(h)) return h
  return null
}
