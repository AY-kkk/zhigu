# 知股 Zhigu · 前端开发 Implementation Spec

版本：1.0 · 2026-09-20  
状态：最终开发规格；供 AI Coding Agent 执行，不代表下述界面已实现或验收。  
技术基线：`6e1c310e7bbef987d88aa7bc7d2ddcad06f82a5e`。所有路径相对于仓库根目录。  
设计方向：**Editorial Research Design System / 编辑式投研工作台**。

> 执行摘要：在现有 Vue 3 + Pinia + Vue Router + Semi Vue 上改造面客界面。使用双引号 Logo、暖纸白、墨黑、编辑红及 Evidence Line，完成观点输入、解析确认、研究中、报告、历史和证据抽屉。中国 A 股语义固定为红色＝支持/正向，绿色＝反证/挑战。业务 API、研究工作流、权限、预算和发布规则保持不变。三张参考图是设计说明，不能作为整页图片嵌入产品；只有 Hero/场景插画允许使用位图，其余 UI 必须使用 Vue/CSS/SVG。

## 0. 输入依据、优先级与执行边界

### 0.1 已核对的资料

| 输入 | 用途 |
|---|---|
| [一期 PRD](../prd/金融C端Agent_MVP_PRD.md) | 单公司观点研究闭环、人工确认、研究真实状态、报告标准、历史记录 |
| [面客架构](金融C端Agent_面客产品架构_v2.md)、[开发交付 SPEC](开发交付_SPEC.md) | 架构与业务边界；不因前端换肤调整服务职责 |
| [上一版前端设计](2026-09-19-zhigu-frontend-design.md) | 三个主导航、Semi Vue 复用基础；与本轮视觉或历史路由冲突时，以本文为准 |
| `app/web/src/`、`app/web/package.json` | 实际路由、组件、store、接口调用和依赖 |
| `app/server/service/finance/types.go`、`research_service.go`、`questions.go` | 面客实际响应字段、取消/删除、追问限制；只读核对 |
| `handoff/contracts/report.schema.json`、`evidence.schema.json` | 报告完整性、证据字段与数据语义 |
| `前端素材/品牌封面视觉.png` | 第一批：Hero 场景、双引号母题、编辑式研究氛围 |
| `前端素材/Logo : Icon : Button : Tag 等基础 UI 素材.png` | 第二批：Logo、图标、按钮、标签、输入与通知的视觉关系 |
| `前端素材/Agent 真正工作时的动态研究与报告交互系统.png` | 第三批：解析、确认、研究、结论、Evidence Line、Drawer、空/异常状态 |

三张 PNG 均为 1536×1024。当前素材库**没有独立 SVG Logo、图标、字体文件或组件源代码**；图上诸如 `logo-primary.svg` 的文字是拟议交付名，不是已存在文件。基础素材图含较重的灰暗/发光展示效果，不作为界面透明度、模糊度或渐变要求。

优先级：用户本轮明确约束 > 现有业务契约与 PRD 的业务规则 > 本文实施规格 > 参考图的展示细节 > 上一版视觉值。若视觉要求需要尚不存在的数据，执行本文的降级方案，不自行扩接口。素材中的证券名称、日期、目标价、预计用时、证据数量均为示意，禁止当真实数据写入界面。

### 0.2 本次允许改动

- 面客 `layout/consumer`、`view/research`、`view/profile`、`view/strategies` 的展示及交互适配。
- `components/workspace`、`components/research`、新增品牌/基础展示组件及局部样式。
- `router/finance.js` 中历史页接线与兼容跳转；保留现有登录和管理员守卫。
- `researchConversation.js` 的请求生命周期、错误恢复、当前证据 ID、UI 状态隔离；只修复界面正确性，不修改领域规则。
- 现有 API wrapper 的前端类型/错误适配和请求取消传递；不得更换端点、请求体契约或鉴权方式。
- 面客静态 SVG/Hero 衍生资源、对应浏览器测试、截图审查记录。

### 0.3 非目标

不重写为 React/Next.js；不重做管理后台；不更换模型协议；不实现真实金融数据接入；不新增服务、数据库结构、长记忆、证券交易、收益预测、持仓绑定、付费或注册流程。不开通 PDF 导出、公开分享、上传附件、图表终端、全站搜索、历史全文搜索和筛选 API。素材有这些图标，不代表本期需提供功能。

本规格的实施不自动授权部署上线；交付代码和可复现验收结果后，按仓库发布流程处理。

## 1. 开发目标与不可变业务规则

### 1.1 用户目标

用户能把一个 A 股单公司观点变成可核查的研究问题，确认范围，理解真实研究状态，分别阅读支持与挑战，沿引用查原文，并回看历史。界面呈现专业研究笔记的秩序与留白，不做交易大屏或聊天机器人气泡墙。

### 1.2 必须保留的规则

| 编号 | 不可修改的规则 | 前端执行要求 |
|---|---|---|
| B01 | 原观点 20–2000 个 Unicode 字符 | 保留 `Array.from(text).length`；空值、19/20/2000/2001 和中文输入法均测试；服务端校验仍为最终依据 |
| B02 | 解析后必须人工确认标的与期限 | 不自动创建研究；只选后端覆盖的候选标的；相对期限不擅自转换成未确认的年份 |
| B03 | 编辑原观点会使旧解析失效 | 调用 `setDraftText`；隐藏/禁用旧确认，重新解析；不能携带旧 draft/revision 启动 |
| B04 | 研究创建、追问有幂等键 | 保留 `Idempotency-Key`；等待中禁用重复提交；同一未确认结果的网络重试复用原操作的 key 和 payload，不改变服务端幂等机制 |
| B05 | 研究状态由服务端负责 | 不以时间、动画或按钮点击推定完成；失败不等于证据不足；取消请求不等于已取消 |
| B06 | 报告由服务端校验后发布 | 只显示 `runView.report` 已发布内容；`quality_status=incomplete` 时 `verdict=null`；不得拼研究中日志成报告 |
| B07 | 证据权限与引用边界 | 可点击引用必须属于 `report.evidence_ids`；不得猜测 ID、绕过鉴权或任意抓外链正文 |
| B08 | 追问限定本报告证据，最多 3 轮 | 不新增检索；服务端 `QUESTION_LIMIT` 为准；刷新不会重置真实额度，不宣称刷新能重获 3 次 |
| B09 | 重新研究创建新任务 | 带 `parent_run_id`，重新确认；旧报告不可被前端覆盖 |
| B10 | 删除与取消为不同操作 | 删除需确认；返回 `deletion_status=scheduled` 只说明删除请求已安排，不能宣称所有副本已立即清除 |
| B11 | owner 隔离、角色守卫、密钥保密 | 保留登录/管理员边界；面客不显示 API Key、内部 prompt、token、调度拓扑、管理员菜单 |
| B12 | 时间、版本、样本模式真实 | 保留 `mode/as_of`、证据三类时间、单位/期间/实际或估计值；fixture 不能包装成实时市场 |

业务逻辑不改，不等于保留现有 UI 缺陷。修复确认按钮无作用、颜色反向、抽屉不可见、请求串页和失败重试失效，属于本规格要求的前端正确性修复。

## 2. 视觉系统与 Design Tokens

### 2.1 Editorial Research 视觉规则

1. 暖纸白为大面积底色，正文墨黑，编辑红只用于品牌、主操作、选中态及支持语义。红色实色不覆盖大面积页面背景。
2. 用标题、细分隔线、引文、编号、时间和留白建立层级。卡片用于确认、证据、结论等独立内容，不给每段文字套卡片。
3. 核心标题可用本机衬线字体，控件和长正文采用清晰的无衬线字体；不加载未知授权的商业字体。
4. 双引号象征“观点与证据”，不用“知”字色块代替 Logo；品牌图形始终是品牌红/墨黑/反白，不因挑战结论把整个平台 Logo 改绿。
5. Evidence Line 是证据关系摘要，不是涨跌走势图、支持票数比较或完成进度条。
6. 参考封面中的黑色暗角只属于场景图片；不为整个 App 增加暗角、噪点、模糊、玻璃或霓虹发光。

