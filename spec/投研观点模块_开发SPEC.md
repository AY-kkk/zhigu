# 投研观点模块 开发 SPEC

版本：V1.0  
日期：2026-09-27  
产品基线：[投研观点模块 PRD](../prd/投研观点模块_PRD.md)  
PRD SHA-256：`7dbd020a75548a9996e6a93a424112139e093d36437ce66a59afac3064521def`  
状态：开发合同；实现与验收状态必须由测试、manifest 和用户签收另行记录

---

## 1. 目标与范围

实现“手动粘贴观点 / 上传研报 / 两者同时提交”到“生成可追溯质证报告”的完整 MVP。

固定证据源：

1. 行情；
2. 财务三表及结构化财务指标；
3. 公司公告和交易所披露；
4. 用户上传研报。

不在本期实现文章入口、任意互联网搜索、多研报比较、OCR、智能追问或工作区保存。

---

## 2. 术语与不变量

| 术语 | 定义 |
|---|---|
| Claim | 用户观点或研报中抽取的待核验主张 |
| Fact Check | 对单条主张的成立/被前置/不成立/存疑判定 |
| Evidence | 本次研究中登记且可定位的来源记录 |
| Target Report | 被质证的上传研报 |
| Source Report | 可直接支持或挑战结论的上传研报证据 |
| Run | 一次不可变输入快照对应的研究任务 |
| Publish | 通过程序校验后对外可见的报告版本 |

全局不变量：

- 观点文本和研报至少提交一项。
- 正式 Run 创建前必须经过用户确认。
- 创建 Run 后输入不可原地修改；修改产生新 Run。
- 事实和推演必须引用本次 Run 登记的 Evidence。
- 用户研报可以直接支持结论，但必须标为 `user_report` 且不得伪装为官方披露。
- 未找到证据只能产生 unknown，不得生成虚构事实。
- fixture、mock、截图不得标记为 live 或真实验收。
- incomplete 报告不得包含总体 verdict。

---

## 3. 领域模型

### 3.1 输入状态

```text
DRAFT
  -> PARSED
  -> CONFIRMED
  -> RUN_CREATED
```

`draft_id + revision` 唯一确定一次解析快照。文本、上传文件、标的或期限变化都会使旧 revision 失效。

### 3.2 证据等级

```text
official_filing  官方公告、年报、交易所披露
structured_data 行情、财务三表、结构化指标
user_report     用户上传研报
```

### 3.3 主张类型

```text
fact       可被来源直接核验
inference  需要传导链或计算
assumption 明确假设、判断或前提
```

### 3.4 单条事实核验状态

```text
supported           成立
prerequisite_missing 被前置
contradicted        不成立
uncertain           存疑
```

### 3.5 总体结论

```text
supported         支持
partially_supported 部分支持
challenged        被挑战
insufficient      证据不足
```

---

## 4. 数据库合同

新增迁移 `app/server/migrations/finance/007_research_viewpoint.sql`。

### 4.1 研报文件

```sql
CREATE TABLE IF NOT EXISTS finance_research_documents (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  draft_id TEXT,
  run_id TEXT,
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL CHECK (media_type IN ('application/pdf','application/vnd.openxmlformats-officedocument.wordprocessingml.document','text/plain')),
  byte_size BIGINT NOT NULL CHECK (byte_size > 0 AND byte_size <= 20971520),
  content_hash TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  extraction_status TEXT NOT NULL CHECK (extraction_status IN ('queued','succeeded','failed')),
  extracted_text TEXT,
  extraction_error TEXT,
  page_count INTEGER,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ,
  UNIQUE(owner_id, content_hash)
);
CREATE INDEX research_documents_owner_created ON finance_research_documents(owner_id, created_at DESC);
```

### 4.2 研报定位

