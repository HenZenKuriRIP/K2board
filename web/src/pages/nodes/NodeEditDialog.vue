<template>
  <el-dialog
    :model-value="visible"
    :title="isEdit ? '编辑节点' : '创建节点'"
    width="720px"
    class="node-edit-dialog"
    @update:model-value="$emit('update:visible', $event)"
    @close="resetForm"
    destroy-on-close
  >
    <el-tabs v-model="activeTab" class="node-tabs">
      <!-- ① 线路类型 -->
      <el-tab-pane label="① 线路类型" name="profile">
        <p class="tab-lead">先选择节点形态，系统会填入推荐协议参数；仍可在后续页微调。</p>
        <div class="profile-grid">
          <button
            v-for="p in profiles"
            :key="p.id"
            type="button"
            class="profile-card"
            :class="{ active: form.profile === p.id, [p.tone]: true }"
            @click="applyProfile(p.id)"
          >
            <div class="pc-top">
              <span class="pc-badge">{{ p.badge }}</span>
              <span v-if="form.profile === p.id" class="pc-check">✓</span>
            </div>
            <div class="pc-title">{{ p.title }}</div>
            <div class="pc-desc">{{ p.desc }}</div>
            <div class="pc-tags">
              <span v-for="t in p.tags" :key="t">{{ t }}</span>
            </div>
          </button>
        </div>
        <div class="profile-summary" v-if="form.profile">
          <strong>当前：</strong>{{ currentProfileMeta?.title }}
          <span class="sum-sep">·</span>
          <code>{{ form.node_type }}</code>
          <span class="sum-sep">/</span>
          <code>{{ form.network || 'tcp' }}</code>
          <span class="sum-sep">/</span>
          <code>{{ tlsLabel(form.tls) }}</code>
          <span v-if="form.flow" class="sum-sep">· flow {{ form.flow }}</span>
        </div>
      </el-tab-pane>

      <!-- ② 接入参数 -->
      <el-tab-pane label="② 接入参数" name="basic">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="108px" label-position="right">
          <el-form-item label="节点名称" prop="name">
            <el-input v-model="form.name" placeholder="例: 日本-东京-直连-01 / 香港-CDN-01" />
          </el-form-item>

          <el-alert
            v-if="isCDN"
            type="success"
            :closable="false"
            show-icon
            class="mb-12"
            title="CDN 线路：客户端连接 Host/Path 指向 CDN；源站证书在节点机配置，不在面板上传。"
          />
          <el-alert
            v-else-if="isReality"
            type="info"
            :closable="false"
            show-icon
            class="mb-12"
            title="直连 REALITY：用户直连节点地址；请在「安全」页生成 REALITY 密钥，minClientVer 建议 1.8.0。"
          />
          <el-alert
            v-else-if="isAnyTLS"
            type="warning"
            :closable="false"
            show-icon
            class="mb-12"
            title="AnyTLS：用户 UUID 作为密码；TLS 证书域名填 SNI，勿使用 REALITY。"
          />

          <el-form-item :label="isCDN ? 'CDN / 接入域名' : '节点地址'" prop="host">
            <el-input
              v-model="form.host"
              :placeholder="isCDN ? 'cdn.example.com（用户与 XHTTP Host）' : 'node.example.com 或 IP'"
            />
          </el-form-item>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="端口">
                <el-input v-model.number="form.port" type="number" placeholder="443" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item v-if="showPathField" :label="isCDN ? 'XHTTP Path' : 'Path'">
                <el-input v-model="form.path" :placeholder="isCDN ? '/vless-cdn' : '/'" />
              </el-form-item>
              <el-form-item v-else-if="form.network === 'grpc'" label="gRPC Service">
                <el-input v-model="form.service_name" placeholder="grpc 服务名" />
              </el-form-item>
              <el-form-item v-else-if="isReality" label="Flow">
                <el-input v-model="form.flow" placeholder="xtls-rprx-vision" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item v-if="form.tls >= 1" label="SNI">
            <el-input
              v-model="form.sni"
              :placeholder="isReality ? '伪装站 SNI，如 www.microsoft.com' : '证书域名 / CDN 域名'"
            />
            <div class="field-hint" v-if="isCDN">通常与 CDN 域名一致；源站 Full Strict 证书匹配此名。</div>
          </el-form-item>

          <!-- 高级：允许改协议细节 -->
          <el-collapse class="adv-collapse">
            <el-collapse-item title="高级：手动改协议 / 传输（一般用线路类型即可）" name="adv">
              <el-row :gutter="16">
                <el-col :span="12">
                  <el-form-item label="协议">
                    <el-select v-model="form.node_type" style="width:100%" @change="onNodeTypeChange">
                      <el-option label="VLESS" value="vless" />
                      <el-option label="AnyTLS" value="anytls" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="传输">
                    <el-select v-model="form.network" style="width:100%" @change="onNetworkChange">
                      <el-option label="TCP（直连 REALITY 推荐）" value="tcp" />
                      <el-option label="XHTTP（CDN 推荐）" value="xhttp" />
                      <el-option label="SplitHTTP" value="splithttp" />
                      <el-option label="WebSocket" value="ws" />
                      <el-option label="gRPC" value="grpc" />
                    </el-select>
                  </el-form-item>
                </el-col>
              </el-row>
              <el-form-item v-if="form.node_type === 'vless' && !isCDN" label="Flow">
                <el-input v-model="form.flow" placeholder="REALITY 常用 xtls-rprx-vision；CDN 请留空" />
              </el-form-item>
            </el-collapse-item>
          </el-collapse>

          <el-divider content-position="left">可见范围</el-divider>
          <el-form-item label="可见范围" prop="scope_mode">
            <div class="scope-section">
              <el-radio-group v-model="form.scope_mode" class="scope-radio-group" @change="onScopeModeChange">
                <el-radio-button value="grouped">
                  <span class="radio-label">按权限组</span>
                </el-radio-button>
                <el-radio-button value="unassigned">
                  <span class="radio-label">未分配</span>
                </el-radio-button>
              </el-radio-group>
              <div v-if="form.scope_mode === 'unassigned'" class="scope-global-hint warn">
                未绑定权限组的节点<strong>不对任何用户开放</strong>。
              </div>
              <div v-else class="scope-group-area">
                <div v-if="groups.length === 0" class="scope-empty">暂无权限组，请先创建</div>
                <div v-else class="group-chips">
                  <div
                    v-for="g in groups"
                    :key="g.id"
                    class="group-chip"
                    :class="{ selected: form.group_ids.includes(g.id) }"
                    @click="toggleGroup(g.id)"
                  >
                    <span class="chip-check"><span v-if="form.group_ids.includes(g.id)">✓</span></span>
                    <span class="chip-name">{{ g.name }}</span>
                  </div>
                </div>
                <div class="scope-footer">
                  <span v-if="form.group_ids.length > 0" class="selected-count">
                    已选 <strong>{{ form.group_ids.length }}</strong> 个组
                  </span>
                  <span v-else class="validation-msg">请至少选择一个权限组</span>
                </div>
              </div>
            </div>
          </el-form-item>
        </el-form>
      </el-tab-pane>

      <!-- ③ 安全 -->
      <el-tab-pane label="③ 安全 / PQ" name="tls">
        <el-form label-width="108px" label-position="right">
          <el-form-item label="TLS 模式">
            <el-radio-group v-model="form.tls" :disabled="isAnyTLS">
              <el-radio-button :value="0">关闭</el-radio-button>
              <el-radio-button :value="1">TLS</el-radio-button>
              <el-radio-button v-if="!isAnyTLS" :value="2">REALITY</el-radio-button>
            </el-radio-group>
            <div class="field-hint" v-if="isAnyTLS">AnyTLS 固定使用 TLS（证书在节点机）。</div>
            <div class="field-hint" v-else-if="isCDN">CDN 线路请用 TLS，勿选 REALITY。</div>
          </el-form-item>

          <template v-if="form.tls === 2">
            <el-divider content-position="left">
              REALITY
              <el-button size="small" type="primary" class="ml-8" :loading="genReality" @click="handleGenerateReality">
                一键生成密钥对
              </el-button>
            </el-divider>
            <el-row :gutter="12">
              <el-col :span="12">
                <el-form-item label="Public Key">
                  <el-input v-model="form.reality_public_key" placeholder="公钥 pbk" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Private Key">
                  <el-input v-model="form.reality_private_key" placeholder="私钥" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="12">
              <el-col :span="12">
                <el-form-item label="Short ID">
                  <el-input v-model="form.reality_short_id" placeholder="hex" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Fingerprint">
                  <el-select v-model="form.reality_fingerprint" style="width:100%" filterable>
                    <el-option
                      v-for="f in fingerprintOptions"
                      :key="f.value"
                      :label="f.label"
                      :value="f.value"
                    />
                  </el-select>
                  <div class="field-hint">
                    默认 <code>chrome</code>（兼容广）。
                    <strong>开启后量子握手（X25519MLKEM768）</strong>时，请选手动带
                    <strong>PQ</strong> 标记的指纹（如 chrome120 / chrome140 / firefox148 / safari26_3），
                    否则更新订阅后客户端会回到无 MLK 的指纹，服务端日志可能显示
                    <code>X25519MLKEM768: false</code>。
                  </div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="Dest">
              <el-input v-model="form.reality_dest" placeholder="www.microsoft.com:443" />
            </el-form-item>
            <el-form-item label="minClientVer">
              <el-input v-model="form.reality_min_client_ver" placeholder="1.8.0" />
              <div class="field-hint">兼容 Mihomo；勿留空导致内核默认 26.3.27。</div>
            </el-form-item>

            <el-divider content-position="left">后量子（可选，默认关）</el-divider>
            <el-alert
              type="info"
              :closable="false"
              show-icon
              class="mb-12"
              title="X25519MLKEM768 无字段（dest 支持则自动）。ML-DSA / Encryption 留空 = 与旧节点完全一致。"
            />
            <el-form-item label="ML-DSA Seed">
              <el-input v-model="form.reality_mldsa65_seed" type="textarea" :rows="2" placeholder="xray mldsa65 → Seed；空=关" />
            </el-form-item>
            <el-form-item label="ML-DSA Verify">
              <el-input v-model="form.reality_mldsa65_verify" type="textarea" :rows="2" placeholder="Verify 公钥 → 订阅；与 Seed 成对" />
            </el-form-item>
            <el-form-item label="调试 show">
              <el-switch v-model="form.reality_show" active-text="开" inactive-text="关" />
            </el-form-item>
          </template>

          <template v-if="form.node_type === 'vless'">
            <el-divider content-position="left">VLESS Encryption（可选）</el-divider>
            <el-alert
              type="warning"
              :closable="false"
              show-icon
              class="mb-12"
              title="留空 = none。开启须成对填写；节点侧 EnableFallback 必须为 false。可用 xray vlessenc 生成。"
            />
            <el-form-item label="decryption">
              <el-input v-model="form.vless_decryption" type="textarea" :rows="2" placeholder="服务端串 / 空" />
            </el-form-item>
            <el-form-item label="encryption">
              <el-input v-model="form.vless_encryption" type="textarea" :rows="2" placeholder="客户端串 → 订阅" />
            </el-form-item>
          </template>
        </el-form>
      </el-tab-pane>

      <!-- ④ 限速 -->
      <el-tab-pane label="④ 限速" name="limit">
        <el-form label-width="108px">
          <el-form-item label="速度限制">
            <el-input v-model.number="form.speed_limit" type="number" placeholder="0" style="max-width:200px" />
            <span class="field-hint inline">Mbps，0 = 不限制</span>
          </el-form-item>
          <el-form-item v-if="isEdit" label="启用">
            <el-switch v-model="form.enable" active-text="启用" inactive-text="禁用" />
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <div class="dlg-foot">
        <span class="foot-hint">{{ footHint }}</span>
        <div>
          <el-button @click="$emit('update:visible', false)">取消</el-button>
          <el-button type="primary" :loading="submitting" @click="handleSubmit">
            {{ isEdit ? '保存修改' : '立即创建' }}
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import request from '@/api/request'
import { createNode, updateNode, type Node } from '@/api/node'

