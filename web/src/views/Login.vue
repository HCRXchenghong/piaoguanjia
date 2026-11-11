<template>
  <div class="auth-page">
    <aside class="auth-side">
      <div class="brand" style="padding: 0; border: 0; height: auto">
        <div class="brand-logo"><icon-file /></div>
        <div>
          <div class="brand-name">{{ state.orgName || '票管家' }}</div>
          <div class="brand-sub">INVOICE HUB</div>
        </div>
      </div>

      <div class="auth-copy">
        <h1>把发票收进一个地方，<br />让报销不再丢三落四。</h1>
        <p>上传图片或 PDF，系统离线识别票面信息并自动填单，成员确认后提交，管理员一键导出带附件的 Excel 汇总。</p>
        <div class="auth-points">
          <div class="auth-point">
            <div class="dot"><icon-thunderbolt /></div>
            <div>离线 OCR：不联网也能识别，票面数据不出内网</div>
          </div>
          <div class="auth-point">
            <div class="dot"><icon-archive /></div>
            <div>附件自动重命名归档，导出即用</div>
          </div>
          <div class="auth-point">
            <div class="dot"><icon-safe /></div>
            <div>按成员分账号，报销状态全程可追溯</div>
          </div>
        </div>
      </div>

      <div class="auth-foot">票管家 v1.1.0 · Go + MySQL + Arco Design Vue · 构建 {{ buildTime }}</div>
    </aside>

    <main class="auth-main">
      <div class="auth-card">
        <template v-if="state.mode === 'login'">
          <h2>欢迎回来</h2>
          <div class="sub">登录后即可上传发票、跟踪报销进度</div>
          <a-form :model="loginForm" layout="vertical" @submit="doLogin">
            <a-form-item field="username" label="用户名">
              <a-input v-model="loginForm.username" placeholder="请输入用户名" size="large" allow-clear>
                <template #prefix><icon-user /></template>
              </a-input>
            </a-form-item>
            <a-form-item field="password" label="密码">
              <a-input-password v-model="loginForm.password" placeholder="请输入密码" size="large" allow-clear>
                <template #prefix><icon-lock /></template>
              </a-input-password>
            </a-form-item>
            <a-button type="primary" size="large" long :loading="state.loading" html-type="submit">
              登录
            </a-button>
            <div class="muted tiny" style="text-align: center; margin-top: 14px">
              账号由管理员创建 · 忘记密码请联系管理员重置
            </div>
          </a-form>
        </template>

        <template v-else>
          <h2>初始化你的发票系统</h2>
          <div class="sub">只需一次，创建第一个管理员账号</div>
          <a-form :model="setupForm" layout="vertical" @submit="doSetup">
            <a-form-item field="org_name" label="团队 / 公司名称">
              <a-input v-model="setupForm.org_name" placeholder="例如：星河科技" size="large" />
            </a-form-item>
            <a-form-item field="username" label="管理员用户名">
              <a-input v-model="setupForm.username" size="large" />
            </a-form-item>
            <a-form-item field="display_name" label="显示名称">
              <a-input v-model="setupForm.display_name" placeholder="例如：财务小林" size="large" />
            </a-form-item>
            <a-form-item field="password" label="登录密码">
              <a-input-password v-model="setupForm.password" placeholder="至少 6 位" size="large" />
            </a-form-item>
            <a-button type="primary" size="large" long :loading="state.loading" html-type="submit">
              创建并进入系统
            </a-button>
          </a-form>
        </template>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { useUserStore } from '@/stores/user'

const buildTime = __BUILD_TIME__
const router = useRouter()
const userStore = useUserStore()

const state = reactive({
  mode: 'login' as 'login' | 'setup',
  loading: false,
  orgName: '',
})

const loginForm = reactive({ username: '', password: '' })
const setupForm = reactive({ org_name: '', username: 'admin', display_name: '', password: '' })

onMounted(async () => {
  try {
    const boot = await api.bootstrap()
    state.orgName = boot.settings?.org_name || ''
    state.mode = boot.needs_setup ? 'setup' : 'login'
  } catch (e: any) {
    Message.error(e.message)
  }
})

async function doLogin() {
  if (!loginForm.username || !loginForm.password) {
    Message.warning('请输入用户名和密码')
    return
  }
  state.loading = true
  try {
    await userStore.login(loginForm.username.trim(), loginForm.password)
    Message.success('登录成功')
    router.replace('/')
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    state.loading = false
  }
}

async function doSetup() {
  if (!setupForm.org_name || !setupForm.username || !setupForm.display_name || !setupForm.password) {
    Message.warning('请填写完整信息')
    return
  }
  state.loading = true
  try {
    await userStore.setup({ ...setupForm })
    Message.success('初始化完成，欢迎使用')
    router.replace('/')
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    state.loading = false
  }
}
</script>