```sql
CREATE TABLE IF NOT EXISTS finance_research_document_spans (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES finance_research_documents(id) ON DELETE CASCADE,
  page_number INTEGER,
  paragraph_index INTEGER,
  start_offset INTEGER NOT NULL,
  end_offset INTEGER NOT NULL,
  text TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CHECK (start_offset >= 0 AND end_offset > start_offset)
);
CREATE INDEX research_document_spans_document ON finance_research_document_spans(document_id, page_number, paragraph_index);
```

### 4.3 质证报告扩展

```sql
ALTER TABLE finance_claim_drafts
  ADD COLUMN IF NOT EXISTS source_mode TEXT NOT NULL DEFAULT 'claim_only',
  ADD COLUMN IF NOT EXISTS document_id TEXT,
  ADD COLUMN IF NOT EXISTS focus_text TEXT;

ALTER TABLE finance_research_runs
  ADD COLUMN IF NOT EXISTS document_id TEXT,
  ADD COLUMN IF NOT EXISTS input_mode TEXT NOT NULL DEFAULT 'claim_only';

CREATE TABLE IF NOT EXISTS finance_claim_fact_checks (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES finance_research_runs(id) ON DELETE CASCADE,
  claim_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('supported','prerequisite_missing','contradicted','uncertain')),
  reason TEXT NOT NULL,
  evidence_ids JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE(run_id, claim_id)
);
```

`VerifiedReport` 新增字段：

```go
FactChecks    []FactCheck    `json:"fact_checks"`
Challenges    []Challenge    `json:"challenges"`
ReasoningGaps []ReasoningGap `json:"reasoning_gaps"`
TailRisks     []TailRisk     `json:"tail_risks"`
TestConditions []TestCondition `json:"test_conditions"`
EvidenceIndex []EvidenceRef  `json:"evidence_index"`
```

---

## 5. API 合同

统一响应仍使用 `{error,data,trace_id}`。所有 consumer 路由必须经过登录和 owner 校验。

### 5.1 上传研报

```http
POST /api/finance/research-documents
Content-Type: multipart/form-data
```

请求字段：`file`、可选 `draft_id`。

成功响应：

```json
{
  "document_id": "doc_...",
  "filename": "report.pdf",
  "media_type": "application/pdf",
  "byte_size": 123456,
  "content_hash": "sha256...",
  "extraction_status": "queued",
  "page_count": null
}
```

拒绝条件：

- 未登录：401 `UNAUTHENTICATED`
- 类型不支持：400 `UNSUPPORTED_DOCUMENT`
- 大于 20 MB：413 `DOCUMENT_TOO_LARGE`
- 内容哈希重复：返回既有 document，不重复存储
- 提取失败：document 保留，`extraction_status=failed`

### 5.2 查询提取状态

```http
GET /api/finance/research-documents/:id
```

返回提取状态、页数、可用 span 数；不得返回其他用户的文件。

### 5.3 解析草稿

```http
POST /api/finance/claims/parse
```

请求：

```json
{
  "text": "可选观点文本",
  "document_id": "可选研报ID",
  "focus_text": "可选聚焦指令"
}
```

约束：

- `text` 与 `document_id` 至少一项。
- `text` 存在时为 20–2,000 字。
- `document_id` 必须属于当前用户且提取成功。
- 响应必须包含 `input_mode`、`items`、`candidates`、`horizon`、`document_id`、`needs_confirmation=true`。

`input_mode`：

```text
claim_only
report_only
claim_and_report
```

### 5.4 确认与创建研究

保留：

```http
PATCH /api/finance/claims/:id
POST /api/finance/research
```

PATCH 可修改 `instrument_id`、`horizon_start`、`horizon_end`、`items`，但修改 `text` 或 `document_id` 必须重新解析。创建请求仍使用 `Idempotency-Key`，不得由客户端提交 `as_of`。

### 5.5 查询研究

```http
GET /api/finance/research/:id
```

`data` 必须包含：

