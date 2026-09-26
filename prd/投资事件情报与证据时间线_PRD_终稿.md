# 投资事件情报与证据时间线 PRD

版本：v1.2｜日期：2026-09-26｜用途：48 小时 MVP 研发与验收基线。

本文规定产品行为和拟建接口，不代表接口已实现或数据账号已开通。替代 v1.0；删除待确认项、通用利好利空打分、30 分钟通知聚合，保留可追溯的事件主链路。

v1.2 增补模块层级、独立窗口和会话恢复契约；工程实现与测试细则见 [开发 SPEC](../spec/投资事件情报与证据时间线_开发SPEC.md)。

## 1. 目标与范围

帮助个人投资者、投研人员在一个页面回答：**谁最先说、事实怎么变、哪些说法冲突、目前是什么状态、与自选标的有什么关系。**

| 范围 | 本期要求 |
|---|---|
| 市场与事件 | A 股；收购、业绩预告、监管立案三类。其他类型显示“不支持自动分析”，不推断结论 |
| 必交 P0 | 关注标的、信源接入、事件归并、证据分级、版本与冲突、状态与变化摘要、出处定位、站内通知、管理员纠错、隔离回放 |
| 后续 P1 | 追问对话、行情窗口验证、自定义订阅、笔记、外部推送。本期不纳入验收 |
| 不做 | 买卖建议、目标价、收益预测、交易、全网实时监控、产业链推断 |
| 交付 | 可操作 Web URL、源码与 README、60–180 秒视频、AI 使用与验证记录、测试说明 |

默认关注即订阅，取消关注即退订；每人最多 20 个标的。演示提供 DEMO.A/B/C 三个虚构标的和内置样例，真实模式只展示实际接入范围。

## 2. 页面与主流程

“事件情报”与“投研观点（观点分析）”“交易策略”为同级业务模块。主导航新增“事件情报”，点击打开独立新窗口 `/app/intel`，不覆盖原窗口、不放入策略子菜单或观点对话抽屉。桌面请求浏览器独立窗口；浏览器策略或移动端改用独立标签页时保留相同隔离行为。内部列表、详情和通知在该窗口内切换。

添加自选 → 查看事件流 → 打开详情 → 核验原文 → 收到变化通知 → 回看前后差异。

| 页面 | 必须展示与支持 |
|---|---|
| 关注设置 | 代码/名称搜索、添加、删除；空态引导；演示模式可一键加载样例 |
| 事件流 | 标的与关系、事件标题、核实状态、业务阶段、信息新鲜度、最近变化；按标的/类型/状态筛选，按最近变化倒序 |
| 事件详情 | 当前结论、变化摘要、时间线、原始/最早已收录来源、有效与失效证据、冲突对照、历史版本、数据覆盖与最后成功处理时间 |
| 通知中心 | 变化原因、前后值、涉及的自选标的、已读/忽略/事件静音；点击定位冻结的变更版本 |
| 回放 | 下一步、重置、模拟时钟；显著“演示数据”水印；不写入真实数据或真实通知 |

所有页面提供加载、空态、失败与重试。移动端点击即可查看引用，不依赖悬停。传闻标注“未经证实”；全站显示“仅整理信息，不构成投资建议”。

## 3. 业务规则

### R1 事件身份与归并

事件是一个持续发展的业务事项，不是一篇文章。稳定身份由主体、事件类型和事项标识组成：收购用交易/方案标识，业绩预告用公司及报告期，立案用公司及案件/立案通知标识。证券名称先映射为唯一主体；无法确定则待处理。

归并顺序：① 同一事项标识或明确引用前序材料，直接关联，不限制间隔；② 同一主体、类型、客体/期间，且存在可核验的同一事项描述，关联；③ 仅名称相似或多个候选，进入疑似队列，不参与任何候选事件的结论；④ 明确为新事项，新建事件。7 天仅用于优先召回，不是身份门槛；召回还须覆盖全历史的精确标识与引用。

金额、比例、日期是可变参数，不参与身份相似度。相同双方的另一笔交易不得自动合并。一篇材料可包含多个事件。管理员可确认归属、驳回或拆分；必须填写理由并留审计。普通用户不能修改共享事件。

### R2 证据与分级

证据是一条可核验陈述，必须包含：主体、命题、适用期间、模态、参数与单位、来源修订版、原文片段。`claim_key` 由事件、命题字段、对象、期间、币种和统计口径组成；单位先归一化，不同期间/口径不直接判冲突。

