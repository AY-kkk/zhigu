# 视觉 QA 结论

- 范围：`/app/intel/demo`，演示空态与 events-v1 回放后的详情态；1440×900、1700×1000、375/390×812/844。
- 结果：partial。`visual_layout_audit.mjs` 三个视口无横向溢出、重叠或断图；桌面/移动截图人工复核通过。
- 状态证据：Playwright fixture 流程覆盖关注→回放→事件详情→证据/时间线/冲突/变化；DOM 同时包含事件卡与 `.intel-detail-main`，其桌面几何为 x=80、y=879、982×984。
- 未验证：Firefox/WebKit/Edge 原生、真实后端数据密度、200 条证据性能、键盘/读屏完整审查。它们不得由 Chromium fixture 结果替代。


## 2026-09-26 复核更新

- 新增演示“开始前不创建会话”状态后，1700/1440/390 三视口 `visual_layout_audit.mjs` 为 0 error；桌面 section 覆盖有 2 个 warning，移动 section 覆盖完整。
- 新增窗口动作、安全登录回跳、跨窗口 storage 重鉴权后，Playwright Intel Chromium 5/5 通过。
- 仍未验证：Firefox/WebKit/Edge 原生矩阵、真实后端密度、10 RPS 负载、5名用户理解。