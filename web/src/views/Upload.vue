<template>
  <div class="page">
    <a-steps :current="step" style="margin-bottom: 22px; max-width: 760px">
      <a-step title="上传识别" description="图片或 PDF" />
      <a-step title="补充信息" description="核对自动填充" />
      <a-step title="确认提交" description="提交到台账" />
    </a-steps>

    <!-- 第一步 -->
    <a-grid v-if="step === 0" :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>选择发票文件</h3>
            <span class="sub">可一次选择多张</span>
          </div>
          <div class="panel-body">
            <div
              class="dropzone"
              :class="{ dragging }"
              @click="pickFile"
              @dragover.prevent="dragging = true"
              @dragleave.prevent="dragging = false"
              @drop.prevent="onDrop"
            >
              <div class="dropzone-icon"><icon-upload /></div>
              <h4>点击选择，或把文件拖到这里</h4>
              <p>单个文件不超过 {{ maxMb }}MB，最多 8 个附件</p>
              <a-space style="margin-top: 12px">
                <a-tag v-for="t in ['PDF', 'JPG', 'PNG', 'WEBP', 'BMP']" :key="t" size="small">{{ t }}</a-tag>
              </a-space>
            </div>
            <input ref="fileInput" type="file" multiple accept=".pdf,.jpg,.jpeg,.png,.webp,.bmp" hidden @change="onSelect" />

            <a-space direction="vertical" fill style="width: 100%; margin-top: 16px">
              <div v-for="f in files" :key="f.id" class="file-row">
                <div class="file-thumb">
                  <img v-if="f.preview" :src="f.preview" style="width: 100%; height: 100%; object-fit: cover" />
                  <icon-file v-else />
                </div>
                <div class="file-meta">
                  <div class="file-name">{{ f.name }}</div>
                  <div class="file-sub">
                    {{ fileSize(f.size) }}
                    <template v-if="f.engine"> · {{ engineText(f.engine) }}</template>
                  </div>
                </div>
                <a-spin v-if="f.status === 'uploading'" :size="16" />
                <icon-check-circle v-else-if="f.status === 'done'" style="color: #10b981" />
                <a-tooltip v-else :content="f.error">
                  <icon-exclamation-circle style="color: #ef4444" />
                </a-tooltip>
                <a-button type="text" size="mini" @click="removeFile(f.id)">
                  <icon-close />
                </a-button>
              </div>
            </a-space>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel" style="height: 100%">
          <div class="panel-head">
            <h3>识别结果</h3>
            <span class="sub">离线引擎</span>
          </div>
          <div class="panel-body">
            <template v-if="firstParse">
              <a-space wrap style="margin-bottom: 14px">
                <a-tag color="arcoblue" size="small">{{ firstParse.kind_label }}</a-tag>
                <a-tag color="gray" size="small">{{ engineText(firstParse.engine) }}</a-tag>
                <a-tag :color="firstParse.confidence >= 0.8 ? 'green' : 'orange'" size="small">
                  识别度 {{ Math.round((firstParse.confidence || 0) * 100) }}%
                </a-tag>
              </a-space>

              <div class="parse-grid">
                <div class="parse-cell">
                  <div class="k">发票号码<ConfTag :conf="conf('invoice_no')" /></div>
                  <div class="v" :class="{ empty: !field('invoice_no') }">{{ field('invoice_no') || '未识别' }}</div>
                </div>
                <div class="parse-cell">
                  <div class="k">开票日期<ConfTag :conf="conf('invoice_date')" /></div>
                  <div class="v" :class="{ empty: !field('invoice_date') }">{{ field('invoice_date') || '未识别' }}</div>
                </div>
                <div class="parse-cell">
                  <div class="k">价税合计<ConfTag :conf="conf('total')" /></div>
                  <div class="v" :class="{ empty: !field('total') }">
                    {{ field('total') ? '¥' + money(field('total')) : '未识别' }}
                  </div>
                </div>
                <div class="parse-cell">
                  <div class="k">税额<ConfTag :conf="conf('tax')" /></div>
                  <div class="v" :class="{ empty: !field('tax') }">
                    {{ field('tax') ? '¥' + money(field('tax')) : '未识别' }}
                  </div>
                </div>
                <div class="parse-cell full">
                  <div class="k">销售方<ConfTag :conf="conf('seller')" /></div>
                  <div class="v" :class="{ empty: !field('seller') }">{{ field('seller') || '未识别' }}</div>
                </div>
                <div class="parse-cell full">
                  <div class="k">购买方<ConfTag :conf="conf('buyer')" /></div>
                  <div class="v" :class="{ empty: !field('buyer') }">{{ field('buyer') || '未识别' }}</div>
                </div>
              </div>

              <template v-if="firstParse.items?.length">
                <div style="font-size: 12.5px; font-weight: 600; margin: 16px 0 8px">票面明细</div>
                <div
                  v-for="(it, i) in firstParse.items"
                  :key="i"
                  style="display: flex; align-items: center; gap: 8px; font-size: 12.5px; background: #fbfbfe; border: 1px solid var(--line); border-radius: 8px; padding: 7px 10px; margin-bottom: 6px"
                >
                  <span style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                    {{ it.name }}<span v-if="it.spec" class="muted"> · {{ it.spec }}</span>
                  </span>
                  <span class="muted">{{ it.qty ? it.qty + ' × ' : '' }}{{ it.price ? '¥' + money(it.price) : '' }}</span>
                  <span style="font-weight: 600">{{ it.amount ? '¥' + money(it.amount) : '' }}</span>
                </div>
              </template>

              <a-alert v-if="firstParse.warnings?.length" type="warning" style="margin-top: 12px">
                {{ firstParse.warnings.join('；') }}
              </a-alert>
            </template>
            <a-empty v-else description="上传后自动识别票面信息" />
          </div>
        </div>
      </a-grid-item>
    </a-grid>

    <div v-if="step === 0" style="display: flex; align-items: center; margin-top: 20px">
      <span class="tiny muted">识别结果仅供参考，下一步可自行修改</span>
      <div style="flex: 1"></div>
      <a-button type="primary" size="large" :disabled="!hasUsableFile" @click="toStep2">
        下一步：补充信息<icon-right />
      </a-button>
    </div>

    <!-- 第二步 -->
    <a-grid v-else-if="step === 1" :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>补充票据信息</h3>
            <span class="sub">带「已识别」的字段来自 OCR</span>
            <div class="spacer"></div>
            <a-radio-group v-model="type" type="button" size="small" @change="onTypeChange">
              <a-radio value="procurement">采购报销</a-radio>
              <a-radio value="travel">出行报销</a-radio>
            </a-radio-group>
          </div>
          <div class="panel-body">
            <a-form :model="form" layout="vertical">
              <template v-if="type === 'procurement'">
                <a-form-item label="名称" required>
                  <template #label>
                    名称<span class="auto-mark" v-if="auto.has('title')">已识别</span>
                  </template>
                  <a-input v-model="form.title" placeholder="例如：A4 复印纸 70g" />
                </a-form-item>
                <a-row :gutter="14">
                  <a-col :span="12">
                    <a-form-item label="类别">
                      <template #label>类别<span class="auto-mark" v-if="auto.has('category')">已识别</span></template>
                      <a-select v-model="form.category" placeholder="请选择类别" allow-clear>
                        <a-option v-for="c in categories" :key="c" :value="c">{{ c }}</a-option>
                      </a-select>
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="规格">
                      <template #label>规格<span class="auto-mark" v-if="auto.has('spec')">已识别</span></template>
                      <a-input v-model="form.spec" placeholder="型号 / 规格" />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-row :gutter="14">
                  <a-col :span="8">
                    <a-form-item label="数量">
                      <template #label>数量<span class="auto-mark" v-if="auto.has('qty')">已识别</span></template>
                      <a-input-number v-model="form.qty" :min="0" :precision="2" placeholder="1" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="8">
                    <a-form-item label="税后单价">
                      <template #label>税后单价<span class="auto-mark" v-if="auto.has('unit_price')">已识别</span></template>
                      <a-input-number v-model="form.unit_price" :min="0" :precision="2" placeholder="0.00" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="8">
                    <a-form-item label="运费">
                      <template #label>运费<span class="auto-mark" v-if="auto.has('shipping')">已识别</span></template>
                      <a-input-number v-model="form.shipping" :min="0" :precision="2" placeholder="0.00" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-form-item label="购买链接">
                  <a-input v-model="form.link" placeholder="https://（选填）" />
                </a-form-item>
              </template>

              <template v-else>
                <a-form-item label="出行事由" required>
                  <template #label>出行事由<span class="auto-mark" v-if="auto.has('title')">已识别</span></template>
                  <a-input v-model="form.title" placeholder="例如：客户拜访 · 广州" />
                </a-form-item>
                <a-row :gutter="14">
                  <a-col :span="12">
                    <a-form-item label="出行时间">
                      <template #label>出行时间<span class="auto-mark" v-if="auto.has('travel_time')">已识别</span></template>
                      <a-date-picker v-model="form.travel_time" value-format="YYYY-MM-DD" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="交通工具">
                      <template #label>交通工具<span class="auto-mark" v-if="auto.has('transport')">已识别</span></template>
                      <a-select v-model="form.transport" allow-clear placeholder="请选择">
                        <a-option v-for="t in transports" :key="t" :value="t">{{ t }}</a-option>
                      </a-select>
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-form-item label="出行路线">
                  <template #label>出行路线<span class="auto-mark" v-if="auto.has('route')">已识别</span></template>
                  <a-input v-model="form.route" placeholder="例如：深圳北 → 广州南" />
                </a-form-item>
                <a-row :gutter="14">
                  <a-col :span="12">
                    <a-form-item label="出行人">
                      <template #label>出行人<span class="auto-mark" v-if="auto.has('traveler')">已识别</span></template>
                      <a-input v-model="form.traveler" placeholder="姓名" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="税后金额" required>
                      <template #label>税后金额<span class="auto-mark" v-if="auto.has('amount')">已识别</span></template>
                      <a-input-number v-model="form.amount" :min="0" :precision="2" placeholder="0.00" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                </a-row>
              </template>

              <a-form-item label="备注" style="margin-bottom: 0">
                <template #label>备注<span class="auto-mark" v-if="auto.has('note')">已识别</span></template>
                <a-textarea v-model="form.note" :auto-size="{ minRows: 3, maxRows: 6 }" placeholder="用途、项目、报销说明…" />
              </a-form-item>
            </a-form>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel mb-16">
          <div class="panel-head"><h3>票面信息</h3><span class="sub">自动识别</span></div>
          <div class="panel-body">
            <div class="parse-grid">
              <div class="parse-cell">
                <div class="k">发票号码</div>
                <div class="v" :class="{ empty: !meta.invoice_no }">{{ meta.invoice_no || '未识别' }}</div>
              </div>
              <div class="parse-cell">
                <div class="k">开票日期</div>
                <div class="v" :class="{ empty: !meta.invoice_date }">{{ meta.invoice_date || '未识别' }}</div>
              </div>
              <div class="parse-cell full">
                <div class="k">销售方</div>
                <div class="v" :class="{ empty: !meta.seller }">{{ meta.seller || '未识别' }}</div>
              </div>
              <div class="parse-cell">
                <div class="k">价税合计</div>
                <div class="v" :class="{ empty: !meta.total }">{{ meta.total ? '¥' + money(meta.total) : '未识别' }}</div>
              </div>
              <div class="parse-cell">
                <div class="k">税额</div>
                <div class="v" :class="{ empty: !meta.tax }">{{ meta.tax ? '¥' + money(meta.tax) : '未识别' }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="panel">
          <div class="panel-head"><h3>附件</h3><span class="sub">{{ files.length }} 个</span></div>
          <div class="panel-body" style="display: grid; gap: 8px">
            <div v-for="f in files" :key="f.id" class="file-row">
              <div class="file-thumb">
                <img v-if="f.preview" :src="f.preview" style="width: 100%; height: 100%; object-fit: cover" />
                <icon-file v-else />
              </div>
              <div class="file-meta">
                <div class="file-name">{{ f.name }}</div>
                <div class="file-sub">{{ fileSize(f.size) }}</div>
              </div>
            </div>
          </div>
        </div>
      </a-grid-item>
    </a-grid>

    <div v-if="step === 1" style="display: flex; align-items: center; margin-top: 20px">
      <a-button @click="step = 0"><icon-left /> 上一步</a-button>
      <div style="flex: 1"></div>
      <span class="tiny muted" style="margin-right: 14px">
        报销金额合计 <b style="color: var(--ink); font-size: 14px">¥{{ money(amount) }}</b>
        <span v-if="alignedToTotal" style="margin-left: 4px">（已对齐票面价税合计）</span>
      </span>
      <a-button type="primary" size="large" @click="toStep3">下一步：核对确认<icon-right /></a-button>
    </div>

    <!-- 第三步 -->
    <a-grid v-else-if="step === 2" :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>请核对最终信息</h3>
            <span class="sub">提交后将进入报销台账</span>
          </div>
          <div class="panel-body">
            <div style="display: flex; align-items: flex-start; padding-bottom: 16px; border-bottom: 1px solid var(--line); margin-bottom: 16px">
              <div style="flex: 1">
                <a-space style="margin-bottom: 6px">
                  <a-tag :color="type === 'travel' ? 'green' : 'arcoblue'" size="small">
                    {{ type === 'travel' ? '出行报销' : '采购报销' }}
                  </a-tag>
                  <a-tag color="orange" size="small">未报销</a-tag>
                </a-space>
                <div class="tiny muted">报销金额（税后）</div>
                <div style="font-size: 30px; font-weight: 700; letter-spacing: -0.02em">
                  <span style="font-size: 15px; color: var(--muted)">¥</span>{{ money(amount) }}
                </div>
              </div>
            </div>

            <a-descriptions :column="2" size="medium" :label-style="{ color: 'var(--muted)', fontSize: '12px' }">
              <template v-if="type === 'procurement'">
                <a-descriptions-item label="名称">{{ form.title }}</a-descriptions-item>
                <a-descriptions-item label="类别">{{ form.category || '—' }}</a-descriptions-item>
                <a-descriptions-item label="规格">{{ form.spec || '—' }}</a-descriptions-item>
                <a-descriptions-item label="数量">{{ form.qty ?? '—' }}</a-descriptions-item>
                <a-descriptions-item label="税后单价">{{ form.unit_price ? '¥' + money(form.unit_price) : '—' }}</a-descriptions-item>
                <a-descriptions-item label="运费">{{ form.shipping ? '¥' + money(form.shipping) : '¥0.00' }}</a-descriptions-item>
                <a-descriptions-item v-if="form.link" label="购买链接" :span="2">{{ form.link }}</a-descriptions-item>
              </template>
              <template v-else>
                <a-descriptions-item label="出行事由">{{ form.title }}</a-descriptions-item>
                <a-descriptions-item label="出行时间">{{ form.travel_time || '—' }}</a-descriptions-item>
                <a-descriptions-item label="交通工具">{{ form.transport || '—' }}</a-descriptions-item>
                <a-descriptions-item label="出行人">{{ form.traveler || '—' }}</a-descriptions-item>
                <a-descriptions-item label="出行路线" :span="2">{{ form.route || '—' }}</a-descriptions-item>
              </template>
              <a-descriptions-item label="备注" :span="2">{{ form.note || '—' }}</a-descriptions-item>
            </a-descriptions>

            <template v-if="meta.invoice_no || meta.seller">
              <div style="font-size: 12.5px; font-weight: 600; margin: 18px 0 8px">票面信息</div>
              <a-descriptions :column="2" size="medium" :label-style="{ color: 'var(--muted)', fontSize: '12px' }">
                <a-descriptions-item label="发票号码">{{ meta.invoice_no || '—' }}</a-descriptions-item>
                <a-descriptions-item label="开票日期">{{ meta.invoice_date || '—' }}</a-descriptions-item>
                <a-descriptions-item label="销售方" :span="2">{{ meta.seller || '—' }}</a-descriptions-item>
                <a-descriptions-item label="价税合计">{{ meta.total ? '¥' + money(meta.total) : '—' }}</a-descriptions-item>
              </a-descriptions>
            </template>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel">
          <div class="panel-head"><h3>附件将一并归档</h3><span class="sub">{{ files.length }} 个</span></div>
          <div class="panel-body" style="display: grid; gap: 8px">
            <div v-for="f in files" :key="f.id" class="file-row">
              <div class="file-thumb">
                <img v-if="f.preview" :src="f.preview" style="width: 100%; height: 100%; object-fit: cover" />
                <icon-file v-else />
              </div>
              <div class="file-meta">
                <div class="file-name">{{ f.name }}</div>
                <div class="file-sub">{{ fileSize(f.size) }}</div>
              </div>
            </div>
          </div>
        </div>
      </a-grid-item>
    </a-grid>

    <div v-if="step === 2" style="display: flex; align-items: center; margin-top: 20px">
      <a-button @click="step = 1"><icon-left /> 上一步</a-button>
      <div style="flex: 1"></div>
      <span class="tiny muted" style="margin-right: 14px">提交后仍可在「我的发票」中修改报销状态</span>
      <a-button type="primary" size="large" :loading="submitting" @click="submit">
        <template #icon><icon-check /></template>
        确定并提交
      </a-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { api, type ParseResult } from '@/api'
import { useUserStore } from '@/stores/user'
import { engineText, fileSize, money } from '@/utils/format'

const router = useRouter()
const userStore = useUserStore()

interface UploadItem {
  id: string
  name: string
  size: number
  preview: string
  status: 'uploading' | 'done' | 'error'
  token?: string
  parse?: ParseResult
  engine?: string
  error?: string
}

const step = ref(0)
const files = ref<UploadItem[]>([])
const fileInput = ref<HTMLInputElement>()
const dragging = ref(false)
const submitting = ref(false)
const type = ref<'procurement' | 'travel'>('procurement')
const auto = ref(new Set<string>())

const form = reactive<any>({
  title: '', category: '', spec: '', link: '', qty: null, unit_price: null, shipping: 0,
  travel_time: '', transport: '', route: '', traveler: '', amount: null, note: '',
})
const meta = reactive<any>({ invoice_no: '', invoice_date: '', seller: '', buyer: '', total: null, tax: null, kind: '' })

const transports = ['飞机', '火车', '高铁', '网约车', '出租车', '公共交通', '汽车', '其他']
const categories = computed(() => userStore.categories.length ? userStore.categories : ['办公用品', '其他'])
const maxMb = computed(() => userStore.settings.max_upload_mb || 20)

// 小的置信度标签组件
const ConfTag = defineComponent({
  props: { conf: { type: Number, default: 0 } },
  setup(props) {
    return () => {
      if (!props.conf) return null
      const cls = props.conf >= 0.85 ? 'conf-tag' : props.conf >= 0.6 ? 'conf-tag mid' : 'conf-tag mid'
      const text = props.conf >= 0.85 ? '已识别' : '请核对'
      return h('span', { class: cls }, text)
    }
  },
})

const firstParse = computed(() => files.value.find((f) => f.parse)?.parse)
const hasUsableFile = computed(() => files.value.some((f) => f.token))

const field = (k: string) => {
  const f = firstParse.value?.fields?.[k]
  return f ? f.value : ''
}
const conf = (k: string) => firstParse.value?.fields?.[k]?.confidence || 0

const computedAmount = computed(() =>
  Math.round((Number(form.qty || 0) * Number(form.unit_price || 0) + Number(form.shipping || 0)) * 100) / 100,
)

// 没填数量/单价时直接以票面价税合计兜底；差几分钱也以票面为准
const alignedToTotal = computed(() => {
  if (type.value !== 'procurement') return false
  const total = Number(meta.total || 0)
  if (!total) return false
  const c = computedAmount.value
  return c <= 0 || Math.abs(c - total) <= 1
})

const amount = computed(() => {
  if (type.value === 'travel') return Number(form.amount || 0)
  return alignedToTotal.value ? Number(meta.total) : computedAmount.value
})

function pickFile() {
  fileInput.value?.click()
}

function onSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) addFiles(Array.from(input.files))
  input.value = ''
}

