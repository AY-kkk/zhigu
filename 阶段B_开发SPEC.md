# 知股一期 · 阶段 B 开发 SPEC

版本：B-1.1 · 2026-09-18（模型接入增补）
状态：**待签收的开发契约；不是完成报告。阶段 A 修复尚未在本轮复审，阶段 B 尚未放行。**

目标：普通用户粘贴一个 A 股单公司投资观点，经确认后，通过真实模型、授权数据和两个独立研究 Agent，获得可核查的证据、反证与未知项。

## 1. 使用方式与范围

主开发入口是本文。配套只有两份：

- [验收与接入清单](handoff/stage-b/验收与接入清单.md)：准入条件、测试矩阵、证据包和签收记录。
- [给 Cursor 的开发指令](handoff/stage-b/CURSOR开发指令.md)：执行顺序、暂停条件与汇报格式。

继承 [PRD v2](金融C端Agent_MVP_PRD.md)、[面客架构 v2](金融C端Agent_面客产品架构_v2.md)、[总 SPEC](开发交付_SPEC.md)。本文只细化阶段 B；产品/架构冲突提交用户确认，不自行覆盖。下文的 B-* 是需求 ID，验收附件逐项对应。

用户本次已明确：**阶段 B 后端必须允许 OpenAI-compatible Chat Completions API 与 Responses API 两种模型接入方式**。本版覆盖此前“只允许 Chat Completions”的限制；其他待签收事项和阶段 A 复审状态不因此改变。这是接口协议选择，不限定只能使用 OpenAI 官方供应商，也不保证任意兼容厂商都具备完整能力。

本轮只编写文档，不修改应用、契约 Schema 或测试代码，不认定阶段 A 已通过。[阶段 A 审查](reviews/阶段A交付审查_2026-09-17.md)是历史问题来源，不代表当前修复仍然失败。

### 1.1 阶段 B 的边界（B-01）

| 必须交付 | 明确不做 |
|---|---|
| 粘贴观点→解析→确认股票/期限/主张→双路研究→报告→证据抽屉 | 自选股持续追踪、用户画像、长期偏好记忆、主动通知 |
| 真正的 GoSaaS 业务集成、Eino DAG、DeerFlow Harness 调用；Chat Completions/Responses 两种底层模型适配 | 第二个用户系统、第二个 Supervisor、DeerFlow 通用聊天前端 |
| 一个已验证模型配置；一种授权财务数据与一种授权正文能力，可来自同一供应商 | 多厂商自动容灾、任意网页浏览、任意 MCP、交易、收益承诺 |
| 历史、取消、删除、报告内追问；后台配置与受控启用 | 量化策略回测、证券推荐榜、全市场覆盖、批量 PDF/OCR |
| 私有环境可复现的技术样机 | 公网开放、合规结论、大规模吞吐或“零幻觉”承诺 |

采用“分关口的纵向闭环”：每关都有独立负例和证据，全部通过才完成 B。不采用最后集中联调，也不先建通用 Agent 平台。一天是时间盒，不是省略闸门的理由。

### 1.2 一个可核查的用户案例

用户输入：“某公司利润改善，未来一年经营前景值得看好。”系统要求用户选择覆盖目录中的真实证券，确认绝对起止日期，并编辑 1–6 个主张。

支持者查利润变化及解释；挑战者查现金流、一次性损益和经营风险，不读取支持者的论证。两者均可得到“不足”，不能强行编造相反观点。报告区分“已披露事实”“基于事实的推断”“待验证假设”，说明哪些新证据会改变判断，不把利润增长直接等同于股价必涨。

离线数值测试可以使用明确标记的虚构样本：上年归母净利润 100、本年 120，增长率应为 20%。该样本不能出现在 live 数据源或冒充真实证券表现。

### 1.3 相对总 SPEC 的新增决定，签收时需明确

保留产品、三框架分工和既有硬预算；新增控制协议版本 header、以可信记录句柄登记证据、私有报告检查记录。将关键金融质量与失败路径验收前移到 B，并提出“3只证券、30例固定集、20次live运行”的样机验收线。这些是本版待确认的收紧要求，不是旧版本已经批准或系统已经达成的指标；不要求 B 完成 C 的完整运营与公网发布建设。

## 2. 先决条件与六个交付关口（B-02）

所有关口初始状态均为 `not_started`，不得从旧文档或开发者描述继承 passed。允许的状态：`not_started / in_progress / blocked / ready_for_review / accepted`。开发者只能提请复核，不能自行把 `ready_for_review` 改成最终签收。

