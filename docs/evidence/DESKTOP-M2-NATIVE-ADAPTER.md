# M2 原生入口 adapter 实现与定向验证

日期：2026-09-12。此记录是未接入 main/app 的平台 adapter 验证，不能称系统入口到持久草稿的完整产品验收。源码未自行提交、没有注册用户正式安装的 SendTo，也没有把 Services 菜单声明单独提前发布。最低 macOS 13 / Go 1.27.1 是本轮用户的最终明确决策。

## 接入契约

- `profileSingleInstanceOptions(dataDir,onSecond) (*application.SingleInstanceOptions,error)`：先解析绝对路径和现有目录别名，按规范路径 SHA256 派生 UniqueID，失败明确返回；不提前创建 profile 或打开数据库。Windows case folding 和 extended UNC 形式统一；Darwin `F_GETPATH` 取得真实目录路径，防止 APFS 大小写别名拆成两个 instance ID。UniqueID 的摘要段以字母 p 开头，也满足 Linux D-Bus 名称要求。
- 二次启动回调收到 **不含 argv[0]** 的参数副本和启动进程的 WorkingDir；首次启动应给 `parseNativeFileArguments(os.Args[1:],cwd)`。Wails beta.18 的 EncryptionKey 保持双方一致的默认零值；固定源码明确零值传原 JSON，不生成不同随机密钥。
- `parseNativeFileArguments` 支持普通文件参数、`--send-files --`；未知 option 明确拒绝，`--` 后可接以短横线开头的真实文件名。相对路径使用所给 WorkingDir，Windows 不猜测另一 drive 的当前目录。每次最多 1024 条路径、1 MiB 路径字节；超限/空值/NUL/无效 UTF-8 整批失败，没有部分接受或截断。
- `attachFileDrop(window,onPaths)` 接真实 `events.Common.WindowFilesDropped` / `DroppedFiles()`，返回取消订阅函数。main 必须设置 `EnableFileDrop=true`，前端目标需 `data-file-drop-target`。全部数据都是原生路径元数据，不读取 WebView 文件正文。
- 所有回调只准备可见草稿，不选择目标、不发送、不自动恢复旧任务。无文件二次激活仍应由 main/app 唤醒现有窗口；重复路径由 Go 草稿服务去重。

## Windows SendTo

`installSendTo(executable)` / `uninstallSendTo(executable)` 通过 `windows.KnownFolderPath(FOLDERID_SendTo)` 定位当前用户目录。真实 Shell Link 由 Windows 的 `WScript.Shell` COM adapter 写属性，未调用 Run/Exec 或执行用户路径字符串。COM 生命周期在独立锁定的 OS 线程上完成。

快捷方式 `LinkSend.lnk` 的 TargetPath 是真实 exe，Arguments 固定 `--send-files --`，WorkingDirectory 是 exe 目录，Description 带 `com.linksend.desktop.sendto/v1` 所有权标记。临时快捷方式完成后，以硬链接创建最终名字，原子 no-replace；已有第三方条目、目录/符号链接、不同安装路径的条目都保留并明确报错，不覆盖。相同安装重复注册幂等。卸载仅删除标记、参数、目标 exe 全匹配且文件 identity 未变的条目，不删除接收文件或其他 SendTo 项。另一安装路径的旧项不会被本安装擅自移除。

自动化只调用内部 `installSendToAt/uninstallSendToAt`，目录均为 `t.TempDir()`；创建的 exe 目标是无执行权限意义的测试内容，从未启动。实际验证包含 Unicode/空格路径的 COM 读回、重复注册/卸载、第三方 `.lnk` 内容保持、另一安装路径保持。Windows directory symlink 用例因缺少系统创建权限明确 SKIP；没有为测试开启开发者模式或绕过权限。

## macOS Finder Services

Darwin Go/CGo/Objective-C provider 使用真实 AppKit `NSApplication.servicesProvider`，Info.plist / Info.dev.plist 的 NSServices 发送类型为 `public.file-url`，消息 `linksendSendFiles`，菜单为“使用 LinkSend 发送”。这不是 Share Extension。当前工作树声明须与 provider 接入一起进入 M2，不提前作为 M1 功能展示。

`registerFinderServices(onPaths)` 必须在 AppKit 主循环已运行、草稿回调就绪后调用；Finder 可以在注册后立即发出请求。注册和 cleanup 通过 Wails `InvokeSync` 在 UI 主线程执行；不替换 application delegate，不覆盖其他现存 services provider，cleanup 幂等。主循环停止前须完成 cleanup。

