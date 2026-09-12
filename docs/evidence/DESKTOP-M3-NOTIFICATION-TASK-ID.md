# M3 真实任务 ID 的通知兼容修复

2026-09-12；独立分支 `codex/desktop-notification-task-ids`，基于发布候选 `408287c`。

原验证器只接受字母、数字、短横线和下划线，因而把真实 core ID `20260912T065442.536203100Z-00000001` 拒绝为无效通知；点击解析又按所有 `.` 分割，无法找回完整任务 ID。

现在允许不透明 ID 内部的点（最长 128 字节），仍拒绝路径/命令字符、空白、控制字符、前后点和 `..`。通知 envelope 继续采用 `linksend.<task ID>.<revision>`；先核对精确 prefix，再只按最后一个点拆分正整数 revision，检查 uint64 溢出和规范十进制。可用 UserInfo 的 task_id/revision 必须是相符字符串；缺失的旧 UserInfo 仍兼容。点击仅回传原任务 ID，后续仍由 core 查任务，不解释路径或命令。

实际通过：desktop `GOWORK=off go test -race ./... -run '^TestNativeNotification' -count=1 -v` 的 6 组测试、desktop 独立完整普通测试和 vet，以及根 `GOWORK=off go test ./...`。新增真实 core ID 提交→callback 往返、含多处点的 opaque ID、并发去重、正 revision 最大值、溢出/前导零/错误 prefix、恶意字符、长度及 UserInfo 类型/值不匹配负例。

Windows opt-in harness 实际运行 Wails beta.18 / Go 1.27.1 / WebView2 152.0.4191.66：

- 应用名 `LinkSend Native Test 252748-18d4825f97f61ed4`，与正式应用隔离。
- 使用 core 相同时间格式生成 `20260912T073139.084089900Z-00000001`，真实通知 API 返回 `submitted / permission unknown`。
- PID 252748 正常 exit 0；托盘和防睡眠 adapter 同时完成释放。
- 独立复核 AppUserModelId、通知类别、指向该测试 EXE 的 CLSID 及本次默认 activator 都不存在，进程已退出；未修改正式应用的注册项。
- 未请求用户点击。真实鼠标点击→core 导航仍属主程序完整验收；本次 callback 解析由自动测试验证。submitted 仅表示 API 接受，不声称系统一定展示通知。

测试 EXE SHA256：`2c407a7dfb03890b1a0c078a61319203c25fe240a139b15646974b0a1a05b4e4`。它是本修复源码的测试构建，不是 Release 包。裸测试 EXE 缺 icon resource 3，保留真实 warning；通知提交成功不等于产品图标验收。

日志、exit code 和资源复核记录在 `.artifacts/desktop-six-features/m3-notification-id-native/` 的 `stdout.log`、`stderr.log`、`exit-code.txt`、`verification.json`。最初用 PowerShell registry provider 逐项复核过慢，已中止该只读查询；随后使用 .NET registry API 完成同一拥有关系检查，无删除用户注册项的命令。
