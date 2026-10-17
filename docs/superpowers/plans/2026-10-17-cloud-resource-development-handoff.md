# 火山云资源与知股公网上线开发实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 `codex/futures-review-release` 工作树收敛为可验证的公网 MVP，先在已有火山云 ECS 上完成数据库、后端、研究/期货 worker、前端和运维闭环，再决定是否迁移到托管数据库。

**Architecture:** 第一阶段采用单台火山云 ECS + EIP + Docker Compose 的同机拓扑：Nginx 对外提供 Vue 和 HTTPS，Go API 只在内网访问 PostgreSQL、research worker 与 futures worker。生产 Compose 必须保持 fail-closed、非 root、不可变镜像和只读 secret mount；数据库默认使用当前 `compose.prod.yaml` 中的 PostgreSQL 16.4，不在未验证前切换到 fixture、外部模型或 live provider。

**Tech Stack:** 火山云 ECS/EIP/Ubuntu/PL0 SSD、Docker Compose、PostgreSQL 16.4、Go 1.24+、Python 3.12、Vue 3/Vite、Nginx、systemd、`pg_dump`/`pg_restore`、age 或 openssl 加密备份。

## Global Constraints

- 当前工作树不是干净发布基线：分支 `codex/futures-review-release`，HEAD `a06f0dd`，领先远端 11 个提交，且有大量未提交改动。后续 Agent 不得 `git reset`、`git checkout --`、覆盖或删除他人的未提交源码。
- 本文记录的云资源来自用户提供的火山云控制台清单；CPU、内存、磁盘容量、EIP 绑定、地域、带宽上限、安全组和实际公网地址尚未在本机验证。
- `app/deploy/compose.prod.yaml` 当前五服务资源上限合计 **6.0 CPU / 5.50 GiB**，不含 Docker、OS、日志和突发内存。未确认 ECS 规格前不得直接宣称该 Compose 可运行。
- 生产环境只接受 `ZHIGU_DEPLOYMENT_ENVIRONMENT=production`、`ZHIGU_DATA_PROVENANCE=real` 的显式配置；缺 secret、TLS、provider/model 证据时必须保持 `read_only` 或返回 `BLOCKED`，不得回退 fixture。
- 真实 provider、模型、数据授权、行情源、DNS/TLS、异机备份、恢复、升级和回滚必须留下命令、退出码、时间、镜像 digest；fixture、静态检查、截图不能替代真实链路验收。
- 所有 secret 只能存在于服务器上的 `app/deploy/secrets/` 或 `/etc/zhigu/maintenance.env`，权限 `600`/只读挂载，不得提交、打印或写入 release manifest。
- 任何会产生费用、修改云资源、购买域名、备案、升级 ECS、绑定/解绑 EIP 的操作，先暂停并请求用户确认。
- 产出状态只使用 `PASS`、`FAIL`、`BLOCKED`、`NOT_RUN`。没有执行的项目不能写成通过。

## 当前云资源清单

| 类型 | 资源 ID | 名称/规格 | 计费与状态 | 能力 | 当前边界 |
|---|---|---|---|---|---|
| ECS 云服务器 | `i-yeexgdl340bw80e34g9x` | `APP-rd65-000` / `ecs.e-c1m1.large.month` | 包年包月、手动续费、运行中；2026-02-03 10:12:43 至 2027-02-03 23:59:59 | Ubuntu、Docker、Go、Python、PostgreSQL、Nginx | 实际 vCPU/内存/磁盘容量需从控制台或 SSH 核实 |
| 弹性公网 IP | `eip-bte0zx40gh6o5h0b2v74gwq5` | `eip-bte0zx40gh6o5h0b2v74gwq5` | BGP 带宽上限计费、包年包月、手动续费、运行中；同上周期 | 公网入口、域名解析目标 | 是否绑定到 ECS、公网地址、带宽上限和出网计费需核实 |
| 镜像服务 | `image-i-yeexgdl340bw80e34g9x` | `Ubuntu-包年包月` | 运行中；包年包月、手动续费 | ECS 的 Ubuntu 系统镜像 | 不是独立服务器；若账单中单独收费需核对镜像计费 |
| 弹性块存储 | `vol-3ww7fzb21z39b9pznlwe` | `APP-rd65-000-volume-1` / 系统盘-极速型 SSD-PL0 | 运行中、挂载在 ECS | 系统、程序、容器卷和本地数据持久化 | 容量、剩余空间、快照和异地备份策略尚未核实 |

### 资源决策