| 分级 | 判定 | 系数 |
|---|---|---|
| 事实 fact | 已发生且有可核验原始凭证；“公司发布预测”是事实，“预测将实现”不是事实 | 1.0 |
| 观点 opinion | 有明确作者的评价、评级或判断 | 0.6 |
| 推测 inference | 有条件、未来预测或因果推演 | 0.3 |
| 传闻 rumor | 消息源不可核实或转述未经证实的说法 | 0.1 |

信源基础权重：法定披露/监管原文 1.0，具名媒体原始报道 0.6，具名研报 0.5，其他可核验来源 0.3，未知来源/社区 0.1。逐条判模态，不能因载体是公告就把全文定为事实；无法判定进入待处理，不计权。

有效权重 = 基础权重 × 分级系数；失效、撤回或隔离证据为 0。权重表示规则化支持强度，不表示真实性概率。

### R3 来源、修订与冲突

- 来源身份与修订分开：同发布方文档 ID/规范化 URL 定位来源；正文或有效披露元数据变化生成不可变修订；完全重复抓取不增版本。URL 相同不能跳过内容变化检查。
- 明确转载/引用的同一命题归入同一来源簇；多根引用保留多条关系。无原始凭证的转述统一按“独立性未核实”处理，不假设互相独立；环或断链标为未核实。
- 展示“已核实原始来源”与“已收录最早材料”。源头缺失必须说明；相同披露时间并列展示；未知时间不能宣称首发。
- 同一 `claim_key` 的互斥值形成冲突。更正必须有明确替代对象：同发布方明确更正或有直接管辖关系的监管结论可替代；同级更正有效，不能只比较等级。
- 旧证据保留并标记 `superseded/retracted`，退出当前计算；只解决被替代命题的冲突。两份同级权威材料无明确替代关系时保留争议，不按抓取先后选赢家。
- 后续权威原文明确核实同一命题时，早先未核实的转述/传闻转为历史证据并关联核实材料，不与后续更正反复冲突；后来的独立反证仍须检测冲突。
- 普通进度更新保留此前已发生事实；例如“签约→交割”是阶段推进，不是互斥冲突。参数修订与阶段变更分别记录。

### R4 状态与结论

状态分三轴，不能把“没收到新消息”解释为业务结束：

| 轴 | 枚举与规则 |
|---|---|
| 核实状态 verification | `unverified/confirmed/denied/disputed`。无直接权威证据为未核实；权威原文明确支持/否认核心命题才确认/否认；有效权威证据互斥且无法消解为争议 |
| 业务阶段 phase | 收购：`unknown/proposed/agreed/completed/terminated`；业绩预告：`unknown/forecast_issued/results_published/withdrawn`；立案：`unknown/opened/decision_issued/closed`。只接受直接原文依据 |
| 信息新鲜度 freshness | `fresh/stale/expired/unknown`，按 R5；“信息过期”不撤销已发生事实 |

每个事件保存一句固定范围的核心命题，如“A 公司存在收购 B 的该项交易安排”。只否认金额不能否认整个交易；“曾洽谈，现终止”不能改成“从未存在”。首条正式公告即可确认，无需第二条材料。新的明确翻案仍在原事项内增版本；明确属于另一笔事项才新建。阶段取有效时间最新的权威进展；同时间互斥或无法排序时为 unknown 并提示争议，旧闻晚到不回退阶段。

结论按 `事件 × 标的 × 命题` 计算。支持、反驳各自按来源簇取最高有效权重，再取各簇最大值，不求和；context 只展示不计权。正反证据分别保留；最高权重相等全部保留，按证据 ID 稳定排序。纯传闻不因数量增加升级。

支持度档位：≥0.8 高、[0.4,0.8) 中、其余低；命题存在冲突时最高为中。事件卡取核心命题支持度；已明确否认时改展示否认依据强度并标明方向。在任一关键参数有未决冲突时封顶中；分数不变、另记封顶规则。该档位不能替代核实状态。来源、规则版本、权重、有效证据、冲突和评估时点写入 `calc_trace`。

当前结论固定为：**已知事实 + 最新变化 + 未决事项 + 标的关系 + 来源覆盖**。标的关系仅为主体/交易对手，必须附主体映射及原文依据，不推断集团或产业链关系。本期不生成通用利好/利空评分：有明确影响观点时展示“某来源认为……”及原文；无依据显示“影响待评估”；相反观点并存显示“观点分歧”。不把未知显示成中性。

### R5 时间与新鲜度

| 字段 | 处理 |
|---|---|
| occurred_at | 实际发生时间；保留日/月/区间精度及“预计”属性；缺失为空 |
| disclosed_at | 原始披露时间；保留来源精度；缺失为空，不用抓取时间冒充 |
| fetched_at | 服务端首次获取该修订的时间；重复抓取另记巡检日志 |
| source_updated_at / event_updated_at | 原文修订时间 / 事件最近入账版本时间，分别显示 |

