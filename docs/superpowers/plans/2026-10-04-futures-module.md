# Futures Research Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本任务已指定交给其他 agent；从 M1 开始执行，不需要再次询问是否开始。

**Goal:** 按 PRD F-1.0 实现第四个业务 Tab「期货研究」的 CU 研究闭环，保持旧三模块兼容。

**Architecture:** Vue 独立模块调用 Go futures API；Go 持有领域数据库、预算、授权和最终发布权；独立 Python worker 执行受限分析和来源 adapter。新模块以默认关闭的能力入口渐进接入，旧模块不依赖 futures。

**Tech Stack:** Vue 3 / JavaScript / Pinia / Vite；Go 1.24.2 / Gin / GORM / PostgreSQL / Decimal；Python 3.12 / 独立 uv.lock；Go testing / unittest / Playwright。

## Global Constraints

- 需求及精确接口：[开发 SPEC](../../../spec/期货研究模块_开发SPEC.md)、[PRD](../../../prd/期货研究模块_PRD.md)、[OpenAPI](../../../contracts/futures/v1/openapi.json)。先完整读取，不能仅凭本清单推断业务规则。
- 主功能代码不得批量重构；宿主修改只限 SPEC §10 的导航、路由和启动注册。
- 全部私有访问绑定 JWT owner、服务端 mode；假数据不能进入 live。
- 研究 8 次模型/24 次工具/48000 tokens、用户5元/模块50元每日限额；先预留后调用；unknown usage 不立即退款。
- 开发过程中不要提交全部工作区文件。仅在用户授权提交时分阶段精确 git add；当前交接未要求提交。
- 每个阶段先写能揭露该阶段错误的测试，观察失败，再实现，再跑对应命令；不得修改断言迎合错误实现。
- 不需要另建用户可见聊天；是否使用子 agent 遵守当次会话授权，不由本文件替代授权。

---

## M0：交接起点（已提供）

- [x] PRD 与当前 Vue JavaScript、Go、Python 代码栈核对。
- [x] 默认关闭且未接宿主的 Go 能力入口、Vue 独立壳、Python ports/policy。
- [x] 严格契约、离线样例、骨架验证脚本。
- [ ] 接手时执行 `git status --short`，记录他人已有改动。
- [ ] 在仓库根执行下列命令，确保环境可复现。预期只报告骨架通过，不能声称 P0 通过。

```bash
PATH="$HOME/sdk/go1.24.2/bin:$PATH" FUTURES_PYTHON="$PWD/app/research-service/.venv/bin/python" bash app/scripts/verify-futures-scaffold.sh
```

若本机没有上述 Go/Python 路径，使用本机对应版本的真实路径。脚本不负责安装。后续增加Python依赖后改用 `app/futures-service/.venv/bin/python`。

## M1：契约、存储、身份与运行控制

**Files:** 新建 `app/server/model/futures/{models.go,dto.go}`、`service/futures/{ports.go,repository.go,bootstrap.go,errors.go,migrations.go}`、`migrations/001_init.sql`；扩展已有 `service.go`、`api/v1/futures/routes.go`；新增 `repository_test.go/migrations_test.go`。更新机器契约时必须同步fixtures。

**Interfaces:** 输入 JWT user_id 和 OpenAPI DTO；输出 futures-only Repository、RuntimeState、DTO。`NewService` 仍无副作用；bootstrap 显式迁移并捕获局部失败；能力响应必须符合 `Capabilities` schema。

- [ ] 用 JSON DTO 测试严格字段和 Decimal 字符串；对于 unknown 字段/重复body尾部输入返回400。
- [ ] 建独立迁移版本和锁，按SPEC §5建表；迁移checksum不一致必须只返回 futures unavailable。
- [ ] 在真实PostgreSQL测试中写 owner1 对象，然后以owner2读取/删除；期待404且正文不存在于响应。每条写SQL带owner/mode；不是仅mock仓储。
- [ ] 实现 repository 和乐观锁/幂等事务。并发版本竞争仅1次成功，另1次409。capabilities/admin operations保留off可用。
- [ ] 将能力读取从常量改为真实runtime+准入，但数据/模型未完成仍ready=false；移除骨架always-off测试前先用真实门槛失败测试替换。
- [ ] 跑 `cd app/server && go test ./service/futures/... ./api/v1/futures/... ./model/futures/...`；全部通过后记录新增表和测试证据。