- **第一阶段建议：** 先使用现有 ECS 作为部署主机，完成公网 MVP 和真实链路验证。
- **容量门槛：** 先执行 `nproc`、`free -h`、`df -h /`。若实际规格低于 `4 vCPU / 8 GiB RAM / 40 GiB 可用磁盘`，不要运行当前完整 `compose.prod.yaml`。优先升级 ECS；如必须保留低配 ECS，只能另做 demo profile，并明确不是生产验收。
- **数据库策略：** 当前生产 Compose 自带 `postgres:16.4`。第一阶段使用 ECS 本地 PostgreSQL volume；稳定后再评估火山云托管 RDS PostgreSQL 16。迁移 RDS 时保留同一 schema/migration，只替换 `ZHIGU_POSTGRES_DSN` 和网络拓扑。
- **缺失资源：** 当前清单没有域名、ICP、TLS 证书、CDN/WAF、对象存储、监控告警、托管数据库、异地备份介质或独立 CI 构建节点。它们不是本阶段自动具备的能力。

## 当前仓库与验证快照

检查时间：2026-10-17（Asia/Shanghai）。

| 项目 | 当前结果 | 后续要求 |
|---|---|---|
| `git status` | `codex/futures-review-release...origin/... [ahead 11]`；62 个已修改、53 个未跟踪路径 | 先审查 diff，分清本次发布相关文件和他人工作 |
| `app/scripts/verify-production.sh --static` | **PASS**；13 个 contract tests、futures contract、脚本语法通过 | 不能替代 full、host、backup、TLS 或 live 验收 |
| `app/scripts/production-preflight.sh` | **BLOCKED**，退出码 `2`：缺少 `app/deploy/secrets/postgres_password` | 在真实主机生成 secret 后重跑 |
| Docker | 本机 `docker` 不可用 | 需要在 ECS 或 CI 构建主机执行 Compose/镜像验收 |
| Go / Python / Node | Go 1.27.2；Python venv 3.12.13；Node 22.22.1；npm 10.9.4 | 以项目 Dockerfile 和 `go.mod`/`pyproject.toml` 的版本下限为准 |
| Migrations | finance `001..011`，futures `001..002`；Go 启动时执行并记录 checksum ledger | 不要手工跳号；升级必须保持 N/N-1 兼容 |
| `compose.prod.yaml` | 服务：`postgres`、`research`、`futures`、`server`、`web`；backend network `internal: true` | 只允许生产 secret、TLS 目录、不可变镜像和只读卷 |

## 文件与责任边界

- `app/deploy/compose.prod.yaml`：单机生产拓扑、资源上限、secret mount、健康检查、网络隔离。
- `app/deploy/compose.bootstrap.yaml`：一次性创建 owner 的 override；创建后必须移除 `bootstrap_owner` secret。
- `app/deploy/compose.test.yaml`：独立测试项目，只允许 fixture。
- `app/deploy/compose.rollback.yaml`：切换到 `ZHIGU_PREVIOUS_SERVER_IMAGE` 的回滚 override。
- `app/deploy/nginx.prod.conf`：80/443、TLS、API 反向代理、上传限制。
- `app/deploy/README.md`：生产发布、备份、恢复、验证边界。
- `app/scripts/production-preflight.sh`：静态配置和主机前置检查。
- `app/scripts/verify-production.sh`：静态/完整发布验证入口。
- `app/scripts/backup.sh`、`restore-verify.sh`：一致性备份、加密、离机复制和恢复演练。
- `app/scripts/upgrade-verify.sh`、`rollback.sh`：N/N-1 升级和应用回滚证据。
- `app/scripts/verify-frontend.sh`：lint、unit、component、contracts、performance、Playwright、浏览器矩阵和 manifest。
- `app/scripts/release-evidence.py`：生成不含 secret 的 release manifest。
- `docs/superpowers/plans/2026-10-13-backend-production.md`、`2026-10-13-frontend-productization.md`：已有产品化/生产化实现计划；部署前先按其中的 Task/Step 逐项核对实际代码状态。

---

## Task 1: 冻结基线并建立发布证据目录

**Files:**
- Create: `artifacts/release/2026-10-17/baseline.md`
- Create: `artifacts/release/2026-10-17/git-status.txt`
- Create: `artifacts/release/2026-10-17/static-verifier.log`
- Create: `artifacts/release/2026-10-17/preflight.log`

**Interfaces:**
- Consumes: 当前 Git 工作树和本地脚本。
- Produces: 后续每个 Agent 都能复用的 baseline、命令退出码和未验证边界。

- [ ] **Step 1: 记录当前 Git 基线**

```bash
mkdir -p artifacts/release/2026-10-17
git status --short --branch | tee artifacts/release/2026-10-17/git-status.txt
git rev-parse HEAD
git diff --stat
```

Expected: 记录 `a06f0dd`、分支、未提交路径和 diff 统计；不得清理工作树。

