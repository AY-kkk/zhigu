# IMPLEMENTATION_STATUS

日期：2026-09-21。阶段：**B 编码已交 `ready_for_review` / 部分 `blocked`，不是 accepted，不是公开运营。**

开发者不得把关口标为 `accepted`。付费模型 live 在 D-05 费用上限与 Key 写入前保持 blocked。

独立宿主仍是唯一入口：PostgreSQL 16 + `app/server` + `app/research-service` + `app/web`。GoSaaS 不是启动路径。

数据主路径：Go 直连东方财富 HSF10/HKF10 + 巨潮目录与公告 HTTP（`ZHIGU_DATA_MODE=live`）。研究 Python 只打 Go 内部 API。Tushare 不是开工条件。覆盖目录为全 A 股（含北交所）+ 港股；质量样本仍为茅台 / 宁德 / 美的。

## 阶段 B 关口（开发者自评）

| 关口 | 状态 | 说明 |
|---|---|---|
| G0 | ready_for_review | A 回归已重跑；基线 SHA `6e1c310e7bbef987d88aa7bc7d2ddcad06f82a5e` |
| G1 数据 | ready_for_review | live 覆盖全 A+港股目录与年度三表；质量样本三股 + `00700.HK` 探针通过；source-contract 在 `artifacts/stage-b/` |
| G1 模型/费用 | blocked | D-02 Key 未进环境；D-05 费用上限未写 |
| G2 | ready_for_review（live 未验证） | 双 adapter 受控测试通过；真实 invoke/admin_test 无 Key |
| G3 | ready_for_review | record_id、percent、provider_records、三股 live 读取 |
| G4 | ready_for_review | 未知项正则、§2.6 裁判、合成三条提示词；90% 人工标注未做 |
| G5 | blocked（live） / ready_for_review（功能链路） | verify 脚本与 PATCH 确认已接；20 次 live 与人工签收未做 |

## 命令证据（2026-09-21）

```
cd app/server && GOTOOLCHAIN=local go test ./... -count=1 -timeout 180s
# ok zhigu/server/service/finance 37.130s

cd app/server && GOTOOLCHAIN=local go test -race ./service/finance -count=1 -timeout 180s
# ok 84.226s

ZHIGU_STAGE_B_LIVE_DATA=1 go test ./service/finance -run 'TestLiveDataSampleGated|TestLiveThreeStatementsAndHK' -count=1 -timeout 120s -v
# 600519.SH / 300750.SZ / 000333.SZ 两年三表 PASS；600519.SH 与 00700.HK 三表6科目+公告 PASS

cd app/research-service && PYTHONPATH=. .venv/bin/python -m pytest tests -q -p no:cacheprovider
# 34 passed

cd app/web && npm run build
# vite build exit 0

cd app/web && npx playwright test e2e/editorial.spec.js e2e/stage-b.spec.js e2e/stage-b-live.spec.js
# editorial+stage-b 17 passed, 3 skipped（缺 ZHIGU_E2E_API / D-02 的真登录与 live 全链路）
```

## 阶段 A 记录（2026-09-18，仍有效）

日期：2026-09-18。阶段：**A（离线工程闭环，第三轮复审问题已修，待再审）**。不是阶段 B/C，不是公开运营。本轮单测与界面核对不能代替阶段 A 验收。

GoSaaS：本地已授权的私有快照仅作后续迁移底座，不随公开仓库分发。**阶段 A 产品确认使用独立 zhigu 宿主**；`app/gosaas` 不是本机启动路径。

## 命令证据（2026-09-18 第三轮修复后重跑）

```
cd app/server && GOTOOLCHAIN=local go test ./service/finance -run 'TestR3' -count=1 -timeout 180s -v
# 通过：8 项 TestR3*（含 malformed 两个子用例）

cd app/server && GOTOOLCHAIN=local go test ./... -count=1 -timeout 180s
# 通过：ok zhigu/server/service/finance 39.907s

cd app/server && GOTOOLCHAIN=local go test -race ./service/finance -count=1 -timeout 180s
# 通过：ok 87.711s

cd app/research-service && PYTHONPATH=. .venv/bin/python -m pytest tests -q -p no:cacheprovider
# 27 passed（含 test_restart_preserves_unacknowledged_completed_result）

cd app/web && npm run build
# vite build exit 0（7.07s）
```