type ProfileId = 'reality' | 'cdn' | 'anytls' | 'custom'

interface Group { id: number; name: string }

const profiles: {
  id: ProfileId
  badge: string
  title: string
  desc: string
  tags: string[]
  tone: string
}[] = [
  {
    id: 'reality',
    badge: 'A · 主推',
    title: 'VLESS · REALITY · Vision',
    desc: '直连抗探测。TCP + REALITY + xtls-rprx-vision，可选后量子。',
    tags: ['tcp', 'REALITY', 'Vision', 'minClientVer 1.8.0'],
    tone: 'tone-indigo',
  },
  {
    id: 'cdn',
    badge: 'B · CDN',
    title: 'VLESS · TLS · XHTTP',
    desc: '过 CDN。TLS + XHTTP + Path，Vision 关闭；证书在节点机。',
    tags: ['xhttp', 'TLS', 'CDN', '无 Vision'],
    tone: 'tone-cyan',
  },
  {
    id: 'anytls',
    badge: 'C',
    title: 'AnyTLS · TLS',
    desc: '多路复用线路。UUID 作密码，TLS 证书域名。',
    tags: ['anytls', 'TLS'],
    tone: 'tone-amber',
  },
  {
    id: 'custom',
    badge: '高级',
    title: '自定义',
    desc: '不套模板，在「接入参数 → 高级」里手改协议与传输。',
    tags: ['手动'],
    tone: 'tone-slate',
  },
]

