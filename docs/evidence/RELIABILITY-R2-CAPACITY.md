# R2首批：自动空间预留（2026-10-19）

基线为 `f5f05ef878d919cd319f514574c35300e9d5b631`。先形成[ADR0016](../adr/0016-automatic-clipboard-file-cache.md)，本增量只落实容量预留和拒绝链路；持久所有权、已发布缓存自动回收、boot identity与原生粘贴验收仍未完成，不提前宣称整个R2完成。

## 行为与边界

接收实际 manifest 经原文件协议验证后，Service 的单一 profile owner 以锁串行预留完整内容、每条目4KiB及16MiB事务余量；根据当前卷自动保留 `max(64MiB, min(1GiB, available/100))`。检查扣除其他未释放的预留，避免两路同时认为同一空闲容量可用。保守地不提前扣减已落盘字节对应的预留；磁盘统计不构成quota保证，真实ENOSPC仍由原引擎处理。

不使用可伪造/不完整的剪贴板事件大小提示决定容量，目录提示即使为0也按实际manifest计算。预留持续到校验/原生发布返回，所有失败、取消和成功退出经幂等释放。本轮不引入数据库/schema或用户设置。

不足时拒绝新复制，沿既有受约束根删除本次从未发布的残留；不会发布部分文件，不删除已有缓存、发送源或普通接收目录。界面以安全码显示“空间不足，暂时无法复制文件”，没有手动清理、容量或期限设置。自动安全回收尚未接入，因此“压力时先回收”的完整要求仍待后续实现。

同时修复拒绝原因丢失：先在私有job记录本地拒绝，再发送协议终态；由该拒绝引发的peer取消/断链不能把磁盘失败覆盖成普通取消。真正先发生的用户取消仍优先。容量错误保留底层ENOSPC，复用已有V1 `DISK_FULL`映射；发送端先类型化任务错误再归还会话，避免单流磁盘失败关闭健康连接。无新wire字段或协议版本。

被取消的文件I/O可能返回EOF而非context.Canceled。归还会话时同时保留job context的取消意图和原I/O错误，避免误关仍健康的QUIC；连接本身已关闭时原检查仍禁止复用。新增真实认证会话的定向负例先证明原归还路径会关闭连接，再验证修复后同会话可继续实际复制。

## 实际失败与验证

- 在原接收链路注入64MiB可用空间，`TestClipboardFilesInsufficientSpacePreservesExistingFiles`实际失败：`insufficient-space copy was published`，退出1，日志 `r2-space-before.log`。没有填满真实磁盘。
- 第一版修复暴露取消/断链覆盖本地失败的竞态，失败日志 `r2-space-after.log`保留；修复底层错误映射、发送端错误类型和私有本地拒绝顺序后，稳定原因及下一次复制恢复通过。
- 初次组合race的“接收端刚拒绝后立即再次复制”有一次未入队（`r2-race.log`），不以正常测试通过撤销。随后针对取消I/O归还路径的确定性回归先退出1（`r2-cancelled-io-before.log`），修复上述取消意图；最终保留原立即复制顺序，没有通过等待发送端终态或扩大deadline消除断言。
- 最终定向组合普通3轮退出0（app14.051秒，`r2-focused-v3.log`），定向race3轮退出0（app24.979秒，`r2-race-v3.log`）；立即复制的独立race10轮退出0（13.737秒，`r2-pressure-race-v3.log`）。覆盖32路并发预留恰好两路可用、未知磁盘/恶意manifest拒绝、幂等释放、实际原生adapter发布前保留预留、空间不足不发布/无残留/旧文件与发送源可读、新复制成功及旧磁盘错误清除、既有四种取消优先级、取消I/O保持健康会话和认证文件传输。
- 前端 `pnpm run typecheck/lint/test/build` 均退出0，16文件/126项测试；新增的是空间不足在复制窗的实际渲染断言。日志 `r2-frontend-*.log`；使用便携Node24.21.0/pnpm12.4.1。先完成dist，再执行Go embed编译。
- 最终产品源码与测试对应的两Go模块workspace/off完整 `go test ./...`、`go vet ./...`、`go build ./...` 均退出0，12条独立命令日志为 `r2-final-{core|desktop}-{workspace|off}-{test|vet|build}.log`；完整核心workspace app96.712秒。最终构建使用已完成的前端dist。提交/远端SHA/CI另行核对，未结束CI不计通过。

日志目录 `.artifacts/reliability-20261019/`。测试只用自有临时文件和内存clipboard adapter，不操作日常剪贴板或安装；不能代替Finder/Explorer键盘粘贴、实机自动回收或准确包原生旅程。没有新删除已发布对象的行为。
