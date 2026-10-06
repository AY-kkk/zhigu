# 期货研究模块 · 开发与交接 SPEC

版本 F-SPEC-1.0｜2026-10-04｜产品基线：[PRD F-1.0](../prd/期货研究模块_PRD.md)。

**给接手 agent：先阅读本节和 §14，再运行 §13 的骨架验证。不要把已有骨架当成已完成功能。按 M1 → M8 开发，每一阶段通过后再继续。** 本文、[机器契约](../contracts/futures/v1/openapi.json)、PRD 和测试共同构成开发合同。若它们冲突，先记录差异并修正文档/契约，不能静默选择较宽松规则。实现状态以 §2 为准，所有上线门槛仍未执行。

用户已授权产出 SPEC 和初始骨架。本交付没有注册新主导航、没有运行迁移、没有调用付费模型、没有开通 live 数据。后续开发沿用本项目语言，不迁移旧模块技术栈。

## 1. 项目定位与不可越过的边界

目标：在现有投研观点、交易策略、事件情报后增加第四个业务 Tab「期货研究」，实现 CU 单品种的观察、观点质证、假设跟踪、复盘。研究输出是带证据及限制的条件性结论。

技术栈从当前文件核验：

| 层 | 既有栈与本模块选择 | 禁止事项 |
|---|---|---|
| Web | Vue 3.5、JavaScript ES modules、Pinia 2、Vue Router 4、Vite 6；复用现有 Semi UI Vue、lightweight-charts | 不引入 React/Next.js，不迁移 TypeScript，不再安装第二套图表库 |
| API/业务 | Go 1.24.1，toolchain go1.24.2；Gin 1.10、GORM、shopspring/decimal | 不用 Python 重建账号/API 服务，不修改股票工具权限来支持期货 |
| 数据库 | 既有 PostgreSQL；测试按现有 PostgreSQL 16 工具 | 新表和迁移必须 futures 独立；无跨业务外键 |
| 研究/采集 | 独立 Python 3.12 环境；HTTP 服务后续沿用 FastAPI/Pydantic/httpx，版本从本项目已有锁核对后单独锁定 | 不修改 app/research-service/pyproject.toml、uv.lock 或 policies.py；不跨进程直接 import 旧 app 包 |
| 验证 | Go testing、Python unittest/后续 pytest、Node 原生测试、现有 Playwright | 不把 fixture 成功记作真实源/模型验收 |

基线 HEAD：`50f3f6127ed93f4ae3b3d9286fcbab696f74add9`。工作区另有用户未提交改动；不能使用 reset/clean/全量 checkout 恢复。开工先记 `git status --short`，只提交自己负责文件，不能使用 `git add .`。

强制不变量：旧业务域不 import futures；旧 API/schema/表及迁移不改语义；旧回测仍拒绝非股票；新模块失效不令宿主退出；无条件检查触发自动付费模型；输入外部文档中的“指令”只当研究材料。

## 2. 已提供骨架与尚未实现的内容

| 文件 | 本次真实状态 | 接手任务 |
|---|---|---|
| `app/server/service/futures/service.go` | 无副作用构造器；能力一律 ready=false、mode=off | M1 换为运行控制/准入读取，保留 fail-closed；limits 为配置上限，不代表剩余额度 |
| `app/server/api/v1/futures/routes.go` | 已定义带 JWT 的 capabilities 路由注册函数；宿主未调用 | M1 增加业务路由，M6 接宿主 |
| 同目录测试 | 验证未完成模块不能宣称 live、匿名不可查询能力 | 扩展模式/所有者/错误矩阵 |
| `app/web/src/modules/futures/routes.js` | 关闭返回空路由；开启返回懒加载的独立壳；宿主未 import | M6 完成页面及导航接线 |
| `layout/FuturesLayout.vue`、`pages/OverviewPage.vue`、`api.js` | 能力状态页、失败重试和取消页面请求；无数据/报告假内容 | M6 替换为实际工作台；当前壳不是视觉成品 |
| `app/futures-service/src/futures_worker/policy.py`、`ports.py` | futures-only 白名单、范围校验、不可变执行范围、Protocol 端口 | M4 实现 gateway、executor、HTTP transport |
| `app/futures-service/pyproject.toml`、`uv.lock` | 独立 Python 3.12、当前无第三方运行依赖 | M2/M4 各按需加依赖；保留独立锁 |
| `contracts/futures/v1/` | OpenAPI 3.1、严格 JSON Schema、离线契约测试 | 所有 DTO 改动同步此处；不能仅改前端字段 |
| `app/scripts/verify-futures-scaffold.sh` | 仅验证骨架与契约，不验证 P0 上线 | M8 另建 verify-futures.sh |

**未实现**：数据库、真实来源、迁移、任务队列、模型调用、报告核验、文件解析、假设、通知、导出、管理后台、宿主接线。不得写一个返回 success 的空实现填满接口，也不得让 UI 的“已接通”依赖环境变量 alone。

## 3. 目录、依赖方向与文件职责

以下是目标目录，标注“待建”的文件需按阶段创建，不要一开始建几十个空文件。

