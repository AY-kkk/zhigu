<template>
  <div class="pane">
    <Chat
      class="chat"
      :chats="conversation.chats"
      role="assistant"
      mode="userBubble"
      align="leftRight"
      :enableUpload="false"
      :showStopGenerate="false"
      :showClearContext="false"
      :hints="[]"
      :roleConfig="roleConfig"
      :chatBoxRenderConfig="chatBoxRenderConfig"
      :renderInputArea="renderInputArea"
      :markdownRenderProps="markdownRenderProps"
      :customMarkDownComponents="markdownComponents"
      :topSlot="topSlot"
    />
  </div>
</template>
<script setup>
import { computed, h } from 'vue'
import { useRouter } from 'vue-router'
import { Button, Chat, Toast, TypographyParagraph, TypographyTitle } from '@kousum/semi-ui-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { CLAIM_EXAMPLES } from '../../utils/researchCopy.js'
import { MSG, copyReportText, messageKind } from './chatAdapter.js'
import ResearchComposer from './ResearchComposer.vue'
import ResearchConfirmCard from './ResearchConfirmCard.vue'
import ResearchStatus from './ResearchStatus.vue'
import Report from './Report.vue'
import SafeSourceLink from './SafeSourceLink.vue'

const router = useRouter()
const conversation = useResearchConversation()
const roleConfig = {
  user: { name: '我' },
  assistant: { name: '知股' }
}
const markdownRenderProps = { format: 'md' }
const markdownComponents = {
  a: SafeSourceLink,
  img: () => null,
  html: () => null
}

const composerMeta = computed(() => {
  const mode = conversation.composerMode
  if (mode === 'busy') {
    return {
      disabled: true,
      label: '解析观点',
      placeholder: '正在研究，完成后可基于证据追问',
      hint: '正在研究，完成后可基于证据追问',
      error: '',
      busy: false
    }
  }
  if (mode === 'followup') {
    return {
      disabled: false,
      label: '发送追问',
      placeholder: '基于这份报告追问',
      hint: '仅使用本报告证据，不进行新检索。每份报告最多 3 轮。追问记录仅保留在本次会话中，刷新后不会完整回放。',
      error: conversation.questionError,
      busy: conversation.questionBusy
    }
  }
  if (mode === 'followup_limit') {
    return {
      disabled: true,
      label: '发送追问',
      placeholder: '已达到每份报告 3 轮追问上限',
      hint: '已达到每份报告 3 轮追问上限。如需新事实，请发起重新研究。',
      error: conversation.questionError,
      busy: false
    }
  }
  if (mode === 'locked') {
    return {
      disabled: true,
      label: '解析观点',
      placeholder: '当前研究没有可追问的完整报告',
      hint: conversation.runError || '当前研究没有可追问的完整报告。',
      error: conversation.runError,
      busy: false
    }
  }
  return {
    disabled: false,
    label: '解析观点',
    placeholder: '粘贴一个关于单家公司的投资观点，20–2,000 字',
    hint: '',
    error: conversation.parseError,
    busy: conversation.parseBusy
  }
})

const topSlot = computed(() => {
  if (conversation.chats.length) return null
  return h('div', { class: 'welcome zhigu-reading' }, [
    h(TypographyTitle, { heading: 2 }, { default: () => '让投资观点，经得起验证' }),
    h(TypographyParagraph, {}, { default: () => '粘贴一段观点，一起查看它的依据、反证与未知。' }),
    h('div', { class: 'examples' }, CLAIM_EXAMPLES.map((item) => h(Button, {
      key: item.key,
      type: 'tertiary',
      onClick: () => conversation.fillExample(item.text)
    }, { default: () => item.label })))
  ])
})

