# M4：接收计划的 App 后端接线

2026-09-12。本记录覆盖 Windows 核心后端与真实本机 QUIC 回归，不替代桌面控件、物理双机、macOS 原生包或跨 NAT 验收。

## 实现与绑定接口

- `IncomingFiles(taskID, {expected_revision, offset, limit})`：每页最多 200 项，只返回名称/相对路径/类型/大小/选择状态/目标相对路径；chunk hashes 与完整 Manifest 留在 Go。完整索引写入 M6 schema4 独立表，不塞进 2 MiB 上限的恢复 JSON。
- `IncomingPlan(taskID, {expected_revision, directory, selected_ids, conflict_policy, policies})`：锁外扫描，再核对 attempt/state/revision；返回新 revision、plan_digest 和选择/空间摘要。`selected_ids=null` 全量，`[]` 全跳过；显式目录须已存在。
- `AcceptReceivePlan(taskID, revision, planDigest)`：接受确切预览，再查本机拒绝和磁盘空间；持久保存目录、PlanDigest、SelectionDigest 与任务计数后才投递一次决定。旧 `AcceptTask` 仍全量，并允许用户同意后创建首次默认目录。
- StartReceive、inbox、ResumeReceive 共用 `taskReceiveOptions`。`PlanChanged` 先同步私有恢复记录，再串行调用 M6 `RegisterInboxReceivePlan`，登记最终路径和 staging/part 的真实 OS 身份；任一步失败都阻止继续。
- `SendHooks.SelectionAccepted` 在正文读写前保存接收选择，恢复不可更换摘要。接收恢复校验 peer、原 TransferID/manifest digest/chunk 策略/总量/条目数和计划。目录失效保留 `recovering / needs_attention / RECEIVE_DIRECTORY_UNAVAILABLE`，用户修复原目录后再明确恢复，不重建或切换保存位置。
- `no_content` 独立终态、原总量、选择/跳过计数贯通任务和队列；空目录是有效 0 B 内容。实际累计 Sent/Received 不被恢复完成时的逻辑总量覆盖。
- Windows 用 `GetDiskFreeSpaceEx`，Darwin/Linux 用 `Statfs`；按完整选中正文加 16 MiB 元数据保留量检查，返回实测 available、required、reserved。预检不是 quota 保证，也未精确核算 inode、目录或恢复 staging 复制峰值；实际 ENOSPC/权限改变仍按真实错误处理。
- `StartInbox` 仅取消空闲 accept 子上下文，活跃传输保留原 context 与冻结计划；下一轮 accept 读取最新配置，任务结束后执行 LAN 延后重配。修改全局目录不能中断当前接收。

## 真实故障与兼容修复

首次真实 QUIC 子集、全跳过和旧恢复回归出现 `SQLITE_BUSY`：每次 `historyDB` 打开都执行写 schema 的读后写事务。合并 M6 `47110f8` 后，当前 schema 只读核验返回，modernc/sqlite 使用已核实的 immediate transaction；新 M4 测试两轮与完整根 race 均通过。

另发现接受后失败的 JSON error 与 chunk 共用 V1 帧位置，旧接收器曾将 52 B error 计入正文。只看预期 hash 仍不足，因为文件可能恰好就是相同 JSON。新增可选 `acceptance_commit_v1`：offer 声明、accept 回显 `commit_barrier=true` 后，sender 先持久化 hook，再发送绑定原 digest/selection_digest 的 `accepted`；receiver 读到正确确认才请求 chunks。未双向协商保留旧时序。旧模式只在小帧不匹配预期 chunk hash 时尝试识别 error；匹配 hash 的合法 JSON 文件仍按正文处理。未协商屏障的旧协议仍有完全同字节帧歧义，不能声称旧 binary 获得新保证。

新真实 QUIC 回归同时覆盖 persistence/source 失败时零正文、以及正文恰好等于 error JSON 时正常落盘。真实 pin、TLS 1.3、原 Manifest、V1、QUIC 数据路径保持；信令与 JavaScript 未获得正文。

## 实际验证

PowerShell 入口 `. 'D:\apps\Osend\.tools\use-desktop-toolchain.ps1'`；逐条核对退出码。以下均 PASS：

- `go test ./internal/app -run 'TestIncoming|TestSenderRejectsChanged' -count=2`（8.512 s）。
- `GOWORK=off go test ./internal/app -count=1`（20.040 s），含接收 4 MiB 后改目录不取消、下一次使用新目录。
- `go test ./internal/transfer -count=1`（4.223 s），含旧帧双向兼容、计划恢复、新屏障与同字节 JSON 反例。
- 根 workspace `go test ./...`；根 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`。
- 根 `GOWORK=off go test -race ./...` 全包通过；App 56.144 s、transfer 7.927 s。
- 桌面独立 `GOWORK=off go mod verify`、`go test ./...`、`go vet ./...`、`go build ./...`。
- 最后增补空间 reserve DTO 后，`GOWORK=off go test -race ./internal/app -run 'TestIncoming|TestSenderRejectsChanged|TestInboxReconfigure' -count=2`（17.899 s）；transfer 定向 `-race -run 'TestSelectionPersistence|TestReceivePlan' -count=1`（4.630 s）。完整根 race 的上述时间对应增补 reserve 字段前的代码。
- `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c ./internal/app` 与 Linux amd64 同命令交叉编译通过；未运行生成的测试程序。
- `git diff --check`。

## 合并与原生验收边界

前端绑定、原生文件清单/冲突选择/目录选择由主分支接入。M5 合并必须让 ContentAccepted 与 SelectionAccepted 都在 accepted 屏障前完成，并让原生内容摘要参与 accepted 验证；M4 App 本身不启用 AcceptNativeContent。

本分支未执行 push/deploy/release，主代理负责完整里程碑审查发布。交叉编译只证明可编译；真实 Windows↔macOS、跨 NAT、原生操作与故障卷仍须单列实际运行结果。
