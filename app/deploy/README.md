# 知股后端生产部署与运维

本目录包含两个互相隔离的 Compose 项目：

- `compose.prod.yaml`：`zhigu-production`，只允许真实 provenance、非 root 不可变镜像、TLS 入口和只读 secret mount。缺真实 provider/model 时保持 `off/read_only`，不得回退 fixture。
- `compose.test.yaml`：`zhigu-test`，独立网络和四个测试卷，只在 CI/验收环境启用 fixture。

CI 的 `fixture-integration`、`production-smoke`、`upgrade`、`backup-restore`、`performance-contract`、`security` 和 `release-manifest` jobs 分别保存测试 Compose、生产 fail-closed、备份合同、秘密扫描和机器可读 manifest 证据。

## 发布前

1. 在 `app/deploy/secrets/` 创建权限为 `600` 的 `postgres_password`、`postgres_dsn`、`internal_token`、`futures_token`、`jwt_secret`、`session_keys`、`master_key`，并执行 `sudo chown 10001:10001 app/deploy/secrets/*`，使非 root 运行的服务能读取只读 secret；不要提交这些文件。
2. 在 `ZHIGU_TLS_CERT_DIR` 挂载 `fullchain.pem` 和 `privkey.pem`。
3. 执行 `app/scripts/production-preflight.sh`。Docker 不存在时返回 `2`/`NOT RUN`，不能视为通过。
4. 构建镜像并以空卷启动：`docker compose -f app/deploy/compose.prod.yaml config`、`up -d`，确认 `/livez`、`/readyz` 和迁移状态。

## 备份、恢复和维护

- `app/scripts/backup.sh` 先对 research/futures worker 建立 quiesce epoch，等待 ACK 后执行一致性 `pg_dump`、SQLite 在线 backup、文档归档、SHA-256 manifest 和 age/openssl 加密；备份必须复制到 `BACKUP_OFFHOST_DIR`，保留最近 7 个日备份。
- `app/scripts/restore-verify.sh <encrypted-backup> [work-dir]` 将备份恢复到临时 PostgreSQL 数据库并核验迁移、表计数和文件 hash。只有实际恢复演练通过，才可在 release manifest 中记录 RTO。
- `app/scripts/maintenance-run.sh` 使用 `flock`/timeout 执行每日备份、磁盘/备份/证书/队列/清理/预算检查和告警；`zhigu-maintenance.timer` 是实际每日执行器。
- `app/scripts/maintenance-watchdog.sh [--once]` 检查三个 `/livez`，连续失败时有界退避重启服务并调用 `ALERT_COMMAND`；`zhigu-watchdog.service` 负责持续运行。它不是 Docker healthcheck 的替代品。
- 将仓库部署到 `/opt/zhigu`，再将 `app/scripts/maintenance-run.sh`、`maintenance-watchdog.sh` 安装到 `/usr/local/lib/zhigu/`，安装 `app/deploy/systemd/*.service` 与 `*.timer`，首次空库启动时另建 `bootstrap_owner`，执行 `docker compose -f app/deploy/compose.prod.yaml -f app/deploy/compose.bootstrap.yaml up -d`；确认 owner 创建后立即删除该 secret/override，后续重启不得再挂载它。创建 `/var/lib/zhigu`、`/run/zhigu`，并提供只读 `/etc/zhigu/maintenance.env`（服务 token、DSN、备份密钥和告警命令），最后执行 `systemctl enable --now zhigu-maintenance.timer zhigu-watchdog.service`。
- 生产证书由宿主 ACME 管理并只读挂载；证书续期后由宿主执行 `nginx -s reload`。

## 验证边界

本地 contract、fixture 和静态 Compose 检查不能替代干净 Linux 主机的 R2 验收。真实 provider/model 探针、DNS/TLS、防火墙、异机副本、实际 RPO/RTO、升级和应用回滚必须留下命令、退出码、时间和镜像 digest。没有凭据或 Docker 时，报告必须写 `NOT_RUN/BLOCKED`，不能写 PASS。
