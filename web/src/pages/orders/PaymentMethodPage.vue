<template>
  <div class="pm-page">
    <div class="k2-page-header">
      <div>
        <h3>支付方式</h3>
        <p class="sub">按渠道填写表单配置 · 密钥以输入框管理，无需手写 JSON</p>
      </div>
      <el-button type="primary" size="large" @click="showCreate" :disabled="!availableCodes.length">
        <el-icon><Plus /></el-icon>
        添加方式
      </el-button>
      <p class="header-tip" style="margin:8px 0 0;font-size:13px;color:#64748b">
        青蛙 / 易支付可添加<strong>多个通道</strong>（如微信、支付宝各一条），用户端以卡片同时显示。
        <strong>排序数字越小越靠前</strong>（用户端按排序展示）。
      </p>
    </div>

    <div class="card-grid" v-loading="loading">
      <div v-for="m in methods" :key="m.id" class="pm-card" :class="{ off: !m.enable }">
        <div class="pm-top">
          <div>
            <div class="pm-name">{{ m.name }}</div>
            <div class="pm-code">{{ gatewayLabel(m.code) }} · {{ m.code }}</div>
          </div>
          <span class="k2-enable-pill" :class="m.enable ? 'yes' : 'no'">
            {{ m.enable ? '启用' : '禁用' }}
          </span>
        </div>
        <p class="pm-remark">{{ m.remark || '无备注' }}</p>
        <div class="pm-actions">
          <span class="k2-id-pill">#{{ m.id }} · 排序 {{ m.sort }}</span>
          <div>
            <el-button size="small" class="k2-action-btn" @click="showEdit(m)">编辑</el-button>
            <el-popconfirm
              title="确认删除？"
              @confirm="handleDelete(m.id)"
            >
              <template #reference>
                <el-button size="small" class="k2-action-btn danger">删除</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </div>
      <div v-if="!loading && !methods.length" class="empty-hint">
        暂无支付方式，点击右上角添加
      </div>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑支付方式' : '添加支付方式'"
      width="640px"
      align-center
      destroy-on-close
      class="pm-dialog"
      :close-on-click-modal="false"
    >
      <div class="dlg-scroll">
        <!-- 基础信息 -->
        <section class="dlg-section">
          <div class="sec-title">基础信息</div>
          <div class="form-grid">
            <div class="field" v-if="!editing">
              <label>支付网关</label>
              <el-select v-model="form.code" style="width:100%" size="default" @change="onCodeChange">
                <el-option
                  v-for="c in availableCodes"
                  :key="c"
                  :label="gatewayLabel(c)"
                  :value="c"
                />
              </el-select>
            </div>
            <div class="field" v-else>
              <label>支付网关</label>
              <el-input :model-value="gatewayLabel(configKind) + ' (' + form.code + ')'" disabled size="default" />
            </div>
            <div class="field" v-if="!editing && isMultiGateway(form.code)">
              <label>通道实例标识</label>
              <el-input
                v-model="form.instance_suffix"
                size="default"
                placeholder="如 alipay / wx（保存为 frog_alipay）"
                clearable
              />
            </div>
            <div class="field">
              <label>显示名称</label>
              <el-input v-model="form.name" size="default" placeholder="用户端展示：支付宝 / 微信支付" />
            </div>
            <div class="field sm">
              <label>排序 <span class="opt">越小越靠前</span></label>
              <el-input-number v-model="form.sort" :min="0" :max="999" controls-position="right" style="width:100%" />
            </div>
            <div class="field sm row-enable">
              <label>启用</label>
              <el-switch v-model="form.enable" active-text="开" inactive-text="关" />
            </div>
            <div class="field full">
              <label>备注 <span class="opt">可选</span></label>
              <el-input v-model="form.remark" size="default" placeholder="内部备注，不对用户展示" />
            </div>
          </div>
        </section>

        <!-- 渠道参数：支付宝 -->
        <section v-if="configKind === 'alipay'" class="dlg-section">
          <div class="sec-title">
            支付宝参数
            <span class="sec-tip">开放平台应用 · 请用支付宝公钥（非应用公钥）</span>
          </div>
          <div class="form-grid">
            <div class="field">
              <label>App ID <span class="req">*</span></label>
              <el-input v-model="cfg.alipay.app_id" size="default" placeholder="20xxxxxxxxxxxx" clearable />
            </div>
            <div class="field">
              <label>支付产品</label>
              <el-select v-model="cfg.alipay.product" style="width:100%" size="default">
                <el-option label="电脑网站支付 (page)" value="page" />
                <el-option label="手机网站支付 (wap)" value="wap" />
              </el-select>
            </div>
            <div class="field">
              <label>环境</label>
              <el-radio-group v-model="cfg.alipay.is_production" size="default">
                <el-radio-button :value="true">正式</el-radio-button>
                <el-radio-button :value="false">沙箱</el-radio-button>
              </el-radio-group>
            </div>
            <div class="field">
              <label>订单超时</label>
              <el-input v-model="cfg.alipay.timeout_express" size="default" placeholder="30m" />
            </div>
            <div class="field full">
              <label>应用私钥 <span class="req">*</span></label>
              <el-input
                v-model="cfg.alipay.private_key"
                type="textarea"
                :rows="3"
                resize="vertical"
                placeholder="-----BEGIN RSA PRIVATE KEY----- 或 PKCS8"
                class="mono-input"
              />
            </div>
            <div class="field full">
              <label>支付宝公钥 <span class="req">*</span></label>
              <el-input
                v-model="cfg.alipay.alipay_public_key"
                type="textarea"
                :rows="3"
                resize="vertical"
                placeholder="-----BEGIN PUBLIC KEY-----（开放平台「支付宝公钥」）"
                class="mono-input"
              />
            </div>
          </div>
          <div class="callout">
            异步通知：<code>{site_url}/api/v1/payment/notify/alipay</code>
            · 请在系统设置填写公网 site_url，并在开放平台配置相同回调地址
          </div>
        </section>

        <!-- 渠道参数：Epusdt / GM Pay -->
        <section v-else-if="configKind === 'epusdt'" class="dlg-section">
          <div class="sec-title">
            Epusdt (GM Pay) 参数
            <span class="sec-tip">
              <a href="https://github.com/GMWalletApp/epusdt" target="_blank" rel="noopener">GMWalletApp/epusdt</a>
              · 需在支付端配置钱包地址
            </span>
          </div>
          <div class="form-grid">
            <div class="field full">
              <label>网关地址 base_url <span class="req">*</span></label>
              <el-input v-model="cfg.epusdt.base_url" size="default" placeholder="https://upay.example.com" clearable />
            </div>
            <div class="field">
              <label>商户 PID <span class="req">*</span></label>
              <el-input v-model="cfg.epusdt.pid" size="default" placeholder="1000" clearable />
            </div>
            <div class="field">
              <label>法币 currency</label>
              <el-select v-model="cfg.epusdt.currency" style="width:100%" size="default">
                <el-option label="cny" value="cny" />
                <el-option label="usd" value="usd" />
              </el-select>
            </div>
            <div class="field full">
              <label>Secret Key <span class="req">*</span></label>
              <el-input v-model="cfg.epusdt.secret_key" size="default" type="password" show-password placeholder="API Keys 中的 secret_key" clearable />
            </div>
            <div class="field">
              <label>币种 token</label>
              <el-select v-model="cfg.epusdt.token" style="width:100%" size="default" filterable allow-create clearable placeholder="收银台自选">
                <el-option label="收银台自选（推荐多链）" value="" />
                <el-option label="usdt" value="usdt" />
                <el-option label="usdc" value="usdc" />
                <el-option label="trx" value="trx" />
              </el-select>
            </div>
            <div class="field">
              <label>网络 network</label>
              <el-select v-model="cfg.epusdt.network" style="width:100%" size="default" filterable allow-create clearable placeholder="收银台自选">
                <el-option label="收银台自选（推荐多链）" value="" />
                <el-option label="tron (TRC20)" value="tron" />
                <el-option label="ethereum" value="ethereum" />
                <el-option label="bsc / binance" value="binance" />
                <el-option label="polygon" value="polygon" />
                <el-option label="solana" value="solana" />
              </el-select>
            </div>
            <div class="field full">
              <label>改写收银台域名</label>
              <el-switch v-model="cfg.epusdt.rewrite_payment_host" active-text="用 base_url 替换内网 IP" inactive-text="保留原 payment_url" />
            </div>
          </div>
          <div class="callout">
            <p>
              异步通知：<code>{site_url}/api/v1/payment/notify/epusdt</code>
              · 创建：<code>POST {base_url}/payments/gmpay/v1/order/create-transaction</code>
            </p>
            <p>
              <strong>多链建议</strong>：token/network 都留空 → 支付端创建「待选链」占位单，用户首次选链绑定父订单，一般不会出现 TRON 父单 + Solana 子单。
              若锁定为 tron 再切换 solana，支付端会生成最多 1 条子订单，且<strong>仅父订单回调</strong>（属 GMPay 设计）。
            </p>
            <p>
              <strong>取消订单</strong>：面板关单只更新本地面板状态；GMPay 公开 API 无商户关单接口，支付端待付款会在过期任务后变为已过期。可在 upay 后台对单笔手动关闭。
            </p>
          </div>
        </section>

        <!-- 渠道参数：青蛙四方 -->
        <section v-else-if="configKind === 'frog'" class="dlg-section">
          <div class="sec-title">
            青蛙四方参数
            <span class="sec-tip">JSON API · MD5 签名 · POST /pay/create</span>
          </div>
          <div class="form-grid">
            <div class="field full">
              <label>网关地址 base_url <span class="req">*</span></label>
              <el-input v-model="cfg.frog.base_url" size="default" placeholder="https://pay.pp.qwgua.com" clearable />
            </div>
            <div class="field">
              <label>路径前缀 path_prefix</label>
              <el-input v-model="cfg.frog.path_prefix" size="default" placeholder="rest（默认）" clearable />
            </div>
            <div class="field">
              <label>商户号 mch_id <span class="req">*</span></label>
              <el-input v-model="cfg.frog.mch_id" size="default" placeholder="8888888888" clearable />
            </div>
            <div class="field">
              <label>通道编码 code <span class="req">*</span></label>
              <el-input v-model="cfg.frog.code" size="default" placeholder="1–4 位通道码" maxlength="4" clearable />
            </div>
            <div class="field full">
              <label>商户密钥 key <span class="req">*</span></label>
              <el-input v-model="cfg.frog.key" size="default" type="password" show-password placeholder="商户密钥（签名用）" clearable />
            </div>
            <div class="field full">
              <label>商品标题 <span class="opt">可选</span></label>
              <el-input v-model="cfg.frog.product_name" size="default" placeholder="数字商品（{plan_name} {trade_tail}）" clearable />
            </div>
            <div class="field">
              <label>device</label>
              <el-input v-model="cfg.frog.device" size="default" placeholder="web" maxlength="10" clearable />
            </div>
          </div>
          <div class="callout">
            <p>
              正式接口（运营）：
              <code>POST {base_url}/rest/pay/create</code> ·
              <code>/rest/pay/query</code> ·
              <code>/rest/pay/balance</code>
            </p>
            <p>
              <strong>base_url</strong> 填主机根，例如 <code>https://pay.pp.qwgua.com</code>；
              默认自动加前缀 <code>/rest</code>。若已写成
              <code>https://pay.pp.qwgua.com/rest</code> 也不会重复。
              无 rest 的网关将 path_prefix 设为 <code>none</code>。
            </p>
            <p>
              异步通知：<code>{site_url}/api/v1/payment/notify/frog</code>
              （POST JSON，status=3 成功，应答 <code>success</code>）。
              同步回跳：下单时带 <code>returnUrl</code>（用户站订单结果页）。
            </p>
            <p>
              与彩虹易支付<strong>不是</strong>同一协议；通道 <code>code</code> 向运营索取。
              请配置系统 <code>site_url</code>。
            </p>
          </div>
        </section>

        <!-- 渠道参数：彩虹易支付 -->
        <section v-else-if="configKind === 'epay'" class="dlg-section">
          <div class="sec-title">
            彩虹易支付参数
            <span class="sec-tip">标准 V1 MD5 · submit.php 页面跳转 · 单商户</span>
          </div>
          <div class="form-grid">
            <div class="field full">
              <label>网关地址 base_url <span class="req">*</span></label>
              <el-input v-model="cfg.epay.base_url" size="default" placeholder="https://pay.example.com" clearable />
            </div>
            <div class="field">
              <label>商户 PID <span class="req">*</span></label>
              <el-input v-model="cfg.epay.pid" size="default" placeholder="1001" clearable />
            </div>
            <div class="field">
              <label>支付方式 type</label>
              <el-select v-model="cfg.epay.type" style="width:100%" size="default" clearable placeholder="收银台自选">
                <el-option label="收银台自选（推荐）" value="" />
                <el-option label="支付宝 alipay" value="alipay" />
                <el-option label="微信支付 wxpay" value="wxpay" />
                <el-option label="QQ钱包 qqpay" value="qqpay" />
                <el-option label="USDT usdt" value="usdt" />
              </el-select>
            </div>
            <div class="field full">
              <label>商户密钥 key <span class="req">*</span></label>
              <el-input v-model="cfg.epay.key" size="default" type="password" show-password placeholder="易支付商户后台密钥" clearable />
            </div>
            <div class="field full">
              <label>商品名称 <span class="opt">可选</span></label>
              <el-input v-model="cfg.epay.product_name" size="default" placeholder="数字商品（可用 {plan_name} {trade_tail}）" clearable />
            </div>
          </div>
          <div class="callout">
            <p>
              异步通知：<code>{site_url}/api/v1/payment/notify/epay</code>
              （支持 GET/POST，应答 <code>success</code>）
            </p>
            <p>
              创建：跳转 <code>{base_url}/submit.php</code>；查单：
              <code>{base_url}/api.php?act=order</code>
            </p>
            <p>
              <strong>type 留空</strong>：用户在易支付收银台自行选择支付宝/微信等。
              固定 type 则直达对应通道（取决于易支付站点是否开通）。
            </p>
          </div>
        </section>

        <!-- 渠道参数：BEpusdt（旧版） -->
        <section v-else-if="configKind === 'bepusdt'" class="dlg-section">
          <div class="sec-title">
            BEpusdt 参数（旧版 API）
            <span class="sec-tip">v03413/bepusdt · 若使用 GMWalletApp/epusdt 请选 epusdt</span>
          </div>
          <div class="form-grid">
            <div class="field full">
              <label>网关地址 base_url <span class="req">*</span></label>
              <el-input v-model="cfg.bepusdt.base_url" size="default" placeholder="https://pay.example.com" clearable />
            </div>
            <div class="field full">
              <label>API Token <span class="req">*</span></label>
              <el-input v-model="cfg.bepusdt.api_token" size="default" type="password" show-password placeholder="对接令牌" clearable />
            </div>
            <div class="field">
              <label>交易类型</label>
              <el-select v-model="cfg.bepusdt.trade_type" style="width:100%" size="default" filterable allow-create>
                <el-option label="USDT TRC20" value="usdt.trc20" />
                <el-option label="USDT ERC20" value="usdt.erc20" />
                <el-option label="USDT BSC" value="usdt.bep20" />
                <el-option label="TRX" value="tron.trx" />
              </el-select>
            </div>
            <div class="field">
              <label>法币</label>
              <el-select v-model="cfg.bepusdt.fiat" style="width:100%" size="default">
                <el-option label="CNY 人民币" value="CNY" />
                <el-option label="USD 美元" value="USD" />
              </el-select>
            </div>
            <div class="field">
              <label>超时（秒）</label>
              <el-input-number v-model="cfg.bepusdt.timeout" :min="120" :max="86400" :step="60" controls-position="right" style="width:100%" />
            </div>
          </div>
          <div class="callout">
            异步通知：<code>{site_url}/api/v1/payment/notify/bepusdt</code>
          </div>
        </section>

        <!-- 未知网关兜底：仍提供键值表单项难度大，给简易 JSON 仅未知 code -->
        <section v-else class="dlg-section">
          <div class="sec-title">渠道配置</div>
          <div class="field full">
            <label>配置 JSON</label>
            <el-input v-model="cfg.raw" type="textarea" :rows="4" class="mono-input" />
          </div>
        </section>
      </div>

      <template #footer>
        <div class="dlg-foot">
          <span class="foot-hint">保存后对用户端立即生效（启用时）</span>
          <div class="foot-actions">
            <el-button @click="dialogVisible = false">取消</el-button>
            <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import request from '@/api/request'