/** uTLS / 订阅 fp= 值。默认 chrome；带 PQ 的与 Shadowrocket 支持 X25519MLKEM768 的别名对齐。 */
const fingerprintOptions: { value: string; label: string }[] = [
  { value: 'chrome', label: 'chrome（默认，兼容）' },
  { value: 'chrome120', label: 'chrome120（PQ · X25519MLKEM768）' },
  { value: 'chrome140', label: 'chrome140（PQ · X25519MLKEM768）' },
  { value: 'firefox', label: 'firefox' },
  { value: 'firefox148', label: 'firefox148（PQ · X25519MLKEM768）' },
  { value: 'safari', label: 'safari' },
  { value: 'safari26_3', label: 'safari26_3（PQ · X25519MLKEM768）' },
  { value: 'ios', label: 'ios' },
  { value: 'android', label: 'android' },
  { value: 'edge', label: 'edge' },
  { value: 'random', label: 'random' },
]

const groups = ref<Group[]>([])
const activeTab = ref('profile')
const props = defineProps<{ visible: boolean; node: Node | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; saved: [] }>()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const genReality = ref(false)
const isEdit = computed(() => !!props.node)

const form = reactive({
  profile: 'reality' as ProfileId,
  name: '',
  scope_mode: 'grouped' as 'unassigned' | 'grouped',
  group_ids: [] as number[],
  node_type: 'vless' as string,
  cipher: 'aes-256-gcm',
  host: '',
  port: 443,
  network: 'tcp' as string,
  tls: 2,
  sni: '',
  path: '/',
  service_name: '',
  flow: 'xtls-rprx-vision',
  speed_limit: 0,
  enable: true,
  reality_public_key: '',
  reality_private_key: '',
  reality_short_id: '',
  reality_fingerprint: 'chrome',
  reality_dest: '',
  reality_min_client_ver: '1.8.0',
  reality_mldsa65_seed: '',
  reality_mldsa65_verify: '',
  reality_show: false,
  vless_decryption: '',
  vless_encryption: '',
})

