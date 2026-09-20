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
  researching: '正在调查相关证据',
  verifying: '正在核对证据与结论',
  completed: '研究已完成',
  incomplete: '研究未完成，仅展示后端已允许发布的内容',
  failed: '研究失败',
  canceling: '正在取消研究，以后端确认为准',
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

export const ACTIVE_STATUSES = ['queued', 'researching', 'verifying', 'canceling']

export function countChars(text) {
  return Array.from(text || '').length
}

export function modeLabel(mode) {
  if (mode === 'fixture') return '离线样本'
  if (mode === 'live') return '真实数据模式'
  return ''
}

export function formatDateTime(value) {
  if (!value) return '暂未提供'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '暂未提供'
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function emptyField(value) {
  if (value === 0) return '0'
  if (value === false) return '否'
  if (value == null || value === '') return '暂未提供'
  return String(value)
}

export function historyTitle(item) {
  const name = item?.instrument_name || item?.instrument_id || '未标注标的'
  return `${name} · 观点研究`
}

export function statusLabel(status) {
  return STATUS_LABELS[status] || status || '未知状态'
}

export function mapRequestError(error, fallback = '网络中断，可重试') {
  const code = error?.code
  if (code === 'CONFIG_NOT_READY') return '研究服务暂未配置完成，请稍后再试'
  if (code === 'ACTIVE_RUN_EXISTS') return '已有研究进行中'
  if (code === 'QUESTION_LIMIT') return '已达到每份报告 3 轮追问上限，请发起重新研究'
  if (code === 'UNSUPPORTED_INSTRUMENT') return '标的不在当前数据源覆盖范围，请修改观点后重试'
  if (error?.status === 404) return '该研究无法访问'
  if (!code && (error?.status >= 500 || /status code/i.test(error?.message || ''))) return fallback
  return error?.message || fallback
}
