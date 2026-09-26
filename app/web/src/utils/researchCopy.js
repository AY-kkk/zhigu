export const STATUS_LABELS = {
  queued: '排队中',
  researching: '调查中',
  verifying: '核对中',
  completed: '完成',
  incomplete: '研究未完成',
  failed: '失败',
  canceling: '取消中',
  canceled: '已取消'
}

export const STATUS_MESSAGES = {
  queued: '正在排队等待研究',
  researching: '正在调查支持与反证',
  verifying: '正在核对证据与结论',
  completed: '研究已完成',
  incomplete: '研究未完成',
  failed: '研究失败',
  canceling: '正在取消研究',
  canceled: '研究已取消'
}

export const VERDICT_LABELS = {
  supported: '得到支持',
  challenged: '受到挑战',
  mixed: '证据混合',
  insufficient: '证据不足'
}

export const CLAIM_TYPE_LABELS = {
  fact: '事实',
  inference: '推断',
  assumption: '假设'
}

export const SOURCE_KIND_LABELS = {
  filing: '公告/披露',
  financials: '财务数据',
  market: '市场数据',
  news: '新闻',
  fixture: '离线样本'
}

export const CLAIM_EXAMPLES = [
  {
    key: 'fact',
    label: '核对事实依据',
    text: '演示公司的收入增长能否支持未来一年股价上涨？请核对该事实与推断。'
  },
  {
    key: 'challenge',
    label: '寻找关键反证',
    text: '演示公司经营现金流持续下降，这是否足以否定收入增长支持股价的判断？请寻找关键反证。'
  },
  {
    key: 'assumption',
    label: '检查推理假设',
    text: '若演示公司未来一年收入增长主要依赖单一客户，这一假设是否足以支撑股价上涨的推断？'
  }
]

export const LIVE_CLAIM_EXAMPLES = [
  {
    key: 'fact',
    label: '核对事实依据',
    text: '贵州茅台利润改善，未来一年经营前景是否值得看好？请核对该事实与推断。'
  },
  {
    key: 'challenge',
    label: '寻找关键反证',
    text: '腾讯控股经营现金流能否支撑其利润质量？请寻找已披露年报中的反证。'
  },
  {
    key: 'assumption',
    label: '检查推理假设',
    text: '若宁德时代未来一年收入增长主要依赖单一客户，这一假设是否足以支撑前景看好的推断？'
  }
]

export function claimExamples(mode) {
  return mode === 'live' ? LIVE_CLAIM_EXAMPLES : CLAIM_EXAMPLES
}

export const ACTIVE_STATUSES = ['queued', 'researching', 'verifying', 'canceling']
export const CANCELABLE_STATUSES = ['queued', 'researching', 'verifying']

export function countChars(text) {
  return Array.from(text || '').length
}

export function modeLabel(mode) {
  if (mode === 'fixture') return '离线样本'
  if (mode === 'live') return '真实数据模式'
  return ''
}

export function modeNotice(mode) {
  if (mode === 'fixture') return '当前为离线样本演示，内容不代表实时市场数据。'
  if (mode === 'live') return '当前为真实数据模式。'
  return '数据模式待确认。'
}

export function sourceKindLabel(kind) {
  if (!kind) return '来源类型暂未提供'
  return SOURCE_KIND_LABELS[kind] || '来源类型暂未提供'
}

