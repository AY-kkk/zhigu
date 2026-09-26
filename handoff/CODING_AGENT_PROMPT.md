# 直接交给编程 Agent 的启动指令

你接手的是“知股：投资观点证据与反证助手”一期。请先阅读工作区的：
1. prd/金融C端Agent_MVP_PRD.md（v2）
2. spec/金融C端Agent_面客产品架构_v2.md
3. spec/开发交付_SPEC.md
4. handoff/contracts/ 与 handoff/acceptance-cases.json

工作区：当前仓库根目录。不要把开发机的绝对路径写入公开文档、日志或提交信息。

目标：按 SPEC 的 T0–T7 实现可启动的面客 MVP；GoSaaS 做业务和 Vue 客户/后台，Eino 是 Go 内唯一总编排，DeerFlow Harness 在内部 Python 执行 supporter/challenger 两个隔离的有界 Agent。不要用 mock/自写 Loop 冒充实际 DeerFlow，不另做 Next.js 客户端，不做一期自选股/长期偏好记忆。

先核验本地仓库 SHA、授权和运行依赖；references/ 不修改。应用放 app/，如目录已有内容先检查和保留。GoSaaS 特有代码授权未确认时，不将它纳入可分发改造工程；可先开发独立契约/测试/新模块。不得静默换掉 GoSaaS。仓库外的私有 GoSaaS 来源目录不在修改范围。

遵循每项任务先失败测试→最小实现→通过测试。优先证明真实 Harness 工具白名单、上下文隔离、模型配置注入与共享预算，然后接 UI。不要把问题推给另一个通用 Supervisor。每个阶段在 app/IMPLEMENTATION_STATUS.md 记录文件、命令、退出码、fixture/live 状态与阻塞原因。

后台必须能保存、测试和启用模型/数据源配置，Key 只写不回显，所有 run 冻结版本。金融数字由结构化数据/确定性计算给出；引用、时间、数字、owner、预算、取消与删除均为后端硬约束。缺少来源时返回证据不足，不编造反方观点或来源。

缺模型或金融数据凭据时使用清楚标记的 fixture 完成可测部分，并注明只达到阶段 A；不要伪造真实数据接入或上线资格。任何外部购买、部署、推送 GitHub、交易执行均不在授权范围。

最终交付 app/ 源码、依赖锁、迁移、启动说明、测试与 E2E 证据、模型后台配置步骤、真实数据覆盖说明和未完成清单。当前 handoff 测试只验证交接材料，不能当作应用通过测试。