```text
contracts/futures/v1/              已建：openapi.json、types.schema.json、分对象 schema
app/server/
  api/v1/futures/                  已建注册；待建 drafts.go/runs.go/hypotheses.go/admin.go/internal.go
  service/futures/                 已建 service.go
    bootstrap.go                  待建：局部初始化、runtime gate、worker 生命周期
    ports.go                      待建：本节定义的领域端口
    repository.go                 待建：owner/mode scoped 读写
    drafts.go / documents.go       待建：revision、材料解析、配额
    runs.go / scheduler.go         待建：幂等创建、lease、取消、重启恢复
    observations.go / formulas.go  待建：准入、修订、Decimal 计算
    reports.go / verifier.go       待建：冻结清单、引用/数字校验、原子发布
    hypotheses.go / checks.go      待建：版本化观点、确定性条件检查
    outbox.go / cleanup.go         待建：幂等通知、删除/保留
    gateway.go / model_gateway.go  待建：独立 grant、费用预留、模型协议
    adapters/model_config.go       待建：只读公共模型配置映射，不复用旧 run
    migrations/001_init.sql        待建：仅 futures_* 表；独立迁移锁/版本表
  model/futures/                  待建：领域 DB 记录，明确 TableName()
app/futures-service/
  src/futures_worker/
    policy.py / ports.py          已建
    api.py / executor.py          待建：FastAPI 入口和有界研究流程
    gateways.py / extract.py      待建：Go 工具/模型入口客户端、受限材料提取
    providers/base.py             待建：Provider Protocol，独立采集流程
    providers/registry.py         待建：许可和品种模板驱动注册
  tests/                          离线政策、契约和以后新增的真实源探针
app/web/src/modules/futures/
  routes.js / api.js              已建，后续扩展
  layout/ / pages/                已建壳；待建 Product/NewResearch/Run/Hypothesis/Notifications/Sources
  stores/workspace.js             待建：本域选择和缓存；无股票 state
  components/                    待建：QualityBadge/EvidenceDrawer/ConditionEditor/Report
```

最小业务端口在 M1 创建 `ports.go`；语义固定如下，输入 DTO 直接对应机器契约，不复制第三套字段：

```go
// DTO types map exactly to contracts/futures/v1/types.schema.json.
// Context cancellation must reach DB, HTTP and model transport.
type Identity struct { OwnerID uint; Mode string }
type Clock interface { Now() time.Time }
type ModelConfigReader interface {
    Resolve(ctx context.Context, versionID string) (ModelConfig, error)
}
type ResearchWorker interface {
    Execute(ctx context.Context, task Task, grant string) (Report, error)
}
type ObservationReader interface {
    Snapshot(ctx context.Context, identity Identity, productID string, asOf time.Time) (Manifest, error)
}
```

`Task/Report` 是契约同名 DTO；`ModelConfig` 仅服务端字段 `VersionID, Provider, BaseURL, Model, SecretRef string`，不 JSON 序列化凭据；`Manifest` 为 `ID string, RecordIDs []string, SourceVersions map[string]string, AsOf time.Time, Hash string`。这三个端口无需通用插件框架。仓储首版用具体类型，避免一表一个空接口。

Go 负责最终授权、预算、状态、事实与计算核验、发布；Python 负责受限解析/分析及数据 adapter。Python 不能直写业务数据库、不能持有用户 JWT 或供应商模型密钥。数据采集与研究执行为不同队列/入口，避免工具递归启动研究。

## 4. 契约规则与 API

### 4.1 单一字段定义

[openapi.json](../contracts/futures/v1/openapi.json) 给出方法、路径、请求、成功状态及 envelope；[types.schema.json](../contracts/futures/v1/types.schema.json) 是字段/枚举的唯一结构定义。`record/task/report/hypothesis/capabilities.schema.json` 为分对象入口。`additionalProperties=false`，未知字段拒绝；Decimal 是十进制字符串，不能使用浮点 JSON number 表示价格/库存/费用。

UTC 时间 RFC3339；交易日和统计期间为 YYYY-MM-DD；整型 owner_id 来自 JWT。`mode=live` 由服务端绑定，不接受客户端指定。开发 fixture 使用测试适配器，不能提供匿名 demo 路由或生成可复用 live grant。

成功 `{data,error:null,trace_id}`；失败 `{data:null,error:{code,message},trace_id}`。Go 沿用 httpx。业务错误用 FUTURES_ 前缀，公共 UNAUTHENTICATED/FORBIDDEN 保留。所有列表稳定按 `(created_at,id)` 降序，默认 20、最大 100；公开合约按到期顺序另提供排序；cursor 签名并绑定 owner/mode/filter，不能跨账号使用。`products/{id}` 中 ID 例如 SHFE.CU，实际合约来自目录，禁止推测合约代码。

### 4.2 接口补充行为（与机器契约一起实现）

