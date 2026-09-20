# 开发与验证

## 环境与启动

主启动入口见 [README](../README.md#快速开始)。Go 1.24.2+（1.24 系列）、Python 3.12、Node 22、PostgreSQL 16。Python fixture 依赖使用 `uv sync --frozen --no-dev`；不要删除 `--frozen` 后在缺少可选上游源码的目录重新解析锁文件。

Go 通过公共模块下载固定 Eino 版本 `v0.10.0-alpha.29`，对应原参考提交 `9d983b36a5112a1c233056b1a099825298fafb8f`，不再依赖本地 `replace`。如需要更改依赖，请提交对应锁文件并运行检查。

Python 的 `harness` extra 保留固定参考目录，用于源码级适配核验。阶段 A fixture 无需该 extra；不要将缺失 Harness 时的失败包装成真实研究成功。

## 常用检查

从仓库根目录执行：

```bash
cd app/server
go mod verify
go test ./... -count=1 -timeout 300s
go test -race ./service/finance -count=1 -timeout 300s
```

Go 测试会在临时目录启动独立 embedded PostgreSQL，首次需要联网下载二进制。无需连接开发数据库。

其中跨进程测试还需要 `app/research-service/.venv/bin/python`；请先按快速开始安装 Python fixture 依赖，再运行 Go 全套测试。CI 会在 Go job 内独立安装，不能依赖另一个 job 的文件系统。

```bash
cd app/research-service
uv sync --frozen --no-dev
uv pip install --python .venv/bin/python pytest rfc3339-validator
ZHIGU_RESEARCH_SQLITE=/tmp/zhigu-test-jobs.sqlite \
  .venv/bin/python -m pytest tests -q -k 'not test_actual_harness_import'
```

这条命令验证 fixture 与回归用例，明确不包括 `test_actual_harness_import`。完整 Harness 检查需要下面的可选依赖。

```bash
# 从仓库根目录执行，验证公开契约；私有参考快照检查单独执行。
app/research-service/.venv/bin/python -m pytest handoff/tests -q \
  -k 'not test_repository_pins_and_reuse_paths'

cd app/web
npm ci
npm run build
npx playwright install chromium
npm run test:e2e
```

未设置 `ZHIGU_E2E_API` 时，浏览器测试只检查登录页和未登录跳转，其余 API 用例会明确跳过。启动 Go、Python、PostgreSQL 后，执行 `ZHIGU_E2E_API=1 npm run test:e2e` 才验证 API 交互。测试使用本地演示账号并会创建 fixture 记录。

本地已安装 Google Chrome 时，可用 `ZHIGU_BROWSER_CHANNEL=chrome npm run test:e2e`；CI 使用 Playwright 下载的 Chromium。测试服务器显式绑定 `127.0.0.1`，避免 Linux 将 localhost 解析到 IPv6 时与浏览器目标地址不一致。

## 可选：完整 Harness 导入检查

```bash
python3 scripts/bootstrap-harness.py
cd app/research-service
uv sync --frozen --extra harness
uv pip install --python .venv/bin/python pytest
ZHIGU_RESEARCH_SQLITE=/tmp/zhigu-harness-test.sqlite \
  .venv/bin/python -m pytest tests -q
```

`scripts/bootstrap-harness.py` 只在维护者本机存在本地 pin 文件时核验可选 Harness 检出；公开克隆默认没有该文件。已有检出必须与 pin 记录相符且工作树干净，否则脚本停止，避免覆盖本地工作。

完整测试通过只说明适配与 fixture 工程路径可运行，不说明真实模型输出质量已验收。`test_repository_pins_and_reuse_paths` 只在维护者本机有 pin 文件时运行，不属于公开克隆的默认 CI。

## 仓库维护

- `main` 为默认分支，功能通过 PR 合入；优先 squash merge，合并后删除临时分支。
- CI 分别检查 Go、Python/契约、Web；Go 的数据库测试失败不会被静默跳过。
- Dependabot 定期检查 Go、npm 和 GitHub Actions。Python 锁文件含可选上游目录依赖，暂由维护者准备固定 Harness 后手动更新，避免机器人在缺失目录的环境中反复解析失败。所有升级都需要重新审查证据与工具边界。
- `scripts/check-public-tree.py` 检查已跟踪文件中的私有目录、运行数据、大文件和常见凭据特征；它不是完整安全审计。
- 发版前检查 README 的能力描述、路线图、实际测试与许可证状态。开发预览不能标为生产稳定版。