const isReality = computed(() => form.profile === 'reality' || (form.node_type === 'vless' && form.tls === 2))
const isCDN = computed(
  () =>
    form.profile === 'cdn' ||
    (form.node_type === 'vless' && form.tls === 1 && (form.network === 'xhttp' || form.network === 'splithttp')),
)
const isAnyTLS = computed(() => form.profile === 'anytls' || form.node_type === 'anytls')
const showPathField = computed(
  () =>
    form.network === 'ws' ||
    form.network === 'xhttp' ||
    form.network === 'splithttp' ||
    form.network === 'httpupgrade',
)
const currentProfileMeta = computed(() => profiles.find((p) => p.id === form.profile))
const footHint = computed(() => {
  if (isCDN.value) return '订阅将下发 type=xhttp + TLS，一般不带 flow'
  if (isReality.value) return '订阅将下发 security=reality + flow/pbk/sid'
  if (isAnyTLS.value) return '订阅按 AnyTLS 规范，密码=用户 UUID'
  return '保存后节点通过 UniProxy 拉 config'
})

const validateScope = (_rule: any, _value: any, callback: any) => {
  if (form.scope_mode === 'grouped' && form.group_ids.length === 0) {
    callback(new Error('请至少选择一个权限组'))
  } else {
    callback()
  }
}
const rules: FormRules = {
  name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入地址', trigger: 'blur' }],
  scope_mode: [{ validator: validateScope, trigger: 'change' }],
}

