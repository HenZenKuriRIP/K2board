<template>
  <div class="settings-page">
    <div class="k2-page-header">
      <div>
        <h3>系统设置</h3>
        <p class="sub">站点、通讯钥匙、管理员与邮件服务配置</p>
      </div>
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        <el-icon><Select /></el-icon>
        保存全部设置
      </el-button>
    </div>

    <div class="settings-grid" v-loading="loading">
      <!-- 站点信息 -->
      <section class="setting-card">
        <div class="card-head">
          <div class="icon i1"><el-icon :size="18"><HomeFilled /></el-icon></div>
          <div>
            <h4>站点信息</h4>
            <p>对外展示与订阅域名</p>
          </div>
        </div>
        <el-form :model="form" label-position="top" class="s-form">
          <el-form-item label="站点名称">
            <el-input v-model="form.site_name" size="large" placeholder="K2Board" />
          </el-form-item>
          <el-form-item label="站点 URL">
            <el-input v-model="form.site_url" size="large" placeholder="https://www.example.com" />
            <div class="hint">
              面板公网地址（www 主域）。用于支付回调 <code>notify</code>、默认支付回跳。
              <b>请钉死，勿轻易修改。</b>
            </div>
          </el-form-item>
          <el-form-item label="订阅域名">
            <el-input v-model="form.subscribe_url" size="large" placeholder="https://www.example.com" />
            <div class="hint">
              生成订阅链接的 origin。客户端已写入的链接依赖此值——
              <b>必须与现网订阅地址一致并长期钉死</b>；留空则回退站点 URL。
            </div>
          </el-form-item>
          <el-form-item label="允许的用户端域名">
            <el-input
              v-model="form.allowed_origins"
              type="textarea"
              :rows="5"
              size="large"
              placeholder="https://user.example.com&#10;https://shadow-a.com&#10;https://shadow-b.net"
            />
            <div class="hint">
              影子注册入口 / 独立用户前端的 Origin 白名单（每行一个
              <code>https://域名</code>）。用于：
              <b>CORS</b>（跨域调 API）与 <b>支付 return_url</b> 主机校验。
              站点 URL / 订阅域名会自动加入，无需重复填写。禁止 <code>*</code>。
              修改后约 30 秒内全站生效（保存时立即刷新缓存）。
            </div>
            <div v-if="effectiveOrigins" class="hint effective-origins">
              <b>当前生效列表：</b>
              <pre>{{ effectiveOrigins }}</pre>
            </div>
          </el-form-item>
        </el-form>
      </section>

      <!-- 通讯钥匙 -->
      <section class="setting-card">
        <div class="card-head">
          <div class="icon i2"><el-icon :size="18"><Lock /></el-icon></div>
          <div>
            <h4>通讯钥匙</h4>
            <p>XrayR4u / UniProxy 统一 Token</p>
          </div>
        </div>
        <el-form label-position="top" class="s-form">
          <el-form-item label="Panel Token">
            <el-input :model-value="displayToken" disabled size="large">
              <template #append>
                <el-button @click="handleGenerateToken">生成新钥匙</el-button>
              </template>
            </el-input>
            <div class="hint">
              SHA256 哈希存储。生成后<b>仅显示一次明文</b>，请立即复制到节点侧 <code>ApiKey</code>
            </div>
          </el-form-item>
          <div v-if="newToken" class="token-alert">
            <div class="token-title">新钥匙（仅显示一次）</div>
            <code>{{ newToken }}</code>
            <div class="token-actions">
              <el-button type="primary" size="small" @click="copyNewToken">复制并保存</el-button>
              <span class="warn">保存后不可再次查看</span>
            </div>
          </div>
        </el-form>
      </section>

      <!-- 管理员 -->
      <section class="setting-card">
        <div class="card-head">
          <div class="icon i3"><el-icon :size="18"><UserFilled /></el-icon></div>
          <div>
            <h4>管理员账号</h4>
            <p>控制台登录凭证</p>
          </div>
        </div>
        <el-form label-position="top" class="s-form">
          <el-form-item label="管理员邮箱">
            <el-input v-model="adminEmail" disabled size="large" />
            <div class="hint">管理员账号不可删除</div>
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="adminNewPass" type="password" show-password size="large" placeholder="留空不修改" />
          </el-form-item>
          <el-button type="primary" :loading="savingPass" :disabled="!adminId" @click="handleChangeAdminPass">
            修改密码
          </el-button>
        </el-form>
      </section>

      <!-- 注册与邮件 -->
      <section class="setting-card">
        <div class="card-head">
          <div class="icon i4"><el-icon :size="18"><Message /></el-icon></div>
          <div>
            <h4>注册与邮件</h4>
            <p>开放注册与 SMTP 验证码（用户注册依赖此配置）</p>
          </div>
        </div>
        <el-form :model="form" label-position="top" class="s-form">
          <el-form-item label="开放注册">
            <el-switch v-model="allowRegister" active-text="开启" inactive-text="关闭" />
            <div class="hint">
              开启后用户端可自助注册。必须同时配置可用 SMTP，否则无法发送验证码。
            </div>
          </el-form-item>
          <el-divider />
          <el-form-item label="SMTP 服务器">
            <el-input v-model="form.smtp_host" size="large" placeholder="smtp.example.com / smtp.qq.com" />
          </el-form-item>
          <div class="row-2">
            <el-form-item label="SMTP 端口">
              <el-select v-model="smtpPort" size="large" style="width:100%" allow-create filterable>
                <el-option :value="587" label="587 (STARTTLS，推荐)" />
                <el-option :value="465" label="465 (SSL/TLS)" />
                <el-option :value="25" label="25 (明文/可选 STARTTLS)" />
              </el-select>
            </el-form-item>
            <el-form-item label="发件邮箱 / 用户名">
              <el-input v-model="form.smtp_user" size="large" placeholder="noreply@example.com" />
            </el-form-item>
          </div>
          <el-form-item label="邮箱密码 / 授权码">
            <el-input
              v-model="form.smtp_pass"
              type="password"
              show-password
              size="large"
              :placeholder="smtpPassPlaceholder"
            />
            <div class="hint">QQ/163 等请使用授权码，不是登录密码。留空保存表示不修改已存密码。</div>
          </el-form-item>
          <el-form-item label="发件人 From（可选）">
            <el-input v-model="form.smtp_from" size="large" placeholder="默认与发件邮箱相同" />
          </el-form-item>
          <el-form-item label="邮件测试">
            <div class="test-row">
              <el-input v-model="testEmail" size="large" placeholder="test@example.com" />
              <el-button type="primary" size="large" :loading="testingEmail" @click="handleTestEmail">
                发送测试
              </el-button>
            </div>
            <div class="hint">可直接用当前表单内容测试（无需先点保存）。常用：587 + 授权码。</div>
          </el-form-item>
        </el-form>
      </section>

      <!-- 推广返佣 -->
      <section id="referral" class="setting-card full-span">
        <div class="card-head">
          <div class="icon i6"><el-icon :size="18"><Share /></el-icon></div>
          <div>
            <h4>推广返佣</h4>
            <p>邀请返佣比例、最低提现与收款方式列表</p>
          </div>
        </div>
        <el-form :model="form" label-position="top" class="s-form">
          <el-form-item label="开启推广">
            <el-switch v-model="referralEnable" active-text="开启" inactive-text="关闭" />
            <div class="hint">关闭后不再计佣，用户也无法申请提现；历史余额保留。</div>
          </el-form-item>
          <div class="row-2">
            <el-form-item label="返佣比例（%）">
              <el-input-number
                v-model="referralRate"
                :min="0"
                :max="100"
                :step="1"
                controls-position="right"
                style="width: 100%"
              />
              <div class="hint">按订单实付金额比例；默认 10。每笔已支付订单均计佣。</div>
            </el-form-item>
            <el-form-item label="最低提现（元）">
              <el-input-number
                v-model="referralMinYuan"
                :min="0"
                :precision="2"
                :step="10"
                controls-position="right"
                style="width: 100%"
              />
              <div class="hint">默认 100 元。用户余额低于此金额不可申请提现。</div>
            </el-form-item>
          </div>
          <el-form-item label="收款方式列表（JSON）">
            <el-input
              v-model="form.referral_payout_methods"
              type="textarea"
              :rows="4"
              placeholder='[{"code":"alipay","name":"支付宝"},{"code":"wechat","name":"微信"}]'
            />
            <div class="hint">
              用户提现时可选的收款渠道。每项需含 <code>code</code> 与 <code>name</code>。
              也可在「推广管理」页审核提现。
            </div>
          </el-form-item>
        </el-form>
      </section>

      <!-- 邮件模板管理 -->
      <section class="setting-card full-span">
        <div class="card-head">
          <div class="icon i5"><el-icon :size="18"><Document /></el-icon></div>
          <div>
            <h4>邮件模板管理</h4>
            <p>注册验证码与密码重置邮件内容可自定义</p>
          </div>
        </div>
        <div class="hint tpl-hint">
          可用占位符：<code v-pre>{{code}}</code> 验证码、
          <code v-pre>{{site_name}}</code> 站点名称（取自上方「站点名称」）
        </div>
        <div class="tpl-grid">
          <div class="tpl-block">
            <h5>注册验证码邮件</h5>
            <el-form label-position="top" class="s-form">
              <el-form-item label="邮件主题">
                <el-input
                  v-model="form.mail_tpl_register_subject"
                  size="large"
                  placeholder="【{{site_name}}】邮箱验证码"
                />
              </el-form-item>
              <el-form-item label="邮件正文">
                <el-input
                  v-model="form.mail_tpl_register_body"
                  type="textarea"
                  :rows="8"
                  placeholder="您的 {{site_name}} 验证码是: {{code}}"
                />
              </el-form-item>
              <el-button size="small" text type="primary" @click="resetTpl('register')">恢复默认</el-button>
            </el-form>
          </div>
          <div class="tpl-block">
            <h5>密码重置邮件</h5>
            <el-form label-position="top" class="s-form">
              <el-form-item label="邮件主题">
                <el-input
                  v-model="form.mail_tpl_reset_subject"
                  size="large"
                  placeholder="【{{site_name}}】重置密码验证码"
                />
              </el-form-item>
              <el-form-item label="邮件正文">
                <el-input
                  v-model="form.mail_tpl_reset_body"
                  type="textarea"
                  :rows="8"
                  placeholder="您正在重置 {{site_name}} 账号密码。验证码：{{code}}"
                />
              </el-form-item>
              <el-button size="small" text type="primary" @click="resetTpl('reset')">恢复默认</el-button>
            </el-form>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Select, HomeFilled, Lock, Message, UserFilled, Document, Share } from '@element-plus/icons-vue'