### 2.2 颜色 Token（本规格确定的实施值）

以下值是对素材方向的工程化定义，不声称从图片精确取色。采用语义变量，禁止业务组件直接使用 Semi 的 `green/red` 来推断结论。

| Token | 值 | 用途 |
|---|---|---|
| `--zg-paper` | `#F7F5F0` | App 主背景，暖纸白 |
| `--zg-surface` | `#FFFEFB` | 正文、卡片、抽屉纸面 |
| `--zg-surface-muted` | `#EEECE7` | 次级背景、禁用控件底色 |
| `--zg-ink` | `#171717` | 标题、正文 |
| `--zg-text-secondary` | `#666666` | 辅助文字；重要字段不再减透明度 |
| `--zg-line` | `#DEDBD4` | 装饰分隔线 |
| `--zg-control-border` | `#8C8982` | 必须可识别的输入框/控件边界 |
| `--zg-brand` | `#E85858` | 双引号 Logo、大图形、装饰强调；不直接用作小字白底对比 |
| `--zg-action` | `#B83A3D` | 主按钮背景、可读红色文字、焦点边框 |
| `--zg-action-hover` | `#A93235` | 主按钮 Hover |
| `--zg-action-active` | `#962C30` | 主按钮 Pressed |
| `--zg-support-fg/bg` | `#B83A3D` / `#FBEDEC` | 支持/正向/得到支持 |
| `--zg-challenge-fg/bg` | `#2E7D32` / `#EAF3E8` | 反证/挑战/受到挑战 |
| `--zg-mixed-fg/bg` | `#5B5193` / `#EEECF6` | 多空因素并存，双环图形 |
| `--zg-insufficient-fg/bg` | `#666666` / `#EEECE7` | 未知、证据不足，虚线空环 |
| `--zg-error-fg/bg` | `#A62F35` / `#FBEDEC` | 请求/表单错误，必须伴感叹号及错误文案 |
| `--zg-warning-fg/bg` | `#785600` / `#FFF5D9` | 需要注意的限制、未完成 |
| `--zg-info-fg/bg` | `#4F565E` / `#ECEEF0` | 离线说明和普通系统提示 |
| `--zg-overlay-mask` | `rgba(23,23,23,.24)` | 抽屉/确认框遮罩 |

**红绿语义的强制区分：**

- `supported` → 红色 + 支持图标 + “得到支持”；`challenged` → 绿色 + 挑战图标 + “受到挑战”。修正当前 `Report.vue` 的反向映射。
- 绿色反证不是“系统报错”，红色支持不是“推荐买入”。颜色只说明证据与当前观点的关系，不说明未来收益。
- 操作成功（复制成功、解析完成）用墨黑勾选 + 中性提示；不沿用系统默认绿色 success 造成歧义。研究完成用中性勾选，不以红/绿勾选暗示 verdict。
- 错误红与支持红是不同 token，且错误必须有“失败/错误”等文字、感叹号及恢复按钮；结论卡和错误条不可共用一个无标签色点。
- `mixed` 使用紫灰双环，不用红绿渐变；`insufficient` 使用中性虚线，不标红为失败。

已按 sRGB 公式计算：墨黑/纸白约 16.45:1；次级文字/纸白 5.27:1；白字/action 5.66:1；support-fg/bg 4.97:1；challenge-fg/bg 4.51:1；mixed-fg/bg 5.90:1。品牌色 `#E85858` 配白字仅约 3.52:1，故主按钮采用 `--zg-action`。执行时仍需检查最终合成背景、Hover/Focus 等实际对比度。

### 2.3 字体、尺寸、间距和层级

```css
.zhigu-consumer {
  --zg-font-body: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
  --zg-font-editorial: "Songti SC", "Noto Serif CJK SC", "Source Han Serif SC", serif;
  --zg-font-number: ui-monospace, "SFMono-Regular", Consolas, monospace;
  --zg-space-1: 4px; --zg-space-2: 8px; --zg-space-3: 12px;
  --zg-space-4: 16px; --zg-space-6: 24px; --zg-space-8: 32px;
  --zg-space-12: 48px; --zg-space-16: 64px;
  --zg-radius-control: 6px; --zg-radius-card: 10px; --zg-radius-tag: 999px;
  --zg-reading-max: 840px;
  --zg-shadow-overlay: 0 12px 40px rgba(23,23,23,.12);
  --zg-z-header: 20; --zg-z-popover: 100; --zg-z-mask: 200;
  --zg-z-drawer: 210; --zg-z-modal: 300; --zg-z-toast: 400;
  --zg-duration-fast: 120ms; --zg-duration-normal: 180ms;
  --zg-duration-drawer: 240ms;
  --zg-ease: cubic-bezier(.2,.8,.2,1);
}
```

| 文本角色 | 桌面字号/行高 | 390px 字号/行高 | 字重 |
|---|---|---|---|
| Hero 标题 | 40/52 | 28/38 | 600，编辑衬线 |
| 页面 H1 | 28/38 | 24/34 | 600 |
| 报告/分组 H2 | 22/32 | 20/30 | 600 |
| 证据卡标题 H3 | 18/28 | 18/28 | 600 |
| 报告正文 | 16/28 | 16/28 | 400 |
| 控件与辅助说明 | 14/22 | 14/22 | 400/500 |
| 元数据 | 12/18 | 12/18 | 400；不得用于结论正文 |

按钮：桌面标准高 40px，主 CTA 44px；移动端所有点击区域最少 44×44px。图标本体 20/24px，流程节点图标 24/32px。卡片内边距 24px，手机 16px。常规卡片不用阴影；层叠浮层才用轻阴影。正文长度不因容器变窄而缩小字号。

### 2.4 Semi Vue 接入方式

保留现有 `@kousum/semi-ui-vue@2.78.4`、`@kousum/semi-icons-vue@2.78.0` 和 Vue 工程。该包为 Vue 适配库，不能直接照抄 React Semi API。

- 继续使用 ConfigProvider、Button、Input/TextArea、Select、Tag、SideSheet、Modal、Toast、Skeleton 等已安装包实际提供的组件；写代码前核对本地包导出与类型。若某组件不存在，用简单 Vue/CSS 实现，不为此升级整套依赖。
- 统一映射 `--semi-color-primary*` 到 action、`--semi-color-bg-*` 到纸面、`--semi-color-text-*` 到墨黑/次级色。结论另用 `EvidenceTag` 语义组件，避免受全局成功/错误主题影响。
- 先验证包的基础样式已加载，再施加主题。当前源文件未见统一 Semi CSS 入口，抽屉定位问题需结合运行时和本地包样式验证，不能仅用大 `z-index` 或 `overflow:hidden` 掩盖。
- `.zhigu-consumer` 限定 C 端样式；后台 Element Plus 保留。Portal 根节点也必须继承主题，但不落在受裁切的正文滚动容器中。
- 不继续增加针对所有 `.semi-navigation`、`.semi-layout` 的无差别 `!important`；优先业务类名和组件公开参数。特别避免移动抽屉继承桌面 88px 导航宽度。
- 可以把 Chat 当消息渲染容器，但必须由应用持有业务状态；如果其默认气泡/布局限制报告阅读，改为 Vue 消息列表，不更换 store/API，也不启用其上传、自动发送、模型请求功能。

## 3. 素材与组件映射