| 关口 | 工作与交付物 | 放行条件 |
|---|---|---|
| G0 基线 | 重新审查 A 修复；记录 Git SHA+工作树差异摘要、启动入口、权限/授权确认、依赖锁 | A 阻断项有独立回归证据；只有一个真实 GoSaaS 业务入口，无独立 finance 用户体系 |
| G1 接入 | 填写附件 D-01～D-07；确定模型、数据、正文、覆盖目录、费用限额 | 凭据可用、权限明确、支持能力有样本；未明确项标 blocked，不猜供应商 |
| G2 执行器 | Go 模型代理与真实 DeerFlow 最小调用、三工具白名单、双任务隔离 | 实际 Harness 产生工具调用并处理工具结果；两任务凭据不串用；不是仅构造对象 |
| G3 证据 | 真数据 connector、正文定位、证据登记、确定性计算 | 能从真实来源到不可变记录再到 evidence_id；篡改指标/来源/时间全部拒绝 |
| G4 编排 | Eino 双分支、合成/复核/最多一次修复、预算/取消/故障 | 有真正并发和发布闸门证据，单路失败不会伪装完整结论 |
| G5 产品 | 真实用户 UI 全链路、后台切换、质量集、live 样本及交付包 | 附件必需测试无失败/跳过；真实调用证据齐全；用户或指定复核者签收 |

顺序 G0→G1→G2→G3→G4→G5。G0 未放行时仅可做隔离的兼容性探索，不得将其宣称为 B 集成完成；真实付费调用仍须先满足 G1 费用授权。任一关口阻塞时交回原因、影响和所需决定，不能用 fixture 顶替。

## 3. 架构：一个业务控制面、一个总编排、两个研究 Loop（B-03）

```text
客户 Vue / 管理 Vue
         │ 只访问 Go 的公开 API
         ▼
GoSaaS：身份与权限 / 业务状态 / 配置版本 / 预算账本 / 证据 / 发布
         │
         ├─ Eino：Freeze → [支持者 ∥ 挑战者] → 校验 → 合成 → 复核 → 发布
         │                    │             最多修复一次，不递归调度
         │                    ▼
         │          Python DeerFlow Harness × 2
         │          独立 task/thread/messages，有界工具与模型循环
         │                    │ 仅允许携带任务能力令牌调用 Go
         ├─ 模型代理 ◄─────────┤ → 已启用模型供应商
         └─ 数据/计算出口 ◄────┘ → 已授权财务与公告正文来源
                  │
             PostgreSQL（Go 为唯一业务写入者）
```

### 3.1 代码责任与扩展边界

以下是目标归属，不表示文件已实现。已有同职责模块原位扩展；迁移/重命名必须列出旧→新映射，不新建平行系统。

| 所属目录 | 责任 | 禁止 |
|---|---|---|
| `app/server/router`、`api`、`model` | GoSaaS 注册入口、真实账号、owner 校验、数据库迁移 | 复制 GoSaaS 到目录即称集成；两套登录与用户表 |
| `app/server/service/finance/workflow.go`、`nodes.go` | Eino 编排、解析、合成、复核 | 在 Python 再建全局研究调度器 |
| 同目录 `config_service.go`、`model_proxy.go`、`budget_service.go`、`models/{chat_completions,responses}.go` | 冻结版本、统一模型网关下的双协议适配、原子账本 | Go/Python 各配供应商 Key；信任模型自报调用数 |
| 同目录 `providers/`、`evidence_service.go`、`report_service.go` | connector、可信记录、计算、发布 | Python 直接写业务库；LLM 文本登记为供应商原文 |
| `app/research-service/app/deerflow_adapter.py`、`policies.py`、`jobs.py` | 真实 Harness 装配、任务隔离、生命周期 | 自写 Loop 后冠名 DeerFlow；吞异常返回假成功 |
| `app/research-service/app/tools/` | 三个工具的薄适配，只请求 Go | 外部数据 Key、任意 SQL、网络或 shell |
| `app/web/src/` | 客户/管理员布局、状态与可信证据展示 | 浏览器直连 Python；展示未发布草稿 |
| `app/tests/`、各模块测试目录 | 真 PostgreSQL 集成、契约、端到端和质量测试 | 以 fixture E2E 替代 live E2E |

G0 必须确定 `app/server` 与 `app/web` 是真正的 GoSaaS 扩展入口。当前目录中若同时存在 `app/gosaas`，不能把它当第二个运行系统，也不能未经确认删除现有代码。先完成入口与身份迁移对齐，再进入 B。

保留单 Go 实例、单 Python 服务进程、PostgreSQL、Python 执行状态 SQLite WAL。SQLite 只存执行态，不存供应商 Key；跨用户共享 checkpoint 禁止。多实例队列、向量库、Redis 非 B 的启动依赖。

### 3.2 依赖边界（B-04）

运行依赖以 `app/server/go.mod`、`app/research-service/uv.lock`、`app/web/package-lock.json` 为准，不追 latest 标签。`references/` 下的源码检出不随公开仓库分发。

确需对可选 Harness 做最小补丁时，放应用内独立、可追溯的依赖补丁目录，并保留许可与兼容测试。源码阅读不等于运行依赖已锁定。

## 4. 用户、管理员与配置契约（B-05～B-08）

### 4.1 用户链路

沿用总 SPEC 的 `/api/finance/*` 路由和响应 envelope，不另造入口。