import request from '@/api/request'

const SMTP_MASK = '********'

const DEFAULT_TPL = {
  register_subject: '【{{site_name}}】邮箱验证码',
  register_body:
    '您的 {{site_name}} 验证码是: {{code}}\n\n' +
    '验证码 10 分钟内有效，请勿泄露给他人。\n\n' +
    '如果您没有注册 {{site_name}} 账号，请忽略此邮件。',
  reset_subject: '【{{site_name}}】重置密码验证码',
  reset_body:
    '您正在重置 {{site_name}} 账号密码。\n\n' +
    '验证码：{{code}}\n\n' +
    '验证码 10 分钟内有效，请勿泄露给他人。\n\n' +
    '如非本人操作，请忽略本邮件，并建议尽快登录检查账号安全。',
}

const form = reactive<Record<string, string>>({
  site_name: '', site_url: '', subscribe_url: '', allowed_origins: '',
  smtp_host: '', smtp_port: '587', smtp_user: '', smtp_pass: '', smtp_from: '',
  allow_register: 'false', panel_token: '',
  mail_tpl_register_subject: '', mail_tpl_register_body: '',
  mail_tpl_reset_subject: '', mail_tpl_reset_body: '',
  referral_enable: 'true',
  referral_rate: '10',
  referral_min_withdraw: '100',
  referral_payout_methods: '',
})
const effectiveOrigins = ref('')
const allowRegister = ref(false)
const referralEnable = ref(true)
const referralRate = ref(10)
const referralMinYuan = ref(100)
const smtpPort = ref<number | string>(587)
const loading = ref(false)
const saving = ref(false)
const newToken = ref('')
const displayToken = ref('')
const adminEmail = ref('')
const adminId = ref(0)
const adminNewPass = ref('')
const savingPass = ref(false)
const testEmail = ref('')
const testingEmail = ref(false)
const smtpPassSet = ref(false)

