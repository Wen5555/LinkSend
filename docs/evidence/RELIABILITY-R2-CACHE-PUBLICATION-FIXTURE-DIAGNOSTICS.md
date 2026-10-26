# R2缓存生命周期发布前置门与诊断

日期：2026-10-26。范围仅为一个既有生命周期用例的前置条件和失败诊断，不改产品、清理授权、20秒发布等待或缓存保护断言。原925 Windows失败根因仍为 **INCONCLUSIVE**，不能称已修产品，也不能归因环境。

## 准确失败与一次隔离诊断

准确 `925ba7c86615eac07fb168989134dacc5f7778c4` packages `40278523523` 的 Windows job `121426414113` 第1次在 `Verify core module` 失败：`TestClipboardCacheLifecycleNeverReclaimsPossiblyPublishedOrUnknown (20.26s)`，`clipboard_cache_lifecycle_test.go:356 copy did not publish`。原日志只证明完整fixture context耗尽，没有发送/接收progress，不能判断是复制前置门、ready帧还是接收提交失败。原始job日志保留于 `.artifacts/r2-helper-ci-925ba7c/windows-job-original.log`。

探针步骤因该失败被skipped，always上传步骤确认没有native-copy文件/API亦无该artifact；因此本run的Shell generation probe为NOT_RUN、JSON=0、mutation=0，不能从它得出unsupported。独立desktop vet错误由另一增量处理；本测试不运行探针或系统剪贴板。

在新ignored共享对象clone检出准确925，初始Git状态为空（`CACHE-SOURCE-INITIAL.json`）。仅在该用例包裹已有假ReadFiles/WriteFiles回调计数，并在原失败分支打印双方只读peer状态、file progress及假OS代次。执行一次：

```text
GOWORK=off go test ./internal/app -run '^TestClipboardCacheLifecycleNeverReclaimsPossiblyPublishedOrUnknown$' -count=1 -timeout 45s -v
```

结果exit0，app6.942s/用例0.61s，日志`cache-diagnostic-first.log`。没有循环重跑以寻找偶发；这次通过不能否定原CI失败，原具体层仍未复现。

## 最小测试增量

共享fixture此前只等待`SendReady`缓存位，该位不独立证明当前pool连接仍健康、当前files capability/session/授权代次及实际files lease仍有效。此事实本身不证明它导致了925失败。

本用例在复制前增加至多2秒的起始前提检查，明确等待双端同一当前session、连接未关闭、files能力、互相匹配的授权代次、发送方实际且未过期的files lease，以及其接收grant revision仍匹配；记录该起始session/代次/lease/期限。该检查位于同一原20秒context范围内，不延长整体发布期限；失败时增加假Read/Write计数、双方peer状态和file progress以区分下一次真实失败的层次。所有`publishing/published/protected`未来时间保护、未知目录不得取得删除权限及源内容断言原样保留。

同一准确925隔离source加此前置门后，只执行该用例一次：exit0，app7.902s/用例0.48s，日志`cache-gate-verified.log`。这是必要的前置门/诊断验证，没有重跑完整矩阵、旧CI或系统clipboard/GUI，没有产品代码修改，没有扩大TTL、期限或权限。

主树目标文件确认无其它改动且对应HEAD blob仍与925相同后，只同步该已验证测试和本证据；gofmt与diff检查通过。机器回执和冻结清单位于同ignored目录。后续准确CI由root统一push，若再出现失败按新阶段证据继续修复；本增量不将925原失败改记通过，不标完整缓存/原生粘贴验收完成。
