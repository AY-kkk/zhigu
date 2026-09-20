export const MSG = {
  USER_CLAIM: 'user_claim',
  CONFIRM: 'confirm',
  STATUS: 'status',
  REPORT: 'report',
  QUESTION: 'question',
  ANSWER: 'answer',
  NOTICE: 'notice'
}

function textOf(value) {
  if (!value) return ''
  if (typeof value === 'string') return value
  if (Array.isArray(value)) {
    return value.map((part) => (typeof part === 'string' ? part : part?.text || '')).join('')
  }
  return String(value)
}

export function messageKind(message) {
  return message?.kind || (message?.role === 'user' ? MSG.USER_CLAIM : MSG.NOTICE)
}

export function buildChats(state) {
  const chats = []
  if (state.userMessage) {
    chats.push({
      id: state.userMessage.id,
      role: 'user',
      kind: MSG.USER_CLAIM,
      content: state.userMessage.text,
      createAt: state.userMessage.createAt,
      status: 'complete'
    })
  }
  if (state.confirmMessage && state.draft) {
    chats.push({
      id: state.confirmMessage.id,
      role: 'assistant',
      kind: MSG.CONFIRM,
      content: '请确认研究范围后再开始。',
      createAt: state.confirmMessage.createAt,
      status: 'complete'
    })
  }
  if (state.notice) {
    chats.push({
      id: state.notice.id,
      role: 'assistant',
      kind: MSG.NOTICE,
      content: state.notice.text,
      createAt: state.notice.createAt,
      status: 'complete'
    })
  }
  if (state.currentRunId && state.runView) {
    chats.push({
      id: `status:${state.currentRunId}`,
      role: 'assistant',
      kind: MSG.STATUS,
      content: state.runView.status || 'queued',
      createAt: Date.parse(state.runView.updated_at) || Date.now(),
      status: 'complete'
    })
  }
  if (state.runView?.report) {
    chats.push({
      id: `report:${state.currentRunId}:${state.runView.report.version || 1}`,
      role: 'assistant',
      kind: MSG.REPORT,
      content: state.runView.report.summary || '研究报告',
      createAt: Date.parse(state.runView.updated_at) || Date.now(),
      status: 'complete'
    })
  }
  for (const item of state.followups || []) {
    chats.push({
      id: item.questionId,
      role: 'user',
      kind: MSG.QUESTION,
      content: item.text,
      createAt: item.createAt,
      status: 'complete'
    })
    if (item.answer) {
      chats.push({
        id: item.answerId,
        role: 'assistant',
        kind: MSG.ANSWER,
        content: item.answer.answer || '',
        createAt: item.createAt + 1,
        status: 'complete'
      })
    }
  }
  return chats
}

export function copyReportText({ report, claim, instrumentId, horizon }) {
  if (!report) return ''
  const lines = []
  lines.push('知股研究报告')
  if (claim?.text) lines.push(`原始观点：${claim.text}`)
  lines.push(`标的：${instrumentId || '未标注'}；期限：${horizon || claim?.horizon || '未标注'}`)
  lines.push(`数据截止时间：${report.as_of || '暂未提供'}`)
  lines.push(`数据记录：${report.mode === 'live' ? '真实数据模式' : '离线样本'}`)
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
  lines.push('复制内容保留来源、数据截止时间与必要限制，不构成投资建议。')
  return lines.join('\n')
}

export { textOf }