const smtpPassPlaceholder = computed(() =>
  smtpPassSet.value ? '已配置（留空不修改）' : 'SMTP 授权码',
)

async function fetchSettings() {
  loading.value = true
  try {
    const res = await request.get('/admin/settings')
    const data = res.data || {}
    Object.assign(form, data)
    if (form.allowed_origins == null) form.allowed_origins = ''
    effectiveOrigins.value = data.allowed_origins_effective || ''
    allowRegister.value = form.allow_register === 'true'
    smtpPort.value = parseInt(form.smtp_port || '587') || 587
    displayToken.value = form.panel_token || '未设置'
    adminEmail.value = data.admin_email || ''
    adminId.value = parseInt(data.admin_id || '0') || 0
    smtpPassSet.value = data.smtp_pass === SMTP_MASK
    // Do not keep mask in editable field so empty save keeps old password
    if (form.smtp_pass === SMTP_MASK) form.smtp_pass = ''
    if (!form.smtp_from) form.smtp_from = ''
    // Fill empty mail templates with defaults for editing
    if (!form.mail_tpl_register_subject) form.mail_tpl_register_subject = DEFAULT_TPL.register_subject
    if (!form.mail_tpl_register_body) form.mail_tpl_register_body = DEFAULT_TPL.register_body
    if (!form.mail_tpl_reset_subject) form.mail_tpl_reset_subject = DEFAULT_TPL.reset_subject
    if (!form.mail_tpl_reset_body) form.mail_tpl_reset_body = DEFAULT_TPL.reset_body
    // Referral (API returns min withdraw already in yuan)
    referralEnable.value = (form.referral_enable || 'true') === 'true'
    referralRate.value = parseInt(form.referral_rate || '10', 10) || 0
    referralMinYuan.value = parseFloat(form.referral_min_withdraw || '100') || 0
    if (!form.referral_payout_methods) {
      form.referral_payout_methods = JSON.stringify([
        { code: 'alipay', name: '支付宝' },
        { code: 'wechat', name: '微信' },
        { code: 'usdt_trc20', name: 'USDT-TRC20' },
        { code: 'bank', name: '银行卡' },
      ])
    }
  } catch { /* ignore */ }
  loading.value = false
}

