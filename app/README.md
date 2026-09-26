# 知股一期（阶段 A：离线工程闭环）

## 投资事件情报与证据时间线（I-1.0）

面向个人投资者与投研人员的同级模块“事件情报”，入口为独立窗口 `/app/intel`，演示入口为 `/app/intel/demo`。本期支持 A 股收购、业绩预告、监管立案三类事件，目标是回答：谁最先说、事实怎么变、哪些说法冲突、目前是什么状态、与自选标的有什么关系。

### 本期边界

- P0：关注标的、事件归并、来源修订、证据分级、三轴状态、冲突/更正、历史版本、出处定位、站内通知、管理员纠错接口、隔离回放。
- 不做：买卖建议、目标价、收益预测、交易、产业链推断、全网实时监控、聊天/外部推送。
- AI 只做实体/命题/引用/候选关系抽取；归并、分级、状态、支持度和结论模板由确定性规则计算。fixture 是人工金标准，不冒充真实数据或真实模型结果。
- 演示回放只写入 `demo` namespace；live 与 demo 的 Cookie、请求头、关注和通知完全隔离。

### 启用与启动

默认关闭。生产/演示启用必须同时设置后端与前端开关，并配置独立 Cookie 密钥：

```sh
export ZHIGU_INTEL_ENABLED=true
export VITE_INTEL_ENABLED=true
export ZHIGU_INTEL_MODE=demo
export ZHIGU_INTEL_PROVIDERS=fixture
export ZHIGU_INTEL_COOKIE_SECRET='至少32字节随机值'
export ZHIGU_INTEL_PUBLIC_ORIGIN='http://localhost:5173'
export ZHIGU_INTEL_DAILY_TOKEN_LIMIT=200000
```

真实模式必须显式配置并验证 `IFIND_MCP_URL,IFIND_AUTHORIZATION,FUYAO_API_KEY` 及 `ZHIGU_INTEL_PROVIDERS`；模型还需 `ZHIGU_INTEL_MODEL_CONFIG_ID,ZHIGU_INTEL_MODEL_CONFIG_DIGEST,ZHIGU_INTEL_MODEL_PROTOCOL,ZHIGU_INTEL_MODEL` 冻结配置。缺凭据时 live provider/模型保持 disabled，不得用 fixture 填充真实空间。公开环境必须改掉仓库示例密钥，并通过 `ZHIGU_INTEL_PUBLIC_ORIGIN` 做同源/Origin 校验。

启动顺序：迁移（包含 `006_intel.sql`）→ Go API/Worker → Vue。演示页先调用 `GET /session` 取得 bootstrap Cookie，再调用 `POST /demo-sessions` 创建隔离空间；刷新通过 session 恢复 step，不自动 reset。

### 验收与证据

```sh
bash app/scripts/verify-intel.sh offline
bash app/scripts/verify-intel.sh integration
bash app/scripts/verify-intel.sh live-data   # 缺授权返回 2，不生成“通过”记录
bash app/scripts/verify-intel.sh live-model  # 缺授权返回 2，不生成“通过”记录
bash app/scripts/verify-intel.sh regression
```

offline 至少执行规则/API/迁移/前端 build/Intel Chromium fixture 流程；integration 在上述基础上加 race。`artifacts/intel/<UTC-run-id>/manifest.json` 记录 commit、工作区差异、PRD/SPEC hash、规则/Schema/prompt 版本和 I01–I38 的证据状态。live 数据、live 模型、5 名用户理解和 10 RPS 负载必须单列真实证据，不能用 fixture 或截图替代。

### 已知限制（必须与上线材料一起披露）

- 当前可重复验证的是确定性 events-v1 回放和本地 PostgreSQL/API/UI 链路；未声明真实供应商授权、真实模型抽取或公开 URL 已上线。
- 管理员抓取/人工导入接口采用 fail-closed：未配置 provider 时返回 `PROVIDER_DISABLED` 或 `DATA_UNAVAILABLE`，不生成伪造样例。
- 外链失效只降级展示允许的历史片段，不把不可访问原文伪装成完整核验；来源权利、request_id 和用户验证记录须在真实上线前补齐。

当前达到 **阶段 A**。未配置真实模型或金融数据凭据，不得声称阶段 B/C 或可公开运营。

## 架构

浏览器 → Vue → Go（Gin/GORM/Eino）→ PostgreSQL
Go → 私网 Python（DeerFlow Harness 适配层 + 三个金融工具）
工具路径：grant → data-query → 证据登记 → 核销。fixture 也走同一出口。