| 设计输入 | 目标资源/组件（待创建） | 规则 |
|---|---|---|
| 第一批品牌封面 | `assets/brand/hero-research.webp`、`hero-research-mobile.webp`；`EditorialHero.vue` | 只用于新研究空白欢迎态；使用场景本身，不充当页面背景；正文与 CTA 由 DOM 渲染 |
| 双引号图形/字标 | `assets/brand/zhigu-symbol.svg`、`public/favicon.svg`；`ZhiguLogo.vue` | 按确认的双引号轮廓制作干净矢量；不从整板裁小 PNG；可访问名称“知股 Zhigu”。产品内不再使用横版字标 SVG |
| 基础图标 | `ZhiguIcon.vue` + 本地 SVG/现有图标包 | navigation 24px、process 32px、evidence 20px；统一线宽约 1.75–2px，使用 currentColor |
| Button/Tag/Input 板 | `ZhiguButton.vue`（必要时封装）、`EvidenceTag.vue`、现有 Composer/ConfirmCard | 用 Vue/CSS/Semi 实现状态；图上的 Hover 发光不复制 |
| 第三批解析/确认 | `ClaimParseState.vue`、`ResearchConfirmCard.vue` | 真实 busy/解析结果驱动，不依次播放伪任务列表 |
| 研究进行中 | `ResearchStatus.vue` + `ResearchStageRail.vue` | 后端真实阶段映射；不显示图上示意“预计 2–3 分钟” |
| 证据线 | `EvidenceLine.vue` | SVG/CSS + 可访问文本，计数和关系由合法报告字段计算 |
| 结论与证据项 | `VerdictSummary.vue`、`EvidenceArgument.vue`、`EvidenceReference.vue` | 数字、日期、证券、来源全部绑定数据，不复刻图片内示例 |
| Evidence Drawer | `EvidenceDrawer.vue` | 可交互抽屉；原文/关键信息/相关数据是实际字段视图 |
| 空状态/异常状态 | `EmptyResearchState.vue`、`ResearchErrorState.vue` | 空状态用纸页 SVG；异常用 SVG 图标和文字，不需要位图 |

素材生产要求：

- 三张设计板保留在 `前端素材/`，不得整体复制到 `public/` 当可点击 UI，也不得把 Logo/Icon 板作为 CSS sprite。
- Hero 原图含英文书脊及场景文字，这些只算装饰，不用 OCR 提取为产品文案。保留右侧书/笔/双引号焦点，避开大面积黑暗边缘；Hero 外层与页面纸面有清晰边界。
- 1440/1024 Hero 场景容器 3:2；768 使用 3:2 紧凑图；390 隐藏装饰场景，保留矢量 Logo、标题及输入。Hero 不出现在研究中/报告/历史页。
- 输出 WebP 优先，宽约 768/1280 的 srcset，固定 width/height 或 aspect-ratio 预留空间；桌面资源目标 ≤250KB，较小版本 ≤120KB。达不到目标时优先减少场景尺寸，不牺牲文字 UI 清晰度。
- 不引入未经确认的新摄影、插画或外链字体。位图作为装饰 `alt=""`；品牌 SVG 若旁边已有可读字标，则图形 `aria-hidden`，链接整体有名称。
- SVG 必须是几何路径，不含嵌入 PNG、脚本、外链或跟踪内容。Logo 不使用普通引号字符拼凑，需在 16/24/32/64px 审查轮廓和空隙。
- 素材清单记录原图、衍生资源、用途、尺寸、来源；未确认权属的素材不得把权属写成“MIT”。实现前端原型可使用用户提供素材，公开发版核对来源记录。

## 4. 信息架构、页面壳与组件树

### 4.1 导航与路由决策

保留三个主导航：**个人界面 / 投研观点 / 交易策略**。历史是顶栏常驻辅助入口，路由独立；不把素材里的设置、更多等图标都增加为主导航。

| 路由 | 新规格 |
|---|---|
| `/app/research/new` | 新观点输入、解析、人工确认；同路由内状态变化 |
| `/app/research/:id` | 初始加载、running、report、incomplete、failed/canceled 等；不新增 `/running`、`/report` 路由 |
| `/app/history` | 独立历史记录页，恢复 PRD 定义，删除当前跳往个人页的 redirect |
| `/app/profile` | 保留账户与“查看我的研究”入口；共享样式，不复制完整历史列表请求 |
| `/app/profile?tab=history` | 兼容重定向到 `/app/history`，其余 profile query 不自动丢弃或误跳 |
| `/app/strategies` | 诚实规划占位页，“前往投研观点”；不出现下单/回测按钮 |
| `/login`、`/admin/*` | 现有功能和权限不变；登录可换 Logo，不扩展认证功能 |

历史页不把三个主导航中的任一项假装为当前页；顶栏“历史记录”显示当前态。侧栏“投研观点”用于返回当前任务（若存在）或新建入口；明确的新建动作调用 reset，再去 `/app/research/new`。从历史打开任务必须按所选 ID 加载，不能继承另一份报告。

### 4.2 推荐组件树

```text
ConsumerLayout
├─ SkipLink
├─ SideNav ─ ZhiguLogo + 三个导航项
├─ WorkspaceHeader ─ 标题 + 历史记录 + 新研究 + 更多
├─ DataModeNotice（由 FixtureBanner 改造）
├─ MainViewport / RouterView
│  ├─ NewResearchPage
│  │  └─ ResearchChatPane
│  │     ├─ EditorialHero（仅 pristine）
│  │     ├─ UserClaimBlock
│  │     ├─ ClaimParseState / ResearchConfirmCard
│  │     └─ ResearchComposer
│  ├─ ResearchDetailPage
│  │  └─ ResearchChatPane
│  │     ├─ UserClaimBlock / ResearchScopeHeader
│  │     ├─ ResearchStatus ─ ResearchStageRail
│  │     ├─ EvidenceLine
│  │     ├─ Report
│  │     │  ├─ VerdictSummary / PublicationNotice
│  │     │  ├─ EvidenceArgument[] ─ EvidenceReference[]
│  │     │  ├─ Assumptions / ChangeConditions / Unknowns
│  │     │  └─ SourceIndex / ReportActions
│  │     ├─ FollowupThread ─ EvidenceReference[]
│  │     └─ ResearchComposer / ContextualRecoveryAction
│  └─ ResearchHistoryPage
│     └─ ResearchHistoryList ─ ResearchHistoryRow / Empty / Error
└─ ViewportOverlayRoot（独立视口层、主题继承）
   ├─ MobileNavigationDrawer
   ├─ EvidenceDrawer
   ├─ DeleteConfirmation
   └─ Toast / SelectPopup / Tooltip
```

组件名单是职责边界，不要求每一个小标题都拆成文件。通用展现组件只收 props/发 emits，不直接发请求；容器/store 调 API。新增业务状态不得在组件、Chat 内部与 Pinia 各维护一份。

`HistoryDrawer` 已从产品中移除。顶栏与“查看全部”进入 `/app/history`，不再保留第二套历史浮层。不删除后端历史能力。

### 4.3 滚动与浮层

App 高度使用 `100dvh`，header/sidebar 固定在壳内；正文仅一个主要滚动容器。Composer 在研究工作区底部占布局空间，可 sticky，但不能覆盖最后一条内容。初始页输入框紧随 Hero，不能贴在 900px 视口底端造成大段无效留白。

浮层根为独立视口覆盖层 `position:fixed; inset:0`，避免受主内容的 overflow/transform 限制；根默认不截获事件，实际浮层恢复 pointer-events。若改用挂在 body 下的 portal，必须给 portal 宿主加主题类、正确生命周期清理，并验证无后台样式污染。不能直接把 body 的 pointer-events 关闭。

## 5. 路由逐页规格

### 5.1 `/app/research/new` — 输入与观点解析

**初始空白态：**