const methods = ref<any[]>([])
const gatewayCodes = ref<string[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref(false)
const editId = ref(0)
const saving = ref(false)

const form = reactive({
  code: 'frog', // create: gateway id；edit: 完整 method code（如 frog_alipay）
  instance_suffix: '', // create multi: alipay / wx → frog_alipay
  name: '',
  enable: false,
  sort: 0,
  remark: '',
})

/** 可多实例网关：同一实现可挂多条支付方式（不同通道 code / 显示名） */
const multiInstanceGateways = ref<string[]>(['frog', 'epay'])

function gatewayKind(code: string): string {
  const c = (code || '').trim()
  const i = c.indexOf('_')
  if (i > 0) {
    const base = c.slice(0, i)
    if (multiInstanceGateways.value.includes(base) || gatewayCodes.value.includes(base)) {
      return base
    }
  }
  return c
}

function isMultiGateway(code: string) {
  return multiInstanceGateways.value.includes(gatewayKind(code))
}

const configKind = computed(() => gatewayKind(form.code))

/** Structured channel config (serialized to JSON on save) */
const cfg = reactive({
  alipay: {
    app_id: '',
    private_key: '',
    alipay_public_key: '',
    is_production: true,
    product: 'page',
    timeout_express: '30m',
  },
  epusdt: {
    base_url: '',
    pid: '1000',
    secret_key: '',
    token: '', // empty = cashier select (status 4)
    network: '',
    currency: 'cny',
    rewrite_payment_host: true,
  },
  bepusdt: {
    base_url: '',
    api_token: '',
    trade_type: 'usdt.trc20',
    timeout: 1200,
    fiat: 'CNY',
  },
  epay: {
    base_url: '',
    pid: '',
    key: '',
    type: '',
    product_name: '数字商品',
  },
  frog: {
    base_url: '',
    path_prefix: 'rest',
    mch_id: '',
    key: '',
    code: '',
    product_name: '数字商品',
    device: 'web',
  },
  raw: '{}',
})

const usedCodes = computed(() => new Set(methods.value.map((m) => m.code)))
/** 单实例网关用过后隐藏；frog/epay 可反复添加 */
const availableCodes = computed(() =>
  gatewayCodes.value.filter((c) => {
    if (multiInstanceGateways.value.includes(c)) return true
    return !usedCodes.value.has(c) || (editing.value && gatewayKind(form.code) === c)
  }),
)

function gatewayLabel(code: string) {
  const base = gatewayKind(code)
  const m: Record<string, string> = {
    alipay: '支付宝',
    epay: '彩虹易支付',
    frog: '青蛙四方',
    epusdt: 'USDT (Epusdt/GMPay)',
    bepusdt: 'USDT (BEpusdt 旧版)',
    giftcard: '发卡收银台',
  }
  const label = m[base] || base
  if (code.includes('_')) return `${label} · ${code}`
  return label
}

/** 生成保存用的 method code：frog + alipay → frog_alipay */
function resolveMethodCode(): string | null {
  if (editing.value) return form.code
  const base = (form.code || '').trim().toLowerCase()
  if (!base) return null
  if (!isMultiGateway(base)) return base
  let suf = (form.instance_suffix || '').trim().toLowerCase().replace(/[^a-z0-9]/g, '')
  if (!suf) {
    // 允许第一条用裸 frog；若已占用则要求后缀
    if (usedCodes.value.has(base)) return null
    return base
  }
  return `${base}_${suf}`
}

function resetCfg(code: string) {
  Object.assign(cfg.alipay, {
    app_id: '',
    private_key: '',
    alipay_public_key: '',
    is_production: true,
    product: 'page',
    timeout_express: '30m',
  })
  Object.assign(cfg.epusdt, {
    base_url: '',
    pid: '1000',
    secret_key: '',
    token: '',
    network: '',
    currency: 'cny',
    rewrite_payment_host: true,
  })
  Object.assign(cfg.bepusdt, {
    base_url: '',
    api_token: '',
    trade_type: 'usdt.trc20',
    timeout: 1200,
    fiat: 'CNY',
  })
  Object.assign(cfg.epay, {
    base_url: '',
    pid: '',
    key: '',
    type: '',
    product_name: '数字商品',
  })
  Object.assign(cfg.frog, {
    base_url: '',
    path_prefix: 'rest',
    mch_id: '',
    key: '',
    code: '',
    product_name: '数字商品',
    device: 'web',
  })
  cfg.raw = '{}'
  void code
}

function parseConfigIntoFields(code: string, configStr: string) {
  const kind = gatewayKind(code)
  resetCfg(kind)
  let obj: any = {}
  try {
    obj = JSON.parse(configStr || '{}') || {}
  } catch {
    cfg.raw = configStr || '{}'
    return
  }
  if (kind === 'alipay') {
    cfg.alipay.app_id = String(obj.app_id || '')
    cfg.alipay.private_key = String(obj.private_key || '')
    cfg.alipay.alipay_public_key = String(obj.alipay_public_key || '')
    cfg.alipay.is_production = obj.is_production !== false
    cfg.alipay.product = obj.product === 'wap' ? 'wap' : 'page'
    cfg.alipay.timeout_express = String(obj.timeout_express || '30m')
  } else if (kind === 'epusdt') {
    cfg.epusdt.base_url = String(obj.base_url || '')
    cfg.epusdt.pid = String(obj.pid || '1000')
    cfg.epusdt.secret_key = String(obj.secret_key || obj.api_token || '')
    const tok = obj.token == null ? '' : String(obj.token)
    const net = obj.network == null ? '' : String(obj.network)
    cfg.epusdt.token = ['auto', 'any', 'cashier', 'select'].includes(tok.toLowerCase()) ? '' : tok
    cfg.epusdt.network = ['auto', 'any', 'cashier', 'select'].includes(net.toLowerCase()) ? '' : net
    cfg.epusdt.currency = String(obj.currency || 'cny')
    cfg.epusdt.rewrite_payment_host = obj.rewrite_payment_host !== false
  } else if (kind === 'bepusdt') {
    cfg.bepusdt.base_url = String(obj.base_url || '')
    cfg.bepusdt.api_token = String(obj.api_token || '')
    cfg.bepusdt.trade_type = String(obj.trade_type || 'usdt.trc20')
    cfg.bepusdt.timeout = Number(obj.timeout) > 0 ? Number(obj.timeout) : 1200
    cfg.bepusdt.fiat = String(obj.fiat || 'CNY')
  } else if (kind === 'epay') {
    cfg.epay.base_url = String(obj.base_url || '')
    cfg.epay.pid = String(obj.pid || '')
    cfg.epay.key = String(obj.key || obj.api_token || obj.secret || '')
    const t = String(obj.type || '').toLowerCase()
    cfg.epay.type = ['auto', 'cashier', 'any', 'select'].includes(t) ? '' : t
    cfg.epay.product_name = String(obj.product_name || '数字商品')
  } else if (kind === 'frog') {
    cfg.frog.base_url = String(obj.base_url || '')
    cfg.frog.path_prefix = String(obj.path_prefix ?? 'rest')
    cfg.frog.mch_id = String(obj.mch_id || obj.mchId || '')
    cfg.frog.key = String(obj.key || obj.secret || obj.api_token || '')
    cfg.frog.code = String(obj.code || obj.channel || '')
    cfg.frog.product_name = String(obj.product_name || '数字商品')
    cfg.frog.device = String(obj.device || 'web')
  } else {
    cfg.raw = JSON.stringify(obj, null, 2)
  }
}

function buildConfigJSON(): string {
  const kind = configKind.value
  if (kind === 'alipay') {
    return JSON.stringify({
      app_id: cfg.alipay.app_id.trim(),
      private_key: cfg.alipay.private_key.trim(),
      alipay_public_key: cfg.alipay.alipay_public_key.trim(),
      is_production: !!cfg.alipay.is_production,
      product: cfg.alipay.product || 'page',
      timeout_express: cfg.alipay.timeout_express.trim() || '30m',
    })
  }
  if (kind === 'epusdt') {
    const token = (cfg.epusdt.token || '').trim()
    const network = (cfg.epusdt.network || '').trim()
    const out: Record<string, unknown> = {
      base_url: cfg.epusdt.base_url.trim().replace(/\/$/, ''),
      pid: cfg.epusdt.pid.trim() || '1000',
      secret_key: cfg.epusdt.secret_key.trim(),
      currency: cfg.epusdt.currency || 'cny',
      rewrite_payment_host: !!cfg.epusdt.rewrite_payment_host,
    }
    if (token || network) {
      out.token = token
      out.network = network
    }
    return JSON.stringify(out)
  }
  if (kind === 'bepusdt') {
    return JSON.stringify({
      base_url: cfg.bepusdt.base_url.trim().replace(/\/$/, ''),
      api_token: cfg.bepusdt.api_token.trim(),
      trade_type: cfg.bepusdt.trade_type || 'usdt.trc20',
      timeout: Number(cfg.bepusdt.timeout) || 1200,
      fiat: cfg.bepusdt.fiat || 'CNY',
    })
  }
  if (kind === 'epay') {
    const out: Record<string, unknown> = {
      base_url: cfg.epay.base_url.trim().replace(/\/$/, ''),
      pid: cfg.epay.pid.trim(),
      key: cfg.epay.key.trim(),
      product_name: (cfg.epay.product_name || '数字商品').trim() || '数字商品',
    }
    const t = (cfg.epay.type || '').trim()
    if (t) out.type = t
    return JSON.stringify(out)
  }
  if (kind === 'frog') {
    const out: Record<string, unknown> = {
      base_url: cfg.frog.base_url.trim().replace(/\/$/, ''),
      mch_id: cfg.frog.mch_id.trim(),
      key: cfg.frog.key.trim(),
      code: cfg.frog.code.trim(),
      product_name: (cfg.frog.product_name || '数字商品').trim() || '数字商品',
      device: (cfg.frog.device || 'web').trim() || 'web',
    }
    const pp = (cfg.frog.path_prefix || 'rest').trim()
    if (pp) out.path_prefix = pp
    return JSON.stringify(out)
  }
  try {
    JSON.parse(cfg.raw || '{}')
    return cfg.raw || '{}'
  } catch {
    throw new Error('配置 JSON 格式无效')
  }
}

function validateBeforeSave(): string | null {
  if (!form.name.trim()) return '请输入显示名称'
  if (!editing.value && isMultiGateway(form.code)) {
    const mc = resolveMethodCode()
    if (!mc) {
      return '该网关已有实例，请填写通道实例标识（如 alipay、wx），将保存为 frog_alipay'
    }
    if (usedCodes.value.has(mc)) return `标识 ${mc} 已存在，请换一个`
  }
  const kind = configKind.value
  if (kind === 'alipay') {
    if (!cfg.alipay.app_id.trim()) return '请填写 App ID'
    if (!cfg.alipay.private_key.trim()) return '请填写应用私钥'
    if (!cfg.alipay.alipay_public_key.trim()) return '请填写支付宝公钥'
  }
  if (kind === 'epusdt') {
    if (!cfg.epusdt.base_url.trim()) return '请填写网关地址'
    if (!cfg.epusdt.secret_key.trim()) return '请填写 Secret Key'
    if (!cfg.epusdt.pid.trim()) return '请填写商户 PID'
    const t = (cfg.epusdt.token || '').trim()
    const n = (cfg.epusdt.network || '').trim()
    if ((t && !n) || (!t && n)) return '币种与网络须同时填写，或都留空（收银台自选）'
  }
  if (kind === 'bepusdt') {
    if (!cfg.bepusdt.base_url.trim()) return '请填写网关地址'
    if (!cfg.bepusdt.api_token.trim()) return '请填写 API Token'
  }
  if (kind === 'epay') {
    if (!cfg.epay.base_url.trim()) return '请填写网关地址'
    if (!cfg.epay.pid.trim()) return '请填写商户 PID'
    if (!cfg.epay.key.trim()) return '请填写商户密钥 key'
  }
  if (kind === 'frog') {
    if (!cfg.frog.base_url.trim()) return '请填写网关地址 base_url'
    if (!cfg.frog.mch_id.trim()) return '请填写商户号 mch_id'
    if (!cfg.frog.key.trim()) return '请填写商户密钥 key'
    if (!cfg.frog.code.trim()) return '请填写通道编码 code（运营提供的微信/支付宝通道码）'
    if (cfg.frog.code.trim().length > 4) return '通道 code 最长 4 字符'
  }
  return null
}

async function fetchAll() {
  loading.value = true
  try {
    const [m, g] = await Promise.all([
      request.get('/admin/payment-methods'),
      request.get('/admin/payment-methods/gateways'),
    ])
    methods.value = m.data || []
    gatewayCodes.value = g.data?.codes || []
    if (Array.isArray(g.data?.multi_instance) && g.data.multi_instance.length) {
      multiInstanceGateways.value = g.data.multi_instance
    }
  } catch {
    methods.value = []
  } finally {
    loading.value = false
  }
}

function defaultRemark(code: string) {
  const k = gatewayKind(code)
  if (k === 'epay') return '彩虹易支付 / 标准易支付 MD5'
  if (k === 'frog') return '青蛙四方 · 可多通道（微信/支付宝各一条）'
  if (k === 'epusdt') return 'GMWalletApp/epusdt GM Pay USDT'
  if (k === 'bepusdt') return '自建 BEpusdt USDT 收款（旧 API）'
  if (k === 'alipay') return '支付宝开放平台网站支付'
  return ''
}

function defaultSort(code: string) {
  const k = gatewayKind(code)
  if (k === 'alipay') return 5
  if (k === 'frog') return 6
  if (k === 'epay') return 7
  if (k === 'epusdt') return 8
  if (k === 'bepusdt') return 10
  return 99
}

function onCodeChange(code: string) {
  form.instance_suffix = ''
  form.name = gatewayLabel(code)
  form.remark = defaultRemark(code)
  form.enable = false
  form.sort = defaultSort(code)
  resetCfg(code)
  // 多通道：名称给用户提示
  if (code === 'frog') form.name = '青蛙支付'
}

function showCreate() {
  editing.value = false
  editId.value = 0
  const prefer = ['frog', 'epay', 'epusdt', 'alipay', 'bepusdt', 'giftcard']
  const code = prefer.find((c) => availableCodes.value.includes(c))
    || availableCodes.value[0]
    || 'frog'
  form.code = code
  form.instance_suffix = ''
  form.name = code === 'frog' ? '青蛙支付' : gatewayLabel(code)
  form.enable = false
  form.sort = defaultSort(code)
  form.remark = defaultRemark(code)
  resetCfg(code)
  dialogVisible.value = true
}

function showEdit(m: any) {
  editing.value = true
  editId.value = m.id
  form.code = m.code
  form.instance_suffix = ''
  form.name = m.name
  form.enable = !!m.enable
  form.sort = m.sort ?? 0
  form.remark = m.remark || ''
  parseConfigIntoFields(gatewayKind(m.code), m.config || '{}')
  dialogVisible.value = true
}

async function handleSave() {
  const err = validateBeforeSave()
  if (err) {
    ElMessage.warning(err)
    return
  }
  const methodCode = resolveMethodCode()
  if (!methodCode) {
    ElMessage.warning('无法生成支付方式 code')
    return
  }
  let config: string
  try {
    config = buildConfigJSON()
  } catch (e: any) {
    ElMessage.warning(e?.message || '配置无效')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await request.put(`/admin/payment-methods/${editId.value}`, {
        name: form.name.trim(),
        enable: form.enable,
        sort: form.sort,
        config,
        remark: form.remark,
      })
    } else {
      await request.post('/admin/payment-methods', {
        code: methodCode,
        name: form.name.trim(),
        enable: form.enable,
        sort: form.sort,
        config,
        remark: form.remark,
      })
    }
    ElMessage.success(editing.value ? '已保存' : `已添加 ${methodCode}`)
    dialogVisible.value = false
    fetchAll()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
  saving.value = false
}

async function handleDelete(id: number) {
  try {
    await request.delete(`/admin/payment-methods/${id}`)
    ElMessage.success('已删除')
    fetchAll()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

// Keep name in sync when user picks gateway on create
watch(() => form.code, (c, prev) => {
  if (editing.value || !dialogVisible.value) return
  if (c && c !== prev && form.name === gatewayLabel(prev || '')) {
    form.name = gatewayLabel(c)
  }
})

onMounted(fetchAll)
</script>

<style scoped>
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.pm-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 18px;
  box-shadow: var(--k2-shadow-sm);
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.pm-card.off { opacity: 0.72; }
.pm-top {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}
.pm-name { font-size: 16px; font-weight: 800; color: #0f172a; }
.pm-code {
  margin-top: 4px;
  font-size: 12px;
  font-weight: 600;
  color: #6366f1;
}
.pm-remark { margin: 0; font-size: 13px; color: #64748b; line-height: 1.45; flex: 1; }
.pm-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1px solid #f1f5f9;
  padding-top: 10px;
}
.k2-action-btn.danger {
  color: #dc2626 !important;
  border-color: #fecaca !important;
  background: #fef2f2 !important;
}
.empty-hint {
  grid-column: 1 / -1;
  text-align: center;
  color: #94a3b8;
  padding: 48px 16px;
  font-size: 14px;
}

/* Dialog layout */
.dlg-scroll {
  max-height: min(62vh, 520px);
  overflow-y: auto;
  padding: 0 4px 4px 2px;
  margin: 0 -4px;
}
.dlg-section {
  margin-bottom: 16px;
}
.dlg-section:last-child {
  margin-bottom: 0;
}
.sec-title {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 13px;
  font-weight: 800;
  color: #0f172a;
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px solid #f1f5f9;
}
.sec-tip {
  font-size: 11px;
  font-weight: 500;
  color: #94a3b8;
}
.sec-tip a {
  color: #6366f1;
  text-decoration: none;
}
.sec-tip a:hover { text-decoration: underline; }

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 14px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}
.field.full { grid-column: 1 / -1; }
.field.sm { /* half width already */ }
.field label {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
}
.field .req { color: #ef4444; }
.field .opt {
  font-weight: 500;
  color: #94a3b8;
  font-size: 11px;
}
.row-enable {
  justify-content: flex-end;
}
.row-enable label {
  margin-bottom: 2px;
}

.mono-input :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.45;
}

.callout {
  margin-top: 10px;
  font-size: 12px;
  color: #64748b;
  line-height: 1.5;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 10px 12px;
}
.callout.warn {
  background: #fffbeb;
  border-color: #fde68a;
  color: #92400e;
}
.callout code {
  font-size: 11px;
  background: #e2e8f0;
  padding: 1px 5px;
  border-radius: 4px;
  word-break: break-all;
}
.callout.warn code {
  background: #fef3c7;
}

.dlg-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  flex-wrap: wrap;
}
.foot-hint {
  font-size: 12px;
  color: #94a3b8;
}
.foot-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

@media (max-width: 640px) {
  .form-grid { grid-template-columns: 1fr; }
  .dlg-scroll { max-height: 55vh; }
}
</style>

<style>
/* Dialog shell — not scoped so it applies to el-dialog teleport */
.pm-dialog.el-dialog {
  border-radius: 16px;
  overflow: hidden;
}
.pm-dialog .el-dialog__header {
  padding: 16px 20px 12px;
  margin-right: 0;
  border-bottom: 1px solid #f1f5f9;
}
.pm-dialog .el-dialog__title {
  font-size: 16px;
  font-weight: 800;
  color: #0f172a;
}
.pm-dialog .el-dialog__body {
  padding: 14px 20px 8px;
}
.pm-dialog .el-dialog__footer {
  padding: 12px 20px 16px;
  border-top: 1px solid #f1f5f9;
}
</style>
