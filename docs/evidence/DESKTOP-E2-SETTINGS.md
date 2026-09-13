# E2 分类设置证据

设置页按通用、文件接收、系统集成、网络与发现、存储与记录、诊断与关于分类。桌面偏好增加单调 `revision`；旧 format 1 文件缺少 revision 时在内存迁为 revision 1。新 `SavePreferencesSection` 用 expected revision 做 CAS，并只合并该分类拥有的字段。陈旧表单得到 `PREFERENCES_REVISION_CONFLICT`，不会覆盖较新的其他分类；旧 `SavePreferences` 继续兼容，背景设置仍使用独立命令。

`TestPreferencesSectionSavePreservesUnownedFieldsAndRejectsStaleRevision` 和 `TestPreferencesSectionSaveAllowsRetryWithoutLosingEarlierCategory` 验证局部合并、陈旧拒绝和刷新后跨分类保存。`TestSettingsFormCannotUndoNewBackgroundPreference` 继续验证旧表单不能回滚背景设置。设备页已有 revision CAS 的本机别名、我的设备、固定顺序与专属接收目录；目录留空即继承全局目录，健康中的任务继续绑定原保存位置。

网络分类保存会影响后续连接并重启 inbox 配置；既有真实 QUIC 测试验证活跃接收仍写入原目录。本轮没有声称无需重连即可改变既有连接路径。

## cdb9577 审查修复

全局同名策略现可持久化 `keep_both|skip|error`，默认仍为 `keep_both`；设备 profile schema 7 增加可留空继承的覆盖字段。普通确认和免确认接收都解析设备覆盖→全局→安全默认，已有活动/恢复任务继续使用既有计划。

设置编辑按字段记录 dirty。后台新 revision 只更新未编辑字段，dirty 输入原样保留并要求显式合并；保存一个分类只清该分类字段，其他分类草稿继续保留，旧 props revision 不会回滚新响应。Playwright 实际输入名称与 `skip` 后模拟后台 revision，二者均保留且保存被门闩阻止，见[截图](assets/e2/e2-c1-settings-dirty-960x640.png)。Go CAS、schema6→7 备份迁移和前端字段级合并均有回归。
