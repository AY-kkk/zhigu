# 知股一期 · 阶段 B 开发 SPEC

版本：B-1.7 · 2026-09-23（统一纳入策略市场、数据库与前端代码合同；研究协议与公开 report Schema 保持不变）
状态：**已写入拍板决定的开发契约；不是完成报告。** 应用实现、真实调用与样机效果均不因本文被认定通过。研究 G0～G5 与策略 BS0～BS4 分别记录状态；本次文档更新不提升任何实现或签收状态。

研究线目标：普通用户粘贴一个 **A 股或港股** 单公司投资观点，经确认后，通过真实模型、受控公开数据与两个独立研究 Agent，获得一份**观点裁判型研报**：裁决对象是用户确认的主张，交付物是带证据、反证、未知原因与四态判断的已发布报告。知股不生产平行的投资评级、目标价或卖方深度。

策略线目标：用户在「交易策略」二级策略市场浏览有来源的完整规则，复制为私有草稿，用 AI 和可视化窗口制作策略，再运行并查看历史回测。优先完成市场浏览，再打通制作与回测。

继承关系：B-1.7 替代 B-1.6，新增 §12 的数据库、接口、前端代码与验收合同。研究线继续全 A 股+港股目录和年度三表，保留三工具、双协议、预算与报告质量线。策略日 K、回测和市场统一纳入阶段 B 的策略线，不接入研究 Agent 工具或 report Schema。

文档优先级：本文 §2 管研究线、§12 管策略线；共同权限与基础设施遵循本文。配套文档：

- [验收与接入清单](验收与接入清单.md)：研究 B-ID 与策略 B-S-ID 的测试和证据。
- [给 Cursor 的开发指令](CURSOR开发指令.md)：按当前工作线推进。
- [交易策略 PRD](../prd/策略模块_PRD.md)：用户流程与产品验收。
- [策略模块计算与交互附录](策略模块_开发SPEC.md)：行情、指标、撮合细则；作为本文的策略专项附录。其旧有「不属于阶段 B」及「只改本文件」表述失效；数据库、路由、版本迁移和开发关口有冲突时以本文 §12 为准。

产品/架构仍继承 [PRD v2](../prd/金融C端Agent_MVP_PRD.md)、[面客架构 v2](金融C端Agent_面客产品架构_v2.md)、[总 SPEC](开发交付_SPEC.md)。旧文档中的 GoSaaS 迁移、完整 records 登记、operation 短名、确认时改用新模型等冲突条款，在阶段 B 按本文执行；交付时记录差异。

研究需求 ID 为 B-01～B-35，策略需求 ID 为 B-S-01～B-S-10，验收附件逐项对应。开发者只能把关口标到 `ready_for_review`，`accepted` 由用户签收。

---

## 1. 范围（B-01）

### 1.1 必须交付 / 明确不做

本表为研究线范围，策略线新增范围见 §12；研究线的排除项不用于否定策略线已明确要求。

| 必须交付 | 明确不做 |
|---|---|
| 粘贴观点→解析→确认股票/期限/主张→双路研究→**观点裁判型研报**→证据抽屉 | 自选股持续追踪、用户画像、长期偏好记忆、主动通知、卖方深度/评级/目标价 |
| **独立宿主** `app/server` 为唯一运行入口；Eino DAG；DeerFlow 真实 Harness；Chat Completions **与** Responses 两个 adapter | 迁入 GoSaaS 进程、第二套用户系统、第二套 Supervisor、DeerFlow 通用聊天前端 |
| 至少一套已验证模型配置（用途可共用）；财务三表关键科目 + 公告检索两条数据能力 | 多厂商自动容灾、任意网页浏览、任意 MCP、Computer Use、实盘交易、收益承诺 |
| 角色提示词写成可执行步骤；合成提示词含三条质量检查；未知项过发布正则 | 任意 MCP、Computer Use、子 Agent、Wind/iFinD/S&P、Excel/PPT/HTML 导出、Hosted Agents 替换本架构 |
| 历史、取消、删除、报告内追问；后台保存→测试→启用 | 在研究报告内执行回测、荐股榜、全市场选股扫描、扫描件 OCR、公网开放、业绩点评/持仓早报入口 |
| 私有环境可复现的技术样机 | 合规结论、大规模吞吐、「零幻觉」、阶段 C 运营加固 |

分关口纵向闭环：研究 G0～G5 与策略 BS0～BS4 各有独立负例和证据，两线必需项全部 `accepted` 才完成 B。不在最后集中联调，不先建通用 Agent 平台。

**时间盒：** 不承诺日历天数内做完 B。每个工作日结束时必须停在明确的 `in_progress` / `blocked` / `ready_for_review`。禁止为赶时间删除断言、降低阈值、用 fixture 顶 live，或把本关失败标到阶段 C。

### 1.2 可核查用户案例

用户输入：「某公司利润改善，未来一年经营前景值得看好。」系统要求从覆盖目录（全 A 股或港股）选择真实证券，确认绝对起止日期，编辑 1–6 个主张。

支持者查利润变化及解释；挑战者查现金流、一次性损益和经营风险，不读取支持者论证。两者均可得到「不足」，不能编造相反观点。报告区分已披露事实、基于事实的推断、待验证假设；不把利润增长写成股价必涨。

离线数值样本（禁止出现在 live 源）：上年归母净利润 100、本年 120，增长率必须为 **20.00%**（单位 `percent`）。

### 1.3 相对总 SPEC 仍有效的收紧（已拍板）

保留产品边界、Eino 唯一总编排、DeerFlow 执行两个隔离 Loop、硬预算数字。B 新增并必须落地：

1. 控制协议 header `X-Zhigu-Contract-Version: 2`（与 JSON Schema 1.0 字段并存）。
2. 证据改为 grant 下的 `record_id` 句柄，Python 不得提交完整可信 records。
3. 私有 `finance_report_checks` 与 `finance_provider_records`。
4. 样机质量线：3 只冻结样本证券、30 例质量集、20 次 live 运行（协议计数见 §2.3）；覆盖目录另接全 A+港股，不把质量集扩成全市场。
5. 观点裁判型研报护栏见 §2.7：步骤化提示词、未知项正则、合成提示词三条、口径只进 Prompt 前缀。不改公开/内部 Schema 字段，不扩 `finance_report_checks`。

---

## 2. 研究线已拍板决定（继承 B-1.6）

以下条款开发中不得再猜。要改必须升 SPEC 版本并改对应测试。

### 2.1 运行底座（D-01 / G0）

| 项 | 决定 |
|---|---|
| 唯一启动进程 | PostgreSQL 16 + `app/server`（Go）+ `app/research-service`（研究执行器）+ `app/web`；可选 `app/data-connector`（仅 Go 可访问） |
| 账号表 | 继续使用现有 `finance_users` 与演示账号 `invitee` / `admin` |
| GoSaaS | **本阶段不迁移、不作为启动依赖。** 不引入 Casbin / MySQL / Redis。私有快照不得拷进可分发仓库 |
| G0 的 A 复核 | 以 [app/IMPLEMENTATION_STATUS.md](../app/IMPLEMENTATION_STATUS.md) + 重跑阶段 A 回归为准。不依赖仓库中不存在的 `reviews/阶段A交付审查_2026-09-17.md` |
| A 产品书面签收 | 可与 G1 并行；**不挡** 数据样本与 adapter 编码。挡的是把 B 标为 accepted |

### 2.2 数据源（D-03 / D-04 / G1 / G3）

| 项 | 决定 |
|---|---|
| 积分/Tushare | **不是开工条件。** 有 token 与足够积分时，只能作为交叉核验，不得替换主路径 |
| 主路径 | 东方财富 HSF10 / HKF10 公开财务 HTTP + 巨潮目录与公告检索 HTTP |
| AKShare 的位置 | **接这条数据路径，不让研究 Agent 自己 `import akshare`。** 取数只发生在 Go 校验 grant 之后 |
| 推荐实现 | 优先 Go 直连 §8.2 冻结 URL；若 G1 样本字段对不上，才启用锁定版本的 `app/data-connector`（AKShare pin）。二者同一时刻只允许一条出网路径 |
| 研究服务 Python | `app/research-service/app/tools/` 只请求 Go；禁止 AKShare、任意 URL、SQL、shell |
| 正文最低合格 | 巨潮返回的标题 + 公告时间 + 原文 URL（PDF 或 HTML）+ 可定位片段。标题/摘要单独不能当事实正文 |
| PDF | 允许抽取 **数字 PDF 文本层**；禁止扫描件 OCR、禁止因此扩大 B 范围 |
| locator | `source_url` + 段落/锚点或字符偏移。不要求纸质页码 |
| 缺字段 | `DATA_UNAVAILABLE` / 证据不足，禁止模型填 0 或编造 |
| 法律边界 | 这些 HTTP 是网站公开接口，不是带 SLA 的官方开源数据许可。样机私有使用；限制清单必须写明：会变、要限流、不宣称可商用再分发 |

冻结质量样本（至少 2 个行业；live 模型 20 次与 G1.2 探针仍用这三只，不把质量集扩成全市场）：

| instrument_id | 名称 | 行业 |
|---|---|---|
| `600519.SH` | 贵州茅台 | 白酒 |
| `300750.SZ` | 宁德时代 | 制造 / 电池 |
| `000333.SZ` | 美的集团 | 家电 / 制造 |

覆盖目录（live）：巨潮 `szse_stock.json` 的 A 股（含北交所，排除 B 股）+ `hke_stock.json` 的港股。解析/确认可搜索代码与名称，`GET /api/finance/instruments?q=` 最多返回 20 条。fixture 仍仅 `DEMO:COMPANY`。

每只覆盖证券在源有年报时，必须能取已披露、同口径 **年度** 三表关键科目（缺字段记不足，禁止填 0）：

| 报表 | 科目 |
|---|---|
| 利润表 | `revenue`、`operating_profit`、`net_income`、`net_income_parent` |
| 资产负债表 | `total_assets`、`total_liabilities`、`equity_parent` |
| 现金流量表 | `operating_cash_flow_net`、`investing_cash_flow_net`、`financing_cash_flow_net`、`cash_and_equivalents` |

