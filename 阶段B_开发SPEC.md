# 知股一期 · 阶段 B 开发 SPEC

版本：B-1.6 · 2026-09-21（覆盖目录扩至全 A 股+港股，财务取数覆盖年度三表关键科目；不改公开 report Schema）  
状态：**已写入拍板决定的开发契约；不是完成报告。** 应用实现、真实调用与样机效果均不因本文被认定通过。G0～G5 初始均为 `not_started`。

目标：普通用户粘贴一个 **A 股或港股** 单公司投资观点，经确认后，通过真实模型、受控公开数据与两个独立研究 Agent，获得一份**观点裁判型研报**：裁决对象是用户确认的主张，交付物是带证据、反证、未知原因与四态判断的已发布报告。知股不生产平行的投资评级、目标价或卖方深度。

继承关系：B-1.6 替代 B-1.5。相对 B-1.5 只放宽研究数据覆盖：全 A 股（含北交所）+ 港股目录，以及已披露年度利润表 / 资产负债表 / 现金流量表关键科目。不改公开 report Schema、不加工具、不把策略日 K 并入研究关口。冲突时以本文 §2 为准。配套只有两份：

- [验收与接入清单](handoff/stage-b/验收与接入清单.md)：D 清单、测试矩阵、证据包。
- [给 Cursor 的开发指令](handoff/stage-b/CURSOR开发指令.md)：执行节奏、暂停条件、汇报格式。

交易策略日 K 不在本阶段范围，独立契约：[策略模块_开发SPEC.md](策略模块_开发SPEC.md)。不得借 B 关口加工具或改 report Schema。研究目录的全 A+港股扩围见 §2.2。

产品/架构仍继承 [PRD v2](金融C端Agent_MVP_PRD.md)、[面客架构 v2](金融C端Agent_面客产品架构_v2.md)、[总 SPEC](开发交付_SPEC.md)。旧文档中的 GoSaaS 迁移、完整 records 登记、operation 短名、确认时改用新模型等冲突条款，在阶段 B 按本文执行；交付时记录差异。

B-\* 是需求 ID，验收附件逐项对应。开发者只能把关口标到 `ready_for_review`，`accepted` 由用户签收。

---

## 1. 范围（B-01）

### 1.1 必须交付 / 明确不做

| 必须交付 | 明确不做 |
|---|---|
| 粘贴观点→解析→确认股票/期限/主张→双路研究→**观点裁判型研报**→证据抽屉 | 自选股持续追踪、用户画像、长期偏好记忆、主动通知、卖方深度/评级/目标价 |
| **独立宿主** `app/server` 为唯一运行入口；Eino DAG；DeerFlow 真实 Harness；Chat Completions **与** Responses 两个 adapter | 迁入 GoSaaS 进程、第二套用户系统、第二套 Supervisor、DeerFlow 通用聊天前端 |
| 至少一套已验证模型配置（用途可共用）；财务三表关键科目 + 公告检索两条数据能力 | 多厂商自动容灾、任意网页浏览、任意 MCP、Computer Use、交易、收益承诺 |
| 角色提示词写成可执行步骤；合成提示词含三条质量检查；未知项过发布正则 | 任意 MCP、Computer Use、子 Agent、Wind/iFinD/S&P、Excel/PPT/HTML 导出、Hosted Agents 替换本架构 |
| 历史、取消、删除、报告内追问；后台保存→测试→启用 | 量化回测、荐股榜、全市场选股扫描、扫描件 OCR、公网开放、业绩点评/持仓早报入口 |
| 私有环境可复现的技术样机 | 合规结论、大规模吞吐、「零幻觉」、阶段 C 运营加固 |

分关口纵向闭环：每关有独立负例和证据，全部 `accepted` 才完成 B。不在最后集中联调，不先建通用 Agent 平台。

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

## 2. 已拍板决定（B-1.6）

以下条款开发中不得再猜。要改必须升 SPEC 版本并改对应测试。

### 2.1 运行底座（D-01 / G0）

| 项 | 决定 |
|---|---|
| 唯一启动进程 | PostgreSQL 16 + `app/server`（Go）+ `app/research-service`（研究执行器）+ `app/web`；可选 `app/data-connector`（仅 Go 可访问） |
| 账号表 | 继续使用现有 `finance_users` 与演示账号 `invitee` / `admin` |
| GoSaaS | **本阶段不迁移、不作为启动依赖。** 不引入 Casbin / MySQL / Redis。私有快照不得拷进可分发仓库 |
| G0 的 A 复核 | 以 [app/IMPLEMENTATION_STATUS.md](app/IMPLEMENTATION_STATUS.md) + 重跑阶段 A 回归为准。不依赖仓库中不存在的 `reviews/阶段A交付审查_2026-09-17.md` |
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

B 的面客完成物是**已发布报告**，不是聊天气泡，也不是公司深度或投资建议。读者读完必须能回答：哪些已披露事实成立、哪些推断没绑住、还缺什么证据。

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

状态只允许：`not_started / in_progress / blocked / ready_for_review / accepted`。编码按下表依赖推进，签收单独记录，不要求逐关人工批准才能编码。任一关阻塞只阻断其依赖项；记录缺项、影响与恢复动作。B 最终 accepted 仍需 G0～G5 全部签收。

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

脚本：`bash app/scripts/verify-stage-b.sh {offline|integration|live|all}`（待实现）。退出：0 通过；1 断言失败；2 环境/凭据/授权阻塞；3 必需 skip/空集/证据不全。0 不等于用户已签收。

证据包目录见验收清单。公开包只留脱敏摘要与 hash。

换模型/协议：新版本 + 能力测试，同一网关。换数据源：新 adapter + 映射测试 + 覆盖目录版本。加角色/工具必须改白名单、预算、协议与负例。

---

## 11. 签收

当前完成的是 **B-1.5 开发契约**。G0～G5、真实效果、费用与人工标注均未通过。

开发按§3依赖表推进；G0～G5分别提交执行证据，最终由YUFAN标注并签收。数据源确切字段仍由G1样本验证，本文件不冒充接口实测结果。

阶段 B 完成只表示私有技术样机，不表示公网许可、全面合规或可用于自动投资决策。
