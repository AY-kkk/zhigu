<template>
  <div class="intel-page">
    <section v-if="workspace.demoNeedsStart" class="intel-section intel-demo-start">
      <p class="eyebrow">Isolated demo</p>
      <h2>开始演示前不会创建会话</h2>
      <p>点击“开始演示”后才创建独立 demo 空间。另一个浏览器打开此链接不会继承当前演示数据。</p>
      <button class="zg-btn zg-btn-primary" :disabled="workspace.bootBusy" @click="workspace.startDemo()">开始演示</button>
      <p v-if="workspace.bootError" class="intel-error">{{ workspace.bootError }}</p>
    </section>
    <div v-else class="intel-live-body">
    <section class="intel-hero">
      <div>
        <p class="eyebrow">事件主链路</p>
        <h2>谁最先说，事实怎么变，哪些说法冲突，现在是什么状态？</h2>
        <p class="intel-lead">把来源修订、证据分级、冲突与变更放在一条可核验的时间线上。当前模块只整理信息，不提供买卖建议、目标价或收益预测。</p>
      </div>
      <div class="intel-status-card">
        <span class="status-label">覆盖范围</span>
        <strong>{{ workspace.dataStatus?.coverage?.scope || 'events-v1' }}</strong>
        <span class="status-note">{{ workspace.dataStatus?.coverage?.status || 'complete' }} · {{ workspace.dataStatus?.coverage?.pending_count || 0 }} 待处理</span>
      </div>
    </section>

    <div v-if="workspace.bootError" class="intel-alert">{{ workspace.bootError }} <button type="button" class="zg-btn zg-btn-ghost" @click="workspace.bootstrap(mode)">重试</button></div>
    <div v-if="mode === 'demo'" class="intel-demo-bar">
      <div><strong>演示数据</strong><span>模拟时钟：{{ workspace.session?.simulated_at ? formatTime(workspace.session.simulated_at) : '尚未开始' }}</span></div>
      <div class="intel-demo-actions">
        <button class="zg-btn zg-btn-primary" :disabled="workspace.replay.busy" @click="workspace.replay('step')">下一步</button>
        <button class="zg-btn zg-btn-ghost" :disabled="workspace.replay.busy" @click="workspace.replay('reset')">重置</button>
      </div>
      <p v-if="workspace.replay.error" class="intel-error">{{ workspace.replay.error }}</p>
    </div>

    <section v-if="workspace.activeTab === 'watchlist'" class="intel-section">
      <div class="intel-section-head"><div><p class="eyebrow">Watchlist</p><h2>关注设置</h2></div><span class="intel-muted">最多 20 个标的</span></div>
      <div class="intel-search-row">
        <input v-model="workspace.instrumentQuery" type="search" placeholder="搜索代码或名称" aria-label="搜索标的" @keyup.enter="workspace.searchInstruments(workspace.instrumentQuery)" />
        <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.searchInstruments(workspace.instrumentQuery)">搜索</button>
      </div>
      <div class="intel-chip-grid">
        <div v-for="inst in workspace.instruments" :key="inst.code" class="intel-chip">
          <div><strong>{{ inst.code }}</strong><span>{{ inst.name }}</span></div>
          <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.addWatchlist(inst.code)">添加</button>
        </div>
      </div>
      <div class="intel-watchlist">
        <div v-for="item in workspace.watchlist" :key="item.code" class="intel-watch-row">
          <span><strong>{{ item.code }}</strong> {{ item.name }}</span>
          <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.removeWatchlist(item.code)">取消关注</button>
        </div>
        <p v-if="!workspace.watchlist.length" class="intel-empty">还没有关注标的。添加后，事件流只展示与你相关的事项。</p>
      </div>
    </section>

    <section v-else-if="workspace.activeTab === 'reviews'" class="intel-section">
      <div class="intel-section-head"><div><p class="eyebrow">Admin review</p><h2>待处理与纠错</h2></div><button class="zg-btn zg-btn-ghost" @click="workspace.fetchReviews()">刷新</button></div>
      <div v-if="workspace.admin.reviewsError" class="intel-alert">{{ workspace.admin.reviewsError }}</div>
      <div v-if="!workspace.admin.reviews.length" class="intel-empty">当前没有待处理项。纠错必须保留理由与审计，不能直接改写核实状态。</div>
      <div v-for="item in workspace.admin.reviews" :key="item.id" class="intel-change">
        <strong>{{ item.kind }} · {{ item.status }}</strong><span>revision {{ item.revision }}</span>
        <p>{{ item.reason || '等待管理员核验' }}</p>
        <div class="intel-notification-actions">
          <button class="zg-btn zg-btn-primary" :disabled="workspace.admin.busy" @click="workspace.resolveReview(item.id, 'assign', 'evt_demo_acq')">确认归属</button>
          <button class="zg-btn zg-btn-ghost" :disabled="workspace.admin.busy" @click="workspace.resolveReview(item.id, 'reject')">驳回</button>
        </div>
      </div>
    </section>

    <section v-else-if="workspace.activeTab === 'sources'" class="intel-section">
      <div class="intel-section-head"><div><p class="eyebrow">Provider access</p><h2>数据接入与人工材料</h2></div><span class="intel-muted">fail-closed</span></div>
      <div class="intel-chip-grid">
        <div v-for="provider in workspace.dataStatus?.providers || []" :key="provider.provider" class="intel-chip">
          <div><strong>{{ provider.provider }}</strong><span>{{ provider.status }} · {{ provider.rights }}</span></div>
          <small v-if="provider.reason">{{ provider.reason }}</small>
        </div>
      </div>
      <div class="intel-form-grid">
        <label>Provider <select v-model="workspace.admin.jobForm.provider"><option value="fixture">fixture</option><option value="ifind">ifind</option><option value="cninfo">cninfo</option><option value="fuyao">fuyao</option></select></label>
        <label>Codes <input v-model="workspace.admin.jobForm.codes" placeholder="DEMO.A,DEMO.B" /></label>
        <label>From <input v-model="workspace.admin.jobForm.from" type="date" /></label>
        <label>To <input v-model="workspace.admin.jobForm.to" type="date" /></label>
        <button class="zg-btn zg-btn-primary" :disabled="workspace.admin.busy" @click="workspace.createIngestion({ provider: workspace.admin.jobForm.provider, codes: workspace.admin.jobForm.codes.split(',').map((x) => x.trim()).filter(Boolean), from: workspace.admin.jobForm.from, to: workspace.admin.jobForm.to })">创建抓取任务</button>
      </div>
      <div class="intel-form-grid">
        <label>来源标题 <input v-model="workspace.admin.sourceForm.title" /></label>
        <label>文档 ID <input v-model="workspace.admin.sourceForm.document_id" /></label>
        <label>原文片段 <textarea v-model="workspace.admin.sourceForm.text" rows="4"></textarea></label>
        <label>导入理由 <input v-model="workspace.admin.sourceForm.import_reason" /></label>
        <button class="zg-btn zg-btn-primary" :disabled="workspace.admin.busy" @click="workspace.importSource(workspace.admin.sourceForm)">导入来源修订</button>
      </div>
      <p v-if="workspace.admin.importError" class="intel-error">{{ workspace.admin.importError }}</p>
      <div v-for="job in workspace.admin.jobs" :key="job.job_id" class="intel-change"><strong>{{ job.job_id }}</strong><span>{{ job.status }}</span></div>
    </section>

    <section v-else-if="workspace.activeTab === 'notifications'" class="intel-section">
      <div class="intel-section-head"><div><p class="eyebrow">Change feed</p><h2>通知中心</h2></div><span class="intel-muted">{{ workspace.unreadCount }} 条未读</span></div>
      <div v-if="workspace.notificationsError" class="intel-alert">{{ workspace.notificationsError }}</div>
      <div v-for="item in workspace.notifications" :key="item.id" class="intel-notification">
        <div><strong>{{ item.title || '事件情报更新' }}</strong><span class="intel-muted">{{ formatTime(item.created_at) }}</span></div>
        <p>{{ item.reason }}</p>
        <div class="intel-notification-actions">
          <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.patchNotification(item.id, 'read')">标记已读</button>
          <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.patchNotification(item.id, 'ignored')">忽略</button>
          <button class="zg-btn zg-btn-ghost" type="button" @click="workspace.selectEvent(item.event_id)">定位事件</button>
        </div>
      </div>
      <p v-if="!workspace.notifications.length" class="intel-empty">暂无变化通知。新变化会保留前后值与冻结版本。</p>
    </section>

    <section v-else-if="workspace.activeTab === 'replay' && mode === 'demo'" class="intel-section">
      <IntelReplayPanel :busy="workspace.replay.busy" :error="workspace.replay.error" @step="workspace.replay('step')" @reset="workspace.replay('reset')" />
    </section>

    <div v-else class="intel-live-body">
      <section class="intel-section intel-event-section">
        <div class="intel-section-head"><div><p class="eyebrow">Event stream</p><h2>事件流</h2></div><span class="intel-muted">按最近变化倒序</span></div>
        <div class="intel-filter-row">
          <select v-model="workspace.eventFilters.type" aria-label="事件类型" @change="workspace.fetchEvents">
            <option value="">全部类型</option><option value="acquisition">收购</option><option value="earnings_forecast">业绩预告</option><option value="regulatory_investigation">监管立案</option>
          </select>
          <select v-model="workspace.eventFilters.verification" aria-label="核实状态" @change="workspace.fetchEvents">
            <option value="">全部状态</option><option value="confirmed">已确认</option><option value="unverified">未核实</option><option value="denied">已否认</option><option value="disputed">有争议</option>
          </select>
          <select v-model="workspace.eventFilters.code" aria-label="标的" @change="workspace.fetchEvents">
            <option value="">全部标的</option><option v-for="item in workspace.watchlist" :key="item.code" :value="item.code">{{ item.code }}</option>
          </select>
        </div>
        <div v-if="workspace.eventsBusy" class="intel-empty">正在加载事件…</div>
        <div v-else-if="workspace.eventsError" class="intel-alert">{{ workspace.eventsError }} <button class="zg-btn zg-btn-ghost" @click="workspace.fetchEvents">重试</button></div>
        <div v-else-if="!workspace.events.length" class="intel-empty">当前关注范围内暂无事件。可先在“关注设置”添加标的，或加载演示样例。</div>
        <div v-else class="intel-event-grid">
          <IntelEventCard v-for="event in workspace.events" :key="event.event_id" :event="event" :selected="workspace.selectedEvent?.event_id === event.event_id" @select="workspace.selectEvent(event.event_id)" />
        </div>
      </section>

      <section v-if="workspace.selectedEvent" class="intel-detail-grid">
        <div class="intel-detail-main">
          <div class="intel-section-head"><div><p class="eyebrow">Current state</p><h2>{{ workspace.selectedEvent.title }}</h2></div><button class="zg-btn zg-btn-ghost" @click="workspace.setMute(workspace.selectedEvent.event_id, true)">静音事项</button></div>
          <div class="intel-state-grid">
            <div><span>核实状态</span><strong>{{ workspace.selectedEvent.verification }}</strong></div>
            <div><span>业务阶段</span><strong>{{ workspace.selectedEvent.phase }}</strong></div>
            <div><span>信息新鲜度</span><strong>{{ workspace.selectedEvent.freshness }}</strong></div>
            <div><span>支持度</span><strong>{{ workspace.selectedEvent.support_level }}</strong></div>
          </div>
          <p class="intel-core">{{ workspace.selectedEvent.core_claim }}</p>
          <div class="intel-summary"><strong>当前结论</strong><span>已知事实 + 最新变化 + 未决事项 + 标的关系 + 来源覆盖。关系只展示主体/交易对手，不推断产业链。</span></div>
          <IntelEvidenceList :items="workspace.evidence" />
          <section class="intel-subsection"><h3>时间线</h3><ol class="intel-timeline"><li v-for="node in workspace.timeline" :key="node.node_id"><strong>{{ node.title }}</strong><span>{{ formatTime(node.disclosed_at || node.recorded_at) }}</span><p>{{ node.excerpt }}</p></li></ol></section>
          <section class="intel-subsection"><h3>冲突对照</h3><p v-if="!workspace.conflicts.length" class="intel-empty">当前没有未决冲突。不同期间/口径不会直接判冲突。</p><div v-for="item in workspace.conflicts" :key="item.id" class="intel-conflict"><strong>{{ item.claim_key }}</strong><span>{{ item.status }}</span><small>证据：{{ (item.evidence_ids || []).join('、') }}</small></div></section>
          <section class="intel-subsection"><h3>历史版本与变化</h3><div v-for="item in workspace.changes" :key="item.change_id" class="intel-change"><strong>版本 {{ item.event_version }} · {{ item.kind }}</strong><span>{{ formatTime(item.recorded_at) }}</span><p>{{ item.summary }}</p></div></section>
        </div>
        <aside class="intel-side">
          <div class="intel-side-card"><strong>出处定位</strong><p>证据均绑定不可变 source_revision_id。原文不可访问时显示降级状态，不伪造定位。</p><button class="zg-btn zg-btn-ghost" @click="workspace.activeTab = 'notifications'">查看变化通知</button></div>
          <div class="intel-side-card"><strong>覆盖与处理</strong><p>{{ workspace.dataStatus?.coverage?.status || 'complete' }} · {{ workspace.dataStatus?.coverage?.scope || 'events-v1' }}</p><small>历史可读；故障时不会把“没有新消息”解释为业务结束。</small></div>
        </aside>
      </section>
    </div>
  </div>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import IntelEventCard from '../../components/intel/IntelEventCard.vue'