| 接口 | 服务端必须做的事情 |
|---|---|
| GET capabilities | 任意已登录用户可查；products 仅该用户获准项。enabled=配置允许，ready=模块基础设施就绪，mode=有效控制模式；三个值不能互相替代 |
| GET products/contracts/workbench | 校验用户准入、品种模板；不得调用 stock catalog；缺口以 null+reason 表示 |
| POST documents / GET documents/:id | multipart 1 文件；202 返回提取任务。类型魔数、大小、页数、解压及超时检查；查状态必须 owner scope |
| POST/GET/PATCH drafts | POST 201；PATCH 完整 input+expected_revision。成功 revision+1、parsed_revision=null。拒绝并发覆盖 |
| POST drafts/:id/parse | 202；只为该 revision 解析；迟到结果不得覆盖新 revision。解析调用计入草稿累计预算 |
| POST runs | body 不接受 as_of/owner/model/prompt/tools；服务端冻结；202 返回完整 Run；未完成解析返回409 |
| GET runs/:id | 每2秒轮询；report 只有 succeeded 且发布校验通过才非 null；失败/取消保留输入和原因 |
| POST runs/:id/cancel | 202；终态任务幂等返回当前对象；执行中的取消先 canceling，不能抢先释放执行槽 |
| GET evidence / export | 必须先证明 owner 可访问 run，再证明 evidence 属于冻结清单；导出重新核查当前再分发权 |
| POST/PATCH hypotheses | 创建只能引用本人 succeeded 的报告；至少1验证+1失效，各最多5；版本冲突409；动作按 §8 校验 |
| POST hypotheses/:id/checks | 仅 manual 条件；程序指标不可人工覆写；reason 和出处/不足说明必须留存 |
| PUT hypotheses/:id/recap | URL 身份与 body 一致；version 是现有复盘版本，首次为1；成功递增，旧版本追加保留 |
| GET delete-impact + DELETE runs/:id | 预览所有关联假设及版本哈希；删除时锁定并复算，变化409；未确认 cascade 时有引用则409 |
| DELETE drafts/documents | 有有效研究引用则409 FUTURES_REFERENCED，返回可读提示通过关联研究删除；禁止留下无证据的活跃研究 |
| PUT watchlist | 全量替换，expected_version 乐观锁；仅已准入品种，≤20；删除关注不删研究 |
| PATCH notifications/:id | 只设当前用户的 read_at；读通知不自动清除假设 needs_review |
| admin sources/admission/operations | 双重 JWT/admin；不返回用户正文；所有修改 expected_version/审计；不能依靠前端隐藏按钮保护 |

RunCreate.claims 为用户选中的解析候选，必须与当前 parsed_revision 中 ID、kind、text、locator 精确匹配；允许子集≤12，不接受任意附加主张。用户编辑主张通过修改原输入并重新解析。候选超过12时 `claims_overflow=true`；读取候选分页后用户选择，不能静默丢弃候选。

工作台增加 query `contract_id`（目录实际合约）、`window=20|60|120`（默认60）、`price_type=settlement|close`（默认 settlement）。这些参数进入缓存键、输入可比性和计算证据，切换后整体刷新。缺少对应实际合约存续天数只显示有效范围。

### 4.3 HTTP 错误精确映射

| HTTP | code / 条件 | 是否自动重试 |
|---|---|---|
| 400 | FUTURES_INVALID_INPUT：未知字段、文字/期限/条件非法、extra JSON 尾部 | 否 |
| 401/403 | 公共认证错误；FUTURES_NOT_ADMITTED 未准入 | 401 转登录；不自动创建研究 |
| 404 | FUTURES_NOT_FOUND：不存在、非本人、已隐藏对象 | 否；三者相同响应 |
| 409 | FUTURES_REVISION_CONFLICT / IDEMPOTENCY_CONFLICT / REFERENCED / INVALID_TRANSITION | 刷新状态或修改请求；不覆盖 |
| 413/422 | FUTURES_DOCUMENT_LIMIT / DOCUMENT_UNREADABLE | 换材料 |
| 429 | FUTURES_QUOTA_EXCEEDED / RATE_LIMITED | 返回 Retry-After；不后台重试付费操作 |
| 503 | FUTURES_DISABLED / UNAVAILABLE / SOURCE_UNAVAILABLE | 保留草稿；查询可重试，研究由用户再确认 |

幂等键为 owner+mode+operation+Idempotency-Key，保留24h；请求体用规范化 JSON 哈希。相同键相同哈希返回最初 HTTP 状态和对象身份，相异409；失败校验不能永久占用键；已接受的任务失败仍返回原任务。作用域唯一约束与对象创建在同一事务内，不用进程内 map 替代。

## 5. 数据库存储与迁移

### 5.1 总体规则

使用独立 `futures_schema_migrations(version bigint PK, checksum text, applied_at timestamptz)`。新迁移用事务+独立 advisory lock；checksum 不符仅关闭 futures。不放进旧 `initialize.Migrate` 的无条件迁移列表。业务开关关闭不执行迁移，但已有部署的取消/删除/清理控制路径仍挂载；首次从未安装时不存在的私有对象返回404。

每张私有业务表含 `id text PK, owner_id bigint NOT NULL, mode text CHECK(mode='live'), created_at timestamptz NOT NULL, deleted_at timestamptz`。所有 SQL WHERE 必须含 owner/mode，内部任务额外校验 domain/run/generation。不可用“先按ID查再只在前端过滤”。时间均 timestamptz，值 numeric(38,12)，JSONB 仅放版本化结构，不把全部业务做成无约束 JSON。

### 5.2 目标表及唯一约束

