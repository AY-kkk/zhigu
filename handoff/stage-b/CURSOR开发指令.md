# 交给 Cursor / 编程 Agent 的阶段 B 指令

请按 [阶段 B SPEC B-1.6](../../阶段B_开发SPEC.md) 开发。文档不代表应用或真实调用已通过。

**拍板以 SPEC §2 为准。** 逐步动作以 SPEC §3 为准。测试矩阵以 [验收与接入清单](验收与接入清单.md) 为准。

**产品一句话：** 交付观点裁判型研报——审理用户确认过的主张，给出四态判断、支持、反证、带前缀的未知项与可回源证据。不是聊天结论，也不是卖方深度/评级/目标价。

- 双协议：Chat Completions **与** Responses adapter 都必须实现；live **可以且应当**兼容 Responses（有凭据则纳入样本，不是只做 Chat）。
- 数据：接东财/新浪 HTTP + 巨潮；AKShare 只允许出现在 Go 校验 grant **之后**的 connector（或锁定版本的 `app/data-connector`）。研究工具禁止 `import akshare`、禁止直连外网。Tushare 积分不是开工条件。
- 底座：继续独立宿主 `app/server` + `finance_users`。**不要**把缺少 GoSaaS 当 G0 阻塞，也**不要**擅自迁 GoSaaS。

## 阅读顺序

只读这三份，不要为开工再读 PRD / 架构 v2 / 总 SPEC（与 §2 冲突时以 §2 为准，读旧文容易走偏）：

1. [阶段 B SPEC B-1.6](../../阶段B_开发SPEC.md) §2、§3、§5～9。
2. [验收与接入清单](验收与接入清单.md) D 清单与测试矩阵。
3. 当前 `app/` 代码与测试。

## 不得变更的任务

- 产品仍是「粘贴 A 股或港股单公司观点→人工确认→观点裁判型研报」，不是二期自选股/长期记忆，也不是机构研报工厂。
- 独立宿主管账号、权限、配置、预算、数据和发布；Eino 是唯一总 DAG；DeerFlow 真正执行两个独立、有界研究 Loop。
- 不新增平行用户体系、第二套后台、第二个 Supervisor；不凭仓库目录或 import 宣称框架已接入。
- Python 研究进程没有供应商真实 Key/业务 DB 权限；消费者不直连 Python；全部模型和数据请求经过 Go 的能力与预算出口。
- 后台 protocol 枚举为 `openai_chat_completions` / `openai_responses`。按 SPEC 第 7 节适配；禁止静默协议兜底或另建 Responses 研究循环。
- `data-query` operation 仅为 `get_financials` / `search_filings`。证据 POST 只收 `record_ids`。`growth_rate` 单位 `percent`。公开与内部 Schema **不加**口径字段。
- 冻结质量样本：`600519.SH`、`300750.SZ`、`000333.SZ`。live 覆盖目录为全 A 股+港股。verdict 按 SPEC §2.6。
- 不使用 fixture 兜底 live；不吞异常变成功报告；不展示未通过闸门的草稿。
- 不改 `references/`；保留用户未提交改动；不自动清理、重置、提交、推送或部署公网。
- 不因本指令自行启用并行子 Agent。
- 不把视觉改版、导出、S1/C1/U1 编号算进 G5。
- 八项补齐以 SPEC B-1.3 起的接口为准（草稿 PATCH、模型/as_of 冻结、数据契约、日期精度、逐主张、grant 重试、四类身份、按依赖推进）。
- **禁止**增加：MCP、浏览器/Computer Use、shell、`spawn_subagent`、Wind 等付费主路径、第 4 个工具、第 3 个研究角色、Hosted Agents 替换本架构、扩 `finance_report_checks` 字段。

## §2.7 在关口里怎么落地（不新开产品线）

