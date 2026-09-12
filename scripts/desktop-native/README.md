# Windows 原生验收入口

PowerShell7、Go1.27.1、已构建的Windows Wails EXE与WebView2。脚本使用真实UIAutomation、系统选择器、Explorer、独立SQLite/身份及真实loopback ICE/QUIC；不会修改主机网络配置，不能当作物理LAN/双NAT结果。

这组源码从2026-09-12实际使用的隔离验收工具整理；`m1-native-tests`只是沿用fixture名称，可验证后续版本。自动生成的EXE、profile/密钥/接收文件和证据目录都被本目录.gitignore排除。不要提交run目录。

在仓库根目录执行，每条命令应核对退出码：

```powershell
go build -o scripts/desktop-native/m1-native-tests/fixture.exe ./scripts/desktop-native/m1-native-tests/fixture
go build -o scripts/desktop-native/m4-native-peer/peer.exe ./scripts/desktop-native/m4-native-peer
$binary = (Resolve-Path apps/desktop/bin/LinkSend.exe).Path
$hash = (Get-FileHash -LiteralPath $binary).Hash.ToLowerInvariant()
pwsh -NoProfile -File scripts/desktop-native/m1-native-tests/start-native-session.ps1 -BinaryPath $binary -ExpectedSha256 $hash -SourceState '填写精确commit及dirty状态'
```

把输出的SESSION_FILE作为下一条参数：

```powershell
pwsh -NoProfile -File scripts/desktop-native/test-m4-m6-native.ps1 -SessionPath '<SESSION_FILE>'
pwsh -NoProfile -File scripts/desktop-native/m1-native-tests/stop-native-session.ps1 -SessionPath '<SESSION_FILE>'
```

测试原生选目录、选收、冲突保留两份、空目录、全跳过、中文检索、Explorer选中实际文件和删记录保文件。测试窗口会暂时切到前台，只按本次PID/HWND/路径定位，不操作旧LinkSend窗口。保存原始FAIL日志；`-SkipTransfers`只适用于此前两次真实传输均已完成、继续验收磁盘/收件箱的场景，不能将失败传输跳成PASS。

清理脚本仅结束本次记录且启动时间吻合的GUI/fixture；退出失败会报告非零，不删除接收文件或profile。请保留待检查证据，手工确认不再需要后再清除。