| 表（全部前缀 futures_） | 特有字段与关联 | 唯一键/重要索引 |
|---|---|---|
| products / product_versions | product_id、template_version、name、exchange、指标定义、准入状态 | product_id；(product_id,version) |
| contracts | actual ID、product、last_trading_at、price_precision、multiplier、calendar/source_version | (contract_id,source_version)；product+expiry |
| sources / source_versions | publisher、credential_ref、rights、发布规则、覆盖、限流、分页/adapter版本 | source_id；(source_id,version) |
| observations | record契约；scope、source natural key、revision、supersedes_id、numeric value、usable_at | (source_id,natural_key,source_version,content_hash) 去重；series_id+period+usable_at |
| series | product/contract、metric、unit、currency、caliber、频率、日历版本 | 完整口径hash唯一；不可仅metric唯一 |
| manifests / manifest_records | frozen as_of/hash/versions、有序 record_id 列表 | manifest_id+record_id；as_of |
| documents | 私有文件key、hash、提取状态、页数/字数、purge_at | owner+mode+id；purge_at |
| drafts / draft_versions | current_revision；input、parsed_revision、parse_state、claims、累计usage | (draft_id,revision)；owner+updated_at |
| runs | draft/version、manifest、status/stage、generation、lease_until、deadline、cancel_requested、model_versions | owner+mode+created_at；status+lease_until |
| reports / evidence_links / calculations | run、report版本、核验结果；冻结record引用；formula/inputs/value | (run_id,report_version)；(run_id,record_id) |
| hypotheses / hypothesis_versions | current_version、lifecycle、expires_at、retention_deadline、user_view | (hypothesis_id,version)；owner+lifecycle；expires_at |
| conditions / checks | hypothesis_version、condition_id、定义；结果、数据revision集、check_version | (hypothesis_id,hypothesis_version,condition_id,input_hash,formula_version) |
| recaps | 四维复盘正文、用户理由、版本 | (hypothesis_id,version) |
| change_events / outbox / notifications | event、dedupe_key、payload、attempts、next_attempt、read_at | notification.dedupe_key unique；outbox pending+next_attempt |
| watchlists | product_ids、version | (owner_id,mode) unique |
| idempotency_keys | operation、key、request_hash、response/status、expires_at | (owner_id,mode,operation,key) unique |
| budget_accounts / reservations / usage | 日账户、limits、reserved/spent、attempt_id、pricing版本、usage_state | account(owner/module,date)；attempt_id unique |
| operations / admission | runtime配置version、mode、用户白名单；品种门槛证据 | version递增；product+version |
| deletion_jobs / tombstones / audit | 删除对象清单、purge阶段、复核证据；不可重建正文的审计 | request_id unique；purge_due_at |

内部外键必须包含足够身份约束（例如复合 UNIQUE(owner_id,mode,id) 与复合 FK），避免只在应用层假设子对象同 owner；公共观测用明确 scope 连接。来源无替代关系冲突并存，不用简单 upsert 抹掉版本。

示例（M1 照此事务语义实现，字段以类型契约补齐）：

```sql
CREATE UNIQUE INDEX futures_idempotency_scope
ON futures_idempotency_keys(owner_id, mode, operation, key);
-- 同一个事务中：锁账户/草稿，检查revision，插入幂等记录，插入run，写outbox。
-- 对象乐观锁：RowsAffected 必须为 1，否则返回 FUTURES_REVISION_CONFLICT。
UPDATE futures_hypotheses
SET current_version = current_version + 1
WHERE id = $1 AND owner_id = $2 AND mode = 'live'
  AND current_version = $3 AND deleted_at IS NULL;
```

数据库连接使用期货独立 pool（同库新连接，max open=4、idle=2）；Go API 不逐请求开连接。Python 默认不直连DB。新增 worker 不能占旧 pool 的所有连接。

## 6. 数据接入、时间与公式

来源先登记再执行，登记清单为 `contracts/futures/v1/source-manifests/<source_id>/<version>.json`，通过 schema 和探针后才激活。必须包含 publisher、URL/定位、rights（展示/模型/导出/保存）、secret引用、metric映射、发布日历/时区/精度、分页停止条件、历史覆盖、超时、速率、重试、adapter版本、责任人；不得写入真实密钥。

Provider 统一输出 Record。使用 `FetchRequest(source_id, product_id, metric, start, end, cursor)`，返回 `FetchPage(records, next_cursor, complete, coverage_start, coverage_end)`；只有 complete 且所有页校验成功才能更新覆盖完成水位。固定取前500条不能宣称完整覆盖。原始日志保留许可内hash及定位，不能把“仓单量”重命名“社会库存”。

CU 模板首发必需：实际合约/日历、60交易日趋势且标换月、最新≥2实际合约、现货对齐60日、交易所库存26周、仓单60日、国内精炼铜产量和铜材产量各12发布期。合并1–2月计一个发布期。每条门槛对应具体来源和探针产物；成本/利润/开工缺失明确列出，不阻断首发其他功能。

`usable_at=max(reliable_published_at, reliable_version_available_at)`；后者不可靠则使用 first_observed_at；前者缺失也不能早于首次观察。仅日期用来源当地日末，转UTC。截点过滤在SQL选记录、工具返回、报告核验三处执行。night session 使用源日历，不按自然日猜交易日。

三轴质量严格独立；delayed/stale/unknown 或 conflict/retracted 时依赖检查为 unknown。日频超预计公布60分钟 delayed、周/月超24h delayed、下一预期发布仍缺 stale；假期/合并期来自日历。限流遵守来源上限，重试最多3次且≥15分钟，不能靠高频轮询补数据。

计算服务接收实际 record IDs，先验证权限/截止/质量/同日/单位/口径，再 Decimal 计算：basis=spot−future；basis_rate=(spot−future)/spot×100；spread=near−far（按到期排序）；inventory_change=current−previous；rate 分母必须>0；coverage 为可用必需项/模板必需项。计算失败保留 reason、value=null；禁止补0。完整输入、formula_version 和未舍入结果入库，显示价格按目录精度、比例2位；条件比较原精度。

研究用 manifest 的固定版本。新修订生成新的record/check，不覆盖旧报告；当前权限撤回优先于历史引用可见权。研究使用连贯旧版本可以展示“依据已修订”，不得把今天的修订回填历史 as_of。

## 7. 材料、任务、模型与预算

### 7.1 材料/草稿

