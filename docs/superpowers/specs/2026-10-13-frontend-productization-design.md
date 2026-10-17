# 知股前端产品化收尾设计

日期：2026-10-13
适用范围：`app/web`
依据：`spec/前端产品化_开发SPEC.md`

## 1. 目标

在现有 Vue 3 + Vue Router + Pinia 项目上完成前端产品化收尾，交付：

1. 以用户任务为中心的全局壳、导航、路由错误恢复和页面标题焦点。
2. 四维状态、来源/时间、错误、陈旧、空、关闭、只读、不可用和部分失败的真实状态表达。
3. 期货概览、品种、发起研究、研究详情、假设、通知和来源管理完整用户旅程。
4. 与 Go API/错误注册表/分页/CSRF/session/capability contract 对齐的 client 和 view model。
5. 320、390、1024、1440 视口下可用的响应式和键盘无障碍体验。
6. 可重复的构建、单测、组件测试、契约测试、E2E、视觉和性能证据。

## 2. 已确认的实施方式

采用增量产品化，不进行全站组件库迁移：

- 消费端继续使用 Semi UI Vue 和 `editorial-tokens.css`。
- 管理端暂留 Element Plus；同一页面不得混用两套基础控件。
- 不增加 CSS-in-JS、状态库或通用 UI 框架。
- 不修改后端 API 契约；发现契约缺口时由前端 adapter 归一化并记录失败。
- 不把 fixture、模型不可用、缓存旧数据或 mock 结果伪装成真实成功。

## 3. 架构边界

### 3.1 数据流

```text
route -> page view model -> Pinia resource store -> typed API client -> HTTP envelope
                                              -> normalized system state
```

页面只消费归一化 view model；公共组件不发请求、不读取全局业务 store。API client 负责 envelope、trace、retryable、分页和 CSRF；store 负责请求竞态、取消、缓存版本和页面生命周期。

### 3.2 四维状态

`deployment_environment`、`operation_mode`、`data_provenance`、`health` 分别展示，禁止用一个 `mode/status` 混合表达。生产环境不显示 fixture 标识；fixture 只在 `test/demo` 或显式测试命名空间出现。

### 3.3 路由与壳

保留现有路由，新增公共组件：

```text
app/web/src/components/workspace/
  AppShell.vue
  SideNav.vue
  WorkspaceHeader.vue
  PageHeader.vue
  RouteErrorBoundary.vue
app/web/src/components/system/
  CapabilityStatus.vue
  DataFreshness.vue
  ProvenanceSummary.vue
  RequestError.vue
  EmptyState.vue
  LoadingState.vue
  ModeBanner.vue
  ConfirmAction.vue
```

已有的 `SideNav.vue`、`WorkspaceHeader.vue`、品牌组件和研究组件优先扩展，不复制职责。

## 4. 页面交付

### 4.1 壳和导航

- 桌面端固定侧栏和轻量顶栏；移动端为可关闭抽屉。
- 一级导航显示研究、策略、情报、期货、历史；管理入口单独分组。
- Header 显示页面名、环境标记、能力异常和账号菜单。
- 每次路由切换将焦点移动到主标题，并提供 skip link。
- disabled 模块保留导航入口，但进入后展示原因和恢复动作。

### 4.2 期货旅程

1. `/app/futures`：首屏展示模式/阻断能力、数据新鲜度、最近失败、通知、关注品种、最近运行和活跃假设。
2. `/app/futures/products/:productId`：展示品种/交易所/代码/时区/截止时间，分离主力、连续和具体合约，提供行情、期限结构、库存/持仓、来源覆盖和研究入口。
3. `/app/futures/research/new`：单页三阶段问题材料、解析核对、范围确认；上传支持限制、进度、取消、部分失败和单文件重试；提交使用 idempotency key。
4. `/app/futures/research/:runId`：按服务端状态显示时间线、阶段、耗时、预算、取消/重试、报告、证据抽屉、导出和 `trace_id`。
5. `/app/futures/hypotheses/:id`：显示命题、方向、置信度、观察期、前提、反证、来源 run、生命周期、校验条件和复盘。
6. `/app/futures/notifications`：支持未读/已读/需处理筛选、定位上下文、batch-read 乐观更新和失败回滚。
7. `/admin/futures/sources`：来源结构化表单、权限、模式、健康、probe、release gate 和删除影响；secret 不回显。