function onDrop(e: DragEvent) {
  dragging.value = false
  const items = Array.from(e.dataTransfer?.files || [])
  if (items.length) addFiles(items)
}

async function addFiles(list: File[]) {
  const room = 8 - files.value.length
  for (const file of list.slice(0, Math.max(0, room))) {
    if (file.size > maxMb.value * 1024 * 1024) {
      Message.error(`${file.name} 超过 ${maxMb.value}MB`)
      continue
    }
    const item: UploadItem = {
      id: Math.random().toString(36).slice(2),
      name: file.name,
      size: file.size,
      preview: file.type.startsWith('image/') ? URL.createObjectURL(file) : '',
      status: 'uploading',
    }
    files.value.push(item)
    // 取数组里的响应式代理，直接改原始对象不会触发视图更新
    const current = files.value[files.value.length - 1]
    try {
      const res = await api.upload(file)
      current.status = 'done'
      current.token = res.token
      current.parse = res.parse as ParseResult
      current.engine = res.parse?.engine
      if (files.value.filter((f) => f.parse).length === 1) prefill(res.parse)
    } catch (err: any) {
      current.status = 'error'
      current.error = err.message
      Message.error(`${file.name}：${err.message}`)
    }
  }
}

function prefill(parsed: ParseResult) {
  if (!parsed?.fields) return
  const s: any = parsed.suggest || {}
  type.value = s.type === 'travel' ? 'travel' : 'procurement'
  meta.invoice_no = parsed.fields.invoice_no?.value || ''
  meta.invoice_date = parsed.fields.invoice_date?.value || ''
  meta.seller = parsed.fields.seller?.value || ''
  meta.buyer = parsed.fields.buyer?.value || ''
  meta.total = parsed.fields.total?.value ?? null
  meta.tax = parsed.fields.tax?.value ?? null
  meta.kind = parsed.kind

  auto.value = new Set<string>()
  const put = (k: string, v: any) => {
    if (v === undefined || v === null || v === '') return
    form[k] = v
    auto.value.add(k)
  }
  if (type.value === 'travel') {
    put('title', s.title)
    put('travel_time', s.travel_time)
    put('transport', s.transport)
    put('route', s.route)
    put('traveler', s.traveler)
    put('amount', s.amount)
  } else {
    put('title', s.title)
    put('category', s.category)
    put('spec', s.spec)
    put('qty', s.qty)
    put('unit_price', s.unit_price)
    put('shipping', s.shipping ?? 0)
  }
  const noteParts = [meta.seller, meta.invoice_no ? `票号 ${meta.invoice_no}` : '', meta.invoice_date].filter(Boolean)
  if (noteParts.length && !form.note) {
    form.note = noteParts.join(' · ')
    auto.value.add('note')
  }
}

