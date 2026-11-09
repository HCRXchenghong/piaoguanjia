<template>
  <div class="page">
    <div class="panel">
      <div class="panel-head">
        <h3>团队成员</h3>
        <span class="sub">共 {{ users.length }} 个账号</span>
        <div class="spacer"></div>
        <a-button type="primary" size="small" @click="openCreate">
          <template #icon><icon-plus /></template>
          新建成员
        </a-button>
      </div>
      <a-table :data="users" :loading="loading" row-key="id" :pagination="false" :bordered="false">
        <template #columns>
          <a-table-column title="成员" :width="200">
            <template #cell="{ record }">
              <div class="person">
                <span class="avatar-chip">{{ initial(record.display_name) }}</span>
                <div>
                  <div style="font-weight: 550">{{ record.display_name }}</div>
                  <div class="tiny muted">{{ record.username }}</div>
                </div>
              </div>
            </template>
          </a-table-column>
          <a-table-column title="角色" :width="110">
            <template #cell="{ record }">
              <a-tag :color="record.role === 'admin' ? 'arcoblue' : 'gray'" size="small">
                {{ record.role === 'admin' ? '管理员' : '成员' }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column title="部门" data-index="dept" :width="120">
            <template #cell="{ record }">{{ record.dept || '—' }}</template>
          </a-table-column>
          <a-table-column title="票据数" data-index="invoice_count" align="right" :width="90" />
          <a-table-column title="累计金额" align="right" :width="120">
            <template #cell="{ record }">
              <span class="money-cell">¥{{ money(record.invoice_amount) }}</span>
            </template>
          </a-table-column>
          <a-table-column title="状态" :width="100">
            <template #cell="{ record }">
              <a-tag :color="record.active ? 'green' : 'gray'" size="small">
                {{ record.active ? '正常' : '已停用' }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column title="最后登录" :width="150">
            <template #cell="{ record }">
              <span class="tiny muted">{{ record.last_login_at ? dateTime(record.last_login_at) : '从未登录' }}</span>
            </template>
          </a-table-column>
          <a-table-column title="操作" align="right" :width="180">
            <template #cell="{ record }">
              <a-space>
                <a-tooltip content="编辑"><a-button type="text" size="mini" @click="openEdit(record)"><icon-edit /></a-button></a-tooltip>
                <a-tooltip content="重置密码"><a-button type="text" size="mini" @click="openReset(record)"><icon-lock /></a-button></a-tooltip>
                <a-tooltip :content="record.active ? '停用' : '启用'">
                  <a-button type="text" size="mini" @click="toggleActive(record)">
                    <icon-check v-if="!record.active" />
                    <icon-close v-else />
                  </a-button>
                </a-tooltip>
                <a-popconfirm content="删除后不可恢复，确定吗？" @ok="remove(record)">
                  <a-button type="text" size="mini" status="danger"><icon-delete /></a-button>
                </a-popconfirm>
              </a-space>
            </template>
          </a-table-column>
        </template>
      </a-table>
    </div>

    <a-modal v-model:visible="createVisible" title="新建成员" :on-before-ok="submitCreate" width="480">
      <a-form :model="createForm" layout="vertical">
        <a-row :gutter="14">
          <a-col :span="12">
            <a-form-item label="显示名称" required>
              <a-input v-model="createForm.display_name" placeholder="张三" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="部门">
              <a-input v-model="createForm.dept" placeholder="研发部（选填）" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="登录用户名" required>
          <a-input v-model="createForm.username" placeholder="字母 / 数字，3-32 位" />
        </a-form-item>
        <a-row :gutter="14">
          <a-col :span="12">
            <a-form-item label="初始密码" required>
              <a-input v-model="createForm.password" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="角色">
              <a-select v-model="createForm.role">
                <a-option value="member">成员</a-option>
                <a-option value="admin">管理员</a-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="editVisible" :title="`编辑「${editForm.display_name}」`" :on-before-ok="submitEdit" width="460">
      <a-form :model="editForm" layout="vertical">
        <a-row :gutter="14">
          <a-col :span="12">
            <a-form-item label="显示名称"><a-input v-model="editForm.display_name" /></a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="部门"><a-input v-model="editForm.dept" /></a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="角色">
          <a-select v-model="editForm.role">
            <a-option value="member">成员</a-option>
            <a-option value="admin">管理员</a-option>
          </a-select>
          <template #extra>管理员可以查看全部票据、管理成员与导出数据</template>
        </a-form-item>
      </a-form>
    </a-modal>
    <a-modal
      v-model:visible="resetVisible"
      :title="`重置「${resetTarget?.display_name || ''}」的密码`"
      :on-before-ok="submitReset"
      width="420"
    >
      <a-form layout="vertical">
        <a-form-item label="新密码" extra="重置后该成员所有登录状态会失效">
          <a-input v-model="resetValue" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api, type User } from '@/api'
import { dateTime, initial, money } from '@/utils/format'

const users = ref<User[]>([])
const loading = ref(false)
const createVisible = ref(false)
const editVisible = ref(false)
const resetVisible = ref(false)
const resetTarget = ref<User | null>(null)
const resetValue = ref('')

const createForm = reactive({ display_name: '', username: '', password: randomPwd(), role: 'member', dept: '' })
const editForm = reactive<any>({ id: 0, display_name: '', dept: '', role: 'member' })

function randomPwd() {
  const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
  return Array.from({ length: 8 }, () => chars[Math.floor(Math.random() * chars.length)]).join('')
}

async function load() {
  loading.value = true
  try {
    const data = await api.users()
    users.value = data.users
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(createForm, {
    display_name: '', username: '', password: randomPwd(), role: 'member', dept: '',
  })
  createVisible.value = true
}

async function submitCreate() {
  if (!createForm.display_name || !createForm.username) {
    Message.warning('请填写显示名称与用户名')
    return false
  }
  try {
    await api.createUser({ ...createForm })
    Message.success('成员已创建')
    createVisible.value = false
    load()
    return true
  } catch (e: any) {
    Message.error(e.message)
    return false
  }
}

function openEdit(record: User) {
  Object.assign(editForm, {
    id: record.id, display_name: record.display_name, dept: record.dept || '', role: record.role,
  })
  editVisible.value = true
}

async function submitEdit() {
  try {
    await api.updateUser(editForm.id, {
      display_name: editForm.display_name, dept: editForm.dept, role: editForm.role,
    })
    Message.success('已保存')
    editVisible.value = false
    load()
    return true
  } catch (e: any) {
    Message.error(e.message)
    return false
  }
}

function openReset(record: User) {
  resetTarget.value = record
  resetValue.value = randomPwd()
  resetVisible.value = true
}

async function submitReset() {
  if (!resetTarget.value) return false
  if (resetValue.value.length < 6) {
    Message.warning('密码至少 6 位')
    return false
  }
  try {
    await api.resetPassword(resetTarget.value.id, resetValue.value)
    Message.success(`「${resetTarget.value.display_name}」的新密码：${resetValue.value}`)
    resetVisible.value = false
    return true
  } catch (e: any) {
    Message.error(e.message)
    return false
  }
}

async function toggleActive(record: User) {
  try {
    await api.updateUser(record.id, { active: !record.active })
    Message.success(record.active ? '已停用' : '已启用')
    load()
  } catch (e: any) {
    Message.error(e.message)
  }
}

async function remove(record: User) {
  try {
    await api.deleteUser(record.id)
    Message.success('已删除')
    load()
  } catch (e: any) {
    Message.error(e.message)
  }
}

onMounted(load)
</script>