质量样本三只另须通过原 G1.2：两年 `revenue` / `net_income_parent` / `operating_cash_flow_net` + 至少一次巨潮命中。港股额外探针 `00700.HK`。`periods` 为期末日 `YYYY-12-31`。中报/季报默认目录外，拒绝。A 股金额 CNY 元；港股金额与币种以东财年报摘要字段为准（常见 CNY/HKD/USD）。银行保险不另做科目映射，缺字段按不足处理。

公告：A 股与港股均走巨潮 `hisAnnouncement/query`（港股 `column=hke`），正文仍是 `static.cninfo.com.cn` 数字 PDF。

### 2.3 模型协议（D-02 / G2 / G5）

| 项 | 决定 |
|---|---|
| Adapter | Chat Completions **与** Responses **都必须实现**，受控测试都必须跑 |
| Live | **可以、而且应当兼容 Responses。** 不是「live 只能 Chat」 |
| 同一 run | 创建草稿、首次解析前冻结 model_config_version/protocol；确认后 run 继承，四用途共用。切 active 不改变已有草稿；版本被撤销则 409 `MODEL_CONFIG_REVOKED`，须重新解析成新草稿 |
| 别名 | 内部 model 别名固定 `finance-research`，真实 model 由冻结配置映射 |
| 拟启用规则 | 用户在后台保存的、能力测试通过的协议才能启用。有 Responses 可用凭据（如官方 OpenAI，或 DeepSeek `https://api.deepseek.com` 且 function 工具实测通过）→ 该协议必须进入 live 样本。只有 Chat 凭据 → Responses 记「实现已测、live 未验证」，实现不得留空 |
| 20 次计数 | 只启用一路：10 输入 × 2 次 = 20，全走该协议。两路都启用：10 输入各协议各 1 次，**分路汇总**，每路 ≥9/10 流程完成，不得用一路成功掩盖另一路全失败 |
| 假兼容 | URL 含 `/responses` 不算通过。必须过 JSON、三工具往返、截断/拒绝、usage。供应商内置 web_search / MCP / file 必须被本系统关掉 |
| 费用 | 未在 D-05 写明上限前，**禁止付费 live**。管理员测试也是真费用，走独立 `purpose=admin_test`，不计入「每用户每天 10 次研究」 |

DeepSeek Responses 与本项目约束对齐处（G1 核验，不当作已测通过）：无状态、`previous_response_id` / `conversation` 不支持、`store` 为 false、`function` 可用。仍须按 §7 做能力测试。Ollama / 多数本地 vLLM 默认只有 Chat Completions，不得假设本机模型能 live 测 Responses。

### 2.4 契约对照（不得再分叉）

| 项 | 阶段 A 现状 | 阶段 B 决定 |
|---|---|---|
| `data-query` operation | `get_financials` / `search_filings` | **保持这两个名字**，与工具名一致。禁止另造 `financials` / `filings` |
| 协议枚举 | 代码曾用 `openai-chat-completions` | 新值仅 `openai_chat_completions` / `openai_responses`。旧值 `openai-chat-completions` **仅此一条** 视为 Chat Completions；其他未知值禁用待确认 |
| 证据 POST | `{grant_id, records:[...]}` | **一律** `{grant_id, record_ids:[...]}`。旧 records 写法返回 400。fixture 同步改，不双轨 |
| 控制协议 | 无 | 所有内部API（含fixture）要求 `X-Zhigu-Contract-Version: 2`，缺失/不匹配409 |
| `growth_rate` | `(v1-v0)/v0` 无 ×100 | 返回十进制字符串，单位 `percent`，例如 `"20.00"`。`previous ≤ 0` → 不足，不输出负转正百分比。G3 改阶段 A 对应测试，当作契约变更 |

### 2.5 模式、解析、前端、签收

| 项 | 决定 |
|---|---|
| fixture / live | 部署/环境决定，浏览器和模型不能选。live 缺配置 → 503 `CONFIG_NOT_READY`，禁止回落 DEMO 报告 |
| 本地与 CI | 允许 `ZHIGU_RESEARCH_EXECUTOR=fixture` |
| 历史精确研究 / PIT | B **关闭**；`as_of` 在确认创建 run 的事务中由 Go 生成并冻结。客户端传 as_of 返回400；排队、工具重试不更新它 |
| 解析 | fixture 可保留「演示公司」关键字。live **必须**模型结构化解析；失败或 0 候选 → 草稿仍在，用户从覆盖目录手选，**不默认第一只股票** |
| G5 前端 | 只验收功能链路（确认、轮询、报告、证据、追问、取消/删除）。**不含**编辑式视觉改版 |
| 语义闸门 | G4 程序校验说了算；语义模型失败不得发布，但不能用模型自述「已校验」代替程序。G5 的 90% 引用蕴含由人工标注 |
| 签收人 | 产品签收与 live 人工标注默认 **YUFAN**；开发者不能 `accepted` |

### 2.6 结论四态与重大数字

先判执行状态，再逐主张判断，最后汇总；以下规则均按顺序首个命中执行。

| 层 | 有序规则 |
|---|---|
| 执行 | 取消/删除/epoch失效→禁发布；异常/超时/验证失败→有独立通过内容则 incomplete/null，否则 failed；正常且报告通过→completed（仅有可信未知说明也可发布） |
| 单主张 | 展望/股价预测→insufficient；同一历史事实存在已核验实质矛盾且未消解→mixed；仅反驳证据成立→challenged；仅支持证据成立→supported；否则 insufficient |
| 报告 | 任一主张mixed，或同时存在supported与challenged→mixed；否则任一insufficient→insufficient；否则全部supported→supported；否则全部challenged→challenged |

判断按证据内容，不按支持者/挑战者身份投票；两边事实不矛盾时合并判断。将“利润增长，所以股价会上涨”拆为两个主张，结果为 supported + insufficient，报告为 insufficient。逐主张结果保存 `{claim_id,verdict,evidence_ids,reason}`，必须与确认的主张逐一对应；混合结论明确说明是资料冲突还是不同主张结果不同。

**必须绑定的重大数字：** 营收 / 归母净利润 / 经营现金流、金额、百分比、同比/环比、以及事实/推断句中的上述数字。  
**不必绑定：** 研究期限中的年份、主张条数、「未来一年」这类范围词。  
未绑定 → 从对外文本删除或改为未知，禁止「仅供参考」留下。

### 2.7 交付物：观点裁判型研报

研究线的面客完成物是**已发布报告**，不是聊天气泡，也不是公司深度或投资建议。读者读完必须能回答：哪些已披露事实成立、哪些推断没绑住、还缺什么证据。

报告必需块（沿用已有字段，不改 v1 `report.schema.json`）：原观点与确认范围、`claim_results`、四态 verdict、`summary`、`support`、`challenge`、`change_conditions`、`unknowns`、证据列表与 `as_of`。`summary` 不新增事实、不输出买卖指令或目标价。

**未知项发布闸门（程序，非语义）：** `unknowns` 每条必须匹配正则  
`^(无法取数|口径不一致|展望未绑定|时间闸门|正文不足)：.+`  
（全角冒号）。空串、仅「待核实」、缺前缀或前缀后无说明 → 不得发布。不新增 Schema 字段。不要求 S1/C1/U1 等展示编号。

**口径只进 Prompt，不进 Schema。** Go 在研究/合成/复核的系统提示词前固定注入下面原文（含换行），模型与 Python 都不能改；`ExecutionContext`、`ResearchTask`、公开 Schema 均不加字段：

```text
研究口径（服务端冻结）：
市场=本次确认标的所属市场（A股或港股）
记账币种=报表披露币种（A股默认CNY；港股以源字段为准）
会计准则=A股中国企业会计准则；港股国际财务报告准则或香港财务报告准则
报表=已披露年度利润表、资产负债表、现金流量表
缺字段记不足，禁止当作0。
```

**角色提示词（G2.8）：** 扩写 `app/research-service/app/prompts/supporter.md` 与 `challenger.md` 为步骤（准用三工具、如何取数、缺数如何写成未知、禁止编造/交易/把展望当事实）。验收看 Harness 能加载且最终 tools 仍为三个，**不要**用检查 Markdown 标题冒充通过。

**合成提示词（G4.2）：** 合成节点提示词含且仅含三条检查——利润质量、经营现金流、一次性损益——提醒模型对照主张，不下独立结论。程序结论仍只走 §2.6。**禁止**为此新增 `finance_report_checks` 字段或第三个研究角色。

证据抽屉展示已有 `source_kind`、`source_url`、`published_at` / `available_at`、`retrieved_at`。这是功能验收，不是视觉改版。

---

## 3. 逐步执行（B-02）

状态只允许：`not_started / in_progress / blocked / ready_for_review / accepted`。编码按下表依赖推进，签收单独记录，不要求逐关人工批准才能编码。任一关阻塞只阻断其依赖项；记录缺项、影响与恢复动作。研究线 accepted 需 G0～G5 全部签收；阶段 B 总签收还需 §12 的 BS0～BS4。

| 工作 | 开始条件 |
|---|---|
| 隔离契约/adapter编码与受控测试 | G0.1基线已记录 |
| G1真实数据样本、应用集成 | G0.2～G0.5检查通过；G0书面签收可并行 |
| G2受控集成 / G3数据实现 | 前者满足G0检查；后者另需G1.2/1.3/1.7完成；两者可并行，无模型Key不阻挡 |
| G4集成 | G2受控测试、G3数据契约通过；可先编码独立节点 |
| 任意付费模型调用 | G0检查通过，G1.4凭据、G1.5费用、G1.7允许名单ready；无需等待G5 |
| G5 live | G2～G4对应执行证据通过，拟启用模型能力测试通过，D清单ready；20次费用已确认 |

### 3.1 G0 基线（B-02、B-03、B-08 的身份部分）

**目的：** 证明当前独立宿主仍是唯一入口，阶段 A 回归可重跑。

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G0.1 | 记录 `git rev-parse HEAD`、`git status --porcelain` 摘要 | 写入证据包 `baseline.json` | 停，先让工作树可描述 |
| G0.2 | 确认启动入口为 README 三进程或 `app/deploy/compose.yaml`；healthz 可指向该 Go | 文档与命令一致；无第二套登录服务 | 列为 blocked，不猜 GoSaaS |
| G0.3 | 确认账号仍为 `finance_users`；不复制 GoSaaS 特有代码进公开树 | `scripts/check-public-tree.py` 或等价检查通过 | 移出私有底座后再继续 |
| G0.4 | 重跑阶段 A：`cd app/server && go test ./...`；`go test -race ./service/finance`；Python fixture 测试；`app/web` build | 与 IMPLEMENTATION_STATUS 同类命令退出 0 | 先修 A 回归，不进 G1 付费/真数据出网 |
| G0.5 | 列出阶段 A 已知缺口（真实模型/数据未接）并标明「B 将处理」 | 缺口清单进 `summary.md` | — |
| G0.6 | 提请 G0 `ready_for_review` | 后续开发按§3依赖表继续；书面签收并行 | 开发者不得自行 accepted |