### 4.3 既有模块回归

观点研究、策略/行情、事件情报和历史页继续使用现有业务组件。重点回归：

- 研究输入、材料上传、事实核验、证据抽屉、报告章节和历史筛选。
- 策略编辑实时验证、行情缺口/陈旧状态、策略市场来源/版本/证据。
- 事件来源/重要性/标的/时间筛选、影响路径、反证、回放和通知确认。

## 5. API、状态和错误

- 所有请求通过 `src/utils/http.js`/模块 client；保留 envelope `{data,error,trace_id}`。
- 浏览器 session 只使用 HttpOnly Cookie；CSRF token 只保存在内存。
- GET 重试有上限；副作用请求绝不自动重试。
- 竞态采用请求序号/AbortController，切换账号或路由后丢弃旧响应。
- 错误必须映射到稳定 code、retryable、retry delay、trace_id；网络错误不得误报凭证错误。
- 模型/来源 HTML 经过 sanitize，不将任意 HTML 注入 DOM。
- 外链走 `SafeSourceLink`，下载和 Object URL 在组件卸载时释放。

## 6. 视觉、响应式和无障碍

- 文字、颜色、边框、间距、阴影全部使用现有 token；红色/绿色只用于支持/反证等语义。
- 关键状态同时使用文本、图标或结构，不依赖单一颜色。
- 视口门槛：1440、1024、390、320；无页面级横向溢出。
- 报告、证据、通知和关键操作在 390 px 可用；移动端关键操作可吸附。
- 表单、上传、dialog、drawer、表格和分页可仅用键盘完成；焦点可见且顺序合理。
- 空状态解释原因，只有存在合法动作时显示 CTA；错误保留在页面中，不用 toast 替代。

## 7. 测试与证据

### 自动化

- Vite production build；路由 chunk 和 CSS/JS 预算检查。
- Vitest/组件测试：系统组件、状态归一化、错误映射、分页、通知 batch、表单和上传。
- 契约测试：capability 四维、envelope/trace/retry、分页 cursor、通知 filter/batch-read、来源 probe/gate 字段。
- Playwright：登录、研究、策略、情报、期货七条核心旅程、刷新恢复、取消、部分失败、404。
- axe、键盘、焦点、200%/400% 缩放和响应式快照。
- 视觉快照只覆盖已审核的稳定页面，不用 mock 截图证明真实后端。

### 外部验收

真实 provider/model、真实后端 E2E、生产部署、真实数据引用和生产无 fixture 标识在部署后执行。缺少真实条件时明确标记 `NOT_RUN/BLOCKED`，不得记为通过。

## 8. 阶段和文件责任

- FE1：合同冻结、API client、四维状态、错误/trace、公共系统组件。
- FE2：AppShell、SideNav、WorkspaceHeader、PageHeader、RouteErrorBoundary 和路由焦点。
- FE3：期货概览、品种、发起研究、研究详情。
- FE4：假设、通知、来源管理。
- FE5：观点/策略/情报回归和跨模块一致性。
- FE6：响应式、无障碍、性能、E2E、视觉和失败注入。
- FE7：生产构建、部署 smoke 和真实接入证据。

文件改动集中在 `app/web/src/components`、`app/web/src/modules/futures`、`app/web/src/api`、`app/web/src/stores`、`app/web/src/router`、`app/web/src/styles` 和 `app/web/e2e`。后端契约文件不修改；只读取现有契约生成校验样例。

## 9. 完成判定

只有以下条件全部满足才标记前端完成：

- FPR-01 至 FPR-43 有自动化结果或签字证据。
- 没有空白路由、占位按钮、静默失败、假成功或生产 fixture 回退。
- 所有页面覆盖加载、空、错误、关闭、只读、成功、陈旧和不可用状态。
- 观点、策略、情报、期货核心旅程无回归。
- 响应式、键盘、axe、性能和视觉证据达到 SPEC。
- 浏览器不持久化 session/JWT；错误、trace、retry、分页、通知 batch、probe 和 release gate 与机器契约一致。
- FPR-44 至 FPR-46 的真实接入证据完成前，不宣称个人正式使用。
