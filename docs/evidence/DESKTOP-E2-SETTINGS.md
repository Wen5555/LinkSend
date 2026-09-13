# E2 分类设置证据

设置页按通用、文件接收、系统集成、网络与发现、存储与记录、诊断与关于分类。桌面偏好增加单调 `revision`；旧 format 1 文件缺少 revision 时在内存迁为 revision 1。新 `SavePreferencesSection` 用 expected revision 做 CAS，并只合并该分类拥有的字段。陈旧表单得到 `PREFERENCES_REVISION_CONFLICT`，不会覆盖较新的其他分类；旧 `SavePreferences` 继续兼容，背景设置仍使用独立命令。

`TestPreferencesSectionSavePreservesUnownedFieldsAndRejectsStaleRevision` 和 `TestPreferencesSectionSaveAllowsRetryWithoutLosingEarlierCategory` 验证局部合并、陈旧拒绝和刷新后跨分类保存。`TestSettingsFormCannotUndoNewBackgroundPreference` 继续验证旧表单不能回滚背景设置。设备页已有 revision CAS 的本机别名、我的设备、固定顺序与专属接收目录；目录留空即继承全局目录，健康中的任务继续绑定原保存位置。

网络分类保存会影响后续连接并重启 inbox 配置；既有真实 QUIC 测试验证活跃接收仍写入原目录。本轮没有声称无需重连即可改变既有连接路径。