G0 **不要求** GoSaaS 登录、Casbin、MySQL。B-03 测试改为：独立宿主 JWT 用户可走 finance API；另一用户 404；Python 无业务库凭据；公开路由不能直达内部口。

### 3.2 G1 接入（D-01～D-07）

**目的：** 冻结目录、数据路径、模型凭据范围和费用。无模型 Key 时仍可完成 **数据样本**；无 D-05 时不可付费。

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G1.1 | 按 §2.2 写入质量样本 3 只 + 全 A/港股覆盖目录与三表科目、年度期末日 | 目录可搜索；质量样本与 `00700.HK` 探针可映射（无 Key） | 不得让模型编股票 |
| G1.2 | 按§8.2对每股运行一次逻辑财务查询、巨潮检索并抽取一份正文；记录所有底层HTTP与原始hash | 两年三指标可映射，正文有locator；冻结source-contract.json与请求上限 | 不通则验证已允许的新浪/AKShare备用，改source version；不得隐式运行时切源 |
| G1.3 | 填写 D-03 能力表：host 允许名单、单位、available_at 规则（见 §8.3） | 清单进证据包，无秘密 | 缺 PIT 则保持历史模式关闭（已拍板） |
| G1.4 | 用户在 **管理后台或本地 env** 配置模型 Key（不进 Git、不进聊天） | D-02：供应商名、base_url、protocol、model 文档链接 | 无 Key → 模型 live 标 blocked；adapter 编码仍可继续 |
| G1.5 | 写入D-05：币种、每run/每天/管理员测试/20次live费用上限及计价来源 | 任何付费请求前必需；缺失返回blocked | 不得先调用后补额度 |
| G1.6 | G1.4/1.5/1.7 ready后，以admin_test测试指定未启用版本的连通性 | 保存test_id与真实出站记录；完整启用仍须G2.6/2.7 | 无凭据/费用则blocked，不影响受控编码 |
| G1.7 | D-06：内部 Go↔Python（及可选 data-connector）只绑回环/compose 内网；egress 允许名单 = 模型 host + §8.2 数据 host | 配置可复查 | 默认全放行则 blocked |
| G1.8 | D-07：确认签收人为 YUFAN，30 例与 live 阈值按本文，不在跑完后降低 | 写进 manifest | — |

G1数据与模型两条支线可并行；具体依赖以§3为准。单次连通成功不等于配置可启用。

### 3.3 G2 执行器（B-06、B-07、B-09、B-12、B-14～B-17）

**目的：** 真 Harness + 双协议 adapter，不是 import 成功。

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G2.1 | 实现 ChatCompletionsAdapter 与 ResponsesAdapter，仅编码/解码，走统一网关与账本 | P-01～P-06 受控测试通过 | 禁止 URL 对换冒充 |
| G2.2 | Go 内部入口：`POST /internal/llm/v1/chat/completions` 与 `/internal/llm/v1/responses`；协议不匹配 409，无出站 | 参数化测试 | — |
| G2.3 | Python 按 `X-Zhigu-Model-Protocol` 选任务级客户端；临时 token 不进 env/YAML | B-16 并发不串凭据 | 暂停并报告 DeerFlow 装配点，禁止自写 Loop 冒名 |
| G2.4 | 证明 **真实 invoke**：模型提出工具 → Go 结果 → 模型终止；最终交给模型的工具名恰为三个 | B-14/B-15；断开 Harness 依赖必须失败 | 公开 CI 无 Harness → 记 blocked，不得标 pass |
| G2.5 | 角色上限：第 5 次模型、第 7 次工具被硬挡 | B-17 | 禁止只靠 Prompt 停 |
| G2.6 | 后台保存→测试→启用：digest 不一致不能启用；换 protocol/Key/model/base_url 使旧测试失效 | B-06/B-07 | 200 不是通过 |
| G2.7 | G1凭据/费用/允许名单ready后，测试该协议真实工具往返（admin_test，受控工具样本） | P-07该协议live能力；未启用版本可测试 | 无凭据则live未验证；不阻断受控实现 |
| G2.8 | 扩写 supporter/challenger 提示词为步骤；Go 把 §2.7 口径原文注入系统提示词前缀 | Harness 加载这两份文件；最终 tools 仍恰为三个；Python `ResearchTask` 字段相对 v1 不增加；注入 MCP/file/shell 仍失败 | 禁止扩 Schema、加工具或子 Agent |

### 3.4 G3 证据（B-10、B-11、B-13、B-18～B-21）

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G3.1 | 实现 Go `providers` + 显式 SQL 迁移 `finance_provider_records` | 同 grant+record_hash 唯一 | 禁止 AutoMigrate 代替迁移 |
| G3.2 | `data-query`仅两种operation；财务按G1实测冻结的1～6次HTTP上限；正文≤1次检索+5次读取；整个工具≤15秒 | B-13统计真实出站，含重定向/元数据请求 | 无冻结上限或超限失败，禁止隐式分页/重试 |
| G3.3 | 证据 POST 只收 `record_ids`；篡改指标/来源/时间拒绝 | B-11 | — |
| G3.4 | 规范化：UTF-8、NFC、CRLF→LF；三指标转 **CNY 元** 十进制字符串，缩放因子进私有 basis | B-21；万元/元一致 | 只改单位标签不缩放 → 失败 |
| G3.5 | `growth_rate` 按 percent；零分母/非正基数不足 | 100→120 = 20.00 | — |
| G3.6 | 对冻结 3 股跑 live 读取（无模型也可） | B-18 | 删 Key/断网必须失败，不得出 fixture 报告 |
| G3.7 | 时间闸门：`published_at`、`available_at` ≤ `as_of`；不得一律复制 published_at 冒充 PIT | B-19；B 关闭历史模式 | 缺可靠时间则该条不用作历史判断 |

### 3.5 G4 编排（B-22～B-29）

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G4.1 | Eino：Freeze → 支持∥挑战（真并发屏障）→ 校验 → 合成 → 复核 → 最多 1 次修复并全量重验 → 发布 | B-23/B-24 | 单路失败不得 completed+supported |
| G4.2 | 私有 `finance_report_checks`：对外每个事实句、summary、重大数字绑定；`unknowns` 过 §2.7 正则；合成提示词含 §2.7 三条，**不**新增 checks 字段、**不**另开角色 | B-22；§2.6 | 模型说已校验无效；扩表或加角色冒充清单 |
| G4.3 | 预算原子预占；unknown 不退款；解析用量只转入一个 run 一次 | B-25 | — |
| G4.4 | 取消 ≤20s 终态；撤权后新出站 0；删除仍占槽直到终止 | B-26/B-27 | — |
| G4.5 | 重启不重放外部调用；deadline 后 5s 内结算，不得标 completed | B-27/B-28 | — |
| G4.6 | SSRF、注入、日志无 Key | B-29 | — |

### 3.6 G5 产品（B-05、B-30～B-35）

| 步 | 动作 | 通过标准 | 失败则 |
|---|---|---|---|
| G5.1 | 实现 `app/scripts/verify-stage-b.sh`：`offline / integration / live / all` | 缺凭据退出 2；必需 skip/空集退出 3 | 禁止假绿 |
| G5.2 | 冻结 30 例质量集（≥10 反向） | B-30 | 空样本不计 |
| G5.3 | 冻结 10 条真实观点（覆盖 3 只股票），按 §2.3 跑 20 次 | ≥18/20 流程完成；≥5 个不同输入含有效事实证据；关键数值/时间/权限错误 0 | 不得删失败重算分母 |
| G5.4 | 浏览器：真用户登录→确认→已发布报告（四态判断、支持/反证、带前缀的未知项）→抽屉可见 retrieved_at→追问/历史；取消与删除 | B-05/B-31/B-32；§2.7 功能块齐全 | 视觉改版、导出、展示编号不是本关范围 |
| G5.5 | YUFAN（或指定人）对 live 对外重要事实/推断做引用蕴含标注 | 支持率 ≥90%，明显误导 0 | 开发者不得代标为通过 |
| G5.6 | 交付包：锁文件、迁移、启动说明、限制清单、脱敏 trace | B-35 | — |

---

## 4. 架构（B-03、B-04）

```text
浏览器 Vue  ──只访问──►  app/server（独立宿主：账号/JWT/状态/预算/证据/发布）
                              │
                              ├─ Eino：Freeze → [supporter ∥ challenger] → 校验 → 合成 → 复核 → 发布
                              │                    │
                              │                    ▼
                              │         research-service（DeerFlow × 2，仅打 Go 内部 API）
                              │                    │
                              ├─ 模型网关 ◄────────┤  Chat Completions / Responses 上游
                              └─ data-query ◄──────┤
                                      │
                                      ▼
                         providers（Go）──► 东财/新浪/巨潮 HTTP
                              或唯一备用：data-connector（锁定 AKShare，仅 Go 可调用）
                                      │
                                      ▼
                               PostgreSQL（Go 唯一业务写入）
```

- 已有同职责模块原位扩展；迁移/重命名必须列出旧→新映射。
- `app/gosaas` 若本机存在：不是启动路径，未经确认不得删除用户文件，也不得当第二运行系统。
- 单 Go、单研究 Python、PostgreSQL、研究侧 SQLite 只存执行态。可选 data-connector 不是第二套研究 Loop，不存供应商模型 Key，不接受浏览器。
- 运行依赖以 `go.mod` / `uv.lock` / `package-lock.json` 为准。`references/` 不随公开仓库分发，不得修改。
- DeerFlow：公开克隆缺失时 G2 真实 invoke 为 **blocked**，不是 pass。维护者本机 pin 与锁文件一致才能签收 G2。

---

## 5. 用户、管理员与配置（B-05～B-08）

沿用 `/api/finance/*` 与统一 envelope。

