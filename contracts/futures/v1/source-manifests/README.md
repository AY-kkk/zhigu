# Source manifests

每个真实来源必须在 `<source_id>/<version>.json` 登记并通过 `SourceManifest` schema、权利审计和来源探针后才能激活。当前没有已授权的真实来源，因此本目录不放置可被生产加载的 manifest，也不把离线 fixture 改名为 live source。

登记文件至少包含 publisher、canonical URL、credential reference（只写 secret 名称）、rights、metrics、发布日历版本、adapter 版本、覆盖范围、限流和分页规则。`source_type=fixture` 只能用于测试适配器，worker 的 live registry 会拒绝它。
