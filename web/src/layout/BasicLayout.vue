<template>
  <a-layout class="app-layout">
    <a-layout-sider
      class="app-sider"
      :width="232"
      :collapsed-width="64"
      :collapsed="collapsed"
      collapsible
      breakpoint="xl"
      :show-collapse-button="false"
      @collapse="(v: boolean) => (collapsed = v)"
    >
      <div class="brand">
        <div class="brand-logo"><icon-file /></div>
        <div v-show="!collapsed">
          <div class="brand-name">{{ userStore.orgName }}</div>
          <div class="brand-sub">INVOICE HUB</div>
        </div>
      </div>

      <a-menu
        class="app-menu"
        theme="dark"
        :collapsed="collapsed"
        :selected-keys="selectedKeys"
        @menu-item-click="onMenuClick"
      >
        <a-menu-item v-for="item in menus" :key="item.path">
          <template #icon><component :is="item.icon" /></template>
          {{ item.label }}
        </a-menu-item>
      </a-menu>

      <div class="sider-foot">
        <a-dropdown trigger="click" position="tr">
          <div class="user-chip" :style="{ justifyContent: collapsed ? 'center' : 'flex-start' }">
            <a-avatar :size="32" :style="{ background: 'linear-gradient(135deg,#6366f1,#a855f7)' }">
              {{ initial(userStore.displayName) }}
            </a-avatar>
            <div v-show="!collapsed" style="min-width: 0; flex: 1">
              <div class="user-name">{{ userStore.displayName }}</div>
              <div class="user-role">{{ userStore.isAdmin ? '管理员' : '成员' }}</div>
            </div>
            <icon-settings v-show="!collapsed" style="color: #5f6784" />
          </div>
          <template #content>
            <a-doption @click="openPassword">修改密码</a-doption>
            <a-doption @click="openHelp">使用帮助</a-doption>
            <a-doption disabled>版本 {{ buildTime }}</a-doption>
            <a-doption @click="logout">退出登录</a-doption>
          </template>
        </a-dropdown>
      </div>
    </a-layout-sider>

    <a-layout>
      <div class="app-header">
        <a-button v-if="isNarrow" shape="circle" type="text" @click="collapsed = !collapsed">
          <icon-menu-unfold />
        </a-button>
        <div>
          <div class="header-title">{{ pageTitle }}</div>
          <div class="header-sub">{{ pageSub }}</div>
        </div>
        <div class="header-actions">
          <a-button type="primary" @click="router.push('/upload')">
            <template #icon><icon-plus /></template>
            上传发票
          </a-button>
        </div>
      </div>

      <a-layout-content class="app-content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>

  <a-modal v-model:visible="pwdVisible" title="修改密码" :on-before-ok="submitPassword" @cancel="resetPwd">
    <a-form :model="pwd" layout="vertical">
      <a-form-item label="原密码"><a-input-password v-model="pwd.old" placeholder="请输入原密码" /></a-form-item>
      <a-form-item label="新密码"><a-input-password v-model="pwd.pwd" placeholder="至少 6 位" /></a-form-item>
      <a-form-item label="确认新密码"><a-input-password v-model="pwd.pwd2" placeholder="再次输入" /></a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message, Modal } from '@arco-design/web-vue'
import { useUserStore } from '@/stores/user'
import { api } from '@/api'
import { initial } from '@/utils/format'

const buildTime = __BUILD_TIME__
const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const isNarrow = ref(window.innerWidth < 1200)
const pwdVisible = ref(false)
const pwd = reactive({ old: '', pwd: '', pwd2: '' })

const menus = computed(() => {
  if (userStore.isAdmin) {
    return [
      { path: '/', label: '概览', icon: 'IconDashboard' },
      { path: '/invoices', label: '全部票据', icon: 'IconFile' },
      { path: '/upload', label: '上传发票', icon: 'IconUpload' },
      { path: '/members', label: '成员管理', icon: 'IconUserGroup' },
      { path: '/export', label: '导出中心', icon: 'IconDownload' },
      { path: '/settings', label: '系统设置', icon: 'IconSettings' },
    ]
  }
  return [
    { path: '/', label: '工作台', icon: 'IconDashboard' },
    { path: '/upload', label: '上传发票', icon: 'IconUpload' },
    { path: '/invoices', label: '我的发票', icon: 'IconFile' },
  ]
})

const selectedKeys = computed(() => [route.path])
const pageTitle = computed(() => {
  const map: Record<string, string> = {
    '/': userStore.isAdmin ? '数据概览' : '工作台',
    '/upload': '上传发票',
    '/invoices': userStore.isAdmin ? '全部票据' : '我的发票',
    '/members': '成员管理',
    '/export': '导出中心',
    '/settings': '系统设置',
  }
  return map[route.path] || '票管家'
})
const pageSub = computed(() => {
  const map: Record<string, string> = {
    '/': userStore.isAdmin ? '团队报销的整体情况' : '你的报销进度一览',
    '/upload': '图片与 PDF 自动识别，离线完成',
    '/invoices': userStore.isAdmin ? '团队所有报销记录' : '你上报的每一张票据',
    '/members': '开账号、重置密码、停用成员',
    '/export': '按时间范围导出附件与 Excel 汇总',
    '/settings': '识别、存储与团队偏好',
  }
  return map[route.path] || ''
})

function onMenuClick(key: string) {
  if (key !== route.path) router.push(key)
}

function openPassword() {
  resetPwd()
  pwdVisible.value = true
}

function resetPwd() {
  pwd.old = ''
  pwd.pwd = ''
  pwd.pwd2 = ''
}

async function submitPassword() {
  if (!pwd.old) {
    Message.warning('请输入原密码')
    return false
  }
  if (pwd.pwd.length < 6) {
    Message.warning('新密码至少 6 位')
    return false
  }
  if (pwd.pwd !== pwd.pwd2) {
    Message.warning('两次输入不一致')
    return false
  }
  try {
    await api.changePassword(pwd.old, pwd.pwd)
    Message.success('密码已更新')
    return true
  } catch (e: any) {
    Message.error(e.message)
    return false
  }
}

function openHelp() {
  Modal.info({
    title: '怎么用？',
    content:
      '1. 点「上传发票」，拖入图片或 PDF，系统会自动识别票面信息；\n' +
      '2. 核对自动填好的表单，补充备注后点「下一步」；\n' +
      '3. 确认无误点「确定并提交」；\n' +
      '4. 财务报销后，可自行把状态改为「已报销」。',
    okText: '知道了',
  })
}

async function logout() {
  await userStore.logout()
  router.replace('/login')
}

function onResize() {
  isNarrow.value = window.innerWidth < 1200
  if (window.innerWidth < 1200) collapsed.value = true
}

onMounted(() => {
  onResize()
  window.addEventListener('resize', onResize)
  if (!userStore.user) userStore.fetchMe().catch(() => {})
})
onUnmounted(() => window.removeEventListener('resize', onResize))
</script>

<style scoped>
.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 10px;
  cursor: pointer;
  transition: background 0.15s;
}

.user-chip:hover {
  background: rgba(255, 255, 255, 0.06);
}

.user-name {
  color: #fff;
  font-size: 13px;
  font-weight: 550;
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-role {
  color: #7d86a3;
  font-size: 11px;
}
</style>