关键SQL行为示例（变量来自身份，不来自body）：

```sql
SELECT * FROM futures_runs
WHERE id = $1 AND owner_id = $2 AND mode = 'live' AND deleted_at IS NULL;
```

## M2：CU 数据模板、adapter 和可重复计算

**Files:** 新建 `service/futures/{observations.go,formulas.go,observations_test.go,formulas_test.go}`、`api/v1/futures/products.go`、`contracts/futures/v1/source-manifests/`；Python `providers/{base.py,registry.py}` 和逐源adapter/test。

**Interfaces:** 输入版本化 SourceManifest 和 FetchRequest；输出 FetchPage→Record→Manifest→Workbench。Go `ObservationReader.Snapshot(ctx,identity,productID,asOf)` 输出固定记录列表；source可替换而主流程不改。

- [ ] 先建立一个离线provider测试：第二页超时，complete必须false且覆盖水位不能推进；三页齐全且逐页校验才true。
- [ ] 登记CU指标源和实际权限，先只跑只读探针。不能假设AKShare字段名字即业务定义；记录分页、时间、修订、库存/仓单的映射证据。
- [ ] 实现first_observed/usable_at/修订和三轴状态；日期精度到当天末尾前不可用。
- [ ] 先写以下公式断言，再实现Decimal服务；相同用例加跨日/错单位/连续合约等拒绝断言。

```text
spot=80000, future=79000 => basis=1000, basis_rate=1.25
near=79000, far=79500 => calendar_spread=-500
previous_inventory=0 => change_rate.value=null, reason=ZERO_BASE
current=90, prior period missing => change.value=null, reason=MISSING_PERIOD
```

- [ ] 实现产品/合约/工作台API，query window/price_type/actual contract都进入缓存和计算身份。
- [ ] 跑Go模块测试、Python provider测试，保存60日/26周/12发布期覆盖探针。真实门槛失败不阻断后续离线工程，但live保持blocked。

## M3：文件、草稿、解析、预算与任务状态

**Files:** 新建 `service/futures/{documents.go,drafts.go,budget.go,scheduler.go,runs.go}` 及对应测试；`api/v1/futures/{documents.go,drafts.go,runs.go}`；Python `extract.py` 及测试。

**Interfaces:** DraftInput→Document/Draft；parsed_revision+选中候选→Run；Task来自冻结版本和剩余预算；队列输出固定generation/lease。M4实现模型端口之前注入离线test double，不能把它作为live fallback。

- [ ] 准备20MiB超限、扫描PDF、DOCX解压超限和合法TXT四类固定测试；先测试实际提取进程的限时/错误，再实现提取。
- [ ] 实现候选分页、revision失效和精确确认校验。改变输入后旧parse成功回包不得覆盖新revision。
- [ ] 预算测试：两个请求争最后额度，仅一个预留成功；费用未知仍占reserved；取消不撤销已用费用。
- [ ] 创建run事务同时写幂等键、run、outbox；20个并发相同键只产生1run。不同hash返回409，不重复增加次数。
- [ ] 实现lease30s/心跳10s、排队60s/执行180s截止和generation写保护；使用可注入Clock测试，不依赖sleep等待几分钟。
- [ ] 测试canceling先于迟到success写入时最终0个已发布report；模拟重启不重新付费。跑Go与Python定向测试并记录结果。

最小状态断言：

```text
queued -> running -> canceling -> [旧generation返回report] -> canceled
assert reports_published == 0
assert reserved_execution_slots == 1 before ACK/lease expiry
assert reserved_execution_slots == 0 after ACK/lease expiry
```

## M4：Python executor、Go grant/model gateway、报告核验

**Files:** 新建 Python `api.py/executor.py/gateways.py`、`tests/test_executor.py`；Go `service/futures/{gateway.go,model_gateway.go,reports.go,verifier.go,adapters/model_config.go}`、`api/v1/futures/internal.go` 及测试。

**Interfaces:** Task→ResearchExecutor.execute→候选Report；工具/模型只能经过Go授权；最终发布在Go。同一个scope、manifest和generation贯穿所有调用。私钥/供应商密钥不进入task。

