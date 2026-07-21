<template>
  <div class="page">
    <header class="card head">
      <div>
        <h2>客户端使用教程</h2>
        <p>
          Windows / macOS / Android 统一使用 <b>FlClash</b>（界面相同）；iOS 使用 Shadowrocket。
          教程对所有登录用户开放。
        </p>
      </div>
      <div class="head-actions">
        <el-button
          type="success"
          size="large"
          :disabled="!hasActiveService"
          @click="copySub"
        >
          ① 复制订阅链接
        </el-button>
        <el-button size="large" @click="$router.push('/user/subscribe')">订阅管理</el-button>
      </div>
    </header>

    <!-- 未订阅：仅提示，不锁定教程正文 -->
    <section v-if="!hasBoundPlan" class="soft-banner card">
      <strong>尚未购买套餐</strong>
      <p>
        您可先按下方教程安装客户端；导入订阅与连接节点需
        <el-button type="primary" link @click="$router.push('/user')">购买套餐</el-button>
        并完成支付。右下角 Crisp 气泡可联系在线客服。
      </p>
    </section>
    <section v-else-if="store.info?.expired" class="soft-banner card warn">
      <strong>套餐已过期</strong>
      <p>
        教程仍可查看；请
        <el-button type="primary" link @click="$router.push('/user')">续费或新购</el-button>
        后复制订阅链接再导入。
      </p>
    </section>

    <section class="card flow">
      <div v-for="(s, i) in quickStartSteps" :key="i" class="flow-item">
        <span class="flow-n" :class="'n' + i">{{ i + 1 }}</span>
        <div>
          <strong>{{ s.t }}</strong>
          <span>{{ s.d }}</span>
        </div>
      </div>
    </section>

    <section>
      <div class="sec-title">② 选择你的系统</div>
      <div class="os-grid">
        <button
          v-for="p in platformTabs"
          :key="p.key"
          type="button"
          class="os-card"
          :class="[p.key, { active: platform === p.key }]"
          @click="platform = p.key"
        >
          <span class="os-icon">{{ p.icon }}</span>
          <span class="os-name">{{ p.label }}</span>
          <span class="os-app">{{ guideOf(p.key)?.name }}</span>
        </button>
      </div>
    </section>

    <section v-if="current" class="card detail">
      <div class="detail-head">
        <div class="title-row">
          <span class="plat-pill" :class="current.platform">{{ platLabel(current.platform) }}</span>
          <h3>{{ current.name }}</h3>
          <span v-if="current.badge" class="rec">{{ current.badge }}</span>
        </div>
        <p>{{ current.summary }}</p>
      </div>

      <div class="zone z-dl">
        <div class="z-label">
          <span class="tag t-dl">下载</span>
          <strong>③ 安装客户端</strong>
        </div>
        <div v-if="current.downloads.length" class="dl-row">
          <a
            v-for="(d, i) in current.downloads"
            :key="i"
            class="dl"
            :class="dlClass(d)"
            :href="d.url"
            target="_blank"
            rel="noopener noreferrer"
          >
            <span class="dl-mark">{{ d.primary ? '推荐' : '备用' }}</span>
            {{ d.label }}
          </a>
        </div>
        <p v-if="current.platform === 'ios'" class="ios-hint">
          iOS 请通过右下角 Crisp 客服索取美区 Apple ID；验证码也向客服索取（详见下方步骤）。
        </p>
        <p v-else class="z-note">绿色按钮优先下载 · 打不开时换「GitHub 全部版本」或切换网络</p>
      </div>

      <div class="zone z-step">
        <div class="z-label">
          <span class="tag t-step">图文步骤</span>
          <strong>④ 按顺序操作（看图即可）</strong>
        </div>
        <ol class="steps">
          <li v-for="(st, si) in current.steps" :key="si">
            <div class="st-h">
              <span class="st-n">{{ si + 1 }}</span>
              <strong>{{ st.title }}</strong>
            </div>
            <p class="st-b" v-html="formatBody(st.body)" />
            <div v-if="st.images?.length" class="imgs">
              <a
                v-for="(src, ii) in st.images"
                :key="ii"
                :href="src"
                target="_blank"
                rel="noopener noreferrer"
                class="img-wrap"
              >
                <img :src="src" :alt="`${st.title} 图${ii + 1}`" loading="lazy" @error="onImgErr" />
                <span class="img-cap">示意图 {{ si + 1 }}-{{ ii + 1 }} · 点击放大</span>
              </a>
            </div>
          </li>
        </ol>
      </div>

      <div v-if="current.tips?.length" class="zone z-tip">
        <div class="z-label">
          <span class="tag t-tip">提示</span>
        </div>
        <ul>
          <li v-for="(tip, ti) in current.tips" :key="ti">{{ tip }}</li>
        </ul>
      </div>
    </section>

    <section class="card note">
      <ul>
        <li>FlClash 开源地址：github.com/chen08209/FlClash · 教程参考：clashbk/clash wiki flclash</li>
        <li>订阅链接仅从本站复制，请勿泄露给他人。</li>
        <li>连不上：更新订阅、换节点，或重新按图导入配置。</li>
        <li>在线客服：登录后点右下角 Crisp 气泡（旁有「在线客服」动态提示）。</li>
      </ul>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserAuthStore } from '@/stores/userAuth'