精确时间 UTC 存储、北京时间展示；仅日期的值保留日期及 `Asia/Shanghai`，不得伪装成精确时刻。计算边界按当地该日结束时刻保守取值；未来“预计”不得充当已发生时间。披露时间异常保留原值并标记存疑，不参与首发和新鲜度计时。

时间线按有效披露时间排序，同时间按来源 ID 排序；未知时间单列。接收历史按入账版本排序，标注“后补材料”。晚到旧材料不得覆盖已明确生效的新修订，也不得仅凭晚到触发翻案。

最新有效信息时点 `L` 取非转载、非隔离材料的有效披露/明确修订时间最大值，不能被重复抓取刷新；最新实质变更缺可靠时间时新鲜度为 unknown。评估时点 `as_of` 和全部输入快照固定：无新信息满 14 天 stale、满 30 天 expired；不另做权重衰减，无 60 天强制清空冲突。

仅在接入范围内巡检、分页补齐和抽取处理均成功、无缺口时推进新鲜度。源故障、截断、模型积压或不可补齐窗口时显示 unknown 并保留上次评估，不产生过期通知。恢复补齐后再判断；无法证明检索范围完整时保持“覆盖有限”，不自动判过期。新有效材料到来可恢复 fresh，历史过期记录保留。

### R6 变更与通知

每次有效事实、阶段、冲突、归属、核实状态或新鲜度变化都生成 `change_id + event_version`，保存前后快照；纯转载不增业务版本。每次计算保存快照，即使支持度档位未变。仅评估时点变化不算业务变更。

确认、否认、翻案、关键参数更正/撤回、业务阶段变化、冲突新增/解除、实质性新事实、首次进入信息过期，以及信息恢复，立即生成站内通知。通知按变更发出，**不以方向或档位改变为前提**。首次纳入关注后的新事件发“新事件”通知；新关注不补发旧历史通知。

唯一键为 `user_id + namespace + change_id`；同一变更多个自选标的合成一条并列出关系。不做跨变更合并，未读旧通知不覆盖。通知与变更通过事务 Outbox 持久化；重试不重复，崩溃可补发。事件同一时刻串行提交版本，管理员编辑使用版本校验。

已读/忽略只作用当前通知；静音停止该事项后续通知，不停止更新。取消关注、静音时取消未发送消息，发送前复核资格；重新关注/取消静音只接收之后的变化。每次 fresh/stale→expired 只发一次，扫描重试不重复。

### R7 AI 与出处

LLM 仅抽取实体、命题、模态、参数、引用及候选关联；规则决定归并、分级、状态和结论模板。JSON 不合法或引用校验失败重试一次，仍失败进入待处理并显示数量。记录模型、prompt/解析器版本、输入哈希、输出、校验结果。

校验主体、字段、数值、单位、期间、否定词及原文支持关系；数字出现不等于抽取正确。无法确定的高影响归并/确认/否认不得自动生效，交管理员处理。

引用绑定不可变 `source_revision_id`、内容哈希、Unicode 字符区间或 PDF 页码与片段。原文数字指向引文；支持度等派生数指向计算明细；抓取时间等指向系统记录。失效外链显示“原文不可访问”，提供权限允许的历史片段，不伪造定位。

## 4. 数据源与接入

以下地址于 2026-09-26 核对官方入口、公开文档或官网配置代码；**未使用账号密钥验证数据调用**。完整 URL 与认证方式已列明；工具权限、配额及内容展示权在接入验收中核实。

### 4.1 文本主源与公告兜底

