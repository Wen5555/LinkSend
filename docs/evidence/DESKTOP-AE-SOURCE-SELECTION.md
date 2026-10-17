# 2026-10-18 N1 目录预设与规则恢复

状态以[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)为准；契约见[ADR0009](../adr/0009-versioned-source-selection.md)。本文件记录源码候选与实际验证，不把loopback或浏览器夹具当物理双机/原生包。

## 已有实际证据

`transfer`共用规则与`os.Root`遍历，默认完整，source预设/自定义规则、包含排除计数/大小/200条展开、撤销规则均已实现。草稿、队列、派发、task recovery、retry与历史resend保存相同规则。任务库9→10有可读校验备份，旧行默认完整；带筛选任务恢复版本2，文件wire仍V1。

本地`go test -v -count=1 -timeout 90s ./internal/transfer`通过，含规则规范化、中文、大小写、嵌套、重叠输入、预览/manifest一致、排除文件变化后重建原transfer身份、取消和完整发送还原。新增及原有符号链接测试因Windows权限明确SKIP，不计为链接实际通过。

`go test -race -count=1 -v -timeout 2m ./internal/app -run '^TestSourceSelection'`首轮通过（11.841s）：经真实签名信令/生产Pion及固定身份TLS QUIC发送中文文件/空目录，目标hash一致且排除目录没有出现。真实首块发送后Pause→保留同profile→重启→明确确认，原task/transfer身份保持、新attempt完成，排除内容变化不重新纳入；入选文件变化分支报告SOURCE_CHANGED。持久规则、幂等冲突、schema9备份及未来版本拒绝同时通过。后续新增草稿来源绑定仍需使用最终回归结果，不能沿用此时间当最终源码通过。

前端曾新增错误的测试环境指令导致document缺失/未安装jsdom；已沿用现有happy-dom，无新增依赖。随后94项测试/typecheck/lint/build通过，后续加入草稿来源字段需重新生成真实绑定并完整复验。最终核心/desktop/race/前端结果待下方回读。原始日志只在忽略的`.artifacts/ae-20261018/n1-*.log`，不提交profile、DB、文件正文或完整随机身份载荷。

## 最终本地回归与兼容

| 范围 | 实际命令/日志 | 终态 |
|---|---|---|
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`、vet/build；`n1-verified-core.log` | PASS；app69.053s、transfer13.332s |
| 完整应用/文件并发 | `go test -race -count=1 -timeout 6m ./internal/app ./internal/transfer`；`n1-verified-race.log` | PASS；app169.926s、transfer14.049s |
| 桌面 | workspace test、off-race/vet/build；`n1-verified-desktop.log` | PASS；workspace7.232s、off-race6.226s；最终dist稳定后再次off build |
| 前端固定工具链 | Node24.21.0/pnpm12.4.1 test/typecheck/lint/build；`n1-verified-frontend-*.log` | PASS；10 files / 95 tests |
| 真实绑定 | Wails beta.18 generate bindings | PASS；1 service / 92 methods / 64 models |

草稿来源绑定新增负例通过：同revision替换规则、哈希前已发生的草稿修改均拒绝；内容摘要随正确草稿版本入队，草稿随后清空仍返回同一幂等收据。关闭预设保留自定义排除，只有明确“恢复完整发送”才清空全部规则。

在仓库外18a88f2导出树中新增单独只读test helper，核对其生产`internal/app/history.go` Git blob与该提交一致，实际旧`taskStoreSchema=9`。当前候选创建有源码预设的schema10草稿，旧真实库打开准确返回`TASK_STORE_VERSION`，前后数据库SHA256不变。helper SHA256 `b8464c00519e8ef513dee77aaca3393a5c446796502073841a1dd14a84b276ba`；日志`n1-old-schema-probe.log`。这是库兼容，不是旧正式原生包升级。

## 浏览器界面与收尾

Playwright独立session `ae-selection`、生产SourceSelectionPanel/CSS和明确mock元数据：960×640浅色/1100×720深色，中文长路径无水平溢出；Tab/Enter恰好一次应用自定义规则，关闭预设仍保留自定义，恢复完整发送成功。体积沿用通用格式避免小文件四舍五入为0MiB，深色控件使用对应token，等待有限主题动画完成后截图并实际查看。最终页面console Errors0/Warnings0；早期夹具favicon404和开发HMR重复createRoot只属于临时入口，重载后无新问题。

本地截图/脚本`output/playwright/ae-selection-20261018/`不入Git，不代表原生RPC。owned浏览器、Vite19346与临时入口按实际终态收尾；准确Windows/Mac原生、物理双机及源码CI/包分别继续。没有修改宿主防火墙、代理、路由、日常文件或剪贴板。