第三轮探针已吸收为 `round3_regression_test.go` 与 `tests/test_round3_probes.py`。Playwright 增加 `edited claim disables confirm until re-parse`（仍需 `ZHIGU_E2E_API`）。

本机浏览器核对（2026-09-18）：解析后改观点，确认按钮消失；报告展示「原始观点 / 研究范围」；390px 证据抽屉 `innerWidth=390, drawerW=390, pre.scrollWidth=pre.clientWidth`；后台保存 `http://127.0.0.1:1/v1` 显示「禁止环回或内网地址」；运行列表为 `run_id · user_* · done / completed · 555ms · 用量 0`。

## 第三轮复审修复

| 项 | 处理 |
|---|---|
| 非法/无身份结果仍 completed | 执行器结果无条件校验 schema 1.0、run/task、status/claim_type；失败走 failed/incomplete |
| 删除运行中任务立刻释放槽位 | 删除 active 走 `canceling` + `stage=deleting`，保留租约；ACK 或租约过期后才 `canceled`/`deleted_at` |
| HTTP 500 当取消 ACK | Cancel/Get 检查非 2xx；非终态不算确认 |
| H1 与全年同比 | 四位年份只匹配完整日历年；`comparableMetrics` 拒绝粒度/跨度不一致 |
| 模型缓存跨用户 | PK `(run_id,task_id,request_id)` + owner 校验；`002_model_cache_scope.sql` 升已有库 |
| 超时后仍发 grant | `runAllowsToolsAt` 含 deadline；token TTL 取剩余截止时间 |
| Python 重启擦未读终态 | `acked` 列；Go Get 终态后 POST `/ack`；重启只清已 ACK 终态 |
| 改观点仍提交旧 draft | 前端清空 draft 并带 `claim_text`；后端 `DRAFT_TEXT_MISMATCH` |
| 390px 证据抽屉 | 宽度 `min(innerWidth,640)`，正文换行 |
| 后台非法配置无提示 | settings 保存/测试/启用 try/catch 展示错误 |
| 报告缺原始观点 | GetResearch 返回 claim/instrument/horizon，Report 渲染 |
| 后台运行列表过简 | 匿名 user、stage/status、耗时、用量、错误码 |

## 第二轮复审修复（S1–S14，仍保留）

| 项 | 处理 |
|---|---|
| S1 结果隔离 | 调度器 run/task 为预期身份；`ValidateRole`/`bindResultIdentity`/`WriteTaskResult` 拒绝跨 run/task；写回失败停止消费 |
| S2 租约失效发布 | 回收租约升 version 并撤权；`Publish` 要求 verifying + 有效租约 + 截止 + 代次 |
| S3 取消过早释放 | `finalizeCancel` 等 ACK 或租约过期；Python queued→running 行数/状态检查；终态拒绝新工具 |
| S4 计算授权 | `consumeGrant` 统一检查过期/工具名/hash/一次性；计算走 task-evidence |
| S5 口径 | `comparableMetrics` 拒绝币种/value_type 混算；quality 集覆盖 CNY vs USD |
| S6 模型代理 | `OwnerID` 取自 run；task 归属校验；DBBudget 探针通过 |
| S7 重复登记 | `ON CONFLICT DO NOTHING` + 幂等 `TaskEvidence` |
| S8 坏报告 | 结构校验失败走 `failOrIncomplete`，不把坏对象降级发布；`Publish` 再验结构与引用；契约含 `simulated` |
| S9 Python 副本 | `purge_private_copies` 清空 payload/result；删除/留存调用 `Purge`；启动恢复先失败再清终态正文 |
| S10 中文哈希 | Go `SetEscapeHTML(false)`，Python `ensure_ascii=False` |
| S11 重启 | FastAPI lifespan `recover_on_start` |
| S12 超时 | freeze/排队/token 消费冻结快照；policy 拒绝负数与上调 |
| S13 缺网关 | 非 unit-test 缺 `ZHIGU_GO_INTERNAL_URL` 记 failed |
| S14 浏览器 | E2E 真实步骤在 `ZHIGU_E2E_API` 下才跑；缺浏览器只记阻塞 |

## 阶段 A 审查修复（第一轮，仍保留）

