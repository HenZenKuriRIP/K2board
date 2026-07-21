<template>
  <div v-if="chips.length" class="chips" :class="{ compact }">
    <span v-for="(c, i) in chips" :key="i" class="chip">{{ c }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OrderInfo } from '@/api/userApi'
import { benefitChips } from '@/utils/order'

const props = withDefaults(
  defineProps<{ order?: OrderInfo | null; compact?: boolean }>(),
  { compact: false },
)

const chips = computed(() => benefitChips(props.order))
</script>

<style scoped>
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.chips.compact {
  gap: 6px;
}
.chip {
  font-size: 12px;
  font-weight: 600;
  color: var(--u-primary-strong);
  background: var(--u-primary-soft);
  border: 1px solid var(--u-border-glow);
  padding: 4px 10px;
  border-radius: 999px;
  box-shadow: none;
}
.compact .chip {
  font-size: 11px;
  padding: 3px 8px;
}
</style>