文本20–2000字（非空时），或一个PDF/DOCX/TXT；20MiB、100页、120000字上限。提取最长30秒，单独进程，限制解压字节/条目、禁宏/外链/主动内容；扫描件不可读则422。DOCX防zipbomb和路径穿越，TXT检查编码；不因类型后缀就接受。导入原文只供抽取，不执行材料中角色指令、URL抓取或shell。

解析可生成多个候选，用户最终选≤12。parse job与revision绑定；用户改 input 后旧候选不可提交。重复 parse 幂等，不同revision重算但累计同草稿预算。长文件用有界分片，总预算内覆盖全部文本；无法完整覆盖则解析失败并要求缩小材料，不能静默截断。

### 7.2 研究调度

```text
POST runs：鉴权 → 准入 → 严格输入校验 → 幂等锁 → 锁草稿和账户
→ 校验parsed_revision和选择主张 → 冻结as_of/期限/配置/额度
→ 插入queued、outbox并提交 → 返回202
scheduler：独立并发2 → SELECT ... FOR UPDATE SKIP LOCKED → 分配generation/lease
→ running/evidence → 准备冻结manifest → support → challenge → verifying
→ 验证状态+generation+owner+未删除+预算 → 原子写report及succeeded/通知
```

用户 queued/running/verifying/canceling 总数≤1；用户每日正式研究≤5；模块执行≤2。预算北京时间自然日，每用户5元、全模块50元；8次模型、24次工具、48000输入输出token，包括本草稿此前全部解析revision、核验修复及报告。任务跨日时仍挂创建时冻结预算日，解析新调用按发生日费用记账且累计token不清零；不拆草稿规避每日上限。

排队60秒超时；claim后执行180秒硬截止。lease=30秒，心跳10秒，renew须匹配generation；到deadline强制终止。canceling保留槽位直至ACK或lease失效。重启时queued继续，running/verifying失效后failed，禁止自动重复付费。重试创建新的run；必须重新解析的草稿预算耗尽时由用户新建草稿，仍受日配额约束。

每次模型请求独立attempt_id，按配置费率和最大输入输出预留用户及模块费用（按固定锁顺序锁两账户）；完成仅一次核销。请求超时/usage未知保留reservation待对账，不释放后马上重试。费率不明阻止live；模型响应里的usage不能提升上限；超额即时停止后续调用。

### 7.3 内部端口和授权

M4 创建独立 FastAPI `POST /internal/futures/v1/execute`（task契约），成功200返回Report候选；`GET /healthz`仅报告该worker。Go HTTP deadline透传；Python不得把候选报告发布为最终成功。服务启动仅loopback/内网，独立服务token从环境读，拒绝缺失token。

worker访问Go：`POST /internal/futures/v1/tools`，body `{scope,task_id,generation,manifest_id,name,arguments}`；`POST /internal/futures/v1/model`，body `{scope,task_id,generation,stage,messages,max_output_tokens,attempt_id}`。前者只允许三种工具：get_observations(metric/series/record IDs)、get_evidence(evidence IDs)、calculate(formula/input IDs)。使用 `futures_` 前缀全名，与 policy.py 一致；参数必须由各工具严格schema验证，无任意URL/SQL/code。

鉴权由独立服务身份+每调用短grant共同完成，grant绑定 domain/owner/mode/run/task/generation/stage/tool/args_hash/expiry/nonce；60秒到期、nonce消费幂等。先冻结参数再向Go授权入口申请该次grant（服务身份+有效任务租约+模板允许范围），不能让worker自己签名；授权入口不能发出超manifest权限。每次调用复核off/取消/删除/租约/预算。任务JSON不含模型密钥或长效凭据，短grant只通过请求头传输。

Go模型端口只读解析已激活的公共模型配置并映射字段，独立HTTP客户端支持context取消和流量限额；不可直接复用 finance.ModelProxy.Complete 的 stock run/grant。配置adapter是唯一允许读取 finance 配置包的边界；finance不反向import。不得放大旧白名单。

### 7.4 双向质证与发布

support与challenge读取同manifest和原始主张，不读取对方未完成输出。顺序执行也满足独立性，不必为“多agent”额外并行付费。默认阶段预算解析≤2、支持≤2、反证≤2、汇总≤1、修复≤1，总≤8且服从草稿实际剩余量；无剩余额度时不开始不能完成的流程。

verifier验证：报告schema；scope/run/as_of/versions与task一致；claim_id属于选中项；引用全部在本run证据清单且仍授权；数字对应已登记计算/记录；无未来数据；条件单位及series可执行；八部分齐全；不含收益/仓位/自报置信度输出。数值声明应输出对应calculation/evidence关联，由程序逐项核对，不用字符串“看起来像数字”代替验证。

结构/引用错误允许一次修复，仍失败为failed；真实证据不足允许succeeded+insufficient_evidence，必须列缺口。模型失败、工具执行错误不是证据不足报告。报告正文不可覆盖，修订提示为旁挂元数据。

## 8. 状态转换、并发与通知

Run 与 lifecycle 枚举使用机器契约。任务事务更新都带当前状态和generation；取消与完成竞态由同一row lock决定，取消提交后任何迟到内容不得publish。

假设合法迁移：draft→active；active↔paused；active/paused→expired；draft/active/paused/expired→closed；未删除→deleted。expired/closed不能恢复，只能新建关联假设；deleted不能恢复。至少1验证+1失效，活跃上限20（active与paused共同计入，暂停不能绕过上限）。期限不能超报告研究期限及实际合约最后交易时刻，修改不能突破证据链保留上限。