已按阶段 A 交付审查修复取消收敛、禁止状态复活、Python 锁、引用/证据绑定、工具身份与撤销、角色预算快照、工具失败状态、跨进程启动配置、租约心跳、配置测试文案与留存清理。

- 内部 HTTP：LLM 代理、grant、data-query、evidence、calculate 不再返回 503
- Worker 走 Eino `Orchestrator.Run`；排队超时与租约按冻结快照
- 解析 5 次/分钟、每日 10 个研究
- Python 三个工具经 Go 登记链；运行时 `import deerflow.client.DeerFlowClient`
- 后台：测试/启用、数据源、policy PATCH、脱敏运行列表
- 面客：主张列表、证据抽屉、重新研究、历史游标
- `uv.lock`、质量集 30 例目录测试、compose 含 web

## 验收案例映射（A01–A40）

| ID | 测试 | 状态 |
|---|---|---|
| A01 | TestTwoIndependentRoles / fixture 双路 | 程序路径有；真实模型未跑 |
| A02 | TestBothInsufficientCompleted | 单测通过 |
| A03 | synthesize 不把缺反证当已证明 | 代码路径有 |
| A04 | TestOneSideFailureIncomplete | 单测通过 |
| A05 | TestConflictingAssumptionsKept | 单测通过 |
| A06 | TestFutureEvidenceRejected | 单测通过 |
| A07 | 无 PIT 拒绝历史精确值 | 闸门有未来/时间校验 |
| A08 | 单位闸门 | quality 集覆盖 CNY/USD |
| A09 | Test 期间 | quality 集 |
| A10 | TestEstimateNotActualGate | 单测通过 |
| A11 | TestZeroDenominator | Go/Python 单测通过 |
| A12 | TestUnregisteredCitation | 单测通过 |
| A13 | TestOwnerIsolation / TestHistoryOwnerOnly | 单测通过 |
| A14 | ValidateRole 仅授权 evidence | 代码有；调度器身份绑定 |
| A15 | test_injection / HasPromptInjection | 单测通过 |
| A16 | assert_exact_tool_allowlist | Python 单测 |
| A17 | test_role_context_isolation | Python 单测 |
| A18 | TestConcurrentBudgetReservation | 单测通过 |
| A19 | ModelProxy 关闭重定向/不隐式重试 | 代码有；无真实 429 供应商 |
| A20 | TestUnknownUsageRetained | 单测通过 |
| A21 | TestCreateRunIdempotency 20 次 | 单测通过 |
| A22 | 同 key 不同 body 409 | 单测通过 |
| A23 | TestConfirmDraftOnce | 单测通过 |
| A24 | test_task_replay_same_payload | Python 单测 |
| A25 | TestCancelPublishRace | 单测通过 |
| A26 | TestDeleteLateWrite | 单测通过 |
| A27 | 详情 2s 轮询、刷新读后端 | Vue 已实现 |
| A28 | 配置冻结在 run.config_versions | 创建时写入 |
| A29 | TestSecretNeverReturned | 单测通过 |
| A30 | TestSSRFBlocked | 单测通过 |
| A31 | TestQueueTimeoutFailsQueuedRun | 单测通过 |
| A32 | TestRepairAtMostOnce / RepairN | 单测通过 |
| A33 | Ask() 不调研究 API | 代码路径有 |
| A34 | AdminRequired 403 | 中间件有 |
| A35 | FixtureBanner 不可关闭 | 客户/后台布局均有 |
| A36 | mark_running_failed_on_restart | lifespan + 单测 |
| A37 | TestActivateRequiresMatchingDigest | 单测通过 |
| A38 | TestAccountConcurrencyLimit | 单测通过 |
| A39 | Playwright 390px + 未登录跳转 | 脚本有；需浏览器 |
| A40 | README 明确不可公开运营 | 已写 |

## 未完成（不是本轮工程缺口，是 B/C 或环境前置）

- 无真实模型/数据凭据，不能宣称 live 双路研究。
- 完整 GoSaaS 进程未作为本机启动路径。
- Playwright Chromium 需本机浏览器；未把下载失败记成业务通过。
- 语义质量人工标注未做。
- 阶段 A 仍待复审，不以本轮单测与界面核对代替验收。