| 数据源 | 地址与调用 | 用途和限制 |
|---|---|---|
| iFinD MCP | [官网与账号配置](https://mcp.51ifind.com/)；新闻公告服务 `https://api-mcp.51ifind.com:8643/ds-mcp-servers/hexin-ifind-ds-news-mcp`；综合服务 `https://api-mcp.51ifind.com:8643/ds-mcp-servers/hexin-ifind-ds-mcp` | 使用账号导出的传输配置及 `Authorization` 原值，不自行添加 Bearer。先 MCP 初始化、`tools/list`，再按返回 Schema `tools/call`；不预设研报/互动权限或编造工具名 |
| 巨潮公告 | [检索入口](https://www.cninfo.com.cn/new/disclosure/stock)；当前仓库适配器使用 `POST http://www.cninfo.com.cn/new/hisAnnouncement/query`；原文由返回的 `adjunctUrl` 与 `http://static.cninfo.com.cn/` 组合 | 网站内部检索端点，非稳定商业 API 承诺。上线验证可用性、HTTPS 支持和访问条款；不可用则停用适配器，不绕过访问限制 |
| 巨潮正式数据服务 | [深证信 API 文档入口](https://webapi.cninfo.com.cn/#/apiDoc) | 正式授权接入的候选入口；具体数据端点以开通产品文档为准，不能把门户地址当数据接口 |
| 原始发布网站 | [上交所公告](https://www.sse.com.cn/disclosure/listedinfo/announcement/)、[深交所公告](https://www.szse.cn/disclosure/listed/notice/index.html)、[证监会](https://www.csrc.gov.cn/) | 原文核验与允许材料的人工导入；本期不承诺自动爬取这些网站 |

iFinD 服务地址和认证配置来自[官网公开配置代码](https://s.thsi.cn/cd/ifind-java-ds-bff-web-container/ifind-mcp-web/assets/index-lGOjm_lC.js)，部署以账号当次导出为准。巨潮端点来自现有适配器 `app/server/service/finance/cninfo.go`，本次仅静态核对。

巨潮表单字段：`pageNum,pageSize,column,plate,tabName=fulltext,stock,searchkey,seDate=起日~止日`；`stock` 值为“证券代码,orgId”，column/plate/orgId 取已核验主体映射，不猜值。按返回总数翻页，保存 `announcementId,announcementTime,adjunctUrl`；时间精度按实际源保留。公告兜底不补齐新闻、研报或传闻。

适配器统一输出 `provider,provider_doc_id,url,publisher,title,source_type,text_or_excerpt,disclosed_at,source_updated_at,precision,rights,request_id`。无可靠日期/原文时保留缺失，不补造。每次记录实际查询条件、分页、截断、覆盖区间和待处理数；语义 Top-K 检索只能标为有限覆盖。

默认公告 5 分钟、新闻 10 分钟轮询；按源串行请求，超时 10 秒，仅网络错误/429/5xx 最多重试 2 次，退避 1 秒、3 秒并服从 Retry-After。连续 5 次失败熔断 5 分钟；认证/权限错误不重试。增量窗口重叠 24 小时，故障后从最后成功游标补抓；未完整处理不推进游标。原文修订另按已知来源检查，不能只靠发布日期增量。

### 4.2 扶摇：标的目录与可选行情

[官网](https://fuyao.aicubes.cn/)｜[在线文档](https://fuyao.aicubes.cn/docs/)｜[官方 REST 契约](https://github.com/HiThink-Tech/Financial-API/blob/main/docs/api/README.md)｜[API Key 管理](https://fuyao.aicubes.cn/admin/)。

Base URL 为 `https://fuyao.aicubes.cn`，请求头 `X-api-key`；HTTP 成功且业务 `code=0` 才算成功，响应 `{code,message,request_id,data}`。密钥只留服务端；null 不补零。扶摇公开能力不包含公告/研报原文，不能作为文本源替代。[能力边界](https://github.com/HiThink-Tech/Financial-API#数据能力与边界)

| 接口（均 GET） | 参数 | 本期用途 |
|---|---|---|
| `/api/meta/tickers/search` | `q` 必填；`asset_type=a-share`；`limit=20` | P0 标的搜索、代码消歧；返回 `data.item[].thscode,name,exchange`，同时保留来源更新时间。不可用时返回已有目录并标陈旧；演示用固定虚构目录。[契约](https://github.com/HiThink-Tech/Financial-API/blob/main/docs/api/meta/tickers-search.md) |
| `/api/a-share/prices/historical` | `thscode,interval=1d,start,end,adjust=forward`；start/end 为毫秒时间戳 | P1 股票日线；返回 `date_ms,close_price` 等，不参与事实核实。[契约](https://github.com/HiThink-Tech/Financial-API/blob/main/docs/api/a-share/prices.md#prices-historical) |
| `/api/a-share-index/prices/historical` | `thscode,interval=1d,start,end`，不传 adjust | P1 基准日线；指数代码经目录核验。[契约](https://github.com/HiThink-Tech/Financial-API/blob/main/docs/api/index/a-share-index.md#prices-historical) |
| `/api/a-share/calendar/trading-days` | 无参数 | P1 交易日；官方范围为近一年至今日，不假定包含未来交易日。[契约](https://github.com/HiThink-Tech/Financial-API/blob/main/docs/api/a-share/calendar-trading-days.md) |

如以后启用行情验证：D0 为披露时仍未收盘的首个交易日；盘后披露取下一交易日，缺精确时间则不算。窗口 D−1 收盘至 D+3 收盘，超额变化 = 个股收益−沪深300收益；交易日、复权和基准数据须齐全，否则显示未完成/不可计算。只描述同期表现，不声称事件造成收益。

## 5. 后端接口契约

以下为**本项目拟建 REST 接口**，统一前缀 `/api/finance/intel/v1`，与供应商接口分开。实现沿用项目后端和鉴权，不另建服务栈。

### 5.1 通用约定

- 真实模式使用现有登录身份；后端从会话取 user_id，不接收客户端指定用户。管理员操作校验角色；私有对象越权按 404 返回。
- 真实页面为 `/app/intel/*`，演示为 `/app/intel/demo/*`；请求显式带 `X-Intel-Mode: live` 或 `demo`。live 只验证现有登录令牌、忽略演示 Cookie；demo 不发送登录令牌，只验证演示 Cookie。demo 请求夹带登录令牌返回 400，不混合身份。
- `POST /demo-sessions` 为匿名限流入口，设置签名 HttpOnly/SameSite Cookie，创建独立 namespace，24 小时后删除。演示会话只可读写本空间、只能用内置样例；真实身份与演示会话不得混用。
- 所有记录带 namespace；真实事件在真实空间共享，关注/通知私有。namespace 从鉴权上下文取得，客户端不能选择其他空间。
- 成功：`{data,error:null,trace_id,meta:{request_id,as_of,coverage,warnings}}`；204 无响应体。列表 data 为 `{items,next_cursor}`；默认 20、最多 100，游标固定快照并有稳定 ID 排序。
- `coverage` 含 `status=complete|limited|unavailable,scope,last_success_at,pending_count,gaps[]`；complete 仅指声明的接入范围。缓存/部分结果返回 200 并标 limited，无可用数据返回 503；空列表不是源故障。
- JSON 时间用 RFC3339 UTC，日期另附 precision；金额为十进制字符串并附 unit/currency；缺失为 null。所有输入校验枚举、长度、标的上限，拒绝未知字段。
- POST 修改请求必须带 `Idempotency-Key`，按用户、空间、路径保存结果 24 小时；同键不同请求返回 409。管理员修改带 `expected_version`，过时返回 409；PUT/DELETE 按资源天然幂等。
- 错误：`{data:null,error:{code,message,retryable,request_id},trace_id,meta:null}`。HTTP 400 参数错误，401 未登录，403 角色不足，404 不存在/不可见，409 版本或幂等冲突，422 不支持类型/引用不合法，429 限流，503 数据源不可用。

### 5.2 用户与查询接口（全部 P0）

| 方法与路径 | 输入 | data / 行为 |
|---|---|---|
| GET `/session` | 由路由决定 `X-Intel-Mode`，不接收 user_id/namespace | `{mode,principal_id,role,namespace_label,expires_at,replay_version,generation,branch,step_index}`；真实模式的演示字段为空。过期401；无演示会话时设置bootstrap Cookie。新窗口刷新先调用本接口 |
| POST `/demo-sessions` | `{fixture_set:"events-v1"}`；先调用session取得bootstrap Cookie | 201 `{session_id,expires_at,namespace_label}`，创建空关注和隔离样例空间；有效会话返回200原会话；每 IP 每小时最多创建10次 |
| GET `/instruments` | `q` 1–50 字，`limit` 1–20 | `{items:[{code,name,exchange}]}`；真实模式只返回核验的 A 股 |
| GET `/watchlist` | 无 | `{items:[{code,name,subscribed_at}]}` |
| PUT `/watchlist/{code}` | 空体 | 200 `{code,subscribed_at}`；幂等关注并默认订阅，重复 PUT 保留原订阅时间 |
| DELETE `/watchlist/{code}` | 无 | 204；取消关注与未发送通知资格，重复删除仍 204 |
| GET `/events` | `code,type,verification,cursor,limit` 均可选 | `{items:EventCard[],next_cursor}`；仅返回关联当前自选的事件，最近变更倒序 |
| GET `/events/{id}` | 可选 `version` | `EventDetail`；不传为当前，传入则返回冻结版本；同空间已知事件可直接访问 |
| GET `/events/{id}/timeline` | `version,cursor,limit` 可选 | `{items:TimelineNode[],next_cursor}`；披露时间轴，含更正、阶段、冲突与信息过期节点 |
| GET `/events/{id}/evidence` | `version,claim_key,grade` 可选；`include_inactive=false,cursor,limit` | `{items:Evidence[],next_cursor}`；可查看失效证据；状态按选定版本 |
| GET `/events/{id}/conflicts` | `version,status=open\|resolved` 可选；`cursor,limit` | `{items:Conflict[],next_cursor}`；按选定版本的冲突状态 |
| GET `/events/{id}/changes` | `version,cursor,limit` 可选 | `{items:Change[],next_cursor}`；按入账版本倒序，仅返回不高于选定版本的变化 |
| GET `/source-revisions/{id}` | 无 | `SourceRevision` 的允许展示片段、定位、原文链接与访问状态，不返回越权全文 |
| GET `/notifications` | `status=unread\|read\|ignored` 可选；`cursor,limit` | `{items:Notification[],next_cursor,unread_count}` |
| PATCH `/notifications/{id}` | `{status:"read"或"ignored"}` | `Notification`；幂等设置，已忽略不能退回 read |
| PUT `/events/{id}/mute` | `{muted:true或false}` | `{event_id,muted,effective_at}`；取消排队消息，解除不补发 |
| GET `/data-status` | 无 | `{providers:[{id,status,last_success_at,pending_count,scope}]}`；不泄露密钥/配置 |
| POST `/replay/actions` | `{action:"step"或"reset",expected_version,branch?}`；branch仅reset接受，main/denial，默认main | `{replay_version,simulated_at,changes[],notification_count,done}`；只允许当前演示空间；step沿用会话分支，原子注入下一条或推进时钟，末尾done=true |

reset 清空本演示空间业务数据并重载样例初态，保留会话；真实模式调用回放返回 403。回放走与真实模式相同的抽取后处理、归并、计算和通知服务；预录抽取结果必须标注模型与校验记录，不能直接写页面结论。

### 5.3 管理与接入接口（全部 P0，仅管理员）

| 方法与路径 | 输入 | data / 行为 |
|---|---|---|
| POST `/admin/ingestion-jobs` | `{provider,codes[],from,to}`，最多 20 标的、31 天 | 202 `{job_id,status:"queued"}`；仅已配置 provider，禁止任意 URL 抓取 |
| GET `/admin/ingestion-jobs/{id}` | 无 | `{status,received,processed,quarantined,cursor,error}`，status 为 queued/running/succeeded/partial/failed |
| POST `/admin/source-revisions` | §4.1 标准材料字段 + `{import_reason}` | 201 `{source_id,revision_id,job_id}`；导入有权使用的片段及来源链接，文本≤100KiB；异步处理，同版重复200返回原ID |
| GET `/admin/review-items` | `kind=merge\|extraction\|conflict` 可选；`cursor,limit` | 待处理记录、候选事件、引用、原因及当前版本 |
| POST `/admin/review-items/{id}/resolve` | `{action:"assign"或"reject"或"replace_extraction",event_id?,evidence?,reason,expected_version}` | assign 归入指定事件；reject 隔离该项；replace_extraction 接受修订的证据集合，仍须引用校验；返回 `{change_ids[],version}` |
| POST `/admin/events/{id}/reassign` | `{evidence_ids[],target_event_id:null或ID,reason,expected_version,target_expected_version?}` | 原子迁移所选证据，null 为新建；重算两边并保留归属审计、旧通知和跳转关系，返回 `{source_event_id,target_event_id,change_ids[]}` |

冲突复核只能修正证据抽取、归属或补入带出处的替代材料，不能无依据直接改核实状态。重分配不得覆盖历史；事件被清空时标为已重归属，并指向新事件。

### 5.4 最小数据对象

| 对象 | 必须字段 |
|---|---|
| EventCard | `event_id,type,title,subjects[{code,role,evidence_ids}],verification,phase,freshness,support_level,event_version,updated_at,open_conflict_count,coverage`；role=subject/counterparty |
| EventDetail | EventCard + `core_claim,summary{facts[],changes[],unknowns[],relations[]},first_sources[],current_snapshot_id,calc_trace,redirect_event_ids[],selected_version,latest_version,is_historical,current_values[],change_id`；summary 每项带 evidence_ids 或 change_id |
| SourceRevision | `source_id,revision_id,provider,publisher,url,title,source_type,content_hash,parser_version,disclosed_at,source_updated_at,fetched_at,precision,rights,quotes[],origin_status,root_links[]` |
| Evidence | `evidence_id,event_id,source_revision_id,claim_key,text,subject_code,target_codes[],occurred_at,precision,is_expected,modality,stance,params[],quote{start,end,text,page},grade,base_weight,weight,status,supersedes[],extraction_run_id`；stance=support/refute/context，status=active/superseded/retracted/quarantined |
| TimelineNode | `node_id,kind,disclosed_at,recorded_at,source_revision_ids[],change_id,evidence_ids[]`；kind=source/update/correction/denial/conflict/expiration/recovery；无披露时间的系统节点按 recorded_at 单列 |
| Conflict | `id,event_id,claim_key,evidence_ids[],status,resolved_by_revision_id,resolved_at`；不同命题分别保存 |
| Change | `change_id,event_id,event_version,kind,before_snapshot_id,after_snapshot_id,diff,trigger_evidence_ids[],recorded_at` |
| Snapshot | `id,event_id,event_version,as_of,rule_version,extraction_run_ids[],source_revision_ids[],coverage,conclusions_by_target,calc_trace,input_hash`；不可覆盖 |
| Notification | `id,change_id,event_id,target_codes[],kind,before,after,status,created_at,read_at,link_version` |
| 用户/执行记录 | `watchlist,subscription,event_mute,outbox,ingestion_job,review_item,extraction_run,audit_log`；均带 namespace，私有记录带 user_id |

来源内偏移为归一化文本 Unicode 码点的 `[start,end)`；params 每项为 `{field,value,unit,currency,period,basis}`。模型原始输出也受 rights 约束，禁止借日志保存未授权全文。规则、解析器或模型升级另建快照；旧快照引用的内容和权重不得原地改写。

## 6. 安全、异常与性能

| 项目 | 固定要求 |
|---|---|
| 数据许可 | 上线前逐源记录可抓取、可保存、可展示范围及依据。研报默认仅许可片段和链接；公开公告也不默认获得无限再分发权。到期内容按授权清理，保留必要哈希和删除审计 |
| 数据异常 | 单源失败标覆盖缺口；全源失败仍可读历史并标更新时间；抽取失败不参与计算。恢复补抓完成前不恢复“覆盖完整” |
| 隔离与内容安全 | 密钥不下发、不入日志；用户不得访问他人私有数据；原文按文本安全渲染，URL 限 http/https；抓取仅管理员配置域名并拦截内网地址/重定向；外部文本不作为执行指令 |
| 可靠性 | 变更与 Outbox 同事务；同事件串行版本；通知至少一次投递、唯一键去重；服务重启后任务恢复，死信可查可重放 |
| 限额 | 用户查询 60 次/分钟、修改 20 次/分钟；管理员抓取同时最多 1 个任务；按 provider 配额限制，超过返回 429 |
| 可观测 | provider 拉取成功率、待处理量、抽取校验失败率、Outbox 积压和通知延迟；trace_id 串起来源→证据→变更→通知；连续 5 次源失败或队列滞留 10 分钟告警 |
| 性能 | 2 vCPU/4GB 应用实例及独立数据库；1万事件、10万证据；50 用户、总10请求/秒，80%列表/20%详情，预热1分钟后测10分钟。P95 列表≤1秒、详情≤1.5秒；单事件200证据重算≤200毫秒 |
| 通知时效 | 从有效材料完成入库处理到通知入箱≤60秒；披露→抓取→处理→通知分别计时，不承诺供应商未索引材料的端到端时延 |
| 兼容与恢复 | Chrome/Edge/Safari 当前及前一主版本、375px 可操作；真实环境每日备份，发布前演练恢复与版本回退；演示空间按24小时 TTL 清理 |

## 7. 验收数据与测试

### 7.1 固定回放样例 events-v1

以下均为虚构材料，时区北京时间，用户名 U 预先关注 A/B；核心命题为“A 正在开展收购 B 的该项安排”。每行是一条完整的最小原文片段，日期时间同时作为可核验披露时间；来源编号为测试定位，非真实外链。

| 步骤 | 披露时点与原文 | 预期当前结果 | 累计通知 |
|---|---|---|---|
| S1 | 9月1日09:00，社区 M1：“网传A正在洽谈收购B，金额10亿元，尚无确认。” | unverified/unknown/fresh；传闻权重0.01、支持度低；金额标未核实 | 1 |
| S2 | 9月1日10:00，M2：“转引M1：网传A收购B，金额10亿元。” | 同源折叠；业务版本与支持度不变 | 1 |
| S3 | 9月10日09:00，A公告 N1：“公司正在推进收购B，拟交易金额10亿元，尚未签署协议。本公告回应9月1日所传同一交易。” | 同一事件；confirmed/proposed/fresh；金额10亿元；权重1.0、支持度高 | 2 |
| S4 | 9月11日09:00，A更正 N2：“更正N1：同一交易拟金额应为8亿元，原10亿元作废，其余不变。” | 同级更正；当前8亿元，旧值留历史；状态和支持度不变；必须通知 | 3 |
| S5 | 9月12日09:00，B公告 N3：“就N1、N2所指同一收购事项，双方确认拟金额仍为10亿元。” | 金额冲突open；核心交易仍confirmed；支持度封顶中，8/10并列不择一 | 4 |
| S6 | 9月13日09:00，B更正 N4：“更正N3：同一交易拟金额为8亿元，撤回10亿元表述。” | 金额冲突resolved；当前8亿元；支持度恢复高 | 5 |
| S7 | 模拟到10月13日09:00；所选范围巡检与处理完整且无新增 | confirmed/proposed/expired；事实保留、支持度不衰减；标信息过期 | 6 |
| S8 | 10月14日09:00，A公告 N5：“N1所指收购现已终止，不再推进。” | 同一事件；confirmed/terminated/fresh；一次变更同时说明终止和信息恢复 | 7 |

独立分支 D：复制 S1 后注入 9月2日 A公告“公司不存在M1所述收购B的安排”，结果 denied/unknown/fresh，累计2条通知；仅否认金额时不得得到 denied。重置后分支从独立空间运行，不串入主路径。

实现需把上述原文、引用跨度、预期事件归属、状态、权重、有效值、冲突和通知数落为版本化 fixture；权重采用十进制定点计算。人工标注金标准不得从待测引擎自动生成。

### 7.2 放行标准

| ID | 测试与通过条件 |
|---|---|
| AC01 用户主链路 | 与观点/策略同级入口，新窗口打开且原窗口草稿与任务不变；添加/取消关注、筛选、详情、原文、历史、通知、静音均可操作；不依赖 P1 |
| AC02 身份 | S1→S3跨7天仍同事件；只改金额不拆链；相同双方不同交易不误合并；多候选进入待处理；纠错后历史可查 |
| AC03 分级与状态 | 同公告内事实/观点/预测/传闻分开；首条正式公告即可确认；部分否认不否认整体；终止不等于此前事实不存在 |
| AC04 更正与冲突 | S4同级更正生效；S5/S6冲突产生及解除；不同期间/单位归一后不误报；不解决无关命题冲突 |
| AC05 支持度 | 80条社区传闻仍为0.01/低；同源不同命题不丢失；A/B分别计算；证据排序不影响结果；缺依据显示待评估 |
| AC06 时间 | 未知披露时间不当首发、不刷新计时；旧闻晚到不翻案；14/30天边界正确；扫描幂等；源故障/积压暂停过期判定 |
| AC07 追溯 | 同URL修订可见且旧版保留；数字存在但字段/主体错配不能通过；原文、计算、元数据三类出处正确；外链失效诚实降级 |
| AC08 通知 | 回放累计条数完全符合表格；同档位更正通知；并发/崩溃重放不丢不重；关注/静音竞态按提交顺序判定；多标的一条列全 |
| AC09 数据与AI故障 | 认证失败、429、超时、空结果、缺字段、模型错误分别得到规定提示；无证据不生成结论，恢复后可补抓重放 |
| AC10 安全与权限 | 跨用户、越权管理员、真实空间回放均拒绝；恶意原文/链接不执行；前端与日志无密钥；保存与展示范围符合来源许可 |
| AC11 实际接入 | 用已获授权账号保存脱敏 tools/list 与Schema、请求时间、request_id和样例；至少3个真实标的核验公告及一种其他文本来源。回放不能替代该项；缺权限则明确只交付演示模式 |
| AC12 性能与交付 | 达到§6负载指标；浏览器和375px冒烟通过；URL外部可操作；README、视频、AI记录、测试结果齐全 |
| AC13 用户理解 | 5名目标用户各对一个未见样例回答五个问题；从详情首屏可交互开始计时，至少4人60秒内全部正确；全部用户不得把传闻当事实或把旧值当当前值 |

所有选定测试样本必须通过，无未关闭的事实错误、权限隔离或漏通知阻断项；“样本全部通过”不外推为任意输入100%可靠。测试报告分别列静态检查、规则测试、接口故障、浏览器、授权数据及用户验证结果。

## 8. 交付说明

README 必须写明目标用户、支持范围、数据入口与实际权限、AI角色、启动方法、配置变量、演示入口、未实现项。服务端配置至少包括 `IFIND_MCP_URL,IFIND_AUTHORIZATION,FUYAO_API_KEY` 及已启用 provider 清单；仓库只放变量名与无密钥样例。

接口实现时同步导出 OpenAPI 和 fixture 测试说明。发布材料附部署版本、规则版本、数据能力清单、测试结果与恢复方案。当前文档可供研发实施和 QA 编写用例；真实用户上线必须通过 AC01–AC13，未通过的授权数据或用户验证项不得以演示替代。
