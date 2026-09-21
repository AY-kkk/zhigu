# 知股 v2：GoSaaS + Eino + DeerFlow 面客产品架构

日期：2026-09-17。状态：源码级设计，未实现或联调。配套 [一期 PRD](金融C端Agent_MVP_PRD.md)。
用户决定保留 GoSaaS 与 Eino，并使用 DeerFlow 研究服务；本文件落实三者的责任边界。目录中“新增”是计划文件，不代表磁盘上已有相应代码。

## 1. 最终分层：一个业务入口，一个总调度层

```text
用户浏览器（Vue C端）                  运营人员（Vue Admin）
       │                                     │
       └────────── GoSaaS / Go Gin ───────────┘
                    │ 身份、权限、额度、任务、模型配置
                    │
               Eino Workflow（Go 进程内）
               拆解/确认 → 研究分派 → 汇总/校验 → 发布
                    │            │
          支持研究任务            反证研究任务
                    │            │
               内部 Python Research API
               ├─ DeerFlow 独立会话 A
               └─ DeerFlow 独立会话 B
                 子 Agent 关闭、长期记忆关闭
                    │
           受限金融工具 / 统一模型调用边界
                    │
            外部数据源、证据原文、LLM API

最终报告、状态、权限：Go 服务统一管理 → PostgreSQL
Python 不直接写业务表，不直接对浏览器开放
```

这是系统层面的多 Agent：两个研究者各自有目标、独立上下文及工具调用循环。并非必须启用 DeerFlow 的子 Agent，才称得上多 Agent。

Eino 是总流程编排库，不是额外部署服务；DeerFlow 是研究执行器，不是第二套面客产品。其现成 Next.js 前端、通用 Gateway 和独立用户管理不并入客户访问链路。

**选择的代价：**比单 Go 服务增加 Python 部署、内部认证、跨进程预算和取消机制。保留三者符合当前用户选择，但不能宣称它是开发量绝对最小的路线。Eino 也可被普通 Go 状态机替代；保留它是为了统一节点编排，而不是因为面客天然需要它。

## 2. 运行时依赖边界

阶段 A 的可运行代码在 `app/`：Go 宿主编排研究流程，Python 适配层执行支持方/质疑方任务，Vue 提供面客与后台。版本以本仓库锁文件为准，不是“外部工程已整包集成”的证明。

- Go：Gin、GORM、Eino，见 `app/server/go.mod`
- Python：FastAPI；可选研究 Harness extra，见 `app/research-service/pyproject.toml`
- 前端：Vue 3 与 Semi Vue，见 `app/web/package-lock.json`

私有业务底座快照不随本仓库分发。阶段 A 使用独立 `app/server` 宿主即可启动。第三方许可见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 3. 代码目录与所有权

基于有使用权的 GoSaaS 工程扩展，不从零复制第二套账号系统：