- [ ] **Step 2: 运行静态验证**

```bash
bash app/scripts/verify-production.sh --static 2>&1 | tee artifacts/release/2026-10-17/static-verifier.log
```

Expected: `PASS: production verifier --static`；这是本地 contract gate，不是生产上线证明。

- [ ] **Step 3: 记录当前 preflight 阻塞**

```bash
set +e
bash app/scripts/production-preflight.sh 2>&1 | tee artifacts/release/2026-10-17/preflight.log
code=${PIPESTATUS[0]}
set -e
echo "exit_code=$code" | tee -a artifacts/release/2026-10-17/preflight.log
test "$code" -eq 2
```

Expected: 缺 secret 时退出 `2`，日志明确 `BLOCKED`。

- [ ] **Step 4: 写 baseline 说明**

在 `baseline.md` 写明：当前分支、HEAD、dirty 状态、静态结果、preflight 结果、Docker 缺失、云资源来自用户控制台清单、未验证项目和禁止动作。

- [ ] **Step 5: 交接检查**

由后续 Agent 重读本文件、`app/deploy/README.md` 和两个 `2026-10-13` plan；不把旧截图、fixture 测试或本机静态 PASS 标为生产 PASS。

---

## Task 2: 核实火山云资源和容量，决定单机部署规格

**Files:**
- Modify: `artifacts/release/2026-10-17/baseline.md`
- Create: `artifacts/release/2026-10-17/host-capacity.txt`
- Create: `artifacts/release/2026-10-17/cloud-resource-check.md`

**Interfaces:**
- Consumes: 用户控制台资源 ID、ECS SSH 或控制台详情。
- Produces: `capacity_decision=pass|upgrade_required|split_required`，后续 Compose 部署只能在通过时继续。

- [ ] **Step 1: 获取主机规格**

在 ECS 上执行：

```bash
hostname
cat /etc/os-release
nproc
free -h
df -h /
lsblk
ip -brief address
ss -lntup
```

Expected: 保存输出；确认 `ecs.e-c1m1.large.month` 的实际 vCPU、内存、系统盘容量和 Ubuntu 版本。

- [ ] **Step 2: 对比 Compose 资源上限**

```bash
python3 - <<'PY'
import re
from pathlib import Path
s=Path('app/deploy/compose.prod.yaml').read_text()
cpu=sum(float(x) for x in re.findall(r'cpus:\s*"([0-9.]+)"',s))
mem=sum(float(x)*(1024 if u=='G' else 1) for x,u in re.findall(r'memory:\s*([0-9.]+)([GM])',s))
print(f'cpu_limit_sum={cpu:.1f}; memory_limit_sum_GiB={mem/1024:.2f}')
PY
```

Expected: 记录 `6.0 CPU / 5.50 GiB`；不能只用容器上限证明主机可承载。

- [ ] **Step 3: 做容量决策**

判定规则：

- 实际规格达到 `4 vCPU / 8 GiB RAM / 40 GiB 可用磁盘` 且预计单机低并发：`capacity_decision=pass`。
- 低于门槛但可通过升级 ECS 解决：`upgrade_required`，向用户请求费用确认后升级。
- 不能升级且必须保留现有规格：`split_required`，停止使用完整 `compose.prod.yaml`，另做明确标注为 demo 的拆分/降载方案。

- [ ] **Step 4: 核对 EIP 和安全组**

确认：

- EIP 是否绑定 `i-yeexgdl340bw80e34g9x`；
- 公网 IP 是否可解析/访问；
- 安全组仅开放 `22`（管理员 IP）、`80`、`443`；
- `5432`、`8080`、`8091`、`8092` 不对公网开放；
- BGP 带宽上限、出网流量计费和续费日期与账单一致。

- [ ] **Step 5: 记录决策**

把实际规格、磁盘剩余量、EIP 绑定、开放端口、计费/续费和 `capacity_decision` 写入 `cloud-resource-check.md`。

---

## Task 3: 完成公网入口前置条件

**Files:**
- Create: `artifacts/release/2026-10-17/domain-tls-check.md`
- Modify: `app/deploy/nginx.prod.conf`（仅在域名/证书路径确认后）

**Interfaces:**
- Consumes: 用户确认的域名、ICP 状态、EIP。
- Produces: 可供 Nginx 挂载的 `fullchain.pem`、`privkey.pem` 和 `ZHIGU_PUBLIC_ORIGIN`。

- [ ] **Step 1: 确认域名和备案**

记录：

- 域名；
- DNS A 记录是否指向 EIP；
- 大陆公网网页是否已完成 ICP 备案；
- 备案未完成时是否只允许 IP/内网 smoke，不能对外正式发布。

- [ ] **Step 2: 申请 TLS 证书**

