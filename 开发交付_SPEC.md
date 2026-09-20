# 知股一期：编程 Agent 开发交付 SPEC / Implementation Plan

版本 1.0｜2026-09-17｜状态：实施契约，应用尚未开发或联调。

2026-09-18 阶段 B 增补：用户明确允许 OpenAI-compatible Chat Completions API 与 Responses API 双协议接入；模型接口细节以 [阶段 B SPEC B-1.1 第6.1节](阶段B_开发SPEC.md#61-双协议模型适配与兼容性关卡)为准。本增补不追溯改变阶段 A 的历史验收。

**Goal：用户粘贴一个 A 股单公司投资观点，确认研究范围后，获得可追溯的支持证据、反证与未知项。**
**Architecture：阶段 A 使用独立 zhigu 宿主完成离线闭环；GoSaaS 为已授权后续迁移底座。Eino 是唯一总编排，DeerFlow/fixture 执行两个隔离、有界的研究 Loop。**
**Tech stack：现有 Vue + Gin/GORM，PostgreSQL，Go/Eino，Python 3.12 + FastAPI + DeerFlow Harness。**
**For agentic workers：使用可用的 executing-plans 技能逐任务实现、测试、记录证据；不因交接文档自动启动子 Agent。本文是开发输入，不代表任何应用模块已完成。**

## 0. 阅读顺序与不可变约束

1. [一期 PRD v2](金融C端Agent_MVP_PRD.md)。
2. [面客架构 v2](金融C端Agent_面客产品架构_v2.md)。
3. 本文 → [仓库清单](references/README.md) → [机器契约](handoff/contracts/) → [验收案例](handoff/acceptance-cases.json)。
4. [直接交给编程 Agent 的指令](handoff/CODING_AGENT_PROMPT.md)。

优先级：用户最新指令 > PRD v2 > 架构 v2 > 本文的实现细化。冲突需指出，不静默修改。旧《技术设计与开源复用》只作历史源码线索，不能恢复成自选股一期或单 Go Agent 方案。

- 保留 GoSaaS + Eino + DeerFlow；不改成另一套全栈框架。
- references/ 是只读、固定提交的参考快照；后续开发放 app/。不改用户原有 GoSaaS 工程。
- 一个客户前端，一个业务写入者，一个总调度器。Python 无业务数据库密码，不接受浏览器直连。
- 不做交易、个性化荐股排名、收益保证、长期偏好记忆、持续自选追踪、批量 PDF/OCR。
- “反证”不是强行生成反话；允许找不到有效反证，允许最终证据不足。
- UI 不显示未经校验的模型草稿、隐藏思维链、密钥或工具原始日志。
- 不自动部署公网、创建远程仓库/PR、购买数据/API，或推送用户内容到额外服务。

## 1. 交付阶段和前置依赖

| 阶段 | 必须交付 | 不得冒充 |
|---|---|---|
| A：离线工程闭环 | fixture 明示；Vue→Go/Eino→Python→受控工具→报告；权限与失败路径可测 | 真实外部数据链路、投资分析效果 |
| B：技术样机 | 真正导入 DeerFlow Harness；真实模型；一种经授权外部金融数据与正文来源；双路研究成功 | 公网面客就绪 |
| C：受邀内测 MVP | 全部 P0、后台设置、预算、取消、隔离、备份恢复、质量集 | 合规和规模化上线已通过 |

一天只是阶段 B 的时间盒，不是阶段 C 的完成承诺。缺少凭据时继续做可测工程，最后明确停在 A，不伪造 B。

开工前依赖：
- 私有业务底座快照未发现项目级 LICENSE/NOTICE。阅读快照不等于获得复制、改造、分发其特有代码的权利。授权未确认时，先完成独立契约、测试和新模块；不要把其特有代码纳入可分发应用。不能擅自更换业务底座。
- 确认一个可商用数据供应商的凭据、覆盖范围、正文与再展示权限。暂无指定供应商，本 SPEC 不杜撰厂商 endpoint。
- 模型配置可选择 Chat Completions 或 Responses 协议；所选协议的 tool calling、结构化输出与上下文续轮需测试通过才可启用。B须实现两种adapter，不要求每家供应商同时支持两协议。
- 没有运行态依赖锁定证明：本地源码快照不等于 Go/Python/Node 兼容性结论。运行依赖以本仓库锁文件为准。

## 2. 应用目录与模块责任

以下 app/ 目录均由后续编程 Agent 创建，本次只提供 handoff/ 和 references/。

```text
app/
  server/                         # 基于获授权的 GoSaaS 扩展，Go module 保留
    router/finance/{consumer,admin,internal}.go
    api/v1/finance/{claims,research,evidence,settings}.go
    model/finance/{draft,run,task,evidence,report,config,usage}.go
    service/finance/
      research_service.go         # owner、幂等、队列、状态和取消
      workflow.go                 # Eino DAG，不创建递归 Supervisor
      nodes.go                    # 解析/合成/复核节点与一次修复
      research_client.go          # Python API；超时先查询原 task_id
      evidence_service.go         # 数据来源、时间、单位、证据登记
      report_service.go           # 发布闸门与报告追问
      config_service.go           # 不可变配置版本、加密和启用测试
      budget_service.go           # 原子预占/核销、全局和角色上限
      model_proxy.go              # 唯一模型凭据与调用出口
      worker.go                   # 持久任务抢占、租约、恢复
      *_test.go
    migrations/finance/001_init.sql
  web/src/
    layout/consumer/index.vue      # 与管理员布局分离
    view/research/{new,detail,history}.vue
    components/research/{ClaimEditor,Report,EvidenceDrawer}.vue
    view/researchAdmin/{settings,runs}.vue
    api/research.js
    router/finance.js
  research-service/
    pyproject.toml
    uv.lock
    app/{main,schemas,jobs,deerflow_adapter,policies}.py
    app/tools/{financials,filings,calculator,provider}.py
    app/prompts/{supporter,challenger}.md
    tests/{test_adapter,test_isolation,test_tools,test_jobs}.py
  contracts/                      # 从 handoff/contracts/ 原样复制，版本化
  tests/{contract,e2e,fixtures}/
  deploy/compose.yaml
  .env.example                    # 仅占位，无默认真密钥
  IMPLEMENTATION_STATUS.md
  README.md
```

沿用 GoSaaS router_biz.go / gorm_biz.go 扩展点注册模块；生产迁移使用显式 SQL。除注册与必要基线修复，不大规模重构原工程。

部署：浏览器 → 反向代理 → Vue / Go；Go → 私网 Python；Go → PostgreSQL。Python 任务状态使用独立 SQLite WAL 文件卷（仅执行状态，不存业务账号/供应商 Key）。首期一个 Go 实例、一个 Python 服务进程；多副本不在本期验收承诺内。不引入 Redis/Celery/Kafka 或向量库作为启动必需品。

## 3. 页面与交互验收

- /app/research/new：输入 20–2,000 字；解析后展示股票候选、绝对期限、最多 6 个主张。未明确选择证券和期限，按钮不能启动；后台同样校验。覆盖外的标的返回 UNSUPPORTED_INSTRUMENT。
- /app/research/:id：每 2 秒轮询状态，离开页面停止轮询；刷新恢复。显示排队/调查/核对/完成/未完成，不显示估算百分比。取消先进入 canceling。
- 报告：原观点、范围/as_of、四态结论、支持、最强反证、改变判断条件、未知、证据抽屉。incomplete 用“研究未完成”，verdict=null。
- /app/history：游标分页，默认 20 条、最大 50；只查本人未删除记录。重新研究产生新 run_id，可填写 parent_run_id，不覆盖旧报告。
- 证据抽屉：原文定位、URL、披露时间、可获得时间、报告期、单位、actual/estimate、获取时间与版本。禁止仅显示搜索摘要。
- 追问：只在已有可见报告的证据集内；前端显示“未进行新一轮检索”。证据不足时建议用户主动点“重新研究”。
- /admin/ai-settings：模型和数据源分页；保存→测试→启用。无普通用户访问权限。
- /admin/research-runs：run_id、匿名化 user_id、阶段、用量、耗时、错误码；默认不显示完整观点/对话。
- 固定 fixture 标识不可被前端隐藏；fixture 证券使用 DEMO:COMPANY，不伪装真实 A 股。
- 报告以结构化组件渲染；如使用 Markdown，关闭 HTML 并净化链接。链接仅允许 http/https，外链 noreferrer。

## 4. 执行流程：外层 DAG，内层有界 Loop

```text
ParseDraft → 人工确认（HTTP 请求之间不挂起 workflow）
                         ↓
                   创建持久化 run
                         ↓
                FreezeContext / Quota
                         ↓
           ┌── Research(supporter) ──┐
           └── Research(challenger) ─┤
                                    ↓
                            ValidateEvidence
                                    ↓
                         Synthesize → Verify
                                    ↓
                       [最多修复一次，再校验]
                                    ↓
                 Publish completed / incomplete
```

Eino Workflow 无环；有限修复在 Verify 节点内完成。ParseDraft 为独立的解析调用/短链，不把用户确认等待放进 180 秒执行窗口。研究双分支返回 Result 或类型化失败，不能一方抛错就吞掉全部证据。

中立输入相同：claim、instrument、as_of、数据版本。两个角色不读取对方生成的 arguments；不将“支持者失败”解释为反证。允许共享授权公共原始数据缓存，不共享用户观点或研究上下文。

DeerFlow adapter 必须完成一个先行兼容性关卡：
1. 使用固定源码的 Harness 包（不是自己写 Loop 再命名为 DeerFlow）。
2. 每 task 建独立 agent/thread/context；subagent_enabled=false，allowed_subagents=[]，skills=[]，memory_enabled=false，plan_mode=false。
3. include_mcp=false；最终 tools 名称必须恰好等于本项目白名单；上游默认内置工具也必须排除。单设 tool_groups 不算通过。
4. 模型客户端绑定本 task 的短期 token 和配置；禁止请求中修改全局 env/YAML，禁止全局共享携带其他用户 token 的客户端。
5. 检查真实 middleware，禁用文件/执行/长期记忆能力；无宿主目录挂载。仅工具白名单不是 OS 沙箱。
6. 若嵌入式客户端不能满足注入和工具隔离，适配层使用固定 Harness 的组装入口并编写契约测试；记录最小补丁和 license，不能悄悄退回自写 Agent 或开放工具。
7. 真实调用证明两个角色、模型出口和工具授权均有效后，才接正式 UI。

短期记忆：角色任务仅自身本轮 messages；不跨 run 复用。追问保存最多最近 3 轮供本报告使用，旧轮次不自动写长期画像。历史报告是业务数据，不是 DeerFlow 长期记忆。

## 5. 面客与管理 API

新 finance API 使用标准 HTTP 状态：200/201/202 成功，400 参数，401 未登录，403 无权限，404 不存在或非本人，409 状态/幂等冲突，422 范围不支持，429 配额，503 配置不可用。
统一响应：成功 {"data":{...},"error":null,"trace_id":"..."}；失败 {"data":null,"error":{"code":"...","message":"..."},"trace_id":"..."}。
可保留旧 GoSaaS API 的原 envelope；新增 research.js 做明确适配，不能依赖旧拦截器把上述响应误判。模型代理例外：按冻结配置及内部入口返回对应 Chat Completions 或 Responses 格式。

| API | 输入与输出约定 |
|---|---|
| POST /api/finance/claims/parse | {text, as_of?} → draft_id, revision, candidates[], suggested_horizon, items[], needs_confirmation, mode；无确定标的也返回草稿，不启动研究 |
| POST /api/finance/research | Header Idempotency-Key；{draft_id, revision, instrument_id, horizon, as_of, parent_run_id?} → 202 {run_id,status,poll_url} |
| GET /api/finance/research/:id | {run_id,status,stage,mode,as_of,report?,warnings[],error?,updated_at}；report 仅已发布版本 |
| GET /api/finance/research?cursor=&limit= | {items[],next_cursor}；按 created_at,id 稳定排序 |
| POST /api/finance/research/:id/cancel | {} → 202 canceling；终态幂等返回原状态 |
| DELETE /api/finance/research/:id | 202 {deletion_status:"scheduled"}；立即隐藏、撤权，异步清理 |
| GET /api/finance/evidence/:id | 对应 evidence schema；从本人有权访问的 run→report/task→evidence 授权，不凭 ID 猜测访问 |
| POST /api/finance/research/:id/questions | Header Idempotency-Key；{text} → {answer,claim_type,evidence_ids,limitations,mode}；不调用研究 API |
| GET /api/finance/instruments?q= | 最多 20 个当前数据源覆盖的证券；用户不能传任意 instrument |

研究创建必须在一个数据库事务中检查草稿归属、revision、期限、配置可用、活动 run 和幂等键；相同 key+规范化请求 hash 返回原 run，相同 key 不同 body 返回 409。同一草稿 revision 最多确认一次；重新研究需新草稿或显式重新研究副本，解析用量不得重复转移。

管理员 API 前缀 /api/finance/admin：
- GET/POST /model-configs；POST /model-configs/:id/versions；GET 只回 has_key。
- POST /model-config-versions/:id/test → test_id；GET /tests/:test_id → 分项状态。
- POST /model-config-versions/:id/activate：只有相同配置 digest 测试通过才能启用。
- GET/POST /data-sources；POST /data-sources/:id/test；POST /data-sources/:id/activate。
- GET/PATCH /policy：预算和来源规则保存新版本；UI 只允许降低本文硬上限。提升上限需要代码与回归测试。
- GET /research-runs：脱敏、分页；不能借此绕过证据 owner 检查。

所有业务权限后端执行：JWT/Casbin 只负责入口，不能替代每次查询的 owner_id 条件。普通测试用户不要复用管理员角色或默认管理员账号。

## 6. 内部协议与工具接口

HTTP JSON，schema_version="1.0"；四份 JSON Schema 是正式字段约定，additionalProperties=false。时间是带时区 RFC3339，服务端规范化 UTC；金额和指标值用十进制字符串。Schema 只校验结构，语义和授权另查。

Go → Python（服务身份凭据放 Authorization，临时模型 token 用单独内部受保护 header，不放请求正文/Prompt/日志）：
- POST /internal/research/tasks：research-task.schema.json，202 {task_id,status:"queued"}。
- GET /internal/research/tasks/:task_id：{task_id,status:queued|running|succeeded|insufficient|failed|canceled,result:null|ResearchResult}。
- POST /internal/research/tasks/:task_id/cancel：202；取消后 GET 确认。
- 同一 task_id + payload hash 不重复执行；不同 payload 返回 409。task_id 固定，不因网络超时换 ID。
- POST 超时先 GET；若不存在才以同一 ID 重发。查询退避 0.5/1/2 秒，上限 2 秒，受 deadline 限制。
- Python 进程重启把此前 running 标 failed/WORKER_RESTARTED；不盲目重跑不确定的模型或工具调用。

Python → Go（每个端点都校验 token 的 run/task/purpose/expiry/revocation）：
- POST /internal/llm/v1/chat/completions：固定别名 finance-research，Go 决定真正 model/base URL/key；stream=false。
- B-1.1新增 POST /internal/llm/v1/responses：同样由Go统一鉴权/配置/预算，采用Responses原生协议，禁止内置工具和托管会话；完整约束见阶段B SPEC。既有chat入口不因新增协议而取消。
- 模型请求必须带 X-Request-ID；代理绑定 token 的 task、用途和规范化 body hash。已成功的同 ID 返回缓存结果；同 ID 不同 body 返回 409；处理中/结果未知不再次触达上游。显式新尝试用新 ID，并重新占用额度。
- POST /internal/finance/tool-grants：{request_id,tool_name,args_hash} → {grant_id,expires_at}，先原子预占工具额度；重复 request_id 返回同一 grant。
- POST /internal/finance/tool-grants/:id/complete：{status: succeeded|failed|unknown,output_hash}；重复核销幂等，不退款调用次数。
- POST /internal/finance/evidence：{grant_id,records:[EvidenceWithoutID]} → 注册的 evidence_ids；evidence_id/run_id/instrument_id/mode 由服务端确定或强制比对。
- GET /internal/finance/data-source-profile：返回冻结的覆盖范围、只读 connector 名与非敏感规则，不返回业务数据库密码。

金融工具仅 3 个，工具名字/参数由 JSON Schema 白名单固定：
- get_financials({metrics[],periods[]})：只读指标查询。instrument/as_of/version 由 task 注入，模型无法改写。
- search_filings({query,limit})：limit<=5，检索已授权来源正文；不能传 URL、SQL 或本地路径。
- calculate_metric({operation,inputs})：operation 仅 growth_rate / ratio / difference；inputs 为已登记 evidence_id + metric + period，不接受任意代码表达式。Decimal 计算，零分母返回 INSUFFICIENT_DENOMINATOR。

全部三类工具均受 6 次/角色、12 次/run 预算，计算也计入工具调用次数。单工具超时 15 秒或剩余 deadline，取较小；重试算新调用。工具结果统一：
{data, evidence_ids, as_of, data_version, quality_status: verified|insufficient|conflicted, warnings}。
原始数据先按 provider 契约规范化并登记，再提供可信 evidence_id 给模型。Python 的注册 payload 仍不天然可信：Go 校验来源白名单、授权 grant、hash、时间、字段和引用范围。

数据源凭据留在 Go。供应商通过新增内部受限数据 connector 出口调用：POST /internal/finance/data-query {grant_id,operation,params}；Go 映射供应商并注入 Key，Python 只接收数据。一个 grant 只能执行一次真实查询，参数 hash 必须匹配；超时不确定结果不重放为新执行。该出口不允许任意 host/URL/SQL。fixture 也从该出口读取 Go 侧固定样本，同样走 grant 和证据登记。

data-query 出口保存可信 connector 返回的规范化记录及记录 hash；证据登记必须匹配该 grant 下记录的原文、指标、locator 和来源，不能只检查 Python 自报 hash。模型生成的文本不能登记成财报原文。calculate_metric 的输出由 Go 侧 Decimal 复算；公式、输入 evidence_id/期间、精度和结果保存 finance_calculations，工具返回基础 evidence_ids，不把派生结果登记成供应商原文。报告数值校验可从此表复算。Python 负责受控工具适配，不新增第二套业务持久层。

## 7. 数据模型与持久化

ID 使用服务端生成的不透明字符串（生产建议 UUID；fixture 可用 run_demo）。时间 timestamptz，金额 numeric/字符串，不用 float。所有表带 created_at/updated_at；以下为最低字段，不是已创建数据库。

| 表 | 核心字段 | 约束/索引 |
|---|---|---|
| finance_claim_drafts | id,owner_id,text,horizon?,items jsonb,revision,as_of,mode,parse_usage_id,confirmed_run_id? | owner_id,created_at；确认时锁行 |
| finance_research_runs | id,owner_id,draft_id,parent_run_id?,claim_snapshot jsonb,instrument_id,horizon,as_of,mode,status,stage,config_versions jsonb,budget_snapshot jsonb,idempotency_key,request_hash,started_at,deadline_at,lease_owner,lease_until,version,deleted_at | UNIQUE(owner_id,idempotency_key)；owner 历史索引；status/lease；一个用户一个活动 run 的部分唯一索引 |
| finance_research_tasks | id,run_id,role,status,payload_hash,attempt,worker_task_id,result jsonb,heartbeat_at | UNIQUE(run_id,role)；UNIQUE(worker_task_id) |
| finance_evidence | id,run_id,instrument_id,source_id,source_url,source_kind,title,locator,text,metrics jsonb,published_at,available_at,retrieved_at,content_hash,data_version,mode | UNIQUE(run_id,source_id,content_hash,locator)；run_id 外键 |
| finance_task_evidence | task_id,evidence_id | 联合主键；必须同一 run |
| finance_calculations | id,run_id,task_id,grant_id,operation,inputs jsonb,formula,precision,result numeric,unit | UNIQUE(grant_id)；输入证据同 run 且已授权；保存确定性计算过程 |
| finance_reports | id,run_id,version,quality_status,body jsonb,validator_version,verification jsonb,published_at | UNIQUE(run_id,version)；公开读取仅 published_at 非空 |
| finance_questions | id,run_id,owner_id,idempotency_key,request_hash,body,response jsonb,status | UNIQUE(owner_id,run_id,idempotency_key)；短期上下文用 |
| finance_config_versions | id,kind,config_digest,public_config jsonb,secret_ciphertext,secret_key_version,test_status,test_digest,active_at | 不可变；修改产生新版本；secret 不参与 GET |
| finance_usage_ledger | id,owner_id,run_id?,task_id?,question_id?,parse_id?,request_id,purpose,kind,status,reserved_tokens,actual_tokens?,upstream_request_id?,error_code | UNIQUE(request_id)；reserved/succeeded/failed/unknown；用于审计而非信任模型自报 |
| finance_tool_grants | id,request_id,run_id,task_id,tool_name,args_hash,status,expires_at,output_hash? | UNIQUE(request_id)；原子执行标记 |
| finance_scheduler | singleton_id,policy_version | 队列抢占时锁行保证全局并发上限 |

owner_id 类型与 GoSaaS 现有用户主键保持一致。每个 repo 查询必须明确 run.owner_id=当前用户；不要依赖随机 ID 或前端过滤。公开基础行情缓存如需要，另表不含用户观点；P0 不必建设跨用户证据共享缓存。

数据库补充约束：活动唯一索引覆盖 queued/researching/verifying/canceling，即使 deleted_at 非空也不能提前释放并发。draft 保存解析时的配置版本，ledger 保留解析请求版本；确认后研究配置才冻结，不能谎称已发生的解析使用了新配置。config_versions 必须包含模型用途映射、数据源版本、来源规则、Prompt 与预算版本，不能只有供应商名称。

状态：
- 草稿：draft → needs_confirmation → confirmed；草稿表管理，不占执行并发。
- run：queued → researching → verifying → completed / incomplete / failed。
- 任一活动态 → canceling → canceled；取消期间禁止新模型/工具请求。
- 排队超 30 秒：failed/QUEUE_TIMEOUT。研究开始时冻结 deadline=started_at+180秒。
- completed 表示研究流程完成，不表示观点正确；两方正常结束但资料不足，可 completed+insufficient。
- 任一方 failed/canceled/超时或关键验证失败：有可发布证据则 incomplete，verdict=null；无可发布内容则 failed。不能把 incomplete 写成“受到挑战”。

发布事务锁 run，要求未删除、未取消、lease/version 匹配；验证证据与配置快照，写 report 再转终态。证据注册和任务结果写回也必须检查该锁/版本。删除先设置 deleted_at、撤销临时权限并停止任务；晚到结果不得恢复记录。

Go 轮询数据库任务抢占：一用户一活动 run，全局两个活动 run；锁 scheduler 行并分配租约（15秒，5秒续约）。Python 最多四个角色任务。进程重启/租约丢失后停止发布；协调器先查询原 task，未能安全恢复则标 incomplete/failed，不自动再跑整条研究。P0 不承诺断点续跑。

取消确认：两个 worker 已终止或租约过期且凭据撤销，才能 canceled；只能声称停止本产品后续工作，不能承诺供应商已撤回/免费。

留存初值：个人报告与追问默认 30 天；删除 24 小时内清理主存私有内容，备份最长 7 天轮转；日志不保留原文/密钥。公开上线前确认适用留存要求，不能将初值当法律结论。删除后的审计只保留最小去标识调用用量，不能还原观点。

## 8. 共享预算、模型控制面与性能测量

硬上限（初始设计，不是实测）：
- 每角色模型 4 次、工具 6 次；总模型 14 次、工具 12 次。
- 解析通常 1 次；两角色最多 8；合成 1；语义检查 1；修复 1；再检查 1；剩余 1 次仅受控失败重试，不是强制使用。
- 解析每用户 5 次/分钟；默认每日 10 个研究。追问每报告最多 3 轮、每轮最多 2 次模型请求，无外部工具，单独计账，不回写已冻结研究预算。
- 建议初始模型输入上限 8,000 tokens、输出 1,200 tokens；每 run 累计输入预占 <=112,000、输出预占 <=16,800。这些是安全上限，非平均成本。禁止用字符数直接冒充实测 token。
- provider 如无可验证 tokenizer/输入限额策略，测试配置失败或使用经验证的保守估算；超长证据先按 claim 相关性裁剪，保留 locator。
- 每次请求原子锁预算，预占次数+token，再发网络；调用重试重新预占。所有 SDK 自动重试默认关闭。
- 未知 timeout/缺 usage：保留最坏预占，标 unknown；不自动退款重试。供应商费用以账单为准。
- 解析 ledger 确认后关联到 run 一次，不产生第二条收费记录；parse_id 不可多次确认。
- 研究者自报 usage 只做对账，实际次数取代理与 grant ledger。并发争最后额度只允许一个成功。
- 单次模型请求超时 min(45秒,剩余deadline)；工具 15秒；180 秒是本地总执行截止，不能保证供应商终止。

阶段B模型配置允许 `openai_chat_completions` 与 `openai_responses` 两种协议；每个版本固定一种，切换必须保存新版本并重新测试，不能自动探测/回落。用途 parser/synthesizer/verifier/research 可映射同一供应商。Go 与 Python 共用同一版本解析和 BudgetService；Python API Key 实际是绑定 task 的临时 token，真正供应商 Key 不出 Go。两种adapter与真实验证范围以阶段B SPEC B-1.1及其验收子矩阵为准。

后台测试必须跑 Go 结构化解析、Python tool-call、无证据样例、错误响应和超时路径。测试本身有管理员独立小额度并计账。任何 key/base_url/model/policy 修改后旧测试失效；运行中切换不影响旧 run。模型 root key 从环境/密钥管理服务注入，用 AES-GCM 等成熟库加密，轮换用 key_version；禁止自制密码算法。

仅允许运营预先允许的供应商主机，禁止内网/localhost/link-local/元数据地址、重定向跳转与 DNS 重绑定。内部 Go↔Python 服务有单独私网 allowlist，不可因 SSRF 防护误阻正常内部请求。所有请求体日志排除 Key、用户观点、源文和 Prompt。

观测：trace_id→run_id→task_id→request_id；记录排队/模型/工具/总耗时、取消时延、ledger unknown、schema拒绝、引用错误。非流式 P0 不承诺 TPOT；TTFT/TPOT 只有供应商提供流式测量时才报告。吞吐报告注明全局 2 run/4 role 的配置、样本、模型、输入长度；不把 TPS 理论值当产品 QPS。

## 9. 金融证据与发布 Harness

结构闸门：Schema、归属、task/result/run 一致、只引用已授权 evidence_id、source policy 白名单。外部网页/PDF正文是数据，里面的“忽略规则/上传密钥”等文字无指令权限。

时间闸门：published_at <= as_of，available_at <= as_of，历史版本必须能证明当时可获得；retrieved_at 可晚于 as_of，但不能据此把今天的修订值当历史值。无 point-in-time 版本支持就拒绝历史精确判断。时区统一。

数值闸门：metric+period+unit+actual/estimate 必须匹配，Decimal 计算并保留输入 evidence_id、公式和精度；零分母/缺数据不计算。文本中的重大数字需可映射结构化指标/原文定位，映射不了就移除或标未知；不能只跑 JSON Schema 就宣布数字可信。

引用闸门：原文 hash=SHA256(UTF-8规范化后的保存原文)，locator 页/段/字段可定位；hash 仅证明一致性，不证明真实性。推断引用前提证据并显式标 inference；assumption 不伪装来源。

语义复核：同一证据是否真的支持主张？增长是否被错误外推成股价必涨？是否把反方缺证据当正方获胜？是否遗漏已发现的重大相反材料？模型复核辅助，不是最终真值。
未通过最多修复一次并重跑程序+语义闸门；仍不通过不发布完整报告。

RAG P0：一个已授权正文来源+供应商检索/小型 PostgreSQL 全文检索即可；中文检索效果需单测。按标题/段落和自然小节返回片段，保留文档ID/原URL/页段位置；不把指标表格切成丢表头的文本。先结构化财务数据，再正文补充。无需为了演示强上 pgvector/GraphRAG 或整库 PDF OCR；未来加向量召回必须保留相同证据契约。

最低质量集 30 个固定案例：事实正确、缺证据、冲突、未来泄漏、单位/期间、预测与事实、注入各有覆盖；至少 10 个反向/不足案例。关键数值和时间/权限硬错误为 0 才能进入受邀测试；语义支持由人工标注核对并记录样本准确率，不能承诺零幻觉。

## 10. 分步实施任务与测试顺序

所有任务先写失败测试，确认失败原因，再最小实现，跑通过；不要修改 references/ 让测试迎合错误。每一步记录命令、退出码、已知限制到 IMPLEMENTATION_STATUS.md。以下是未来应用的验收命令，不是本轮已执行结果。

每个 T1–T7 的执行卡必须逐项勾选；T0 用基线检查替代红绿测试：

- [ ] 建立本任务列出的文件/接口与失败测试，不先写全部业务实现。
- [ ] 运行本任务测试，保存预期失败及原因；编译/依赖失败不可当作业务红灯。
- [ ] 实现最小变更，使本任务测试通过，不扩展一期范围。
- [ ] 跑本任务测试和受影响的回归测试；权限/预算任务加并发测试。
- [ ] 更新 IMPLEMENTATION_STATUS.md 与案例映射；如已有开发 Git 仓库可做本地小提交，不自动推送。

组件接口基线（类型字段从共享 JSON Schema 生成/对应，不允许 Go/Python 各自发明字段）：

```go
// app/server/service/finance/interfaces.go，待开发；这里只约定接口。
type ResearchClient interface {
    Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error)
    Get(ctx context.Context, taskID string) (TaskSnapshot, error)
    Cancel(ctx context.Context, taskID string) (TaskSnapshot, error)
}
type EvidenceGate interface {
    ValidateRole(ctx context.Context, run RunSnapshot, result ResearchResult) (ValidatedRole, error)
}
type BudgetService interface {
    Reserve(ctx context.Context, request BudgetRequest) (Reservation, error)
    Reconcile(ctx context.Context, reservationID string, usage ObservedUsage) error
}
type ReportPublisher interface {
    // expectedVersion 防止取消/删除后晚到发布；实现内开启数据库事务。
    Publish(ctx context.Context, runID string, expectedVersion int64, report VerifiedReport) error
}
```

TaskReceipt={task_id,status}；TaskSnapshot 为第6节 GET 响应；RunSnapshot 为冻结的 run 数据；ValidatedRole 只能由闸门构造，含通过的论证与证据；BudgetRequest 至少含 request_id/run_id/task_id?/purpose/kind/body_hash/reserved_input/reserved_output；ObservedUsage 含 actual_input?/actual_output?/unknown/upstream_request_id?。所有方法接受剩余 deadline/cancel context，error 必须区分 validation/budget/timeout/transport/canceled，不吞成空成功。

### T0：基线与授权关口

读取 manifest，验证 8 个 SHA、origin、工作树；检查 GoSaaS 授权。获授权后从 references/gosaas 克隆到 app，origin 保留读取但不 push；保留现有改动，不覆盖已有 app。
检查 Go1.24/toolchain、Python3.12、Node runtime；web 快照未包含依赖锁文件，第一次解析依赖后生成并提交 package-lock.json，再使用 npm ci。不得宣称依赖已经锁定。
运行 server 的 go test ./...、web 的 npm install / npm run build；上游基线失败独立记录。已观察到 announcement/gen.go 的行尾 go:generate 写法，若重现错误，仅在 app 最小修复，不能预先声称构建通过。

### T1：契约、数据库、状态机

文件：model/finance、migrations、research_service、worker、API。
测试：TestCreateRunIdempotency、TestOwnerIsolation、TestConfirmDraftOnce、TestCancelPublishRace、TestDeleteLateWrite。
先用假 ResearchClient，确保 queued→终态、刷新/历史/删除链路；数据库用真实 PostgreSQL 测事务，不用 SQLite 代替并发验收。

### T2：统一配置、预算、代理

文件：config_service、budget_service、model_proxy，管理 API。
测试：TestConcurrentBudgetReservation、TestUnknownUsageRetained、TestFrozenConfig、TestSecretNeverReturned、TestSSRFBlocked。
用本地 fake upstream 模拟超时/usage缺失/429/恶意跳转，确认重试不绕预算；真实供应商 smoke 仅在凭据具备时执行。

### T3：DeerFlow 兼容性关卡

文件：research-service 的 adapter/policies/jobs，固定依赖组合。
测试：test_actual_harness_import、test_exact_tool_allowlist、test_role_context_isolation、test_no_global_credential_mutation、test_task_replay_after_restart。
先红后绿验证真实工具清单与两个 token 隔离；失败则报告具体上游限制，不能用 mock 冒充真实 Harness。
使用独立 execution SQLite 和协程信号量 4；任务状态持久化，取消/重启可测试。

### T4：金融 Provider 与证据

文件：tools、evidence_service、data-query 出口。
fixture provider 必须经过真实授权/登记链路，只替换外部数据输入；接入真实 provider 后冻结 data_source_version。
测试：test_future_evidence_rejected、test_estimate_not_actual、test_period_unit_mismatch、test_unregistered_citation、test_zero_denominator、test_prompt_injection_no_side_effect。
AKShare 仅作可选样例，不能把其公告标题接口当正文来源，不把代码许可当数据再分发授权。

### T5：Eino 双分支与发布

文件：workflow/nodes/research_client/report_service。
测试：TestTwoIndependentRoles、TestOneSideFailureIncomplete、TestBothInsufficientCompleted、TestRepairAtMostOnce、TestBudgetDeadline、TestNoPublishAfterCancel。
离线失败路径跑通后，真实模型+真实数据验证一条研究，留下脱敏 trace；不保存供应商密钥或用户私有观点作为公共 fixture。

### T6：客户界面、后台、追问

文件：上述 Vue pages/layout/components/api/router。
交互端到端：登录→输入→确认→进度→报告→证据→追问→历史→取消/删除；手机宽度 390px 可操作，无水平溢出。
状态由后端驱动，刷新不丢；确认重复点击只创建一个 run。普通用户不能请求 admin API。断网给出可重试提示，不伪造完成。
追问不调用外部工具；用户提出新事实问题返回不足+重新研究入口。

### T7：内测验收与可启动交付

补 deploy/compose.yaml、.env.example、README（仅允许测试账号、模型设置、数据设置、fixture/live切换）、备份与恢复命令。
实现后必须提供：
- server：go test ./... 与 finance race/integration tests；
- research-service：uv run pytest，实际 Harness tests 不可全 skip；
- web：npm ci、npm run build；新增 test:e2e 脚本跑浏览器用例；
- handoff 的全部验收案例映射到测试 ID；
- 一个固定示例研究的 live 验证，或清晰说明凭据缺失仅达到阶段 A。

建议未来 CI 命令（在各目录执行，先由开发者创建对应测试/脚本）：
```sh
go test ./...
go test -race ./service/finance/...
uv run pytest tests -q
npm ci
npm run build
npm run test:e2e
```

## 11. 本次交付的验证与后续完成定义

本次附带的 handoff/tests 只验证契约、样本和参考仓库一致性，不调用真实模型、不运行 GoSaaS、不等于应用 E2E。
可执行：
```sh
python3 -m venv /tmp/finance-spec-check
/tmp/finance-spec-check/bin/python -m pip install -r handoff/requirements-validation.txt
/tmp/finance-spec-check/bin/python -m unittest discover -s handoff/tests -v
```

最终编程 Agent 必须交回：可启动 app/、实际依赖锁、迁移、测试结果、真实/离线模式说明、真实数据覆盖范围、模型配置步骤、已知风险、未完成清单。不得只交漂亮页面、通用 DeerFlow 聊天窗口，或宣称“接上模型=完成金融 Agent”。

确认项目权限、数据和运行环境前，不擅自代用户作商用授权或上线判断。