function resetTpl(kind: 'register' | 'reset') {
  if (kind === 'register') {
    form.mail_tpl_register_subject = DEFAULT_TPL.register_subject
    form.mail_tpl_register_body = DEFAULT_TPL.register_body
  } else {
    form.mail_tpl_reset_subject = DEFAULT_TPL.reset_subject
    form.mail_tpl_reset_body = DEFAULT_TPL.reset_body
  }
  ElMessage.success('已恢复默认模板（需点击保存生效）')
}

function buildPayload(): Record<string, string> {
  const payload: Record<string, string> = {
    site_name: form.site_name || '',
    site_url: form.site_url || '',
    subscribe_url: form.subscribe_url || '',
    allowed_origins: form.allowed_origins || '',
    allow_register: allowRegister.value ? 'true' : 'false',
    smtp_host: form.smtp_host || '',
    smtp_port: String(smtpPort.value || 587),
    smtp_user: form.smtp_user || '',
    smtp_from: form.smtp_from || '',
    mail_tpl_register_subject: form.mail_tpl_register_subject || '',
    mail_tpl_register_body: form.mail_tpl_register_body || '',
    mail_tpl_reset_subject: form.mail_tpl_reset_subject || '',
    mail_tpl_reset_body: form.mail_tpl_reset_body || '',
    referral_enable: referralEnable.value ? 'true' : 'false',
    referral_rate: String(Math.max(0, Math.min(100, Math.round(referralRate.value || 0)))),
    // yuan — backend converts to cents
    referral_min_withdraw: String(referralMinYuan.value ?? 0),
    referral_payout_methods: form.referral_payout_methods || '',
  }
  // Only send password when user typed a new one
  if (form.smtp_pass && form.smtp_pass !== SMTP_MASK) {
    payload.smtp_pass = form.smtp_pass
  }
  // panel_token only when generating new
  if (newToken.value) {
    payload.panel_token = newToken.value
  }
  return payload
}

async function handleSave() {
  saving.value = true
  try {
    await request.put('/admin/settings', buildPayload())
    ElMessage.success('已保存')
    newToken.value = ''
    await fetchSettings()
  } catch { /* interceptor toast */ }
  saving.value = false
}

function handleGenerateToken() {
  const chars = '0123456789abcdef'
  let t = ''
  for (let i = 0; i < 32; i++) t += chars[Math.floor(Math.random() * 16)]
  newToken.value = t
}

async function copyNewToken() {
  await navigator.clipboard.writeText(newToken.value)
  await handleSave()
  ElMessage.success('钥匙已保存并复制到剪贴板')
}