1. 输入 20–2,000 字 → `claims/parse` 创建草稿；解析失败不启动研究，不猜证券。
2. 人工确认覆盖内证券、绝对期限、as_of、1–6 条主张；未确认禁止创建 run。默认 as_of 为服务端本次研究时点；历史精确研究默认关闭，只有具备 PIT 能力的数据配置才能开放。
3. `research` 使用 `Idempotency-Key`，校验 owner/revision，冻结上下文后入队。重复同键同请求返回原 run，异体返回 409。
4. 每 2 秒轮询；显示实际阶段、数据截止时间、live/fixture 模式，不显示猜测进度百分比。
5. 只渲染发布后的报告；`incomplete` 明示“研究未完成”，结论为空。证据抽屉展示原文位置、来源、时间、指标口径、计算依据。
6. 历史/取消/删除沿用原 API。追问仅使用该报告已经获准展示的证据，最多 3 轮，不新增检索、不改变报告版本。新事实请求提示主动重新研究。

### 4.2 后台最低可用设置

`/admin/ai-settings` 必须提供：

| 设置 | 必填内容 | 保存/测试/启用行为 |
|---|---|---|
| 模型 | 名称、protocol、允许列表内 base_url、model、Key、输入/输出上限、超时、用途映射 | protocol 必选 `openai_chat_completions` 或 `openai_responses`；保存不可变版本；Key 写入加密，仅返回 has_key |
| 模型测试 | Go 结构化输出、Python 实际工具调用、错误/超时处理、无证据输出 | 返回 test_id、分项结果、配置 digest、调用账本；不是收到 200 就全部通过 |
| 数据源 | connector 类型、Key、覆盖目录、财务/正文能力、PIT 能力、使用与展示权限记录 | 测一次真实指标、一次正文定位；缺正文不能启用为完整研究配置 |
| 策略 | 每用户/全局并发、模型/工具次数、Token/费用限额、允许来源 | 只能降低硬上限；变更创建版本 |
| 运行审计 | run/task/request ID、模式、配置版本、耗时、用量、错误 | 默认脱敏；不得泄露观点原文、Prompt、Key |

保存→测试→启用是三个动作。只有完全相同 digest 的必需分项通过才允许启用。protocol、Key、model、base_url 或相关策略变更使旧测试无效。Go/Python 两条调用路径都要按所选协议测试；后台显示“协议支持（实现）/当前配置真实测试结果”，不能将两者混为一个绿色状态。测试使用隔离的管理员任务上下文和独立小额度，不使用某个消费者的 token。

base_url 保存供应商提供的 API 根路径，不包含末尾 `/chat/completions` 或 `/responses`；例如 OpenAI 官方根路径为 `https://api.openai.com/v1`。适配器仅追加对应资源路径一次；第三方如使用不同根路径，由管理员按官方文档填写，禁止盲目追加第二个 `/v1`。完整请求地址在后台脱敏预览，不能让模型改写。已有配置只有能明确证明是 Chat Completions 时才迁移赋值；未知旧配置禁用待确认，不猜协议。

已启用配置切换只影响新 run；运行中任务继续使用已冻结版本和对应凭据版本。紧急撤销某 Key 是独立停用动作：使受影响运行显式失败/取消，不悄悄换模型续跑。草稿记录实际解析版本，不能把此前解析追记为新版本。

### 4.3 模式与权限

- `fixture/live` 在部署配置确定并由 Go 下发，浏览器和模型不能选择。live 缺配置返回 503 `CONFIG_NOT_READY`，绝不回落 fixture。
- 录制回放仅属测试，不新增可被用户选择的生产模式。是否实际访问上游由证据包声明，不能仅依据 `mode=live`。
- JWT/Casbin 管入口，每次读取/取消/删除/证据/追问仍检查真实 GoSaaS owner。非本人资源统一 404，admin 页面权限在后端复查。

## 5. 内部协议、可信记录与并发安全（B-09～B-13）

### 5.1 版本和上下文

保留四份 [v1.0 JSON Schema](handoff/contracts/)的 task/result/evidence/report 字段，禁止向 `additionalProperties=false` 对象偷偷加字段。B 新增的内部控制协议统一要求 header `X-Zhigu-Contract-Version: 2`；这里的 2 是控制协议版本，不是上述 Schema 版本。Go/Python 同批升级，live 拒绝缺失/不匹配版本（409 `CONTRACT_VERSION_MISMATCH`），无静默降级。

新增 registration/校验账本等对象在开发时补独立 Schema 与双端契约测试，落 `app/contracts/internal-v2/`；不得事后让测试迁就实现。本文是该新增协议的设计定义，Schema 文件尚未生成。

Go→Python 仍为 `/internal/research/tasks` 的 POST/GET/cancel；服务身份与任务临时 token 分开传输。临时 token 只在受保护 header 中注入，不写请求正文、Prompt、日志、SQLite 明文。服务身份仅用于调度与查询，不能替代 task token 调金融/模型出口。

Go 为每个 task 保存不可变 `ExecutionContext`：run_id、task_id、role、owner、配置/数据/Prompt/策略版本、mode、deadline、execution_epoch、允许能力；Python 不能自报覆盖。token 绑定该上下文和到期时间（不超过 deadline），Go 检查撤销状态及当前 epoch。task/thread 一一对应，thread_id 不跨 run 复用。

### 5.2 Python→Go 的规范输入

