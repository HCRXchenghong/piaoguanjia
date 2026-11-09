import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/',
    component: () => import('@/layout/BasicLayout.vue'),
    children: [
      { path: '', name: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { title: '工作台' } },
      { path: 'upload', name: 'upload', component: () => import('@/views/Upload.vue'), meta: { title: '上传发票' } },
      { path: 'invoices', name: 'invoices', component: () => import('@/views/Invoices.vue'), meta: { title: '票据' } },
      { path: 'members', name: 'members', component: () => import('@/views/Members.vue'), meta: { title: '成员管理', admin: true } },
      { path: 'export', name: 'export', component: () => import('@/views/Export.vue'), meta: { title: '导出中心', admin: true } },
      { path: 'settings', name: 'settings', component: () => import('@/views/Settings.vue'), meta: { title: '系统设置', admin: true } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  const store = useUserStore()
  if (to.meta.public) {
    if (store.token && to.name === 'login') return { path: '/' }
    return true
  }
  if (!store.token) return { path: '/login', query: { redirect: to.fullPath } }
  if (!store.user) {
    try {
      await store.fetchMe()
    } catch {
      store.clear()
      return { path: '/login' }
    }
  }
  if (to.meta.admin && store.user?.role !== 'admin') return { path: '/' }
  if (to.meta.title) document.title = `${to.meta.title} · 票管家`
  return true
})

export default router