Hypothesis PATCH 动作必填规则：edit提供proposition/expires_at/conditions且产生新语义版本；activate/pause/resume/close只携带expected_version/action；decide提供user_view/reason/reviewed_check_version；acknowledge提供reviewed_check_version。不属于该动作的额外字段400。返回完整新版本；备注/复盘独立记录，不假装证据变化。

metric_compare比较本期原值；metric_change比较本期减紧邻前期的绝对变化（阈值单位相同；首版不做任意表达式）；consecutive_periods为最近N个连续发布期的逐期绝对变化都满足operator/threshold，因此至少需要N+1个观测；manual由用户登记。跨口径/缺期/冲突/撤回/时效不足→unknown，不能跳过缺期或补0。manual未知无需伪造证据，必须说明原因。

检查身份由hypothesis版本、condition、输入版本hash、公式版本组成。初次激活只记初值；恢复时只记录当前检查并注明恢复，不补放历史通知。新证据/结果变化/修订置needs_review=true；用户只能确认已看到的check_version，若并发新增check则不能清掉新待复核。程序永不自动把user_view改成retained/rejected。

同一事务写check+change_event+outbox；outbox至少一次投递，notification dedupe_key包含owner/mode/hypothesis/version/condition/checkversion/type。退避10s/30s/60s/300s重试，超10次进死信并告警，补投沿用dedupe_key。source故障按用户+来源+故障周期去重。新观测登记后5分钟内通知可见，扫描每30秒，批处理最多2000条活跃假设，分批且不占研究执行槽。

到期计时在paused期间继续；到期停止普通检查，生成复盘项；相关历史修订可标注但不重开假设。复盘四字段为事实兑现、传导支持、实际合约表现、数据充分性，不以价格方向替代理论正确性。

## 9. 权限、删除和导出

私有正文30日；未关联文档/废弃草稿7日；活跃假设引用链保留到终态后30日但从链最初形成起绝不超过90日。新建假设引用旧报告继承原链起点，不能通过复制延长。达到90日停止跟踪并提示重新采集/研究；新链必须冻结新的有效材料。

删除先事务写tombstone、所有私有关联deleted_at、取消标记、清理任务；立即不可读，24h内从数据库正文/文件/索引/cache/worker副本清除；保留审计180日不得恢复原文。公共授权记录不随单用户删除；私有引用必须清。备份30日轮转，恢复先重放tombstones再提供访问。

off/read_only均保留鉴权后的cancel/delete、管理员operations和清理worker；read_only还允许历史授权读取及通知标已读，其余写入停止。模块表暂不可用不能谎称删除成功；返回503并告警，恢复后优先清理；不是扩大访问权的理由。管理员默认只能脱敏状态，用户材料查阅需具体工单授权及审计。

导出为临时生成HTML附件，Content-Disposition attachment、Cache-Control no-store、禁script/事件属性/远端资源；原文和模型文本严格escape。当前rights.export=false的原文、数值及可反推出受限值的计算结果均去除，留下受限说明及许可内来源定位；无临时公开URL，不持久存导出正文。

## 10. 前端实现及宿主最小接线

完整页面路径与PRD §3一致。未来仅允许修改这些宿主点（加注释说明范围）：

1. `app/web/src/router/finance.js`：import buildFuturesRoutes并在ConsumerLayout外展开。不得把新页面嵌入会加载stock catalog的旧布局；admin sources路由用独立layout并meta.requiresAdmin=true。
2. `app/web/src/components/workspace/SideNav.vue`：业务末尾增量加入 `{key:'futures',text:'期货研究',icon:'research',to:'/app/futures'}`，受VITE_FUTURES_ENABLED控制；加selected分支。保留旧research当前run跳转、intel新窗口逻辑。
3. FuturesLayout不要复用该有stock store副作用的SideNav，使用轻量纯菜单组件（只接items/selected/emits）和现有品牌样式；其研究入口可以返回旧research/new，不能清空旧store。
4. `app/server/main.go`：注册futures能力/维护入口，再局部bootstrap；失败日志+ready=false，不log.Fatal；配置关闭不初始化数据/模型/业务worker。不要调用旧Migrate来跑期货表。

开发接线示例（只有M1–M5基础通过后才执行）：

```js
import { buildFuturesRoutes } from '../modules/futures/routes.js'
// 放在现有顶层 routes 数组中，ConsumerLayout之外。
...buildFuturesRoutes(import.meta.env.VITE_FUTURES_ENABLED === 'true')
```

```go
// 这一行现在只返回关闭能力；实现 bootstrap 后再开放其他端点。
futuresapi.Register(engine, futuressvc.NewService(os.Getenv("ZHIGU_FUTURES_ENABLED") == "true"))
```

workspace store缓存key包含account/mode/product/contract/window/price_type；换账号立即清空并abort旧请求，响应回来还需比对请求代次。只在进入futures时请求，离开停止页面轮询不取消后台研究；重新进入恢复服务端状态。不要在localStorage放原文/报告/凭据。

页面验收必须覆盖：20/60/120日数据缺口；所有三轴状态中文说明；五个工作台页签；文件/解析确认；任务阶段/取消/失败；证据drawer聚焦返回；条件编辑及409刷新；复盘和删除影响确认；390px无整页横向滚动、1440px桌面；图表数值表可读。首版不展示排行榜/无证据看多卡片。

## 11. 源码借鉴和依赖控制

