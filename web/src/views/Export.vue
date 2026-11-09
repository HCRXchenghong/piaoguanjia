<template>
  <div class="page">
    <a-grid :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>设置导出范围</h3>
            <span class="sub">生成 ZIP 压缩包</span>
          </div>
          <div class="panel-body">
            <a-form :model="form" layout="vertical">
              <a-form-item label="时间范围">
                <a-radio-group v-model="mode" type="button" @change="refresh">
                  <a-radio value="month">按月份</a-radio>
                  <a-radio value="year">按年份</a-radio>
                  <a-radio value="custom">自定义</a-radio>
                  <a-radio value="all">全部</a-radio>
                </a-radio-group>
              </a-form-item>

              <a-form-item v-if="mode === 'month'" label="选择月份">
                <a-select v-model="form.month" style="width: 220px" @change="refresh">
                  <a-option v-for="m in months" :key="m.value" :value="m.value">{{ m.label }}</a-option>
                </a-select>
              </a-form-item>
              <a-form-item v-else-if="mode === 'year'" label="选择年份">
                <a-select v-model="form.year" style="width: 160px" @change="refresh">
                  <a-option v-for="y in years" :key="y" :value="String(y)">{{ y }} 年</a-option>
                </a-select>
              </a-form-item>
              <a-form-item v-else-if="mode === 'custom'" label="起止日期">
                <a-range-picker v-model="form.range" value-format="YYYY-MM-DD" style="width: 280px" @change="refresh" />
              </a-form-item>

              <a-row :gutter="14">
                <a-col :span="12">
                  <a-form-item label="报销类型">
                    <a-select v-model="form.type" allow-clear placeholder="全部类型" @change="refresh">
                      <a-option value="procurement">采购报销</a-option>
                      <a-option value="travel">出行报销</a-option>
                    </a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item label="报销状态">
                    <a-select v-model="form.status" allow-clear placeholder="全部状态" @change="refresh">
                      <a-option value="pending">未报销</a-option>
                      <a-option value="reimbursed">已报销</a-option>
                    </a-select>
                  </a-form-item>
                </a-col>
              </a-row>

              <a-form-item label="指定成员" style="margin-bottom: 0">
                <a-select v-model="form.user_id" allow-clear placeholder="全部成员" @change="refresh">
                  <a-option v-for="u in members" :key="u.id" :value="String(u.id)">{{ u.display_name }}</a-option>
                </a-select>
              </a-form-item>
            </a-form>

            <a-alert style="margin-top: 16px">
              压缩包内包含：<b>发票原件</b>（重命名为「附件001_类别_金额元.pdf」）、
              <b>采购报销明细.xlsx</b> 与 <b>出行报销明细.xlsx</b>（两类分开成表，没有数据的类型不生成文件），
              每个表含「明细 / 汇总 / 分类统计」三个工作表。
            </a-alert>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel">
          <div class="panel-head"><h3>导出预览</h3><span class="sub">按当前条件</span></div>
          <div class="panel-body">
            <a-spin :loading="loading" style="width: 100%">
              <div style="text-align: center; padding: 6px 0 18px">
                <div class="tiny muted">符合条件的票据</div>
                <div style="font-size: 34px; font-weight: 700; letter-spacing: -0.02em">
                  {{ preview.count }} <small style="font-size: 14px; color: var(--muted)">张</small>
                </div>
                <div style="font-size: 18px; font-weight: 600">¥{{ money(preview.amount) }}</div>
              </div>
              <a-descriptions :column="1" size="medium" :label-style="{ color: 'var(--muted)', fontSize: '12px' }">
                <a-descriptions-item label="统计区间">{{ rangeLabel }}</a-descriptions-item>
                <a-descriptions-item label="报销类型">
                  {{ form.type === 'procurement' ? '采购报销' : form.type === 'travel' ? '出行报销' : '全部' }}
                </a-descriptions-item>
                <a-descriptions-item label="报销状态">
                  {{ form.status === 'pending' ? '未报销' : form.status === 'reimbursed' ? '已报销' : '全部' }}
                </a-descriptions-item>
                <a-descriptions-item label="成员范围">
                  {{ form.user_id ? members.find((m) => String(m.id) === form.user_id)?.display_name : '全部成员' }}
                </a-descriptions-item>
                <a-descriptions-item label="文件名预览">
                  <span class="tiny muted">{{ fileNamePreview }}</span>
                </a-descriptions-item>
              </a-descriptions>
            </a-spin>
          </div>
          <div class="panel-body" style="border-top: 1px solid var(--line)">
            <a-button type="primary" long size="large" :disabled="!preview.count" :loading="downloading" @click="doExport">
              <template #icon><icon-download /></template>
              生成并下载压缩包
            </a-button>
            <div class="tiny muted" style="text-align: center; margin-top: 8px">文件较多时需要几秒钟</div>
          </div>
        </div>
      </a-grid-item>
    </a-grid>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api, exportUrl, type User } from '@/api'