```text
web/src/
  view/research/                  # 新增：new、detail、history，C端独立布局
  components/research/            # 新增：ClaimEditor、EvidenceDrawer、Report
  view/researchAdmin/             # 新增：模型/数据源/预算设置、任务记录
  api/research.js                 # 新增：只调用 Go 业务 API

server/
  router/finance/                 # 新增：面客路由、管理路由
  api/v1/finance/                 # 新增：请求校验、用户/管理员授权
  model/finance/                  # 新增：任务、主张、证据、报告、配置版本
  service/finance/
    research_service.go          # 创建任务、限额、取消、幂等、任务状态
    workflow.go                  # Eino DAG；唯一总流程
    nodes.go                     # 观点拆解、汇总、复核节点
    research_client.go           # 调用 Python，不执行第二套 Agent 规划
    evidence_service.go          # 证据注册、来源权限、时间和数字校验
    report_service.go            # 通过校验才能发布最终报告
    config_service.go            # 模型/数据源版本、测试启用与密钥保护
    budget_service.go            # 共享预算、配额预占、用量核对
    model_proxy.go               # 内部受限模型转发，不是公开通用代理

research-service/                # 新增：独立 Python 服务
  pyproject.toml                 # 锁定 DeerFlow 及接口依赖组合
  app/
    main.py                      # FastAPI，仅内网监听
    schemas.py                   # 与 Go 共享的输入/输出契约
    jobs.py                      # 角色任务执行、查询、取消
    deerflow_adapter.py          # 唯一 DeerFlow 接入点，隔离版本变化
    policies.py                  # 角色、工具、次数、截止时间约束
    tools/
      financials.py              # 数据供应商只读查询与规范化
      filings.py                 # 正文检索、日期过滤、证据登记
      calculator.py              # 确定性指标计算
    prompts/
      supporter.md               # 最强支持证据与成立条件，也记录矛盾
      challenger.md              # 最强反证、替代解释；允许找不到反证

contracts/
  research-task.schema.json      # Go/Python 都验证
  research-result.schema.json
tests/
  fixtures/                      # 有来源、时间和预期结果的固定用例
  contract/                      # 协议兼容、版本和未知字段校验
  e2e/                           # 真实链路、越权、取消、失败和预算
```

前端只有 Vue；不要再次添加 Next.js。Python 依赖 DeerFlow Harness，不复制修改整套上游业务应用。不写死未经测试的 SDK 版本组合；先做 adapter 合约测试再锁依赖。

## 4. 执行链路与 Agent 边界

1. Go 校验用户权限、限额、观点长度和证券覆盖范围。输入解析可调用模型，结果必须经 Schema 校验；证券/期限有歧义时先返回待确认，不直接发起调查。
2. 用户确认后，Go 冻结主张、as_of、模型/数据源/Prompt 版本及全局预算，持久化任务。
3. Eino 两个分支分别调用 ResearchClient，提交 supporter / challenger 两个角色任务。同一中立主张和数据截止时间，独立 thread_id；不传对方生成的论证。
4. 每个 DeerFlow 研究者可按新证据决定下一步工具查询，但不能生成第三个 Agent、越过工具白名单或自己扩大预算。
5. Python 将来源和原文登记为证据，返回绑定证据 ID 的论证、矛盾、缺口与实际用量。
6. Go 核验角色结果；Eino 汇总已通过的事实，生成候选报告；程序硬校验和模型语义复核通过后，Go 发布。
7. 一方失败时，分支返回类型化 failure，不让整个 DAG 无条件吞错。已验证资料可以展示为 incomplete；未完成双向研究不得标为完整报告。
8. 追问默认只查报告已有证据，不重新启动两路研究；用户明确要求补充研究时才新建有额度和版本的任务。

Workflow 图是 DAG，DeerFlow 的调查 Loop 位于研究节点内部。最终报告校验器没有工具规划循环，不为展示“多个角色”再创建一个自由裁判 Agent。修复最多一次，修复后再次验证，预算不足则不发布。

## 5. Go ↔ Python 的接口契约

### 面客 API（Go）

| API | 语义 |
|---|---|
| `POST /api/finance/claims/parse` | 解析观点，返回待用户确认的主张草稿 |
| `POST /api/finance/research` | 提交已确认主张；返回 202/run_id；要求 Idempotency-Key |
| `GET /api/finance/research/:id` | 获取本人任务状态、进度、已发布报告或明确失败信息 |
| `POST /api/finance/research/:id/cancel` | 请求取消；先取消中，确认 worker 停止后终态 |
| `POST /api/finance/research/:id/questions` | 基于已发布报告追问，仍执行权限/预算/引用检查 |
| `GET /api/finance/research` | 我的历史；分页、owner 过滤 |
| `DELETE /api/finance/research/:id` | 先撤销活动执行授权，再清理用户报告/私有载荷，避免删除后又被 worker 写回 |
| `GET /api/finance/evidence/:id` | 返回经授权的原文证据，不做任意 URL 代理 |