1. 输入 20–2,000 字 → `claims/parse`。live 用模型结构化输出；失败不猜证券、不启动研究。
2. 用户保存覆盖内证券、绝对期限、1–6条主张后确认；as_of由服务端生成。B无PIT开关。
3. `research` 带 `Idempotency-Key`；同键同体返回原 run，异体 409。
4. 每 2 秒轮询真实阶段与 `live|fixture`，不显示猜测百分比。
5. 只渲染已发布报告。`incomplete` 明示「研究未完成」，`verdict=null`。
6. 追问最多 3 轮，只用该报告已获准证据，不新检索。新事实提示重新研究。

### 5.1 草稿与确认契约

以下为B请求/响应的 `data` 字段；旧创建参数 `claim_text、horizon、as_of` 不再接收，前后端同批升级。公开报告仍使用v1 Schema。

| API | 请求 → 响应 |
|---|---|
| POST `/api/finance/claims/parse` | `{text}` → 草稿：`{draft_id,revision,parse_status,candidates,items,instrument_id,horizon_start,horizon_end,model_config_version,protocol,mode}` |
| GET `/api/finance/claims/:id` | 返回本人草稿；解析失败仍可读取，`parse_status=failed`，缺失选择为null、items可为空 |
| PATCH `/api/finance/claims/:id` | `{revision,instrument_id,horizon_start,horizon_end,items}` → 更新后的完整草稿，revision加1；不调用模型 |
| POST `/api/finance/research` | Header `Idempotency-Key`；`{draft_id,revision,parent_run_id?}` → 202 `{run_id,status,poll_url,as_of,model_config_version}` |

`items`为1～6个 `{claim_id,text,claim_type}`，claim_type沿用fact/inference/assumption；claim_id在草稿内唯一。期限为ISO日期，start≤end；缺字段返回422，版本过期或草稿已确认返回409。候选为空/解析失败时，用户可手填完整范围和items后确认，记录 `scope_origin=manual`；原始text修改须重新解析成新草稿。

确认事务检查owner、revision、模型版本未撤销、范围完整与幂等；复制已保存范围，禁止重新使用旧items或当前active模型。相同幂等键/请求返回原run，异体409；同一草稿只能确认一次。run的模型继承草稿，数据/Prompt/预算版本和as_of在确认事务冻结；解析消耗只关联此run一次。内部v1 `claim.horizon`固定编码为`YYYY-MM-DD/YYYY-MM-DD`，数据库保存两个日期字段。

GET `/api/finance/research/:id` 的 `data`新增 `claim_results`（§2.6结构），只返回与已发布report同版本的结果；未发布为null。它不嵌入旧report Schema。

### 5.2 后台

后台 `/admin/ai-settings`：

| 设置 | 必填 | 行为 |
|---|---|---|
| 模型 | 名称、protocol（两协议枚举）、允许名单内base_url、model、Key、输入/输出上限、超时 | 四用途固定共用版本，不提供逐用途切换；保存不可变版本，GET只回has_key |
| 模型测试 | Go JSON、Python 工具往返、错误/超时、无证据输出 | 返回 test_id、分项、digest、账本；不是 HTTP 200 即通过。后台分列「协议实现 / 当前配置 live」 |
| 数据源 | connector 类型 `akshare_http`（名称表示路径，不表示研究进程 import）、覆盖目录、财务/正文能力、展示限制记录 | 测一次真实三指标、一次巨潮检索。缺正文能力不得启用为完整研究配置 |
| 策略 | 并发与硬上限，只允许降低 | 变更出新版本 |
| 运行审计 | run/task/request、模式、配置版本、耗时、用量、错误 | 默认脱敏 |

base_url为供应商根路径，不含资源后缀，适配器只追加一次。切换active只影响此后新建草稿；旧草稿/run继续使用原版本。紧急撤销使对应草稿不能确认、在途运行失败/取消；不换模型续跑。

POST `/api/finance/admin/model-config-versions/:id/test` 可测试指定未启用版本；先验证管理员、费用和允许名单，再签发admin_test上下文。测试工具返回受控样本，不计作真实数据接入证明。test结果绑定config digest；全部必需项通过才能activate，无需预先启用配置才能测试。

JWT 管入口；每次读/改仍查 owner。非本人统一 404。

---

## 6. 内部协议（B-09～B-13）

保留 v1.0 四份公开 Schema（task/result/evidence/report），禁止给 `additionalProperties=false` 偷加字段。B 内部控制用 header `X-Zhigu-Contract-Version: 2`。新增对象 Schema 落 `app/contracts/internal-v2/`，先契约后实现。

Go→Python：POST `/internal/research/tasks`、GET `/internal/research/tasks/:task_id`、POST `/internal/research/tasks/:task_id/cancel`；Authorization携带服务token，提交另带 `X-Zhigu-Task-Token`。Python→Go的Authorization携带该任务能力token。临时token不进正文、Prompt、日志、SQLite明文；所有内部入口（含fixture）要求控制协议header。

Go持有下述调用上下文，Python只能使用Go签发的能力。控制层以purpose分账，模型stage另标parser/supporter/challenger/synthesizer/verifier/repair/reverify，不增总额度。

| purpose | 必需关联 / 配置 / 能力与预算 |
|---|---|
| parse | owner_id+draft_id；草稿冻结版本；仅模型，45秒内最多2次请求，确认后用量转入run一次 |
| research | owner_id+run_id；角色调用另有task_id；run冻结版本；角色可用三工具，Go合成/复核只用模型；共享run预算 |
| question | owner_id+run_id+report_version+question_id；原报告模型版本；仅模型和已发布证据，45秒内最多2次；单独分账；模型撤销则409 |
| admin_test | admin owner_id+test_id；指定待测版本；每个测试case≤120秒/3模型/3工具，独立费用额度；不能调用消费者run或发布报告 |

Go本地节点也须先注册上下文再调用统一网关；无run的parse/admin_test不占研究并发或每日10次配额，仍受自己的频率/费用约束。

| 路由 | 约定 |
|---|---|
| `POST /internal/llm/v1/chat/completions` | stream=false；仅 protocol=`openai_chat_completions`；model 别名 `finance-research` |
| `POST /internal/llm/v1/responses` | stream=false、background=false、store=false；仅 `openai_responses`；只允许本项目 function tools |
| `POST /internal/finance/tool-grants` | `{request_id,tool_name,args_hash}` |
| `POST /internal/finance/data-query` | `{grant_id,operation,params}`；operation **仅** `get_financials` / `search_filings`。成功返回 `records:[{record_id,record_hash,record}]` |
| `POST /internal/finance/evidence` | `{grant_id,record_ids:[...]}` |
| `POST /internal/finance/calculate` | `{grant_id,operation,inputs}` |
| `POST /internal/finance/tool-grants/:id/complete` | `{status,output_hash}`；unknown 不得改成虚假 succeeded |
| `GET /internal/finance/data-source-profile` | 冻结覆盖与规则，无 Key |

以上金融API统一 `{data,error,trace_id}`；模型API保留原生格式。`tool-grants`的data为 `{grant_id,expires_at,status}`；`evidence`为 `{evidence_ids}`；`calculate`为 `{calculation_id,value,unit,formula,precision,evidence_ids}`。data-query的data为 `{records,quality_status,warnings}`；profile字段见下表。成功200，grant首次创建201；处理中409 `REQUEST_IN_PROGRESS`；校验400、scope403、预算429、来源不可用503。

### 6.1 核心对象（待生成对应 internal-v2 Schema）

下表字段均必填；`?`表示可空/不适用，所有ID为字符串，金额/指标为十进制字符串，时间为RFC3339 UTC；未知字段拒绝。

| 对象 | 字段与约束 |
|---|---|
| ExecutionContext | `context_id,owner_id,purpose,stage,draft_id?,run_id?,task_id?,question_id?,test_id?,report_version?,model_config_version,protocol,source_policy_version?,prompt_version,budget_version,mode,as_of?,deadline_at,execution_epoch,allowed_tools[]`。role 由 task 映射；expires/revocation 存服务端能力记录，不能靠 header 自报。口径四行只进系统提示词前缀（§2.7），本对象不加字段 |
| record | `instrument_id,source_id,source_url,source_kind,title,locator,text,metrics,published_at,available_at,retrieved_at,data_version,mode,basis`；metrics同v1 Evidence，basis含原始值/单位、转换因子、合并口径、revision_id?、date_precision、availability_basis；金融记录无正文时text由Go确定性格式化 |
| metric_ref | `{evidence_id,metric,period}`；period为年度期末日；必须唯一匹配该证据中的一个指标，否则422 |
| numeric_binding | `{pointer,display_value,display_unit,source}`；source为 `{kind:metric,ref:metric_ref}` 或 `{kind:calculation,calculation_id}`，二选一；pointer指向报告文本字段，数字和单位须可复算匹配 |
| data-source-profile | `{source_policy_version,data_version,connector_mode,instruments,metrics,periods,financial_http_limit,filing_http_limit,tool_timeout_ms,source_rules}`；connector_mode仅go_http/akshare_service；source_rules含允许host、正文/日期规则，无凭据 |

record_id由Go生成，`record_hash=SHA256(RFC8785(record))`，不包含随机ID；`content_hash`只对规范化text。Go校验、规范化后写finance_provider_records，创建v1 evidence时由Go补ID及content_hash；前端按date_precision展示日期精度，不把占位时刻显示为精确披露时间。

### 6.2 grant与登记重试

grant状态为reserved→running→succeeded/failed/unknown；真实出站前原子抢占，超时不确定为unknown，次数不退款。args_hash覆盖工具完整参数；operation必须匹配tool_name。同request_id同体返回原grant，异体409。grant.expires_at限制首次执行开始；成功后的登记/读取仍受任务能力期限控制。output_hash为对应查询/计算成功响应data的规范JSON SHA-256，Go生成，Python核销时复算。

成功查询将records与grant结果一起提交。证据登记是独立、可重试的本地事务：同grant+record_id返回相同evidence_id；响应丢失后重试不得再次出网。查询running返回409，failed/unknown禁止重执行；已成功grant可在任务有效期间重试登记。complete仅核对Go保存的status/output_hash，不触发查询、不覆盖unknown。任务能力过期、取消、旧epoch均禁止登记/写回。

入口与冻结协议不符 → 409 `MODEL_PROTOCOL_MISMATCH`，不改路由、不重试另一协议。Go 向 Python 传非敏感 `X-Zhigu-Model-Protocol`。