function removeFile(id: string) {
  const item = files.value.find((f) => f.id === id)
  if (item?.preview) URL.revokeObjectURL(item.preview)
  files.value = files.value.filter((f) => f.id !== id)
}

function onTypeChange() {
  // 切到出行时，用票面金额兜底
  if (type.value === 'travel' && !form.amount && meta.total) form.amount = meta.total
}

function toStep2() {
  if (!hasUsableFile.value) {
    Message.warning('请先上传至少一个可用的附件')
    return
  }
  if (!form.title && meta.seller) form.title = meta.seller
  step.value = 1
}

function toStep3() {
  if (!form.title) {
    Message.warning(type.value === 'travel' ? '请填写出行事由' : '请填写名称')
    return
  }
  if (type.value === 'travel' && !form.amount) {
    Message.warning('请填写税后金额')
    return
  }
  step.value = 2
}

async function submit() {
  submitting.value = true
  try {
    const payload: any = {
      type: type.value,
      title: form.title,
      note: form.note,
      amount: amount.value,
      meta: {
        invoice_no: meta.invoice_no,
        invoice_date: meta.invoice_date,
        seller: meta.seller,
        buyer: meta.buyer,
        total: meta.total,
        tax: meta.tax,
        invoice_kind: meta.kind,
      },
      ocr_engine: firstParse.value?.engine || '',
      ocr_confidence: firstParse.value?.confidence || 0,
    }
    if (type.value === 'travel') {
      Object.assign(payload, {
        travel_time: form.travel_time, transport: form.transport,
        route: form.route, traveler: form.traveler,
      })
    } else {
      Object.assign(payload, {
        category: form.category, spec: form.spec, link: form.link,
        qty: form.qty, unit_price: form.unit_price, shipping: form.shipping || 0,
      })
    }
    const uploads = files.value.filter((f) => f.token).map((f) => f.token as string)
    await api.createInvoice(payload, uploads)
    Message.success('提交成功，已加入报销台账')
    files.value.forEach((f) => f.preview && URL.revokeObjectURL(f.preview))
    router.push('/invoices')
  } catch (e: any) {
    Message.error(e.message)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.auto-mark {
  display: inline-block;
  margin-left: 6px;
  font-size: 10.5px;
  font-weight: 400;
  color: #4f46e5;
  background: #eef0ff;
  padding: 0 6px;
  border-radius: 5px;
  line-height: 16px;
}
</style>
