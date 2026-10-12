# 知股后端生产化 · 开发 SPEC

版本 BPROD-SPEC-1.0｜2026-10-12｜适用基线：`codex/futures-review-release`。

本文定义知股个人项目从“离线工程预览”达到“完整、稳定、可在单台 Linux 主机部署”的后端开发合同。目标不是在开发机提前证明真实供应商长期可用，而是先完成所有后端组件、稳定性与生产部署能力；部署上线后再注入真实凭据并执行真实数据与模型验证。

本文与以下文件共同构成开发约束：

- [`期货研究模块_开发SPEC.md`](./期货研究模块_开发SPEC.md)：期货领域、API、数据和研究门禁；
- [`投研观点模块_开发SPEC.md`](./投研观点模块_开发SPEC.md)：观点研究与报告发布规则；
- [`阶段B_开发SPEC.md`](./阶段B_开发SPEC.md)：股票数据与真实模型边界；
- `contracts/`、`app/contracts/`：机器可读契约；
- [`前端产品化_开发SPEC.md`](./前端产品化_开发SPEC.md)：浏览器端产品合同。

若本文与机器契约冲突，先记录差异并同步修正文档和契约，不得让实现静默选择更宽松行为。本文不放宽既有安全、证据、预算、权限、删除与引用校验规则。

## 1. 交付目标与发布顺序

### 1.1 唯一交付目标

交付一个可重复构建、可在干净 Linux 主机部署、可观测、可备份恢复、可回滚的单用户生产系统。部署完成后，站点、API、两个研究 worker、PostgreSQL、迁移、持久化、健康检查和故障降级必须完整运行。

上线前允许真实供应商与真实模型探针处于 `NOT RUN`，但必须满足：

1. 对接适配器、配置入口、权限检查、超时、限流、失败映射和验证脚本已经实现；
2. 不配置真实凭据时明确返回 disabled/unavailable，不产生 fixture 报告；
3. fixture 只在测试或显式 demo 环境启用，生产环境启动时默认禁止；
4. 上线后无需改代码或重建镜像，只需注入 secret、切换配置并运行探针即可接入真实数据和模型。

### 1.2 强制顺序

```text
组件完整
  → 离线与故障稳定性验收
  → 生产镜像和 Compose 验收
  → 干净 Linux 主机部署
  → 注入真实数据/模型凭据
  → 线上真实链路验证
  → 个人正式使用
```

禁止把线上主机当开发环境。部署前不得遗留空实现、假成功接口、占位 worker、只在开发机存在的依赖或必须手工修改容器文件的步骤。

### 1.3 发布门禁

| 门禁 | 含义 | 是否允许真实凭据缺失 |
|---|---|---|
| R0 组件完成 | 所有服务、路由、页面所需 API、迁移、任务和维护路径已实现 | 是 |
| R1 稳定性 | 单元、契约、集成、竞态、故障、负载、备份恢复测试通过 | 是 |
| R2 可部署 | 镜像、Compose、配置、HTTPS、健康检查、升级和回滚通过 | 是 |
| R3 线上接入 | 真实来源和模型在已部署环境完成连接与最小样本验证 | 否 |
| R4 正式使用 | 核心真实旅程通过，失败模式明确，无 fixture 污染 | 否 |

本项目是个人项目，不设计多租户、灰度放量、运营审批、组织权限或分批发布。`off/read_only/live` 仅作为故障控制和数据安全开关。

## 2. 范围与非目标

### 2.1 必须交付

- `server`、`research-service`、`futures-service`、`web`、`postgres` 五个生产服务；
- 两个 Python worker 的独立不可变镜像与依赖锁；
- PostgreSQL 数据和两个 worker 私有数据的持久卷；
- 统一生产配置、secret 注入、启动前校验与配置摘要；
- 数据库新装、增量升级、失败回滚和 checksum 检查；
- API/worker 超时、取消、重试、预算、幂等、租约和重启恢复；
- `/healthz`、`/readyz`、结构化日志、运行指标和可执行诊断；
- 备份、恢复、清理、保留、导出和回滚脚本；
- 生产 Compose 构建、启动、升级、重启、备份恢复和回滚验证；
- 上线后真实来源、真实模型和数据质量探针入口；
- 与前端一致的错误码、能力状态和运行模式。