本地私有业务底座快照在 `references/gosaas/` / `app/gosaas/`，这两个目录不随公开仓库分发。**阶段 A 产品确认：使用独立 zhigu 宿主（`app/server`）完成离线闭环**；GoSaaS 进程（MySQL/Redis/Casbin）不是本阶段启动路径。阶段 B 再把面客路由迁到已授权的业务插件。同步命令：`app/scripts/vendor-gosaas.sh`（只读本地快照，不 push）。禁止静默更换业务底座。

## 测试账号（仅本地）

| 用户 | 密码 | 角色 |
|---|---|---|
| invitee | Passw0rd! | user |
| invitee-b | Passw0rd! | user |
| admin | Passw0rd! | admin |

## 启动（fixture 全链路）

必须同时启动 PostgreSQL、Go、Python。未设置 `ZHIGU_RESEARCH_URL` 时 Go **拒绝启动**，不会静默改用单进程 fake。

```sh
export ZHIGU_POSTGRES_DSN='host=127.0.0.1 user=zhigu password=zhigu dbname=zhigu port=5432 sslmode=disable TimeZone=UTC'
export ZHIGU_INTERNAL_TOKEN=zhigu-internal-dev
export ZHIGU_RESEARCH_URL=http://127.0.0.1:8091
export ZHIGU_GO_INTERNAL_URL=http://127.0.0.1:8080
export ZHIGU_RESEARCH_EXECUTOR=fixture
# 行情接真实数据源；缺省 fixture 只供离线测试，展示真实 K 线必须设 live（S-02）
export ZHIGU_MARKET_MODE=live

# 1. PostgreSQL 16，执行 app/server/migrations/finance/001_init.sql
# 2. Python 研究服务
cd app/research-service && uv sync --frozen --no-dev && uv run --no-sync uvicorn app.main:app --port 8091
# 3. Go（另开终端，继承上面的环境变量）
cd app/server && GOTOOLCHAIN=local go run .
# 4. Vue
cd app/web && npm ci && npm run dev
```

打开 `/login`。fixture 证券仅为 `DEMO:COMPANY`。

单进程 fake 仅允许 `ZHIGU_RESEARCH_MODE=unit-test`（单元测试）。集成/本机启动缺 URL 会失败。停掉 Python 后新研究必须失败，不能再产出完成报告。

Docker：`app/deploy/compose.yaml` 已设置 `ZHIGU_RESEARCH_URL` 与相同 token。**没有 Docker 时不要宣称 compose 已在本机跑通。**

## 后台模型设置

1. 用 admin 登录 `/admin/ai-settings`
2. 保存 Base URL / Model / Key（GET 只回 has_key）
3. 点「测试」：阶段 A 只做格式校验，**不是**上游连接通过；真实模型测试属于阶段 B
4. 可保存 fixture 数据源；无凭据时保持 fixture，不要启用 live

## 数据源

阶段 A 只有 fixture connector。live 需要经授权的供应商凭据、覆盖范围与再展示权限。本 SPEC 不杜撰厂商 endpoint。

## 备份与恢复

```sh
pg_dump "$ZHIGU_POSTGRES_DSN" > backup.sql
psql "$ZHIGU_POSTGRES_DSN" < backup.sql
```

个人报告默认留存 30 天；删除后 24 小时内清理主存私有内容（草稿正文、报告正文、任务结果、数据记录 payload、追问正文与回答、证据原文与指标）。保留 ID/哈希/状态等不可还原审计字段。Python 任务库是磁盘 SQLite（`ZHIGU_RESEARCH_SQLITE`，compose 挂 `researchdata` 卷会跨进程保留）；终态 payload/result 由启动恢复与 `purge_private_copies` 清理，不是“进程退出即丢弃”。这是工程初值，不是法律结论。

## 测试

```sh
cd app/server && GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go test -race ./service/finance/...
cd app/research-service && .venv/bin/python -m pytest tests -q  # 完整 Harness 依赖准备见 spec/development.md
cd app/web && npm ci && VITE_INTEL_ENABLED=true npm run build && npm run test:e2e
```

## 已知限制

- 阶段 A：fixture 闭环；无真实外部数据/模型凭据。
- 本机必须启动 Python；Go 不再在缺 URL 时假装研究成功。
- `ZHIGU_RESEARCH_EXECUTOR=fixture` 明确使用 fixture 执行器；缺省尝试 DeerFlow Harness，失败则任务为 failed，不伪装成证据不足。
- Playwright 需要本机 Chromium；下载失败时不要把 e2e 记成业务绿灯。完整登录→研究闭环需要本机 API。
- 不承诺公开运营、投资效果或供应商撤回/免费。