function tlsLabel(t: number) {
  if (t === 2) return 'REALITY'
  if (t === 1) return 'TLS'
  return '无TLS'
}

function applyProfile(id: ProfileId) {
  form.profile = id
  if (id === 'reality') {
    form.node_type = 'vless'
    form.network = 'tcp'
    form.tls = 2
    form.flow = 'xtls-rprx-vision'
    form.path = '/'
    if (!form.reality_min_client_ver) form.reality_min_client_ver = '1.8.0'
  } else if (id === 'cdn') {
    form.node_type = 'vless'
    form.network = 'xhttp'
    form.tls = 1
    form.flow = ''
    if (!form.path || form.path === '/') form.path = '/vless-cdn'
  } else if (id === 'anytls') {
    form.node_type = 'anytls'
    form.network = 'tcp'
    form.tls = 1
    form.flow = ''
  }
  // custom: keep current fields
}

function detectProfile(n: Node): ProfileId {
  if (n.node_type === 'anytls') return 'anytls'
  const net = (n.network || '').toLowerCase()
  if (n.node_type === 'vless' && n.tls === 2 && (net === 'tcp' || net === '')) return 'reality'
  if (n.node_type === 'vless' && n.tls === 1 && (net === 'xhttp' || net === 'splithttp')) return 'cdn'
  if (n.node_type === 'vless' && n.tls === 2) return 'reality'
  return 'custom'
}

function onScopeModeChange(mode: 'unassigned' | 'grouped') {
  if (mode === 'unassigned') form.group_ids = []
  formRef.value?.clearValidate('scope_mode')
}
function toggleGroup(gid: number) {
  const idx = form.group_ids.indexOf(gid)
  if (idx >= 0) form.group_ids.splice(idx, 1)
  else form.group_ids.push(gid)
  formRef.value?.clearValidate('scope_mode')
}
function onNodeTypeChange(nt: string) {
  if (nt === 'anytls') {
    form.tls = 1
    form.flow = ''
    form.profile = 'anytls'
  } else if (form.profile === 'anytls') {
    form.profile = 'custom'
  }
}
function onNetworkChange(net: string) {
  if (net === 'xhttp' || net === 'splithttp') {
    form.tls = 1
    form.flow = ''
    if (form.profile === 'reality') form.profile = 'cdn'
  }
  if (net === 'tcp' && form.tls === 2) form.profile = 'reality'
}