export function formatDateTime(value) {
  if (!value) return '暂未提供'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '暂未提供'
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }).formatToParts(date)
  const get = (type) => parts.find((part) => part.type === type)?.value || ''
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}（北京时间）`
}

export function formatDate(value) {
  if (value === 0) return '0'
  if (value == null || value === '') return '暂未提供'
  const raw = String(value)
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(raw)
  if (match) return `${match[1]}-${match[2]}-${match[3]}`
  return emptyField(value)
}

export function emptyField(value) {
  if (value === 0) return '0'
  if (value === false) return '否'
  if (value == null || value === '') return '暂未提供'
  return String(value)
}

export function historyTitle(item) {
  const name = item?.instrument_name || item?.instrument_id
  if (!name) return '证券 ID · 观点研究'
  return `${name} · 观点研究`
}

export function statusLabel(status) {
  return STATUS_LABELS[status] || '状态暂不可用'
}

export function mapRequestError(error, fallback = '连接暂时中断，内容已保留。') {
  const code = error?.code
  const status = error?.status
  if (code === 'CONFIG_NOT_READY') return '研究服务暂未配置完成，请稍后再试。'
  if (code === 'ACTIVE_RUN_EXISTS') return '已有研究进行中'
  if (code === 'QUESTION_LIMIT') return '已达到每份报告 3 轮追问上限，请发起重新研究'
  if (code === 'UNSUPPORTED_INSTRUMENT') return '标的不在当前数据源覆盖范围，请修改观点后重试'
  if (status === 401 || code === 'UNAUTHENTICATED') return '登录状态已失效，请重新登录。'
  if (status === 403 || status === 404) return '该研究无法访问。'
  if (code === 'INTERNAL' || status >= 500) return '服务暂时不可用，请稍后重试。'
  if (code === 'NETWORK' || code === 'TIMEOUT' || code === 'ERR_NETWORK' || code === 'ECONNABORTED' || !status) {
    if (typeof error?.message === 'string' && /status code/i.test(error.message)) {
      return '服务暂时不可用，请稍后重试。'
    }
    return fallback
  }
  if (typeof error?.message === 'string' && /status code|network error|timeout/i.test(error.message)) {
    return fallback
  }
  return error?.message || fallback
}

export function copyReportText({ report, claim, instrumentId, horizon }) {
  if (!report) return ''
  const lines = []
  lines.push('知股研究报告')
  if (claim?.text) lines.push(`原始观点：${claim.text}`)
  lines.push(`标的：${instrumentId || '未标注'}；期限：${horizon || claim?.horizon || '未标注'}`)
  lines.push(`数据截止时间：${report.as_of || '暂未提供'}`)
  if (report.mode === 'live') lines.push('数据记录：真实数据模式')
  else if (report.mode === 'fixture') lines.push('数据记录：离线样本')
  else lines.push('数据记录：暂未提供')
  if (report.version) lines.push(`报告版本：${report.version}`)
  if (report.quality_status === 'incomplete') lines.push('发布完整性：研究未完成，仅含已发布部分')
  else if (report.quality_status === 'completed') lines.push('发布完整性：已完成')
  if (report.verdict) {
    const map = { supported: '得到支持', challenged: '受到挑战', mixed: '证据混合', insufficient: '证据不足' }
    lines.push(`判断：${map[report.verdict] || report.verdict}`)
  }
  if (report.summary) lines.push(`摘要：${report.summary}`)
  if (report.support?.length) {
    lines.push('支持证据：')
    report.support.forEach((item, i) => lines.push(`${i + 1}. ${item.text}`))
  }
  if (report.challenge?.length) {
    lines.push('最强反证：')
    report.challenge.forEach((item, i) => lines.push(`${i + 1}. ${item.text}`))
  }
  if (report.assumptions?.length) {
    lines.push('关键假设：')
    report.assumptions.forEach((item) => lines.push(`- ${item}`))
  }
  if (report.change_conditions?.length) {
    lines.push('改变判断的条件：')
    report.change_conditions.forEach((item) => lines.push(`- ${item}`))
  }
  if (report.unknowns?.length) {
    lines.push('未知项与证据缺口：')
    report.unknowns.forEach((item) => lines.push(`- ${item}`))
  }
  if (report.evidence_ids?.length) {
    lines.push(`来源编号：${report.evidence_ids.map((_, i) => `[${i + 1}]`).join(' ')}`)
  }
  lines.push('复制内容不含未加载的原文或外链全文。复制内容保留来源、数据截止时间与必要限制，不构成投资建议。')
  return lines.join('\n')
}
