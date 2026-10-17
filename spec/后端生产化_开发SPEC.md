# 知股后端生产化 · 开发 SPEC

版本 BPROD-SPEC-1.1｜2026-10-13｜适用基线：`codex/futures-review-release`。

1.1 修订重点：生产认证改为 fail-closed；拆分部署环境、操作模式、数据来源和健康状态；补齐机器错误合同、真实探针发布闸门、一致性备份与可执行运维责任。实现和验收均以本版本为准。

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
2. 不配置真实凭据时明确返回 `operation_mode=off/read_only`、`health=unavailable/degraded` 和原因，不产生 fixture 报告；
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

### 1.4 正交运行状态

以下四个维度必须分别建模、持久化或由服务端计算，禁止复用一个 `mode/status` 字段表达多种含义：

| 维度 | 允许值 | 含义 |
|---|---|---|
| `deployment_environment` | `test/demo/production` | 部署隔离级别；`demo` 是环境，不是业务运行模式 |
| `operation_mode` | `off/read_only/live` | 是否可读、可写及可发起付费调用 |
| `data_provenance` | `fixture/real` | 数据和报告的实际来源 |
| `health` | `ready/degraded/unavailable` | 当前依赖与服务健康状态 |

生产环境必须满足 `data_provenance=real`；缺少真实能力时使用 `operation_mode=off/read_only` 和精确 `health/reason`，不得退回 fixture。`fixture`、`unavailable`、`demo` 均不得作为 `operation_mode` 的值。

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
| BG-02 | 生产 Web 未构建期货路由 | 所有发布路由编入生产产物，运行时只由服务端 capability 控制可用性 |
| BG-03 | research 容器启动时安装 uv 和依赖 | 建立多阶段不可变镜像，启动不联网 |
| BG-04 | 生产编排固定 fixture executor | 生产默认禁止 fixture；未配置真实能力时 fail-closed |
| BG-05 | Docker/Compose 未在干净宿主验收 | CI 和目标 Linux 主机均执行构建及启动验证 |
| BG-06 | CI 未固定运行期货 race | 将期货 service/API race 纳入必过任务 |
| BG-07 | 无完整 ready/metrics/诊断合同 | 实现服务级和依赖级可观测接口 |
| BG-08 | 无自动化备份恢复证据 | 提供脚本并在临时数据库完成恢复核验 |
| BG-09 | 缺升级/回滚演练 | 从上一发布版本升级，再回滚应用且保留新表数据 |
| BG-10 | 真实探针与发布状态混淆 | `PASS/FAIL/NOT_RUN/BLOCKED` 四态严格区分 |
| BG-11 | 浏览器 token、默认账号和默认 secret 可用于生产 | 改为 HttpOnly 同源会话、一次性 owner 引导和生产无默认值 |
| BG-12 | 状态、错误码、trace 字段在契约和前端间不一致 | 建立唯一错误注册表和四维 capability schema，跨语言生成与校验 |
| BG-13 | 数据库、worker SQLite 与文档卷无法证明同一时点一致 | 建立写入屏障、worker quiesce ACK、backup epoch、加密异机副本和恢复演练 |
| BG-14 | `live` 只靠脚本和人工约定 | 服务端持久化 probe/release gate，事务校验配置摘要后才能切换 |

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
| server | 会话鉴权、领域状态、数据库、预算、证据、发布门禁、worker 调度 | 不在请求内运行任意模型代码，不返回 secret |
| research-service | 股票观点受限研究执行 | 不直写业务 DB，不持有用户会话凭据 |
| futures-service | 期货受限研究、材料解析和来源 adapter | 不访问股票内部状态，不发布最终报告 |
| postgres | 持久化业务事实、版本、审计和任务状态 | 不对公网开放 |

### 4.3 文件责任

必须按现有结构实施，避免新建通用平台层：