### 2.2 明确不做

- Kubernetes、服务网格、分布式队列或多节点高可用；
- Redis、Kafka、ClickHouse、向量数据库等无实测必要的新基础设施；
- 多租户、团队空间、组织 RBAC、计费售卖或公众注册；
- 自动交易、券商下单、资金托管或收益承诺；
- 为上线验证绕过来源授权、模型费用或证据发布门禁；
- 在容器启动时从宿主源码 bind mount 运行生产服务；
- 在生产启动命令里执行 `pip install`、`npm install` 或动态下载工具链。

## 3. 当前基线与必须消除的缺口

### 3.1 已具备

- Go 业务宿主、PostgreSQL、研究服务和期货独立 worker；
- 股票观点、策略、事件情报和期货研究的领域代码与测试；
- 期货严格契约、预算、grant、删除、导出、任务调度和报告核验；
- Web 构建、fixture 浏览器流程、公开树检查和 GitHub Actions；
- 基础 `server`、`web` Dockerfile 和 `compose.prod.yaml`。

### 3.2 发布阻塞

| ID | 当前缺口 | 完成要求 |
|---|---|---|
| BG-01 | 生产 Compose 没有 `futures-service` | 增加独立服务、健康检查、token、网络和依赖关系 |
| BG-02 | 生产 Web 未构建期货开关 | 所有发布功能开关在镜像构建时显式固定并记录 |
| BG-03 | research 容器启动时安装 uv 和依赖 | 建立多阶段不可变镜像，启动不联网 |
| BG-04 | 生产编排固定 fixture executor | 生产默认禁止 fixture；未配置真实能力时 fail-closed |
| BG-05 | Docker/Compose 未在干净宿主验收 | CI 和目标 Linux 主机均执行构建及启动验证 |
| BG-06 | CI 未固定运行期货 race | 将期货 service/API race 纳入必过任务 |
| BG-07 | 无完整 ready/metrics/诊断合同 | 实现服务级和依赖级可观测接口 |
| BG-08 | 无自动化备份恢复证据 | 提供脚本并在临时数据库完成恢复核验 |
| BG-09 | 缺升级/回滚演练 | 从上一发布版本升级，再回滚应用且保留新表数据 |
| BG-10 | 真实探针与发布状态混淆 | `PASS/FAIL/NOT_RUN/BLOCKED` 四态严格区分 |

## 4. 目标拓扑与依赖方向

### 4.1 单机生产拓扑

```text
Internet
  → Nginx(Web 镜像，TLS，同源 /api 代理)
      → Go server :8080
          → PostgreSQL :5432
          → research-service :8091
          → futures-service :8092
          → 外部数据/模型 HTTPS
```

仅 `web` 暴露宿主 80/443；Go、PostgreSQL 和 worker 只加入 Compose 私有网络。数据库端口、worker 端口和内部 API 不映射到公网。开发 Compose 可保留回环端口，生产 Compose 不得复用开发凭据。

### 4.2 服务职责

| 服务 | 唯一职责 | 禁止事项 |
|---|---|---|
| web | 静态资源、TLS、同源 API 反向代理、安全响应头 | 不保存 API key，不直连 worker/数据库 |
| server | JWT、领域状态、数据库、预算、证据、发布门禁、worker 调度 | 不在请求内运行任意模型代码，不返回 secret |
| research-service | 股票观点受限研究执行 | 不直写业务 DB，不持有用户 JWT |
| futures-service | 期货受限研究、材料解析和来源 adapter | 不访问股票内部状态，不发布最终报告 |
| postgres | 持久化业务事实、版本、审计和任务状态 | 不对公网开放 |

### 4.3 文件责任

必须按现有结构实施，避免新建通用平台层：