const chatBoxRenderConfig = {
  renderChatBoxAvatar: () => null,
  renderChatBoxTitle: () => null,
  renderChatBoxContent: (props) => {
    const message = props.message || {}
    const kind = messageKind(message)
    if (kind === MSG.CONFIRM) {
      return h(ResearchConfirmCard, {
        onConfirmed: (id) => router.push(`/app/research/${id}`),
        onEditClaim: () => {}
      })
    }
    if (kind === MSG.STATUS) return h(ResearchStatus)
    if (kind === MSG.REPORT) {
      return h(Report, {
        report: conversation.runView.report,
        claim: conversation.runView.claim,
        instrumentId: conversation.runView.instrument_id,
        horizon: conversation.runView.horizon,
        onOpenEvidence: (id, el) => conversation.openEvidence(id, el)
      })
    }
    if (kind === MSG.ANSWER) {
      const item = conversation.followups.find((row) => row.answerId === message.id)
      const allowed = conversation.runView?.report?.evidence_ids || []
      return h('div', { class: 'answer' }, [
        h('p', {}, item?.answer?.answer || message.content),
        (item?.answer?.limitations || []).map((line) => h('p', { class: 'limit', key: line }, line)),
        h('p', { class: 'limit' }, '仅使用本报告证据，不进行新检索。'),
        (item?.answer?.evidence_ids || []).filter((id) => allowed.includes(id)).map((id) => h('span', { key: id }, ''))
      ])
    }
    if (kind === MSG.NOTICE) {
      return h('p', { class: 'notice' }, message.content)
    }
    return props.defaultContent
  },
  renderChatBoxAction: (props) => {
    const message = props.message || {}
    const kind = messageKind(message)
    const nodes = []
    if (kind === MSG.REPORT && conversation.runView?.report) {
      nodes.push(h(Button, { size: 'small', type: 'tertiary', onClick: onCopyReport }, { default: () => '复制报告' }))
      nodes.push(h(Button, { size: 'small', type: 'tertiary', onClick: onReResearch }, { default: () => '重新研究' }))
    }
    if (kind === MSG.USER_CLAIM || kind === MSG.QUESTION) {
      nodes.push(h(Button, { size: 'small', type: 'tertiary', onClick: () => copyText(message.content) }, { default: () => '复制' }))
    }
    if (!nodes.length) return null
    return h('div', { class: props.className || 'actions' }, nodes)
  }
}

function renderInputArea() {
  const meta = composerMeta.value
  return h(ResearchComposer, {
    modelValue: conversation.draftText,
    'onUpdate:modelValue': (value) => conversation.setDraftText(value),
    placeholder: meta.placeholder,
    hint: meta.hint,
    error: meta.error,
    actionLabel: meta.label,
    disabled: meta.disabled,
    busy: meta.busy,
    onSubmit: onSubmit
  })
}

async function onSubmit() {
  const mode = conversation.composerMode
  if (mode === 'followup') {
    const text = conversation.draftText
    conversation.draftText = ''
    await conversation.askFollowup(text)
    return
  }
  if (mode === 'claim') await conversation.parseCurrent()
}

function onReResearch() {
  if (!conversation.currentRunId) return
  const parent = conversation.currentRunId
  conversation.resetAll()
  conversation.prepareNew({ parent_run_id: parent })
  router.push({ path: '/app/research/new', query: { parent_run_id: parent } })
}

async function onCopyReport() {
  const text = copyReportText({
    report: conversation.runView?.report,
    claim: conversation.runView?.claim,
    instrumentId: conversation.runView?.instrument_id,
    horizon: conversation.runView?.horizon
  })
  await copyText(text)
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text || '')
    Toast.success({ content: '已复制', duration: 2 })
  } catch {
    Toast.error({ content: '复制失败' })
  }
}
</script>
<style scoped>
.pane { flex: 1; min-height: 0; display: flex; }
.chat { flex: 1; min-height: 0; }
</style>
<style>
.welcome { padding: 48px 32px 24px; }
.welcome h2, .welcome .semi-typography-h2 { font-size: 28px; line-height: 38px; font-weight: 600; }
.examples { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
.notice, .limit { color: #716b70; font-size: 12px; line-height: 18px; }
.answer p { overflow-wrap: anywhere; }
</style>
