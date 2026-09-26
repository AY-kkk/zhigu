# Intel 生产部署基线

`compose.prod.yaml` 提供 PostgreSQL、研究服务、Go API/Worker 和 Nginx SPA 的同源拓扑。使用前必须设置 `POSTGRES_PASSWORD,ZHIGU_INTERNAL_TOKEN,ZHIGU_JWT_SECRET,ZHIGU_INTEL_COOKIE_SECRET,ZHIGU_INTEL_PUBLIC_ORIGIN`，真实 provider/模型还需独立授权配置。

发布顺序：

1. 备份 PostgreSQL 与研究任务库。
2. 构建并启动 `postgres/research/server`，确认 `/healthz` 与迁移成功。
3. 构建并启动 `web`，验证 `/app/intel` 深链接和 `/api/*` 不回退 HTML。
4. 仅在 live provider/模型 AC11 证据完成后切换 `ZHIGU_INTEL_MODE=live`。
5. 演练回退：先关导航与新写入、停 worker、切镜像；保留 Intel 表，不 DROP 数据。

当前环境没有 Docker/云平台凭据，因此该文件和镜像构建尚未在真实宿主验收。不得把本地 fixture 测试当作生产部署通过。