function resetReality() {
  form.reality_public_key = ''
  form.reality_private_key = ''
  form.reality_short_id = ''
  form.reality_fingerprint = 'chrome'
  form.reality_dest = ''
  form.reality_min_client_ver = '1.8.0'
  form.reality_mldsa65_seed = ''
  form.reality_mldsa65_verify = ''
  form.reality_show = false
}

function resetForm() {
  form.profile = 'reality'
  form.name = ''
  form.scope_mode = 'grouped'
  form.group_ids = []
  form.node_type = 'vless'
  form.cipher = 'aes-256-gcm'
  form.host = ''
  form.port = 443
  form.network = 'tcp'
  form.tls = 2
  form.sni = ''
  form.path = '/'
  form.service_name = ''
  form.flow = 'xtls-rprx-vision'
  form.speed_limit = 0
  form.enable = true
  form.vless_decryption = ''
  form.vless_encryption = ''
  resetReality()
  formRef.value?.resetFields()
  activeTab.value = 'profile'
}

watch(
  () => props.visible,
  (val) => {
    if (val && props.node) {
      const n = props.node
      const ids = Array.isArray(n.group_ids) ? n.group_ids : n.group_id ? [n.group_id] : []
      form.name = n.name
      form.scope_mode = ids.length === 0 ? 'unassigned' : 'grouped'
      form.group_ids = ids
      form.node_type = n.node_type
      form.cipher = n.cipher || 'aes-256-gcm'
      form.host = n.host
      form.port = n.port
      form.network = n.network || 'tcp'
      form.tls = n.tls
      form.sni = n.sni
      form.path = n.path || '/'
      form.service_name = n.service_name
      form.flow = n.flow || ''
      form.speed_limit = n.speed_limit
      form.enable = n.enable
      form.vless_decryption = n.vless_decryption || ''
      form.vless_encryption = n.vless_encryption || ''
      form.profile = detectProfile(n)
      if (n.reality_settings) {
        try {
          const rs = typeof n.reality_settings === 'string' ? JSON.parse(n.reality_settings) : n.reality_settings
          form.reality_public_key = rs.public_key || ''
          form.reality_private_key = rs.private_key || ''
          form.reality_short_id = rs.short_id || ''
          form.reality_fingerprint = rs.fingerprint || 'chrome'
          form.reality_dest = rs.dest || ''
          form.reality_min_client_ver = rs.min_client_ver || rs.minClientVer || '1.8.0'
          form.reality_mldsa65_seed = rs.mldsa65_seed || rs.mldsa65Seed || ''
          form.reality_mldsa65_verify = rs.mldsa65_verify || rs.mldsa65Verify || ''
          form.reality_show = !!rs.show
        } catch {
          resetReality()
        }
      } else {
        resetReality()
      }
      activeTab.value = 'basic'
    } else if (val) {
      resetForm()
    }
  },
)

async function handleGenerateReality() {
  genReality.value = true
  try {
    const r = await request.get('/admin/nodes/reality/generate', { params: { sni: form.sni } })
    const d = r.data
    form.reality_public_key = d.public_key
    form.reality_private_key = d.private_key
    form.reality_short_id = d.short_id
    form.reality_fingerprint = d.fingerprint || 'chrome'
    form.reality_dest = d.dest || (form.sni ? `${form.sni}:443` : '')
    ElMessage.success('REALITY 参数已生成')
  } catch {
    ElMessage.error('生成失败')
  }
  genReality.value = false
}