- Header：页面标题“投研观点”，右侧“历史记录”和更多；未输入时不重复放“新研究”主按钮。
- Hero：矢量双引号 + 标题“让投资观点，经得起验证”；副文“从一个观点出发，查看支持、反证与仍未确定的部分。”右侧场景插画只作视觉引导。
- 输入区有持续可见的 label“投资观点”，placeholder“粘贴一个关于单家 A 股公司的投资观点，20–2,000 字”。下方解释当前覆盖范围，以后端候选/证券列表为准，不写“全市场已接入”。
- 三个示例按钮沿用 `CLAIM_EXAMPLES`，仅填文本、不解析、不提交；fixture 环境示例明确是演示公司，不换成素材里的真实股票结论。
- 主按钮“解析观点”，右箭头；初始 `0/2000` 用次级灰。未触碰输入时不显示红色错误；字符不足在失焦或尝试提交后提示；超过 2000 明确错误。恰好 2000 合法，不能照素材写成“已超上限”。

**提交/解析中：**

- 显示用户观点块和单个 loading 解析卡：“正在解析你的投资观点…”；辅助文字“识别标的、研究期限与关键主张”。这些是任务说明，不是已执行日志。
- 无解析事件接口，不能逐项打勾“证券已识别→期限已识别→主张已提取”；不展示三段虚假过程。
- 一次只允许一个解析请求。锁定该次输入或以请求快照防止旧响应覆盖新输入；保留原文。失败展示可重试错误条，输入可编辑。

**解析完成/人工确认：**

- 卡片标题“确认研究范围”，按“标的公司、研究期限、关键主张”排序；最多 6 条主张并标注事实/推断/假设。
- 标的 Select 来自 `draft.candidates`，展示 name + symbol（如有）；不能自行输入未覆盖证券 ID。候选为 0 时提示修改观点，禁止确认；多候选保留显式选择。
- 期限字段展示 `suggested_horizon` 并可编辑；未提供则留空请求确认，不生成默认 6–12 个月。
- “修改原观点”必须将焦点移回输入框并允许修改；现有空回调必须接线。修改主张的路径是修改原观点并重新解析，不直接篡改后端 draft.items。
- 显示“确认后将冻结本次研究的数据截止时间”；提交字段保留 `draft_id/revision/instrument_id/horizon/as_of/parent_run_id/claim_text`。
- “确认并开始研究”只有 `canStart` 为 true 才可点。等待中按钮显示“正在创建研究”，防双击；成功取得 `run_id` 后才导航。
- `ACTIVE_RUN_EXISTS` 显示“已有研究进行中”，有真实 active ID 才给“返回研究”链接；不把创建失败伪装为排队成功。

### 5.2 `/app/research/:id` — 首次载入与研究中

**首次访问、刷新和切换 ID：**

- 先展示 scope/status 骨架，占位不含 fake 证券、时间和结论；未获取详情不显示新研究 Hero。
- 请求以 URL 的 ID 为准。切换 A→B 时清空 A 的可见 report/evidence/错误；A 的迟到响应不得覆盖 B。
- 失败时显示“暂时无法读取这项研究”与重试；404/越权采用相同“该研究无法访问”文案，返回历史。不能露出其他用户信息。

**Running 正文顺序：**原观点/标的期限 → 当前真实阶段 → 流程轨道 → Evidence Line 未发布态 → 说明/异常 → 取消操作。下方 Composer 禁用并提示“研究完成后，可基于已发布证据追问”。

| 后端状态 | 主要文案 | 流程表示 | 操作 |
|---|---|---|---|
| `queued` | 正在排队等待研究 | “排队”当前；其余待开始 | 取消研究 |
| `researching` | 正在调查支持与反证 | “调查”当前；不拆成独立工具调用进度 | 取消研究 |
| `verifying` | 正在核对证据与结论 | “核对”当前；报告尚未发布 | 取消研究 |
| `canceling` | 正在取消研究 | 停止正常进度动画；保留已知阶段 | 禁用取消并显示“取消中” |
| `completed` 且有效 report | 研究已完成 | 中性完成标识，转报告态 | 阅读/复制/追问/重新研究 |
| `incomplete` | 研究未完成 | 不把未执行节点标为 done | 阅读已发布部分（如有）/重新研究 |
| `failed` | 研究失败 | 错误态，不能全步骤勾选 | 重新研究/返回历史 |
| `canceled` | 研究已取消 | 停止态，不能全步骤勾选 | 重新研究/返回历史 |
| 未识别值 | 暂无法识别研究状态 | 中性未知态 | 刷新状态/返回历史 |

素材四个节点“理解观点 / 查找证据 / 交叉核对 / 生成报告”在当前接口下作如下收敛：理解观点在创建前确认卡完成；running rail 显示排队/调查/核对/报告四个位置；报告只有实际发布后标完成，不伪造单独 `generating_report` 事件。`stage=done` 本身不代表研究成功；终态 `status` 和 report 发布质量优先。

当前轮询基准为 2 秒，保留现有策略，修复重入/重复定时器、卸载后请求和过期响应。断网保留最后已知状态并标注“连接中断，显示上次状态”，恢复查询后才能更新；不让 loading 无限替代已加载内容。最近更新时间来自 `updated_at`；不把它当任务开始时间。无准确开始时间和 ETA，默认不显示计时器或预计用时。

### 5.3 Evidence Line — 数据诚实的证据关系摘要

**布局：**细中性横线与中央双引号节点；左侧为支持红，右侧为挑战绿，下方单列“未知项”。左右位置不因结论变化交换。节点尺寸固定，不因数量扩大成胜负图。390px 将标签/计数分两行，未知项放下一行，保留明确文字。

**发布前：**中性节点 + “证据整理中，发布后显示可核查来源”。支持和挑战均为“—”，不是 0。当前 `ResearchView` 没有实时证据流/计数，不能将素材的 3→5→8 用定时器播放；不请求尚未授权的 evidence ID。

**发布后计算规则：**

```text
allowed = Set(report.evidence_ids)
supportIDs = union(report.support[*].evidence_ids) ∩ allowed
challengeIDs = union(report.challenge[*].evidence_ids) ∩ allowed
supportCount = size(supportIDs)
challengeCount = size(challengeIDs)
unknownCount = length(report.unknowns)
```

- 标签必须是“支持来源 N / 挑战来源 M / 未知项 K”；来源计数按 evidence ID 去重，未知项是条目数，不能统称“未知证据 K”。
- 同一 ID 同时支持一个子主张并挑战另一个时，可计入两侧；提示“同一来源可能关联不同论点；数量不代表证据强度”，不得强制互斥或算多数票。
- 若没有 report → `null/—`；有合规 report 且数组为空 → `0`。不要混用这两种情况。
- completed/insufficient 可以是 0/0 并展示证据不足；不推断“没有反证＝得到支持”。
- `incomplete` 仅统计已发布部分，并在证据线上方标“部分发布，非完整覆盖”；不绘制胜出一侧。
- 两侧按钮如可交互，只滚动/聚焦对应报告章节。0 或无数据时为普通文本，不生成假链接。
- 图形 SVG `aria-hidden=true`，旁边有同等文本摘要。屏幕阅读器不逐点朗读动画。

未来只有后端新增经授权、可审计的证据事件契约后才能扩展实时节点；本期不得修改后端以配合动画。

### 5.4 `/app/research/:id` — 已发布报告

报告直接出现在同一页，不强制先点“查看完整报告”才能看正文。若保留该按钮，它只是同页锚点，不二次创建任务、不新开空白页面。

内容顺序固定：

1. **原观点与研究范围**：原文、证券 ID（有可靠映射时加名称）、期限、数据截止时间、模式、报告版本。
2. **核心判断**：`quality_status` 和 `verdict` 分开显示；一段 summary，不自行改写为买卖建议。
3. **Evidence Line**：按上节计算的已发布来源关系。
4. **支持证据**：每条 Argument 展示类型、正文、引用编号；支持红用于细线与 Tag，正文仍墨黑。
5. **最强反证**：同等层级与可读性，挑战绿；不默认折叠，不用更小字弱化。
6. **关键假设**：`report.assumptions`，当前组件遗漏该字段，本轮补齐；不冒充已验证事实。
7. **改变判断的条件**：`change_conditions`。
8. **未知项与证据缺口**：`unknowns`。
9. **来源索引**：`report.evidence_ids` 顺序形成稳定 `[1]…[N]`，同报告所有入口一致。
10. **报告操作与追问**：复制报告、重新研究、基于该报告追问；删除放更多菜单。

