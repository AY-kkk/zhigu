# 线上部署状态

- 目标：完整 Vue + Go + PostgreSQL 同源部署，不是仅静态前端。
- Vercel CLI：当前环境未安装，且没有 Vercel 登录/Token。
- fallback deploy：已调用 `vercel-deploy/scripts/deploy.sh app/web`；服务端返回“recommended way to deploy is now via the Vercel CLI”，未返回 previewUrl，因此没有可交付线上 URL。
- 结论：public URL / production deploy 保持 blocked。不得把本地 URL、fixture 页面或静态 preview 当作 AC12 线上交付通过。
- 恢复所需：部署平台与账号/Token、PostgreSQL 托管、`ZHIGU_INTEL_COOKIE_SECRET`、公开 origin、provider/模型授权，以及发布后的回退与备份恢复演练。
- 已新增 `app/server/Dockerfile`、`app/web/Dockerfile`、`app/web/nginx.conf` 与 `app/deploy/compose.prod.yaml`，并验证 Go 主服务/Intel chain 二进制可构建、Vite 生产构建可生成。
- 当前机器没有 Docker/云平台凭据，因此镜像构建、Compose 启动、外部 URL、备份恢复和回退演练仍为 blocked；这些产物是部署基线，不是上线通过证明。