```text
app/deploy/
  compose.prod.yaml              生产拓扑唯一入口
  .env.prod.example              非敏感生产配置模板
  scripts/preflight.sh           主机与配置预检
  scripts/deploy.sh              构建、迁移、启动、ready 验证
  scripts/backup.sh              PostgreSQL/worker 数据一致备份
  scripts/restore-verify.sh      在临时目标中恢复并核验
  scripts/rollback.sh            镜像回退与模式关闭
app/server/
  Dockerfile                     不可变非 root 镜像
  internal/runtime/              配置校验、版本信息、健康聚合
app/research-service/
  Dockerfile                     锁文件构建、非 root、无启动下载
app/futures-service/
  Dockerfile                     独立锁文件构建、非 root、无启动下载
app/web/
  Dockerfile                     固定构建参数
  nginx.conf                     TLS、API 代理、SPA、缓存与安全头
app/scripts/
  verify-production.sh           R0–R2 统一验证和 manifest
  verify-live-connectors.sh      R3 真实来源验证
  verify-live-models.sh          R3 真实模型验证
```

文件名可在保持职责一一对应的前提下沿用仓库既有脚本命名；不得把所有逻辑塞进一个超过 500 行的 shell 脚本。

## 5. 配置、Secret 与运行模式

### 5.1 配置来源

生产只接受环境变量和只读 secret 文件。优先使用 Docker Compose `secrets`；本地开发可使用被 `.gitignore` 排除的 `.env`。任何 API key、JWT secret、内部 token、数据库密码和 Cookie secret 不得进入：

- Git、镜像层、前端构建变量；
- 日志、错误响应、manifest、报告或导出；
- Compose 默认值或 example 文件示例值；
- 浏览器 localStorage 之外的非必要位置；JWT 迁移另行评估，不在本 SPEC 扩展。

### 5.2 必需生产变量

| 类别 | 变量 | 规则 |
|---|---|---|
| 基础 | `ZHIGU_ENV=production` | 缺失则不是生产模式 |
| URL | `ZHIGU_PUBLIC_ORIGIN` | 必须为 HTTPS、无 path、禁止环回/私网域名 |
| DB | `POSTGRES_USER/DB/PASSWORD` | 密码无默认值；DSN 由 Compose 内部生成 |
| 身份 | `ZHIGU_JWT_SECRET` | 至少 32 随机字节，禁止示例值 |
| 内部 | `ZHIGU_INTERNAL_TOKEN` | Go↔research 独立 token |
| 期货 | `FUTURES_SERVICE_TOKEN` | Go↔futures 独立 token，不得等于 internal token |
| 期货模式 | `ZHIGU_FUTURES_ENABLED/MODE` | 首次部署默认 `true/read_only` 或 `false/off` |
| 事件 | `ZHIGU_INTEL_ENABLED/MODE` | 未完成真实验证时使用 `demo` 或关闭 |
| 数据 | provider URL、secret ref | 缺失时 provider disabled，不回退 fixture |
| 模型 | model URL/name/version/price/key | 缺一项则该模型 disabled |

### 5.3 启动校验

`server` 启动前生成不含 secret 的配置摘要，至少包含：版本 SHA、构建时间、环境、启用模块、模式、provider 名称、模型配置版本、迁移版本和持久目录。以下情况生产启动必须失败：

- secret 为空、为仓库示例值或长度不足；
- `PUBLIC_ORIGIN` 不是 HTTPS；
- `ZHIGU_RESEARCH_EXECUTOR=fixture`；
- live 模式缺费率、模型版本或必要 provider；
- 持久目录不可写或位于容器临时层；
- 数据库迁移 checksum 冲突；
- 两个内部 token 相同。

模块 `off/read_only` 且真实凭据缺失时宿主可以健康启动，但 `/readyz` 必须返回每个模块的精确状态与 reason，前端据此展示。

## 6. 镜像与 Compose 合同

### 6.1 镜像

所有镜像必须：

- 由锁文件确定依赖；
- 多阶段构建，运行层不含编译器和包管理缓存；
- 使用固定主版本基础镜像，并由 Dependabot 管理 digest/版本更新；
- 以非 root 用户运行；
- 包含 OCI labels：revision、source、created、version；
- 支持只读根文件系统，写入仅发生在声明卷或 `/tmp`；
- 启动不访问包仓库；
- 收到 SIGTERM 后 20 秒内停止接收新任务并完成安全退出。