四种结论：`supported/得到支持`、`challenged/受到挑战`、`mixed/证据混合`、`insufficient/证据不足`。只有后端 verdict 决定结论。证据不足是已完成调查的一种合法结论，不用请求错误样式。报告未完成则没有这四者之一的总判断卡，显示“研究未完成”的质量告知。

空 section：存在合规报告但对应数组为空时显示“本次已发布报告未提供支持证据/反证/该类条目”，不复制“已搜索所有来源”之类无证据声明。来源正文缺失不补写、不使用网页摘要替代。

**追问：**

- 报告存在即可按现有后端能力提供追问，包括已发布的 incomplete 报告，但必须保留“研究未完成，回答仅限已发布部分”的提示。未发布 report 不提供追问。
- 明示“仅使用本报告证据，不进行新检索；每份报告最多 3 轮”。现有接口没有历史追问列表或剩余额度，不显示伪造的准确剩余次数；本地 3 次门禁和服务端 QUESTION_LIMIT 同时保留。
- 追问输入采用非空校验；不要把原观点的 20 字下限误套到“依据是什么？”这种短追问。原观点仍严格 20–2000；追问前端上限维持 2000，服务端接口不改。
- loading 留下用户问题 + 中性等待提示，收到 answer 后一次性呈现，不做假 token 流、假打字机。失败保留原文，不吞掉输入或占用本地成功轮次。
- `answer.evidence_ids` 与 allowed 集合相交后，用真正 `EvidenceReference` 按钮显示；修正现有空 span 引用。answer/limitations 以安全文本渲染。
- 提示“追问记录仅保留在本次会话，刷新后不会完整回放”；刷新不能让服务端额度清零。服务端拒绝后锁定该会话追问入口，提供重新研究。

**复制：**仅复制已发布内容，保留质量状态、四态 verdict（有值才写）、假设、未知项、版本、截止时间、模式及引用编号。当前复制文本缺少质量/假设等时应补齐。不得在未加载来源详情时宣称附带了完整原文/URL；模式缺失写“暂未提供”，不能默认当 fixture 或 live。复制失败提示失败，不先弹成功。

**缺报告：**`completed` 却 `report=null` 不展示“完整报告已生成”；显示“报告暂不可用”并可重试查询。错误状态和 report 字段矛盾时保留真实任务状态；不自行合成结论，把矛盾记入验收缺陷。

### 5.5 Evidence Drawer

入口：报告与追问的引用编号、来源索引。打开时记录触发按钮、`requestedEvidenceId` 和所属 `run_id`；只能读取 allowed ID。保留报告滚动位置，不导航新路由。

| 区域 | 内容/绑定 |
|---|---|
| Header | “证据详情”、本报告引用编号、关闭按钮；所属支持/挑战关系由报告引用上下文推导，不能从证据正文猜测 |
| 标题 | `evidence.title`；来源类型 `source_kind` 的中文映射；不把类型“新闻”伪装成媒体名称 |
| 原文 Tab（默认） | `text`、`locator`、安全原文外链、复制原文；保留换行，作为纯文本 |
| 关键信息 Tab | `published_at/available_at/retrieved_at`、`data_version`、`mode`；名称明确区分披露/可获得/获取时间 |
| 相关数据 Tab | `metrics[]`：metric、value、unit、period_start/end、value_type；无数据时显示“此证据未提供结构化指标” |

source_kind：filing＝公告/披露，financials＝财务数据，market＝市场数据，news＝新闻，fixture＝离线样本；未知值给中性“来源类型暂未提供”。Schema 未承诺 `source_name/currency/评级/目标价/可信度分数`，不可虚构。金额单位保留后端原值或有明确映射的中文解释，`value="0"` 必须显示 0，不经过浮点运算改写金融精度。

状态行为：

- loading：抽屉立即在视口内打开，骨架 + “正在加载证据”，关闭始终可用。
- success：显示上述字段；同一 evidence 在两侧被引用时 Tag 显示“同时关联支持与挑战”，不强行给单一极性。
- empty：成功响应但缺少必要证据正文/结构时显示“证据内容暂不可用”，不显示空白假成功。可重试/关闭。
- error：错误条 + “重试加载”。必须按保存的 requestedEvidenceId 重试，不能依赖尚不存在的 `evidence.evidence_id`；修复现有失败后重试无效路径。
- 快速打开 A→B：只显示 B 的响应；关闭后的迟到请求不得重新打开或污染下次 Drawer。
- 关闭：Esc、关闭按钮或遮罩；焦点回到仍存在的触发元素，否则回报告来源区。锁定背景滚动/焦点，Tab 不穿透。
- Tabs 只改变展示，不触发新模型推理；当前 Tab 变化不伪造“AI 提炼的关键信息”。

### 5.6 `/app/history` — 我的研究

- H1“我的研究”，说明“回看观点与证据，重新验证新的问题”，右侧“新研究”。
- 使用编辑式列表，不做行情表格。每条显示：证券 ID/可靠名称、状态、创建时间、离线/真实模式、打开入口与更多操作。
- 当前 HistoryItem 只有 `run_id/status/instrument_id/mode/created_at`；不能虚构观点全文、期限、verdict、证据数或报告摘要。不为凑卡片额外并发加载所有详情；标题回退“证券 ID · 观点研究”。
- 数据来源 `listResearch({limit:20,cursor})`，按服务端顺序；“加载更多”追加且按 run_id 去重。无 cursor 不显示更多；加载失败保留现有列表并允许重试该页。
- 首次 loading：3–5 行中性骨架；成功且零记录才显示纸页 SVG + “还没有研究记录” + “开始第一条研究”；请求失败不是空态。
- 打开：跳 `/app/research/:id`，按真实状态呈现 running/report，不用状态名拼不存在的路由。
- 重新研究：去 `/app/research/new?parent_run_id=<id>`，创建新任务而不是覆盖记录。当前无需求强制预填原文；若从已加载详情带原文可预填，但仍需重新解析/确认，不添加后台 copy API。
- 删除：在该行更多菜单中确认，删除目标 ID 来自该行，不调用可能指向其他任务的 `deleteCurrent()`。确认文案区分“从研究记录移除并安排清理”和“取消研究”；活动任务删除提示将先停止相关执行。
- 返回 `scheduled` 后标“删除处理中”，重新拉取列表验证移除；还存在的行保留处理中标识，不能宣称彻底销毁。删除失败保持记录与恢复入口。
- 不增加无接口支持的全文搜索、排序切换或跨全部历史过滤按钮。本期仅按服务端排序分页。

## 6. 组件状态矩阵

下表是最低覆盖集。normal/hover/focus/pressed/disabled 同时适用于所有可交互控件。

