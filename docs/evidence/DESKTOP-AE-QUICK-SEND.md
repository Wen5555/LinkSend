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

## Mac原生续验与菜单可发现性（2026-10-18）

用户已开启现有控制方式的辅助功能。6edddad准确arm64包、原隔离profile/PID38062：只读status-state第02轮观察到真实菜单及“快速发送文件”，随后通过System Events点击该精确菜单项，实际Wails面板含“托盘快速发送”/独立草稿说明/设备与文件选择控件。用户随后确认“托盘里面有啊”。菜单出现的具体触发时序未知，不能归因为右键注入成功。

真实点击面板“选择文件”打开macOS `Open`窗口，读取其按钮后点击`Cancel`，再点击快速面板“取消”。任务库`send_drafts`操作前后整行一致（均为空）；无配对设备、未入队或发送文件。这只证明原生菜单→quick面板→文件picker→取消与空草稿保持，不证明非空主草稿或完整发送。job前缀`/tmp/codex-ssh/job-linksend-ae-mac-`：`status-state-6edddad-20261018-02`、`click-observed-quick-6edddad-20261018-02`、`quick-picker-open-6edddad-20261018-04`、`quick-picker-cancel-6edddad-20261018-01`均exit0。之前几次脚本因AppleScript语法/对象列表和只读DB定位失败，保留失败，不作产品失败。

为响应入口难以辨认，候选只在Mac添加`LinkSend`菜单栏文字，并允许主点击使用锁定Wails原生菜单默认行为；Windows点击唤主窗保持。核心off全test、桌面workspace与off-race/vet/build通过，标签追加后desktop off-race/vet/build再次通过（11.021s）。前端未改，沿用108测试边界；新准确包/主点击待验。

本次Mac实例环境`LINKSEND_SERVER_URL=https://127.0.0.1:1`，主页信令不可达是故意隔离条件。正式`linksend.oooai.de`的HTTPS/WSS与Mac真实客户端必须另验；旧公网TLS超时没有因本节原生通过而消除。

## 98469ce准确包主点击续验（2026-10-18）

五项workflow全部success：core push/PR39087260830/39087262943、desktop push/PR39087260258/39087262949、packages39087260261。三平台四资产外层digest、内层SHA/BUILD-INFO/COMMITTED/clean/版本核验通过，完整表在忽略artifact `pkg-98469ce/PACKAGE-VERIFICATION.json`。Mac arm64 DMG SHA256 `6044b02725d209c50fd1ebb917d7a39def87f674e69d7ccd2bac12633ea7568a`；实际EXE SHA256 `b722bc257ef300462638097f8d069c651971e178f4c341c7e1a178834aced87a`，codesign deep/strict/arm64/bundle通过。

新专用Mac-only loopback fixture经既有正常Bootstrap/邀请/Join完成配对并释放profile owner；通过生产Service.SaveDraft预置main revision1/held-main.txt，不写trust/SQL。新准确app PID39645、profile-a，clipboard=false，旧准确包与公共实例先按PID/path停掉。原生status AX名称现在为LinkSend；真实AXPress使PRIMARY_MENU_COUNT=1，再点击“快速发送文件”打开面板，job `.../job-linksend-ae-mac-primary-quick-98469ce-20261018-01` exit0。这验证本次标签/主点击修复，不继承旧包其他原生结果。

随后点击已观察到的AXPopUpButton“快速发送目标”，旧AX引用失效，独立读取只返回窗口外壳；只读确认PID39645前台/可见、AXMain=true/AXFocused=false/未最小化。不能仅凭AX外壳断言产品空白或选择成功。截图技能只读权限检查requested=false/screenCapture=false，未请求或改变安全权限，没有截图/视觉QA。已异步请用户观察并选择M1原生对端；回答前不盲输或用RPC代替原生选择。

checkpoint job `.../job-linksend-ae-mac-quick-checkpoint-98469ce-20261018-01` exit0：main整行/revision1精确保持、quick行仍空、SQLite quick_check=ok、clipboard=false。`quick-native-checkpoint-receipt.json`已下载；完整quick文件/后台重开/重启旅程仍NOT RUN。app和30分钟fixture为续验保留，精确所有权及收尾入口见唯一TODO。
