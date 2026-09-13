# E4-01 存储与会话支撑证据

日期：2026-09-14。profile SQLite owner 与认证 QUIC 文件会话复用已形成源码候选；文件/剪贴板公平调度仍为 NOT RUN，不能把本文件视为 E4-01 完成。

## 单一 task history 连接

此前每次 task snapshot/recovery 持久化都会重新调用 historyDB，包含 sql.Open、schema/metadata 查询与 Close。taskManager 现在持有 profile 生命周期唯一的 task-history.sqlite 连接，并把同一 sql.DB 交给 desktopStore；MaxOpenConns=1 保持 task、queue、draft 和 inbox 元数据的串行 owner；构造失败和正常 Shutdown 都显式关闭，持久化失败仍通过既有 historyErr 撤销 durability claim。测试覆盖 taskManager 与 desktopStore 指针同一、失败构造释放文件句柄和 profile lock、Shutdown 后数据库可重命名、历史恢复与写失败。

实际命令：

- go test ./internal/app -run 'TestProfileIdentityAndDiagnosticsAreLocal|TestFailedConstructionReleasesProfileLock|TestTaskHistory' -count=1：PASS。
- go test ./... 与 go vet ./...：PASS。
- go test ./internal/app -run '^$' -bench BenchmarkTaskHistoryConnectionLifecycle -benchtime=100x -count=3：PASS。

Windows amd64、i9-13980HX 的 100 次写结果：persistent 为 2.024/2.076/2.084 ms/op；reopen_each_write 为 3.663/4.254/3.699 ms/op。中位数约从 3.699 ms 降至 2.076 ms，约减少 43.9%。这是本机微基准，不代表网络吞吐或跨平台结果。

## 尚未完成

下一步必须验证同一认证 peer/grant generation 的 QUIC 连接复用、每操作独立 stream、取消单流不关闭文件任务，以及带预算的文件/剪贴板公平调度。自动剪贴板仍默认关闭且尚未实现。

复测（共享单一 sql.DB 后）：persistent 2.140/2.176/2.190 ms/op；reopen_each_write 3.813/4.052/4.183 ms/op，中位数减少约46.3%。定向 race 三轮 PASS。
## 认证 QUIC 双向复用

会话池以 peer ID + authorization generation 为键，连接两端持续接受独立双向 stream。连续两次正向文件和一次原始响应方反向文件保持相同 session_id；传输中取消第一条 stream 后，第二文件仍在同一 QUIC 会话完成。授权撤销、连接错误、网络变化、3 秒空闲与 Shutdown 关闭池中连接；活动文件流不会被并发新文件替换。

真实 Pion ICE + quic-go 定向测试 TestPairingCodePersistentInboxAndAlwaysAccept 与 TestCancelledStreamKeepsAuthenticatedSessionForNextFile 普通 5 轮、race 2 轮 PASS。该结果为本机双实例 loopback，物理 Windows/Mac 双向流仍需另行验证。