### 6.2 Compose

`compose.prod.yaml` 必须包含：

- `postgres`、`research`、`futures`、`server`、`web`；
- 显式私有网络；
- PostgreSQL、worker 私有数据和期货文档三个命名卷；
- 服务级 healthcheck；
- `depends_on.condition=service_healthy` 仅用于启动排序，应用仍实现重连；
- `restart: unless-stopped`；
- CPU/内存/PID/日志轮转上限；
- secret 只读挂载；
- 镜像 tag 使用 release version 或 commit SHA，禁止生产 `latest`；
- Web 构建参数同时固定 `VITE_INTEL_ENABLED` 与 `VITE_FUTURES_ENABLED`。

### 6.3 Web 代理和 TLS

Nginx 必须：

- 443 提供 TLS，80 仅 301 到 HTTPS；
- `/api/` 代理 Go，保留 `Host`、`X-Forwarded-*`、request ID；
- API 响应禁止 SPA fallback；
- SPA 深链接 fallback 到 `index.html`；
- `index.html` 使用 `no-cache`，带 hash 静态资源使用一年 immutable cache；
- 设置 CSP、HSTS、`X-Content-Type-Options`、`Referrer-Policy` 和 frame 限制；
- 上传体积上限与应用 20 MiB 限制一致并留协议开销；
- API/导出/事件流配置适当超时，不能使用一个无限全局 timeout。

证书由宿主 ACME 工具签发并以只读文件挂载；续期后执行 Nginx reload。证书签发本身不进入业务容器。

## 7. 数据库、迁移与持久化

### 7.1 迁移

- 新装必须从空 PostgreSQL 16 完整迁移成功；
- 升级必须从上一正式 release 的数据库快照迁移成功；
- 每个迁移有不可变版本和 checksum；
- 迁移使用事务或明确记录不可事务 DDL 的恢复步骤；
- 迁移失败不得继续启动写流量；
- 不自动 drop 表、列或用户数据；破坏性清理由独立、显式、备份后的维护命令执行；
- futures 独立迁移失败只允许宿主以 futures disabled/read_only 运行，不得宣称 futures ready。

### 7.2 持久目录

| 数据 | 生产位置 | 备份 |
|---|---|---|
| PostgreSQL | `pgdata` volume | 必须 |
| research SQLite/task copy | `researchdata` volume | 必须，与 DB 同批次 |
| futures worker 状态 | `futuresdata` volume | 必须，与 DB 同批次 |
| 期货上传文档 | `futuresdocuments` volume | 必须，遵循删除/保留规则 |
| 临时提取文件 | 容器 `/tmp` | 不备份，任务终止清理 |

### 7.3 备份与恢复

`backup.sh` 必须：

1. 将两个模块切到 `read_only`，停止领取新任务；
2. 等待活动写事务结束或在超时后明确失败；
3. `pg_dump --format=custom`；
4. 对 worker DB/文件和文档卷创建带校验和的归档；
5. 生成 manifest：版本、时间、DB 迁移版本、文件 hash、对象数量；
6. 恢复原运行模式。

`restore-verify.sh` 必须在新的临时数据库与临时卷中恢复，验证迁移版本、核心表计数、最近一条 run/hypothesis、tombstone 不复活、文件 hash 和健康检查。只有“备份文件生成”不算恢复通过。

### 7.4 保留与删除

延续既有 24 小时私有内容清理、审计保留和 tombstone 规则。生产环境必须运行清理 worker，指标显示待清理数量、最老任务年龄和最近成功时间。清理失败只重试幂等步骤，不恢复已删除正文。

## 8. Worker、调度与故障语义

### 8.1 Worker API

两个 worker 均提供：

- `GET /healthz`：进程存活，不检查付费上游；
- `GET /readyz`：配置、持久目录、Go 内部入口可用；
- 受 token 保护的执行、取消、状态、ACK/Purge 接口；
- 请求体严格 schema，拒绝未知字段和额外 JSON；
- request ID、run/task/generation 结构化日志；
- context deadline 和客户端断开向下游传播。

