<template>
  <div class="page" v-if="stats">
    <div class="hero mb-16" v-if="!userStore.isAdmin">
      <div>
        <h2>今天有需要报销的发票吗？</h2>
        <p>上传图片或 PDF，系统会自动识别票面信息并帮你填好表单。</p>
      </div>
      <a-button size="large" class="hero-btn" @click="router.push('/upload')">
        <template #icon><icon-upload /></template>
        上传发票
      </a-button>
    </div>

    <a-grid :cols="{ xs: 1, sm: 2, lg: 4 }" :col-gap="16" :row-gap="16" class="mb-16">
      <a-grid-item v-for="card in cards" :key="card.label">
        <div class="stat-card">
          <div class="stat-icon" :style="{ background: card.bg, color: card.color }">
            <component :is="card.icon" />
          </div>
          <div class="stat-label">{{ card.label }}</div>
          <div class="stat-value">{{ card.value }}<small v-if="card.suffix">{{ card.suffix }}</small></div>
          <div class="stat-foot">{{ card.foot }}</div>
        </div>
      </a-grid-item>
    </a-grid>

    <a-grid :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16" class="mb-16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>月度{{ userStore.isAdmin ? '支出' : '报销' }}</h3>
            <span class="sub">{{ year }} 年</span>
            <div class="spacer"></div>
            <a-select v-model="year" size="small" style="width: 104px" @change="load">
              <a-option v-for="y in years" :key="y" :value="String(y)">{{ y }} 年</a-option>
            </a-select>
          </div>
          <div class="panel-body">
            <div ref="barRef" style="height: 240px"></div>
          </div>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel">
          <div class="panel-head">
            <h3>类别分布</h3>
            <span class="sub">{{ year }} 年</span>
          </div>
          <div class="panel-body">
            <div v-if="!stats.categories?.length" style="height: 240px; display: grid; place-items: center">
              <a-empty description="暂无分类数据" />
            </div>
            <div v-else ref="pieRef" style="height: 240px"></div>
          </div>
        </div>
      </a-grid-item>
    </a-grid>

    <a-grid :cols="{ xs: 1, lg: 3 }" :col-gap="16" :row-gap="16">
      <a-grid-item :span="{ xs: 1, lg: 2 }">
        <div class="panel">
          <div class="panel-head">
            <h3>最近上报</h3>
            <span class="sub">最新 6 条</span>
            <div class="spacer"></div>
            <a-button type="text" size="small" @click="router.push('/invoices')">
              全部<icon-right />
            </a-button>
          </div>
          <a-table
            :data="stats.recent"
            :pagination="false"
            row-key="id"
            :bordered="false"
            :loading="loading"
            @row-click="(record: any) => openDetail(record.id)"
          >
            <template #columns>
              <a-table-column title="票据" data-index="title">
                <template #cell="{ record }">
                  <div class="table-title">{{ record.title }}</div>
                  <div class="table-sub">{{ record.category || record.route || '未分类' }}</div>
                </template>
              </a-table-column>
              <a-table-column title="类型" data-index="type" :width="110">
                <template #cell="{ record }">
                  <a-tag :color="record.type === 'travel' ? 'green' : 'arcoblue'" size="small">
                    {{ record.type === 'travel' ? '出行' : '采购' }}
                  </a-tag>
                </template>
              </a-table-column>
              <a-table-column title="金额" data-index="amount" align="right" :width="120">
                <template #cell="{ record }">
                  <span class="money-cell">¥{{ money(record.amount) }}</span>
                </template>
              </a-table-column>
              <a-table-column title="状态" data-index="status" :width="110">
                <template #cell="{ record }">
                  <a-tag :color="record.status === 'reimbursed' ? 'green' : 'orange'" size="small">
                    {{ record.status === 'reimbursed' ? '已报销' : '未报销' }}
                  </a-tag>
                </template>
              </a-table-column>
              <a-table-column title="时间" :width="110">
                <template #cell="{ record }">
                  <span class="tiny muted">{{ relativeTime(record.created_at) }}</span>
                </template>
              </a-table-column>
            </template>
            <template #empty>
              <a-empty description="还没有上报记录">
                <a-button type="primary" @click="router.push('/upload')">上传第一张发票</a-button>
              </a-empty>
            </template>
          </a-table>
        </div>
      </a-grid-item>

      <a-grid-item :span="{ xs: 1, lg: 1 }">
        <div class="panel" v-if="userStore.isAdmin">
          <div class="panel-head"><h3>成员报销</h3><span class="sub">{{ year }} 年</span></div>
          <a-table :data="stats.members" :pagination="false" row-key="user_id" :bordered="false" size="small">
            <template #columns>
              <a-table-column title="成员">
                <template #cell="{ record }">
                  <div class="person">
                    <span class="avatar-chip">{{ initial(record.name) }}</span>
                    <div>
                      <div>{{ record.name }}</div>
                      <div class="tiny muted">{{ record.count }} 张</div>
                    </div>
                  </div>
                </template>
              </a-table-column>
              <a-table-column title="未报销" align="right" :width="96">
                <template #cell="{ record }">
                  <span style="color: #b45309">¥{{ money(record.pending) }}</span>
                </template>
              </a-table-column>
              <a-table-column title="合计" align="right" :width="100">
                <template #cell="{ record }">
                  <span class="money-cell">¥{{ money(record.amount) }}</span>
                </template>
              </a-table-column>
            </template>
          </a-table>
        </div>

        <div class="panel" v-else>
          <div class="panel-head"><h3>快捷操作</h3></div>
          <div class="panel-body" style="display: grid; gap: 10px">
            <div v-for="q in quickActions" :key="q.label" class="quick-card" @click="router.push(q.path)">
              <div class="quick-ico" :style="{ background: q.bg, color: q.color }">
                <component :is="q.icon" />
              </div>
              <div>
                <b>{{ q.label }}</b>
                <span>{{ q.desc }}</span>
              </div>
            </div>
          </div>
        </div>
      </a-grid-item>
    </a-grid>
  </div>

  <InvoiceDrawer v-model:visible="drawerVisible" :invoice-id="currentId" @changed="load" />
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { api } from '@/api'
import { useUserStore } from '@/stores/user'
import { initial, money, relativeTime } from '@/utils/format'
import InvoiceDrawer from '@/components/InvoiceDrawer.vue'

