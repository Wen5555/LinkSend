# 2026-10-18 N1 目录预设与规则恢复

状态以[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)为准；契约见[ADR0009](../adr/0009-versioned-source-selection.md)。本文件记录源码候选与实际验证，不把loopback或浏览器夹具当物理双机/原生包。

## 6ec3cc4准确Mac一次可用性收尾（2026-10-18）

按用户“把四项现有功能收尾到可用、不做过多测试”的最新选择，本轮只补一个最小目录实际发送，复用已有构建/自动测试，不扩展恢复或性能矩阵。准确源码仍为`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`，arm64 EXE SHA256 `e13f0391f5074996c74fdd7e6ebedd241f0966612a827f9ce676cef39cee3e87`，没有产品源码修改或新构建。

新隔离根`/Users/wen/.codex/supervision/linksend-ae-20261018/preset-native-6ec3cc4`复制已停止的正常配对fixture、准确app和控制库，保留私有profile备份。真实系统picker选择“源码 示例”，主草稿保留原held-main.txt；原生点击“目录预设”并用下箭头/Return选择“源码目录”，Go持久为revision3/source。展开“自定义排除规则”后首次keystroke被当前中文输入法转换，严格比对失败、未点击保存；随后通过现有AX设置准确`*.tmp`并点击“应用排除规则”，Go保存revision4、version1/source/exclude=["*.tmp"]。没有改输入法、系统剪贴板或权限。

原生只点击一次主发送，队列`415776405e2a9d5011f78d2206850622`、task `20261018T053418.545701000Z-00000001`实际completed，3个文件共80B、固定身份Pion/QUIC、双方确认、relay=false/direct_unknown。接收端实际文件集合恰为下列三项，并有空的`源码 示例/保留 空目录`；`.git`、node_modules、dist、build及src/local.tmp均未出现在接收树，所有原源文件（含被排除者）仍保持原hash。

| 接收相对路径 | 字节 | 实际源/目标SHA256 |
|---|---:|---|
| held-main.txt | 38 | eb2adf72e84bb5e0ce1613f1d76311504bbbc218a98c93787cf15cb869b8d6a5 |
| 源码 示例/README.md | 26 | 31e9374ca01ed089f94906e0b7b0efc571fa56ca0d3f61a97e626345ad8c86ff |
| 源码 示例/src/中文.txt | 16 | c4523b38e93bf13243a0957c354fdfc66315d7eef065aa51d4ab296214dc1fc1 |

发送成功才清main paths至revision5，保存的筛选规则保持；quick及旧完成queue/task整行保持。随后额外核对一次快速面板原生目标选择：从B清空再选回B，quick revision4→5→6，除revision/timestamp外内容恢复相同，main/queue/tasks不变、没有入队。该操作补上旧AX只读窗口外壳时未证明的键盘选择边界，详见[N4证据](DESKTOP-AE-QUICK-SEND.md)。

最终真实菜单退出app，receiver正常exit0，专属rendezvous SIGTERM/exit0、监听关闭；2queue/2task均completed（含原各1）、0batch、main5/quick6，rate=0、clipboard=false/grants=0，两库quick_check=ok，身份/偏好/原profile/私有备份保持。非敏感收据`.artifacts/ae-20261018/mac-preset-6ec3cc4-final-receipt.json`已下载。关键job统一前缀`/tmp/codex-ssh/job-ls-ae-mac-preset-`和后缀`-6ec3cc4-20261018-01`：start、signal、receive、pick-goto/path/confirm、menu-stable、source、rules-open、rule-ax-save、send、quick-open/target、quit、cleanup，均终态exit0；首次transfer导航有失效AX引用、首次menu查找无唯一对象、首次输入法校验失败分别exit1，未误提交。此为准确Mac同机功能可用证据，不声明新的物理LAN、规则恢复或双NAT通过。

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

## 阶段来源与Linux链接补验

实现提交`0499cb5200ade5e37cd7f923c20d0058f129f8c1`已推送`codex/lan-rebuild-ae`，远端SHA一致，仍为Draft PR #9。core push/PR `39076292261` / `39076296013`及desktop push/PR `39076292097` / `39076295986`均success；[包workflow39076292203](https://github.com/Wen5555/LinkSend/actions/runs/39076292203)终态success，Windows amd64、Mac arm64/amd64 job均success。构建不替代下载hash核验、准确包启动或物理互传。

为补Windows权限不足的链接负例，核对当前transfer/identity/transport及go.mod/go.sum对0499cb5无差异，以Go1.27.1、`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -buildvcs=false`编译组件测试binary，不冒充原生候选包。SHA256 `e9b38a7c773309d4764364ff896b3bd78be177140a95df60470e3f79c53586a8`，上传JP独立`/tmp/linksend-ae-n1-0499cb5-transfer.test`，核对hash后通过`runuser -u linksend`在唯一TMPDIR执行。

实际5项规则/计数/预览/链接测试全部PASS、无SKIP，包括`TestSourceSelectionNeverFollowsSymlink`与`TestRootRejectsEscapingLink`；脚本见本地`n1-linux-path-tests.sh`，SHA256 `b563496bba988f8cb583db7a061ddb3ff361ee1164022693bdd211496cb94a63`。job `/tmp/codex-ssh/job-linksend-ae-n1-paths-20261018-01`，request同名、exit0。测试无网络监听，Go清理自己的临时子目录后空TMPDIR及临时输出已移除，binary与job证据保留；不触碰信令/控制库/STUN/宿主规则。

浏览器owned最终PID160740与Vite exec78883均已结束，19346无监听，两份`ae-source-selection-20261018`临时入口已移除。根`.playwright-cli/`及原ios/linux未跟踪内容保持。

准确包增量：6edddad Windows原生源码预设预览包含1文件/2目录/1MiB，排除1文件/1目录/23B；同草稿两个CLI目标实际接收集合/中文空目录/hash均通过。三个原生picker取消修复也已准确包复验，草稿与接收偏好保持；物理/原生恢复仍另验。包与精确收据见[原生检查点](DESKTOP-AE-NATIVE-CHECKPOINT.md)。
