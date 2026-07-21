<template>
  <!--
    Custom Crisp launcher: drag to move; tap to open/close chat.
    Position persisted in localStorage. Native Crisp bubble is CSS-hidden.
  -->
  <button
    v-if="visible"
    ref="btnRef"
    type="button"
    class="crisp-fab"
    :class="{ dragging, 'is-left': isLeftHalf }"
    :style="fabStyle"
    aria-label="在线客服，拖动可移动位置"
    title="点击咨询 · 拖动可移动"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
    @click.prevent
  >
    <span class="fab-ico-wrap" aria-hidden="true">
      <span class="fab-ico">💬</span>
      <span class="fab-pulse" />
    </span>
    <span class="fab-label">在线客服</span>
  </button>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useUserAuthStore } from '@/stores/userAuth'
import { toggleCrisp, loadCrisp, isCrispConfigured } from '@/utils/crisp'

const POS_KEY = 'k2_crisp_fab_pos'
/** Approximate pill size for clamp / default placement */
const FAB_W = 148
const FAB_H = 60
const DRAG_THRESHOLD = 8

const store = useUserAuthStore()
const visible = ref(false)
const btnRef = ref<HTMLButtonElement | null>(null)

/** position as left/top in CSS pixels (viewport) */
const left = ref(0)
const top = ref(0)
const dragging = ref(false)

let startX = 0
let startY = 0
let originLeft = 0
let originTop = 0
let moved = false
let activePointer: number | null = null

const isLeftHalf = computed(() => left.value + FAB_W / 2 < window.innerWidth / 2)

const fabStyle = computed(() => ({
  left: `${Math.round(left.value)}px`,
  top: `${Math.round(top.value)}px`,
  right: 'auto',
  bottom: 'auto',
}))

function mobileBottomReserve(): number {
  if (typeof window === 'undefined') return 16
  return window.innerWidth < 960 ? 80 : 16
}

function measureFab(): { w: number; h: number } {
  const el = btnRef.value
  if (el) {
    const r = el.getBoundingClientRect()
    if (r.width > 0 && r.height > 0) return { w: r.width, h: r.height }
  }
  return { w: FAB_W, h: FAB_H }
}

function clampPos(x: number, y: number): { x: number; y: number } {
  const { w, h } = measureFab()
  const pad = 8
  const maxX = Math.max(pad, window.innerWidth - w - pad)
  const maxY = Math.max(pad, window.innerHeight - h - pad - mobileBottomReserve())
  return {
    x: Math.min(maxX, Math.max(pad, x)),
    y: Math.min(maxY, Math.max(pad, y)),
  }
}

function defaultPos(): { x: number; y: number } {
  const { w, h } = measureFab()
  const padR = 16
  const padB = 20 + mobileBottomReserve()
  return clampPos(
    window.innerWidth - w - padR,
    window.innerHeight - h - padB,
  )
}

function loadPos() {
  try {
    const raw = localStorage.getItem(POS_KEY)
    if (raw) {
      const p = JSON.parse(raw) as { x?: number; y?: number }
      if (typeof p.x === 'number' && typeof p.y === 'number') {
        const c = clampPos(p.x, p.y)
        left.value = c.x
        top.value = c.y
        return
      }
    }
  } catch {
    /* ignore */
  }
  const d = defaultPos()
  left.value = d.x
  top.value = d.y
}

function savePos() {
  try {
    localStorage.setItem(POS_KEY, JSON.stringify({ x: left.value, y: top.value }))
  } catch {
    /* ignore */
  }
}

function onPointerDown(e: PointerEvent) {
  if (e.button != null && e.button !== 0) return
  activePointer = e.pointerId
  startX = e.clientX
  startY = e.clientY
  originLeft = left.value
  originTop = top.value
  moved = false
  dragging.value = false
  try {
    btnRef.value?.setPointerCapture(e.pointerId)
  } catch {
    /* ignore */
  }
}