`args_hash`：RFC 8785 JSON + SHA-256。grant 下真实查询/计算至多一次。`record_hash` 覆盖完整规范化记录；`content_hash` 只表示原文。

错误码最低集：`CONFIG_NOT_READY`、`MODEL_CONFIG_REVOKED`、`CONTRACT_VERSION_MISMATCH`、`TASK_SCOPE_DENIED`、`EXECUTION_REVOKED`、`IDEMPOTENCY_CONFLICT`、`REQUEST_IN_PROGRESS`、`BUDGET_EXHAUSTED`、`DEADLINE_EXCEEDED`、`DATA_UNAVAILABLE`、`EVIDENCE_REJECTED`、`WORKER_RESTARTED`、`MODEL_PROTOCOL_MISMATCH`。任意异常不得变成空内容成功。

---

## 7. 真实模型与有界 DeerFlow（B-14～B-17）

两种底层接入都是本期范围。一个配置版本一种协议。技术链：`Go 网关 → ChatCompletionsAdapter | ResponsesAdapter → 上游`。Eino 与 Python 都走该网关。禁止把 Responses 经外部 Chat 转发冒充原生。

字段映射按 OpenAI 官方迁移文档；第三方必须实测。内部统一：文本片段、工具请求列表、完成/拒绝/截断/错误、usage、上游标识、仅当前 task 可回放的 continuation Items。Responses 不得丢弃必要 reasoning/encrypted 项；它们不进报告与常规日志。

角色续轮Items只由Python任务级模型adapter保存在内存，按task隔离；adapter把工具结果与原Items交回Go的Responses入口。Go负责协议转发/账本，不另维护会话。Go自身节点的续轮由该节点上下文持有。任务结束/取消/重启即清除；重启按失败处理，不从SQLite恢复续轮。Go仅可为幂等保存受保护响应缓存，删除时清理，不能把它当跨任务记忆。

只做非流式、文本与三个 function tools。显式 store=false。Responses 禁止 background、previous_response_id、conversation、内置 web/file/computer/MCP。不支持必要续轮的配置测试失败。

Token/费用：各协议 usage 归一到现有账本，reasoning/cached 不漏、不重复加。回放输入重新预占。usage 缺失为 unknown。不把 Chat 字段原样转发给 Responses。

研究循环：

- 角色 supporter / challenger；中立输入相同；私有 messages。
- 工具仅 `get_financials`、`search_filings`、`calculate_metric`。
- 研究进程网络只允许 Go 内部 API（代码与测试禁止非白名单 URL；compose 内网。B 不做 iptables 验收）。
- 每角色最多 4 次模型、6 次工具；硬挡。
- checkpoint 按 task 分区或关闭；删除时清理。

工具参数：

| 工具 | 模型可填 | 服务端固定 |
|---|---|---|
| get_financials | metrics 1–3（目录枚举）、periods 1–2（`YYYY-12-31`） | 证券、as_of、版本 |
| search_filings | query 1–300 字、limit 1–5 | 同证券、来源白名单；最多 5 段带 locator 的文本；禁止 URL/SQL/路径参数 |
| calculate_metric | `{operation,inputs}`；growth_rate的inputs为`{current:metric_ref,previous:metric_ref}`，ratio为`{numerator:metric_ref,denominator:metric_ref}`，difference为`{left:metric_ref,right:metric_ref}` | 恰好两个有名输入，同task指标；不接收无序数组 |

---

## 8. 数据、计算与发布（B-18～B-24）

### 8.1 Connector 责任

财务 adapter：instrument catalog + financials。正文 adapter：filings 搜索/读取。LLM 不得猜 URL 或供应商字段。映射表必须有测试样本。对模型与 `data-query` 只暴露 `get_financials` / `search_filings`，不得增加 operation，不得按运行时文档发现 API。

可选data-connector仅提供POST `/internal/data/fetch`：Go服务鉴权，输入`{request_id,source_version,instrument_id,operation,params,deadline_at,max_http_requests}`，输出`{responses:[{url,status,content_type,body_base64,retrieved_at}],http_count}`。参数/host来自冻结契约；≤15秒且不超过剩余deadline，重复request_id不重出网。它只取原始响应；校验、文本抽取/规范化、record_hash和业务入库均由Go负责。Go模式和服务模式每个source_version只选一个，失败不自动切换。

### 8.2 冻结出站（G1 核验，变更要升 source version）

下列仅是探测起点。G1.2必须生成私有证据文件 `source-contract.json`，逐operation写明：确切URL/方法、headers与body模板、证券编码映射、字段JSON路径、单位/缩放、年度/合并口径、日期/修订字段、分页规则、HTTP上限、依赖pin及三股响应样本hash。正文另记录下载与提取版本、locator样例。该文件随data_version冻结；未完成不得声称接口已冻结。

| 能力 | 起点 |
|---|---|
| 证券目录 | `GET http://www.cninfo.com.cn/new/data/szse_stock.json`（A 股/北交所）与 `hke_stock.json`（港股，含 orgId） |
| 利润表 / 资产负债表 / 现金流量表 | 东方财富 HSF10：`RPT_F10_FINANCE_GINCOME` / `GBALANCE` / `GCASHFLOW`；港股 HKF10：`RPT_CUSTOM_HKSK_APPFN_CASHFLOW_SUMMARY` + `RPT_HKF10_FN_INCOME_PC` / `BALANCE_PC` / `CASHFLOW_PC` |
| 公告检索 | `POST http://www.cninfo.com.cn/new/hisAnnouncement/query`（A 股 sse/szse/third，港股 hke） |
| 公告文件 | `http://static.cninfo.com.cn/` + 返回的 `adjunctUrl` |
| User-Agent | 固定产品 UA + 联系方式；限流；原始响应落盘 |

允许 host 由 G1.7 冻结。拒绝内网、元数据、loopback（内部 Go↔Python↔data-connector 另表）。

财务一次逻辑工具允许1～6次HTTP，以三股样本实际所需上限冻结（含两年、元数据和重定向）；工具次数仍只计一次，底层HTTP另计。正文最多1次检索+5次读取。超过上限或15秒即失败，不隐式翻页、重试或切源。启用已允许的新浪/AKShare备用只需验证新source version，不更改运行中版本；若备用仍不能满足本契约则blocked。

### 8.3 时间

B 只做当前研究时点。`published_at`、`available_at` ≤ `as_of`。

`available_at` 规则（A 股）：

1. 有可靠披露时间戳及对应版本：转换UTC；basis记录依据，available_at不得早于披露时间。
2. 只有日期且可绑定原始披露版本：basis标 `date_precision=day`、`availability_basis=conservative_day_end`；published_at以该日00:00编码，available_at取北京时间次日00:00。前者仅兼容v1字段，不表示精确时刻；当日材料不能通过截止时间闸门。
3. 动态接口修订时间不明且无法绑定原始版本，或完全无日期：不登记为事实证据，记未知。不能用首次抓取后的新值倒填旧披露时间。
4. 同一run保持确认时as_of；晚于它的材料排除，可在用户主动重新研究时使用。历史模式仍关闭。

### 8.4 计算

- `growth_rate=(current-previous)/previous×100`，单位 percent；previous≤0 不足。
- `difference=left-right`，同指标、同单位、可比口径；`ratio=numerator/denominator`，同期间且同单位，结果unit=`ratio`，分母0拒绝。growth_rate允许相邻完整年度；跨币种、actual/estimate混用或不可比口径拒绝。
- Decimal；展示 HALF_UP 两位。公式与 calculation_id 可回查。

### 8.5 发布

角色结果 → Go 校验 → 合成 → 程序检查 → 语义复核 → 至多一次修复并全量重验 → 发布。修复不得开自由循环。

`finance_report_checks`：候选 hash、JSON pointer、claim_type、evidence_ids、numeric_bindings、规则版本、程序结果、语义结果、失败原因、run_id、尝试序号。修改候选后旧检查失效。

对外文本凡事实性陈述都要过检查。summary 不新增事实、不改写成投资建议。按 §2.6 判 verdict。不得输出买卖指令、保证收益、无依据置信百分比。每个输入主张在 `claim_results` 或 unknowns 中有去向。unknowns 须通过 §2.7 正则。

---

## 9. 预算、故障与安全（B-25～B-29）

硬限额（代码约束）：

| 项 | 规则 |
|---|---|
| 模型 | 每角色 4；每 run 14（解析、两角色、合成、复核、修复、再检查、重试） |
| 工具 | 每角色 6；每 run 12 |
| Token | 单请求入 8000 / 出 1200；run 累计预占入 112000 / 出 16800 |
| 时间 | 排队 30s；执行 180s；模型 ≤45s；工具 ≤15s；与剩余 deadline 取小 |
| 并发 | 每用户 1 活动 run；全局 2；Python 4 角色任务 |
| 次数 | 解析 5 次/用户/分钟；研究 10 次/用户/天；追问 3 轮 × ≤2 模型，无外部工具 |
| 费用 | D-05 未设则禁止付费；无可靠计价不得声称费用封顶已验证 |

通常分配：解析1 + 两角色8 + 合成1 + 复核1 + 修复1 + 再检查1 = 13，剩 1 次重试。共享剩余额度，不各留 14。

网络前原子预占。SDK 自动重试关闭。unknown 保留预占。取消不承诺供应商撤回费用。删除 24h 内清私有内容；未终止删除仍占槽。Python 重启标 `WORKER_RESTARTED`，不盲目重放。B 不承诺断点续跑。超时研究不得 completed。

执行权由Go worker持有：租约15秒、独立每5秒续租；`execution_epoch`仅在新抢占/撤权时递增，`version`在每次状态变更时递增，两者分开。取消/删除事务撤销能力并提高epoch；所有出站、证据写入、任务回写与发布均检查当前epoch。发布还须锁run、核对version/租约/候选hash，禁止覆盖canceling或deleted。worker退出，或租约过期且能力撤销后转canceled；Python只报告任务状态，最终业务终态由Go裁决。

---

## 10. 测试与交付（B-30～B-35）

三层证据不可互相替代：确定性（含真 PostgreSQL）→ 真 Eino/Harness → 浏览器 live。

质量集 30 例、live 20 次、人工 90% 见 §3.6 与验收清单。阈值是 B 拟定验收线，跑完后不得降低。