const router = useRouter()
const userStore = useUserStore()
const stats = ref<any>(null)
const loading = ref(false)
const year = ref(String(new Date().getFullYear()))
const years = computed(() => {
  const now = new Date().getFullYear()
  return Array.from({ length: 5 }, (_, i) => now - i)
})

const barRef = ref<HTMLElement>()
const pieRef = ref<HTMLElement>()
const barChart = shallowRef<echarts.ECharts>()
const pieChart = shallowRef<echarts.ECharts>()
const drawerVisible = ref(false)
const currentId = ref(0)
const memberCount = ref(0)

const cards = computed(() => {
  const t = stats.value?.totals || {}
  const month = new Date().getMonth() + 1
  const thisMonth = (stats.value?.months || []).find((m: any) => m.month === month)
  return [
    {
      label: userStore.isAdmin ? '年度总支出' : '本月支出',
      value: `¥${money(userStore.isAdmin ? t.amount : thisMonth?.amount || 0)}`,
      foot: userStore.isAdmin ? `共 ${t.count || 0} 张票据` : `${thisMonth?.count || 0} 张票据`,
      icon: 'IconBarChart', color: '#4f46e5', bg: '#eef0ff',
    },
    {
      label: '待报销', value: `¥${money(t.pending_amount)}`,
      foot: `${t.pending_count || 0} 张待处理`, icon: 'IconClockCircle', color: '#b45309', bg: '#fef5e5',
    },
    {
      label: '已报销', value: `¥${money(t.reimbursed_amount)}`,
      foot: `${t.reimbursed_count || 0} 张已完成`, icon: 'IconCheckCircle', color: '#047857', bg: '#e7f8f0',
    },
    {
      label: userStore.isAdmin ? '团队规模' : '本年票据',
      value: userStore.isAdmin ? String(memberCount.value) : String(t.count || 0),
      suffix: userStore.isAdmin ? '人' : '张',
      foot: userStore.isAdmin ? '按成员汇总' : `累计 ¥${money(t.amount)}`,
      icon: 'IconUserGroup', color: '#0369a1', bg: '#e0f2fe',
    },
  ]
})

