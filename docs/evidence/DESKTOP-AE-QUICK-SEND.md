# 2026-10-18 N4 托盘快速发送

契约见[ADR0012](../adr/0012-tray-quick-send.md)，当前状态由[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。源码增加托盘/菜单栏“快速发送文件”，唤起既有窗口中的轻量对话面板；没有新实例/快捷键权限/传输队列。

## 实际行为

- quick草稿和main为不同持久行；打开/重复打开不改草稿，取消保留quick与main，选择器取消保留来源，提交成功后只清quick路径。SaveQuickSendDraft拒绝main、多目标和筛选元数据。
- 最近/常用排序沿用Go设备pin/last-used目录，搜索名称/identity、最多10个候选；文件10条分页。目标撤权禁发送，离线需明确等待。新发现不获得授权。
- Go EnqueueQuickSend只接受revision/等待选择，读取精确quick草稿，通过原Enqueue预检/事务与顺序队列发送。8并发只有一项；同revision已提交后改变等待策略拒绝；清草稿和重启后返回原收据，重启不自动派发。
- `TestQuickSendSavedDraftActuallyTransfersThroughExistingQueue`经真实Pion、固定身份QUIC发送中文文件，实际目标内容一致，main revision/来源不变。不是物理Win↔Mac或系统托盘点击。

## 命令与终态

| 范围 | 实际命令/日志 | 结果 |
|---|---|---|
| 定向核心 | `go test -race -count=1 -timeout 90s ./internal/app -run '^TestQuickSend'`；n4-core-second.log | PASS，6.168s |
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`及vet/build；n4-verified-core.log | PASS，app84.022s |
| 完整应用并发 | `go test -race -count=1 -timeout 6m ./internal/app`；n4-verified-race.log | PASS，192.535s |
| 桌面两模式 | workspace test、off-race/vet/build；n4-verified-desktop.log | PASS，7.522s/6.736s；最后dist后off build通过 |
| 前端 | 固定Node24.21.0/pnpm12.4.1 test/typecheck/lint/build；n4-select-capture-tests.log与n4-select-verified-*.log | PASS，12 files/108 tests |
| 真实绑定 | Wails beta.18/Go1.27；n4-bindings.log | PASS，99methods/66models/2events |

测试覆盖main/quick并存、重开/重启、陈旧CAS、未配对不增信任、离线/撤权、请求幂等、清空后收据、重复点击、取消/焦点、picker取消及晚到旧快照。一次新增重复点击断言没有等待mutation微任务，初读0 calls失败，补正确等待后通过；初次真实useCommand导入未mock Wails runtime造成测试内localhost3000拒绝噪声，补mock后最终日志无该错误。上述是夹具问题，不冒充产品网络故障。

## 浏览器与资源

Playwright独立session ae-quick、生产QuickSendDialog/CSS，临时Vite resolver明确替换为内存元数据backend。960×640浅色/1100×720深色无水平溢出，13文件10条分页与独立滚动、离线等待、完整identity搜索、取消/重开保留选择、Tab/Shift+Tab闭环及Escape通过。窗口内滚动可达footer，长列表小窗不宣称所有header/footer同时可见；截图实际查看。

浏览器曾发现目标选择后仍回到旧identity：React受控select恢复值早于异步mutation读取event.target.value。改为事件内立即捕获identity，再交给mutation；新增108th测试及实际浏览器select复验目标正确、发送按钮可用。旧失败截图保留，最终dark-1100-fixed.png。0 errors/1 warning，warning为Wails浏览器模式提示。

截图在`output/playwright/ae-quick-20261018/`，原始日志/profile/DB在忽略artifact。owned浏览器、Vite及4个临时入口/配置/metadata文件已收尾，原`.playwright-cli`、ios/linux目录保持。原生Windows托盘/Mac菜单栏真实点击、后台唤起和准确包物理旅程仍待后续同提交验收；不能用浏览器按钮模拟代替。

N4提交 `f23383d` 已推送，五项CI/三平台包success，准确四资产与Windows/Mac实际启动来源见[原生检查点](DESKTOP-AE-NATIVE-CHECKPOINT.md)。Windows原生N1目录/QUIC接收、N3设置通过，取消picker错误已定位并修复源码；这些不代表N4托盘真实点击已通过。
