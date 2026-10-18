# 2026-10-18 客户端灾后恢复与授权隔离

状态以[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)为准。本文保留edb67ef前置修复快照，后半部分记录显式恢复、通用上下文退休和桌面入口候选；准确包原生、外部UDP和双NAT仍待验，不代替完整B退出条件。

## edb67ef：旧组与独立LAN重叠的修复前失败

旧实现把跨组缺席peer从 `group+lan` 降为LAN时保留原GrantGeneration/GrantedAt，因此service的前后代次比较看不到变化：旧活动任务未取消，队列仍可能借保留LAN关系确认/派发。成员响应重建peer时还会丢LAN request ID；跨组/本机incarnation改变会丢LAN地址/历史，GrantedAt归零。

新增实际回归命令：

```powershell
go test -count=1 ./internal/identity ./internal/app -run 'TestMembershipTransition'
```

修复前exit1：identity四种场景均暴露屏障或LAN证据丢失；service场景在等待1秒后明确报告旧活动任务未取消。原始日志 `.artifacts/ae-20261018/membership-transition-red.log` 留在本地，不提交随机identity/完整载荷。

## 修复行为与通过结果

- 旧group+lan降为LAN、组或本机incarnation改变均提升授权代次并更新GrantedAt，关闭免确认；代次耗尽原子拒绝，原文件保持。沿用service的现有代次比较来清剪贴板grant/关session/取消旧任务，不新增旁路权限。
- 有效独立LAN的地址、最多8条历史及request ID保留；普通同上下文刷新维持原代次和同意；无关LAN-only peer保持。
- 持久队列保持原authorization_generation，旧任务/旧队列确认被拒；profile重启后仍拒旧队列，新的显式LAN入队取得当前代次。identity文件与queue使用真实临时磁盘，传输未启动，不用该fixture冒充实际双机文件测试。
- wire与trust schema2保持，SPEC/SECURITY/ADR0007同步契约；本批没有客户端重置按钮、自动401恢复、旧快照退休或撤销请求跨上下文的新承诺。

修复后相同定向命令exit0；新增代次耗尽/磁盘原子性负例和profile重启确认也通过。进一步命令及结果：

| 验证 | 实际命令 | 结果 |
|---|---|---|
| 核心完整 | `GOWORK=off go test -count=1 -timeout 5m ./...` | PASS；app59.304s |
| 核心静态/构建 | `GOWORK=off go vet ./...`、`go build ./...` | PASS |
| 授权与应用并发 | `go test -race -count=1 -timeout 5m ./internal/identity ./internal/app` | PASS；app122.937s |
| 桌面两个模式 | desktop内workspace及`GOWORK=off go test -count=1 ./...` | PASS |
| 桌面静态/构建 | desktop内`GOWORK=off go vet ./...`、`go build ./...` | PASS |

前端本批无变更，沿用紧邻STUN阶段准确Node24.21.0/pnpm12.4.1的86tests/typecheck/lint/build结果，不重复称为本批原生验收。最终准确包与真实网络测试仍待完整功能集成。

## 显式恢复、通用退休与桌面入口候选（2026-10-18）

上述“尚无恢复API/schema2保持”仅指edb67ef。本次候选新增Prepare/Continue/Cancel/Status及真实Wails绑定、设备页恢复面板；设计与状态详见[ADR0008](../adr/0008-membership-recovery-and-retired-contexts.md)。wire、控制库schema4与桌面任务库schema9保持，trust在恢复或成员上下文替换时升级至schema3。任务库9已按e43043d的history.go核对，先前写7为文档笔误，本恢复批次没有任务库迁移。

### 已运行的控制/存储夹具

真实 `server.Server` + SQLite由同一HTTP listener切换至新的空DB，正常签名API执行：旧profile401保持auth_failed、不自动开始恢复；用户确认后正常Initialize，新邀请码正常Join，双方实际获得新peer grant，身份不变、无缓存owner提权。旧任务/queue代次保持，旧队列不能借保留LAN确认，源文件保留；重复同一完成请求不再次初始化/消费邀请码。

将真实初始化成功响应主动断开，API实际失败；intent仍started，取消转paused。关闭/重开同一profile后，以正常Devices核对既有结果完成恢复，Initialize计数仍为1，未重放邀请码。没有删除profile、修改控制库记录、关闭pin或伪造旧组恢复。