脚本：`bash app/scripts/verify-stage-b.sh {offline|integration|live|all}` 已存在基础入口，但当前 integration 未覆盖完整浏览器／跨进程验收；须补齐本节和 §12.10 的分项，不能把现有退出 0 当全阶段通过。退出：0 通过；1 断言失败；2 环境/凭据/授权阻塞；3 必需 skip/空集/证据不全。0 不等于用户已签收。

证据包目录见验收清单。公开包只留脱敏摘要与 hash。

换模型/协议：新版本 + 能力测试，同一网关。换数据源：新 adapter + 映射测试 + 覆盖目录版本。加角色/工具必须改白名单、预算、协议与负例。

---

## 11. 签收

当前交付为 **B-1.7 开发契约整合**。本次只做源码静态核对与文档检查，未运行迁移、应用测试、真实模型或人工质量验收。既有证据仍按对应版本核查，不以文档更新重置或补写通过状态。

研究按 §3、策略按 §12.10 的依赖表推进；G0～G5 与 BS0～BS4 分别提交执行证据，最终由 YUFAN 标注并签收。数据源确切字段仍由G1样本验证，本文件不冒充接口实测结果。

阶段 B 完成只表示私有技术样机，不表示公网许可、全面合规或可用于自动投资决策。

---

## 12. 策略市场、数据库与前端开发合同（B-S-01～B-S-10）

本节将策略市场与制作／回测纳入阶段 B。表中「现有」只表示本次读取到源码；「新增／修改」是待开发要求，不是已完成能力。仍采用 PostgreSQL 16、Go/Gin/GORM、Vue 3/Pinia/Vue Router，共用 `finance_users`，不引入第二套用户系统或 Python 策略执行器。策略生成保留 Chat Completions 与 Responses 两种底层协议，使用独立身份／预算，不创建假 research run。

本节阅读顺序：§12.1核对现状 → §12.2～12.3数据库 → §12.4～12.6规则与API → §12.7后端文件 → §12.8前端文件 → §12.9～12.10验收与实施。

### 12.1 源码基线与差距

静态核对日期 2026-09-23，HEAD=`2e0b329`，工作区有用户未提交修改。后续开发必须重新记录 SHA 与 dirty 清单；不得覆盖现有行情、图表或研究改动。

| 范围 | 已有代码 | 本次需要补齐 |
|---|---|---|
| 数据库 | `app/server/model/strategy/models.go`、`migrations/finance/004_strategy.sql`；含 workspace、draft、generation、strategy、version、backtest、idempotency | 市场条目／不可变版本／审核证据／操作日志，草稿编辑态与来源字段，可靠增量迁移 |
| API 与编排 | `api/v1/finance/strategy_handlers.go` 的 `RegisterStrategy`，`service/workbench/hub.go`；`main.go` 注册 | 独立市场读写服务与路由、原子复制；修复复用路径中的并发版本检查 |
| 规则计算 | `service/strategy/dsl.go`、`eval.go` 与 `service/backtest/`；当前 `strategy.v1` | 当前操作数对象仅支持 constant，lag 位于条件节点；不能宣称支持 `{ref,lag}`。新增 v2 编译／求值，保留旧版本重放 |
| 前端 | `src/view/strategies/index.vue`、`api/strategies.js`、`stores/strategyWorkspace.js`、`components/strategies/StrategyRail.vue` | 市场列表／详情、二级导航、通用规则窗口、复制草稿深链加载；当前 K 值专用修改函数不能代表通用编辑器 |
| 路由与布局 | `src/router/finance.js` 仅有工作台；消费者布局将全部 `/app/strategies*` 视为固定工作台 | 新增市场路由，市场页允许纵向滚动；保留工作台行情与移动端页签 |
| 现有缺口 | `PatchDraft` 先读后写；`SaveStrategy` 忽略 base_version 且多步非事务；迁移器简单按分号切句并吞部分 SQL 错误 | 草稿 CAS、保存事务与 base_version 校验；新增迁移不得依赖原有错误吞噬实现成功 |

### 12.2 数据表与约束（B-S-02）

新增显式迁移 `app/server/migrations/finance/005_strategy_market.sql`，不改写已部署的 004。使用现有 text ID 习惯（如 `smi_`、`smv_`），用户 FK 使用与 `finance_users.id` 一致的 INTEGER。JSONB 内禁止存密钥，十进制数用字符串，时间用 UTC TIMESTAMPTZ；业务日期用 DATE。以下字段为最低合同，分号分组中的同名字段类型一致。

| 新表 | 最低字段与类型 | 约束／索引 |
|---|---|---|
| `finance_strategy_market_items` | `id TEXT PK`；`slug TEXT`；`status TEXT`；`current_version_id TEXT NULL`；`revision INTEGER`；`created_by INTEGER FK`；`created_at,updated_at TIMESTAMPTZ`；`published_at,withdrawn_at TIMESTAMPTZ NULL` | slug 唯一；status=`draft/published/withdrawn`；revision≥1。索引 `(status,updated_at DESC,id DESC)`；已发布必须有当前版本 |
| `finance_strategy_market_versions` | `id TEXT PK`；`item_id TEXT FK`；`version_no INTEGER`；`name,summary,category TEXT`；`tags,markets JSONB`；`signal_period TEXT`；`description,hypothesis,failure_cases TEXT`；`sources JSONB`；`rights_note TEXT`；`editor_schema_version TEXT`；`rule_template,backtest_defaults JSONB`；`content_hash TEXT`；`validation_status TEXT`；`validation_report JSONB`；`created_by INTEGER FK`；`created_at TIMESTAMPTZ`；`validated_at,published_at TIMESTAMPTZ NULL` | UNIQUE(item_id,version_no)、UNIQUE(item_id,id)；validation_status=`pending/passed/failed`；名称1～80字、摘要1～160字；模板／来源等内容插入后不可改，修改创建新版本并重验；索引 category、signal_period、item_id |
| `finance_strategy_market_evidence` | `id TEXT PK`；`item_id,market_version_id TEXT`；`status TEXT`；`validation_instrument_id TEXT`；`bound_dsl_hash TEXT`；`config,manifest,metrics,equity,trades,limitations JSONB`；`result_hash,evidence_hash TEXT`；`review_note TEXT`；`created_by INTEGER FK`；`reviewed_by INTEGER FK NULL`；`created_at,reviewed_at TIMESTAMPTZ`（后者可空） | 复合 FK(item_id,market_version_id)→versions(item_id,id)；status=`pending/approved/rejected/revoked`；索引 `(market_version_id,status,created_at DESC,id DESC)`。无证据不建伪记录，不存「零收益」占位 |
| `finance_strategy_market_audit` | `id TEXT PK`；`item_id TEXT FK`；`market_version_id TEXT NULL`；`actor_id INTEGER FK`；`action,request_id TEXT`；`before_revision,after_revision INTEGER`；`reason TEXT`；`payload_hash TEXT`；`created_at TIMESTAMPTZ` | action=`create_version/validate/publish/withdraw/approve_evidence/revoke_evidence`；同版本归属校验；管理员只读审计，应用禁止 UPDATE／DELETE；不记录整段用户文本 |

先创建 items（current_version 暂不加 FK），再建 versions，最后添加 `(id,current_version_id)`→versions`(item_id,id)` 的复合 FK，避免指向其他条目的版本。版本、证据、来源引用用 RESTRICT，市场下架不用物理删除；私人草稿删除不得级联删除市场内容。JSONB 对象／数组形状设 CHECK，具体字段白名单由同版本 JSON Schema 严格校验。

扩展现有表，不新建平行的个人策略／回测表：

| 现有表 | 新增列 | 使用规则 |
|---|---|---|
| `finance_strategy_drafts` | `editor_schema_version TEXT NULL`、`editor_state JSONB NULL`、`field_sources JSONB NOT NULL DEFAULT '{}'`、`backtest_config_draft JSONB NOT NULL DEFAULT '{}'`、`origin_market_item_id TEXT NULL`、`origin_market_version_id TEXT NULL` | 两个来源 ID 同为空或同非空，复合 FK 指向同一市场版本；服务端赋值且不可由普通 PATCH 改写。旧记录允许 editor_state=NULL |
| `finance_strategy_versions` | 同上六列 | 保存时冻结草稿编辑态、配置建议与来源；新保存仍必须有完整可执行 DSL；旧版本不回填推测来源 |
| `finance_strategy_generations` | `mode TEXT NOT NULL DEFAULT 'generate'`、`explanation TEXT NULL` | mode=`generate/modify/explain`；解释成功保存文本但不改草稿规则与revision |
| `finance_strategy_idempotency` | 新增 `response_snapshot JSONB NULL`，复用 `(owner_id,operation,idempotency_key)` 唯一键 | 新 operation=`market_copy`，object_id 为私人 draft_id；请求 hash 覆盖 item_id、market_version_id 和规范化 overrides；新操作保存首次响应便于稳定重放，返回前仍校验对象可见性／未删除，旧记录按原对象查询兼容 |

来源显示状态由查询关联市场条目计算，不写进历史版本内容；市场参数更新不影响副本。若后续删除用户草稿，对应幂等重试返回 410，不以相同 key 新建草稿。

`rule_template` 使用独立 `strategy.market.v1` 信封，包含 `editor_state`、`instrument_binding={mode:select_one,markets:[...]}`，不填写假 instrument_id。`sources[]` 至少含 title、URL 或内部记录号、整理日期、原版／改编说明；`backtest_defaults` 只允许本模块可配置字段。审核状态、发布时间、能力标记、content_hash 不接受客户端自证。

### 12.3 迁移与数据生命周期（B-S-02／03）

- 修改 `initialize/db.go`：增加 `finance_schema_migrations(version TEXT PK,checksum TEXT,applied_at TIMESTAMPTZ)`，事务级数据库锁确保单执行者，每个迁移文件原子执行后登记 checksum；失败回滚且阻止启动，不把 duplicate key 等数据问题一概当成功。
- 新库执行 001～005；已有库先只读核对 001～004 的实际表／列／约束与预期，确认一致才登记历史基线，再运行 005。若存在历史部分迁移，明确报差异并修复后继续，不能按文件名盲目补登记。已登记 checksum 改变时停止。
- 替换不支持 SQL 复合语句的简单分号切割方式，使用可处理完整迁移的执行器；保留已提交迁移内容。对空库、已有004库、重复启动、并发启动和中途错误回滚做真实 PostgreSQL 测试。
- 部署顺序：先兼容性迁移→后端新读写→前端入口。旧策略仍可读；关闭市场入口是应用回退方式，不删新表或市场来源列。实施前保存备份并在隔离库验证恢复；本次文档交付不执行备份或迁移。
- 默认 seed 不发布策略、不写收益。测试样例仅存测试 fixtures；正式市场没有审核内容时保留空态。