import { money } from '@/utils/format'

const mode = ref<'month' | 'year' | 'custom' | 'all'>('year')
const now = new Date()
const form = reactive<any>({
  month: `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`,
  year: String(now.getFullYear()),
  range: [],
  type: '',
  status: '',
  user_id: '',
})
const preview = reactive({ count: 0, amount: 0 })
const loading = ref(false)
const downloading = ref(false)
const members = ref<User[]>([])

const years = computed(() => Array.from({ length: 5 }, (_, i) => now.getFullYear() - i))
const months = computed(() => {
  const d = new Date()
  const out = []
  for (let i = 0; i < 24; i += 1) {
    const y = d.getFullYear()
    const m = d.getMonth() + 1
    out.push({ value: `${y}-${String(m).padStart(2, '0')}`, label: `${y} 年 ${m} 月` })
    d.setMonth(d.getMonth() - 1)
  }
  return out
})

const fileNamePreview = computed(() => {
  let base = '发票汇总'
  if (mode.value === 'month') base += '_' + (months.value.find((m) => m.value === form.month)?.label.replace(/\s/g, '') || '')
  else if (mode.value === 'year') base += '_' + form.year + '年'
  else if (mode.value === 'custom' && form.range?.length === 2) base += '_' + form.range[0] + '至' + form.range[1]
  if (form.status === 'pending') base += '_未报销'
  if (form.status === 'reimbursed') base += '_已报销'
  return `${base}_日期_时间.zip`
})

const rangeLabel = computed(() => {
  if (mode.value === 'month') return months.value.find((m) => m.value === form.month)?.label || '—'
  if (mode.value === 'year') return `${form.year} 年`
  if (mode.value === 'custom') {
    return form.range?.length === 2 ? `${form.range[0]} 至 ${form.range[1]}` : '请选择日期'
  }
  return '全部历史'
})

function buildParams() {
  const p: any = {}
  if (mode.value === 'month' && form.month) {
    const [y, m] = form.month.split('-')
    p.year = y
    p.month = String(Number(m))
  } else if (mode.value === 'year') {
    p.year = form.year
  } else if (mode.value === 'custom' && form.range?.length === 2) {
    p.date_from = form.range[0]
    p.date_to = form.range[1]
  }
  if (form.type) p.type = form.type
  if (form.status) p.status = form.status
  if (form.user_id) p.user_id = form.user_id
  return p
}

async function refresh() {
  loading.value = true
  try {
    const data = await api.invoices({ ...buildParams(), page: 1, page_size: 1 })
    preview.count = data.total
    preview.amount = data.sum_amount
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    loading.value = false
  }
}

function doExport() {
  downloading.value = true
  window.open(exportUrl(buildParams()), '_blank')
  setTimeout(() => {
    downloading.value = false
    Message.success(`已开始下载 ${preview.count} 张票据的压缩包`)
  }, 1200)
}

onMounted(async () => {
  try {
    const data = await api.users()
    members.value = data.users.filter((u: User) => u.active)
  } catch {
    /* ignore */
  }
  refresh()
})
</script>