优先使用宿主 ACME/证书续期工具，将证书放到 `${ZHIGU_TLS_CERT_DIR}/fullchain.pem` 和 `privkey.pem`；证书续期后执行：

```bash
sudo nginx -t
sudo nginx -s reload
```

Expected: 证书文件存在且私钥权限仅 root/服务用户可读。

- [ ] **Step 3: 固化公网 origin**

```bash
read -r -p 'Public HTTPS origin: ' ZHIGU_PUBLIC_ORIGIN
export ZHIGU_PUBLIC_ORIGIN
export ZHIGU_TLS_CERT_DIR='/etc/zhigu/tls'
```

Expected: `production-preflight.sh` 不再因 origin/TLS 缺失而阻塞；origin 不能使用 localhost 或私网地址，除非显式 `ZHIGU_ALLOW_LOOPBACK_SMOKE=1` 且只做 smoke。

- [ ] **Step 4: 记录验证边界**

`domain-tls-check.md` 必须区分 DNS、备案、证书签发、HTTPS 重定向、证书续期和公网可达性；没有真实 DNS/TLS 证据时保持 `BLOCKED`。

---

## Task 4: 收敛代码发布基线并构建不可变镜像

**Files:**
- Modify: 只涉及本次发布相关代码/配置；不得批量清理他人改动。
- Create: `artifacts/release/2026-10-17/release-manifest.json`
- Create: `artifacts/release/2026-10-17/frontend-results.json`

**Interfaces:**
- Consumes: Task 1 baseline、现有 backend/frontend plan。
- Produces: 可追溯 Git SHA、镜像 digest、测试结果、迁移版本和已知限制。

- [ ] **Step 1: 审查未提交差异**

```bash
git diff -- app/deploy app/scripts app/server app/research-service app/futures-service app/web contracts app/contracts
```

按以下维度分组：产品功能、生产化、测试、部署脚本、文档、他人并行改动。不得将不相关改动混入发布提交。

- [ ] **Step 2: 对照两个 2026-10-13 plan 核对实际状态**

逐项核对：

- backend production：runtime config、session/CSRF、trace/error、futures contract、release gate、worker image、Compose/backup、CI evidence；
- frontend productization：runtime envelope、system state、shared shell、futures pages、research pages、hypotheses/notifications/admin、回归、E2E/a11y/responsive/performance。

未完成项写 `BLOCKED/NOT_RUN`，不能通过改 checklist 标记掩盖。

- [ ] **Step 3: 运行本地静态与单元 gate**

```bash
bash app/scripts/verify-production.sh --static
(cd app/server && go test ./... -count=1)
(cd app/futures-service && PYTHONPATH=src .venv/bin/python -m unittest discover -s tests -p 'test_*.py')
(cd app/research-service && .venv/bin/python -m pytest tests -q -k 'not actual_harness_import')
```

Expected: 各命令退出 `0`；若失败，先修复后再构建镜像。

- [ ] **Step 4: 运行前端 gate**

```bash
bash app/scripts/verify-frontend.sh artifacts/release/2026-10-17/frontend
```

Expected: lint、unit、component、contracts、performance、Playwright 全部有真实退出码；Playwright 缺浏览器时标 `BLOCKED`，不能伪造 manifest。

- [ ] **Step 5: 构建镜像**

在有 Docker 的 ECS 或 CI：

```bash
docker compose -f app/deploy/compose.prod.yaml config
docker compose -f app/deploy/compose.prod.yaml build
export RELEASE_SHA="$(git rev-parse HEAD)"
mkdir -p artifacts/release/2026-10-17/digests /var/lib/zhigu/images
docker compose -f app/deploy/compose.prod.yaml images --format json > artifacts/release/2026-10-17/images.json
python3 - <<'PY'
import json
import subprocess
from pathlib import Path
out = Path('artifacts/release/2026-10-17/digests')
rows = json.loads(Path('artifacts/release/2026-10-17/images.json').read_text())
release_sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
for row in rows:
    service = row['Service']
    image_id = row.get('ImageID') or row.get('ID')
    if not image_id:
        raise SystemExit(f'compose image row lacks ImageID/ID: {row}')
    tag = f'zhigu/{service}:{release_sha}'
    subprocess.run(['docker', 'tag', image_id, tag], check=True)
    subprocess.run(['docker', 'save', '--output', f'/var/lib/zhigu/images/{service}.tar', tag], check=True)
    inspected = json.loads(subprocess.check_output(['docker', 'image', 'inspect', image_id], text=True))[0]
    digest = (inspected.get('RepoDigests') or [inspected['Id']])[0]
    (out / f'{service}.txt').write_text(digest + '\n')
PY
```

