import { ref, watch, onUnmounted, type Ref, type ComputedRef } from 'vue'
import { parseExpireMs, formatCountdown } from '@/utils/format'

/**
 * Live countdown for a pending order's expired_at.
 * remaining hits 0 → calls onExpire once (e.g. refresh order from API).
 */
export function useOrderCountdown(
  expiredAt: Ref<string | number | null | undefined> | ComputedRef<string | number | null | undefined>,
  options?: {
    /** Only run while status is pending */
    active?: Ref<boolean> | ComputedRef<boolean>
    onExpire?: () => void
  },
) {
  const remaining = ref(0)
  const label = ref('00:00')
  const expired = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null
  let firedExpire = false

  function tick() {
    const active = options?.active ? options.active.value : true
    if (!active) {
      remaining.value = 0
      label.value = '00:00'
      expired.value = false
      return
    }
    const end = parseExpireMs(expiredAt.value)
    if (!end) {
      remaining.value = 0
      label.value = '—'
      expired.value = false
      return
    }
    const sec = Math.max(0, Math.floor((end - Date.now()) / 1000))
    remaining.value = sec
    label.value = formatCountdown(sec)
    if (sec <= 0) {
      expired.value = true
      if (!firedExpire) {
        firedExpire = true
        options?.onExpire?.()
      }
    } else {
      expired.value = false
      firedExpire = false
    }
  }

  function start() {
    stop()
    tick()
    timer = setInterval(tick, 1000)
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  watch(
    () => [expiredAt.value, options?.active?.value] as const,
    () => {
      firedExpire = false
      start()
    },
    { immediate: true },
  )

  onUnmounted(stop)

  return { remaining, label, expired, start, stop, tick }
}
