<template>
  <el-dialog
    :model-value="visible"
    :title="isEdit ? '编辑用户' : '创建用户'"
    width="580px"
    class="user-edit-dialog"
    align-center
    @update:model-value="$emit('update:visible', $event)"
    @close="resetForm"
    destroy-on-close
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" label-position="right">
      <el-form-item label="邮箱" prop="email">
        <el-input v-model="form.email" :disabled="isEdit" placeholder="user@example.com" />
      </el-form-item>
      <el-form-item :label="isEdit ? '密码(留空不变)' : '密码'" :prop="isEdit ? '' : 'password'">
        <el-input v-model="form.password" type="password" show-password :placeholder="isEdit ? '留空则不修改' : '至少6位'" />
      </el-form-item>

      <el-divider content-position="left">套餐分配</el-divider>

      <el-form-item label="订阅计划">
        <el-select
          v-model="form.plan_id"
          style="width:100%"
          :clearable="isEdit"
          :placeholder="isEdit ? '选择或更换订阅计划（可清空）' : '选择订阅计划'"
          @change="onPlanChange"
        >
          <el-option v-for="p in plans" :key="p.id" :label="planLabel(p)" :value="p.id" />
        </el-select>
        <div class="field-hint">
          {{
            isEdit
              ? '给已注册用户开通/更换套餐：将同步权限组、流量/速度/设备限制，并按套餐周期重算到期（可再改下方数值）'
              : '选择计划后自动分配：权限组 + 节点 + 流量/速度/设备限制 + 到期时间'
          }}
        </div>
      </el-form-item>

      <el-form-item label="权限组">
        <el-select
          v-model="form.group_id"
          style="width:100%"
          clearable
          placeholder="由订阅计划自动带出，也可单独调整"
        >
          <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
        <div class="field-hint muted">
          节点可见性由权限组决定；一般随订阅计划自动填充，无需手改
        </div>
      </el-form-item>

      <el-divider content-position="left">流量与限制</el-divider>
      <el-form-item label="流量限制">
        <div style="display:flex;gap:8px;width:100%">
          <el-input v-model.number="form.traffic_limit" type="number" placeholder="0" />
          <span style="line-height:32px;color:#999;font-size:12px;white-space:nowrap">GB</span>
        </div>
        <div class="form-hint">
          未绑定套餐/权限组时保持 0 表示「未开通」，不要理解成无限流量；
          已开通用户填 0 才表示不限流量。
        </div>
      </el-form-item>
      <el-form-item label="速度限制">
        <div style="display:flex;gap:8px;width:100%">
          <el-input v-model.number="form.speed_limit" type="number" placeholder="0" />
          <span style="line-height:32px;color:#999;font-size:12px;white-space:nowrap">Mbps</span>
        </div>
      </el-form-item>
      <el-form-item label="设备限制">
        <el-input v-model.number="form.device_limit" type="number" placeholder="0" />
      </el-form-item>
      <el-form-item label="到期时间">
        <el-date-picker
          v-model="expireDate" type="date" placeholder="留空 = 未设到期（未开通勿当永久）"
          style="width:100%" value-format="x"
        />
        <div class="form-hint">
          无套餐且无权限组时留空表示未开通；仅已开通用户留空才表示永久有效。
        </div>
      </el-form-item>
      <el-form-item v-if="isEdit" label="账号状态">
        <el-switch v-model="form.enable" active-text="正常" inactive-text="封禁" />
        <div class="form-hint">封禁仅管理员风控使用；套餐是否可用看「到期时间」，到期用户仍可登录续费。</div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ isEdit ? '保存修改' : '立即创建' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { createUser, updateUser, type User, type CreateUserParams, type UpdateUserParams } from '@/api/user'
import request from '@/api/request'
import { formatBytes, bytesToGB, gbToBytes, safeNum } from '@/utils/format'

interface Group { id: number; name: string; traffic_limit: number; speed_limit: number; device_limit: number }
interface Plan { id: number; name: string; group_id: number; duration: number; traffic_limit: number; speed_limit: number; device_limit: number }

const props = defineProps<{ visible: boolean; user: User | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; saved: [] }>()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const expireDate = ref<number | null>(null)
const isEdit = computed(() => !!props.user)
const groups = ref<Group[]>([])
const plans = ref<Plan[]>([])

const form = reactive({
  email: '', password: '', group_id: 0 as number, plan_id: 0 as number,
  traffic_limit: 0, speed_limit: 0, device_limit: 0, enable: true,
})

const rules = computed<FormRules>(() => ({
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email', message: '邮箱格式不正确', trigger: 'blur' },
  ],
  password: isEdit.value
    ? []
    : [
        { required: true, message: '请输入密码', trigger: 'blur' },
        { min: 6, message: '密码至少 6 位', trigger: 'blur' },
      ],
}))