function onPointerMove(e: PointerEvent) {
  if (activePointer == null || e.pointerId !== activePointer) return
  const dx = e.clientX - startX
  const dy = e.clientY - startY
  if (!moved && Math.hypot(dx, dy) < DRAG_THRESHOLD) return
  moved = true
  dragging.value = true
  const c = clampPos(originLeft + dx, originTop + dy)
  left.value = c.x
  top.value = c.y
  e.preventDefault()
}

function onPointerUp(e: PointerEvent) {
  if (activePointer == null || e.pointerId !== activePointer) return
  activePointer = null
  try {
    btnRef.value?.releasePointerCapture(e.pointerId)
  } catch {
    /* ignore */
  }
  if (moved) {
    savePos()
    dragging.value = false
    return
  }
  dragging.value = false
  toggleCrisp()
}

function onResize() {
  const c = clampPos(left.value, top.value)
  left.value = c.x
  top.value = c.y
}

function refreshVisible() {
  // Hide FAB entirely when this deploy has no Crisp Website ID
  visible.value = !!store.isLoggedIn && isCrispConfigured()
  if (visible.value) {
    loadCrisp()
    // next frame so button is measured for clamp
    requestAnimationFrame(() => loadPos())
  }
}

onMounted(() => {
  refreshVisible()
  window.addEventListener('resize', onResize)
  window.addEventListener('orientationchange', onResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  window.removeEventListener('orientationchange', onResize)
})

watch(
  () => store.isLoggedIn,
  () => refreshVisible(),
)
</script>

<style scoped>
.crisp-fab {
  position: fixed;
  z-index: 99998;
  min-width: 148px;
  height: 60px;
  margin: 0;
  padding: 0 18px 0 10px;
  border: none;
  border-radius: 999px;
  cursor: grab;
  display: inline-flex;
  flex-direction: row;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  font-family: inherit;
  color: #fff;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 42%, #7c3aed 100%);
  box-shadow:
    0 8px 28px rgba(79, 70, 229, 0.5),
    0 2px 8px rgba(15, 23, 42, 0.18),
    0 0 0 2px rgba(255, 255, 255, 0.28) inset;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
  -webkit-tap-highlight-color: transparent;
  transition: box-shadow 0.15s ease, transform 0.12s ease, filter 0.12s ease;
}
.crisp-fab:hover:not(.dragging) {
  filter: brightness(1.07);
  transform: scale(1.04);
  box-shadow:
    0 10px 32px rgba(79, 70, 229, 0.55),
    0 0 0 2px rgba(255, 255, 255, 0.32) inset;
}
.crisp-fab.dragging {
  cursor: grabbing;
  transform: scale(1.06);
  box-shadow:
    0 14px 36px rgba(79, 70, 229, 0.58),
    0 0 0 2px rgba(255, 255, 255, 0.35) inset;
  opacity: 0.96;
}

.fab-ico-wrap {
  position: relative;
  flex-shrink: 0;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.22);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.2) inset;
  pointer-events: none;
}
.fab-ico {
  font-size: 26px;
  line-height: 1;
  display: block;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.2));
}
.fab-pulse {
  position: absolute;
  inset: -3px;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.55);
  animation: fab-ring 1.8s ease-out infinite;
  pointer-events: none;
}
.fab-label {
  font-size: 15px;
  font-weight: 800;
  letter-spacing: 0.06em;
  line-height: 1.2;
  white-space: nowrap;
  pointer-events: none;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.18);
}

@keyframes fab-ring {
  0% {
    transform: scale(0.92);
    opacity: 0.85;
  }
  70% {
    transform: scale(1.18);
    opacity: 0;
  }
  100% {
    transform: scale(1.18);
    opacity: 0;
  }
}

@media (max-width: 420px) {
  .crisp-fab {
    min-width: 136px;
    height: 56px;
    padding: 0 14px 0 8px;
    gap: 8px;
  }
  .fab-ico-wrap {
    width: 40px;
    height: 40px;
  }
  .fab-ico {
    font-size: 24px;
  }
  .fab-label {
    font-size: 14px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .crisp-fab {
    transition: none;
  }
  .fab-pulse {
    animation: none;
    display: none;
  }
}
</style>
