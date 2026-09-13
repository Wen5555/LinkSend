# E0 验证工具

这些是开发原型和验证工具，不是正式共享功能。没有模拟设备列表，没有网络文件发送，不能替代 E1/E3。

- `inspect-{mac,hk,nl}.sh`：只读主机/工具/拓扑。nl 仅创建临时 user/net namespace 做能力检查，不改宿主网络。
- `control-inspect`：HTTPS health → WSS 认证 → 已认证设备列表。需要**当前没有 GUI/WSS 使用的身份**，新的 WSS 会替换同身份连接。使用隔离目录中的受保护测试身份；不输出密钥/邀请码/设备全名，不修改授权或发送正文。
- `windows-share-prototype`：.NET9 WPF + Windows AppInstance/ShareTargetActivatedEventArgs/StorageItems；真实 MSIX，独立包标识。只接管最多128个、总实际64MiB的小 fixture，持久回执分 captured/completed/failed；正式 E3 不能照搬全文件复制策略。
- `mac-share-prototype`：Swift/Cocoa host + 真 `.appex`，使用 `_NSExtensionMain` 可执行入口、sandbox/App Group entitlements。最多1个/1MiB fixture，系统 file representation completion handler 内接管；真实系统唤起结果另存。ad-hoc 不能证明正式签名交付。

Windows 构建：`dotnet publish scripts/desktop-experience/windows-share-prototype/LinkSend.SharePrototype.csproj -c Release --self-contained true -o .artifacts/desktop-experience/e0/windows-share-package`。
复制 AppxManifest.xml 和既有 appicon.png 到 stage/Assets/Logo.png，再用已安装 SDK makeappx pack 与开发证书 signtool sign。
安装需要系统接受包签名；当前用户 TrustedPeople 不足，不能通过自动修改机器安全策略绕过。

Mac 脚本使用显式 E0 task root；必须通过 codex-ssh-manager 上传/run-script，保留 jobPath。
初次 build wrapper 不覆盖已有目录；重建 wrapper 仅在明确需要修补原型时使用。断线先读取已有日志/exit-code，不重新执行。
manager resume-job 在该 Mac zsh 下出现 `read-only variable: status`，可用 manager download 读取既有 stdout.log/stderr.log/exit-code，不代表原作业失败。

`activate-mac-prototype.sh` 的 NSSharingService 是真实系统扩展激活，**不是 Finder 人工菜单点击**。系统 Accessibility/签名/授权缺口必须单列。工具不读取或写入用户日常剪贴板。
脚本应仅用于此 E0 的受控 fixture；不是通用接收任意外部文件的产品组件。
