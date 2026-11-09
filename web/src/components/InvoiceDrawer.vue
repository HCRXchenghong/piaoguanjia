<template>
  <a-drawer
    :visible="visible"
    :width="520"
    :footer="false"
    unmount-on-close
    @cancel="close"
  >
    <template #title>
      <div v-if="invoice">
        <a-space style="margin-bottom: 6px">
          <a-tag :color="invoice.type === 'travel' ? 'green' : 'arcoblue'" size="small">
            {{ invoice.type === 'travel' ? '出行报销' : '采购报销' }}
          </a-tag>
          <a-tag :color="invoice.status === 'reimbursed' ? 'green' : 'orange'" size="small">
            {{ invoice.status === 'reimbursed' ? '已报销' : '未报销' }}
          </a-tag>
        </a-space>
        <div style="font-size: 16px; font-weight: 600">{{ invoice.title }}</div>
        <div class="tiny muted">{{ invoice.category || '未分类' }} · 编号 #{{ invoice.id }}</div>
      </div>
      <span v-else>票据详情</span>
    </template>

    <a-spin :loading="loading" style="width: 100%">
      <template v-if="invoice">
        <div style="padding: 6px 0 16px; border-bottom: 1px solid var(--line); margin-bottom: 16px">
          <div class="tiny muted">报销金额（税后）</div>
          <div style="font-size: 28px; font-weight: 700; letter-spacing: -0.02em">
            <span style="font-size: 15px; color: var(--muted)">¥</span>{{ money(invoice.amount) }}
          </div>
          <div style="margin-top: 6px">
            <a-tag v-if="invoice.ocr_engine" size="small" color="gray">
              {{ engineText(invoice.ocr_engine) }}
              <template v-if="invoice.ocr_confidence"> · {{ Math.round(invoice.ocr_confidence * 100) }}%</template>
            </a-tag>
          </div>
        </div>

        <a-descriptions :column="2" size="medium" :label-style="{ color: 'var(--muted)', fontSize: '12px' }">
          <template v-if="invoice.type === 'procurement'">
            <a-descriptions-item label="名称">{{ invoice.title }}</a-descriptions-item>
            <a-descriptions-item label="类别">{{ invoice.category || '—' }}</a-descriptions-item>
            <a-descriptions-item label="规格">{{ invoice.spec || '—' }}</a-descriptions-item>
            <a-descriptions-item label="数量">{{ invoice.qty ?? '—' }}</a-descriptions-item>
            <a-descriptions-item label="税后单价">
              {{ invoice.unit_price != null ? '¥' + money(invoice.unit_price) : '—' }}
            </a-descriptions-item>
            <a-descriptions-item label="运费">
              {{ invoice.shipping ? '¥' + money(invoice.shipping) : '¥0.00' }}
            </a-descriptions-item>
          </template>
          <template v-else>
            <a-descriptions-item label="出行事由">{{ invoice.title }}</a-descriptions-item>
            <a-descriptions-item label="出行时间">{{ invoice.travel_time || '—' }}</a-descriptions-item>
            <a-descriptions-item label="交通工具">{{ invoice.transport || '—' }}</a-descriptions-item>
            <a-descriptions-item label="出行人">{{ invoice.traveler || '—' }}</a-descriptions-item>
            <a-descriptions-item label="出行路线" :span="2">{{ invoice.route || '—' }}</a-descriptions-item>
          </template>
          <a-descriptions-item label="发票号码">{{ invoice.invoice_no || '—' }}</a-descriptions-item>
          <a-descriptions-item label="开票日期">{{ invoice.invoice_date || '—' }}</a-descriptions-item>
          <a-descriptions-item label="销售方" :span="2">{{ invoice.seller || '—' }}</a-descriptions-item>
          <a-descriptions-item label="上报人">{{ invoice.display_name }}</a-descriptions-item>
          <a-descriptions-item label="上报时间">{{ dateTime(invoice.created_at) }}</a-descriptions-item>
          <a-descriptions-item v-if="invoice.reimbursed_at" label="报销时间" :span="2">
            {{ dateTime(invoice.reimbursed_at) }}
          </a-descriptions-item>
        </a-descriptions>

        <template v-if="invoice.note">
          <div style="font-size: 13px; font-weight: 600; margin: 18px 0 8px">备注</div>
          <div style="background: #fbfbfe; border: 1px solid var(--line); border-radius: 10px; padding: 12px; font-size: 13px; line-height: 1.7; white-space: pre-wrap">
            {{ invoice.note }}
          </div>
        </template>

        <div style="font-size: 13px; font-weight: 600; margin: 18px 0 8px">
          附件（{{ invoice.files?.length || 0 }}）
        </div>
        <a-space direction="vertical" fill style="width: 100%">
          <a
            v-for="f in invoice.files"
            :key="f.id"
            :href="fileUrl(f.id)"
            target="_blank"
            class="file-row"
            style="text-decoration: none; color: inherit"
          >
            <div class="file-thumb">
              <img
                v-if="f.mime.startsWith('image/')"
                :src="thumbUrl(f.id)"
                style="width: 100%; height: 100%; object-fit: cover"
                @error="(e: any) => (e.target.style.display = 'none')"
              />
              <icon-file v-else />
            </div>
            <div class="file-meta">
              <div class="file-name">{{ f.name }}</div>
              <div class="file-sub">{{ (f.ext || '').toUpperCase() }} · {{ fileSize(f.size) }}</div>
            </div>
            <icon-eye />
          </a>
        </a-space>

        <div style="display: flex; gap: 10px; margin-top: 24px">
          <a-popconfirm content="删除后该票据与附件将从列表移除，确定吗？" @ok="remove">
            <a-button status="danger">
              <template #icon><icon-delete /></template>
              删除
            </a-button>
          </a-popconfirm>
          <div style="flex: 1"></div>
          <a-button v-if="invoice.status === 'pending'" type="primary" :loading="saving" @click="toggleStatus">
            标记为已报销
          </a-button>
          <a-button v-else :loading="saving" @click="toggleStatus">改回未报销</a-button>
        </div>
      </template>
    </a-spin>
  </a-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api, fileUrl, thumbUrl, type Invoice } from '@/api'
import { dateTime, engineText, fileSize, money } from '@/utils/format'

const props = defineProps<{ visible: boolean; invoiceId: number }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const invoice = ref<Invoice | null>(null)
const loading = ref(false)
const saving = ref(false)

watch(
  () => [props.visible, props.invoiceId],
  async ([visible, id]) => {
    if (!visible || !id) return
    loading.value = true
    try {
      const data = await api.invoice(Number(id))
      invoice.value = data.invoice
    } catch (e: any) {
      Message.error(e.message)
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

function close() {
  emit('update:visible', false)
}

async function toggleStatus() {
  if (!invoice.value) return
  saving.value = true
  try {
    const next = invoice.value.status === 'reimbursed' ? 'pending' : 'reimbursed'
    const data = await api.updateInvoice(invoice.value.id, { status: next })
    invoice.value = data.invoice
    Message.success(next === 'reimbursed' ? '已标记为已报销' : '已改回未报销')
    emit('changed')
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!invoice.value) return
  try {
    await api.deleteInvoice(invoice.value.id)
    Message.success('已删除')
    emit('changed')
    close()
  } catch (e: any) {
    Message.error(e.message)
  }
}
</script>

<style scoped>
:deep(.arco-drawer-header) {
  height: auto;
  min-height: 64px;
  padding: 16px 20px;
  align-items: flex-start;
}

:deep(.arco-drawer-header-title) {
  flex: 1;
  min-width: 0;
}
</style>