Expected: 使用不可变镜像 tag/digest；构建过程中不安装运行时依赖、不挂载源码目录。

- [ ] **Step 6: 生成 release manifest**

```bash
python3 app/scripts/release-evidence.py \
  --output artifacts/release/2026-10-17/release-manifest.json \
  --version 2026-10-17-mvp \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/postgres.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/research.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/futures.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/server.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/web.txt)" \
  --frontend-result artifacts/release/2026-10-17/frontend/results.json
```

Expected: manifest 无 secret、包含 `git_sha`、dirty 状态、镜像 digest、迁移版本、测试和已知限制；dirty 状态必须保留真实值。

---

## Task 5: 引导 ECS 宿主和部署目录

**Files:**
- Create: `/opt/zhigu`（服务器）
- Create: `/etc/zhigu/tls`、`/etc/zhigu/maintenance.env`、`/var/lib/zhigu`、`/run/zhigu`
- Install: `app/deploy/systemd/zhigu-maintenance.service`、`zhigu-maintenance.timer`、`zhigu-watchdog.service`

**Interfaces:**
- Consumes: Task 2 capacity pass、Task 3 domain/TLS。
- Produces: Docker/Compose 可用、部署目录、非 root 运行用户、systemd 维护任务。

- [ ] **Step 1: 更新并检查 Ubuntu**

```bash
sudo apt-get update
sudo apt-get upgrade -y
. /etc/os-release && echo "$PRETTY_NAME"
docker --version || true
docker compose version || true
```

Expected: 记录 Ubuntu 版本；Docker/Compose 不可用时安装匹配版本，不要在未确认系统版本时执行未知安装脚本。

- [ ] **Step 2: 创建目录和用户**

```bash
sudo install -d -o root -g root -m 755 /opt/zhigu
sudo install -d -o root -g root -m 750 /etc/zhigu/tls
sudo install -d -o root -g root -m 750 /var/lib/zhigu
sudo install -d -o root -g root -m 755 /run/zhigu
sudo useradd --system --home /nonexistent --shell /usr/sbin/nologin zhigu || true
```

Expected: 目录权限明确；应用容器继续按 Dockerfile 非 root 约束运行。

- [ ] **Step 3: 部署仓库和 secret**

```bash
export RELEASE_REPO_URL='https://github.com/AY-kkk/zhigu.git'
export RELEASE_SHA="$(git rev-parse HEAD)" # 当前基线为 a06f0dd；仅在 Task 4 发布提交完成后取值
sudo git clone "$RELEASE_REPO_URL" /opt/zhigu
cd /opt/zhigu
sudo git checkout "$RELEASE_SHA"
sudo install -d -m 700 app/deploy/secrets
```

在 `app/deploy/secrets/` 创建：

`postgres_password`、`postgres_dsn`、`internal_token`、`futures_token`、`jwt_secret`、`session_keys`、`master_key`。

```bash
sudo chmod 600 app/deploy/secrets/*
```

Expected: secret 不进入 Git、不进入日志；`postgres_dsn` 必须指向 Compose 内部 `postgres` 或后续 RDS 私网地址。

- [ ] **Step 4: 创建 maintenance 环境**

创建只读 `/etc/zhigu/maintenance.env`，提供 worker token、DSN、备份密钥路径和告警命令；权限 `600`，不要写入 shell history 或仓库。

- [ ] **Step 5: 启用维护服务**

```bash
sudo cp app/deploy/systemd/zhigu-maintenance.service /etc/systemd/system/
sudo cp app/deploy/systemd/zhigu-maintenance.timer /etc/systemd/system/
sudo cp app/deploy/systemd/zhigu-watchdog.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now zhigu-maintenance.timer zhigu-watchdog.service
```

Expected: timer/service 状态 `active`；watchdog 失败时能告警且不会无限重启风暴。

---

## Task 6: 启动数据库、迁移和一次性 owner

**Files:**
- `app/deploy/compose.prod.yaml`
- `app/deploy/compose.bootstrap.yaml`
- `app/server/migrations/finance/*.sql`
- `app/server/migrations/futures/*.sql`

**Interfaces:**
- Consumes: Task 5 secret、Docker host。
- Produces: PostgreSQL 16.4、`finance_schema_migrations`、`futures_schema_migrations`、唯一 owner。

- [ ] **Step 1: 先启动 PostgreSQL**

```bash
cd /opt/zhigu
docker compose -f app/deploy/compose.prod.yaml up -d postgres
docker compose -f app/deploy/compose.prod.yaml ps postgres
docker compose -f app/deploy/compose.prod.yaml exec postgres pg_isready -U zhigu -d zhigu
```

Expected: PostgreSQL healthcheck `healthy`；5432 只在主机 `127.0.0.1` 或 backend network 可达。

