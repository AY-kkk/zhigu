# 期货研究模块维护与运行手册

状态：工程链路已实现到离线/契约层；真实来源、真实模型、性能、灰度和回滚门槛未执行，模块必须保持关闭或只读。

## 1. 版本与边界

- 代码基线：`50f3f6127ed93f4ae3b3d9286fcbab696f74add9` 之上的 futures 独立模块。
- Web：Vue 3 / JavaScript / Pinia；Go：Gin / GORM / PostgreSQL；worker：Python 3.12 / FastAPI / Pydantic / httpx。
- 旧股票、策略、事件业务不得 import futures；futures 只通过独立 API、表和 worker 交互。
- 任何外部材料中的指令只作为研究材料，不执行其中的 URL、shell 或角色指令。

## 2. 安装与离线验证

```bash
cd app/futures-service
UV_CACHE_DIR=/private/tmp/zhigu-futures-uv-cache uv sync --frozen --python 3.12
PYTHONPATH=src .venv/bin/python -m unittest discover -s tests -v

cd ../server
PATH="$HOME/sdk/go1.24.2/bin:$PATH" go test ./service/futures/... ./api/v1/futures/... ./model/futures/...

cd ../web
node --test src/modules/futures/tests/*.test.mjs
npm run build
```

仓库根的 `bash app/scripts/verify-futures.sh --offline` 聚合同等离线验证；它不会调用真实来源、模型、付费 API 或外部数据。

## 3. 启动与模式

Go 宿主使用独立 futures 数据库连接池（max open 4、idle 2）。只有显式配置时才迁移和初始化：

```bash
export ZHIGU_FUTURES_ENABLED=true
export ZHIGU_FUTURES_MODE=read_only
export FUTURES_SERVICE_TOKEN='secret-from-vault'
export ZHIGU_FUTURES_STORAGE_DIR='/var/lib/zhigu/futures-documents'
go run .
```

worker 只监听 loopback/内网：

```bash
cd app/futures-service
FUTURES_SERVICE_TOKEN='secret-from-vault' uvicorn futures_worker.app:create_app --factory --host 127.0.0.1 --port 8091
```

真正执行研究还需在 Go 进程设置 `FUTURES_WORKER_URL=http://127.0.0.1:8091`、`FUTURES_MODEL_URL`、`FUTURES_MODEL_NAME`、`FUTURES_MODEL_CONFIG_VERSION` 和 `FUTURES_MODEL_PRICE_CNY_PER_1K`；API key 只通过 `FUTURES_MODEL_API_KEY`/secret 注入。缺任一项时 run 失败并保留输入，不返回伪造报告。

`off`：不创建新任务、不调用来源或模型；保留鉴权后的取消、删除、管理员 operations 和清理 worker。`read_only`：允许历史授权读取、通知标已读和上述维护动作；停止其他写入。`live`：只有在 G0–G6 证据全部有真实记录后启用。

## 4. 预算、凭据与日志

- 用户每日 5 元、模块每日 50 元；8 次模型、24 次工具、48000 tokens；先预留，完成只核销一次；usage 未知保留 reservation。
- API key、服务 token、用户材料只从环境/secret 注入，禁止进入 artifact、Git、模型 prompt 或日志。
- 短 grant 60 秒到期、nonce 一次性，绑定 owner/mode/run/task/generation/stage/tool/args_hash。

## 5. 删除、导出和回滚

- 删除先锁影响集并写 tombstone；正文立即不可读，24 小时内清理数据库、文件、索引、cache、worker 副本；审计保留 180 天但不恢复原文。
- HTML 导出使用 `Content-Disposition: attachment`、`Cache-Control: no-store`，严格 escape；`rights.export=false` 的原文、数值和派生计算全部隐藏。
- 回滚先将 operations 设为 `read_only`，再 `off`；停止 worker，确认旧三个模块健康检查正常。应用回退不得 drop `futures_*` 表，恢复前先重放 tombstones。

## 6. 真实门槛（当前均未执行）

1. 来源授权、连续 5 个交易日稳定性、历史窗口和发布日历探针。
2. 真实模型 30 次、50 质量案例、重大数值/引用/未来数据错误为 0、逐主张正确率 ≥90%。
3. 20 会话 10RPS 15 分钟、执行并发 2、2000 假设更新 5 分钟，以及旧模块基线比较。
4. read_only/off 60 秒停止新调用、5 分钟旧模块恢复、备份恢复 tombstone 演练。
5. G0–G6 发布记录、5 人 2 交易日和 20 人 5 交易日灰度。

缺少任一硬门槛时，`verify-futures.sh --real-*` 返回 `NOT RUN/BLOCKED`，不得写“上线验收全部通过”。