| 组件 | 状态 | 可见结果/允许动作 | 禁止行为 |
|---|---|---|---|
| Hero | pristine / editing / parsed | pristine 展示；editing 保留紧凑引导；提交解析后移除场景 | running/report 重复大封面 |
| Composer(claim) | untouched / short / valid / long | untouched 灰计数；valid 可解析；校验错误文字关联输入 | 0 字和恰好 2000 字直接报超限 |
| Composer | parsing / creating | loading + 禁用重复发送，保留原文 | 清空原文、自动创建研究 |
| ConfirmCard | missing / valid / stale / submitting / error | 缺标的期限禁用；stale 必须重解析；错误可恢复 | 使用旧 revision；素材股票写死 |
| StageRail | queued / researching / verifying | 当前节点明确，其余 pending/真实完成 | 按计时器推进、展示推理 token |
| StageRail | canceling / failed / canceled / incomplete | 非成功终态；未执行节点不打勾 | 失败也全绿/全红成功勾选 |
| EvidenceLine | unpublished / complete / partial | — / 合法计数 / 部分发布标记 | 从未公开数据造数量与“胜率” |
| VerdictSummary | supported / challenged / mixed / insufficient | 红支持、绿挑战、紫灰混合、中性不足 | 根据支持条数生成结论 |
| PublicationNotice | incomplete / failed / inconsistent | 明确限制、已发布范围、恢复动作 | incomplete 填 null verdict 为 mixed |
| EvidenceReference | allowed / not-allowed / loading | 合法编号可点；非法引用给不可用文本并记诊断 | 非法 ID 发请求、猜原文链接 |
| EvidenceDrawer | closed / loading / ready / empty / error | 焦点/滚动正确，失败可按 ID 重试 | 离屏、背景穿透、A 响应覆盖 B |
| Followup | idle / submitting / answer / error / limit | 非空可提交；只报告证据；limit 给重新研究 | 自动外部检索、刷新重置额度 |
| HistoryList | initial-loading / ready / empty / error / more-loading | 空态和错误分离；追加失败保留旧行 | 失败显示“暂无数据”假空态 |
| DeleteModal | confirm / submitting / scheduled / error | 精确目标；后端受理后“处理中”；失败保留上下文 | 取消按钮等价删除、Toast 假成功 |
| DataModeNotice | fixture / live / unknown | fixture 常驻离线文字；live 中性数据模式；unknown 标模式未确认 | 默认 live、关闭后掩盖 fixture |
| Toast | success / error | 中文可读、短暂；重要错误同时在页面保留 | 用 Toast 替代所有错误恢复 |

## 7. Loading / Error / Incomplete / Empty 规则

四种状态必须分别建模，不用一个 Boolean `loading` 掩盖所有情况：

1. **Loading**：网络请求等待；无结果时骨架，有结果刷新时保留内容并轻提示。loading 不产生金融结论，也不让 skeleton 冒充原文。
2. **Error**：请求未成功、权限失效或任务失败。明确发生在“解析/创建/读取状态/读取证据/历史分页/追问”的哪一步；保留可恢复输入与已加载内容。
3. **Incomplete**：研究执行不完整，可能有后端发布的部分报告；明确缺口，保留 `verdict=null`，允许按现有能力查阅部分证据，不包装为完整研究成功。
4. **Empty**：成功响应确实为空；历史无记录、某类证据为空、无结构化指标分别写对应文案。断网/500/403 不得解释成没有记录。

错误映射应统一在 `researchCopy.js`/HTTP adapter，页面不得直接渲染未知的 `error.message`、堆栈或英文 Axios 文案：

| 情况 | 面客文案与恢复 |
|---|---|
| 网络超时/离线 | “连接暂时中断，内容已保留。”＋重试当前读取/操作；不新建另一研究 |
| HTTP 5xx / INTERNAL | “服务暂时不可用，请稍后重试。”保留上下文 |
| 401 | “登录状态已失效，请重新登录。”清理会话敏感状态，按已有登录流程恢复，不造 token |
| 403/404 详情 | “该研究无法访问。”返回历史，不泄露是否属于他人 |
| CONFIG_NOT_READY | “研究服务暂未配置完成，请稍后再试。”不给用户 API Key 表单 |
| UNSUPPORTED_INSTRUMENT | “标的不在当前数据源覆盖范围。”回到观点与候选确认 |
| ACTIVE_RUN_EXISTS | 返回真实活动任务；没有 ID 时不显示断链按钮 |
| QUESTION_LIMIT | 已达本报告 3 轮上限，提供重新研究 |
| 未知状态/模式/字段 | 中性“暂未提供/状态暂不可用”；不回退成 completed/live |

DataModeNotice 从当前可信响应 `mode` 取值：draft / run / report / history-item 的各自模式不能互相篡改。新页尚无模式信息时，当前 Stage A 环境可以显式使用公开的 fixture 环境配置；若未配置则显示“数据模式待确认”，不能凭浏览器能打开页面推断 live。fixture 告知可改成低密度中性条，但文字持续可见、不能关闭后完全隐藏。

## 8. 响应式规格（四个必测宽度）

| 项目 | 1440×900 | 1024×768 | 768×1024 | 390×844 |
|---|---|---|---|---|
| 主导航 | 88px 常驻窄栏 | 88px 常驻 | 72px 常驻；该宽度不按 mobile 隐藏 | 顶栏菜单；260px 左抽屉 |
| 顶栏 | 64px 高，32px 内边距 | 64px，24px | 64px，24px | 至少 56px，16px；操作可收纳 |
| 主阅读列 | 最大 840px，居中 | 可用宽 888px 内取 840px | 可用宽 648px，全部使用 | 358px（左右16） |
| Hero | 左文右图，约 52/48，gap24 | 左文右图，gap24 | 上文下图，图高最多200px | 隐藏装饰位图，保留文字/Logo |
| 报告 | 单正文列，支持/反证顺序阅读 | 同左 | 同左 | 单列，16px正文不缩小 |
| Evidence Drawer | 520px 右覆盖层 | 480px 右覆盖层 | 440px 右覆盖层 | 100vw、100dvh 全屏 |
| Confirmation | 字段单列，操作靠右 | 同左 | 同左 | 字段单列；主 CTA 全宽，次操作另行 |
| 历史列表 | 内容与操作同一行 | 同左 | 长标题可换行 | 元数据换行，操作在行尾/下一行 |
| Evidence Line | 横线+三组文本 | 同左 | 紧凑横线 | 支持/挑战两列，未知项下一行 |

断点精确定义：`>=1200` 大桌面；`1024–1199` 桌面；`768–1023` 平板；`<768` 手机。还需在 767/768 与 1023/1024 检查切换，无双导航或丢入口。

- 上表阅读宽是内侧可用区：1440/1024 保留至少 24px 边距，768 为 `768-72-48=648`，390 为 `390-32=358`。不同时重复加内外 padding 把正文再次缩小。
- 手机点击“打开导航”后必须能在可见视口内找到三个入口并完成跳转；不能以 DOM 存在或 `isVisible()` 单独判断可用。
- 不用 `overflow-x:hidden` 遮盖溢出的控件。长 URL 使用换行，指标表允许局部横向滚动并标注，不让页面横向滚动。
- 输入区考虑 `env(safe-area-inset-bottom)`、虚拟键盘及视觉视口；键盘打开时输入与发送按钮可达，不用固定大 min-height 撑出空白。
- 200% 浏览器缩放与 320px 窄视口至少保证核心阅读/提交/关闭可用；顶部文字可收纳到有名称的更多菜单，不能只截断到无法识别。
- 抽屉打开时锁背景滚动；关闭后恢复原位置。Select 下拉必须在 Drawer/Modal 之上且不超出视口。

## 9. 动效与真实进度

| 动效 | 规格 | 数据约束 |
|---|---|---|
| Button/Tag hover | 120ms，颜色/边框，不改变布局 | 不触发任何研究动作 |
| 内容出现 | 180ms opacity + translateY ≤4px | 真实响应到达后才展示 |
| Drawer | 240ms 横向进入 + 遮罩淡入 | 打开后即具备 loading/关闭；不是等待 API 才出现 |
| 解析/当前阶段 loading | 800–1000ms 线性小 spinner | 仅表示等待，不是倒计时 |
| Evidence Line 更新 | 180ms 颜色/透明度过渡 | 有真实新发布数据时一次更新；不递增补齐数值 |
| 路由切换 | 最多150ms，或无动画 | 不保留上一任务内容充当过渡 |

明确禁止：虚假百分比、随机进度条、无依据 ETA、假日志、模拟搜索源、假 token 打字流、为了“动态”让证据数增长。**隐藏思维链不得向用户展示**：不输出内部 reasoning、system prompt、工具原始返回、Agent 私聊和调度调试日志；允许显示经公开接口确认的阶段摘要与已发布证据。隐藏思维链不意味着隐藏出处，证据文本、时间、单位、限制仍必须可查。

