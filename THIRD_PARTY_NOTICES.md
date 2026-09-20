# 第三方组件与来源说明

本仓库通过包管理器引用第三方依赖；其许可不因本仓库许可状态改变。发布二进制、镜像或打包第三方代码时，需要随分发物保留对应许可证和版权说明。

| 组件 | 用途 | 上游许可 |
| --- | --- | --- |
| [Eino](https://github.com/cloudwego/eino) | Go 编排；版本见 `app/server/go.mod` | Apache-2.0 |
| [DeerFlow](https://github.com/bytedance/deer-flow) | 可选研究 Harness extra；版本见 `app/research-service/uv.lock` | MIT |
| [Gin](https://github.com/gin-gonic/gin) | HTTP 框架 | MIT |
| [GORM](https://github.com/go-gorm/gorm) | 数据访问 | MIT |
| [Vue](https://github.com/vuejs/core) | 前端 | MIT |
| [Element Plus](https://github.com/element-plus/element-plus) | UI 组件 | MIT |
| [semi-design-vue](https://github.com/rashagu/semi-design-vue) / `@kousum/semi-ui-vue@2.78.4` | 面客 UI 组件；版本见 `app/web/package-lock.json` | MIT |
| `@kousum/semi-icons-vue@2.78.0` | 面客 UI 图标 | MIT |
| `@douyinfe/semi-foundation` / `@douyinfe/semi-theme-default@2.78.0` | Semi Vue 组件基础与默认主题依赖 | MIT |
| [FastAPI](https://github.com/fastapi/fastapi) | Python 内部 API | MIT |

完整依赖版本由 `app/server/go.mod` / `go.sum`、`app/research-service/uv.lock` 和 `app/web/package-lock.json` 记录；上表不是全部传递依赖清单。

`references/` 不打包外部源码检出。`app/gosaas/` 和 `references/gosaas/` 是本地私有副本，未找到公开许可证，本次公开发布不包含这些目录；既有本地使用授权不被解释为向公众再分发授权。阶段 A 的独立宿主无需该代码即可运行。

页面截图来自本项目本地 fixture 环境。