- [ ] **Step 2: 启动 Go 以执行迁移和 owner 检查**

首次启动使用一次性 override：

```bash
umask 077
read -r -p 'Owner username: ' OWNER_USERNAME
read -r -s -p 'Owner password: ' OWNER_PASSWORD
echo
export OWNER_USERNAME OWNER_PASSWORD
python3 - <<'PY'
import json
import os
from pathlib import Path
value = {
    'username': os.environ['OWNER_USERNAME'],
    'password': os.environ['OWNER_PASSWORD'],
}
Path('app/deploy/secrets/bootstrap_owner').write_text(json.dumps(value) + '\n')
PY
unset OWNER_PASSWORD
chmod 600 app/deploy/secrets/bootstrap_owner
docker compose -f app/deploy/compose.prod.yaml -f app/deploy/compose.bootstrap.yaml up -d server
```

Expected: server 启动日志记录 finance `001..011`、futures `001..002` 的 checksum ledger；owner 创建成功。

- [ ] **Step 3: 移除一次性 bootstrap secret**

```bash
rm app/deploy/secrets/bootstrap_owner
docker compose -f app/deploy/compose.prod.yaml up -d server
```

Expected: owner 已存在时 server 拒绝再次挂载 bootstrap secret；删除动作只删除一次性凭据，不删除数据库。

- [ ] **Step 4: 校验迁移**

```bash
docker compose -f app/deploy/compose.prod.yaml exec postgres \
  psql -U zhigu -d zhigu -c 'SELECT version, checksum FROM finance_schema_migrations ORDER BY version;'
docker compose -f app/deploy/compose.prod.yaml exec postgres \
  psql -U zhigu -d zhigu -c 'SELECT version, checksum FROM futures_schema_migrations ORDER BY version;'
```

Expected: 版本单调、checksum 与镜像内 migration 一致；缺表/漂移时停止，不手工修补为通过。

---

## Task 7: 部署 worker、Go API 和前端入口

**Files:**
- `app/deploy/compose.prod.yaml`
- `app/deploy/nginx.prod.conf`
- `app/research-service/Dockerfile`
- `app/futures-service/Dockerfile`
- `app/server/Dockerfile`
- `app/web/Dockerfile`

**Interfaces:**
- Consumes: Task 6 数据库和 owner。
- Produces: `/livez`、`/readyz`、私有 `/metrics`、HTTPS 页面和 `/api/*` 反向代理。

- [ ] **Step 1: 启动 research worker**

```bash
docker compose -f app/deploy/compose.prod.yaml up -d research
curl -fsS http://127.0.0.1:8091/livez
curl -fsS http://127.0.0.1:8091/readyz
```

Expected: `research` healthcheck 通过；缺 `ZHIGU_INTERNAL_TOKEN` 或 Go URL 必须失败关闭。

- [ ] **Step 2: 启动 futures worker**

```bash
docker compose -f app/deploy/compose.prod.yaml up -d futures
curl -fsS http://127.0.0.1:8092/livez
curl -fsS http://127.0.0.1:8092/readyz
```

Expected: futures 服务 token 校验、quiesce/backup endpoint 可用；不把 `FUTURES_STORAGE_DIR` 放进镜像层。

- [ ] **Step 3: 启动 Go API**

```bash
docker compose -f app/deploy/compose.prod.yaml up -d server
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8080/readyz
METRICS_TOKEN="$(cat app/deploy/secrets/internal_token)"
curl -fsS -H "Authorization: Bearer $METRICS_TOKEN" http://127.0.0.1:8080/metrics | head
unset METRICS_TOKEN
```

Expected: `/readyz` 检查 DB 和 worker；`/metrics` 无 token 返回 `401`；错误响应 trace ID 稳定且不泄密。

- [ ] **Step 4: 启动 Nginx/Vue**

```bash
docker compose -f app/deploy/compose.prod.yaml up -d web
read -r -p 'EIP address: ' EIP_ADDR
read -r -p 'Public HTTPS origin: ' ZHIGU_PUBLIC_ORIGIN
curl -I "http://$EIP_ADDR/"
curl -I "$ZHIGU_PUBLIC_ORIGIN/"
curl -fsS "$ZHIGU_PUBLIC_ORIGIN/api/health" 2>/dev/null || true
```

Expected: 80 跳转 HTTPS；443 使用有效证书；SPA 深链接不回退成 API JSON；`/api/*` 不被静态 HTML 吞掉。

- [ ] **Step 5: 检查网络暴露**

```bash
ss -lntup
```

Expected: 公网只允许 22/80/443；5432/8080/8091/8092 不应绑定 `0.0.0.0` 且安全组不应放行。

---