### 8.2 调度不变量

- 领取任务使用数据库锁和 generation，不依赖进程内唯一状态；
- 宿主重启后 queued 可继续，running/verifying 按既有规则失败或恢复，不能重复付费；
- canceling 在 ACK 或租约过期前占用执行槽；
- `off/read_only` 在 60 秒内停止新付费调用；
- 迟到结果不能覆盖较新 generation、取消、删除或新 draft revision；
- usage 未知时保留费用 reservation；
- 任何模型/工具错误不得转换成“证据不足的成功报告”。

### 8.3 重试

| 操作 | 自动重试 |
|---|---|
| DB 短暂连接失败 | 有界指数退避，最多 3 次 |
| 只读 provider GET | 遵守 Retry-After，最多 3 次 |
| 付费模型调用 | 默认不自动跨 attempt 重试；由冻结预算允许的一次 repair 管理 |
| 幂等 outbox 通知 | 可重试，dedupe key 保证一次可见 |
| 创建 run/删除/预算 | 不以 HTTP 重试代替数据库幂等 |
| schema/权限/引用错误 | 不重试 |

## 9. API 与错误合同

所有 API 延续 `{data,error,trace_id}` envelope。生产必须统一以下错误属性：

```json
{
  "data": null,
  "error": {
    "code": "FUTURES_SOURCE_UNAVAILABLE",
    "message": "当前数据来源暂不可用，请稍后重新检查",
    "retryable": true
  },
  "trace_id": "01J..."
}
```

- `code` 稳定、可供前端映射；
- `message` 是安全的用户文本，不含 URL、SQL、路径、secret 或上游原文；
- `retryable` 只表示重复同一只读请求可能成功，不授权自动重做付费操作；
- 429/503 设置合理 `Retry-After`；
- 非本人对象和不存在对象统一 404；
- 内部错误记录 cause chain，外部只返回稳定码；
- 每个响应有 trace ID，Nginx、Go 和 worker 贯通同一 ID。

新增或改变字段必须先更新机器契约、负例和前端映射，不能只改某一层。

## 10. 上线后真实数据和模型接入

### 10.1 上线前已完成的能力

每个 provider/model adapter 在上线前必须具备：

- 严格配置 schema 和 secret reference；
- 连接超时、总超时、响应体上限、重定向规则和 TLS 校验；
- 分页、覆盖、水位、发布时间/版本时间和单位映射；
- 原始响应 fixture、契约测试、畸形响应和限流测试；
- `probe` 命令，默认只读且有费用上限；
- `PASS/FAIL/NOT_RUN/BLOCKED` 证据状态；
- 禁止以 fixture 填充 live namespace。

### 10.2 部署后的接入步骤

1. 模块保持 `read_only`；
2. 通过 secret 文件注入 provider/model 凭据；
3. 重载服务并核对配置摘要，不输出 secret；
4. 运行连接、权限、覆盖、分页、发布时间和费率探针；
5. 对固定小样本执行真实数据读取；
6. 对固定少量研究执行真实模型调用和发布校验；
7. 核对预算、日志脱敏、数据来源、报告引用和删除；
8. 探针通过后由所有者显式切换 `live`。

本项目不要求灰度用户或运营签收。真实探针失败时保持 `read_only`，修复配置或 adapter 后重跑；不得修改数据库状态伪造 PASS。

## 11. 安全与隐私

### 11.1 网络

- 仅 Web 暴露公网；
- worker 和 DB 只监听容器私网；
- 出站只允许声明的 HTTPS provider/model 域名；
- 用户 URL 抓取继续执行 SSRF 防护、DNS/IP 二次校验和响应上限；
- Nginx 与 Go 均设置请求体、header、连接和速率限制。

### 11.2 身份与授权

- 个人项目仍保留 JWT 和 owner scope；不能用“只有我使用”删除后端授权；
- admin API 后端强制 admin role；
- 内部 token 与用户 JWT 分离；
- futures grant 继续绑定 owner/mode/run/task/generation/tool/args hash/nonce/expiry；
- Cookie 设置 `HttpOnly/Secure/SameSite`，Origin 检查与 PUBLIC_ORIGIN 一致。