- [ ] 使用 `uv add` 在独立futures-service添加已核对版本的FastAPI/Pydantic/httpx，提交独立锁；不要改旧worker环境。
- [ ] 从 ports.py 实现HTTP gateway，显式传deadline/cancel，不用无限重试。先测试错误owner/run/tool/manifest/args_hash/expiry/nonce拒绝。
- [ ] 实现工具grant签发和消费；三个工具参数严格校验。provider读记录，不允许用户指定URL或系统命令。
- [ ] 实现共享模型配置只读adapter及期货独立预算模型入口；按attempt_id核销。
- [ ] 先写support/challenge相同manifest、不可互读未完成结果测试，再实现阶段调度；任意角色失败不得报告成功。
- [ ] verifier对捏造数字、错误引用、future record、另一owner数据分别拒绝；证据不足但无非法事实输出允许succeeded。一次修复后仍错则failed。
- [ ] 跑完整Go期货测试和Python executor测试，提交脱敏报告样例与核验记录。真实模型30次属于M7，不能以test double替代。

## M5：假设、检查、复盘、通知、删除与导出

**Files:** 新建 `service/futures/{hypotheses.go,checks.go,outbox.go,cleanup.go,exports.go}` 和对应测试；`api/v1/futures/{hypotheses.go,notifications.go,privacy.go}`。

**Interfaces:** 已发布Report→Hypothesis版本；新Record→Check→ChangeEvent→Outbox→Notification；删除输出Deletion/tombstone，导出输出HTML附件。全部依赖M1仓储和M2固定版本记录。

- [ ] 先测试全部允许/禁止生命周期转换，expired/closed不可恢复，修改命题必须新增语义版本。
- [ ] 实现四类条件；consecutive_periods N=2必须3个连续观测。缺期或陈旧时unknown，禁止自动改变user_view。
- [ ] 新check与outbox同事务，重复投递只一通知；初次满足只记录初值；修订翻转产生修订通知。
- [ ] 实现版本化用户决定/acknowledge，过时reviewed_check_version不能清除新待复核。
- [ ] 实现到期及四维复盘，不用价格方向自动判研究成功。
- [ ] 删除先锁影响集，impact_version变更409；成功立即隐藏，清理24h；off不停止清理；备份恢复先tombstone。
- [ ] 导出受限来源时连同可反推受限数值的计算一并隐藏；escape所有用户/模型文本；断言HTML无script、无外部图片/JS。
- [ ] 跑对应Go测试并执行PRD F27–40/F46离线场景。记录清理范围，不宣称真实备份演练已经执行。

## M6：Web 页面和宿主增量接线

**Files:** 扩展 `app/web/src/modules/futures/` 的routes/api/layout/pages/components/stores；新增 `app/web/e2e/futures.spec.js`；仅允许宿主修改 `router/finance.js`、`components/workspace/SideNav.vue`、`app/server/main.go`。

**Interfaces:** Web只使用OpenAPI；能力关闭时不新建业务请求；元数据 module=futures。不依赖股票conversation store或事件store。

- [ ] 在独立页面实现数据覆盖、五页签、研究输入确认、任务进度、报告/证据、条件跟踪/复盘、通知/管理、删除影响确认。
- [ ] 以fixture HTTP响应写Playwright表单/状态测试，明确这些测试是UI契约测试；完成后再接真实API。
- [ ] 在routes.js新增PRD全部子路由，不把多个业务页伪装为相同占位页。原骨架routes测试期待数组必须随路由完整实现更新，并增加页面路径断言。
- [ ] 加期货专属store，账号切换清空cache并abort请求；响应代次比对。刷新恢复run、不自动启动研究；离开Tab只停轮询。
- [ ] 依据SPEC §10接主路由及导航，业务排序第四且不改变intel新窗口逻辑；旧SideNav不挂到futures布局。
- [ ] Go注册入口并局部bootstrap；迁移/worker不可用只改变期货状态，不log.Fatal、不阻断旧startup。
- [ ] 执行以下命令并检查390px/1440px、键盘、旧草稿、开关两态网络请求。