### 12.4 规则版本、草稿与 AI 合同（B-S-05／06）

1. 保留 `strategy.v1` 解析、编译、求值与旧回测重放；新增 `strategy.v2` 支持 `{"ref":"volume","lag":1}`，字符串引用视为 lag=0，常数仍为 decimal string。两版本均严格校验互斥节点、未知字段、类型和范围。v1 的节点 lag 语义不得重解释；显式升级时计算等价映射并保存新版本。
2. 新建 `strategy.editor.v1`：字段含 name、可空 instrument_id、signal_period、price_basis、indicators、entry、exit、position、risk、execution。条件组／操作数采用 v2 结构，叶节点另可带 `repeat:1..20`；连续放量作为受限快捷节点 `{kind:volume_increase,days:N}`。界面节点 ID／展开状态放独立 ui_metadata，不参加交易语义 hash。
3. 服务端将编辑态编译为 v2 与 compiled。repeat 按滞后偏移展开，连续放量按策略附录的 N+1 日口径生成比较与交易日连续性检查；连续性检查写入可信 compiled 并参与其 hash，不能由浏览器随意传入，求值必须执行这些检查。展开后仍≤100条件节点、8层、lag≤250；J不裁到0～100。
4. 草稿允许缺标的或阈值，返回 `needs_clarification` 与字段级 missing_fields；此时 DSL／compiled 不可执行。完整校验后 `ready` 只代表规则完整，回测还需数据和费用门禁。缺字段与非法完整输入分开处理。
5. `PATCH /strategy-drafts/:id` 扩展为 `{revision,editor_schema_version,editor_state,backtest_config_draft?}`，或兼容旧 `{revision,dsl}`；两种表示不可同时提交。owner由鉴权取得，SQL更新必须匹配 id＋owner＋revision，成功revision+1；冲突409，返回的最新草稿仍经owner校验。
6. GET/PATCH 草稿在原返回字段上增加 editor_schema_version、editor_state、field_sources、backtest_config_draft、missing_fields、origin。field_sources 由服务端维护，值为 user/context/system_default/market_default/ai_suggestion；origin含市场ID、版本及当前上架状态。不能让请求自行设置审核状态或市场来源。
7. 保存策略复用现有 POST `/strategies`、`/strategies/:id/versions`，在同一事务中锁定草稿与策略、核对 revision 和 base_version_id、写版本／指针／幂等记录。无效引用不留空策略；并发同请求只写一版本，异体同key409。模型迟到结果也按其 base_revision CAS，不能覆盖窗口编辑。
8. AI仍通过既有 generation 路径，接收最新编辑态、指标目录及明确变更目标；生成结果经同一编译器。市场详情的「让 AI 解释」进入工作台的草稿上下文，以 explain 模式运行现有生成服务：只返回 explanation、不修改规则；生成请求新增 mode=`generate/modify/explain`，默认generate，GET generation返回mode与explanation，解释成功仅使generation终态ready，不改草稿revision。计入独立策略模型预算。没有Key可复制／手动编辑，不伪装AI解释成功。
9. 老版本打开时由服务端显式返回可转换编辑态与转换状态；不能仅通过前端 readK／walkSetK 推测完整规则。保存转换结果创建新版本，原版不改。

### 12.5 市场 API 与 DTO（B-S-03／04）

新增路由前缀 `/api/finance/strategy-market`，与行情 `/api/finance/market` 区分。沿用 `httpx.Envelope{data,error,trace_id}`、Bearer和未知字段拒绝规则；返回新DTO而非直接序列化GORM对象。前端 `utils/http.js` 已解一层信封，组件只读取 `res.data`，不得再次猜测 data.data。

| 方法／路径（相对上述前缀） | 请求 | 成功 data |
|---|---|---|
| GET `/items` | `q`≤100字；category、market、period、validation、evidence、cursor；limit默认20、最大50 | `{items:[MarketCard],next_cursor}`；只查已发布当前版本 |
| GET `/items/:id` | 无必需query | `MarketDetail`：当前版本、完整只读规则与来源、能力／copyable、证据摘要 |
| GET `/items/:id/evidence/:evidenceId` | section=`overview/equity/trades`，后两者cursor＋limit≤100 | 审核通过且属于该当前版本的证据；概览含配置／manifest／指标／限制，曲线及成交分页读取 |
| POST `/items/:id/copies` | `Idempotency-Key`；`{market_version_id,overrides?:{instrument_id,initial_cash,currency,start,end}}`，字符串金额／ISO日期 | 201 `{draft_id,revision,status,origin,next_path}`；next_path=`/app/strategies?draft_id=...`；重试同体200同一草稿 |

MarketCard字段：`id,market_version_id,version_no,name,summary,category,tags,markets,signal_period,validation_status,backtest_status,published_at,updated_at,copyable,copy_disabled_reason`。`backtest_status=not_tested/has_evidence`仅从approved证据导出，不等于策略有效；列表不加收益排行。MarketDetail增加description、hypothesis、failure_cases、sources、rights_note、editor_schema_version、editor_state、backtest_defaults、evidence_summaries。用户密钥、私人run_id、创建人内部身份不进入这些DTO。说明文本按纯文本或经过净化的Markdown渲染，禁止原样v-html；来源URL只允许http/https，内部来源编号不可拼成任意外链。

列表默认 `(updated_at DESC,id DESC)` 游标分页，游标绑定筛选条件，改变筛选从第一页重查；非法枚举／游标400，不当空结果。未发布或从未存在条目404，曾发布后下架410，仅返回最小状态；不继续公开未审核新版本。数据库发布指针为列表和详情唯一当前版本依据。

复制事务：先按用户＋operation＋key做数据库级串行化并查幂等记录；已有同体记录可返回原副本（即使源随后下架），不产生新复制；异体409。新请求锁定市场条目，核验 published、请求版本为current、validation=passed、引擎支持，再原子写来源草稿＋幂等。并发下架使用同一条目锁。更换股票只改变私人草稿，不改变市场原版或证据；缺股票时允许创建待补全草稿，绝不拿验证样本股票自动替用户选择。

错误至少包括：401 `UNAUTHENTICATED`；403 `FORBIDDEN`；404 `NOT_FOUND`；409 `REVISION_CONFLICT/MARKET_VERSION_CHANGED/IDEMPOTENCY_CONFLICT`；410 `MARKET_ITEM_WITHDRAWN/COPY_TARGET_GONE`；422 `STRATEGY_INVALID/STRATEGY_UNSUPPORTED/MARKET_NOT_COPYABLE`；503 `CONFIG_NOT_READY`。用户输入有误400，数据不足使用既有回测终态，不返回伪成功。

### 12.6 上架与证据维护入口（B-S-03／08）

首版复用现有管理员身份，通过受保护 API 完成内容维护，不强制新增后台页面；不能靠直接改数据库或消费者上传来完成验收。前缀 `/api/admin/strategy-market`，必须同时经过 AuthRequired 与 AdminRequired；响应仍用统一信封。

| 方法／路径 | 请求和处理 |
|---|---|
| POST `/items` | `{slug,version}`；version包含§12.2的可写内容字段；同事务建draft条目与首个不可变版本；201返回item_id/version_id/revision |
| POST `/items/:id/versions` | `{revision,version}`；校验条目revision，创建新版本并revision+1；不自动改变已发布指针 |
| POST `/items/:id/versions/:versionId/validate` | `{revision,validation_instrument_id}`；在受支持真实目录标的上绑定规则并编译、核对来源和必填字段，生成服务端报告／hash，revision+1；不运行回测、不声称数据就绪 |
| POST `/items/:id/publish` | `{revision,market_version_id,reason}`；版本属于条目、来源／权限信息完整、内容hash匹配、规则校验通过才原子切换当前指针并发布；可无回测证据，显示未回测 |
| POST `/items/:id/withdraw` | `{revision,reason}`；原子下架并revision+1，停止新复制，保留副本与历史 |
| POST `/items/:id/versions/:versionId/evidence` | `{revision,source_run_id,rights_note}`；仅允许管理员自己持有的、用于策略验证的succeeded回测记录，核验绑定DSL／标的与该市场版本一致；提取白名单配置、manifest、净值／交易，不复制私有输入，创建pending审核快照 |
| POST `/items/:id/evidence/:evidenceId/review` | `{revision,decision:approve/reject/revoke,reason}`；批准须有完整manifest、结果hash及披露权限记录；撤销后公开查询不得返回原内容；更新revision与审计 |

上表所有写入要求 Idempotency-Key，复用策略幂等表但operation分离（如market_publish、market_evidence_review）；请求hash包含路径、revision和完整规范化请求体。版本/证据内容不原地修改，审核结果与状态更新留审计；失败回滚。source_run_id仅作受控导入请求与私有审计引用，不返回C端；导入snapshot后不依赖私人回测API展示证据。

证据导入不自动拉取外站或运行任意文件／URL；导入前检查记录用途、来源授权及敏感字段，公开DTO禁止输出原始用户文本、模型凭证、owner信息。若无法取得合格历史回测记录，允许发布“未回测”的规则条目；不得用手填收益完成证据状态。策略解释和参数明确均不构成投资有效性验证。

### 12.7 后端代码职责映射（B-S-02～08）

以下路径均相对仓库根；新增文件不得据此声称已经存在。

