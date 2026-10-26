# R2 私有缓存的普通源读取与路径出口拒绝

日期：2026-10-26。范围是已验证的出口拒绝增量，**没有系统clipboard发布、Explorer键盘粘贴或Native自动删除通过**。缓存保持程序自动管理，不增加清理按钮或容量/期限设置。

普通准备、hash和每次ReadChunk现在检查实际借用的文件/目录FD；返回的约束引用只在实际FD成功Close后释放，取消本身不释放，晚加限制不可移除。默认nil hook保留原transfer API。app统一接入Enqueue/batch/ConfirmQueue、StartSend、Resume、Resend、文件剪贴板发送和临时分享源重建；调用方预先准备的对象也必须追加同一拒绝约束。

当前profile的clipboard-files私有树不能直接或通过实际目录别名进入普通源发送。选择父目录且明确排除该子树仍可发送普通文件，不能只按字符串祖先一概拒绝。旧记录Open/Reveal、重发、打开任务目录及preview相对条目也经过拒绝门，excluded条目不暴露可拼回缓存物理路径的名字。外部独立用户硬链接名字不被清理器认领，原用户文件及缓存正文均保留。

Windows用GetFinalPathNameByHandle检查实际打开对象；其他平台核对解析后的路径和FD仍为同一对象。**ValidateExternalFilePath校验结束到OS按名字重新打开之间仍有窗口**；本增量不建立OS异步读取租约或最终无路径出口证明，不能因此启用Native回收。真正pathless Shell publisher、watcher代次竞争、完整inventory及实际原生引用终止继续后续实现。

最小验证与来源：

- transfer实际FD拒绝、父目录/排除、sticky约束、取消及成功Close后释放的定向选择exit0 / 3.207s，相关vet通过。
- app三个自有文件系统用例一次exit0 / 7.188s：普通父目录/排除与ReadChunk、旧history/重发/零新队列、真实Windows junction别名拒绝，正文保留；最后仅补public.txt仍出现在preview的断言，重验该一个用例exit0 / 4.754s。
- core app仅编译exit0、core vet exit0、desktop独立模块GOWORK=off/CGO0仅编译exit0；无额外完整矩阵、race、GUI、系统剪贴板或网络旅程。
- `.artifacts/r2-cache-export-gates/`保存app-first.log、preview-final.log、core-compile.log、core-vet.log、desktop-compile.log及20代码文件逐项SHA冻结清单freeze.json。独立只读审阅未发现阻断本拒绝门增量的问题；结论限此范围。

最终准确提交CI单独核对；后续C++ Shell primitives工作不包含在本次冻结代码清单中，不把源码编译或拒绝门测试冒充完整R2完成。

实际命令（加载既有`.tools/use-desktop-toolchain.ps1`，以下均`GOWORK=off`；对应日志与退出码见上文）：

```text
# D:/apps/Osend
go test ./internal/app -run '^$' -count=1
go vet ./internal/app ./internal/transfer
go test ./internal/app -run '^TestClipboardCacheExports' -count=1 -timeout 60s -v
go test ./internal/app -run '^TestClipboardCacheExportsOldHistoryAndPreviewMetadata$' -count=1 -timeout 60s -v
# D:/apps/Osend/apps/desktop, CGO_ENABLED=0
go test . -run '^$' -count=1
```

transfer子代理另报告实际选择`^Test(SourceAccess|SourceSelectionPreviewPrepareAndResumeAgree)`、package `./internal/transfer`、count1/timeout60s/-v，4项通过3.207s；该命令的shell原句未在root日志中，来源明确为子代理报告，不重新运行来伪造原记录。