测试覆盖旧snapshot即使revision9999也拒绝、普通重配同样退休旧context、本地LocalBlock保持、旧outbox在新context前被隔离、新组低revision不与旧组直接比较、代次耗尽/退休容量/无效intent/异常备份拒绝且原策略字节不变。重复/缺本机/错误key或incarnation的完整快照不能部分退休原上下文；新/legacy/只有旧group而缺actor-incarnation的策略有明确处理。

legacy初始缓存没有旧context时，用真实旧服务的Devices请求屏障与并发Prepare证明：完整读取/应用先完成并进入退休集合，随后换空库恢复，晚到旧context仍不能应用。恢复与选中的旧outbox并发时，旧tuple在操作锁后重新检查，DELETE计数为0。LAN签发/补组在pending期间不初始化/加入新组。

独立LAN保留地址/历史/request ID并采用新的代次，pending时仍可授权；group-only不能借旧组发送，新的明确LAN同意可以建立独立grant。这里只验证控制、授权和持久队列边界，未把这些夹具写成LAN或双NAT文件hash通过。

候选开发中发现过两个实际回归：广义401前置检查阻断正常被移除后的新码重配，已保留正常流程并改为撤销首发前live actor证明；授权pin miss使用TryLock导致8并发幂等入队误拒绝，已改为串行等待刷新。原始失败日志保留，相关并发/重启场景race五轮通过；最终验证使用后续准确源码，不借用中间失败快照。

### 本地命令与结果

Go实际版本 `go1.27.1 windows/amd64`，默认workspace为 `D:\apps\Osend\go.work`；显式off命令均在独立进程环境设置。固定Node24.21.0/pnpm12.4.1含嵌套命令已确认。

| 验证 | 实际命令/本地日志 | 结果 |
|---|---|---|
| 核心完整/off | `GOWORK=off go test -count=1 -timeout 5m ./...`；`recovery-verified-core.log` | PASS；app58.438s |
| 核心静态/构建 | `GOWORK=off go vet ./...`、`go build ./...` | PASS |
| identity/app完整race | `go test -race -count=1 -timeout 5m ./internal/identity ./internal/app`；`recovery-verified-race.log` | PASS；identity8.541s、app122.250s |
| 幂等入队/延迟旧读/旧outbox并发 | `go test -race -count=5 ./internal/app -run 'TestQueueIdempotentConcurrentEnqueueAndRestartConfirmation\|TestRecoveryCapturesLegacyContext\|TestRecoverySerializes'` | PASS；14.547s |
| 桌面两个模式 | workspace test；off `go test -race -count=1 ./...`、vet/build | PASS |
| 前端 | `pnpm run test/typecheck/lint/build`；`recovery-verified-frontend-*.log` | PASS；9 files / 90 tests |
| Wails绑定 | beta.18 `generate bindings -clean=true -ts -i` | PASS；1 service / 92 methods / 62 models |

### 旧库兼容与原文

在仓库外18a88f2导出树中编译只读兼容probe，核对其identity.go Git blob与该提交一致。旧库读取schema2成功、schema3准确报 `unsupported trust policy schema`；当前库读取同一schema3成功，前后策略SHA256不变。helper SHA256 `53b299b45db04f9460e28d21b91bb8cb131b0f53d87915b6c3432b42aac2ada4`，旧库probe二进制 `759b0c270c459cd2e8fccdc45230c1ed94484f0fa181d127077c2be672b4ab08`。

这是旧真实库+单独helper的兼容探针，未冒充旧正式包或原生升级验收。首次恢复保存精确原策略 `trust.json.pre-schema-3` 并建立/保留pre-schema1失败关闭标记；删除当前策略时不会打开空denylist或.previous。备份异常/容量满在策略授权变更前拒绝，不自动覆盖原备份或删除退休历史。

### 浏览器界面与资源终态

Playwright独立session `ae-recovery` 对真实DevicesPage/RecoveryPanel、生产CSS和明确mock控制API做960×640浅色、1100×720深色检查。首次窄窗口发现恢复入口位于设备目录之后，已移到页首。深色中输入合成码并Tab/Enter完成，记录一次continue、无重复prepare，页面无水平溢出、console Errors0/Warnings0。960窄窗口内容较长，完整原生DPI/键盘矩阵仍留E。

