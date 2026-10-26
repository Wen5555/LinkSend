# v0.7.0 收尾检查

日期：2026-10-26。用户选择v0.7.0，要求最小功能验证、及时推送与GitHub发行收尾。最终CI和发行资产以正式Release manifest为准；本页保留准备阶段的失败与范围。

## 版本一致性

产品版本统一0.7.0，Windows固定四段版本0.7.0.0，文件协议V1。同步根模块、前端及md5、Wails资源、NSIS、Appx及Mac bundle；不运行覆盖定制安装逻辑的build-assets更新。首次 `GOWORK=off go test ./internal/protocol -count=1` 发现遗漏Appx旧0.6.2.0，修正后相同检查PASS。前端首次调用默认旧pnpm入口因无TTY拒绝install，未产生成功结果；随后使用 `.tools/use-desktop-toolchain.ps1` 的既有工具链仍遇node_modules/eslint访问拒绝，未修饰为PASS；不删除或重建用户依赖目录，最终由准确干净CI完成前端typecheck/build。

## inbox fixture 的时间前提

5ad Windows packages run `40287412178` 停于 `TestPairingCodePersistentInboxAndAlwaysAccept`，6.99秒后反向传输出现新SessionID。产品允许3秒空闲后关闭；原日志缺少timer时间线，根因保持INCONCLUSIVE，不能证明该次由到期造成或称产品bug已修复。

仅 `internal/app/inbox_test.go` 调整fixture：每轮真实双端session完成并idle后，停泊既有timer；下一轮仍使用原acquire/receive路径，保持相同session、两个WSS和授权断言。撤权前恢复原 `directSessionIdleTTL`，核本端撤权关闭和远端自然回收。不改生产TTL、状态标志或安全CI门。

首次单例遗漏恢复原timer，三次传输/复用通过但远端回收FAIL（5.83秒）；保留 `.artifacts/release-070-closeout/inbox-fixture-first-restore-needed.log`。补齐恢复后仅重验该用例，PASS（用例3.76秒，包10.879秒）：

```text
GOWORK=off go test ./internal/app -run '^TestPairingCodePersistentInboxAndAlwaysAccept$' -count=1 -timeout 45s -v
```

freeze：`.artifacts/release-070-closeout/inbox-fixture-freeze.json`；最终源SHA256 `35e043013d226719abb7ae7fddeb3822d9599efd51926016ed9d083280f1431c`。该最小单例不是原CI精确根因证明，最终准确提交仍须CI。

## 发行边界

不再运行本机GUI/clipboard/安装旅程，不再扩展Shell/自动回收实现。one-shot OLE探针有效运行0次，撤flag，NOT_RUN。现有生产5ee公网分享通过、真实Windows↔Linux证据各自绑定原提交；新0.7资产只报告真实准确源码CI和静态核验。R0–R6整体未完成，Native删除关闭，详细缺项保留在唯一TODO。
