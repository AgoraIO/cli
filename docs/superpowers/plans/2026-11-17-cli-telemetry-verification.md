# CLI 埋点最终方案：本地实现与验收

2026-11-17；本记录覆盖本地实现与验收，不包含发布、部署或真实入库验证。提交与 PR 状态以 Git/GitHub 为准。

## 已实现

- 默认按保存偏好、运行时退出设置和合法采集地址决定采集；移除 consent 配置、询问和返回字段。首次告知只写 stderr，独立私有标记不重写 config.json；quiet 暂不告知。状态中的 enabled 表示保存偏好。
- 普通 CLI 使用启动快照；MCP 每次工具调用读取一次偏好，读取失败跳过采集。已开始的操作不重新读取配置。
- createProject 共享边界记录一次创建结果及其自身耗时、错误和退出码。Recipe/Quickstart、shell/MCP 均传递 operation；后续功能开通、查询失败不改写已经成功的创建事件。合法 32 位十六进制 App ID 立即保留，早期事件不回填；复用不记录创建。
- 首个事件懒启动当前二进制的隐藏发送入口，事件经有界 NDJSON 管道传递。父子各 20 条缓冲，单事件最多 4 KiB；满额丢新事件。每操作一个子进程、每父进程最多两个，异步回收；父进程最多等待 100 ms 本地交接。
- 子进程无业务环境、认证或配置读取，输出接空设备；独立 HTTP 客户端、不重定向、不重放、不重试。每请求一个事件、请求最多三秒，EOF 后最多三秒；业务运行期间没有三秒生命周期上限。
- SSP 同步严格 schema、command/event 匹配、app_id 校验和 OpenAPI。event_id 仅用于已有进程内去重；PostHog 不接收顶层 uuid 或 event_id 属性。原大小限制、限流、环境标记、禁用 person profile/GeoIP 保留。
- 帮助、生成命令文档、Telemetry、automation、llms、README、CHANGELOG 和 SSP 运维说明同步。

## 已执行的验证

| 检查 | 结果 |
| --- | --- |
| Go 全量 `go test ./... -count=1` | 通过 |
| 埋点与告知测试 `go test -race ...` | 通过 |
| 加强的父进程退出测试 | 通过；阻塞首条 HTTP 响应直至父进程退出，再确认后续阶段和最终事件收到 |
| `go vet ./...` | 通过 |
| `make lint` 的格式、错误码、生成帮助检查 | 通过；本机未安装 golangci-lint，按 Makefile 跳过该工具 |
| Linux amd64 / Windows amd64 交叉编译 | 通过；Windows 模块下载改用 Go 校验摘要的代理完成 |
| SSP BFF + contracts | 174 测试通过 |
| SSP OpenAPI + collector 定向测试 | 10 测试通过 |
| SSP 全仓 typecheck / build | 通过 |
| SSP 本次六个修改代码/协议文件 Biome | 通过 |
| SSP 全仓 Biome | 17 errors / 10 warnings，诊断均位于本次未改文件；没有顺带修改 |
| CLI → 实际 SSP 本地路由 → HTTP 模拟 PostHog | 通过，收到 10 条事件；检查默认采集、JSON/MCP、安装标识、操作隔离、App ID、创建结果、退出设置与无 uuid/event_id 转发 |

测试还覆盖创建认证失败、Token 刷新及 401 重发计时、创建后功能/查询失败、无效 App ID、MCP 损坏/null/错误类型配置、stdout 隔离、告知失败、不覆写关闭偏好、慢管道、缓冲上限、单条超限不影响合法事件、长操作持续发送、请求/EOF 超时和子进程名额。

原有三个项目集成测试会向上找到用户目录的 `.agora/project.json`，导致本地假服务返回 404；只给这三个测试增加临时工作目录隔离，未改变业务项目解析行为。原有未提交修改保留。Go 缓存的沙箱访问失败后，仅对对应构建/测试请求宿主执行。

## 尚未验证 / 未启用

- macOS 本机验证了实际子进程行为；Linux、Windows 当前只有交叉编译证据。进程脱离、句柄、回收和 Windows Job Object 下的行为仍需各平台运行验收；现有 CI 三平台矩阵会执行这些测试，本次没有触发远端 workflow。
- 真实 PostHog 部署是否接受省略 uuid、并在查询结果生成 UUID，仍未知。模拟接收器不能证明真实版本行为或数据入库。正式启用前必须查询确认，不能恢复自定义 UUID 作为回退。
- 未配置生产采集地址或 Token；源码内嵌地址仍为空。未部署、发布、修改远端或验证真实入库。

缺失事件不能证明业务失败，创建调用失败不能证明服务端没有创建。统计仅代表观测到的调用成功次数或按合法 App ID 去重的观测项目数，不是完整项目台账。

## 2026-11-18 范围调整

用户要求开启安装画像。当前 collector 改为 `$process_person_profile: true`，按已有安装 ID 允许创建/更新 PostHog Person；GeoIP 继续关闭。上文关闭 Person 的描述及对应测试记录是 2026-11-17 的历史状态。当前测试与告知说明已同步；真实 PostHog Person 创建、实际套餐用量仍需部署环境验证。

本次调整验证：collector 7 项测试通过；CLI 告知定向测试通过；BFF typecheck 和修改代码 Biome 通过；本地 CLI→SSP→模拟 PostHog 收到 10 条事件，断言 Person 处理为 true 且 GeoIP 仍关闭。模拟接收器未验证真实 Person 创建。
