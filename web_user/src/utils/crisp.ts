/**
 * Crisp live chat — ONLY after user is logged in.
 * Native bottom-right launcher is fully suppressed; app uses a custom draggable FAB.
 * Do NOT call loadCrisp() on public auth pages (login/register/forgot).
 */

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type CrispQueue = any[] & { push: (...args: any[]) => number | void }

declare global {
  interface Window {
    $crisp?: CrispQueue
    CRISP_WEBSITE_ID?: string
  }
}

const SCRIPT_ID = 'crisp-chat-loader'
const HIDE_STYLE_ID = 'k2-crisp-hide-native-launcher'

let hooksInstalled = false
/** Tracks open state so custom FAB can toggle close. */
let chatIsOpen = false

/**
 * Resolve Crisp Website ID for THIS deployment only.
 * Never hardcode a personal account in the repo — open-source clones would all land on one inbox.
 *
 * Priority:
 * 1. window.__K2_CRISP_WEBSITE_ID__ from /config.js (runtime, per host)
 * 2. VITE_CRISP_WEBSITE_ID at build time (.env.production local, not committed)
 * 3. empty → Crisp disabled (no script, no FAB side effects beyond open no-op)
 */
function websiteId(): string {
  try {
    const w = (window as unknown as { __K2_CRISP_WEBSITE_ID__?: string }).__K2_CRISP_WEBSITE_ID__
    if (typeof w === 'string' && w.trim()) return w.trim()
  } catch {
    /* ignore */
  }
  const fromEnv = (import.meta.env.VITE_CRISP_WEBSITE_ID as string | undefined)?.trim()
  return fromEnv || ''
}

/** True when this deployment has a Crisp Website ID configured. */
export function isCrispConfigured(): boolean {
  return websiteId().length > 0
}

function q(): CrispQueue | undefined {
  return typeof window !== 'undefined' ? window.$crisp : undefined
}

/**
 * Aggressive CSS: hide Crisp floating launcher only (chat panel when open still works).
 * Selectors cover multiple Crisp client versions; chat:hide is the primary control.
 */
function injectHideNativeLauncher(): void {
  if (typeof document === 'undefined') return
  let style = document.getElementById(HIDE_STYLE_ID) as HTMLStyleElement | null
  if (!style) {
    style = document.createElement('style')
    style.id = HIDE_STYLE_ID
    document.head.appendChild(style)
  }
  // Re-write every time so hot reload / re-login always gets latest rules
  style.textContent = `
    /* ── K2: hide official floating bubble only ── */
    html.k2-crisp-launcher-off #crisp-chatbox > div > a[role="button"],
    html.k2-crisp-launcher-off #crisp-chatbox > div > a,
    html.k2-crisp-launcher-off #crisp-chatbox a[data-maximized],
    html.k2-crisp-launcher-off .crisp-client .cc-1yy0g,
    html.k2-crisp-launcher-off .crisp-client .cc-1brb6,
    html.k2-crisp-launcher-off .crisp-client .cc-kxkl,
    html.k2-crisp-launcher-off .crisp-client .cc-tlyw,
    html.k2-crisp-launcher-off .crisp-client .cc-nsge,
    html.k2-crisp-launcher-off .crisp-client .cc-1m2mf,
    html.k2-crisp-launcher-off .crisp-client .cc-unoo,
    html.k2-crisp-launcher-off #crisp-chatbox-button,
    html.k2-crisp-launcher-off [id^="crisp-chatbox"] > div > a[role="button"] {
      display: none !important;
      visibility: hidden !important;
      opacity: 0 !important;
      pointer-events: none !important;
      transform: scale(0) !important;
      width: 0 !important;
      height: 0 !important;
      max-width: 0 !important;
      max-height: 0 !important;
      min-width: 0 !important;
      min-height: 0 !important;
      margin: 0 !important;
      padding: 0 !important;
      overflow: hidden !important;
      clip: rect(0, 0, 0, 0) !important;
      position: absolute !important;
      left: -9999px !important;
    }
  `
  document.documentElement.classList.add('k2-crisp-launcher-off')
}

/** SDK: hide entire widget when chat is not open (removes official ball). */
function hideNativeWidget(): void {
  try {
    q()?.push(['do', 'chat:hide'])
  } catch {
    /* ignore */
  }
  injectHideNativeLauncher()
  // Extra DOM pass: zero-size any leftover launcher anchors
  try {
    document.querySelectorAll('#crisp-chatbox > div > a, #crisp-chatbox-button').forEach((el) => {
      const node = el as HTMLElement
      // Don't hide when chat panel is open (opened state uses different structure)
      const box = document.getElementById('crisp-chatbox')
      const open =
        box?.getAttribute('data-visible') === 'true' ||
        box?.classList.contains('crisp--opened') ||
        !!document.querySelector('.crisp-client .cc-1m2mf[data-visible="true"]')
      if (open) return
      node.style.setProperty('display', 'none', 'important')
      node.style.setProperty('visibility', 'hidden', 'important')
      node.style.setProperty('pointer-events', 'none', 'important')
    })
  } catch {
    /* ignore */
  }
}