### 11.3 文件和内容

- PDF/DOCX/TXT 按魔数、大小、页数、压缩比和路径穿越检查；
- 提取进程限制 CPU、内存、文件数和超时；
- 外部内容中的指令只作为材料；
- HTML 导出严格 escape 并遵守来源再分发权；
- 日志不记录正文、完整 URL query、token、API key 或原始模型 prompt。

### 11.4 依赖

CI 执行 Go、Python、npm 依赖漏洞审查和 secret 扫描。阻断规则：已知可利用的 critical/high 且运行路径可达时不得发布；例外必须记录包、CVE、不可达证据、到期日和修复版本。

## 12. 可观测性与运维

### 12.1 健康合同

| 接口 | 成功条件 | 失败影响 |
|---|---|---|
| `/healthz` | Go 进程事件循环可响应 | 容器重启 |
| `/readyz` | DB 可查询、迁移一致、必需 worker ready | Nginx 不应发送业务流量 |
| worker `/healthz` | 进程可响应 | worker 重启 |
| worker `/readyz` | token、持久目录、Go internal 可用 | 相关模块不可创建新任务 |

`/readyz` 返回模块数组，包含 `enabled/ready/mode/reason/version`；不因一个可选 provider 失败把整个宿主伪装成宕机，但对应模块必须 fail-closed。

### 12.2 日志

所有服务输出 JSON 到 stdout/stderr，至少包含：

- timestamp、level、service、version；
- trace_id、request_id；
- owner 的不可逆匿名标识；
- run_id/task_id/generation/stage（适用时）；
- error_code、duration_ms、upstream、attempt；
- 不含 secret 和私有正文。

### 12.3 指标

至少提供：HTTP 请求数/延迟/状态、DB pool、任务队列长度和最老年龄、各状态 run 数、worker 调用延迟、provider/模型调用与错误、预算 reservation/spent/unknown、清理积压、备份时间、迁移版本。个人部署可先使用 Prometheus 文本端点和结构化日志，不要求单独引入 Grafana 服务。

### 12.4 告警

个人项目的最小告警通过部署脚本或外部 uptime 检查完成，覆盖：站点不可达、ready 失败、磁盘低于 15%、备份超过 26 小时未成功、队列最老任务超阈值、清理失败、预算异常和证书 14 天内过期。

## 13. 性能与资源预算

上线前使用 fixture/本地 mock 执行可重复负载，不依赖真实供应商：

| 场景 | 目标 |
|---|---|
| 健康/能力查询 | 20 RPS，15 分钟，P95 < 300 ms，错误率 < 0.1% |
| 普通只读 API | 10 RPS，15 分钟，P95 < 800 ms |
| 创建草稿/解析排队 | 2 RPS，5 分钟，无重复对象、无预算负数 |
| 期货调度 | 并发 2，取消/超时/重启后无状态复活 |
| 假设检查 | 2000 条在 5 分钟内完成或保持可观测 backlog |
| 文件上传 | 20 MiB 上限下内存稳定，无全文件多份复制 |
| 容器重启 | 60 秒内 ready；活动任务按状态机收敛 |

阈值是单台 2 vCPU/4 GiB Linux 主机的最低验收目标。若目标主机更小，必须在发布 manifest 记录实测配置和降低后的并发值，不能取消正确性门禁。

## 14. 测试矩阵

### 14.1 上线前必过

| 层 | 必须执行 |
|---|---|
| Go | `go test ./...`；finance/intel/futures/API race；迁移、取消、预算、删除、重启 |
| research Python | 锁文件安装；全部非 live 测试；SQLite 重启和 purge |
| futures Python | 锁文件安装；全部 unittest；HTTP/transport/provider 故障 |
| 契约 | OpenAPI/JSON Schema 正反例；Go/Python/Web 字段一致性 |
| Web | production build、Node tests、完整 Playwright fixture 流程 |
| 集成 | Web→Go→PostgreSQL→两个 worker；禁止全层 mock |
| 安全 | secret、依赖、SSRF、IDOR、上传、导出、日志脱敏 |
| 生产 | Docker build、Compose config、空库启动、升级、restart、backup/restore、rollback |
| 性能 | §13 全部场景 |