```bash
cd app/web
node --test src/modules/futures/tests/*.test.mjs
npm run build
npx playwright test e2e/futures.spec.js
```

## M7：真实准入、回归、负载与灰度

**Files:** 新增 `artifacts/futures/<run-id>/manifest.json`、来源探针/研究评测/性能报告、`docs/futures-operations.md`。

**Interfaces:** 输入已实现服务、已授权来源、真实模型配置；输出可审阅证据和runtime gating配置。密钥通过环境/secret引用，不写artifact。

- [ ] 从PRD逐条生成F01–F47执行记录：环境/版本/输入/预期/实际/证据路径；未执行写未执行，不能填pass。
- [ ] 来源连续5交易日稳定及历史窗口验证；真实模型30次；50质量案例，重大数值/引用/未来数据错误0、主张正确率≥90%。
- [ ] 20会话合计10RPS15分钟；执行并发2；2000假设更新5分钟完成；与旧功能同负载基线比较并保存原始序列。
- [ ] 运行旧三个模块验收脚本前先读环境要求；开关两态回归。若旧基线原有失败，记录基线与新差异，不修改旧断言掩盖。
- [ ] 演练read_only/off：60秒停止新调用，5分钟旧模块恢复检查；off仍删除；应用回退不down futures表。
- [ ] 按PRD G0–G6获取实际发布记录再开放5人2交易日、20人5交易日灰度；缺任何硬门槛保持关闭。用户授权开发不等于数据采购/第三方服务订阅授权。

## M8：最终可复现交付

**Files:** 新增 `app/scripts/verify-futures.sh`、`docs/futures-operations.md`；更新本计划进度和SPEC实现状态；提供精确变更文件列表。

**Interfaces:** 验证脚本必须区分离线、真实来源、真实模型、UI、性能和回滚，任一未执行不得输出“上线验收全部通过”。

- [ ] 从干净安装环境按维护文档跑一次，核对Go/Node/Python版本、迁移、独立worker进程和关闭路径。
- [ ] 检查所有有状态API都做owner/mode过滤、所有新增依赖被锁定、无fixture生产导入、无未完成业务成功返回。
- [ ] 将新增文件、宿主改动、已跑/未跑测试、门槛状态、回滚步骤写入交付报告。
- [ ] 只在目标P0全部通过且上线证据齐全时称“可上线”；否则准确报告实现完成程度及阻塞项。

## 2026-10-05 执行记录（当前分支）

- [x] M0：记录工作区既有未提交改动；骨架验证通过，未覆盖 P0。
- [x] M1（代码）：模型、独立迁移、owner/mode 仓储、严格 JSON、运行/预算原语已实现；真实 PostgreSQL 测试因沙箱禁止绑定 `127.0.0.1` 未执行。
- [x] M2（代码）：Decimal 公式、观测快照/修订/可用时点、manifest-only HTTP provider、分页水位与 live fixture 拒绝已实现；真实 CU 来源授权/adapter 探针未执行。
- [x] M3（代码）：PDF/DOCX/TXT 安全解析、草稿 revision、预算预留/未知 usage 保留、run 幂等/generation/取消竞态、并发2调度和 lease 已实现；真实文件/数据库验收未执行。
- [x] M4（代码）：独立 FastAPI/Pydantic worker、短 grant、受限工具、模型预算、支持/反证隔离、一次 repair、候选核验发布链路已实现；真实模型探针未执行。
- [x] M5（代码）：生命周期、连续期缺期 unknown、用户决策、复盘、outbox、通知、watchlist、删除/tombstone/24h清理和 HTML 脱敏已实现；数据库/备份演练未执行。
- [~] M6：六个用户页面、管理员来源页、期货 store、42 操作路由矩阵、宿主导航/路由/启动接线已完成；Node 10 项和 Vite build 通过，Playwright 因沙箱绑定限制未执行。
- [ ] M7：真实来源、真实模型、质量集、性能、旧模块回归、灰度与回滚全部 blocked；不得开放 live。
- [x] M8（交付物）：`verify-futures.sh`、维护手册、F01–F47、来源/模型/性能/回滚 gate 记录和 artifact manifest 已生成；干净安装复核仍未执行，交付状态为“代码完成/离线验证完成，live 门槛未通过”。
