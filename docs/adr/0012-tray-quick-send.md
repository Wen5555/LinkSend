# ADR0012：托盘快速发送与独立草稿

日期：2026-10-18。状态：源码候选与本地自动测试通过，原生托盘/菜单栏与物理验收另列于[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)。证据见[N4记录](../evidence/DESKTOP-AE-QUICK-SEND.md)。

Windows托盘/macOS菜单栏增加“快速发送文件”，显示既有应用窗口中的轻量对话面板。复用同一个Wails服务/profile owner，不创建第二个应用实例、全局快捷键或第二套传输队列。主窗口隐藏/后台时先唤起，再用单调revision请求打开面板；重复点击只重开同一面板。

macOS菜单栏显示`LinkSend`文字，并保留Wails beta.18的主点击菜单行为；“显示 LinkSend”由菜单明确提供。Windows主点击继续唤回主窗，右键打开菜单。已核对锁定依赖`systemtray_darwin.go:processClick`与`setLabel`，不增加系统权限。文字/主点击改动的准确包仍须独立验证。

独立`send_drafts`行ID为quick，主草稿main保持。路径/目标/revision仍由Go保存与CAS核对；快速面板首版单个已配对目标、默认完整内容，不继承主草稿的多目标或筛选。取消仅关闭面板，quick未发送草稿保留；明确入队成功后才清空quick路径，主草稿不变。文件选择器取消不清已有选择，源文件不复制到JS/IPC。

最近/常用设备复用Go已配对目录的pin/last-used排序，可搜索完整identity；排序不产生新信任。离线目标需明确等待，撤权和源变化仍受原Enqueue/派发屏障。一次入队稳定request ID，CAS失败/响应丢失不静默重复；取消面板不冒充取消已提交队列。

桌面命令为OpenQuickSend、QuickSendDraft、SaveQuickSendDraft、EnqueueQuickSend；主窗口通过typed quick-send:open事件及时打开面板，BackgroundStatus.quick_send_revision轮询可补偿错过事件。旧事件revision不重复打开；单个App状态只渲染一个面板，来件确认优先。面板查询按草稿revision拒绝旧快照覆盖刚保存状态，Tab/Shift+Tab闭环与Escape/关闭恢复焦点。

入队命令只携带quick草稿revision和等待选择，Go读路径与peer，并以quick-send-v1加revision派生稳定请求ID。同revision改变等待策略与已提交请求冲突，未提交离线失败后可改为明确等待；清空路径或重启后仍返回原队列收据。并发重复只生成一项，底层全部预检/源hash/派发/取消仍归现有队列。JS不持有文件正文。

界面、Go草稿/队列、重复打开/提交、取消、离线/撤权、主草稿并存、重启与实际同机QUIC文件通过；后台原生入口单独待验。任务库11、wire/预算/身份与relay=false保持；签名Share Target/Share Extension仍按原范围暂缓。