## Task 8: 执行公网真实链路冒烟

**Files:**
- Create: `artifacts/release/2026-10-17/public-smoke.md`
- Create: `artifacts/release/2026-10-17/public-smoke.json`

**Interfaces:**
- Consumes: Task 7 HTTPS 入口、owner 登录。
- Produces: 用户可见链路证据；不包含真实 API key、密码或完整敏感数据。

- [ ] **Step 1: 浏览器/HTTP 入口**

验证：

1. `${ZHIGU_PUBLIC_ORIGIN}/login` 可加载；
2. owner 登录后跳转到研究入口；
3. `ZHIGU_DEPLOYMENT_ENVIRONMENT=production` 下登录不返回 JSON token；
4. cookie 具备 `Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`；
5. CSRF/Origin/Referer 保护生效。

- [ ] **Step 2: 研究主链路**

执行一次真实 `Vue -> Go -> PostgreSQL -> research worker -> report` 流程，记录请求 trace ID、状态、耗时和结果边界。

- [ ] **Step 3: 期货/策略链路**

执行一次真实 `Vue -> Go -> PostgreSQL -> futures worker` 的创建/读取/发布 gate 流程；无真实 provider/model 时保持 read-only，并明确 `NOT_RUN/BLOCKED`。

- [ ] **Step 4: 失败注入**

至少验证：

- worker 停止后 `/readyz` 失败，不生成完成报告；
- DB 不可用时不伪造成功；
- 重复请求不产生重复副作用；
- 401/403/404/409/422/429/503 错误 envelope 稳定；
- secret、DSN、Authorization 不进入前端、日志或 manifest。

- [ ] **Step 5: 写真实链路结论**

`public-smoke.md` 中明确：已验证链路、未验证 provider/model/数据授权、fixture 边界、trace ID 和剩余风险。没有真实执行就写 `NOT_RUN`。

---

## Task 9: 验证备份、升级和回滚

**Files:**
- `app/scripts/backup.sh`
- `app/scripts/restore-verify.sh`
- `app/scripts/upgrade-verify.sh`
- `app/scripts/rollback.sh`
- `app/deploy/compose.rollback.yaml`

**Interfaces:**
- Consumes: Task 7/8 稳定服务、备份密钥、上一版镜像和数据库 dump。
- Produces: 实际 RPO/RTO、恢复演练、N/N-1 兼容性和回滚证据。

- [ ] **Step 1: 创建备份**

```bash
export BACKUP_KEY_FILE=/etc/zhigu/backup.key
export BACKUP_OFFHOST_DIR=/mnt/zhigu-offhost-backup
export FUTURES_SERVICE_TOKEN_FILE=/opt/zhigu/app/deploy/secrets/futures_token
export ZHIGU_INTERNAL_TOKEN_FILE=/opt/zhigu/app/deploy/secrets/internal_token
bash app/scripts/backup.sh | tee artifacts/release/2026-10-17/backup.log
```

Expected: quiesce epoch、`pg_dump`、SQLite online backup、文档归档、SHA-256、age/openssl 加密、离机副本和保留 7 个日备份。

- [ ] **Step 2: 恢复到临时数据库**

```bash
BACKUP_ARCHIVE="$(tail -n 1 artifacts/release/2026-10-17/backup.log)"
bash app/scripts/restore-verify.sh "$BACKUP_ARCHIVE" /var/lib/zhigu/restore-check 2>&1 | tee artifacts/release/2026-10-17/restore.log
```

Expected: hash 通过、迁移 ledger 完整、表计数和文件 hash 合理；记录实际恢复耗时，不用计划值冒充 RTO。

- [ ] **Step 3: 升级/回滚验证**

```bash
bash app/scripts/upgrade-verify.sh --run 2>&1 | tee artifacts/release/2026-10-17/upgrade.log
read -r -p "Previous immutable server image digest: " ZHIGU_PREVIOUS_SERVER_IMAGE
export ZHIGU_PREVIOUS_SERVER_IMAGE
bash app/scripts/rollback.sh 2>&1 | tee artifacts/release/2026-10-17/rollback.log
unset ZHIGU_PREVIOUS_SERVER_IMAGE
```

Expected: 前一版 DB 能被当前应用迁移，当前 DB 至少兼容前一版核心查询；回滚后 `/readyz` 和用户计数/关键数据不丢失。

- [ ] **Step 4: 启用维护 timer**

```bash
systemctl list-timers | grep zhigu
systemctl status zhigu-maintenance.timer zhigu-watchdog.service --no-pager
```

Expected: 每日备份、磁盘/证书/队列/预算检查和告警均实际调度；watchdog 不是 healthcheck 的替代品。

---

## Task 10: 生成发布证据并决定后续架构