```json
{
  "run_id": "run_...",
  "status": "completed",
  "input_mode": "claim_and_report",
  "claim": {"text": "...", "items": []},
  "document": {
    "document_id": "doc_...",
    "filename": "report.pdf",
    "page_count": 18,
    "source_role": ["target", "evidence"]
  },
  "report": {
    "schema_version": "research-report.v2",
    "summary": "...",
    "verdict": "partially_supported",
    "fact_checks": [],
    "challenges": [],
    "reasoning_gaps": [],
    "tail_risks": [],
    "test_conditions": [],
    "evidence_index": []
  }
}
```

### 5.6 报告下载

```http
GET /api/finance/research/:id/export?format=html
```

返回 `text/html` 单文件报告，必须：

- 无远程脚本；
- 外链使用 `rel="noreferrer noopener"`；
- 内嵌最小 CSS；
- 包含七块和免责声明；
- incomplete 报告不得包含总体 verdict。

---

## 6. 解析规则

解析器必须输出结构化 `ClaimParseResult`：

```go
type ClaimParseResult struct {
    InputMode string
    InstrumentCandidates []Instrument
    Items []ClaimItem
    HorizonStart string
    HorizonEnd string
    Numbers []NumberMention
    NeedsConfirmation bool
}
```

规则优先级：

1. 用户明确指定标的；
2. 文本中唯一证券代码或公司名；
3. 研报封面/首页唯一标的；
4. 返回多个候选，禁止猜测。

研报解析必须保留页码/段落定位。模型输出只能作为候选，必须经过 Schema、数字、定位和来源校验后落库。

---

## 7. 研究执行

### 7.1 两个角色

- supporter：只寻找支持证据；
- challenger：只寻找反证、替代解释和遗漏风险。

任务上下文隔离。角色只能调用：

```text
get_market_data
get_financials
search_filings
read_document_spans
calculate_metric
```

禁止任意网页、新闻、代码执行、长期记忆和跨 Run 证据复用。

### 7.2 用户研报

`read_document_spans` 可读取当前 document 的页码和段落。研报可以：

- 提供事实证据；
- 提供被挑战观点；
- 提供需要外部核验的引用线索。

系统必须对每条研报事实记录 `verification_status=independent_verified|reported_only`。`reported_only` 可支持结论，但界面必须显示“研报陈述，未独立核验”。

### 7.3 合成

合成器必须生成 `research-report.v2`，禁止把固定假设或固定 change condition 写入所有研究。每条输出必须绑定 claim_id 和 evidence_ids。

程序裁决只使用已登记证据与明确规则，不得用模型置信度覆盖 `contradicted` 或 `prerequisite_missing`。

---

## 8. 发布门禁

发布前必须全部通过：

1. Schema 校验；
2. Run、owner、document 归属校验；
3. Evidence ID 白名单；
4. fact/inference 引用覆盖；
5. 数值、单位、期间匹配；
6. 发布日期与 `as_of` 时间闸门；
7. 七块结构非空规则；
8. incomplete 无总体 verdict；
9. 用户研报来源等级可见；
10. 固定免责声明存在。

失败时返回具体 pointer 和原因，不发布部分可信内容为 completed。

---

## 9. 前端合同

### 9.1 输入区

`/app/research/new` 支持：

- 文本输入；
- PDF/DOCX/TXT 上传；
- 两种输入可组合；
- 显示文件解析状态和失败重试；
- 文本或文件变化后清除旧解析结果。

### 9.2 确认卡

展示：

- input mode；
- 标的候选；
- 主张列表及类型；
- 研报名称和来源角色；
- 研究期限；
- 数据截止时间；
- 预计额度/成本；
- “确认并开始研究”。

### 9.3 报告页

固定展示七块：

1. 核心判断；
2. 事实核验表；
3. 逐条质疑；
4. 推理链缺口；
5. 被忽略的风险；
6. 证实与证伪条件；
7. 证据清单。

