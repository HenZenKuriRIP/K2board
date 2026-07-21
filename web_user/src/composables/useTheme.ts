import { ref, computed, watch } from 'vue'

export type UserTheme = 'dark' | 'light'

const STORAGE_KEY = 'k2_user_theme'
const theme = ref<UserTheme>('light')
let inited = false

function readStored(): UserTheme {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* ignore */
  }
  return 'light'
}

/** Apply theme class/attrs on <html> for CSS vars + Element Plus dark. */
export function applyTheme(mode: UserTheme) {
  const root = document.documentElement
  root.setAttribute('data-theme', mode)
  root.classList.toggle('dark', mode === 'dark')
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    /* ignore */
  }
}

/** Call once before app mount to avoid light flash. */
export function initTheme() {
  if (inited) return theme.value
  theme.value = readStored()
  applyTheme(theme.value)
  inited = true
  return theme.value
}

export function useTheme() {
  if (!inited && typeof document !== 'undefined') {
    initTheme()
  }

  const isDark = computed(() => theme.value === 'dark')
  const isLight = computed(() => theme.value === 'light')

  function setTheme(mode: UserTheme) {
    theme.value = mode
    applyTheme(mode)
  }

  function toggleTheme() {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  watch(theme, (m) => applyTheme(m))

  return {
    theme,
    isDark,
    isLight,
    setTheme,
    toggleTheme,
  }
}
