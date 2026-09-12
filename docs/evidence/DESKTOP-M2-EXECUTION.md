# M2 系统入口与单实例接线

当前阶段实现窗口拖放、Windows 当前用户 SendTo 和 macOS Finder Services 的草稿入口。
系统入口不授予发送权限，也不直接调度传输。Share Target、Share Extension 和图标拖入不属于本阶段已实现入口。

## 所有权与失败恢复

`application.New` 之前按实际 profile 的规范路径建立 Wails 单实例键；core 的内核 profile 锁仍保护唯一 SQLite/调度 owner。
第二进程使用自己的 WorkingDir 解析路径。Wails beta.18 即使通知首实例失败仍会退出，
因此启动入口先写 `desktop-activations-v1` 元数据日志，首实例在通知或每秒兜底扫描时消费。
日志不打开 identity/trust/SQLite，也不含文件正文。

单次最多 1024 路径/1 MiB 编码，待处理最多 64 条/16 MiB。
生产者使用独立内核文件锁；记录先 fsync、再命名；首实例只有在 `MergeDraftPaths` 事务成功后才删除记录。
崩溃重放按路径去重；数据库失败保留记录，未来/损坏格式保留并显示问题，不能假报“已加入”。
Unix 还同步目录；Windows payload 使用 FlushFileBuffers，但不承诺突然断电时目录元数据绝对持久。
启动参数或落盘失败在退出前显示系统原生错误。

草稿数量/大小预览只读取 Go 文件元数据，正文与哈希仍在显式入队/派发时验证。
预览设时间和条目限额；不可读/超限显示未完整统计，不把估算当传输承诺。

SendTo 只修改本用户的固定入口，使用 marker 与目标 EXE 双重核对，不覆盖另一安装的入口。
安装器为 user scope；卸载先调用当前 EXE 的 `--uninstall-sendto`，只删除自身安装文件与空安装目录，
保留用户 profile、WebView 数据和收到的文件。
Finder provider 在实际 WindowRuntimeReady 后注册，退出前解除；持久回调错误传回 Finder。
临时/受限 URL 显式拒绝并要求复制到普通本地路径，不能保存随即失效的源。

## 已执行验证

- Go 桌面独立 `GOWORK=off go test -race ./...`、mod verify、vet PASS；原生平台 adapter 见 [专门记录](DESKTOP-M2-NATIVE-ADAPTER.md)。
- 激活日志测试覆盖 20 个并发生产者、失败事务保留、重启消费、64 条上限、损坏/future 记录保留和目录 symlink 拒绝。
- Windows Wails production build、前端 typecheck/独立 bindings/lint/29 tests/build PASS；后续预览字段曾触发 TypeScript nullable 检查，修正后重跑。
- Windows 真窗口/UIA 与 SQLite 只读核对：同时两个入口、Unicode/空格/相对路径、重复去重、隐藏窗口后再次唤回并可见草稿，4 个第二实例退出均为 0。
- 独立第二 profile 与首 profile 同时运行；冷启动参数进入草稿；正常关闭、同 profile 重启后草稿 revision 和路径保持，均退出 0。
- 已停止本轮所有夹具和测试 GUI，没有向用户设备发送内容。

Windows 原生入口验证的具体未提交快照：EXE SHA256
`9320d7dbae9eddf3c8826abecd33ef6472b65142be10e69e9c61155b3fc6ec0d`。
证据 `.artifacts/desktop-six-features/m1-native-tests/run-20260912T060321109Z/`，
脚本 `test-m2-entry-integration.ps1`、`test-m2-cold-entry.ps1`。
该哈希只属于验证时快照，不冒充后续精确提交的 CI 资产。

## 尚待原生/跨功能联调

真实 Explorer 发送到菜单、窗口 OLE 拖放、Finder 菜单触发的完整最终包流程、忙于正文传输时入口、
最终安装/卸载以及完整 Mac 应用接线尚需单独记录。平台 callback 单测、COM `.lnk`、真实 NSPasteboard provider 测试
不能替代这些整机工作流。M3–M6 合并时继续完成相应联调，不把尚未运行项填成 PASS。
