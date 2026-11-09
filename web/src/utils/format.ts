export function money(v: number | string | null | undefined, digits = 2): string {
  const n = Number(v ?? 0)
  if (Number.isNaN(n)) return '0.00'
  return n.toLocaleString('zh-CN', { minimumFractionDigits: digits, maximumFractionDigits: digits })
}

export function moneyText(v: number | string | null | undefined): string {
  return `¥${money(v)}`
}

export function shortDate(s?: string): string {
  if (!s) return '—'
  return s.slice(0, 10)
}

export function dateTime(s?: string): string {
  if (!s) return '—'
  return s.slice(0, 16)
}

export function relativeTime(s?: string): string {
  if (!s) return ''
  const t = new Date(s.replace(' ', 'T')).getTime()
  if (Number.isNaN(t)) return s
  const diff = (Date.now() - t) / 1000
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`
  return s.slice(0, 10)
}

export function fileSize(bytes: number): string {
  const n = Number(bytes || 0)
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function statusText(s: string): string {
  return s === 'reimbursed' ? '已报销' : '未报销'
}

export function typeText(t: string): string {
  return t === 'travel' ? '出行报销' : '采购报销'
}

export function engineText(e?: string): string {
  const map: Record<string, string> = {
    'pdf-text': 'PDF 文字层',
    'windows-ocr': '本地 OCR',
    'pdf-image-ocr': '扫描件 OCR',
    none: '未识别',
  }
  return map[e || ''] || e || '未识别'
}

export function initial(name?: string): string {
  return (name || '?').trim().slice(0, 1).toUpperCase()
}

export function currentMonth(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

export function percent(v: number, total: number): string {
  if (!total) return '0%'
  return `${Math.round((v / total) * 100)}%`
}