`prefers-reduced-motion: reduce`：禁用位移和循环装饰动画；保留静态“加载中”文字，可无旋转 spinner；滚动跳转改为即时。禁止闪烁或持续脉冲红光。

## 10. Accessibility

- 语义结构 `nav/header/main/article/section`，每页一个 H1，报告标题依次 H2/H3；提供“跳到主要内容”。
- 正文/控件文字对比度 ≥4.5:1，大字 ≥3:1，必要控件边界与图标 ≥3:1。不得靠色彩单独表示支持/挑战/错误。
- 所有图标按钮有中文 accessible name；引用按钮名如“查看证据 2：来源标题”，标题未知时只读“查看证据 2”。
- Focus ring 2px、offset 2px，在纸白/深色按钮上均清晰；不能 `outline:none` 后不补焦点样式。
- 表单 label 与控件关联；错误使用 `aria-describedby` 和 `aria-invalid`；字数中性读数不每敲一个字都打断朗读。
- busy 容器设置 `aria-busy`，阶段文本使用 `aria-live=polite`，仅在阶段变化时播报；2秒轮询不重复播报整份报告。
- Drawer/Modal：dialog + aria-modal、正确标题、初始焦点、Tab focus trap、Esc、关闭后焦点恢复；背景 inert 或等效处理。Tabs 支持左右键、Home/End，selected/tabindex 正确。
- Enter 默认换行，Ctrl/Cmd+Enter 显式提交；中文组合输入期间禁止触发。关闭或错误恢复后焦点落到可继续操作的位置。
- 装饰图 `alt=""`，SVG装饰 aria-hidden；Logo 的文字替代稳定。禁用按钮仍有邻近原因说明。
- 键盘完整走通输入→确认→研究→证据→历史；手机触控目标≥44px。读屏检查至少覆盖一个支持结论、一个挑战结论和未完成报告。

## 11. API 与数据绑定契约

### 11.1 保留端点

统一使用 `src/api/research.js` 与 `utils/http.js`，不在组件里直连模型、Python、金融供应商或任意原文 URL。

| 操作 | 端点/方法 | 绑定和限制 |
|---|---|---|
| 登录 | POST `/api/finance/auth/login` | 保留既有 token/role 会话流程 |
| 观点解析 | POST `/api/finance/claims/parse` `{text}` | draft_id/revision/candidates/suggested_horizon/items/needs_confirmation/mode |
| 创建 | POST `/api/finance/research` | 保留现有七个 payload 字段及 Idempotency-Key；接受后拿 run_id |
| 详情/轮询 | GET `/api/finance/research/:id` | status/stage/mode/as_of/claim/report/warnings/error/updated_at；不是流式事件 |
| 历史 | GET `/api/finance/research?limit=20&cursor=…` | items/next_cursor，不自行生成页码或 total |
| 取消 | POST `/api/finance/research/:id/cancel` | 以后端回读为准，可能先 canceling |
| 删除 | DELETE `/api/finance/research/:id` | deletion_status=scheduled；不宣称立即物理清除 |
| 证据 | GET `/api/finance/evidence/:id` | 必须先验本报告 allowed IDs；纯数据读取 |
| 追问 | POST `/api/finance/research/:id/questions` `{text}` | Idempotency-Key；answer/claim_type/evidence_ids/limitations/mode |
| 已覆盖证券 | GET `/api/finance/instruments` | items；可为 ID 补名称，不用静态全市场股票表替代 |

HTTP envelope 已在 interceptor 解包：页面使用 `res.data`，不重复 `.data.data`。保留 Bearer token 和 `X-Request-ID`；调试信息不得把 Authorization/prompt/用户完整观点写入公开日志。对 runView.error 与 transport error 分开处理。

### 11.2 View Model 与数据隔离

允许新增纯函数 `toResearchViewModel` / `toEvidenceLineModel` / `toHistoryRowModel`，负责中文标签、展示字段、合法计数、空值和状态映射；不负责推断 verdict、财务计算或研究调度。

- `status`（执行状态）、`quality_status`（发布完整性）、`verdict`（证据判断）是三个独立维度，不合成单一 success Boolean。
- 当前 report schema 的 quality 仅 completed/incomplete；不要把代码中容错的 `quality_status=failed` 当正式契约扩大。failed 是任务状态。
- 保留原始 timestamp；统一面客显示 `YYYY-MM-DD HH:mm（北京时间）`，明确使用 `Asia/Shanghai`；仅日期字段不进行隐式 UTC 日期漂移。
- 0/false 是有效值，缺失与空字符串用“暂未提供”；不得用 `value || '—'` 抹掉 0。
- URL 使用 `sanitizeHref` 的 http/https 白名单及 `rel="noopener noreferrer"`；外链新标签打开。源文本与模型输出不允许 `v-html`，不渲染脚本/原始 HTML/远程跟踪图片。
- 本地缓存以 run_id/report.version/evidence_id 隔离；账号切换、删除、退出清理相关前端状态。不将报告正文/证据新持久化到 localStorage。
- 解析、证据和详情请求使用 request sequence 或取消机制，避免慢响应串页；计时器只有一个，离开详情/退出时停止。前端取消网络请求不等于取消研究任务。

### 11.3 缺少的能力及默认降级

| 素材/期望 | 当前缺少的数据 | 本期默认处理 |
|---|---|---|
| 动态新增支持/反证节点 | 实时证据事件和授权计数 | 发布前 Evidence Line 显示“—” |
| “预计 2–3 分钟” | 可信 ETA/历史耗时统计 | 不显示预计时间 |
| 生成报告独立进度 | 独立细分阶段 | 核对后等待真正 published report |
| 历史观点摘要/判定/证据数 | HistoryItem 无这些字段 | 只展示当前字段，不静态补齐 |
| 追问历史回放/剩余次数 | 无查询接口 | 明示本次会话与服务端额度限制 |
| PDF 导出/分享链接 | 无产物或公开分享契约 | 不渲染按钮，保留复制报告 |
| Drawer“AI关键信息” | 没有额外模型提炼结果 | 关键信息只显示已有元数据 |
| “已尝试多种来源仍无证据” | 无充分搜索日志证明 | 改为“本次已发布报告未提供该类证据” |

不得为提升截图完整度创建生产 fallback mock。演示/测试数据只能进入测试 harness，标明 fixture，不能混入真实 API 失败分支。

## 12. 实施顺序、文件责任与每阶段验收

先做能独立验证的基础，再做路由状态，不一次性替换全部代码。每阶段提交要可构建、可回退；无关后端文件和管理功能不混入。

### Phase 0 — 锁定基线与验收夹具

工作：核对 Git HEAD/worktree；阅读本规格和契约；记录现有启动和测试结果。建立浏览器 API mock 场景（不进产品运行分支）：解析成功/失败、多候选、各状态、四种 verdict、incomplete、空历史、证据失败/重试、权限错误、追问限制。Mock 必须满足真实 schema。

- [ ] 记录实际基线 SHA；未覆盖用户已有修改。
- [ ] 保存当前桌面/手机截图，注明路由、状态和是否模拟数据。
- [ ] 明确当前前端与真实后端联调的验证边界。
- [ ] 输入边界、报告数据与证据引用夹具完整；不使用素材目标价当真数据。

**Review R0：**1440/390 的当前 new 页与已有问题；确认基线，不要求旧版本通过新设计。

### Phase 1 — Tokens、SVG、Shell 与浮层

工作文件：`styles/consumer-semi.css`、新增 `styles/editorial-tokens.css`、`components/brand/*`、`assets/brand/*`、`layout/consumer/index.vue`、`SideNav.vue`、`WorkspaceHeader.vue`、`utils/popup.js`。核对 Semi 组件基础 CSS，完成 Drawer 定位/主题隔离。