每个请求都验证任务归属、用途、期限、撤销、租约 epoch；在真实副作用发生前再次检查。所有 JSON 十进制数值用字符串；时间 RFC3339 UTC。

| 路由 | B 的准确约定 |
|---|---|
| `POST /internal/llm/v1/chat/completions` | Chat Completions 内部入口，stream=false，固定 model 别名；仅接受冻结 protocol=openai_chat_completions 的上下文 |
| `POST /internal/llm/v1/responses` | Responses 内部入口，stream=false、background=false、store=false；仅接受冻结 protocol=openai_responses 的上下文；只允许本项目 function tools |
| `POST /internal/finance/tool-grants` | `{request_id,tool_name,args_hash}`；Go 原子预占一次工具额度，绑定 task/epoch；返回 grant_id、expires_at |
| `POST /internal/finance/data-query` | `{grant_id,operation,params}`；operation 仅 financials/filings，与 tool 一致；Go 重新计算规范化参数 hash。成功返回 `records:[{record_id,record_hash,record}]` 和数据质量信息 |
| `POST /internal/finance/evidence` | **B 改为 `{grant_id,record_ids:[...]}`，不再接受 Python 提交完整 records。** Go 从该 grant 的可信记录创建 evidence，返回 evidence_ids。旧 records 写法在 live 返回 400 |
| `POST /internal/finance/calculate` | `{grant_id,operation,inputs}`；输入见下文三工具约束，Go 复算并存 calculation_id，返回结果、公式、精度及基础 evidence_ids |
| `POST /internal/finance/tool-grants/:id/complete` | `{status,output_hash}`；与 Go 实际执行记录核对，重复幂等；Python 上报不能把 unknown 改成虚假 succeeded，不能退款次数 |
| `GET /internal/finance/data-source-profile` | 只返回当前任务冻结的覆盖、指标、来源规则和能力；不返回 Key/任意 URL 执行能力 |

两个模型入口共用鉴权、预算、幂等、撤销、deadline 与审计层，均要求 `X-Request-ID`；真实 endpoint/key/model 由冻结配置解析。入口协议与冻结配置不符返回 409 `MODEL_PROTOCOL_MISMATCH`，不自动改路由/重试另一协议。Go 在 dispatch header `X-Zhigu-Model-Protocol` 传给 Python 非敏感协议枚举；Python 验证它与任务绑定的模型配置一致后选择任务级 adapter。该字段不添加到旧 task Schema，不能由浏览器覆盖。

`args_hash` 使用规范 JSON（RFC 8785）UTF-8 SHA-256；各工具参数对象不得包含额外字段。Go 按工具 Schema 校验后计算，同一规范化规则在 Go/Python 有共同测试向量。grant 绑定一个操作：数据查询或计算真实执行至多一次，注册/核销不再次访问上游、不重复收费。重试发生未知结果时不可通过换 grant 自动重放。

`record_id` 是 Go 保存的可信 connector 结果句柄，不是 LLM 自造。`record_hash` 覆盖完整规范化记录：证券、来源、URL、locator、正文、指标全部字段、披露/可获知时间、数据版本；原 `content_hash` 继续表示保存原文 hash，不能代替完整记录校验。其他 task 的句柄不可直接登记；两角色可读同一公开来源，但各自通过工具取得授权关联。

正文规范化统一为 UTF-8、Unicode NFC、CRLF 转 LF，不删除段落/单位或重写数值；hash 针对规范化后实际保存的内容。新增 `finance_provider_records` 由 Go 写：id、run_id、task_id、grant_id、record_hash、normalized_record、retrieved_at、expires_at；同 grant+record_hash 唯一，归属外键与删除清理必测。登记操作从该表生成 v1 evidence，不让 Python 改写可信字段。

### 5.3 幂等与错误

- task_id+payload hash 相同只执行一次，异体 409。POST 超时先查同 task_id，不新建任务 ID。
- 模型 request_id 的缓存键必须绑定 task/purpose/protocol/config_version/body hash；两入口共用唯一 request_id 账本，跨入口复用 ID 是冲突而非新执行。已完成可复用；处理中/结果未知不再出站；异体 409。明确的新尝试用新 ID，重新占额度。
- 401 无/无效凭据；403 scope/撤销/跨任务；409 版本、幂等、epoch 冲突；422 不支持的参数/数据口径；429 预算；503 依赖不可用；504 到期。金融内部 API 使用总 SPEC envelope；模型代理保持兼容错误结构。
- 最低错误码：`CONFIG_NOT_READY`、`CONTRACT_VERSION_MISMATCH`、`TASK_SCOPE_DENIED`、`EXECUTION_REVOKED`、`IDEMPOTENCY_CONFLICT`、`BUDGET_EXHAUSTED`、`DEADLINE_EXCEEDED`、`DATA_UNAVAILABLE`、`EVIDENCE_REJECTED`、`WORKER_RESTARTED`。
- 任意异常不得降成空内容成功；日志记录脱敏错误和 trace，不复制 Key/完整模型输入。

## 6. 真实模型与有界 DeerFlow（B-14～B-17）

### 6.1 双协议模型适配与兼容性关卡

