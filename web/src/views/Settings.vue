<template>
  <div class="page">
    <a-grid :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel mb-16">
          <div class="panel-head">
            <h3>团队信息</h3>
            <div class="spacer"></div>
            <a-button type="primary" size="small" :loading="saving" @click="save">保存设置</a-button>
          </div>
          <div class="panel-body">
            <a-form layout="vertical">
              <a-form-item label="团队 / 公司名称" extra="显示在侧边栏与导出文件名中">
                <a-input v-model="form.org_name" placeholder="例如：星河科技" />
              </a-form-item>
            </a-form>
          </div>
        </div>

        <div class="panel mb-16">
          <div class="panel-head"><h3>消费类别</h3><span class="sub">成员填单时可选</span></div>
          <div class="panel-body">
            <a-tag
              v-for="(c, i) in form.categories"
              :key="c"
              closable
              style="margin: 0 8px 8px 0; padding: 2px 10px"
              @close="form.categories.splice(i, 1)"
            >
              {{ c }}
            </a-tag>
            <div style="display: flex; gap: 8px; margin-top: 6px; max-width: 420px">
              <a-input v-model="newCategory" placeholder="输入类别后回车添加" @press-enter="addCategory" />
              <a-button @click="addCategory">添加</a-button>
            </div>
          </div>
        </div>

        <div class="panel mb-16">
          <div class="panel-head"><h3>上传与识别</h3></div>
          <div class="panel-body">
            <a-form layout="vertical">
              <a-form-item label="单文件大小上限（MB）" extra="超过该大小的文件会被拒绝">
                <a-input-number v-model="form.max_upload_mb" :min="1" :max="100" style="width: 160px" />
              </a-form-item>
              <a-form-item label="离线识别引擎" extra="完全离线，票面数据不出内网；PDF 文字层不依赖 OCR">
                <a-space direction="vertical" style="width: 100%" :size="8">
                  <a-radio-group v-model="ocrEngine" type="button">
                    <a-radio value="auto">自动</a-radio>
                    <a-radio value="windows">Windows 内置</a-radio>
                    <a-radio value="tesseract">Tesseract</a-radio>
                    <a-radio value="none">关闭</a-radio>
                  </a-radio-group>
                  <div class="tiny">
                    <a-tag :color="settings.ocr_available ? 'green' : 'orange'" size="small">
                      {{ settings.ocr_available ? '可用' : '不可用' }}
                    </a-tag>
                    <span style="margin-left: 6px">当前生效：{{ settings.ocr_engine_label || '未启用' }}</span>
                    <span v-if="settings.ocr_detail" class="muted"> · {{ settings.ocr_detail }}</span>
                  </div>
                  <div class="tiny muted">
                    平台：{{ settings.ocr_platform || '—' }}。Linux / macOS 需安装
                    <code>tesseract-ocr</code> 与中文语言包 <code>tesseract-ocr-chi-sim</code>。
                  </div>
                </a-space>
              </a-form-item>
            </a-form>
          </div>
        </div>

        <div class="panel">
          <div class="panel-head">
            <h3>存储与保留</h3>
            <span class="sub">自动清理策略</span>
          </div>
          <div class="panel-body">
            <a-form layout="vertical">
              <a-form-item label="永久保存" extra="开启后系统不会自动删除任何票据与附件">
                <a-switch v-model="permanent" />
              </a-form-item>
              <template v-if="!permanent">
                <a-row :gutter="14">
                  <a-col :span="12">
                    <a-form-item label="保留天数">
                      <a-input-number v-model="form.retention_days" :min="1" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="清理范围" extra="只删原件可保留台账">
                      <a-select v-model="form.retention_scope">
                        <a-option value="files">仅删除原始附件</a-option>
                        <a-option value="all">删除记录与附件</a-option>
                      </a-select>
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-alert type="warning">清理任务每天凌晨自动执行一次，也可在右侧手动触发。删除操作不可恢复，请先备份。</a-alert>
              </template>
            </a-form>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel mb-16">
          <div class="panel-head"><h3>数据概览</h3></div>
          <div class="panel-body">
            <a-descriptions :column="2" size="medium" :label-style="{ color: 'var(--muted)', fontSize: '12px' }">
              <a-descriptions-item label="票据总数">{{ storage.invoices }} 张</a-descriptions-item>
              <a-descriptions-item label="附件文件">{{ storage.files }} 个</a-descriptions-item>
              <a-descriptions-item label="占用空间">{{ sizeText }}</a-descriptions-item>
              <a-descriptions-item label="保留策略">
                {{ form.retention_mode === 'permanent' ? '永久保存' : `保留 ${form.retention_days} 天` }}
              </a-descriptions-item>
            </a-descriptions>
            <a-space direction="vertical" fill style="width: 100%; margin-top: 14px">
              <a-button long @click="backup">
                <template #icon><icon-archive /></template>
                下载完整备份（数据 + 附件）
              </a-button>
              <a-popconfirm content="将按当前策略清理过期数据，确定吗？" @ok="cleanup">
                <a-button long status="danger">
                  <template #icon><icon-delete /></template>
                  立即执行保留期清理
                </a-button>
              </a-popconfirm>
            </a-space>
          </div>
        </div>

        <div class="panel mb-16">
          <div class="panel-head"><h3>安全建议</h3></div>
          <div class="panel-body tiny muted" style="line-height: 1.9">
            · 服务默认监听局域网，请勿直接暴露到公网；如需外网访问，建议用 VPN 或反向代理加 HTTPS。<br />
            · 定期下载备份并另存到其他设备。<br />
            · 管理员账号请使用强密码，离职成员及时停用。
          </div>
        </div>

        <div class="panel">
          <div class="panel-head"><h3>操作日志</h3><span class="sub">最近 60 条</span></div>
          <div style="max-height: 420px; overflow: auto; padding: 6px 16px">
            <div
              v-for="log in logs"
              :key="log.id"
              style="display: flex; gap: 10px; padding: 9px 0; border-bottom: 1px solid #f2f3f8; font-size: 12.5px"
            >
              <div class="tiny muted" style="width: 78px; flex: none">{{ (log.created_at || '').slice(5, 16) }}</div>
              <div style="flex: 1; min-width: 0">
                <div><b>{{ log.actor_name || '系统' }}</b> · {{ actionLabel(log.action) }}</div>
                <div class="tiny muted" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ log.detail }}
                </div>
              </div>
            </div>
            <a-empty v-if="!logs.length" description="暂无日志" />
          </div>
        </div>
      </a-grid-item>
    </a-grid>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api, backupUrl } from '@/api'