引用按钮打开证据抽屉，显示原文定位和 `verification_status`。页面常驻免责声明。

### 9.4 状态

必须区分：

- 文件解析中；
- 等待确认；
- 排队；
- 研究中；
- 核对中；
- 已完成；
- incomplete；
- 失败；
- 取消中/已取消。

---

## 10. 安全、隐私与合规

- 文件仅限当前 owner 读取；删除 Run 不自动删除用户文档。
- 下载链接必须再次做 owner 校验，不能仅依赖不可猜测 ID。
- 文本提取结果按私人数据保存，不进入日志。
- 文件名、模型输出和上传文本视为不可信输入。
- HTML 导出禁止内联脚本和远程资源。
- 复制或导出前清除电话、邮箱等非必要个人信息。
- 报告常驻“本报告仅供研究参考，不构成投资建议”。
- 用户研报仅短摘录，不导出大段全文。

---

## 11. 测试矩阵

| ID | 层级 | 必须覆盖 |
|---|---|---|
| RV-01 | Go unit | 三种 input_mode 与缺输入拒绝 |
| RV-02 | Go unit | 文件类型、20 MB、重复 hash、owner 隔离 |
| RV-03 | Go integration | PDF/DOCX/TXT 提取与 span 定位 |
| RV-04 | Go unit | 观点/研报主张抽取 Schema 与数字口径 |
| RV-05 | Go unit | revision 变更、幂等创建、as_of 不可伪造 |
| RV-06 | Python unit | supporter/challenger 隔离和工具 allowlist |
| RV-07 | Python unit | `read_document_spans` 只读当前 document |
| RV-08 | Python unit | reported_only 与 independent_verified 标签 |
| RV-09 | Go unit | fact_checks 四态与总体四态裁决 |
| RV-10 | Go unit | 发布门禁拒绝无引用、错数字、未来证据 |
| RV-11 | Go integration | incomplete 无总体 verdict |
| RV-12 | Vue unit/component | 上传、解析、确认、旧结果失效 |
| RV-13 | Playwright | 三种输入的 UI 流程 |
| RV-14 | Playwright + Go/PG/worker | 真实链路生成七块报告 |
| RV-15 | Go unit | HTML 单文件、无脚本、免责声明 |
| RV-16 | Eval | 50 条人工样本和质量指标 |

Mock UI 通过只能覆盖 RV-12/13，不得替代 RV-14/16。

---

## 12. 验收门槛

### 12.1 功能

- 三种输入均可完成 Run；
- 研报可同时作为 target 和 evidence；
- 七块报告完整；
- 每条 fact/inference 可回到证据；
- 修改输入必须重新解析；
- 取消、失败、重试、重新研究可用。

### 12.2 质量

- 50 条人工评测；
- 事实错误率 ≤ 3%；
- 击中要害评分 ≥ 4/5；
- 可溯源论据 ≥ 90%；
- 链接有效率 ≥ 95%；
- 数字/单位/期间错误 = 0；
- 编造来源/原文 = 0。

### 12.3 性能

- 文本解析 P90 ≤ 2 秒；
- 首个过程事件 P90 ≤ 5 秒；
- 普通文本或可提取文本研报端到端 P90 ≤ 90 秒；
- OCR 不计入该 P90。

### 12.4 工程证据

- 真实 Vue → Go → PostgreSQL → connector/worker → 报告；
- fixture、mock、live、人工评测分别记录；
- 缺凭据写 `blocked` 或 `unverified`；
- manifest 缺项、必需 skip 或证据文件不存在时必须失败。

---

## 13. 完成定义

只有以下条件全部满足才可交付：

1. RV-01 至 RV-15 全部通过；
2. RV-16 达到质量门槛；
3. 真实数据与真实模型证据齐全；
4. 公共树检查通过；
5. README 记录启动、模式和限制；
6. 用户完成最终签收；
7. 开发者不得自行标记 `accepted`。
