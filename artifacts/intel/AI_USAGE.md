# AI 使用与验证记录

- AI/LLM 角色：只用于实体、命题、模态、参数、引用跨度和候选关系的抽取建议；不直接决定归并、证据分级、三轴状态、支持度、冲突解决或投资结论。
- 确定性规则：`contracts/intel/rules-v1.json` 与 `app/server/service/intel`；S1–S8/D 的 expected.json 是人工标注金标准，不由待测引擎生成。
- 验证：`service/intel/rules_test.go`, `identity_test.go`, `time_test.go`, `fixture_test.go`；API/隔离由 `api/v1/intel/api_test.go` 覆盖。
- 边界：没有真实模型凭据时不运行 live-model；没有授权数据时不宣称真实数据接入；任何 AI 原文/输出均按 rights 限制展示。