**两种底层接入均为 B 的正式实现范围，不是 Responses 留到二期。**一个模型配置版本选择一种协议；parser/synthesizer/verifier/research 可以绑定不同配置版本，也可以共用一个。协议随模型配置冻结，运行中不自动切换；“支持双协议”不要求同一供应商或同一 model 同时支持两者。

技术入口固定为 `Go 统一模型网关 → ChatCompletionsAdapter / ResponsesAdapter → 对应上游`。Go 的 Eino 节点经同一网关服务调用；Python 的任务级模型 adapter 经对应内部 HTTP 入口调用。两个 adapter 只处理协议编码/解码，不另建 Agent Loop、工具执行器、预算账本或账号。上游 Responses 不经过外部 Chat 转发代理冒充原生接入。

官方接口的必要差异如下；不能仅替换请求 URL。以下字段映射依据 [OpenAI 官方迁移文档](https://developers.openai.com/api/docs/guides/migrate-to-responses)，第三方兼容性仍须单独实测。

| 项 | Chat Completions | Responses |
|---|---|---|
| 上游资源 | POST `/v1/chat/completions` | POST `/v1/responses`（表中为官方路径；第三方根路径按配置） |
| 输入/正文 | messages；解析 choices 中的 message | input Items，必要时 instructions；按类型遍历 output，不假设 output[0] 就是文本 |
| 工具定义/结果回传 | 嵌套 function 定义；tool_calls 与 tool_call_id | function 定义字段平铺；function_call 与 function_call_output，通过 call_id 配对 |
| 结构化输出 | response_format | text.format |
| 延续上下文 | 显式消息历史 | 本项目选择显式 Items 回放；不启用供应商会话托管 |

工具返回值必须对应原调用 ID；一次响应含多个工具调用时逐个校验/计次。工具调用与正文是不同内容，不能把函数参数展示成报告。具体格式参见 [官方 Function calling 文档](https://developers.openai.com/api/docs/guides/function-calling)。

应用内部结果统一为文本片段、工具请求列表、完成/拒绝/截断/错误状态、usage、上游标识与私有 continuation 信息。Responses adapter 必须保留下一轮所需的原始 Items（包括供应商要求的 reasoning/encrypted continuation 项），不能降成纯文本再丢弃；它们只在当前 task 的受保护短期上下文中回放，不进入报告、常规日志或长期记忆。不支持必要上下文延续的模型配置测试失败，不能用第二次请求“看起来有文本”判通过。

B 的两协议都只做非流式、文本与三个自定义 function tools；显式请求 store=false，Responses 同时禁止 background=true、previous_response_id、conversation 和内置 web/file/computer/MCP 工具。无状态模式不等于供应商零日志/零留存承诺，D-02 仍需核实供应商数据政策。Responses 的拒绝、incomplete/failed 或 Chat 的长度截断等状态均不得视为完整成功；输出仍经过金融发布闸门。

Token、费用和上下文重放必须统一核算：适配器将各协议实际 usage 归一到已有账本，保留协议专属明细，reasoning/cached 分类不能漏算或与总数重复相加。输出硬限额包含模型消耗的推理输出预算，不仅是用户可见文字。每次回放的完整输入都重新预占，usage 缺失仍为 unknown；拒绝以字符数冒充实耗。模型参数使用逐协议/逐配置的允许列表，不把 Chat 的格式/Token/采样字段原样转发给 Responses；不支持的参数在测试阶段显式失败。

配置测试必须按所选协议验证文本、结构化 JSON、实际工具往返、上下文延续、usage、错误/拒绝/截断处理。“兼容”标签或单次 HTTP 200 都不是结论。两种 adapter 的受控契约与真实框架集成测试必须全部通过；live 测试覆盖当前准备启用的协议配置，未提供凭据的另一协议记为“实现已测、live未验证”，不能宣传该路线已实测可用，也不能借此把另一 adapter 留成占位。

固定 DeerFlow 源码中的 `DeerFlowClient` 构造不等于执行：Agent 在首次使用时延迟装配，工具/中间件还可能在装配阶段追加。因此 G2 必须证明真实 invoke 路径进入 Harness，并检查**最终交给模型的工具 Schema 与实际中间件**，不能只检查 import、构造函数或初始 `_get_tools()`。

适配层必须提供每 task 独立模型客户端和工具闭包，将模型连接到 Go 代理。不得假定上游构造函数支持不存在的 `token` 参数；先写兼容测试，再选择固定源码已存在的装配入口。必要最小依赖补丁要先报告原因/范围；不能修改全局 env/YAML、全局 monkeypatch 或共享带凭据客户端来绕过注入限制。

### 6.2 研究循环约束

- 固定角色 supporter/challenger；中立输入相同，各自私有 messages。先支持/挑战哪条主张、下一步查什么由角色模型决定，不预写两套固定答案。
- 最终允许工具恰为 `get_financials`、`search_filings`、`calculate_metric`。禁止默认内置工具、shell、filesystem、任意 HTTP、MCP、递归 subagent、动态 skills、长期 memory、计划调度工具。
- 无宿主工作目录挂载；Python 网络出口只允许 Go 内部 API。仅删工具名不等于进程沙箱，部署约束也要验收。
- 从中间件、checkpointer 到 tool callable 都需隔离。checkpoint 禁用或按 task 分区且受删除清理；不能跨请求加载默认全局历史。
- 在受控工具调用测试中，至少完成“模型提出工具调用→Go 工具结果→模型利用结果终止”一次；再用真实供应商重跑。强制 tool_choice 可用于能力测试，但不能冒充自主研究质量测试。
- 角色最多 4 次模型、6 次工具；达到上限立即返回类型化结果，不靠 Prompt 说“请停止”。角色执行只能写自己的 result，不能发布用户报告。

### 6.3 三个工具的输入与裁剪

| 工具 | 模型可填 | 服务端固定 / 输出限制 |
|---|---|---|
| get_financials | metrics（目录枚举，1–3 个）、periods（目录内期末日，1–2 个） | 证券、as_of、版本固定；返回原始值/单位/期间/value_type 和 evidence_ids |
| search_filings | query（1–300 字）、limit（1–5） | 同证券、来源白名单；最多 5 个带页段位置的正文片段；不接受 URL/SQL/路径 |
| calculate_metric | operation=growth_rate/ratio/difference，inputs（恰好两个 evidence_id+metric+period） | 同 task/run 授权的已登记指标；输出 calculation_id/Decimal 结果/公式/单位/精度 |

来源 connector 每次逻辑查询的 HTTP 扇出也要有界：财务最多 1 个供应商请求；正文最多 1 次搜索+5 次正文读取，不分页爬全站；全部在单工具 15 秒内。实际 HTTP 次数另计账/费用，不能把工具次数当供应商账单次数。已返回完整正文时不重复抓取。

上下文构造先按主张相关性选择证据，再按段落裁剪，保留来源与 locator；禁止截断 JSON/表头来凑 Token。单请求输入/输出不能超过预算；超出时减少材料或返回不足，不静默提高限额。

## 7. 数据、RAG 与报告发布（B-18～B-24）

### 7.1 最小数据能力

G1 冻结至少 3 只 A 股、覆盖至少 2 个行业的演示目录；证券名称由真实供应商覆盖决定，不虚构。每只至少有两个已披露、同口径年度的营业收入、归母净利润、经营活动现金流净额，以及可核查正文片段。没有某指标不填 0；目录外请求拒绝。

财务 adapter 提供 instrument catalog、financials 查询；正文 adapter 提供 filings 搜索/读取。供应商字段→内部字段映射、单位转换、公告时间和修订口径必须形成测试样本。禁止让 LLM 直接猜接口 URL 或供应商字段。

最小财务字典固定为 `revenue`（营业收入）、`net_income_parent`（归母净利润）、`operating_cash_flow_net`（经营活动现金流净额）。原始币种/单位先保存，B 的这三个规范值统一为 `CNY` 元的十进制字符串，不能只改单位标签不缩放数值。`normalized_record` 包含可映射到 v1 evidence 的来源、文本、指标、时间字段，另有私有 basis：原始值/单位、转换因子、年度起止、合并/母公司口径、修订标识及 available_at 的依据。缺少可比口径就不能计算同比；这些私有字段不直接塞进旧 evidence Schema。

B 默认仅做当前研究时点的证据分析，不保证历史回测。证据必须满足 published_at、available_at ≤ as_of。`available_at` 必须有供应商/原始披露/已存快照等可靠依据；不能一律复制 published_at，也不能将今天修订值伪装成过去版本。缺可靠时间或 PIT 版本则排除历史判断，返回能力不足。

RAG 最小方案是授权正文检索→自然小节/段落片段→证据登记；无需向量库。每段带文档 ID、标题层级、原 URL、页/段 locator。完整表头和单位随数值保留，财务关键数字优先走结构化数据。若选定来源只有 PDF，G1 必须明确其正文提取方式与成本；扫描件/OCR 不得临时偷偷扩大 B 范围。

### 7.2 确定性计算

`growth_rate(current, previous)=(current-previous)/previous×100`，单位 percent；B 对 previous≤0 返回不足并说明不适合常规增速，负转正不输出误导百分比。`difference(a,b)=a-b` 要求同单位与可比口径；`ratio(numerator,denominator)=numerator/denominator` 要求明确同期间与合法单位组合，分母为 0 拒绝。

用 Decimal，不用二进制浮点。中间结果至少 18 位有效数字；计算表保存未作展示舍入的结果与精度策略，展示按 HALF_UP 保留 2 位小数，货币缩放有明确单位。公式、输入证据、期间、actual/estimate、会计口径与转换记录可回查。测试至少覆盖万元/元、不同年度、预测冒充实际、负基数、零分母与舍入边界。

### 7.3 发布流程与数值绑定

角色结果→Go 校验登记与权限→合成候选→程序校验→语义复核→必要时最多一次修复并完整重验→发布事务。Eino 是唯一调度者；修复在有界节点内进行，不建立自由循环。

Go 增加私有 `finance_report_checks` 校验记录，不改变 v1.0 公开报告字段。每个记录含 report候选hash、JSON pointer、claim_type、evidence_ids、numeric_bindings、规则版本、程序结果、语义结果、失败原因。numeric binding 指向 evidence_id+metric+period 或 calculation_id，注明展示数值与单位；不接受“模型说已校验”。

该表还必须关联 run_id、候选报告版本、校验尝试序号和 created_at；同候选hash+pointer+尝试唯一。检查记录、可信 provider records 与相应索引用显式增量迁移创建，不能运行时 AutoMigrate 代替迁移交付。修改候选后旧检查自动失效；发布事务只认完全匹配的候选与通过记录。

所有对外文本字段，包括 summary、support、challenge、改变判断条件、unknowns 中的事实性陈述，都必须经过检查；summary 只能总结已通过的条目，不新增事实。重大数字没有绑定就移除/转为未知，不用“仅供参考”保留。假设清楚标注，未来情景值不能伪装实际披露值。原报告 Schema 的数组/字符串字段保持原状，绑定信息保存在上述私有校验记录。

程序检查：结构、同 run 授权、真实来源/完整记录、截止时间、期间/单位/实际预测口径、数值计算、候选hash一致。语义检查：证据是否支持陈述、是否过度外推、是否遗漏已发现的重大冲突、是否把缺证据等同正/反方胜利。语义模型并非真值；质量集仍需人工标注。

修复后重新运行全部闸门，不只重跑失败的 JSON 校验。仍失败的段落不得进入任何用户响应。可用片段须独立校验后形成 `incomplete` 报告；若无法安全分离则 failed，不复制未通过的完整草稿。管理员日志默认同样不展示未经处理的草稿内容。

### 7.4 结果语义

| 情况 | run / report |
|---|---|
| 两角色正常结束且全部发布检查通过 | completed；supported/challenged/mixed/insufficient 之一 |
| 正常研究但客观资料不足 | completed + insufficient；列明缺口，不是假失败也不是成功证明观点 |
| 单角色异常、预算/依赖/关键验证失败，尚有独立通过的内容 | incomplete + verdict=null；注明未完成原因 |
| 无安全可发布内容 | failed，无 report |
| 用户取消/删除、epoch 失效 | 禁止新发布；已有报告不得被晚到结果覆盖 |

每个输入主张必须在结果或未知项中有去向；报告同时存在支持与反证不自动意味着 mixed，最终判断须基于材料质量，不能投票。不得输出买卖指令、保证收益或缺乏依据的置信百分比。

## 8. 预算、状态、故障和安全（B-25～B-29）

### 8.1 硬限额是代码约束，不是 Prompt 建议

| 项 | 最大值/规则 |
|---|---|
| 模型请求 | 每角色 4；每 run 14，含解析、合成、复核、修复、再检查、任何重试 |
| 工具 | 每角色 6；每 run 12，包括计算；实际供应商 HTTP 扇出另受第 6 节限制 |
| Token | 单请求输入 8,000 / 输出 1,200；run 累计输入预占 112,000 / 输出预占 16,800 |
| 时间 | 排队 30 秒；开始执行到截止 180 秒；模型≤45 秒，工具≤15 秒，均取剩余 deadline 较小值 |
| 并发 | 每用户 1 活动 run；全局 2；Python 4 角色任务 |
| 次数与追问 | 解析 5 次/用户/分钟，研究 10 次/用户/天；每报告 3 轮追问，每轮≤2 模型请求，独立计账且无外部工具 |
| 货币费用 | 管理员在 D-05 明确 run/day 上限及计价来源，未设置不允许真实付费测试；无可靠计价不能谎称费用封顶已验证 |

通常分配：解析1+两角色8+合成1+复核1+修复1+再检查1=13，最多剩1次重试；解析用2次就不剩额外重试。任何阶段只能消耗共享剩余额度，不能各自保留完整独立14次。

网络前原子预占次数、输入/最大输出 Token 和对应费用；SDK 自动重试关闭。无 usage、网络超时或计费不确定保留预占，标 unknown；不能直接退款再调用。解析消耗只转入一个确认 run 一次。测试与正常业务账本分开，但都是真费用。

### 8.2 取消、删除、重启

Go 抢占租约 15 秒、每 5 秒独立续约，不能因等待模型阻塞心跳。每次新抢占提高 execution_epoch；状态变更用期望版本/epoch 的 CAS 或等价行锁事务，不能无条件设置 verifying 覆盖 canceling。

取消提交事务即撤销新调用权限，进入 canceling；通知两个角色停止。已在途供应商调用可能完成并计费，不能承诺撤回。仅当 worker 已终止，或其有效执行权过期且 token 已撤销，才 canceled。受控集成测试要求取消至终态≤20 秒；撤权后新的出站请求数必须为 0。

发布/证据注册/任务回写都检查未删除、未取消、有效租约/epoch。删除立即隐藏并撤权；24 小时内清理私有观点、正文副本、结果、追问、Python checkpoint/任务 payload 和请求缓存；备份轮转与最小去标识账本按总 SPEC。未终止的删除任务仍占并发槽，不能删除即释放后继续跑。

Python 重启将原 running 明确标 `WORKER_RESTARTED`，不盲目重放外部调用。Go 重启先查询原任务，不能换 task_id 重跑全图；无安全恢复证明就 incomplete/failed。B 不承诺断点续跑。

总 deadline 到期即禁发新调用；本地最多 5 秒完成终态结算，不借“修复”延长研究。安全终态可以晚于 deadline 的短暂结算窗口，但不能将超时研究标 completed。

### 8.3 安全底线

真实供应商 host 明确允许列表，拒绝内网/元数据/loopback、恶意跳转及 DNS 重绑定；内部 Go↔Python 使用单独私网规则。消费者不能设置 provider URL。外部正文中的指令只作数据，不改变工具、权限、模型配置或发起额外网络请求。报告净化 HTML 和链接；密钥/私有材料不入 Git、截图、公共 trace。

## 9. 测试、质量与交付（B-30～B-34）

### 9.1 三层证据，不能互相替代

1. **确定性测试**：真实应用逻辑+受控模型/数据服务，覆盖每个失败断言、并发预算和状态竞争；事务相关使用真实 PostgreSQL。
2. **框架与接入测试**：真实 Eino/Harness+供应商能力测试，证明工具执行和配置隔离；不能仅 import，不能跳过。
3. **产品 live 测试**：真实普通用户登录，浏览器点击→实际服务→真实模型与数据→报告抽屉。没有该层，只能说局部接入完成。

最低固定质量集 30 例，至少 10 例为反向/不足，覆盖事实、缺证据、相互冲突、未来泄漏、单位/期间、预测冒充事实、注入。每例必须有输入、证据集、预期允许/禁止结论和程序断言，空样本/恒真断言不能计数。关键数值、时间、权限错误容忍为 0；不是对开放世界“零幻觉”的宣称。

G5 另固定 10 个真实观点输入，覆盖 D-04 的 3 只证券，每例运行 2 次共 20 次；计费前确认费用授权，不挑成功结果重算分母。至少 18/20 在执行窗口内完成流程；completed+insufficient 可算流程完成，但至少 5 个不同输入须产生含有效事实证据的非空报告，防止全部拒答过关。失败原因逐一记录，出现安全/关键数值/时间错误直接阻断。

人工对20次 live 输出中所有对外重要事实/推断做引用蕴含标注，支持率≥90%，明显误导性重要结论为 0。分母是实际重要陈述数，不是引用链接数；同时记录未知/拒答比例。阈值是 B 的拟定验收线，不是已有实测结果或线上 SLA，签收前如调整要版本化，不能跑完再降低。

记录模型/数据/Prompt/策略版本、样本数、总耗时和 p50/p95、ledger 用量/unknown、费用或保守预占、取消时延。非流式不宣称 TTFT/TPOT 实测，20 次样本也不能用于规模化 TPS 承诺。

### 9.2 可执行入口与证据包

测试 ID、命令约定、证据目录详见附件。开发者需实现统一 `app/scripts/verify-stage-b.sh`（当前是待开发要求），分 `offline / integration / live / all`；缺凭据返回 blocked 和非零退出码，必需测试 skipped 必须失败，禁止空测试集绿色退出。

交付必须有代码基线、依赖锁、迁移、启动文档、管理员配置说明、测试映射、真实 trace、去敏账本、质量标注与限制清单。所有完成状态以机器输出和独立复核为准；开发者手写“全绿”或模型自述不能当证据。

## 10. 后续扩展与变更纪律（B-35）

- **换模型/协议**：后台选择两种已支持协议之一，新增版本和对应能力测试，复用同一网关/账本/用途映射；不让 Python 另配 Key，不把协议切换做成静默失败兜底。
- **换数据源/增证券**：实现 provider adapter、字段映射与权限/时间测试，更新覆盖目录与 source version；不改消费者 API 和 evidence 信任边界。
- **加向量检索/PDF**：在检索层扩展，仍输出带真实定位和版本的可信记录，不绕过原文/数字闸门。
- **加研究角色/工具**：不是改一段 Prompt；必须修改能力白名单、预算、任务 Schema/控制协议、并发与负例，再评审。B 不提前搭建通用插件市场。
- **二期记忆/自选股**：新增用户确认的业务实体和权限/留存规则；不把本轮 DeerFlow checkpoint 升格为长期用户画像。
- 变更记录写清“原因→受影响 B-ID→接口/迁移→测试→费用/时间影响”。不得为赶进度删除断言、降低阈值或把失败归到阶段 C。

## 11. 签收条件与当前状态

本文把总 SPEC 中容易被口头解释的部分明确为：真实 Harness 执行证明、记录句柄登记、控制协议版本、配置激活门槛、全报告校验、一次修复边界、失败终态、测试证据与签收人。

**当前只完成 SPEC 设计交付；G0～G5、供应商选择、费用额度、应用实现与真实效果均未因本文被认定通过。**

签收本设计后先执行 G0；供应商信息未到位时停在对应接入关口。用户或指定复核者确认全部必需证据后，才能写“阶段 B 技术样机完成”；阶段 C 与公网开放另行验收。