截图与mock脚本仅本地 `output/playwright/ae-recovery-20261018/`，未入Git；合成码不是服务邀请码。浏览器PID143716与Vite session53768已停止，19345无监听；临时HTML已移除，原有根 `.playwright-cli/` 未动。浏览器结果不替代Windows/macOS原生RPC、实际系统入口或跨网文件验证。

### 阶段同步与CI

实现提交 `e43043d25c0cbb03ece846b093297dc47069a711` 已推送 `codex/lan-rebuild-ae`，远端SHA一致，沿用Draft PR #9。core push/PR `39073187230` / `39073189614`、desktop push/PR `39073187229` / `39073189666`、[三平台包39073187228](https://github.com/Wen5555/LinkSend/actions/runs/39073187228)均success；Windows amd64、Mac arm64/amd64均success。后续A源码改动需自己的准确包，不能把这些包结果迁移过去。

### 98469ce准确Mac到已部署新服务的原生恢复（进行中）

完成N4后停止准确Mac package，远端同任务私有目录完整备份post-quick测试profile，密钥仍只在原Mac受限目录，未回传。旧loopback fixture/CLI均正常退出。使用同一准确98469ce app与原profile/原身份，进程级将服务地址切至已部署的严格`https://linksend.oooai.de`；偏好、源/收到文件、主草稿与completed quick队列保持。这是从旧组profile到不同服务URL的新码加入案例，不是同URL空库或物理文件测试。

真实原生设备页→服务重建后重新连接→默认加入新组→勾选明确确认→准备恢复，持久intent为prepared/join且endpoint fingerprint严格绑定正式URL；native prepare job01 exit0。原生“取消准备”后intent消失、旧context/两草稿/completed队列保持，job cancel-prepare exit0。再次原生prepare（job02）后真实停止/重启应用，同一完整intent精确保持；设备页自动显示继续恢复、新码字段和禁用确认按钮，restart-prepared job exit0。

专用公共组通过正常客户端创建新一次性码，仅在Mac进程内存中交给原生输入控件，无码/密钥写入本仓库或日志。第一次原生键入/确认进入持久started，实际请求返回PAIRING_CODE_INVALID。独立公共B的Devices确认当前仍2成员、旧测试A身份没有加入；不是成功。后续新码元数据为9字符，自动键入合成ABCD-EFGH实读18字符，AXValue写入也未匹配；未据此伪称产品根因或反复提交，正在请用户手动输入合成文本（不确认）区分自动化/输入问题。原intent保持started，后续必须使用“核对并继续恢复”并先查实际远端结果，不重做prepare。

实际面板还暴露错误显示缺陷：`RuntimeError: signaling HTTP 400: PAIRING_CODE_INVALID: ...`被首个大写token HTTP遮蔽，原有中文映射未命中。新增真实包装格式回归RED，局部提取传输包装后的协议码，输入异常与错误显示分别记录；该后续源码不改变98469ce包的以上证据。当前恢复尚未完成，旧context退休/远端加入/重启后新组在线与文件保留最终核验仍待执行。

错误提示修复本地终态：固定Node24.21.0/pnpm12.4.1，12files/109tests、typecheck（含绑定）、lint/build均exit0，日志`native-error-frontend-*.log`。核心`GOWORK=off go test ./...`缓存通过；desktop workspace/off test7.246/6.928s、off vet/build全部exit0，日志`native-error-core-off.log`和`native-error-desktop.log`。必要dist重新生成，原协议码、配对输入/请求与Go源码不变；新准确包原生错误文案尚未复验。

cf19349后续五项CI/三平台包与四资产完整核验已通过；新准确Mac独立空profile的真实无效配对操作已显示既有中文提示，见[准确包续验](DESKTOP-AE-NATIVE-CHECKPOINT.md)。该独立错误测试已收尾；恢复profile保持98469ce，同一started intent在进一步真实停止/重启后仍保持，owned43588/恢复面板显示，新码字段为空，等待手动合成输入观察。检查点收据确认偏好字节、两草稿、源hash、唯一completed task/queue仍保持；独立服务仍2成员/旧测试身份未加入，恢复尚未完成。