测试不得静默 skip。允许的 skip 只有真实 provider/model 探针，必须输出 `NOT_RUN` 和缺失条件；浏览器、Docker 或数据库依赖缺失必须使 R1/R2 验证失败，而不是记为通过。

### 14.2 故障注入

必须覆盖：

- PostgreSQL 启动晚、短暂断连、磁盘只读；
- worker 未启动、处理中退出、返回畸形 JSON、迟到成功；
- provider 401/403/404/429/500、分页循环、超大响应、超时；
- 模型 429、usage 缺失、schema 错误、引用不存在；
- Nginx 413、API 超时、客户端中断；
- 迁移中途失败、checksum 不符；
- 备份过程中活动任务、恢复后 tombstone；
- off/read_only 切换时并发创建与付费调用。

## 15. CI/CD 与证据产物

### 15.1 CI jobs

在现有 CI 基础上增加：

1. `go-race`：包含 futures service/API；
2. `images`：构建四个应用镜像并运行容器 health；
3. `compose-smoke`：空卷启动完整拓扑，运行 fixture 集成后销毁；
4. `upgrade`：从上一 release fixture DB 升级；
5. `backup-restore`：生成备份并恢复到新卷；
6. `security`：依赖与 secret 扫描；
7. `release-manifest`：汇总版本、镜像 digest、测试、skip 和 artifact hash。

### 15.2 发布产物

每次候选发布生成 `artifacts/release/<version>/manifest.json`，至少包含：

- Git SHA、dirty 状态、构建时间；
- 四个镜像 digest；
- 配置 schema 版本、迁移版本；
- 各测试命令、退出码、通过/失败/skip 数；
- R0–R4 状态；
- 真实探针 `NOT_RUN/BLOCKED/PASS/FAIL`；
- 备份恢复和回滚证据路径；
- 已知限制。

只有 R0–R2 全部 PASS 才可标记“可部署”。只有 R3 PASS 才可切换真实链路；fixture 结果不得改变 R3。

## 16. 开发阶段

### M1：生产配置与 fail-closed

- 建立统一环境校验和脱敏配置摘要；
- 生产禁止 fixture；
- 补齐 futures/research ready 状态与错误码；
- 契约、单测和前端状态字段同步。

### M2：不可变镜像与完整 Compose

- 为两个 Python worker 建 Dockerfile；
- 修复 server/web 镜像；
- 增加 futures 服务、卷、secret、网络、限制和 health；
- 在本机/CI 完成无网络启动验证。

### M3：迁移、持久化与恢复

- 空库/升级迁移；
- 备份、恢复核验、清理积压指标；
- 回滚应用但保留新数据结构。

### M4：可观测性与安全

- ready/metrics/JSON 日志；
- TLS、安全头、速率和体积限制；
- 依赖、secret、SSRF、IDOR 和日志脱敏检查。

### M5：稳定性与发布验证

- 完整集成、race、故障注入、负载；
- `verify-production.sh` 与 manifest；
- 在干净 Linux 主机完成 R2。

### M6：上线后真实接入

- 注入 secret，运行 provider/model 探针；
- 固定小样本真实链路；
- 确认预算、引用、删除和 fail-closed；
- R3 通过后由所有者切 `live`。

阶段必须按顺序通过；不得用 M6 的一次真实成功替代 M1–M5 的工程门禁。

## 17. 可执行验收项

### 17.1 组件与部署

| ID | 验收 |
|---|---|
| BPR-01 | 生产 Compose 明确包含五个服务，只有 Web 暴露公网 |
| BPR-02 | 所有应用镜像启动不联网、不 bind mount 源码、以非 root 运行 |
| BPR-03 | 期货 Web flag、server mode、worker URL/token 和存储卷完整接线 |
| BPR-04 | 空 Linux 主机仅安装 Docker/Compose、配置 secret 后可部署 |
| BPR-05 | HTTPS、SPA 深链和 `/api/*` 路由正确，API 错误不回退 HTML |
| BPR-06 | `docker compose restart` 后数据、任务和模式正确恢复 |
| BPR-07 | 缺真实凭据时系统健康但能力 disabled/read_only，绝不产 fixture live 数据 |
| BPR-08 | 生产配置错误在启动前失败并给出不泄密的精确原因 |