| 文件／目录 | 动作与职责 |
|---|---|
| `app/server/migrations/finance/005_strategy_market.sql` | 新增4张市场表、现有表增量列／FK／索引；配合迁移账本，不塞默认推荐策略 |
| `app/server/initialize/db.go`、`migrations/embed.go` | 修改迁移执行与核对逻辑，沿用嵌入SQL；账本、锁、事务和checksum测试 |
| `app/server/model/strategy/market.go`（新）与 `models.go` | 市场GORM模型；扩展Draft/Version，JSONB映射与SQL字段一致 |
| `app/server/service/strategy_market/{service,types,publication,copy,evidence}.go`（新） | 市场查询DTO、审核发布／下架、原子复制／幂等、证据脱敏与审核；共享DB但不混进finance研究服务 |
| `app/server/api/v1/finance/strategy_market_handlers.go`（新） | 消费者与管理员路由、输入校验、HTTP错误；在 `app/server/main.go` 注入并注册；不误改无实际接线的router_biz占位 |
| `app/server/service/strategy/{dsl_v2,editor}.go`（新）、`dsl.go`、`eval.go` | v1兼容、v2操作数和求值、编辑态编译与升级、连续窗口校验；generation/prompt同步同版本Schema |
| `app/server/service/workbench/{hub,drafts,versions}.go`（后两者新） | 从hub拆出或明确封装草稿CAS、保存事务、来源传播；保留既有行情方法和HTTP兼容入口 |
| `app/server/api/v1/finance/strategy_handlers.go` | 扩展草稿请求／返回，解释模式与新版字段，拒绝客户端owner和来源伪造 |
| `app/server/service/backtest/engine.go`、`causal.go` | 按冻结规则版本调用求值器、执行连续性检查；不同编译版本进入manifest/hash |
| `app/server/contracts/strategy/`（新） | `editor-v1.schema.json`、`dsl-v2.schema.json`、`market-v1.schema.json`、`market.openapi.yaml`；必须与运行时校验一致，覆盖请求／DTO／错误 |

### 12.8 前端页面、组件与状态合同（B-S-01／05～07）

沿用 Vue SFC＋JavaScript 与现有组件库，不为本轮切换框架或整站重构。

| 文件／路由 | 动作与职责 |
|---|---|
| `app/web/src/router/finance.js` | 新增 `/app/strategies/market`→market.vue、`/app/strategies/market/:id`→marketDetail.vue；保留工作台。市场路由meta `strategySection=market`、`scrollMode=page`，工作台`workspace`；共用登录守卫 |
| `app/web/src/view/strategies/market.vue`（新） | 市场列表、搜索／筛选、游标分页、加载／错误／空态；URL query保留筛选，返回保持滚动位置 |
| `app/web/src/view/strategies/marketDetail.vue`（新） | 只读规则、来源、假设、证据分页；复制／解释操作；版本变化、下架、证据撤销即时明确提示 |
| `app/web/src/components/strategies/StrategySubnav.vue`（新） | 市场／工作台二级导航，详情归属市场；沿用一级SideNav，不新增一级菜单 |
| `StrategyMarketCard.vue`、`StrategyProvenance.vue`、`StrategyEvidencePanel.vue`（同目录新增） | 卡片只展示有依据状态；来源安全链接；只读历史证据与私人回测结果分开 |
| `RuleEditor.vue`、`RuleGroup.vue`、`RuleConditionRow.vue`（同目录新增） | 同一规则组件支持只读与编辑；桌面抽屉、手机全屏；完整字段／AND/OR/NOT、持续条件；窗口副本、应用／取消／冲突处理 |
| `app/web/src/api/strategyMarket.js`（新） | §12.5端点封装；AbortSignal、Idempotency-Key；不在组件拼URL／手动解多层信封 |
| `app/web/src/stores/strategyMarket.js`（新） | 列表与详情独立busy/error；请求序号防旧响应覆盖，筛选切换清空旧游标；稳定复制key、草稿跳转、证据加载；不持久化私人副本内容到共享市场缓存 |
| `app/web/src/stores/strategyWorkspace.js`、`api/strategies.js` | 增加 `loadDraft(draft_id)`、通用编辑态与CAS、origin、配置草稿、字段来源和解释；复用保存／回测，替换K专用编辑路径；不改变图表指标与策略指标的隔离 |
| `app/web/src/view/strategies/index.vue`、`StrategyRail.vue` | 接收query draft_id深链，刷新从后端加载本人草稿，外人ID404；规则编辑入口及来源提示；未保存内容离开提示；新草稿清除旧策略versionId和旧回测结果 |
| `app/web/src/layout/consumer/index.vue` | 将当前 `/app/strategies*` 一律固定布局改为按route meta区分；列表／详情可滚到页尾，工作台仍保持行情布局 |
| `app/web/src/components/workspace/{SideNav,WorkspaceHeader}.vue` | 保持策略路径一级高亮及标题；必要时显示二级面包屑，不改研究导航行为 |

规则窗口读取编辑态，展示完整参数、字段来源和白话摘要，不在前端重新计算回测或推测指标；本地预校验仅为输入反馈，服务端为最终判定。连续放量和J值口径沿用专项附录，不以样例参数做市场推荐。

交互状态必须验收：搜索防抖300ms并取消旧请求；请求失败显示重试而非空态；复制中禁重复按钮、网络超时使用原key重试，直到确定结果后才清除key；改请求体需新key。切到其他账号清空两个store、复制key和缓存。市场版本变化409后重新载入并让用户核对，不能无提示改复制对象。

详情复制成功进入 `?draft_id=`；原工作台有未保存编辑时先保存或由用户选择放弃／取消导航，取消导航不重复复制。挂载市场页不启动行情轮询或回测，卸载工作台清理计时器／未完成请求；后台运行中的回测状态从服务器恢复，不因切页取消。应用规则不自动运行回测。

### 12.9 可执行验收项（B-S-01～10）

| ID | 必须覆盖 | 最低证据 |
|---|---|---|
| B-S-01 | 二级路由、筛选返回、详情、登录、桌面／手机滚动、失败／空／下架状态 | Playwright截图＋网络trace；包括窄屏页尾操作 |
| B-S-02 | 新库／旧004库迁移、checksum、并发启动、失败原子回滚、FK／唯一约束 | 隔离真实PostgreSQL测试与升级前后数据断言 |
| B-S-03 | 管理员上架／下架、版本不可变、来源审核、普通用户写403、draft不可见 | API＋DB＋审计记录；无正式内容空态 |
| B-S-04 | 同key同体只一副本、异体409、并发下架、跨用户隔离、已删副本不重建、旧版本冲突 | 并发HTTP与数据库行数／owner断言 |
| B-S-05 | AI→窗口→保存→重开无损、局部修改、草稿CAS、base_version冲突、两协议生成／解释 | 受控模型请求与持久化比对；live按凭据另记 |
| B-S-06 | v1重放不变、v2类型／lag／未知字段、连续窗口／预热、J边界与负值、取消编辑 | 人工可算序列＋Schema负例＋规则往返测试 |
| B-S-07 | 市场复制→个人编辑→保存→回测→结果；切股票／版本清理旧结果，取消／恢复与错误 | 真Vue→Go→PostgreSQL／worker链路；fixture必须标明 |
| B-S-08 | 证据仅approved且版本匹配、撤销后不可读、私有run不外泄、无证据不填收益 | API负例、DTO字段断言、公开结果hash／manifest核对 |
| B-S-09 | 原研究三工具／report／双协议不变，个人策略owner守卫、现有行情工作台回归 | 相关研究回归＋既有strategy测试，无必需skip |
| B-S-10 | 正式内容来源／使用权限、数据覆盖、真实生成与真实回测的验证层级可追踪 | 内容清单与证据分层；测试夹具不能作为上架策略 |

### 12.10 实施顺序、脚本与完成定义

| 关口 | 依赖 | 交付与签收 |
|---|---|---|
| BS0 基线与合同 | 当前SHA／dirty清单及G0账号／宿主检查 | 核对§12.1、Schemas／DTO、迁移方案；只记录实际证据 |
| BS1 数据库与市场服务 | BS0；隔离PostgreSQL | 表／迁移、内容维护、列表／详情、证据审核；B-S-02／03／08相关项 |
| BS2 市场浏览 | BS0可先按合同做UI；与BS1通过后联调 | 列表／详情／二级入口／真实空态；B-S-01。mock UI通过不等于服务联调完成 |
| BS3 复制与制作 | BS1；规则v2与草稿／保存事务就绪 | 原子复制、来源、AI／规则窗口、版本兼容；B-S-04／05／06 |
| BS4 回测与联合验收 | BS2／BS3、专项附录行情与回测对应关口通过 | B-S-01～10、受影响研究回归、用户签收；BS2浏览完成不能代替BS4 |

研究G1真实模型或数据阻塞不挡无外部依赖的市场／编辑器开发。策略付费调用仍需D-02／D-05就绪，策略行情源与研究源分别记录能力，不借研究探针证明回测数据完整。无合格策略内容可验收市场空态与受控功能；B-S-10及正式内容闭环保持缺口，不要求为凑数伪造策略。

新增测试文件建议：`app/server/initialize/migrations_test.go`、`service/strategy_market/*_test.go`、`api/v1/finance/strategy_market_handlers_test.go`、`service/strategy/editor_test.go`、`app/web/e2e/strategy-market.spec.js`。复用已存在 `service/workbench/http_test.go`、`service/strategy/dsl_test.go`、`service/backtest/engine_test.go`、`app/web/e2e/strategy.spec.js`；新增路径只是待开发入口。

扩展现有 `app/scripts/verify-stage-b.sh` 为阶段B统一证据入口，在manifest分别报告 research与strategy。研究保留§10的标准；策略至少运行如下检查：

| 模式 | 策略线命令／要求 |
|---|---|
| offline | `cd app/server && go test ./...`；`cd app/web && npm run build`；Schema与规则测试不可空集 |
| integration | 隔离PostgreSQL执行迁移／并发／事务测试；`go test -race ./service/strategy_market/... ./service/workbench/...`；启动真实Go+Web后 `npx playwright test e2e/strategy-market.spec.js e2e/strategy.spec.js`。替身上游只可标controlled |
| live | 市场来源审核、真实行情／费用／公司行动覆盖、实际模型生成与样本回测另存证据；两协议可用性分开记录，无Key不伪通过 |

原策略专项附录S-01～S-13及其细分项继续作为计算和回测验收细则，由manifest映射到B-S-ID，不因新增市场缩减。现有脚本退出0不证明新增测试已运行；manifest必须列出精确测试节点和数量。退出码、证据路径沿用§10及验收清单。

阶段B总完成需研究G0～G5和策略BS0～BS4的必需项全部有证据并由用户accepted。允许分线／分关口报告进展；本次文档整合只完成开发规范，不执行迁移、不更改运行中的应用、不认定策略有效性。