### 内部 Research API（自行新增，不冒充 DeerFlow 原生端点）

- `POST /internal/research/tasks`：受认证服务调用，幂等创建角色任务。
- `GET /internal/research/tasks/:task_id`：查询状态及结构化结果。
- `POST /internal/research/tasks/:task_id/cancel`：传递取消并查询确认状态。

任务字段：`schema_version, run_id, task_id, role, claim, instrument_id, as_of, source_policy_version, model_config_version, prompt_version, max_model_calls, max_tool_calls, deadline_at`。

模型输入不包含内部凭据。role 只允许 supporter/challenger，由 Go 服务指定；instrument 与 as_of 在工具层强制约束，模型不能通过 tool arguments 改掉。

结果字段：`task_id, status, arguments[], evidence_ids[], unknowns[], counterevidence[], usage, errors[]`。每条 argument 含 claim_type、text、evidence_ids；status 为 succeeded/insufficient/failed/canceled。引用必须属于当前任务授权证据集。

使用短时、绑定 run_id/task_id/权限的服务令牌和受保护传输；Python 无公网入口。重放同一 task_id 返回原任务，不重复计费启动。Go 校验返回任务标识和 Schema；网络故障后先查询状态，不能盲目重发新任务。

## 6. 模型与数据配置：一个控制面

运营人员只在 GoSaaS 配置，Go 持久化不可变模型版本、密钥引用和测试结果。Eino 与 DeerFlow 均按同一 run 的冻结版本调用。

2026-09-21 起阶段 B 以 [SPEC B-1.2](阶段B_开发SPEC.md) 为准：独立宿主运行；live 兼容 Responses；数据走东财/新浪+巨潮（AKShare 不得进入研究工具进程）。

2026-09-18 用户明确双协议接入，阶段 B 实现两个受限内部入口：`/internal/llm/v1/chat/completions` 与 `/internal/llm/v1/responses`。后台protocol分别为 `openai_chat_completions`、`openai_responses`；入口必须匹配任务冻结配置。两者共享Go网关的鉴权、用途、允许上游、预算与供应商Key注入，不是两套后台。Eino 使用同一配置/预算服务，不绕过统计。

Python 使用临时任务凭据，不保存供应商 Key；模型 Profile 使用服务端固定别名，由 Go 映射真实模型 ID。运行中换模型不会改变已签发任务的映射。DeerFlow adapter 必须将临时凭据绑定当前实例，不复用携带另一个用户凭据的全局客户端。

这是需要新增并测试的集成模块，不是 GoSaaS 或 DeerFlow 已经提供的本产品网关。阶段B实现Chat Completions和Responses两种adapter，均为非流式；工具调用、结构化结果、上下文和usage按各协议适配并分别验收，不能仅更换URL。每个配置只选一种协议，不静默回落，不做通用多厂商代理平台；未真实验证的协议配置不能启用。

配置页：模型协议/Base URL/Model ID/Key、研究/汇总用途、数据源凭据、次数与时限。每次启用同时检查 Go 与 Python 的真实调用路径，以及 JSON/工具调用能力。列表只返回 has_key；Key 加密保存、请求体日志排除、根密钥独立于数据库。

限制上游主机、阻断内网/环回/链路本地目标及跨主机重定向。服务到服务流量可访问明确配置的内部服务，但模型不得通过工具扩展该允许名单。不可依赖模型提示词保密或限制权限。

## 7. 数据库、任务和成本控制

业务数据库由 Go 唯一管理，新增：
- research_runs：owner、claim、as_of、versions、status、budget、deadline、idempotency_key。
- research_tasks：parent_run、role、worker_task_id、attempt、状态和用量；唯一约束防重复启动。
- evidence：来源、原文/数值、单位、报告期、发布日期、取得时间、hash、授权范围。
- reports：run、version、claims、evidence_ids、quality_status；仅通过发布检查可见。
- model_config_versions / data_source_versions：只存加密凭据或密钥引用。
- usage_ledger：调用预占、完成核对、退款/释放与状态，不依赖模型自报。