async function fetchOptions() {
  try {
    const [gr, pr] = await Promise.all([request.get('/admin/groups'), request.get('/admin/plans')])
    groups.value = gr.data || []; plans.value = pr.data || []
  } catch {}
}

function planLabel(p: Plan) {
  const d = p.duration / 86400
  const dur = d === 30 ? '月' : d === 90 ? '季' : d === 180 ? '半年' : d === 365 ? '年' : `${d}天`
  return `${p.name} (${dur}${p.traffic_limit > 0 ? ' ' + formatBytes(p.traffic_limit) : ''})`
}

function onPlanChange() {
  // 清空计划时不改限额/组（管理员可能只想解绑 plan_id）
  if (!form.plan_id) return
  const p = plans.value.find(x => x.id === form.plan_id)
  if (!p) return
  // 与创建一致：套餐模板写入表单，保存后生效
  form.traffic_limit = p.traffic_limit > 0 ? bytesToGB(p.traffic_limit) : 0
  form.speed_limit = p.speed_limit > 0 ? p.speed_limit : 0
  form.device_limit = p.device_limit > 0 ? p.device_limit : 0
  if (p.duration > 0) {
    expireDate.value = Date.now() + p.duration * 1000
  } else {
    expireDate.value = null
  }
  form.group_id = p.group_id > 0 ? p.group_id : 0
}

watch(() => props.visible, (val) => {
  if (val && props.user) {
    form.email = props.user.email; form.password = ''; form.group_id = props.user.group_id || 0
    form.plan_id = props.user.plan_id || 0
    form.traffic_limit = bytesToGB(props.user.traffic_limit); form.speed_limit = props.user.speed_limit
    form.device_limit = props.user.device_limit; form.enable = props.user.enable
    expireDate.value = props.user.expire_at > 0 ? props.user.expire_at * 1000 : null
  } else if (val) { resetForm() }
})

function resetForm() {
  form.email = ''; form.password = ''; form.group_id = 0; form.plan_id = 0
  form.traffic_limit = 0; form.speed_limit = 0; form.device_limit = 0; form.enable = true
  expireDate.value = null; formRef.value?.resetFields()
}

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (!isEdit.value && (!form.password || form.password.length < 6)) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  submitting.value = true
  try {
    const expireRaw = expireDate.value
    const expireAt = expireRaw
      ? Math.floor(Number(expireRaw) / 1000)
      : 0
    const trafficLimit = gbToBytes(safeNum(form.traffic_limit))
    const speedLimit = safeNum(form.speed_limit)
    const deviceLimit = Math.max(0, Math.floor(safeNum(form.device_limit)))
    const groupId = safeNum(form.group_id)
    const planId = safeNum(form.plan_id)

    if (isEdit.value) {
      const pwd = (form.password || '').trim()
      if (pwd && pwd.length < 6) {
        ElMessage.warning('新密码至少 6 位')
        submitting.value = false
        return
      }
      const p: UpdateUserParams = {
        traffic_limit: trafficLimit,
        speed_limit: speedLimit,
        device_limit: deviceLimit,
        enable: form.enable,
        expire_at: Number.isFinite(expireAt) ? expireAt : 0,
        group_id: groupId,
        plan_id: planId,
      }
      if (pwd) p.password = pwd
      const res: any = await updateUser(props.user!.id, p)
      const data = res?.data
      if (data?.password_updated) {
        ElMessage.success(data.message || '密码已重置，请用新密码在用户端登录')
      } else if (data?.message) {
        ElMessage.success(data.message)
      } else if (pwd) {
        ElMessage.success('密码已重置')
      } else {
        ElMessage.success('用户信息已更新（密码未改：密码框留空）')
      }
    } else {
      const p: CreateUserParams = {
        email: form.email.trim().toLowerCase(),
        password: form.password.trim(),
        group_id: groupId,
        plan_id: planId,
        traffic_limit: trafficLimit,
        speed_limit: speedLimit,
        device_limit: deviceLimit,
        expire_at: Number.isFinite(expireAt) ? expireAt : 0,
      }
      await createUser(p)
      ElMessage.success('创建成功')
    }
    emit('update:visible', false)
    emit('saved')
  } catch {
    // error toast already shown by request interceptor
  }
  submitting.value = false
}

onMounted(fetchOptions)
</script>

<style scoped>
:deep(.el-divider__text) {
  font-size: 12px;
  font-weight: 700;
  color: #64748b;
  letter-spacing: 0.04em;
}
:deep(.el-form-item__label) {
  font-weight: 600;
  color: #475569;
}
.field-hint {
  color: #1677ff;
  font-size: 11px;
  margin-top: 4px;
  line-height: 1.45;
}
.field-hint.muted {
  color: #94a3b8;
}
.form-hint {
  color: #94a3b8;
  font-size: 11px;
  margin-top: 6px;
  line-height: 1.45;
}
</style>