import { userHasBoundPlan, userHasActiveService } from '@/utils/userService'
import {
  clientGuides,
  platformTabs,
  quickStartSteps,
  type ClientGuide,
  type GuideLink,
} from '@/data/clientGuides'

const store = useUserAuthStore()
const platform = ref<ClientGuide['platform']>('windows')

const hasBoundPlan = computed(() => userHasBoundPlan(store.info))
const hasActiveService = computed(() => userHasActiveService(store.info))

const current = computed(() => guideOf(platform.value))

function guideOf(key: ClientGuide['platform']) {
  return clientGuides.find((g) => g.platform === key)
}

function platLabel(p: ClientGuide['platform']) {
  if (p === 'windows') return 'Windows'
  if (p === 'macos') return 'macOS'
  if (p === 'android') return 'Android'
  return 'iOS'
}

function dlClass(d: GuideLink) {
  if (d.primary) return 'primary'
  if (/github/i.test(d.url + d.label)) return 'github'
  return 'other'
}

function formatBody(body: string) {
  return body
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
}

async function copySub() {
  if (!hasActiveService.value) {
    ElMessage.warning('请先购买有效套餐后再复制订阅')
    return
  }
  let url = store.info?.subscribe_url
  if (!url) {
    ElMessage.warning('暂无订阅链接，请先开通套餐')
    return
  }
  url = url.replace(/([?&])flag=[^&]*/g, '$1').replace(/[?&]$/, '').replace(/\?&/, '?')
  const sep = url.includes('?') ? '&' : '?'
  try {
    await navigator.clipboard.writeText(`${url}${sep}flag=clash`)
    ElMessage.success('已复制 FlClash/Clash 订阅链接')
  } catch {
    ElMessage.error('复制失败，请到「订阅」页手动复制')
  }
}

function onImgErr(e: Event) {
  const el = e.target as HTMLImageElement
  const wrap = el.closest('.img-wrap') as HTMLElement | null
  if (wrap) wrap.style.display = 'none'
}

onMounted(() => {
  if (store.isLoggedIn && !store.info) store.fetchInfo()
})
</script>

