# 首次公开发布验证

日期：2026-09-18。范围：当前阶段 A 源码、公开依赖安装路径、中文文档与仓库配置。

## 本地结果

| 检查 | 结果 |
| --- | --- |
| Eino 上游版本 | `v0.10.0-alpha.29` 与既有本地参考提交完全相同；已移除本地 replace |
| `go mod verify` | 通过 |
| `go test ./... -count=1 -timeout 300s` | 通过，finance 包约 42 秒 |
| `go test -race ./service/finance -count=1 -timeout 300s` | 通过，约 92 秒 |
| Python 全套本地测试 | 27 passed；包括本机已安装的 Harness 导入检查 |
| 公开契约验证 | 20 passed，4 subtests passed；不运行私有参考快照用例 |
| Python 无参考目录安装 | 独立临时目录中 `uv sync --frozen --no-dev` 通过，23 个包 |
| 干净公开目录验证 | Python fixture 26 passed、1 个 Harness 用例明确排除；公开契约 20 passed |
| Web 构建 | 通过；存在主包超过 500 kB 的体积提示 |
| 实际页面 | 研究入口、完成报告与管理员运行列表可打开。改版前界面截图已移除 |
| 敏感文件与历史 | 当前候选内容与原始提交的常见凭据特征扫描无命中；不等于完整安全审计 |
| Docker Compose | 已改为 fixture 最小依赖与回环端口；本机没有 Docker，未声称完整启动通过 |

## CI 范围

公开 CI 运行 Go 测试及 race 检查、Python fixture/回归用例、公开契约检查、发布文件检查、Web 构建及无后端浏览器冒烟测试。它明确不涵盖以下内容：

- 完整真实模型研究和金融数据供应商可用性。
- 私有上游的授权与源码快照测试。
- 未启动后端时的完整浏览器研究链路；相关测试会标明跳过。
- 语义质量人工标注、生产压测或独立安全审计。

远端 CI 的实际状态与日志以 [Actions](https://github.com/AY-kkk/zhigu/actions/workflows/ci.yml) 为准。

首次远端运行发现两项环境前置遗漏：Go 跨进程测试需要独立安装 Python worker；Linux 上 Vite 的默认 localhost 监听与浏览器的 IPv4 地址不一致。已补齐 Go job 的 Python 依赖，并让 Playwright 启动服务器显式绑定 `127.0.0.1:5173`，保留全部既有测试断言。

## 发布内容

包含当前 Go/Python/Vue 业务代码、回归测试、契约、规格与中文文档。保留本地原始提交历史。

不发布 `.env`、API 凭据、数据库、依赖缓存、运行日志、临时审查目录或私有 GoSaaS 源码。本地原文件保留。原创代码的开源许可证尚未指定，详情见 [许可说明](../git-hub说明文档/LICENSE.md)。

## 面客 Semi UI 改版补充

面客前端使用 `@kousum/semi-ui-vue@2.78.4` 与 `@kousum/semi-icons-vue@2.78.0`，对应上游 `rashagu/semi-design-vue` 固定提交 `15074390002324fd1fe9467d61b4a5dc93a475f5`。组件库通过 npm 依赖安装，未把上游源码检出目录复制进本仓库；许可和来源记录见 [第三方组件与来源说明](../THIRD_PARTY_NOTICES.md)。

本次面客改版的 Web CI 已通过：`npm ci`、`npm run build`、Chromium 无后端浏览器冒烟检查。该结果证明公开构建和静态面客路径可用，不等于真实模型、实时金融数据或完整后端研究链路已完成验收。