import { useUserStore } from '@/stores/user'
import { fileSize } from '@/utils/format'

const userStore = useUserStore()
const saving = ref(false)
const newCategory = ref('')
const permanent = ref(true)
const ocrEngine = ref('auto')
const settings = reactive<any>({ ocr_available: false })
const storage = reactive({ invoices: 0, files: 0, bytes: 0 })
const logs = ref<any[]>([])

const form = reactive<any>({
  org_name: '', categories: [], max_upload_mb: 20,
  retention_mode: 'permanent', retention_days: 365, retention_scope: 'files',
})

const actionLabels: Record<string, string> = {
  setup: '初始化系统', login: '登录', create_invoice: '提交票据', update_invoice: '更新票据',
  delete_invoice: '删除票据', export: '导出数据', create_user: '新建成员', update_user: '更新成员',
  reset_password: '重置密码', delete_user: '删除成员', update_settings: '修改设置',
  change_password: '修改密码', retention_cleanup: '保留期清理',
}
const actionLabel = (a: string) => actionLabels[a] || a

const sizeText = computed(() => fileSize(storage.bytes))

function addCategory() {
  const v = newCategory.value.trim()
  if (!v) return
  if (!form.categories.includes(v)) form.categories.push(v)
  newCategory.value = ''
}

async function load() {
  try {
    const data = await api.settings()
    const s = data.settings
    Object.assign(settings, s)
    form.org_name = s.org_name || ''
    form.categories = Array.isArray(s.categories) ? [...s.categories] : []
    form.max_upload_mb = Number(s.max_upload_mb || 20)
    form.retention_mode = s.retention_mode || 'permanent'
    form.retention_days = Number(s.retention_days || 365)
    form.retention_scope = s.retention_scope || 'files'
    permanent.value = form.retention_mode === 'permanent'
    ocrEngine.value = s.ocr_engine || 'auto'
  } catch (e: any) {
    Message.error(e.message)
  }
  try {
    const info = await api.storage()
    storage.invoices = info.invoices
    storage.files = info.files
    storage.bytes = info.bytes
  } catch {
    /* ignore */
  }
  try {
    const audit = await api.audit(60)
    logs.value = audit.logs
  } catch {
    /* ignore */
  }
}

async function save() {
  saving.value = true
  try {
    const payload = {
      org_name: form.org_name,
      categories: form.categories,
      max_upload_mb: form.max_upload_mb,
      ocr_engine: ocrEngine.value,
      retention_mode: permanent.value ? 'permanent' : 'auto',
      retention_days: form.retention_days,
      retention_scope: form.retention_scope,
    }
    const data = await api.saveSettings(payload)
    userStore.settings = { ...userStore.settings, ...data.settings }
    Message.success('设置已保存')
    load()
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    saving.value = false
  }
}

function backup() {
  window.open(backupUrl(), '_blank')
  Message.info('开始下载备份…')
}

async function cleanup() {
  try {
    const data = await api.cleanup()
    const r = data.result
    Message.success(r.skipped ? '当前为永久保存，无需清理' : `清理完成：${r.expired} 条记录、${r.files} 个附件`)
    load()
  } catch (e: any) {
    Message.error(e.message)
  }
}

onMounted(load)
</script>