function installCrispHooks(): void {
  if (hooksInstalled) return
  hooksInstalled = true
  const crisp = q()
  if (!crisp) return

  // Queue works even before l.js finishes loading
  try {
    crisp.push([
      'on',
      'session:loaded',
      () => {
        chatIsOpen = false
        hideNativeWidget()
      },
    ])
    crisp.push([
      'on',
      'chat:opened',
      () => {
        chatIsOpen = true
        // Keep official ball CSS-hidden while panel is open
        injectHideNativeLauncher()
      },
    ])
    crisp.push([
      'on',
      'chat:closed',
      () => {
        chatIsOpen = false
        // After user closes chat, suppress official ball again
        hideNativeWidget()
      },
    ])
    // Some builds fire this when minimizing
    crisp.push([
      'on',
      'chat:hidden',
      () => {
        chatIsOpen = false
        injectHideNativeLauncher()
        hideNativeWidget()
      },
    ])
  } catch {
    /* ignore */
  }
}

/**
 * Load Crisp SDK. Call only when the user is authenticated.
 * Default launcher is suppressed — use openCrisp() from custom UI.
 */
export function loadCrisp(): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return

  const id = websiteId()
  if (!id) {
    // No ID configured for this deploy — do not load Crisp at all
    return
  }

  window.$crisp = window.$crisp || ([] as unknown as CrispQueue)
  window.CRISP_WEBSITE_ID = id

  injectHideNativeLauncher()
  installCrispHooks()

  // If already loaded from a previous visit in-session, re-hide
  if (document.getElementById(SCRIPT_ID) || document.querySelector('script[src="https://client.crisp.chat/l.js"]')) {
    hideNativeWidget()
    return
  }

  const d = document
  const s = d.createElement('script')
  s.id = SCRIPT_ID
  s.src = 'https://client.crisp.chat/l.js'
  s.async = true
  s.onload = () => {
    // Script ready — hide as soon as possible (session:loaded may lag)
    setTimeout(hideNativeWidget, 50)
    setTimeout(hideNativeWidget, 400)
    setTimeout(hideNativeWidget, 1200)
  }
  d.getElementsByTagName('head')[0].appendChild(s)
}

/**
 * Attach identity after login. Also ensures Crisp is loaded (logged-in only).
 */
export function identifyCrisp(user: {
  email?: string
  id?: number
  plan_name?: string
  group_name?: string
  expire_text?: string
}): void {
  if (typeof window === 'undefined') return
  loadCrisp()
  const crisp = q()
  if (!crisp) return

  if (user.email) {
    crisp.push(['set', 'user:email', [user.email]])
    const nick = user.email.split('@')[0]
    if (nick) crisp.push(['set', 'user:nickname', [nick]])
  }

  if (user.id != null && user.id > 0) {
    crisp.push([
      'set',
      'session:data',
      [
        [
          ['user_id', String(user.id)],
          ['plan', user.plan_name || ''],
          ['group', user.group_name || ''],
          ['expire', user.expire_text || ''],
        ],
      ],
    ])
  }
}

/**
 * Fully tear down Crisp on logout so the bubble does not linger on login pages.
 */
export function unloadCrisp(): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return

  hooksInstalled = false
  chatIsOpen = false

  try {
    window.$crisp?.push(['do', 'chat:close'])
    window.$crisp?.push(['do', 'chat:hide'])
    window.$crisp?.push(['do', 'session:reset'])
  } catch {
    /* ignore */
  }

  document.querySelectorAll('.crisp-client, #crisp-chatbox').forEach((el) => el.remove())
  document.getElementById(SCRIPT_ID)?.remove()
  document.querySelectorAll('script[src="https://client.crisp.chat/l.js"]').forEach((el) => el.remove())
  document.getElementById(HIDE_STYLE_ID)?.remove()
  document.documentElement.classList.remove('k2-crisp-launcher-off')

  try {
    delete window.$crisp
    delete window.CRISP_WEBSITE_ID
  } catch {
    window.$crisp = undefined
    window.CRISP_WEBSITE_ID = undefined
  }
}

/** @deprecated use unloadCrisp — kept for call sites */
export function resetCrisp(): void {
  unloadCrisp()
}

/** Open chat from custom FAB. Shows widget only for the open panel. */
export function openCrisp(): void {
  loadCrisp()
  try {
    // Must show before open; otherwise chat:hide keeps it invisible
    q()?.push(['do', 'chat:show'])
    q()?.push(['do', 'chat:open'])
  } catch {
    /* ignore */
  }
  // After open, keep CSS class so if a launcher flashes it stays hidden
  injectHideNativeLauncher()
}

/** Close chat and re-hide official launcher (custom FAB remains). */
export function closeCrisp(): void {
  try {
    q()?.push(['do', 'chat:close'])
  } catch {
    /* ignore */
  }
  chatIsOpen = false
  hideNativeWidget()
}

/** Tap FAB: open if closed, close if open. */
export function toggleCrisp(): void {
  if (chatIsOpen) {
    closeCrisp()
  } else {
    openCrisp()
  }
}
