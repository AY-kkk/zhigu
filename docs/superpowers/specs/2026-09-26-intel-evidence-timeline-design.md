# 投资事件情报与证据时间线设计

日期：2026-09-26。基线：PRD v1.2 与 SPEC I-1.0；本文记录实施设计，不改变已冻结业务语义。

## 目标与边界

在与“投研观点”“交易策略”同级的独立 `/app/intel` 窗口中，完成 A 股收购、业绩预告、监管立案三类事件的关注、事件流、证据时间线、冲突/版本、通知和隔离回放。聊天、行情验证、外部推送、交易建议不在本期。

## 架构

1. **纯规则域**：`app/server/service/intel` 将证据权重、来源簇、冲突、三轴状态、新鲜度和事件身份计算实现为可重复纯函数；`rules-v1.json` 是规则版本。
2. **持久执行**：`finance_intel_*` 表保存 namespace、来源修订、证据、快照、变更、通知、任务、审计和幂等；演示与 live namespace 隔离。
3. **API**：Gin 注册 `/api/finance/intel/v1`，固定 Intel envelope、demo/live Scope、Cookie/Mode/Origin 校验和24个冻结路由。
4. **前端**：Vue/Pinia 独立 `IntelLayout` 与 `/app/intel`/`/app/intel/demo` 路由；旧研究/策略状态不在该窗口初始化。主导航通过 `window.open(..., noopener)` 打开新窗口。
5. **fixture**：`contracts/intel/fixtures/events-v1` 保存人工金标准 S1–S8/D。回放提交写入快照、变更和 Outbox/通知；reset 增加 generation，旧写回失效。

## 关键决策

- 传闻数量不升级支持度；支持/反驳分别按根来源簇最大值再取簇间最大值，不求和。
- 同级明确更正可替代旧参数；同级冲突不按抓取先后择一，保持 open 直到有明确替代。
- 新鲜度只在覆盖完整时推进；14/30天边界分别进入 stale/expired，缺时间或覆盖缺口为 unknown。
- live 请求只接受 JWT；demo 请求只接受签名 Cookie，夹带真实令牌返回 `MODE_CONFLICT`。
- provider/模型缺授权时 fail-closed，不用 fixture 填充 live，也不生成“已通过”记录。

## 验收分层

- 确定性层：规则、身份、时间、events-v1 金标准、迁移和 API 测试。
- 持久层：embedded PostgreSQL 的 demo session/replay/reset/通知隔离测试。
- 浏览器层：Chromium fixture 流程和375px冒烟；Firefox/WebKit/Edge仍待目标环境矩阵。
- 外部层：授权真实数据、真实模型、负载、5名用户理解、公开URL和回退演练必须单独提供证据。

## 完成判定

只有 PRD AC01–AC13 对应证据齐全且无事实错误、隔离或漏通知阻断项，才可签收真实上线。fixture 绿测、构建绿测或截图只证明对应层级。
