<template>
  <div class="page" v-loading="store.loading">
    <div class="page-head">
      <div>
        <h2>订阅管理</h2>
        <p v-if="hasActiveService">复制对应客户端链接，导入后即可使用</p>
        <p v-else>开通套餐后可在此管理订阅链接</p>
      </div>
      <el-button text type="primary" @click="$router.push('/user/docs')">客户端使用教程</el-button>
    </div>

    <section v-if="!hasBoundPlan" class="sub-warn glass-panel">
      <strong>尚未开通有效套餐</strong>
      <p>请先在仪表盘购买套餐并完成支付。未开通时<strong>不提供订阅链接</strong>，客户端无法连接节点。</p>
      <el-button type="primary" size="small" @click="$router.push('/user')">去购买套餐</el-button>
    </section>
    <section v-else-if="store.info?.expired" class="sub-warn warn glass-panel">
      <strong>套餐已过期</strong>
      <p>请续费或重新购买后再导入客户端，否则无法连接节点。过期期间订阅链接已停用。</p>
      <el-button type="primary" size="small" @click="$router.push('/user')">去续费</el-button>
    </section>

    <!-- 仅有效套餐时展示订阅链接（避免误导新用户） -->
    <template v-if="hasActiveService">
      <section class="glass-panel card main-card">
        <div class="card-head">
          <h3>推荐订阅（FlClash）</h3>
          <span class="chip">默认</span>
        </div>
        <div class="url-row" v-if="clashUrl">
          <el-input :model-value="clashUrl" readonly size="large" class="url-input" />
          <el-button type="primary" size="large" @click="copyClash">一键复制</el-button>
        </div>
        <p class="hint">
          默认已是 <b>Clash / FlClash</b> 专用格式（YAML）。请勿使用未带格式参数的旧链接导入 FlClash。
        </p>

        <div class="flag-grid">
          <button
            v-for="f in flags"
            :key="f.flag"
            type="button"
            class="flag-btn u-lift"
            :class="{ recommend: f.recommend }"
            @click="copyFlag(f)"
          >
            <span class="flag-name">{{ f.name }}</span>
            <span class="flag-desc">{{ f.desc }}</span>
          </button>
        </div>
      </section>

      <section class="glass-panel card guide-card">
        <div class="card-head">
          <h3>快速配置</h3>
          <span class="chip soft">3 步完成</span>
        </div>
        <div class="steps">
          <div class="step" v-for="(s, i) in steps" :key="i">
            <div class="step-num">{{ i + 1 }}</div>
            <div>
              <strong>{{ s.t }}</strong>
              <p>{{ s.d }}</p>
            </div>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import { userHasBoundPlan, userHasActiveService } from '@/utils/userService'

const store = useUserAuthStore()

const hasBoundPlan = computed(() => userHasBoundPlan(store.info))
const hasActiveService = computed(() => userHasActiveService(store.info))

const flags = [
  { flag: 'clash', name: 'FlClash', desc: 'Win / Mac / 安卓 · 推荐', recommend: true },
  { flag: 'shadowrocket', name: '小火箭', desc: 'Shadowrocket · iOS', recommend: false },
  { flag: 'surge', name: 'Surge', desc: 'iOS / macOS', recommend: false },
]

const steps = [
  { t: '复制订阅链接', d: '优先点「一键复制」或「FlClash」按钮（已含 Clash 格式）' },
  { t: '导入客户端', d: 'FlClash / 小火箭 / Surge 中粘贴订阅 URL 导入' },
  { t: '选择节点连接', d: '更新配置后选择节点，开启系统代理即可' },
]

const baseUrl = computed(() => store.info?.subscribe_url || '')

const clashUrl = computed(() => withFlag('clash'))

function withFlag(flag: string) {
  const u = baseUrl.value
  if (!u) return ''
  let clean = u.replace(/([?&])flag=[^&]*/g, '$1').replace(/[?&]$/, '')
  clean = clean.replace(/\?&/, '?').replace(/&&/g, '&')
  const sep = clean.includes('?') ? '&' : '?'
  return `${clean}${sep}flag=${flag}`
}

async function copyText(url: string, tip: string) {
  if (!hasActiveService.value) {
    ElMessage.warning('请先购买有效套餐后再复制订阅')
    return
  }
  if (!url) {
    ElMessage.warning('暂无订阅链接')
    return
  }
  await navigator.clipboard.writeText(url)
  ElMessage.success(tip)
}

async function copyClash() {
  await copyText(clashUrl.value, 'FlClash 订阅链接已复制')
}

async function copyFlag(f: { flag: string; name: string }) {
  await copyText(withFlag(f.flag), `${f.name} 链接已复制`)
}

onMounted(() => {
  if (!store.info) store.fetchInfo()
})
</script>

<style scoped>
.page { width: 100%; display: flex; flex-direction: column; gap: 16px; }
.page-head {
  display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; flex-wrap: wrap;
}
.page-head h2 { margin: 0; font-size: 22px; font-weight: 800; color: var(--u-text); }
.page-head p { margin: 6px 0 0; font-size: 13px; color: var(--u-text-3); }

.sub-warn {
  padding: 18px 20px;
  border-radius: 16px;
  border: 1px solid rgba(251, 191, 36, 0.35);
  background: var(--u-warning-soft);
}
.sub-warn.warn {
  border-color: rgba(248, 113, 113, 0.35);
  background: var(--u-danger-soft);
}
.sub-warn strong { display: block; font-size: 15px; color: var(--u-text); margin-bottom: 6px; }
.sub-warn p { margin: 0 0 12px; font-size: 13px; color: var(--u-text-2); line-height: 1.55; }

.card {
  padding: 20px;
  border-radius: 16px;
}
.card-head {
  display: flex; align-items: center; gap: 10px; margin-bottom: 14px;
}
.card-head h3 { margin: 0; font-size: 16px; font-weight: 800; color: var(--u-text); }
.chip {
  font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 999px;
  background: var(--u-primary-soft); color: var(--u-primary); border: 1px solid var(--u-border-glow);
}
.chip.soft { background: var(--u-surface-2); color: var(--u-text-3); border-color: var(--u-border); }

.url-row { display: flex; gap: 10px; flex-wrap: wrap; }
.url-input { flex: 1; min-width: 200px; }
.hint { margin: 12px 0 0; font-size: 12px; color: var(--u-text-3); line-height: 1.5; }

.flag-grid {
  margin-top: 16px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 10px;
}
.flag-btn {
  text-align: left;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid var(--u-border);
  background: var(--u-surface-2);
  color: var(--u-text);
  cursor: pointer;
  font-family: inherit;
}
.flag-btn.recommend { border-color: var(--u-border-glow); background: var(--u-primary-soft); }
.flag-name { display: block; font-weight: 800; font-size: 13px; margin-bottom: 4px; }
.flag-desc { font-size: 11px; color: var(--u-text-3); }

.steps { display: flex; flex-direction: column; gap: 12px; }
.step { display: flex; gap: 12px; align-items: flex-start; }
.step-num {
  width: 28px; height: 28px; border-radius: 8px; display: grid; place-items: center;
  font-weight: 800; font-size: 13px; color: var(--u-text-inv);
  background: var(--u-primary); flex-shrink: 0;
}
.step strong { display: block; font-size: 13px; color: var(--u-text); }
.step p { margin: 4px 0 0; font-size: 12px; color: var(--u-text-3); line-height: 1.45; }
</style>