[调研](../docs/research/2026-10-02-futures-github-references.md)是固定时点证据，真正复制代码前核对对应commit/license；新增第三方NOTICE放期货模块自己的依赖清单，不恢复/覆盖用户已删除的根文档。

- TradingAgents_for_Futures：只参考角色拆分；不能复制按文字长度/引用数打置信度、fallback .75逻辑。
- AKShare：仅可替换provider；逐接口证明上游含义、分页、授权与时间，不把MIT当成数据再分发许可；基差方向以本SPEC为准。
- futures-research：所审commit无许可证，不复制实现；借鉴证伪/留痕方法。
- TqSdk：可选未来adapter；当前不要求购买或假装有历史权限。
- TradingAgents：借鉴角色独立和截止时间，不替换当前股票Harness。
- vnpy_ctastrategy：P2另立项，本次不接回测/下单。

## 12. 交付阶段（每阶段可单独验收）

详细可勾选执行清单见[实施计划](../docs/superpowers/plans/2026-10-04-futures-module.md)。不允许“先完成所有UI再补鉴权/数据”。

| 阶段 | 开发产物 | 通过条件 | PRD映射 |
|---|---|---|---|
| M0 已交付 | 本SPEC、严格契约、默认关闭骨架、验证脚本 | 骨架测试通过；声明未接宿主 | AR01–08 |
| M1 | DTO/业务errors、独立迁移/表/仓储、能力与runtime控制、owner/admin过滤 | 跨用户404；off维护可用；checksum/迁移失败局部隔离；乐观锁和幂等并发测试 | FR14–17、AR01–10；F16/F20/F38/F42 |
| M2 | CU模板、源登记adapter、观测修订/时效、公式、workbench | 60/26/12等覆盖实证；基差正负/同日/连续合约拒绝；未来/冲突拒绝 | FR05–08；F03–16 |
| M3 | 材料提取、草稿解析revision、预算账本/队列/lease | 超限/扫描/zipbomb拒绝；并发额度不超支；迟到/重启/取消隔离 | FR01/14/15；F17–20/F36–37 |
| M4 | worker transport、短grant、模型adapter、支持/反证、verifier、报告 | 数字引用截止精确；可用证据不足与执行失败区分；一次修复上限；无原文日志 | FR02/09/10；F21–26 |
| M5 | 假设/条件/复盘/outbox/通知/watchlist/清理/导出 | 缺期unknown；修订再复核；幂等通知；删除24h及导出限制 | FR03/04/11–13/16；F27–35/F38–40/F46 |
| M6 | 页面、host路由/导航/启动局部接线、管理员UI | 原三个业务行为保持；390px/1440px/键盘；开关两态无交叉请求 | D01–08、AR03/06/08；F01–02/F41–43/F45 |
| M7 | 真实源/模型准入、50案例、性能/灰度/回滚 | PRD G0–G6证据；不能自动填已通过 | 全F01–F47、NFR01/02 |
| M8 | 开发报告/维护手册/验收脚本/交接 | 干净安装按文档重现；无TODO挡上线；残余风险和状态真实 | G6 |

## 13. 验证命令、样例和测试要求

### 13.1 当前骨架可执行验证

从仓库根运行，需本机已安装Go1.24.2、Node22及现有npm依赖；Python指定3.12解释器。脚本不会下载依赖或连接模型/来源。

```bash
PATH="$HOME/sdk/go1.24.2/bin:$PATH" FUTURES_PYTHON="$PWD/app/research-service/.venv/bin/python" bash app/scripts/verify-futures-scaffold.sh
```

这里借用已有Python3.12解释器执行标准库worker测试，并只读使用其已有jsonschema做契约验证，没有向旧venv安装包。验证工具要求jsonschema 4.26.0；M1在期货独立环境添加该开发依赖并锁定。独立环境使用 `cd app/futures-service` 后 `UV_CACHE_DIR=/private/tmp/zhigu-futures-uv-cache uv sync --frozen --python 3.12`，再 `PYTHONPATH=src .venv/bin/python -m unittest discover -s tests -v`。Python依赖扩展后后续测试必须使用自己的venv。

### 13.2 M1–M8 必增测试（不得仅mock所有依赖）

| 测试文件（待建） | 固定输入/必须断言 |
|---|---|
| `service/futures/repository_test.go` | owner1创建run，owner2 GET/evidence/export/delete都404；admin默认也不可查正文 |
| `service/futures/migrations_test.go` | 新库安装/重复安装/checksum错；禁用不建表；损坏迁移只让模块不可用 |
| `service/futures/formulas_test.go` | spot80000、future79000→basis1000、rate1.25%；near79000 far79500→-500；跨日/单位不同比较拒绝 |
| `service/futures/observations_test.go` | as_of前/后一秒分别可用/拒绝；日期精度当日中午不可用；同一期revision变更保留两个记录 |
| `service/futures/budget_test.go` | 同用户双并发争最后1元；只一个预留成功；未知usage不退；cancel仍有已发生成本 |
| `service/futures/runs_test.go` | 同幂等键20并发只1run；不同hash409；过期generation回包0报告；取消后成功回包不发布 |
| `service/futures/verifier_test.go` | 另一owner evidence、未来记录、捏造计算、错误claim、fixture混live都拒绝；缺证据合法报告可成功 |
| `service/futures/checks_test.go` | 连续两期下降需3点；中间缺点unknown；初值met不发变化提醒；修订生成新检查 |
| `service/futures/cleanup_test.go` | 删除影响变化409；off仍删除；过期链停止；恢复备份先tombstone；导出不泄漏受限数值 |
| `api/v1/futures/internal_test.go` | 错owner/run/generation/manifest/args_hash、过期token、重放nonce、工具越界均拒绝 |
| `app/futures-service/tests/test_executor.py` | 两角色同manifest无互读；工具失败不伪装insufficient；提示注入不能加工具；repair最多1次 |
| `app/web/e2e/futures.spec.js` | 第四tab/旧窗口行为、表单revision、刷新进度、证据、版本冲突、off、账号切换、移动端 |

