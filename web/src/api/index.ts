import axios from 'axios'

export const TOKEN_KEY = 'ticketbox_token'

export interface User {
  id: number
  username: string
  display_name: string
  role: 'admin' | 'member'
  dept: string
  active: boolean
  created_at: string
  last_login_at: string
  invoice_count?: number
  invoice_amount?: number
}

export interface InvoiceFile {
  id: number
  name: string
  mime: string
  size: number
  ext: string
}

export interface Invoice {
  id: number
  user_id: number
  display_name: string
  type: 'procurement' | 'travel'
  type_label: string
  status: 'pending' | 'reimbursed'
  status_label: string
  title: string
  category: string
  spec: string
  link: string
  qty: number | null
  unit_price: number | null
  shipping: number | null
  amount: number
  travel_time: string
  transport: string
  route: string
  traveler: string
  note: string
  invoice_no: string
  invoice_code: string
  invoice_date: string
  seller: string
  buyer: string
  tax: number | null
  total: number | null
  invoice_kind: string
  ocr_engine: string
  ocr_confidence: number
  created_at: string
  updated_at: string
  reimbursed_at: string
  files: InvoiceFile[]
  attachments?: string[]
}

export interface FieldValue {
  value: any
  confidence: number
  source: string
}

export interface ParseItem {
  name: string
  spec: string
  unit: string
  qty: number | null
  price: number | null
  amount: number | null
  rate: number | null
  tax: number | null
}

export interface ParseResult {
  kind: string
  kind_label: string
  suggest_type: string
  fields: Record<string, FieldValue>
  items: ParseItem[]
  travel: Record<string, any>
  suggest: Record<string, any>
  warnings: string[]
  raw_text: string
  engine: string
  pages: number
  elapsed_ms: number
  confidence: number
  filename: string
}

const http = axios.create({ baseURL: '', timeout: 180000 })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers = config.headers || {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (res) => res.data,
  (error) => {
    const status = error.response?.status
    const message = error.response?.data?.error || error.message || '网络错误'
    if (status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      if (!location.hash.startsWith('#/login')) location.hash = '#/login'
    }
    return Promise.reject(new Error(message))
  },
)

export const api = {
  bootstrap: () => http.get<any, any>('/api/bootstrap'),
  setup: (data: any) => http.post<any, any>('/api/setup', data),
  login: (username: string, password: string) => http.post<any, any>('/api/login', { username, password }),
  logout: () => http.post<any, any>('/api/logout', {}),
  me: () => http.get<any, any>('/api/me'),
  changePassword: (old_password: string, new_password: string) =>
    http.post<any, any>('/api/me/password', { old_password, new_password }),

  upload: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return http.post<any, any>('/api/uploads', form, { headers: { 'Content-Type': 'multipart/form-data' } })
  },
  createInvoice: (payload: any, uploads: string[]) => http.post<any, any>('/api/invoices', { payload, uploads }),
  invoices: (params: any) => http.get<any, any>('/api/invoices', { params }),
  invoice: (id: number) => http.get<any, any>(`/api/invoices/${id}`),
  updateInvoice: (id: number, data: any) => http.patch<any, any>(`/api/invoices/${id}`, data),
  deleteInvoice: (id: number) => http.delete<any, any>(`/api/invoices/${id}`),
  stats: (params: any) => http.get<any, any>('/api/stats', { params }),

  users: () => http.get<any, any>('/api/users'),
  createUser: (data: any) => http.post<any, any>('/api/users', data),
  updateUser: (id: number, data: any) => http.patch<any, any>(`/api/users/${id}`, data),
  resetPassword: (id: number, password: string) => http.post<any, any>(`/api/users/${id}/password`, { password }),
  deleteUser: (id: number) => http.delete<any, any>(`/api/users/${id}`),

  settings: () => http.get<any, any>('/api/settings'),
  saveSettings: (data: any) => http.put<any, any>('/api/settings', data),
  audit: (limit = 60) => http.get<any, any>('/api/admin/audit', { params: { limit } }),
  storage: () => http.get<any, any>('/api/admin/storage'),
  cleanup: () => http.post<any, any>('/api/admin/cleanup', {}),
}

export function exportUrl(params: Record<string, any>) {
  const token = localStorage.getItem(TOKEN_KEY) || ''
  const sp = new URLSearchParams()
  Object.entries(params || {}).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  })
  sp.set('token', token)
  return `/api/export?${sp.toString()}`
}

export function backupUrl() {
  const token = localStorage.getItem(TOKEN_KEY) || ''
  return `/api/admin/backup?token=${encodeURIComponent(token)}`
}

export function fileUrl(id: number) {
  return `/api/files/${id}`
}

export function thumbUrl(id: number) {
  return `/api/files/${id}/thumb`
}