async function handleSubmit() {
  // Ensure basic tab form is validated
  activeTab.value = 'basic'
  await new Promise((r) => setTimeout(r, 50))
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    ElMessage.warning('请完善接入参数（名称、地址、权限组）')
    return
  }

  if (isCDN.value && form.tls === 2) {
    ElMessage.error('CDN 线路请使用 TLS，不要使用 REALITY')
    activeTab.value = 'tls'
    return
  }
  if (form.tls === 2 && form.reality_mldsa65_seed && form.reality_mldsa65_seed === form.reality_private_key) {
    ElMessage.error('ML-DSA Seed 不能与 REALITY Private Key 相同')
    activeTab.value = 'tls'
    return
  }
  if (form.tls === 2 && (!form.reality_public_key || !form.reality_private_key || !form.reality_short_id)) {
    ElMessage.warning('REALITY 节点请填写或生成密钥对')
    activeTab.value = 'tls'
  }

  const dec = (form.vless_decryption || '').trim()
  const enc = (form.vless_encryption || '').trim()
  if ((dec && dec !== 'none' && !enc) || (enc && (!dec || dec === 'none'))) {
    ElMessage.warning('VLESS Encryption 需成对填写 decryption 与 encryption，或全部留空')
  }

  // CDN: force no vision flow
  let flow = form.flow
  if (isCDN.value || form.network === 'xhttp' || form.network === 'splithttp') {
    flow = ''
  }

  let realitySettings: any = null
  if (form.tls === 2) {
    realitySettings = {
      public_key: form.reality_public_key,
      private_key: form.reality_private_key,
      short_id: form.reality_short_id,
      fingerprint: form.reality_fingerprint,
      dest: form.reality_dest,
      min_client_ver: (form.reality_min_client_ver || '1.8.0').trim() || '1.8.0',
    }
    const seed = (form.reality_mldsa65_seed || '').trim()
    const verify = (form.reality_mldsa65_verify || '').trim()
    if (seed) realitySettings.mldsa65_seed = seed
    if (verify) realitySettings.mldsa65_verify = verify
    if (form.reality_show) realitySettings.show = true
  }

  submitting.value = true
  try {
    const base = {
      name: form.name,
      group_ids: form.scope_mode === 'unassigned' ? [] : form.group_ids,
      node_type: form.node_type,
      host: form.host,
      port: form.port,
      network: form.network,
      tls: form.tls,
      sni: form.sni,
      path: form.path,
      service_name: form.service_name,
      flow,
      speed_limit: form.speed_limit,
      cipher: form.cipher,
      reality_settings: realitySettings,
      vless_decryption: form.vless_decryption || '',
      vless_encryption: form.vless_encryption || '',
    }
    if (isEdit.value) {
      await updateNode(props.node!.id, { ...base, enable: form.enable })
    } else {
      await createNode(base)
    }
    ElMessage.success(isEdit.value ? '已更新' : '已创建')
    emit('update:visible', false)
    emit('saved')
  } catch {
    /* interceptor */
  }
  submitting.value = false
}

async function fetchGroups() {
  try {
    const r = await request.get('/admin/groups')
    groups.value = r.data || []
  } catch {
    /* */
  }
}
onMounted(fetchGroups)
</script>