### 17.2 数据与状态

| ID | 验收 |
|---|---|
| BPR-09 | 空库和上一 release 数据库均迁移成功 |
| BPR-10 | migration checksum 冲突阻止相关模块 ready |
| BPR-11 | 备份可恢复到新卷，核心计数/hash 一致 |
| BPR-12 | 删除后的正文和文件不因恢复而复活 |
| BPR-13 | worker 退出、宿主重启和迟到结果均按状态机收敛 |
| BPR-14 | canceling、lease、generation 和预算 race 测试通过 |
| BPR-15 | off/read_only 60 秒内停止新付费调用 |
| BPR-16 | 清理任务可观测且重复执行幂等 |

### 17.3 安全与可观测

| ID | 验收 |
|---|---|
| BPR-17 | 外部只可访问 80/443，DB/Go/worker 端口不可公网连接 |
| BPR-18 | secret 扫描、日志扫描、镜像历史扫描无真实凭据 |
| BPR-19 | owner/admin/internal grant 权限负例全部通过 |
| BPR-20 | SSRF、路径穿越、zip bomb、XSS 导出负例通过 |
| BPR-21 | trace ID 贯通 Nginx→Go→worker |
| BPR-22 | ready 能区分宿主、模块、provider 和模型状态 |
| BPR-23 | 队列、预算、清理、备份、证书和磁盘均有可检测指标/告警 |
| BPR-24 | 用户错误响应无 SQL、路径、上游正文和 secret |

### 17.4 测试与发布

| ID | 验收 |
|---|---|
| BPR-25 | Go 全量和规定 race 命令退出 0 |
| BPR-26 | 两个 Python worker 全量非 live 测试退出 0 |
| BPR-27 | 契约正反例和跨语言一致性测试退出 0 |
| BPR-28 | Web build、浏览器 fixture E2E 与真实本地集成 E2E 退出 0 |
| BPR-29 | Docker build、Compose smoke、升级、恢复、回滚退出 0 |
| BPR-30 | §13 性能基线通过且无状态/预算错误 |
| BPR-31 | 所有 skip 仅为真实探针且标为 NOT_RUN |
| BPR-32 | release manifest 可追溯到 commit、镜像 digest 和测试输出 |

### 17.5 上线后真实接入

| ID | 验收 |
|---|---|
| BPR-33 | 无需改代码即可注入 provider/model secret |
| BPR-34 | provider 探针验证权限、分页、覆盖、时间、单位和来源版本 |
| BPR-35 | 模型探针验证协议、费率、usage、schema 和预算 |
| BPR-36 | 固定小样本真实研究报告通过引用与数字发布校验 |
| BPR-37 | 探针失败保持 read_only 且前端展示真实原因 |
| BPR-38 | 删除真实样本后主存、worker 副本和文件按规则清理 |
| BPR-39 | R3 manifest 不包含凭据、私有原文或未授权数据 |
| BPR-40 | 所有者显式切换 live 后才允许正式研究 |

## 18. 完成定义

后端生产化只有同时满足以下条件才完成：

1. BPR-01–BPR-32 全部 PASS；
2. 在干净 Linux 主机完成一次部署、重启、备份恢复和应用回滚；
3. 生产镜像无源码 bind mount、无启动下载、无默认 secret、无 fixture 回退；
4. 前端能基于真实 capabilities/ready/error 状态工作；
5. release manifest 将真实探针准确标为 NOT_RUN、BLOCKED、PASS 或 FAIL；
6. 文档中的命令由未参与开发的人按步骤可以复现；
7. 仓库没有未解释测试 skip、临时调试路由或“成功”占位实现。

部署上线后，BPR-33–BPR-40 是真实接入完成定义。它们不阻止 R2“可部署”结论，但未通过前不得把真实能力标为可用。