**Files:**
- Create: `artifacts/release/2026-10-17/release-manifest.json`
- Create: `artifacts/release/2026-10-17/final-acceptance.md`
- Modify: `app/deploy/README.md`（仅补充实际命令和已验证状态）

**Interfaces:**
- Consumes: Task 1-9 的日志、镜像 digest、备份和 smoke 证据。
- Produces: 可审计发布记录和下一阶段决策。

- [ ] **Step 1: 重新生成 manifest**

```bash
python3 app/scripts/release-evidence.py \
  --output artifacts/release/2026-10-17/release-manifest.json \
  --version 2026-10-17-mvp \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/postgres.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/research.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/futures.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/server.txt)" \
  --image-digest "$(cat artifacts/release/2026-10-17/digests/web.txt)" \
  --test 'bash app/scripts/verify-production.sh --static' \
  --frontend-result artifacts/release/2026-10-17/frontend/results.json \
  --evidence static=artifacts/release/2026-10-17/static-verifier.log \
  --evidence smoke=artifacts/release/2026-10-17/public-smoke.json \
  --evidence backup=artifacts/release/2026-10-17/backup.log \
  --evidence restore=artifacts/release/2026-10-17/restore.log
```

Expected: manifest `dirty=false` 才能作为正式发布基线；否则只能作为开发快照，不能对外宣称 release complete。

- [ ] **Step 2: 填写 final-acceptance**

必须逐项写 `PASS/FAIL/BLOCKED/NOT_RUN`：

- ECS 容量和 EIP 绑定；
- DNS/ICP/TLS；
- Docker/Compose；
- migration ledger；
- `/livez`、`/readyz`、`/metrics`；
- 登录/CSRF/session；
- 研究真实链路；
- futures 真实链路；
- provider/model 探针；
- 备份/恢复；
- 升级/回滚；
- 公网 HTTPS；
- secret 扫描；
- release manifest。

- [ ] **Step 3: 做 RDS/单机决策**

若以下任一条件成立，下一阶段迁移火山云托管 RDS PostgreSQL：

- ECS 资源不足或数据库负载影响 worker；
- 需要自动备份、主备高可用、PITR 或异地容灾；
- 需要独立数据库权限、审计或容量扩展；
- 需要将 API/worker 与数据库分别升级。

迁移前先做 schema/数据 dump、网络权限、TLS/私网连接、N/N-1 验证和回滚演练。

- [ ] **Step 4: 交接给后续 Agent**

后续 Agent 必须先读：

1. 本文；
2. `app/deploy/README.md`；
3. `docs/superpowers/plans/2026-10-13-backend-production.md`；
4. `docs/superpowers/plans/2026-10-13-frontend-productization.md`；
5. `spec/后端生产化_开发SPEC.md`；
6. `spec/期货模块_开发SPEC.md`（如存在）。

执行规则：

- 每次只处理一个 Task；
- 每个 Task 结束记录命令、退出码和证据路径；
- 不修改与当前 Task 无关的未提交文件；
- 不提交 secret、token、DSN、私钥或用户隐私；
- 遇到外部账号、费用、备案、证书购买或生产开关变更，暂停请求用户确认；
- 将未完成项保留为 `BLOCKED/NOT_RUN`，不通过改文档伪装完成。

## Definition of Done

只有同时满足以下条件，才可写“公网 MVP 已上线”：

- `app/scripts/production-preflight.sh` 在干净主机退出 `0`；
- `app/scripts/verify-production.sh --full` 退出 `0`；
- `app/scripts/verify-frontend.sh` 产生真实测试结果且 Playwright 通过；
- `app/scripts/upgrade-verify.sh --run` 通过；
- `app/scripts/backup.sh` 与 `app/scripts/restore-verify.sh` 完成真实恢复；
- 公网 HTTPS、登录、研究、策略/期货主链路均可访问；
- `/readyz`、私有 `/metrics` 和告警可用；
- release manifest 不含 secret，记录镜像 digest、迁移版本、Git SHA 和已知限制；
- 真实 provider/model/数据授权单独标记已验证或未验证；
- 用户已经确认公网、域名/备案和产生费用的云资源操作。

## 当前推荐执行顺序

```text
1. 冻结当前 dirty 工作树，完成发布边界
2. 核实 ECS vCPU/RAM/磁盘/EIP/安全组
3. 确认域名、ICP、TLS
4. 修复/完成 backend + frontend production plan
5. 构建不可变镜像并保存 digest
6. 引导 ECS、secret、systemd
7. 启动 PostgreSQL -> migration/owner -> worker -> server -> web
8. HTTPS smoke + 真实研究/期货链路
9. backup/restore + upgrade/rollback
10. release manifest + RDS/单机架构决策
```