<style scoped>
.tab-lead {
  margin: 0 0 14px;
  font-size: 13px;
  color: #64748b;
  line-height: 1.5;
}
.profile-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
@media (max-width: 640px) {
  .profile-grid {
    grid-template-columns: 1fr;
  }
}
.profile-card {
  text-align: left;
  border: 1.5px solid #e5e7eb;
  border-radius: 14px;
  padding: 14px 14px 12px;
  background: #fff;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s, transform 0.12s;
  font-family: inherit;
}
.profile-card:hover {
  border-color: #c7d2fe;
  box-shadow: 0 4px 14px rgba(79, 70, 229, 0.08);
  transform: translateY(-1px);
}
.profile-card.active {
  border-color: #4f46e5;
  box-shadow: 0 0 0 1px #4f46e5, 0 6px 16px rgba(79, 70, 229, 0.12);
  background: #f8faff;
}
.profile-card.tone-cyan.active {
  border-color: #0891b2;
  box-shadow: 0 0 0 1px #0891b2, 0 6px 16px rgba(8, 145, 178, 0.12);
  background: #f0fdfa;
}
.profile-card.tone-amber.active {
  border-color: #d97706;
  box-shadow: 0 0 0 1px #d97706, 0 6px 16px rgba(217, 119, 6, 0.1);
  background: #fffbeb;
}
.pc-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.pc-badge {
  font-size: 11px;
  font-weight: 700;
  color: #4f46e5;
  background: #eef2ff;
  padding: 2px 8px;
  border-radius: 999px;
}
.tone-cyan .pc-badge {
  color: #0e7490;
  background: #cffafe;
}
.tone-amber .pc-badge {
  color: #b45309;
  background: #fef3c7;
}
.tone-slate .pc-badge {
  color: #475569;
  background: #f1f5f9;
}
.pc-check {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #4f46e5;
  color: #fff;
  font-size: 12px;
  font-weight: 800;
  display: grid;
  place-items: center;
}
.pc-title {
  font-size: 14px;
  font-weight: 800;
  color: #0f172a;
  margin-bottom: 6px;
  letter-spacing: -0.02em;
}
.pc-desc {
  font-size: 12px;
  color: #64748b;
  line-height: 1.45;
  margin-bottom: 10px;
  min-height: 34px;
}
.pc-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.pc-tags span {
  font-size: 10px;
  font-weight: 600;
  color: #475569;
  background: #f1f5f9;
  padding: 2px 7px;
  border-radius: 6px;
}
.profile-summary {
  margin-top: 14px;
  padding: 10px 12px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  font-size: 12px;
  color: #475569;
}
.profile-summary code {
  font-size: 11px;
  background: #eef2ff;
  color: #4338ca;
  padding: 1px 6px;
  border-radius: 4px;
}
.sum-sep {
  margin: 0 4px;
  color: #cbd5e1;
}
.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.45;
}
.field-hint code {
  font-size: 11px;
  background: #f1f5f9;
  color: #475569;
  padding: 0 4px;
  border-radius: 3px;
}
.field-hint strong {
  color: #64748b;
  font-weight: 700;
}
.field-hint.inline {
  margin-left: 10px;
  display: inline;
}
.mb-12 {
  margin-bottom: 12px;
}
.ml-8 {
  margin-left: 8px;
}
.adv-collapse {
  margin: 4px 0 12px;
  border: none;
}
.adv-collapse :deep(.el-collapse-item__header) {
  font-size: 12px;
  color: #64748b;
  height: 36px;
  background: #f8fafc;
  border-radius: 8px;
  padding: 0 12px;
  border: 1px solid #e5e7eb;
}
.dlg-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  width: 100%;
}
.foot-hint {
  font-size: 12px;
  color: #94a3b8;
  text-align: left;
  flex: 1;
}
.scope-section {
  width: 100%;
}
.scope-radio-group {
  margin-bottom: 12px;
}
.radio-label {
  font-weight: 600;
}
.scope-global-hint.warn {
  display: flex;
  padding: 12px 14px;
  background: #fff7ed;
  border: 1px solid #fed7aa;
  border-radius: 8px;
  color: #9a3412;
  font-size: 13px;
  line-height: 1.5;
}
.scope-empty {
  padding: 16px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}
.group-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.group-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border: 1.5px solid #e5e7eb;
  border-radius: 8px;
  cursor: pointer;
  background: #fff;
  transition: 0.15s ease;
}
.group-chip:hover {
  border-color: #a5b4fc;
}
.group-chip.selected {
  border-color: #4f46e5;
  background: #eef2ff;
}
.chip-check {
  width: 16px;
  height: 16px;
  border-radius: 4px;
  border: 1.5px solid #d1d5db;
  display: grid;
  place-items: center;
  font-size: 10px;
  font-weight: 800;
  color: #fff;
}
.group-chip.selected .chip-check {
  background: #4f46e5;
  border-color: #4f46e5;
}
.chip-name {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
}
.scope-footer {
  margin-top: 10px;
  font-size: 13px;
}
.selected-count {
  color: #16a34a;
}
.selected-count strong {
  color: #4f46e5;
}
.validation-msg {
  color: #dc2626;
}
.node-tabs :deep(.el-tabs__item) {
  font-weight: 600;
}
</style>
