<template>
  <div class="page">
    <div class="panel mb-16">
      <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 14px 18px">
        <a-radio-group v-model="filters.status" type="button" size="small" @change="reload">
          <a-radio value="">全部</a-radio>
          <a-radio value="pending">未报销</a-radio>
          <a-radio value="reimbursed">已报销</a-radio>
        </a-radio-group>
        <span class="tiny muted" style="margin-right: 4px">共 {{ total }} 张 · 合计 ¥{{ money(sumAmount) }}</span><div style="flex: 1"></div>
        <a-select v-model="filters.type" size="small" placeholder="全部类型" allow-clear style="width: 120px" @change="reload">
          <a-option value="procurement">采购报销</a-option>
          <a-option value="travel">出行报销</a-option>
        </a-select>
        <a-select v-model="filters.month" size="small" placeholder="全部月份" allow-clear style="width: 128px" @change="reload">
          <a-option v-for="m in months" :key="m.value" :value="m.value">{{ m.label }}</a-option>
        </a-select>
        <a-input-search
          v-model="filters.q"
          size="small"
          placeholder="搜索名称 / 销方 / 票号"
          style="width: 220px"
          @search="reload"
          @clear="reload"
        />
        <a-button size="small" @click="download">
          <template #icon><icon-download /></template>
          导出
        </a-button>
      </div>
    </div>

    <div class="panel">
      <a-table
        :data="list"
        :loading="loading"
        row-key="id"
        :pagination="pagination"
        :bordered="false"
        @page-change="onPageChange"
        @row-click="(record: any) => openDetail(record.id)"
        :row-class="() => 'clickable-row'"
      >
        <template #columns>
          <a-table-column title="票据" data-index="title" :ellipsis="true" :tooltip="true">
            <template #cell="{ record }">
              <div class="table-title">{{ record.title }}</div>
              <div class="table-sub">
                {{ record.category || record.route || '未分类' }}
                <template v-if="record.invoice_no"> · {{ record.invoice_no }}</template>
              </div>
            </template>
          </a-table-column>
          <a-table-column title="类型" :width="110">
            <template #cell="{ record }">
              <a-tag :color="record.type === 'travel' ? 'green' : 'arcoblue'" size="small">
                {{ record.type === 'travel' ? '出行' : '采购' }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column title="金额" align="right" :width="120">
            <template #cell="{ record }">
              <span class="money-cell">¥{{ money(record.amount) }}</span>
            </template>
          </a-table-column>
          <a-table-column title="报销状态" :width="110">
            <template #cell="{ record }">
              <a-tag :color="record.status === 'reimbursed' ? 'green' : 'orange'" size="small">
                {{ record.status === 'reimbursed' ? '已报销' : '未报销' }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column v-if="isAdmin" title="上报人" :width="140">
            <template #cell="{ record }">
              <div class="person">
                <span class="avatar-chip">{{ initial(record.display_name) }}</span>
                <span>{{ record.display_name }}</span>
              </div>
            </template>
          </a-table-column>
          <a-table-column title="开票日期" :width="120">
            <template #cell="{ record }">
              <span class="tiny muted">{{ record.invoice_date || shortDate(record.created_at) }}</span>
            </template>
          </a-table-column>
          <a-table-column title="操作" align="right" :width="130">
            <template #cell="{ record }">
              <a-space @click.stop>
                <a-button type="text" size="mini" @click="openDetail(record.id)">
                  <icon-eye />
                </a-button>
                <a-button type="text" size="mini" @click="toggle(record)">
                  <icon-check v-if="record.status === 'pending'" />
                  <icon-refresh v-else />
                </a-button>
              </a-space>
            </template>
          </a-table-column>
        </template>
        <template #empty>
          <a-empty description="没有找到票据">
            <a-button type="primary" @click="router.push('/upload')">上传发票</a-button>
          </a-empty>
        </template>
      </a-table>

    </div>

    <InvoiceDrawer v-model:visible="drawerVisible" :invoice-id="currentId" @changed="load" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { api, exportUrl, type Invoice } from '@/api'
import { useUserStore } from '@/stores/user'
import { initial, money, shortDate } from '@/utils/format'
import InvoiceDrawer from '@/components/InvoiceDrawer.vue'

const router = useRouter()
const userStore = useUserStore()
const isAdmin = computed(() => userStore.isAdmin)

const filters = reactive({ status: '', type: '', month: '', q: '' })
const list = ref<Invoice[]>([])
const total = ref(0)
const sumAmount = ref(0)
const page = ref(1)
const pageSize = ref(15)
const loading = ref(false)
const drawerVisible = ref(false)
const currentId = ref(0)

const months = computed(() => {
  const d = new Date()
  const out = []
  for (let i = 0; i < 18; i += 1) {
    const y = d.getFullYear()
    const m = d.getMonth() + 1
    out.push({ value: `${y}-${String(m).padStart(2, '0')}`, label: `${y} 年 ${m} 月` })
    d.setMonth(d.getMonth() - 1)
  }
  return out
})

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showTotal: true,
  showJumper: total.value > pageSize.value * 3,
  size: 'small' as const,
}))

function buildParams() {
  const params: any = { page: page.value, page_size: pageSize.value }
  if (filters.status) params.status = filters.status
  if (filters.type) params.type = filters.type
  if (filters.q) params.q = filters.q
  if (filters.month) {
    const [y, m] = filters.month.split('-')
    params.year = y
    params.month = String(Number(m))
  }
  if (!isAdmin.value) params.scope = 'mine'
  return params
}

async function load() {
  loading.value = true
  try {
    const data = await api.invoices(buildParams())
    list.value = data.items
    total.value = data.total
    sumAmount.value = data.sum_amount
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  load()
}

function onPageChange(p: number) {
  page.value = p
  load()
}

function openDetail(id: number) {
  currentId.value = id
  drawerVisible.value = true
}

async function toggle(record: Invoice) {
  try {
    const next = record.status === 'reimbursed' ? 'pending' : 'reimbursed'
    await api.updateInvoice(record.id, { status: next })
    Message.success(next === 'reimbursed' ? '已标记为已报销' : '已改回未报销')
    load()
  } catch (e: any) {
    Message.error(e.message)
  }
}

function download() {
  const params: any = {}
  if (filters.status) params.status = filters.status
  if (filters.type) params.type = filters.type
  if (filters.month) {
    const [y, m] = filters.month.split('-')
    params.year = y
    params.month = String(Number(m))
  }
  if (!isAdmin.value) params.scope = 'mine'
  window.open(exportUrl(params), '_blank')
  Message.info('正在生成压缩包，请稍候…')
}

onMounted(load)
</script>

<style scoped>
:deep(.clickable-row) {
  cursor: pointer;
}
</style>
