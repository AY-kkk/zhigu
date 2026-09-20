export function sanitizeHref(href) {
  if (!href || typeof href !== 'string') return ''
  const t = href.trim()
  if (!/^https?:\/\//i.test(t)) return ''
  return t
}

export function externalRel() {
  return 'noreferrer noopener'
}