任务总状态：draft → needs_confirmation → queued → researching → verifying → completed/incomplete/failed/canceling/canceled。draft/needs_confirmation 阶段不执行外部调查。

一期执行上限：两个研究任务，每方最多 4 次模型请求、6 次数据工具；总最多 14 次模型请求、12 次数据工具、180 秒执行时间，排队最多 30 秒。解析、汇总、复核、修复和所有重试都计费记账。解析阶段受单独用户请求限流，确认后将同一研究解析用量计入总预算，不重复收取/统计。

模型发起前以原子方式预占调用和 token 额度，结束按 usage 核对；没有 usage 时标未知并保守占用，不将其视为免费。供应商内部缓存和扣费以实际账单为准。模型/HTTP 库自动重试须关闭或统一纳入计数。

Python 工具调用也要向 Go 的预算服务取得授权；并发分支不能各自误认为拥有完整总预算。每个模型请求和工具请求使用剩余 deadline；取消必须传播并停止后续调用，已发出的供应商请求不保证可撤回。

开发/内测先限制一个用户一个活动研究、全局两个研究 run、Python 最多四个角色任务；这是初始保护值，不是生产容量承诺。Go 内置持久化任务表可用于单业务实例；多副本上线前验证抢占租约、恢复、扩容和排队能力。Eino 本身不自动持久化整条业务链。

原文/数值留存与业务数据库分离：少量文本可在数据库，原始文件后续入对象存储；私人观点与提示词不得作为共享公共缓存。二期记忆和自选另建业务表，不混入 DeerFlow 默认长期记忆。

## 8. 金融 Harness 与面客发布门槛

研究实例关闭：子 Agent 递归、自动长期记忆、任意 Shell/文件写入、任意 MCP、开放 URL 抓取、Agent 自行注册工具。只允许金融数据、受控正文检索、计算三类工具。

仅配置 `subagent_enabled=false` 不代表安全限制全部完成；要检查最终实际工具集合与 middleware。DeerFlow 默认是通用 Agent，不能直接当金融沙箱用。启动和每次构建 Agent 时验证工具集合，出现额外高权限工具则失败关闭。

验证分三层：
1. 结构与访问：Schema、证据集合、用户/任务归属、资料日期。
2. 确定性事实：数字、单位、期间、复权口径、引用原文、程序计算。
3. 语义判断：证据是否支持该主张、是否把预测当事实、是否忽略相反材料。模型辅助检查并人工评测，不宣称零幻觉。

进度只展示“理解观点/查找证据/交叉核对/生成报告”，不公开隐藏思维链或未经校验草稿。部分结果显式标 incomplete，支持方失败不能被反方当作反证。

从框架到面客还须完成：账户隔离、接口反滥用、配额与费用上限、来源授权、隐私留存/删除、HTTPS/密钥管理、监控告警、备份恢复、安全和负载测试、适用合规评估。免责声明不能替代这些条件。

## 9. 建设顺序与当前状态

先验证五个关口，不同时重写所有界面：
1. GoSaaS 启动、依赖与代码授权核验。
2. Eino 节点调用 Python，DeerFlow 工具白名单和任务上下文隔离可验证。
3. 同一观点跑通两路真实研究，接一个真实外部数据接口，返回证据契约。
4. Go 完成统一模型配置、预算、校验和报告发布；失败不能绕过发布检查。
5. 接入 Vue 观点页、报告页、历史页及管理页面，再做受邀内测。

本轮只更新设计文档，未修改私有业务底座或研究执行器源码、未启动客户服务、未创建远程仓库。实现前的设计审阅依据是本 v2，不继续沿用 v1 的单服务和自选股主入口。工作区尚非 Git 仓库，本轮未创建提交。
