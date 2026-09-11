# Mac 原生冒烟测试单

以下步骤适用于产品 `0.2.0` 的对应架构 DMG。CI 编译、DMG 挂载、浏览器截图或仅创建原生窗口均不能替代真实交互验收。

1. 核对资产清单中的产品版本、commit、架构、大小和 SHA256；挂载 DMG，将测试副本放入独立位置，不覆盖既有正式应用。
2. 启动后核对设置/诊断中的 `version=0.2.0`、`protocol_version=1`、`relay=false`；使用独立 profile 和一次性邀请，逐字核对双方完整指纹。
3. 实际操作原生文件和目录选择器，覆盖取消、中文/空格路径、权限拒绝；完成传输后点击打开接收目录。
4. Windows→Mac、Mac→Windows 分别测试空文件、多文件、空目录、大于两个 chunk 的文件；接受前不得传正文，完成后比较 SHA256、verified/committed/bilateral-confirmed 状态。
5. 分别测试拒绝、冲突、取消和 deadline；稳定错误与界面下一步建议应一致，原文件不得被覆盖。
6. 在发送首块、部分块、Verifying、部分文件已提交和 receiver committed/sender unconfirmed 附近执行暂停、断网或强杀。重启后必须保持 task/TransferID，创建新的 attempt/session/ICE generation，重新确认后只请求缺失或损坏块，并记录实际发送/接收/重传/唯一验证/提交字节。
7. 活跃任务期间实际点击 macOS 红点并按 Cmd+Q；验证退出保护。空闲时两种方式均应退出。不得绕过 TCC/辅助功能权限来伪造点击证据。
8. 验证 Tab/Shift+Tab/Enter/Esc、可见焦点、系统字体、深色模式、高 DPI、窄窗口与“减少动态效果”。
9. 重启后检查历史列表、Paused/Recovering 入口、损坏记录隔离和不可恢复任务的明确说明。

截至 2026-09-11，macOS arm64 的窗口创建与 idle quit Apple Event PASS 来自历史 dirty r2 测试快照；committed `0.2.0` DMG 在 GitHub runner 的挂载、plist、架构和 strict ad-hoc codesign 为 PASS，但物理 Mac 在候选收尾时 SSH timeout，未重新启动该包。红点实际点击、Cmd+Q 按键、原生对话框、活跃任务保护和 GUI 恢复入口仍为 NOT_RUN；Intel Mac 启动仍为 NOT_RUN。