<style scoped>
.page {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.card {
  background: var(--u-surface);
  border: 1px solid var(--u-border);
  border-radius: 14px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}

.soft-banner {
  padding: 14px 18px;
  border-color: rgba(251, 191, 36, 0.35);
  background: var(--u-warning-soft);
}
.soft-banner.warn {
  border-color: rgba(248, 113, 113, 0.35);
  background: var(--u-danger-soft);
}
.soft-banner strong {
  display: block;
  font-size: 14px;
  color: var(--u-text);
  margin-bottom: 6px;
}
.soft-banner p {
  margin: 0;
  font-size: 13px;
  color: var(--u-text-2);
  line-height: 1.55;
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  padding: 18px 20px;
}
.head h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 800;
  color: var(--u-text);
}
.head p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--u-text-3);
  line-height: 1.5;
}
.head p b { color: var(--u-primary); }
.head-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* Lock */
.lock-panel {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  padding: 22px 20px;
  border-color: rgba(251, 191, 36, 0.4);
  background: var(--u-warning-soft);
}
.lock-panel.warn {
  border-color: rgba(248, 113, 113, 0.4);
  background: var(--u-danger-soft);
}
.lock-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  font-size: 22px;
  background: var(--u-surface);
  border: 1px solid var(--u-border);
  flex-shrink: 0;
}
.lock-main strong {
  display: block;
  font-size: 16px;
  font-weight: 800;
  color: var(--u-text);
  margin-bottom: 8px;
}
.lock-main p {
  margin: 0 0 14px;
  font-size: 13px;
  color: var(--u-text-2);
  line-height: 1.6;
}
.lock-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.flow {
  display: grid;
  grid-template-columns: 1fr;
  padding: 6px 8px;
}
@media (min-width: 800px) {
  .flow { grid-template-columns: repeat(4, 1fr); }
}
.flow-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 10px;
}
.flow-n {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: #fff;
  flex-shrink: 0;
}
.flow-n.n0 { background: #16a34a; }
.flow-n.n1 { background: #2563eb; }
.flow-n.n2 { background: #7c3aed; }
.flow-n.n3 { background: #d97706; }
.flow-item strong {
  display: block;
  font-size: 13px;
  color: var(--u-text);
}
.flow-item span {
  font-size: 12px;
  color: var(--u-text-3);
}

.sec-title {
  font-size: 14px;
  font-weight: 800;
  color: var(--u-text);
  margin: 4px 0 10px;
}
.os-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}
@media (min-width: 700px) {
  .os-grid { grid-template-columns: repeat(4, 1fr); }
}
.os-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--u-border);
  background: var(--u-surface);
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.os-card:hover { border-color: #a5b4fc; }
.os-card.active {
  border-color: var(--u-primary);
  box-shadow: 0 0 0 2px var(--u-primary-soft);
}
.os-icon { font-size: 22px; }
.os-name { font-weight: 800; font-size: 14px; color: var(--u-text); }
.os-app { font-size: 11px; color: var(--u-text-3); }

.detail { padding: 18px 20px 22px; }
.detail-head { margin-bottom: 16px; }
.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.title-row h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 800;
  color: var(--u-text);
}
.detail-head > p {
  margin: 8px 0 0;
  font-size: 13px;
  color: var(--u-text-3);
  line-height: 1.5;
}
.plat-pill {
  font-size: 11px;
  font-weight: 700;
  padding: 3px 8px;
  border-radius: 999px;
  border: 1px solid;
}
.plat-pill.windows { color: #0369a1; background: #e0f2fe; border-color: #bae6fd; }
.plat-pill.macos { color: var(--u-primary); background: #ede9fe; border-color: #ddd6fe; }
.plat-pill.android { color: #166534; background: #dcfce7; border-color: #bbf7d0; }
.plat-pill.ios { color: #9a3412; background: #ffedd5; border-color: #fed7aa; }
.rec {
  font-size: 11px;
  font-weight: 700;
  color: #14532d;
  background: #bbf7d0;
  padding: 2px 8px;
  border-radius: 999px;
}

.zone {
  margin-top: 14px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--u-border);
  background: var(--u-surface-2);
}
.z-dl { background: #f0fdf4; border-color: #bbf7d0; }
.z-step { background: var(--u-surface-2); }
.z-tip { background: #fffbeb; border-color: #fde68a; margin-bottom: 0; }
.z-label {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.z-label strong { font-size: 14px; color: var(--u-text); }
.tag {
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  padding: 2px 7px;
  border-radius: 6px;
  color: #fff;
}
.t-dl { background: #16a34a; }
.t-step { background: #4f46e5; }
.t-tip { background: #d97706; }

.dl-row { display: flex; flex-wrap: wrap; gap: 8px; }
.dl {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  border: 1px solid var(--u-border);
  color: var(--u-text);
  background: var(--u-surface);
}
.dl.primary {
  background: #16a34a;
  border-color: #15803d;
  color: #fff;
}
.dl.github {
  background: #0f172a;
  border-color: #1e293b;
  color: #fff;
}
.dl-mark {
  font-size: 10px;
  font-weight: 800;
  opacity: 0.85;
}
.z-note, .ios-hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.5;
}

.steps {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.st-h {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.st-n {
  width: 24px;
  height: 24px;
  border-radius: 7px;
  display: grid;
  place-items: center;
  font-size: 12px;
  font-weight: 800;
  color: #fff;
  background: var(--u-primary);
  flex-shrink: 0;
}
.st-h strong { font-size: 14px; color: var(--u-text); }
.st-b {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--u-text-2);
  line-height: 1.6;
}
.imgs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px;
}
.img-wrap {
  display: block;
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid var(--u-border);
  background: var(--u-surface);
  text-decoration: none;
}
.img-wrap img {
  display: block;
  width: 100%;
  height: auto;
  vertical-align: top;
}
.img-cap {
  display: block;
  padding: 6px 8px;
  font-size: 11px;
  color: var(--u-text-3);
  background: var(--u-surface-2);
}

.z-tip ul {
  margin: 0;
  padding-left: 1.15em;
  font-size: 13px;
  color: var(--u-text-2);
  line-height: 1.55;
}

.note {
  padding: 14px 18px;
}
.note ul {
  margin: 0;
  padding-left: 1.15em;
  font-size: 12px;
  color: var(--u-text-3);
  line-height: 1.6;
}
</style>