import IntelEvidenceList from '../../components/intel/IntelEvidenceList.vue'
import IntelReplayPanel from '../../components/intel/IntelReplayPanel.vue'
import { useIntelWorkspace } from '../../stores/intelWorkspace.js'
const route = useRoute()
const workspace = useIntelWorkspace()
const mode = computed(() => route.meta?.mode === 'demo' ? 'demo' : 'live')
function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '—' }
</script>
<style scoped>
.intel-page { display: grid; gap: 26px; }
.intel-hero { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(240px, .5fr); gap: 24px; align-items: end; }
.eyebrow { margin: 0 0 8px; color: #c94236; font-size: 11px; font-weight: 800; letter-spacing: .14em; text-transform: uppercase; }
.intel-hero h2 { max-width: 800px; margin: 0; font-family: Georgia, 'Songti SC', serif; font-size: clamp(26px, 4vw, 46px); line-height: 1.18; font-weight: 500; }
.intel-lead { max-width: 740px; margin: 16px 0 0; color: var(--zg-muted); line-height: 1.8; }
.intel-status-card { display: grid; gap: 7px; padding: 18px; border: 1px solid var(--zg-border); border-radius: 16px; background: #fffdfb; }
.status-label, .status-note { color: var(--zg-muted); font-size: 12px; }
.intel-status-card strong { font-size: 18px; }
.intel-section { padding: 22px; border: 1px solid var(--zg-border); border-radius: 18px; background: #fffdfb; }
.intel-section-head { display: flex; justify-content: space-between; align-items: end; gap: 12px; margin-bottom: 16px; }
.intel-section-head h2 { margin: 0; font-family: Georgia, 'Songti SC', serif; font-size: 26px; font-weight: 500; }
.intel-muted { color: var(--zg-muted); font-size: 12px; }
.intel-alert { display: flex; gap: 12px; align-items: center; padding: 12px 14px; border: 1px solid #f1c2bb; border-radius: 12px; background: #fff4f1; color: #a1372b; }
.intel-demo-bar { display: grid; grid-template-columns: 1fr auto; gap: 12px; padding: 15px 18px; border: 1px solid #efd5cf; border-radius: 14px; background: #fff8f5; }
.intel-demo-bar strong { margin-right: 12px; color: #b42318; }
.intel-demo-bar span { color: var(--zg-muted); font-size: 12px; }
.intel-demo-actions { display: flex; gap: 8px; }
.intel-error { grid-column: 1 / -1; margin: 0; color: #b42318; font-size: 12px; }
.intel-search-row, .intel-filter-row { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; }
.intel-search-row input, .intel-filter-row select { min-height: 40px; padding: 8px 11px; border: 1px solid var(--zg-border); border-radius: 10px; background: white; color: inherit; }
.intel-search-row input { flex: 1; min-width: 220px; }
.intel-chip-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 10px; margin-bottom: 18px; }
.intel-chip, .intel-watch-row { display: flex; justify-content: space-between; gap: 12px; align-items: center; padding: 12px; border: 1px solid var(--zg-border); border-radius: 12px; }
.intel-chip div, .intel-watch-row span { display: grid; gap: 3px; }
.intel-chip span { color: var(--zg-muted); font-size: 12px; }
.intel-watchlist { display: grid; gap: 8px; }
.intel-empty { padding: 18px; border: 1px dashed var(--zg-border); border-radius: 12px; color: var(--zg-muted); }
.intel-event-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; }
.intel-detail-grid { display: grid; grid-template-columns: minmax(0, 1fr) 280px; gap: 18px; align-items: start; }
.intel-detail-main, .intel-side-card { padding: 22px; border: 1px solid var(--zg-border); border-radius: 18px; background: #fffdfb; }
.intel-state-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; margin: 18px 0; }
.intel-state-grid div { display: grid; gap: 5px; padding: 12px; border-radius: 12px; background: var(--zg-soft); }
.intel-state-grid span { color: var(--zg-muted); font-size: 11px; }
.intel-state-grid strong { font-size: 15px; }
.intel-core { margin: 16px 0; padding: 15px 17px; border-left: 3px solid #c94236; background: #fff6f3; line-height: 1.75; }
.intel-summary { display: grid; gap: 6px; padding: 15px 0 21px; border-bottom: 1px solid var(--zg-border); color: var(--zg-muted); font-size: 13px; line-height: 1.7; }
.intel-summary strong { color: var(--zg-ink); }
.intel-subsection { padding-top: 23px; }
.intel-subsection h3 { margin: 0 0 12px; font-family: Georgia, 'Songti SC', serif; font-size: 21px; font-weight: 500; }
.intel-timeline { display: grid; gap: 11px; padding: 0 0 0 20px; margin: 0; }
.intel-timeline li { padding: 12px 14px; border-left: 1px solid #e7b4ab; }
.intel-timeline span { margin-left: 8px; color: var(--zg-muted); font-size: 11px; }
.intel-timeline p { margin: 7px 0 0; color: var(--zg-muted); line-height: 1.6; }
.intel-conflict, .intel-change { display: grid; gap: 5px; padding: 13px; margin-bottom: 8px; border: 1px solid var(--zg-border); border-radius: 12px; }
.intel-conflict span, .intel-change span { color: var(--zg-muted); font-size: 11px; }
.intel-conflict small, .intel-change p { margin: 0; color: var(--zg-muted); line-height: 1.6; }
.intel-side { display: grid; gap: 12px; }
.intel-side-card { display: grid; gap: 9px; }
.intel-side-card p, .intel-side-card small { margin: 0; color: var(--zg-muted); line-height: 1.7; font-size: 12px; }
.intel-notification { display: grid; gap: 7px; padding: 15px 0; border-bottom: 1px solid var(--zg-border); }
.intel-notification > div { display: flex; justify-content: space-between; gap: 10px; }
.intel-notification p { margin: 0; color: var(--zg-muted); line-height: 1.6; }
.intel-notification-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.intel-form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; align-items: end; margin: 14px 0; }
.intel-form-grid label { display: grid; gap: 5px; color: var(--zg-muted); font-size: 12px; }
.intel-form-grid input, .intel-form-grid select, .intel-form-grid textarea { width: 100%; padding: 9px 10px; border: 1px solid var(--zg-border); border-radius: 9px; background: white; color: inherit; font: inherit; }
@media (max-width: 900px) { .intel-detail-grid { grid-template-columns: 1fr; } .intel-side { grid-template-columns: repeat(2, 1fr); } }
@media (max-width: 767px) {
  .intel-hero { grid-template-columns: 1fr; }
  .intel-state-grid { grid-template-columns: repeat(2, 1fr); }
  .intel-demo-bar { grid-template-columns: 1fr; }
  .intel-side { grid-template-columns: 1fr; }
}
</style>