async function handleChangeAdminPass() {
  if (!adminId.value) {
    ElMessage.warning('未找到管理员账号')
    return
  }
  if (!adminNewPass.value || adminNewPass.value.length < 6) {
    ElMessage.warning('密码至少6位')
    return
  }
  savingPass.value = true
  try {
    await request.put(`/admin/users/${adminId.value}`, { password: adminNewPass.value })
    ElMessage.success('密码已修改')
    adminNewPass.value = ''
  } catch {
    ElMessage.error('修改失败')
  }
  savingPass.value = false
}

async function handleTestEmail() {
  if (!testEmail.value) {
    ElMessage.warning('请输入测试邮箱')
    return
  }
  if (!form.smtp_host) {
    ElMessage.warning('请先填写 SMTP 服务器')
    return
  }
  testingEmail.value = true
  try {
    const body: Record<string, string> = {
      to: testEmail.value,
      smtp_host: form.smtp_host,
      smtp_port: String(smtpPort.value || 587),
      smtp_user: form.smtp_user || '',
      smtp_from: form.smtp_from || '',
    }
    if (form.smtp_pass && form.smtp_pass !== SMTP_MASK) {
      body.smtp_pass = form.smtp_pass
    }
    await request.post('/admin/settings/test-email', body)
    ElMessage.success('测试邮件已发送')
  } catch {
    // interceptor shows backend detail
  }
  testingEmail.value = false
}

onMounted(async () => {
  await fetchSettings()
  if (location.hash === '#referral') {
    document.getElementById('referral')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
})
</script>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.setting-card {
  background: #fff;
  border: 1px solid var(--k2-border);
  border-radius: 18px;
  padding: 22px;
  box-shadow: var(--k2-shadow-sm);
}
.card-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid #f1f5f9;
}
.icon {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  color: #fff;
  flex-shrink: 0;
}
.icon.i1 { background: linear-gradient(135deg, #6366f1, #4f46e5); }
.icon.i2 { background: linear-gradient(135deg, #f59e0b, #d97706); }
.icon.i3 { background: linear-gradient(135deg, #34d399, #059669); }
.icon.i4 { background: linear-gradient(135deg, #22d3ee, #0891b2); }
.icon.i5 { background: linear-gradient(135deg, #a78bfa, #7c3aed); }
.icon.i6 { background: linear-gradient(135deg, #f472b6, #db2777); }
.full-span { grid-column: 1 / -1; }
.tpl-hint {
  margin: -6px 0 16px;
}
.tpl-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.tpl-block {
  padding: 16px;
  border-radius: 14px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}
.tpl-block h5 {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 800;
  color: #0f172a;
}
.card-head h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
  color: #0f172a;
}
.card-head p {
  margin: 2px 0 0;
  font-size: 12px;
  color: #94a3b8;
}
.s-form :deep(.el-form-item__label) {
  font-weight: 600;
  color: #475569;
}
.hint {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.5;
}
.hint code {
  background: #f1f5f9;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  color: #475569;
}
.hint b { color: #dc2626; }
.hint.effective-origins {
  margin-top: 10px;
  padding: 10px 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}
.hint.effective-origins pre {
  margin: 6px 0 0;
  font-size: 11px;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.token-alert {
  margin-top: 8px;
  padding: 14px 16px;
  border-radius: 14px;
  background: linear-gradient(135deg, #ecfdf5, #f0fdf4);
  border: 1px solid #a7f3d0;
}
.token-title {
  font-weight: 800;
  color: #059669;
  margin-bottom: 8px;
  font-size: 13px;
}
.token-alert code {
  display: block;
  font-size: 13px;
  word-break: break-all;
  user-select: all;
  color: #0f172a;
  background: rgba(255, 255, 255, 0.7);
  padding: 10px 12px;
  border-radius: 10px;
}
.token-actions {
  margin-top: 10px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.warn {
  color: #dc2626;
  font-size: 12px;
  font-weight: 600;
}
.row-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 12px;
}
.test-row {
  display: flex;
  gap: 10px;
  width: 100%;
}
@media (max-width: 960px) {
  .settings-grid { grid-template-columns: 1fr; }
  .row-2 { grid-template-columns: 1fr; }
  .test-row { flex-direction: column; }
  .tpl-grid { grid-template-columns: 1fr; }
}
</style>
