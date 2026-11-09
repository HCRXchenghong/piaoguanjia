import { defineStore } from 'pinia'
import { api, TOKEN_KEY, type User } from '@/api'

interface State {
  token: string
  user: User | null
  settings: Record<string, any>
}

export const useUserStore = defineStore('user', {
  state: (): State => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: null,
    settings: {},
  }),
  getters: {
    isAdmin: (s) => s.user?.role === 'admin',
    displayName: (s) => s.user?.display_name || '',
    orgName: (s) => s.settings.org_name || '我的团队',
    categories: (s): string[] => s.settings.categories || [],
  },
  actions: {
    setSession(token: string, user: User, settings: any) {
      this.token = token
      this.user = user
      this.settings = settings || {}
      if (token) localStorage.setItem(TOKEN_KEY, token)
    },
    async login(username: string, password: string) {
      const data = await api.login(username, password)
      this.setSession(data.token, data.user, data.settings)
      return data
    },
    async setup(payload: any) {
      const data = await api.setup(payload)
      this.setSession(data.token, data.user, data.settings)
      return data
    },
    async fetchMe() {
      const data = await api.me()
      this.user = data.user
      this.settings = data.settings || {}
      if (!this.token) this.token = localStorage.getItem(TOKEN_KEY) || ''
      return data
    },
    async fetchSettings() {
      const data = await api.settings()
      this.settings = data.settings || {}
      return data
    },
    async logout() {
      try {
        await api.logout()
      } catch {
        /* ignore */
      }
      this.clear()
    },
    clear() {
      this.token = ''
      this.user = null
      localStorage.removeItem(TOKEN_KEY)
    },
  },
})