const quickActions = [
  { label: '上传新发票', desc: '图片 / PDF 自动识别', path: '/upload', icon: 'IconUpload', bg: '#eef0ff', color: '#4f46e5' },
  { label: '待报销票据', desc: '查看还没报销的记录', path: '/invoices?status=pending', icon: 'IconClockCircle', bg: '#fef5e5', color: '#b45309' },
  { label: '我的全部票据', desc: '筛选、查看与修改状态', path: '/invoices', icon: 'IconFile', bg: '#e7f8f0', color: '#047857' },
]

async function load() {
  loading.value = true
  try {
    const params: any = { year: year.value }
    if (!userStore.isAdmin) params.scope = 'mine'
    stats.value = await api.stats(params)
    if (userStore.isAdmin) {
      try {
        const data = await api.users()
        memberCount.value = data.users.filter((u: any) => u.active).length
      } catch {
        memberCount.value = stats.value?.members?.length || 0
      }
    }
    await nextTick()
    renderCharts()
  } finally {
    loading.value = false
  }
}

function renderCharts() {
  const months = stats.value?.months || []
  const map = new Map<number, any>(months.map((m: any) => [m.month, m]))
  const values = Array.from({ length: 12 }, (_, i) => map.get(i + 1) || { amount: 0, count: 0 })
  const currentMonth = new Date().getMonth() + 1
  const isCurrentYear = String(new Date().getFullYear()) === year.value

  if (barRef.value) {
    barChart.value = barChart.value || echarts.init(barRef.value)
    barChart.value.setOption({
      grid: { left: 8, right: 12, top: 24, bottom: 4, containLabel: true },
      tooltip: {
        trigger: 'axis',
        formatter: (params: any) => {
          const p = params[0]
          const m = values[p.dataIndex]
          return `${p.name}<br/>¥${money(m.amount)} · ${m.count} 张`
        },
      },
      xAxis: {
        type: 'category',
        data: values.map((_, i) => `${i + 1}月`),
        axisLine: { lineStyle: { color: '#e6e8f0' } },
        axisTick: { show: false },
        axisLabel: { color: '#98a2b3', fontSize: 11 },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#f0f1f7' } },
        axisLabel: { color: '#98a2b3', fontSize: 11 },
      },
      series: [
        {
          type: 'bar',
          barMaxWidth: 26,
          data: values.map((v, i) => ({
            value: v.amount,
            itemStyle: {
              borderRadius: [6, 6, 2, 2],
              color: isCurrentYear && i + 1 === currentMonth
                ? new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                    { offset: 0, color: '#a78bfa' }, { offset: 1, color: '#7c3aed' },
                  ])
                : new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                    { offset: 0, color: '#818cf8' }, { offset: 1, color: '#4f46e5' },
                  ]),
            },
          })),
        },
      ],
    })
  }

  const categories = stats.value?.categories || []
  if (pieRef.value) {
    pieChart.value = pieChart.value || echarts.init(pieRef.value)
    pieChart.value.setOption({
      tooltip: {
        trigger: 'item',
        formatter: (p: any) => `${p.name}<br/>¥${money(p.value)} (${p.percent}%)`,
      },
      legend: {
        bottom: 0,
        icon: 'circle',
        itemWidth: 8,
        itemHeight: 8,
        textStyle: { color: '#667085', fontSize: 11 },
      },
      series: [
        {
          type: 'pie',
          radius: ['52%', '72%'],
          center: ['50%', '44%'],
          avoidLabelOverlap: true,
          itemStyle: { borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          data: categories.map((c: any) => ({ name: c.name, value: c.amount })),
          color: ['#4f46e5', '#0ea5e9', '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#14b8a6', '#94a3b8'],
        },
      ],
    })
  }
}

function openDetail(id: number) {
  currentId.value = id
  drawerVisible.value = true
}

function onResize() {
  barChart.value?.resize()
  pieChart.value?.resize()
}

onMounted(() => {
  load()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  barChart.value?.dispose()
  pieChart.value?.dispose()
})
</script>

<style scoped>
.hero-btn {
  background: #fff !important;
  color: #312e81 !important;
  border: 0 !important;
  box-shadow: 0 8px 20px -10px rgba(0, 0, 0, 0.55);
}
</style>