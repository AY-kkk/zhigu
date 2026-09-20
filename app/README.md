# 知股一期（阶段 A：离线工程闭环）

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
cd app/research-service && .venv/bin/python -m pytest tests -q  # 完整 Harness 依赖准备见 docs/development.md
cd app/web && npm ci && npm run build && npm run test:e2e
```

## 已知限制

- 阶段 A：fixture 闭环；无真实外部数据/模型凭据。
- 本机必须启动 Python；Go 不再在缺 URL 时假装研究成功。
- `ZHIGU_RESEARCH_EXECUTOR=fixture` 明确使用 fixture 执行器；缺省尝试 DeerFlow Harness，失败则任务为 failed，不伪装成证据不足。
- Playwright 需要本机 Chromium；下载失败时不要把 e2e 记成业务绿灯。完整登录→研究闭环需要本机 API。
- 不承诺公开运营、投资效果或供应商撤回/免费。