后续测试命令：`cd app/server && go test ./service/futures/... ./api/v1/futures/... ./model/futures/...`（model目录M1建立后）；`cd app/web && npm run build`；`npx playwright test e2e/futures.spec.js`（M6建立后）；旧回归依仓库已有 `app/scripts/verify-research-viewpoint.sh`、`verify-strategy.sh`、`verify-intel.sh`，先阅读运行要求，不以无服务环境的失败掩盖代码问题。

真实质量集50条；数值/引用/未来数据关键错误0；逐主张正确性≥90%；真实模型≥30次；20会话10RPS15分钟查询P95≤800ms、5xx<1%；额定并发2研究P90≤90s；旧模块P95劣化≤10%、错误率增≤0.1百分点。当前骨架测试不代表这些指标已达成。

## 14. 开工提示词与最终交付格式

将下面内容作为接手agent首条任务；不要仅发“看PRD做一下”。

> 在当前仓库实现第四业务Tab「期货研究」。依次阅读 prd/期货研究模块_PRD.md、spec/期货研究模块_开发SPEC.md、contracts/futures/v1/openapi.json 和 docs/superpowers/plans/2026-10-04-futures-module.md。先记录git status并运行骨架验证，再按M1到M8推进。语言为现有Vue JavaScript、Go、Python3.12，禁止改旧股票/策略/事件业务语义。当前仅有未接宿主的默认关闭骨架，数据/模型/任务链路均未实现。每阶段先写对应边界测试，完成后记录真实命令/结果，不能用假数据通过live门槛。遇到来源授权或真实凭据缺失，完成不依赖它的工程部分并保持该项blocked，不臆造接口或宣称已上线。每次更新同时维护契约、测试和实施清单；不要覆盖用户已有未提交修改。最终交付变更文件、验证证据、未通过门槛、回滚方法与维护文档。

验收产物目录为 `artifacts/futures/<run-id>/`，只保存脱敏输出/版本hash/测试结果，真实密钥和用户材料不能提交。交付摘要包含：实现到哪个M阶段、已通过测试、未运行的真实环境门槛、已修改宿主文件、兼容性证据、开启/只读/停用步骤。禁止用“全部完成”覆盖尚缺数据许可、模型探针或真实回归。

## 15. 本次交接验证记录

2026-10-04，完成的是M0骨架与交接文档。Go futures服务/API测试、Python两项政策测试、Node路由及两项Vue单文件编译测试已通过；JSON Schema定义/本地引用、5个合法离线样例及10个非法输入均通过验证。Go响应与共享capabilities样例也有一致性断言。

`npm run build` 已通过；构建输出存在大于500kB chunk提示，本次未修改现有bundle组织。新增页面尚未接宿主，因此它们另用Vue编译器验证，不能把宿主build通过当作新增页面端到端验收。

Go默认缓存写入曾受本机沙箱限制，切换临时GOCACHE后完成验证；验证脚本已采用可覆盖的临时缓存默认值。没有运行数据迁移、真实源/模型调用、浏览器端到端、旧业务全流程或负载/回滚演练。没有修改旧业务源文件或旧依赖锁。后续接线后须按M6/M7完成这些验证。

## 16. 2026-10-05 实现状态与证据边界

本次在基线 `50f3f6127ed93f4ae3b3d9286fcbab696f74add9` 上完成 futures 工程代码、42 操作路由矩阵、六个用户页面和管理员来源页、独立 Python worker、预算/运行状态原语、报告 verifier、假设/条件/导出脱敏逻辑及 M8 交付物。离线契约、Python 12 项、Go 非数据库测试、Node 10 项和 `npm run build` 已通过。

真实 PostgreSQL 测试与 Playwright 已编写但未在本机执行：沙箱拒绝绑定 `127.0.0.1`，自动审批通道又返回工具格式错误，不能将失败记为产品失败或通过。独立 `uv.lock` 已从现有已锁 Python 3.12 依赖闭包生成并可被 `uv export --frozen --offline` 读取；干净环境的 `uv sync --frozen` 仍需网络/缓存复核。

真实来源授权、真实模型 30 次、50 案例质量集、性能/旧模块回归、灰度、备份恢复和回滚演练均未执行。故当前状态是“离线工程实现完成，P0 live 门槛 blocked”，不能称“可上线”或“上线验收全部通过”。证据见 `artifacts/futures/2026-10-05-engineering/` 与 `docs/futures-operations.md`。

截至本轮续作，42 个公开操作均已接入严格 DTO 和运行模式闸门；文档解析、草稿/研究、并发调度、短 grant、受限工具、模型预算、报告核验发布、假设/检查/复盘、watchlist、通知 outbox、删除清理和脱敏导出均有实现代码。新增独立 HTTP provider 和 Pydantic 契约 fixture 测试。上述实现仍不替代 PostgreSQL 并发测试、浏览器端到端、真实来源/模型、性能与备份恢复验收；这些证据缺口继续阻塞 live。
