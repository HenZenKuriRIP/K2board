<template>
  <!-- Full-bleed photo: keep top-left art logo in frame on all viewports -->
  <div class="scene" aria-hidden="true">
    <img
      class="scene-photo"
      :src="bgSrc"
      alt=""
      decoding="async"
      fetchpriority="high"
      @error="onBgError"
    />
    <div class="scene-glow" />
    <div class="scene-edge" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

// Bump when replacing public/background.png
const BG_VER = '20260721c'
const PRIMARY = `/background.png?v=${BG_VER}`
const FALLBACK = `/background.jpg?v=${BG_VER}`
const bgSrc = ref(PRIMARY)

function onBgError() {
  if (!bgSrc.value.includes('background.jpg')) {
    bgSrc.value = FALLBACK
  }
}
</script>

<style scoped>
.scene {
  position: fixed;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  overflow: hidden;
  background: #1a0a2e;
}
.scene-photo {
  position: absolute;
  /* Slight negative inset avoids subpixel gaps without heavy crop */
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: block;
  border: 0;
  object-fit: cover;
  /*
   * Anchor to TOP-LEFT so the art logo stays visible.
   * center/center was cropping the logo off the left on wide PC screens.
   */
  object-position: left top;
  transform: none;
  animation: none;
  filter: saturate(1.03) contrast(1.01);
}

/* Large desktop: still prefer left-top, mild horizontal bias into frame */
@media (min-width: 1100px) {
  .scene-photo {
    object-position: 8% 6%;
  }
}
@media (min-width: 900px) and (max-width: 1099.98px) {
  .scene-photo {
    object-position: 4% 4%;
  }
}

/* Tablet / iPad portrait & landscape */
@media (max-width: 899.98px) {
  .scene-photo {
    object-position: 6% 8%;
  }
}
@media (max-width: 899.98px) and (orientation: portrait) {
  .scene-photo {
    /* Tall screens: lock upper-left of landscape artwork */
    object-position: 10% 12%;
  }
}
@media (max-width: 480px) and (orientation: portrait) {
  .scene-photo {
    object-position: 8% 10%;
  }
}
/* iPad landscape (~1024–1366 short height) */
@media (min-width: 900px) and (max-height: 900px) {
  .scene-photo {
    object-position: 5% 10%;
  }
}

.scene-glow {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse 60% 50% at 75% 40%, rgba(244, 63, 94, 0.05), transparent 55%);
  mix-blend-mode: soft-light;
  pointer-events: none;
}
.scene-edge {
  position: absolute;
  inset: 0;
  /* Very light — do not cover top-left logo */
  background:
    linear-gradient(180deg, rgba(8, 6, 18, 0.06) 0%, transparent 14%),
    linear-gradient(0deg, rgba(6, 4, 14, 0.28) 0%, transparent 22%),
    radial-gradient(
      ellipse 110% 100% at 50% 50%,
      transparent 0%,
      transparent 80%,
      rgba(6, 4, 16, 0.12) 100%
    );
  pointer-events: none;
}
@media (max-width: 899.98px) {
  .scene-edge {
    background:
      linear-gradient(180deg, rgba(8, 6, 18, 0.04) 0%, transparent 16%),
      linear-gradient(0deg, rgba(6, 4, 14, 0.38) 0%, transparent 32%),
      radial-gradient(
        ellipse 110% 100% at 50% 45%,
        transparent 0%,
        transparent 78%,
        rgba(6, 4, 16, 0.12) 100%
      );
  }
}
</style>
