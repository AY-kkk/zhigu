# 投资事件情报与证据时间线交付证据

本目录保存每次 `verify-intel.sh` 运行生成的 `manifest.json`。manifest 的状态是证据状态，不是自动上线结论：

- `offline/integration`：确定性规则、迁移、API、fixture replay、Vite build、Intel Chromium UI 可重复验证。
- `live-data/live-model`：缺授权/凭据时明确 `blocked`，不把 fixture 当真实供应商或模型结果。
- `user_validation/performance/public_url`：必须补充真实用户任务记录、负载报告和外部可操作 URL 后才能签收。

发布前必须把真实 provider 的脱敏 `tools/list`/Schema、request_id、样本、权限依据、备份恢复演练、回退步骤和 60–180 秒演示视频加入对应 run 目录，并由用户确认。