```text
app/deploy/
  compose.prod.yaml              生产拓扑唯一入口
  compose.test.yaml              fixture 集成测试覆盖层，禁止部署到生产
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

## 5. 配置、Secret、会话与运行模式

### 5.1 配置来源

生产只接受环境变量和只读 secret 文件，敏感值优先通过 `*_FILE` 或 secret reference 读取。Docker Compose 使用 `secrets`；本地开发可使用被 `.gitignore` 排除的 `.env`。任何 API key、会话签名/加密密钥、内部 token、数据库密码和引导凭据不得进入：

- Git、镜像层、前端构建变量；
- 日志、错误响应、manifest、报告或导出；
- Compose 默认值或 example 文件示例值；
- 浏览器 `localStorage`、`sessionStorage`、IndexedDB、URL、可被前端 JavaScript 读取的 Cookie 或其他持久化客户端存储。

本地测试环境的弱 secret、fixture executor 和演示账号只有在 `ZHIGU_ENV=test` 且显式设置 `ZHIGU_ALLOW_INSECURE_DEV=1` 时允许，并且服务只能绑定回环地址。生产镜像、生产 Compose 和 CI 的生产 smoke 必须拒绝该开关。

### 5.2 必需生产变量

| 类别 | 变量 | 规则 |
|---|---|---|
| 基础 | `ZHIGU_ENV=production` | 生产 Compose 必填且无默认值；缺失或其他值直接退出 |
| URL | `ZHIGU_PUBLIC_ORIGIN` | 必须为 HTTPS、无 path、禁止环回/私网域名 |
| DB | `POSTGRES_USER/DB`、`POSTGRES_PASSWORD_FILE` | 密码无默认值；DSN 由 Compose 内部生成 |
| 身份 | `ZHIGU_SESSION_KEYS_FILE` | 当前和上一把至少 256-bit 随机密钥；支持轮换，禁止示例值 |
| 引导 | `ZHIGU_BOOTSTRAP_OWNER_FILE` | 仅首次初始化读取；不存在默认用户或默认密码，完成后必须移除/轮换 |
| 内部 | `ZHIGU_INTERNAL_TOKEN_FILE` | Go↔research 独立 token |
| 期货 | `FUTURES_SERVICE_TOKEN_FILE` | Go↔futures 独立 token，不得等于 internal token |
| 期货模式 | `ZHIGU_FUTURES_ENABLED/MODE` | 首次部署默认 `true/read_only` 或 `false/off` |
| 事件 | `ZHIGU_INTEL_ENABLED/MODE` | 未完成真实验证时使用 `read_only` 或 `off`；`demo` 仅用于环境字段 |
| 数据 | provider URL、secret ref | 缺失时 provider disabled，不回退 fixture |
| 模型 | model URL/name/version/price/key | 缺一项则该模型 disabled |

### 5.3 浏览器会话与 owner 引导

- 浏览器会话使用同源 `__Host-zhigu_session` Cookie，必须设置 `Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/` 且不得设置 `Domain`；浏览器端不得读取或持久化 bearer token；
- 登录响应不得把 session/JWT 放入 JSON。已认证客户端通过 `GET /api/v1/session/csrf` 获取与会话绑定、可轮换的 CSRF token，前端只保存在内存并在写请求的 `X-CSRF-Token` 中提交；
- 状态修改请求同时校验精确 `Origin`/`Referer` 和 CSRF token；缺失或不匹配返回稳定错误码；
- 若会话载荷使用 JWT，必须固定允许算法并校验 `iss`、`aud`、`iat`、`exp`、`jti`；不得根据 token header 动态选择算法；密钥轮换仅接受当前和上一把密钥；
- 会话必须有空闲和绝对过期时间，登出、密码修改和 owner 禁用能在服务端撤销现有会话；
- 新 owner 密码使用 Argon2id（最低 `m=19 MiB,t=2,p=1`）；迁移期可接受现有 cost ≥ 12 的 bcrypt，并处理其 72-byte 输入上限。参数需在目标主机基准测试且单次验证不超过 500 ms；登录成功时发现旧算法/参数应渐进 rehash。比较与错误文案不得泄露账号是否存在；
- 登录按“来源 IP + 账号”执行有界速率限制和递增退避；审计记录结果、trace ID 和匿名主体，不记录密码、token 或完整请求体；
- 首次 owner 只能通过宿主 CLI 或一次性只读 secret 引导；已有 owner 时重复引导必须失败。密码轮换/恢复只通过需宿主权限的 CLI 执行并撤销全部会话。仓库、迁移和生产镜像不得 seed `admin/invitee` 等默认账号；
- 非浏览器自动化若需要 bearer token，必须使用单独、可撤销、最小 scope 的 personal token，不得复用浏览器会话或内部服务 token。

### 5.4 启动校验

`server` 启动前生成不含 secret 的配置摘要，至少包含：版本 SHA、构建时间、四维运行状态、启用模块、provider 名称、模型配置版本、迁移版本和持久目录。以下情况生产启动必须失败：

- secret 为空、为仓库示例值或长度不足；
- `PUBLIC_ORIGIN` 不是 HTTPS；
- `ZHIGU_ALLOW_INSECURE_DEV=1`、任一 fixture executor、`data_provenance=fixture` 或存在默认账号/密码；
- live 模式缺费率、模型版本或必要 provider；
- 持久目录不可写或位于容器临时层；
- 数据库迁移 checksum 冲突；
- 两个内部 token 相同，或任一内部 token 与会话密钥相同；
- owner 已存在但仍挂载 bootstrap 凭据，或尚无 owner 且未提供合法的一次性引导方式。

模块 `off/read_only` 且真实凭据缺失时宿主可以存活；`/readyz` 和 capability API 必须返回每个模块的 `deployment_environment/operation_mode/data_provenance/health/reason`。生产中缺真实能力不得改写为 demo 或 fixture。

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
- PostgreSQL、两个 worker 私有数据和期货文档共四个命名卷；
- 服务级 healthcheck；
- `depends_on.condition=service_healthy` 仅用于启动排序，应用仍实现重连；
- `restart: unless-stopped`；
- 可由 `docker compose config` 验证的 CPU、内存、PID 和日志轮转上限；
- secret 只读挂载；
- 镜像 tag 使用 release version 或 commit SHA，禁止生产 `latest`；
- 事件和期货路由都编入生产 Web 产物；构建开关不得作为生产运行时门禁，实际可用性由 capability API 决定。

`compose.prod.yaml` 绝不挂载 fixture、演示账号或弱 secret。`compose.test.yaml` 只能作为独立 project/网络/卷的测试覆盖层启用 fixture，CI 必须同时验证：生产 Compose 在无真实凭据时 fail-closed，测试 Compose 才能跑 fixture 全链路。二者不得共享卷、secret 名或发布命令。

### 6.3 Web 代理和 TLS

Nginx 必须：

- 443 提供 TLS，80 仅 301 到 HTTPS；
- `/api/` 代理 Go，保留 `Host`、`X-Forwarded-*` 和合法 `X-Trace-ID`；
- API 响应禁止 SPA fallback；
- SPA 深链接 fallback 到 `index.html`；
- `index.html` 使用 `no-cache`，带 hash 静态资源使用一年 immutable cache；
- 设置 CSP、HSTS、`X-Content-Type-Options`、`Referrer-Policy` 和 frame 限制；
- 上传体积上限与应用 20 MiB 限制一致并留协议开销；
- API/导出/事件流配置适当超时，不能使用一个无限全局 timeout。

Nginx 不得把 Docker `unhealthy` 误当成持续运行期的自动摘流机制。Go 在未 ready 时对业务 API 返回 503 和稳定错误码；外部 watchdog/systemd 负责进程级恢复。`/metrics`、内部诊断和 worker 健康接口不得暴露到公网。

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

应用发布必须声明数据库兼容窗口：当前应用版本 N 与数据库 schema N、N-1 双向兼容。迁移采用 expand → backfill → switch → contract；删除列、收紧类型等 contract 操作不得与首次应用升级同批执行。`rollback.sh` 只回滚到声明兼容的应用版本，不能以回滚二进制假定数据库自动降级。

### 7.2 持久目录

| 数据 | 生产位置 | 备份 |
|---|---|---|
| PostgreSQL | `pgdata` volume | 必须 |
| research SQLite/task copy | `researchdata` volume | 必须，与 DB 同批次 |
| futures worker 状态 | `futuresdata` volume | 必须，与 DB 同批次 |
| 期货上传文档 | `futuresdocuments` volume | 必须，遵循删除/保留规则 |
| 临时提取文件 | 容器 `/tmp` | 不备份，任务终止清理 |

### 7.3 备份与恢复

`backup.sh` 必须通过数据库中的全局备份锁和 `backup_epoch` 建立跨存储写入屏障：

1. 原子提交两个模块为 `read_only`、撤销新 grant 并停止领取任务；提交后不得再开始新的写入或付费调用；
2. 等待 Go 活动写事务归零，并要求两个 worker 返回包含 epoch、最后序列号和持久化完成状态的 quiesce ACK；超时即失败；
3. 对 PostgreSQL 执行一致性 `pg_dump --format=custom`；worker SQLite 使用在线备份 API/`.backup` 并完成 WAL checkpoint，禁止直接复制运行中的数据库文件；
4. 在写入屏障仍生效时归档 worker 文件和文档卷，生成 SHA-256；
5. 生成 manifest：backup ID/epoch、版本、开始/截止时间、DB 迁移版本、worker 序列号、配置摘要 hash、文件 hash 和对象数量；
6. 成功或失败均释放屏障。原模式为 `live` 时只恢复到 `read_only`，重新通过 readiness/release gate 后由所有者显式恢复 `live`。

`restore-verify.sh` 必须在新的临时数据库与临时卷中恢复，验证迁移版本、核心表计数、最近一条 run/hypothesis、tombstone 不复活、文件 hash 和健康检查。只有“备份文件生成”不算恢复通过。

备份最低目标：RPO ≤ 24 小时、RTO ≤ 2 小时；保留最近 7 个日备份和 4 个周备份；备份使用独立密钥加密，至少一份复制到宿主故障域之外。恢复演练至少每个候选 release 一次，manifest 记录实际恢复耗时。明文备份、只存在同一宿主的备份或未验证恢复均不算通过。

### 7.4 保留与删除

延续既有 24 小时私有内容清理、审计保留和 tombstone 规则。生产环境必须运行清理 worker，指标显示待清理数量、最老任务年龄和最近成功时间。清理失败只重试幂等步骤，不恢复已删除正文。

## 8. Worker、调度与故障语义

### 8.1 Worker API

两个 worker 均提供：

- `GET /healthz`：进程存活，不检查付费上游；
- `GET /readyz`：配置、持久目录、Go 内部入口可用；
- 受 token 保护的执行、取消、状态、ACK/Purge 接口；
- 请求体严格 schema，拒绝未知字段和额外 JSON；
- trace ID、业务幂等键、run/task/generation 结构化日志；
- context deadline 和客户端断开向下游传播。

### 8.2 调度不变量

- 领取任务使用数据库锁和 generation，不依赖进程内唯一状态；
- 宿主重启后 queued 可继续，running/verifying 按既有规则失败或恢复，不能重复付费；
- canceling 在 ACK 或租约过期前占用执行槽；
- `off/read_only` 的事务提交必须立即拒绝新 grant 和新付费调用；60 秒只是已在执行调用的取消/租约收敛上限；
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

所有 API 延续 `{data,error,trace_id}` envelope。`trace_id` 是跨 Nginx、Go、worker 和上游调用的唯一关联字段；`request_id` 只用于业务请求/幂等键，不得与 trace ID 混用。生产必须统一以下错误属性：

```json
{
  "data": null,
  "error": {
    "code": "FUTURES_SOURCE_UNAVAILABLE",
    "message": "当前数据来源暂不可用，请稍后重新检查",
    "retryable": true,
    "retry_after_seconds": 30
  },
  "trace_id": "01J..."
}
```

- `code` 稳定、可供前端映射；
- `message` 是安全的用户文本，不含 URL、SQL、路径、secret 或上游原文；
- `retryable` 是所有错误对象必填 boolean，只表示重复同一只读请求可能成功，不授权自动重做付费操作；
- `retry_after_seconds` 可选；存在时必须与响应 `Retry-After` 一致，429 必须设置，临时 503 应设置；
- 非本人对象和不存在对象统一 404；
- 内部错误记录 cause chain，外部只返回稳定码；
- 每个响应设置 `X-Trace-ID`，且与 envelope `trace_id` 相同；入口只接受合法 UUID/ULID 形式的外部 `X-Trace-ID` 作为 trace 候选，否则生成新值；
- 错误 code 的唯一来源为版本化 `contracts/error-codes.json`（或等价生成源）。例如前端必须映射完整 `FUTURES_SOURCE_UNAVAILABLE`，不得自行剥离前缀为另一个 code；
- 任何页面操作依赖的 code 必须在注册表中声明 HTTP 状态、默认 `retryable`、安全文案 key 和允许出现的接口。

新增或改变字段必须先更新机器契约、负例和前端映射，不能只改某一层。

所有列表接口必须实现稳定排序、`limit/cursor` 和可继续使用的 `next_cursor`；只有结果确实结束时才返回 `null`。cursor 必须不透明、版本化并绑定 owner、筛选条件和排序，篡改/过期返回注册错误码，不得退回第一页造成重复。

通知接口必须支持 `read`、`action_required` 筛选，并提供 `POST /api/v1/futures/notifications/batch-read`：最多 100 个 IDs，同一 owner 内原子更新，重复调用幂等；任一 ID 非法/越权则整批不写入并按 404 语义返回。来源管理必须提供 `POST /api/v1/admin/futures/sources/{source_id}/probes`（202 + `probe_id`）和 `GET /api/v1/admin/futures/sources/{source_id}/probes/{probe_id}`；来源列表至少返回 `last_success_at/data_as_of/last_error_code/last_probe_id`。这些接口及负例必须先进入 OpenAPI/JSON Schema，再允许前端接线。

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

每次探针必须持久化不可变 `probe_run`：`id/kind/provider_or_model/config_digest/adapter_version/deployment_environment/sample_scope/status/evidence_hash/started_at/finished_at/expires_at`。`config_digest` 覆盖 endpoint、权限、模型/adapter/费率版本及 secret revision 或 keyed fingerprint，但绝不包含/暴露 secret 原文。原始证据受权限和保留规则保护，状态不得由客户端直接写入。

服务端同时持久化 `release_gate`：模块、目标 `operation_mode`、当前配置摘要、所需 probe IDs、gate 状态、批准 owner、批准时间和失效时间。gate 的失效时间不得晚于最早探针失效时间，且最长 24 小时。provider/model 配置、adapter 版本、费率、secret 内容/revision 或 reference 变更后，关联 gate 立即失效并回到 `read_only`。

`GET /api/v1/admin/futures/operations` 必须返回当前四维状态、配置摘要和可用/失效的 `release_gate`；`PATCH /api/v1/admin/futures/operations` 的 live 请求必须携带 `expected_version/config_digest/release_gate_id/reason`。probe 完成后由服务端生成或更新 gate，客户端无权创建 PASS gate。股票研究/事件模块使用等价合同，不得另设更宽松旁路。

### 10.2 部署后的接入步骤

1. 模块保持 `read_only`；
2. 通过 secret 文件注入 provider/model 凭据；
3. 重载服务并核对配置摘要，不输出 secret；
4. 运行连接、权限、覆盖、分页、发布时间和费率探针；
5. 对固定小样本执行真实数据读取；
6. 对固定少量研究执行真实模型调用和发布校验；
7. 核对预算、日志脱敏、数据来源、报告引用和删除；
8. 探针通过后由所有者请求切换 `live`；服务端在同一事务内锁定模块配置并校验：生产环境、真实 provenance、必需探针均为未过期 PASS、摘要/adapter 版本完全匹配、预算和 readiness 正常；任一条件变化即拒绝切换。

本项目不要求灰度用户或运营签收。真实探针失败时保持 `read_only`，修复配置或 adapter 后重跑；不得修改数据库状态伪造 PASS。

## 11. 安全与隐私

### 11.1 网络

- 仅 Web 暴露公网；
- worker 和 DB 只监听容器私网；
- 出站只允许声明的 HTTPS provider/model 域名；生产通过宿主防火墙或受控 forward proxy 强制执行，Compose 私网或 DNS allowlist 本身不算隔离；
- 用户 URL 抓取继续执行 SSRF 防护、每次连接和重定向后的 DNS/IP 二次校验、私网/保留地址拒绝和响应上限；
- Nginx 与 Go 均设置请求体、header、连接和速率限制。

### 11.2 身份与授权

- 个人项目仍保留服务端会话和 owner scope；不能用“只有我使用”删除后端授权；
- admin API 后端强制 admin role；
- 内部 token 与用户会话/personal token 分离；
- futures grant 继续绑定 owner/mode/run/task/generation/tool/args hash/nonce/expiry；
- 浏览器会话、CSRF、撤销、密钥轮换和 owner 引导严格遵守 §5.3；生产不得 seed 默认账号或接受仓库内默认密码。

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
| `/healthz` | Go 进程事件循环可响应 | Docker 标记 unhealthy；watchdog 决定重启 |
| `/readyz` | DB 可查询、迁移一致、必需 worker ready | 业务 API 返回 503；模块 fail-closed |
| worker `/healthz` | 进程可响应 | Docker 标记 unhealthy；watchdog 决定重启 |
| worker `/readyz` | token、持久目录、Go internal 可用 | 相关模块不可创建新任务 |

Docker healthcheck 只改变容器健康标记，`restart: unless-stopped` 不会因 unhealthy 自动重启。生产部署必须配置独立 watchdog/systemd health watcher，按有界退避重启连续失败的进程并告警，避免无限重启风暴。

`/readyz` 返回模块数组，包含 `enabled/deployment_environment/operation_mode/data_provenance/health/reason/version/config_digest`；不因一个可选 provider 失败把整个宿主伪装成宕机，但对应模块必须 fail-closed。Go 对未 ready 的写/付费业务请求返回 503；不得依赖单节点 Nginx 自动感知 Docker 健康并“摘流”。

### 12.2 日志

所有服务输出 JSON 到 stdout/stderr，至少包含：

- timestamp、level、service、version；
- trace_id、业务 request_id/idempotency_key（适用时）；
- owner 的不可逆匿名标识；
- run_id/task_id/generation/stage（适用时）；
- error_code、duration_ms、upstream、attempt；
- 不含 secret 和私有正文。

### 12.3 指标

至少提供：HTTP 请求数/延迟/状态、DB pool、任务队列长度和最老年龄、各状态 run 数、worker 调用延迟、provider/模型调用与错误、预算 reservation/spent/unknown、清理积压、备份时间、迁移版本。个人部署可先使用仅私网可访问并要求内部鉴权的 Prometheus 文本端点和结构化日志，不要求单独引入 Grafana 服务。

### 12.4 告警

个人项目的最小告警由外部 uptime 检查加宿主维护任务完成，覆盖：站点不可达、ready 失败、磁盘低于 15%、备份超过 26 小时未成功、队列最老任务超阈值、清理失败、预算异常和证书 14 天内过期。仅有指标而无人执行检查或通知不算完成。

### 12.5 运维执行器

生产必须选择并文档化一个实际执行器（systemd service/timer、受限 cron 或独立 maintenance 容器），至少负责：每日加密备份与异机复制、私有内容/临时文件清理、证书续期后的 Nginx reload、health watchdog、磁盘/队列/备份/证书检查和告警投递。每项任务均有互斥锁、超时、非零退出码、结构化日志、最近成功时间和失败告警；不得只提供需要人工运行的脚本。

## 13. 性能与资源预算

上线前使用 fixture/本地 mock 执行可重复负载，不依赖真实供应商。工具及版本固定在仓库，目标为 2 vCPU/4 GiB Linux；每个场景先预热 2 分钟，再独立运行 5 次，以下阈值要求 5 次中的最差 P95 和累计错误率均通过。测试报告记录 commit、镜像 digest、CPU/内存限制、数据规模和命令；禁止以未记录的开发机结果替代：

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
| Web | production build、Node tests、完整 Playwright fixture 流程；浏览器会话不落入 Web Storage |
| 集成 | Web→Go→PostgreSQL→两个 worker；禁止全层 mock |
| 安全 | secret、依赖、SSRF、IDOR、上传、导出、日志脱敏 |
| 生产 | Docker build、生产 Compose fail-closed smoke、独立测试 Compose fixture 集成、空库启动、升级、restart/watchdog、backup/restore、rollback |
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
3. `production-smoke`：仅用 `compose.prod.yaml` 空卷和随机一次性 owner bootstrap 启动，验证无 fixture/default account/default secret 且缺真实 provider/model 凭据时 fail-closed；
4. `fixture-integration`：使用隔离的 `compose.test.yaml` 跑完整 HTTP/浏览器链路后销毁；
5. `upgrade`：从上一 release fixture DB 升级并验证 N/N-1 兼容；
6. `backup-restore`：生成加密备份并恢复到新卷，核对 backup epoch；
7. `security`：依赖、secret、会话、CSRF 和生产默认值扫描；
8. `release-manifest`：汇总版本、镜像 digest、测试、skip 和 artifact hash。

### 15.2 发布产物

每次候选发布生成 `artifacts/release/<version>/manifest.json`，至少包含：

- Git SHA、dirty 状态、构建时间；
- 四个镜像 digest；
- 配置 schema 版本、迁移版本；
- 各测试命令、退出码、通过/失败/skip 数；
- R0–R4 状态；
- 真实探针 `NOT_RUN/BLOCKED/PASS/FAIL`；
- 四维 capability、配置摘要、probe IDs 和 release gate 状态；
- 备份恢复和回滚证据路径；
- 已知限制。

只有 R0–R2 全部 PASS 才可标记“可部署”。只有 R3 PASS 才可切换真实链路；fixture 结果不得改变 R3。

## 16. 开发阶段

### M1：生产配置与 fail-closed

- 建立统一环境校验和脱敏配置摘要；
- 移除默认账号/secret，交付 owner 引导、HttpOnly 会话、CSRF、撤销与登录限流；
- 生产禁止 fixture；
- 补齐四维 capability、futures/research ready 状态与唯一错误码注册表；
- 契约、单测和前端状态字段同步。

### M2：不可变镜像与完整 Compose

- 为两个 Python worker 建 Dockerfile；
- 修复 server/web 镜像；
- 增加 futures 服务、卷、secret、网络、限制和 health；
- 在本机/CI 完成无网络启动验证。

### M3：迁移、持久化与恢复

- 空库/升级迁移；
- 写入屏障、backup epoch、加密异机备份、恢复核验和清理积压指标；
- expand/contract 迁移与 N/N-1 应用回滚。

### M4：可观测性与安全

- ready/metrics/JSON 日志、watchdog 和维护任务执行器；
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
- R3 通过且配置摘要与 release gate 匹配后，由所有者请求服务端事务切换 `live`。

阶段必须按顺序通过；不得用 M6 的一次真实成功替代 M1–M5 的工程门禁。

## 17. 可执行验收项

### 17.1 组件与部署

| ID | 验收 |
|---|---|
| BPR-01 | 生产 Compose 明确包含五个服务，只有 Web 暴露公网 |
| BPR-02 | 所有应用镜像启动不联网、不 bind mount 源码、以非 root 运行；生产环境值缺失即退出 |
| BPR-03 | 事件/期货路由编入生产 Web，server mode、worker URL/token 和四个存储卷完整接线；运行时只认 capability |
| BPR-04 | 空 Linux 主机仅安装 Docker/Compose、配置 secret 后可部署 |
| BPR-05 | HTTPS、SPA 深链和 `/api/*` 路由正确，API 错误不回退 HTML |
| BPR-06 | `docker compose restart` 后数据、任务和模式正确恢复 |
| BPR-07 | `deployment_environment/operation_mode/data_provenance/health` 四维独立；缺真实凭据时 off/read_only 且绝不产 fixture live 数据 |
| BPR-08 | 生产配置错误、弱 secret、fixture、默认账号/密码和 insecure dev 开关在启动前失败并给出不泄密原因 |

### 17.2 数据与状态

| ID | 验收 |
|---|---|
| BPR-09 | 空库和上一 release 数据库均迁移成功 |
| BPR-10 | migration checksum 冲突阻止相关模块 ready |
| BPR-11 | 写入屏障与 worker ACK 建立同一 backup epoch；加密异机备份可在 RTO 内恢复到新卷，核心计数/hash 一致 |
| BPR-12 | 删除后的正文和文件不因恢复而复活 |
| BPR-13 | worker 退出、宿主重启和迟到结果均按状态机收敛 |
| BPR-14 | canceling、lease、generation 和预算 race 测试通过 |
| BPR-15 | off/read_only 事务提交后立即拒绝新 grant/付费调用，已有调用 60 秒内收敛 |
| BPR-16 | 清理任务可观测且重复执行幂等 |

### 17.3 安全与可观测

| ID | 验收 |
|---|---|
| BPR-17 | 外部只可访问 80/443，DB/Go/worker 端口不可公网连接 |
| BPR-18 | secret/默认值/日志/镜像历史/Web Storage 扫描无真实凭据、默认用户和浏览器 token |
| BPR-19 | owner 引导、HttpOnly 会话、CSRF、撤销、限流及 owner/admin/internal grant 权限负例全部通过 |
| BPR-20 | SSRF、路径穿越、zip bomb、XSS 导出负例通过 |
| BPR-21 | `X-Trace-ID` 与 envelope `trace_id` 一致并贯通 Nginx→Go→worker；非法外部 ID 被替换 |
| BPR-22 | ready 返回四维状态并区分宿主、模块、provider 和模型；未 ready 业务请求返回 503 |
| BPR-23 | watchdog 和维护执行器实际运行；队列、预算、清理、备份、证书和磁盘均有指标与失败告警 |
| BPR-24 | 用户错误响应无 SQL、路径、上游正文和 secret |

### 17.4 测试与发布

| ID | 验收 |
|---|---|
| BPR-25 | Go 全量和规定 race 命令退出 0 |
| BPR-26 | 两个 Python worker 全量非 live 测试退出 0 |
| BPR-27 | 唯一错误注册表、retry/trace、分页筛选、通知批量已读、来源 probe 的契约正反例和跨语言测试退出 0 |
| BPR-28 | Web build、隔离测试 Compose 的浏览器 fixture/真实 HTTP 集成 E2E 退出 0 |
| BPR-29 | Docker build、生产 fail-closed smoke、隔离 fixture 集成、升级、watchdog、恢复、回滚退出 0 |
| BPR-30 | §13 在固定资源、工具版本、数据规模和 5 次独立运行下通过，且无状态/预算错误 |
| BPR-31 | 所有 skip 仅为真实探针且标为 NOT_RUN |
| BPR-32 | release manifest 可追溯到 commit、镜像 digest 和测试输出 |

### 17.5 上线后真实接入

| ID | 验收 |
|---|---|
| BPR-33 | 无需改代码即可注入 provider/model secret |
| BPR-34 | provider `probe_run` 验证权限、分页、覆盖、时间、单位和来源版本，证据 hash 可追溯 |
| BPR-35 | 模型 `probe_run` 验证协议、费率、usage、schema 和预算，配置摘要/adapter 版本与 gate 一致 |
| BPR-36 | 固定小样本真实研究报告通过引用与数字发布校验 |
| BPR-37 | 探针失败保持 read_only；配置、adapter、费率或 secret ref 变化立即使 gate 失效，前端展示真实原因 |
| BPR-38 | 删除真实样本后主存、worker 副本和文件按规则清理 |
| BPR-39 | R3 manifest 不包含凭据、私有原文或未授权数据 |
| BPR-40 | 只有服务端事务校验未过期 PASS、配置摘要、真实 provenance、预算和 readiness 后，所有者显式切换 live 才允许正式研究 |

## 18. 完成定义

后端生产化只有同时满足以下条件才完成：

1. BPR-01–BPR-32 全部 PASS；
2. 在干净 Linux 主机完成一次部署、重启、备份恢复和应用回滚；
3. 生产镜像无源码 bind mount、无启动下载、无默认账号/secret、无 fixture 回退，浏览器会话不进入 Web Storage；
4. 前端能基于四维 capabilities、ready 和唯一错误注册表工作，所有页面操作均有真实服务端 API；
5. release manifest 将真实探针准确标为 NOT_RUN、BLOCKED、PASS 或 FAIL，并记录匹配的配置摘要与 gate；
6. 备份达到 RPO/RTO、加密和异机要求，watchdog/清理/证书/告警由实际执行器调度；
7. 文档中的命令由未参与开发的人按步骤可以复现；
8. 仓库没有未解释测试 skip、临时调试路由或“成功”占位实现。

部署上线后，BPR-33–BPR-40 是真实接入完成定义。它们不阻止 R2“可部署”结论，但未通过前不得把真实能力标为可用。

## 19. 规范参考

- [OWASP HTML5 Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html)：浏览器敏感凭据存储边界；
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)：Argon2id、bcrypt 和渐进 rehash 基线；
- [Docker restart policy](https://docs.docker.com/engine/containers/start-containers-automatically/)：容器重启策略语义；
- [Docker Compose startup order](https://docs.docker.com/compose/how-tos/startup-order/)：`depends_on` 与 healthcheck 的启动依赖边界。
