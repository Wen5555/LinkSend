# R1同策略lease晚到造成文本复制静默丢失

日期：2026-10-26。来源为准确 `366e62ff39440bd9101b68623a50d8d4f7face23` 第1次 Windows CI 真失败；修复只动非文件剪贴板目标lease选择的一行与原用例的确定屏障。没有系统clipboard、GUI、旧CI重跑或完整矩阵。

## 原失败与因果

push/PR core与desktop均首次success，但 packages `40275356333` 的 Windows job `121416842755` 在 `Verify core module` 失败，尚未编译新的C++ helper。原日志 `.artifacts/r2-helper-ci-366e62f/windows-job-original.log:348-349`：`TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists (6.93s)`，`clipboard_transport_test.go:466 reverse clipboard did not commit`；两侧均ready、SendReady/ReceiveReady=true，Error为空。没有新包下载；它不能被归因为Shell原语或未经证明的环境抖动。

原fixture为A receive授权触发自动renew，又显式发送一次lease；B启用send和两张lease到达的顺序并不固定。若同策略第二张lease在B实际复制使native generation变为3后才安装，该lease baseline=3。文本/链接/图片路径用`Outbound`挑最新lease，即使另有仍有效且baseline=2的同策略旧lease，`BindScoped`也会因`copy.OSGeneration <= lease.baseline`拒绝最新lease。调用方静默跳过该目标并返回nil，因而可以表现为两侧ready、原生Read已执行却没有接收提交。

这不是独立event ack尚未完成的问题：文本流没有该ack，且本用例随后B的grant更新与forward Commit共用grant mutex，能等到其完成。关键是复制与lease安装的OS基线顺序。

## 确定屏障与最小修复

在新ignored共享对象clone中检出准确366，初始Git状态为零项，回执 `REVERSE-SOURCE-INITIAL.json`。只改原测试，沿用真实已认证QUIC及内存假adapter：

1. 先启用B send，再启用A receive，保证自动发来的第一张lease可以安装。
2. 通过既有`clipboardLeaseBeforeWrite`只阻塞第二张真实lease；等B已安装第一张，确认第一次forward已把native generation推进到2。
3. 设置假native generation=3，再释放第二张lease，等B实际最新lease ID改变后才调用`ClipboardChanged(3)`。原3秒断言和全部后续文件共存、策略替换、新复制取消与shutdown断言保持。

旧产品代码加上述fixture执行一次：

```text
GOWORK=off go test ./internal/app -run '^TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists$' -count=1 -timeout 45s -v
```

实际exit1，app13.881s/用例6.89s。`reverse-old.log`明确two writes=2、old/late ID不同、native=3、reads=1，仍是双方ready/Error空而reverse未提交。45秒仅为本次前后相同的命令级运行上限；各复制成功/失败断言仍为原3秒，没有用加长等待掩盖失败。

产品只将`ClipboardChanged`非文件路径的`Outbound(...)`改为已有`OutboundForObservation(..., observation)`。该方法已供文件路径使用，选择仍有效且早于当前复制的重叠lease，同时保留peer/session/授权代次/kind、当前observation revision/OS代次及期限校验；policy改变仍替换旧lease，暂停/撤权/旧复制/重放门不放宽。网络wire、schema、权限、限流与身份认证均无变化。

完全相同fixture和命令再执行一次：exit0，app11.728s/用例4.64s。`reverse-fixed.log`保持两张不同ID/native3的确定晚到窗口并完成reverse及原用例全部后续断言。没有为通过而取消屏障或修改deadline，没有运行其他矩阵、原生剪贴板或真实LAN/NAT。

## 来源、收尾与边界

隔离source的最终改动与主树同步前，确认两个目标主树无其它修改、HEAD对应blob仍与366相同；仅同步一行产品修复和同一已验证fixture。独立只读复核事先指出相同lease-baseline因果及权限顺序，未替代本次运行证据。日志、源码初始回执、固定测试hash和本增量冻结清单均位于 `.artifacts/r2-helper-ci-366e62f/`。

原366 failure保留，`5ee9a40`准确Windows STATIC_PASS资产及生产待授权候选证据不改写为新修复的结果。新提交CI由root统一push后核对；本地loopback用例不替代真实网络、Explorer粘贴或已发布缓存自动删除验收。
