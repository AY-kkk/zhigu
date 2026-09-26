export function timeKey(t) {
  if (t == null) return ''
  if (typeof t === 'string') return t.slice(0, 10)
  if (typeof t === 'number' && Number.isFinite(t)) {
    const d = new Date(t * 1000)
    if (Number.isNaN(d.getTime())) return String(t)
    const y = d.getUTCFullYear()
    const m = String(d.getUTCMonth() + 1).padStart(2, '0')
    const day = String(d.getUTCDate()).padStart(2, '0')
    return `${y}-${m}-${day}`
  }
  if (typeof t === 'object' && t.year) {
    return `${t.year}-${String(t.month).padStart(2, '0')}-${String(t.day).padStart(2, '0')}`
  }
  return String(t)
}