| 做法 | 关口 | 文件 | 验收 |
|---|---|---|---|
| 角色提示词写成步骤 | G2.8 | `app/research-service/app/prompts/supporter.md`、`challenger.md` | Harness 能加载；最终 tools 仍为三个。不要用检查 Markdown 标题当通过 |
| 口径前缀 | G2.8 | Go 注入系统提示词，原文见 SPEC §2.7 | `ResearchTask` / `ExecutionContext` 不加字段 |
| 未知项正则 | G4.2 | 发布闸门 | `unknowns` 每条匹配 `^(无法取数\|口径不一致\|展望未绑定\|时间闸门\|正文不足)：.+` |
| 合成三条 | G4.2 | 合成提示词（可新建 `app/server/service/finance/prompts/synthesizer.md`） | 不新开角色、不扩 checks 表；结论仍走 §2.6 |
| 抽屉取数时间 | G5.4 | 现有证据 GET / `EvidenceDrawer` | 可见已有 `retrieved_at` |

## 第一轮必须交回什么

先读上面三份、识别基线，交回一页开工检查：

1. Git SHA、工作树改动摘要；启动入口必须是独立宿主。
2. G0 按 SPEC §3.1：A 回归是否已重跑。未跑则 G0=`not_started`。
3. D-01～D-07：哪些已拍板、哪些仍 blocked（Key、D-05）。不让用户把 Key 粘进对话。
4. B-ID 对现有/待新增文件的映射（双协议、header v2、record_id、growth_rate percent、§2.7 提示词与正则）。
5. 记录 G0.1，再按 §3 选下一步；已完成的不重复做。

编码以依赖检查通过为条件。G2/G3 可并行。无 Key 不挡受控编码。付费必须先有 G1.4/1.5/1.7。`accepted` 仅 YUFAN。

## 每个关口的工作节奏

1. 写出本关 B-ID、文件、输入/输出与断言。股票代码不得另选。
2. 先写会失败的测试并跑红。编译/网络失败是环境阻塞，不是业务红灯。
3. 最小实现。测真实路径与副作用，不用检查代码字符串、固定 true 冒充行为。未知项闸门测的是**发布后的报告字符串**，不是提示词全文。
4. 跑本关测试和受影响回归；状态/权限/预算用真实 PostgreSQL；Harness 查最终装配与 invoke。
5. 按证据包保存命令与退出码，更新 `app/IMPLEMENTATION_STATUS.md`。
6. 提交 `blocked` 或 `ready_for_review`。失败只阻断依赖项。

提示词只写本角色在三工具下如何取数、如何把缺数写成带前缀的未知、如何避免把展望写成事实。合成三条不是估值框架。

## 必须暂停并提出明确问题的情况

- 当前步骤需要真实模型，但未配置对应 Key。
- D-05 未写却被要求跑 live。
- 东财及已允许备用均不能满足 G1。验证备用并升 source version 不需重复选型；运行中不得切源。
- DeerFlow 装配做不到任务级模型注入、三工具白名单或中间件隔离。先报告位置与最小补丁，不能另写 Loop 冒名。
- 必须变更公开 Schema、Chat/Responses 之外的协议、硬预算、作用域、§2.6 结论表、§2.7 正则或质量阈值。
- 需要公开部署、购买 API、把私有数据发往额外第三方，或把 AKShare 放进研究工具进程。
- 为实现「更完整的研报」而需要 MCP、Wind、第 3 个角色、子 Agent、导出物，或增加 `data-query` operation。

**不要**因为旧文档提到 GoSaaS 而暂停。本阶段不迁。
**不要**把口径写进 Schema，或把合成三条写进 `finance_report_checks`。

## 每次汇报格式

```text
关口：Gx，步骤：Gx.y，状态：in_progress / blocked / ready_for_review
涉及需求：B-xx …
实际改动：文件与职责，不列未实现计划为成果
验证：命令、退出码、通过/失败/跳过数、证据路径
链路类型：受控离线 / 真实框架集成 / 真实供应商 / 浏览器live
协议：Chat 实现/集成/live ； Responses 实现/集成/live
数据：G1.2 三股样本是否已通（东财或新浪 + 巨潮）
交付物：本关是否仍是观点裁判型研报（是/否/本关无关）
范围偏离：无 / 列出新工具、新角色、新数据源或 Schema 字段
尚未证明：明确列出
阻塞与所需决定：如无则写无
下一步：SPEC §3 中的下一个编号步骤
```

阶段 B 完成由 SPEC 与验收矩阵决定，不以页面漂亮、测试总数或「接上模型」代替。两 adapter 受控测试必跑；缺某协议凭据则 live 未验证，实现不得留空。
