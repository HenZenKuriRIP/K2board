<template>
  <div class="empty-state" :class="variant">
    <div class="illus" aria-hidden="true">
      <!-- pending -->
      <svg v-if="variant === 'pending'" viewBox="0 0 120 100" width="120" height="100" fill="none">
        <ellipse cx="60" cy="88" rx="36" ry="6" fill="rgba(251,191,36,0.12)" />
        <rect x="28" y="22" width="64" height="52" rx="12" fill="rgba(251,191,36,0.08)" stroke="rgba(251,191,36,0.35)" stroke-width="1.5"/>
        <circle cx="60" cy="48" r="14" stroke="#fbbf24" stroke-width="2" fill="rgba(251,191,36,0.1)"/>
        <path d="M60 40v10l7 4" stroke="#fbbf24" stroke-width="2" stroke-linecap="round"/>
        <path d="M40 18h40" stroke="rgba(165,180,252,0.4)" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <!-- paid -->
      <svg v-else-if="variant === 'paid'" viewBox="0 0 120 100" width="120" height="100" fill="none">
        <ellipse cx="60" cy="88" rx="36" ry="6" fill="rgba(52,211,153,0.12)" />
        <rect x="28" y="22" width="64" height="52" rx="12" fill="rgba(52,211,153,0.08)" stroke="rgba(52,211,153,0.35)" stroke-width="1.5"/>
        <circle cx="60" cy="48" r="14" fill="rgba(52,211,153,0.15)" stroke="#34d399" stroke-width="2"/>
        <path d="M53 48l5 5 10-12" stroke="#34d399" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      <!-- cancelled -->
      <svg v-else-if="variant === 'cancelled'" viewBox="0 0 120 100" width="120" height="100" fill="none">
        <ellipse cx="60" cy="88" rx="36" ry="6" fill="rgba(148,163,184,0.12)" />
        <rect x="28" y="22" width="64" height="52" rx="12" fill="rgba(148,163,184,0.06)" stroke="rgba(148,163,184,0.3)" stroke-width="1.5"/>
        <circle cx="60" cy="48" r="14" stroke="#94a3b8" stroke-width="2" fill="rgba(148,163,184,0.08)"/>
        <path d="M54 42l12 12M66 42l-12 12" stroke="#94a3b8" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <!-- not found -->
      <svg v-else-if="variant === 'notfound'" viewBox="0 0 120 100" width="120" height="100" fill="none">
        <ellipse cx="60" cy="88" rx="36" ry="6" fill="rgba(129,140,248,0.1)" />
        <circle cx="52" cy="46" r="18" stroke="rgba(165,180,252,0.5)" stroke-width="2" fill="rgba(99,102,241,0.08)"/>
        <path d="M64 58l14 14" stroke="#a5b4fc" stroke-width="2.5" stroke-linecap="round"/>
        <path d="M46 46h12M52 40v12" stroke="rgba(165,180,252,0.45)" stroke-width="1.5" stroke-linecap="round"/>
      </svg>
      <!-- default / all -->
      <svg v-else viewBox="0 0 120 100" width="120" height="100" fill="none">
        <ellipse cx="60" cy="88" rx="40" ry="6" fill="rgba(99,102,241,0.12)" />
        <rect x="22" y="28" width="52" height="40" rx="10" fill="rgba(99,102,241,0.1)" stroke="rgba(129,140,248,0.4)" stroke-width="1.5" transform="rotate(-6 48 48)"/>
        <rect x="36" y="20" width="56" height="44" rx="10" fill="rgba(15,23,42,0.6)" stroke="rgba(165,180,252,0.45)" stroke-width="1.5"/>
        <path d="M48 36h32M48 46h24M48 56h18" stroke="rgba(165,180,252,0.45)" stroke-width="2" stroke-linecap="round"/>
      </svg>
    </div>
    <h4 v-if="title">{{ title }}</h4>
    <p v-if="description">{{ description }}</p>
    <div class="actions" v-if="$slots.default">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'all' | 'pending' | 'paid' | 'cancelled' | 'notfound'
    title?: string
    description?: string
  }>(),
  { variant: 'all', title: '', description: '' },
)
</script>

<style scoped>
.empty-state {
  text-align: center;
  padding: 40px 24px 36px;
  color: var(--u-text-3);
  border-radius: 20px;
}
.illus {
  display: flex;
  justify-content: center;
  margin-bottom: 8px;
  opacity: 0.95;
}
h4 {
  margin: 8px 0 6px;
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text-2);
}
p {
  margin: 0 auto;
  max-width: 320px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--u-text-3);
}
.actions {
  margin-top: 18px;
  display: flex;
  gap: 10px;
  justify-content: center;
  flex-wrap: wrap;
}
</style>
