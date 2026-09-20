# 金融投研 Agent 免费/开源 API 清单

**核验日期：** 2026-09-18
**适用范围：** 当前仓库的阶段 A / 阶段 B 规划
**结论：** 阶段 A 不需要马上接入真实模型或大规模 RAG。应先把数据源封装成受控 connector，并保留来源、定位、发布日期、抓取时间、内容哈希和数据版本。免费不等于可商用、可再分发或长期稳定。

## 一、按当前 Agent 需求分组

| 能力 | 推荐方案 | 免费/开源属性 | API 地址 | 当前优先级 |
|---|---|---|---|---|
| 中国 A 股行情、财务、公告 | AKShare；Tushare 作为备用 | AKShare 是 MIT 开源适配器；Tushare 是有积分/权限的免费额度服务 | [AKShare 文档](https://akshare.akfamily.xyz/)；Tushare HTTP `http://api.tushare.pro` | P0 |
| 美国上市公司申报与 XBRL | SEC EDGAR APIs | 官方公开 API，无 API key；需合规 User-Agent 和速率控制 | `https://data.sec.gov/submissions/CIK##########.json`；`https://data.sec.gov/api/xbrl/companyfacts/CIK##########.json` | P0 |
| 宏观、国家指标 | World Bank Indicators API | 无 API key | `https://api.worldbank.org/v2/country/{country}/indicator/{indicator}?format=json` | P0 |
| 汇率、欧洲宏观 | ECB SDMX REST API | 公开访问 | `https://data-api.ecb.europa.eu/service/data/...` | P1 |
| 美国宏观时间序列 | FRED API | 免费注册 API key | `https://api.stlouisfed.org/fred/series/observations` | P1 |
| 全球新闻/事件检索 | GDELT DOC 2.0；自托管 SearXNG | GDELT 公共检索 API；SearXNG 开源自托管 | `https://api.gdeltproject.org/api/v2/doc/doc`；自托管 `http://<searxng>/search?q=...&format=json` | P0/P1 |
| 文章/PDF 转文本 | Apache Tika；GROBID；Jina Reader（云端便利层） | Tika/GROBID 开源自托管；Jina 有免费限速 | Tika `http://localhost:9998/tika/text`；GROBID `http://localhost:8070/api/...`；Jina `https://r.jina.ai/<URL>` | P1 |
| 本地模型推理 | Ollama；llama.cpp；vLLM | 开源/本地运行；模型权重和算力另计 | Ollama `http://localhost:11434/api`；llama.cpp `http://localhost:8080/v1`；vLLM `http://localhost:8000/v1` | 阶段 B |
| 临时外部模型测试 | OpenRouter 免费模型路由 | 免费模型和免费额度会变；需 API key、隐私和限额控制 | `https://openrouter.ai/api/v1/chat/completions`；模型列表 `https://openrouter.ai/api/v1/models` | 阶段 B/开发 |
| 向量检索 | PostgreSQL + pgvector；Qdrant 备用 | 开源自托管 | pgvector 使用 PostgreSQL SQL；Qdrant `http://localhost:6333` | P1，阶段 A 可不接 |

## 二、最适合当前项目的最小组合

### 1. 阶段 A：先保持 fixture，准备 connector

阶段 A 的验收重点是工具权限、预算、证据登记、Decimal 计算和可恢复状态，不是接入真实 LLM。建议只实现以下 connector 接口和离线 fixture：

1. **A 股：** AKShare 作为 Python adapter，统一输出 `symbol / metric / value / unit / period / source_url / published_at / retrieved_at / data_version`。重要财务字段增加 Tushare 或公告原文二次核验。
2. **全球申报：** SEC submissions + companyfacts，原始 JSON 落盘后再解析，不能只保存模型摘要。
3. **宏观：** World Bank 起步；需要汇率或欧洲数据时加 ECB；美国宏观再加 FRED。
4. **新闻：** GDELT 做事件检索；生产环境优先自托管 SearXNG 或接授权新闻源，避免把随机公共搜索实例当作稳定依赖。
5. **证据：** 每个外部结果写入 evidence registry，至少包含 `source_id、source_kind、source_url、locator、published_at、available_at、retrieved_at、content_hash、data_version`。

### 2. 阶段 B：模型与 RAG

- 本地开发优先 Ollama 或 llama.cpp；有 GPU 部署再考虑 vLLM。这样不会把金融材料默认发送给第三方模型。
- RAG 先用现有 PostgreSQL 的全文检索；需要 embedding 时优先 pgvector。当前规格不需要一开始引入 Qdrant、GraphRAG 或复杂编排。
- OpenRouter 只用于阶段 B 的模型能力对比和联调，必须固定模型 ID、限制预算、记录 provider/model/usage，并在发送材料前做脱敏和数据保留审查。

## 三、免费条件和限制核验

### 公开或无 key

- [SEC EDGAR API](https://www.sec.gov/search-filings/edgar-application-programming-interfaces)：`data.sec.gov` 提供 submissions 和 XBRL JSON；不需要 API key，但要求有效 User-Agent，速率限制可能调整。
- [World Bank Indicators API](https://datahelpdesk.worldbank.org/knowledgebase/articles/889392)：Indicators API v2 可直接调用，不需要认证，适合国家和宏观上下文。
- [ECB Data API](https://data.ecb.europa.eu/help/api/data)：SDMX REST 服务可程序化读取欧洲央行数据，部署时仍需自行验证访问频率和服务条款。
- [GDELT DOC 2.0](https://blog.gdeltproject.org/gdelt-doc-2-0-api-debuts/)：提供全文新闻检索；它是发现线索的来源，不能直接替代已核验的一手公告。

### 免费但需 key、积分或开发限制

- [FRED API key](https://fred.stlouisfed.org/docs/api/fred/v2/api_key.html)：需注册并申请 key；适合美国宏观时间序列。
- [Tushare 积分与权限](https://tushare.pro/document/1?doc_id=290)：部分日线能力在低积分门槛下可用，但不同接口权限、频率和字段不同；它不是一个无条件开放的开源数据服务。
- [Alpha Vantage 支持页](https://www.alphavantage.co/support/)：免费 key；标准免费服务有日请求限制，开放源码/教育项目的更高额度需要符合条件并申请。
- [Twelve Data 定价](https://twelvedata.com/pricing)：Basic 免费额度按分钟和日请求计费，部分接口按 credits 加权；适合开发和内部验证，商业展示/再分发前要核对许可。
- [The Guardian Open Platform](https://open-platform.theguardian.com/access/)：非商业免费 key，有请求频率和每日额度；商业用途需要另行许可。
- [NewsAPI Pricing](https://newsapi.org/pricing)：免费 developer 计划明确面向开发/测试，存在日额度和时效限制，不应直接当作生产新闻源。

## 四、开源自托管组件

- [AKShare 文档](https://akshare.akfamily.xyz/)：开源金融数据接口，覆盖股票、基金、债券、期货、宏观等；它聚合上游网站，接口和上游页面可能变化，因此必须做缓存、失败重试和来源快照。
- [OpenBB Platform](https://docs.openbb.co/platform/developer_guide)：开源数据连接/统一层，不等于免费数据本身；每个 provider 仍有自己的 key、许可和稳定性。
- [SearXNG Search API](https://docs.searxng.org/dev/search_api.html)：自托管元搜索 API；公共实例可能关闭 JSON 或限制访问。
- [Apache Tika Server](https://tika.apache.org/docs/4.0.x/using-tika/server/index.html)：自托管 PDF/Office 文本提取，适合将原始材料送入证据管线。
- [GROBID](https://grobid.readthedocs.io/en/latest/Grobid-service/)：自托管学术/结构化 PDF 解析，成本和运维复杂度高于 Tika。
- [pgvector](https://github.com/pgvector/pgvector)：PostgreSQL 向量相似度扩展，与当前数据库架构最顺；建议在全文检索验证后再启用。
- [Qdrant API](https://qdrant.tech/documentation/quick-start/)：独立向量数据库备用方案，默认本地端口 `6333`。
- [Ollama API](https://github.com/ollama/ollama/blob/main/docs/api/introduction.mdx)、[llama.cpp server](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md)、[vLLM OpenAI-compatible server](https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/)：本地或自托管模型 API；软件可免费使用，但模型许可证、显卡、存储和运维不是免费的。

## 五、不要在当前 P0 直接依赖的方案

1. **随机公共 SearXNG 实例、未固定版本的网页抓取、非官方行情接口：** 结果不可审计、字段会漂移，无法满足证据链。
2. **NewsAPI/Guardian 免费计划作为生产新闻主源：** 免费条件本身有开发、商业和速率边界。
3. **把 OpenRouter 免费模型当生产金融结论引擎：** 免费模型路由、限额、模型版本和数据处理政策可能变化；只能作为阶段 B 联调或对比基线。
4. **一开始引入 Qdrant/GraphRAG：** 当前规格的最小 RAG 只要求一手正文加供应商检索或小型 PostgreSQL 全文检索，先验证证据召回和引用定位。

## 六、建议的接入顺序

1. `AKShare -> 受控 Python connector -> Go evidence registry`，先覆盖 A 股查询和三类财务表。
2. `SEC + World Bank + ECB`，形成公开、可复核的全球申报和宏观来源。
3. `GDELT + Tika`，把新闻线索和 PDF 原文纳入 evidence snapshot。
4. 阶段 B 再接 `Ollama/llama.cpp`，执行真实 tool-calling 和引用回答评测。
5. 最后根据规模决定 `pgvector` 或 Qdrant；不要先为“可能需要”引入额外服务。

## 七、接入验收标准

- 每个 connector 都能返回原始响应、规范化结果、来源 URL、定位信息和抓取时间。
- 同一查询重复执行时，能区分“数据变化”“解析变化”“模型变化”。
- 外部服务超时、字段缺失、权限不足时，Agent 必须返回“证据不足/暂不可核验”，不能用模型自报结果填空。
- 重要指标至少一手来源 + 一份独立交叉来源；新闻只能作为线索，不能替代公告、财报或监管文件。
- 为每个服务配置超时、重试、限流、缓存、API key 注入和费用上限；key 不进仓库、不写日志。