provider 用 `NSPasteboard readObjectsForClasses:[NSURL class]` 读文件 URL，保留 Unicode/多文件/目录，检查文件可达性，只将 JSON 路径数组交给 Go。根本没有读取或传递文件正文。现代文件 URL 路径已足够，移除了 macOS 10.14 起废弃的 NSFilenamesPboardType fallback。

权限边界经过真实测试修正：`startAccessingSecurityScopedResource` 对普通用户文件也可能返回 true，不能把它等同于“没有持久权限”。当前包为 unsandboxed；adapter 释放 URL scope 后检查 app-sandbox entitlement，并用独立 POSIX open/fstat 确认普通文件/目录可读，不读取字节。依赖 sandbox 临时授权的来源明确拒绝并提示复制到本地，不存只在本次调用有效的 URL 当作持久来源。系统临时目录（含符号链接解析后的 `/tmp`、`/private/tmp`、NSTemporaryDirectory）明确拒绝并提示先复制本地。后续入队时仍需核心做源验证/快照；普通文件被移动、权限改变和重启恢复不能由这一瞬间检查保证。

## 实际验证与开发失败

Windows 在 desktop module、`GOWORK=off`、Go 1.27.1 下执行 `go test -race -run 'TestNative|TestSendTo' -count=1 -v ./...` 和 `go vet ./...`，最终两条命令均退出 0。8 个顶层测试中 7 项 PASS，目录 symlink 权限用例 1 项 SKIP；路径/参数/同 profile/不同 profile、drive-relative/UNC、真实 `.lnk`、所有权负例及 case alias 均实际通过。

Mac 初始 adapter snapshot 的底座是精确 M0 archive，另覆盖本次 adapter 文件，不伪称已提交 M2。首个 job `/tmp/codex-ssh/desktop-m2-native-adapter-20260912T045827Z` 退出 0：Go race 定向测试、vet、build、两份 plist lint、真实 AppKit provider 注册与唯一 NSPasteboard 的 Unicode 多文件/缺失文件/非文件/cleanup 测试均通过。该初稿的 scope 持有策略随后被更严格的持久访问规则取代。

中间版本 job `/tmp/codex-ssh/desktop-m2-native-final-c-20260912T050635Z` **退出 4**，普通文件被“scope 返回 true 就拒绝”的错误判断误伤；该 FAIL 没有抹去。修正权限判定后，最终 Objective-C 源码 SHA256 为 `4a07ad21e0abbec20fdec01077503b9a7d0f9f8cb99c3c5010ada078d96c38b2`，job `/tmp/codex-ssh/desktop-m2-native-final-v2-20260912T050859Z` 退出 0：

- 以 `clang -mmacosx-version-min=13.0 -Wall -Wextra -Werror -framework Cocoa -framework Security` 编译真实 provider 与独立原生测试程序，无警告。
- AppKit 注册成功；真正 NSPasteboard 的 Unicode 双文件/文件夹路径回调一致。
- 缺失文件、非文件剪贴板、真实系统临时文件均拒绝，Go 路径回调次数未增加。
- `F_GETPATH` 对 APFS 上同一目录的大小写别名返回相同 canonical 路径。
- cleanup 清空本 provider，其他 provider 不被覆盖。

最终全部 adapter 源码归档 SHA256 `bb08f59a389b8954f1571a16ccfe1442e9a2c6c262d8b06a5b8a3bc621625958`，上传后仅覆盖隔离 M2 adapter 工作目录；最终 Go 1.27.1 / CGo race 和 plist minimum=13 检查 job `/tmp/codex-ssh/desktop-m2-native-final-go-20260912T050957Z` 退出 0。Mac 的 5 项定向 Go 测试全部 PASS（包含真实目录 symlink 和 APFS 大小写别名），两份 plist lint 为 OK，读取最低版本为 `13.0.0`。Go test link 仅保留上游重复 `-lobjc` 的 warning，没有先前的 minimum OS 13/11 冲突；独立 Objective-C `-Werror` 编译没有警告。

以上原生程序直接使用真实 AppKit services provider 和系统 NSPasteboard，证明 adapter 行为；尚未用 Finder 的实际 Services 菜单发起，也未验证窗口关闭时的二次启动→持久草稿。因此完整 Finder 菜单、Windows Explorer SendTo→主进程草稿、运行/隐藏/忙时重复激活仍为待 main/app 接入后的 NOT_RUN。没有用浏览器截图或调用普通 Go 回调冒充这些入口验收。