- [ ] 品牌双引号 Logo 使用 SVG；Hero 是唯一需要的场景位图；未把设计板导入页面。
- [ ] 纸白/墨黑/编辑红、文字层级、88/72px 导航和响应式正确。
- [ ] 手机导航可见且三个入口可操作，不在屏幕下方；Select、Modal、Toast 层级正确。
- [ ] 后台布局/Element Plus 不受污染；无窗口大小切换后残留遮罩。
- [ ] 键盘焦点、减弱动态效果和颜色对比基本检查通过。

**Review R1：**1440/1024/768/390 的 Shell；390 导航展开；Logo 16/24/32/64px 并排审查。

### Phase 2 — 新观点与解析确认

工作文件：`view/research/new.vue`、`ResearchChatPane.vue`、`ResearchComposer.vue`、`ResearchConfirmCard.vue`、新 Hero/ParseState、store 解析请求适配。

- [ ] Hero/输入有合理距离，无整屏空白；场景图失败不影响输入。
- [ ] 0/19/20/2000/2001、Unicode/中文组合输入、快捷键通过。
- [ ] 示例仅填充；20–2000 校验与按钮状态一致；0 不红、2000 不报超限。
- [ ] 解析失败原文保留；多候选/无候选/无期限可理解。
- [ ] 修改原观点按钮确实聚焦输入并使旧解析失效。
- [ ] 只有人工确认触发创建；重复点击只发一次当前操作，重试不换未决幂等键。

**Review R2：**pristine / parsing / confirmed / validation error / parse error，1440 与390；确认卡长文本在768可读。

### Phase 3 — 研究生命周期与 Evidence Line

工作文件：`view/research/detail.vue`、`ResearchStatus.vue`、`ResearchStageRail.vue`、`EvidenceLine.vue`、store 加载/轮询/取消。

- [ ] queued/researching/verifying/canceling/canceled/failed/incomplete 各有正确视图。
- [ ] 状态更新只来自接口；无伪百分比、ETA、模拟日志、隐藏思维链展示。
- [ ] 没有 report 时 Evidence Line 是“—”，有空报告证据才是0。
- [ ] 断线保留最后状态，重试恢复；刷新按 URL ID 恢复。
- [ ] A→B、迟到响应、离开后定时器清理、取消中重复操作均通过。
- [ ] 失败/取消未把所有阶段标为完成。

**Review R3：**同一1440/390视口的 researching / verifying / canceling / incomplete / disconnected；检查动画静止与 reduced-motion 两种模式。

### Phase 4 — 报告、证据抽屉与追问

工作文件：`Report.vue`、新增 VerdictSummary/EvidenceArgument、`EvidenceDrawer.vue`、`EvidenceReference.vue`、`researchCopy.js` 的复制报告文案、store 证据/追问适配。

- [ ] 四种 verdict 红绿正确；incomplete 不展示完整总判断。
- [ ] 原观点、截止时间、模式、版本、支持/反证、假设、条件、未知项均可读。
- [ ] 合法引用可点；非法 ID 不发请求；两侧去重计数及未知项计数正确。
- [ ] Drawer字段完整、零值正确、单位精度不改；失败重试使用 requested ID。
- [ ] Drawer 开关焦点/滚动/遮罩正确；A→B 请求竞态通过。
- [ ] 短追问可发送，额度失败有恢复说明；答案引用按钮真实可用。
- [ ] 复制报告/原文成功与失败都验；复制内容带限制，不伪造 PDF/分享功能。

**Review R4：**supported/challenged/mixed/insufficient/incomplete 五张结论；长报告顶部、中段、末尾；Drawer原文/数据/错误态；1440与390必拍，1024/768验证抽屉尺寸。

### Phase 5 — 历史路由与恢复流程

工作文件：`router/finance.js`、`view/research/history.vue`、`view/profile/index.vue`、`ResearchHistoryList.vue`。

- [ ] `/app/history` 直接可访问、刷新不跳个人页；旧 profile?tab=history 兼容。
- [ ] ready/empty/loading/error/分页失败互斥且正确；不出现“暂无数据”与500同时显示。
- [ ] 打开、重新研究、删除都操作正确 run_id；重新研究保持旧报告。
- [ ] 删除请求 scheduled 不被描述为已永久清除；列表回读验证。
- [ ] 个人页只有清晰的研究入口，不再两套列表状态互相冲突。
- [ ] 登录/管理员守卫保留；普通用户不暴露管理员 UI。

**Review R5：**历史有记录/空/失败、删除确认/处理中、390操作菜单；验证浏览器前进后退与刷新。

### Phase 6 — 集成与交付

- [ ] `npm run build` 通过，记录已有/新增 bundle warning，不能把 warning 写成测试失败或忽略新增巨大资源。
- [ ] `npm run test:e2e` 通过已有非 API 用例，并更新旧“history 跳个人页”的过时断言为本规格路由行为。
- [ ] 新增有意义的浏览器回归：抽屉实际可见区域、四态语义、incomplete、证据重试、跨 run 隔离、历史分页/删除、短追问与错误恢复。
- [ ] API mock 通过不等于真实后端通过；有本地 Go/Python/Postgres 环境时执行 `ZHIGU_E2E_API=1 npm run test:e2e`，否则明确未验，不伪造成功。
- [ ] 4个视口、200%缩放、键盘、长文本、减弱动态效果完成审查；无页面横向溢出或无法关闭浮层。
- [ ] 修改范围仅前端/测试/资源/文档；API、Schema、Go/Python 工作流未因 UI 改写。
- [ ] 提交变更清单、运行命令与结果、已知限制、素材清单和截图索引；不把本地凭据、node_modules、dist、浏览器状态文件提交进仓库。

**Review R6：**从新观点开始走一遍完整用户路径；无后端时分别给“契约模拟路径”与“真实联调未验证”的结论，不混写。

## 13. 截图审查与最终完成定义

截图保存到已忽略的 `reviews/editorial-implementation/<run>/`。命名：`R4-report-challenged-390x844.png`；索引记录 commit、URL、viewport、数据来源（mock/fixture/live）、状态、操作步骤、发现问题和是否复核。截图必须打开检查，不能只声称文件生成。

最低检查项：

- 构图：Hero 与输入的主次；正文对齐；支持与反证同等可见；红色点到为止。
- 状态真实性：截图里的数量/结论与 fixture 或响应一致；研究中没有未发布报告。
- 交互：hover/keyboard-focus/disabled/loading；真实点开 Drawer，测其边界位于视口内，而不只查 DOM。
- 长内容：长公司名/多候选、6条主张、2000字观点、20,000字证据、长 URL/指标单位、零值、缺失日期。
- 响应式：四个规定视口；手机键盘与安全区；抽屉内滚动不带动背景；报表底部不被 Composer 挡住。
- 安全与可信：非法外链/HTML 不执行；未授权 ID 不请求；fixture 和 incomplete 标签不能因截图美观被删掉。

最终验收为“功能通过＋视觉通过＋证据边界清楚”。任何未解决的主流程阻断、移动导航离屏、引用不可访问、红绿反向、incomplete 假完成或 mock 冒充 live，均不得标记完成。

## 14. 交给 AI Coding Agent 的执行指令

> 请以本文件为当前前端改版规格，先核对仓库状态与实际接口，按 Phase 0→6 实施。延续 Vue/Semi/Pinia，不改研究业务逻辑、服务端 API 和权限。用户已确认 Editorial Research、双引号 Logo、暖纸白/墨黑/编辑红与 A 股红支持绿挑战语义，不重新设计另一套风格。把三批图片转换为真实可交互组件；只有 Hero 场景可使用位图。缺少独立 SVG 时制作干净矢量，不将设计稿截图塞进 UI。缺接口能力按本文降级，不造百分比、证据数、ETA、数据或隐藏思维链。优先修复浮层位置、旧解析失效、证据重试、历史路由和红绿语义。每阶段运行相应检查并保存可复核截图；最终说明真实通过、模拟通过、未验证的边界。遇到契约冲突记录具体字段和阻断点，不修改后端来凑设计，不留下看似能点却无功能的按钮。

