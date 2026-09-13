# E2 默认一次接收证据

候选源码在 `codex/desktop-experience-upgrade`。桌面确认框默认只显示设备、前三个文件、总项数/大小、保存目录和免确认选项；“查看详情”才展开目录、同名策略和选收。普通“接收文件”调用 Go `AcceptIncomingDefault(task_id, attempt_id, revision, remember)`，Go 在同一命令中重验任务/attempt/revision、授权 generation、目录和空间，按 `keep_both` 构造计划，持久化计划后才发出同意。恢复任务继续使用已持久化计划，不重新生成名称或扩大选收范围。

`TestAcceptIncomingDefaultIsAttemptBoundAndKeepsBothOverRealQUIC` 使用真实 loopback ICE、TLS 1.3 和 QUIC：错误 attempt、错误 revision 和第二次点击均拒绝；目标中原 `same.txt` 内容保持不变，新正文写入同名兄弟文件，双方完成且计划摘要已保存。`TestDefaultAcceptanceReportsPreferenceFailureWithoutReversingAcceptance` 验证本次接收成功后，免确认偏好写失败只返回独立提示，不反转本次 acceptance。既有 `TestIncomingSelectionRemainsBoundDuringResumeOverRealQUIC` 继续覆盖恢复沿用原计划。

自动测试不是物理网络或准确安装包验收；E1 已记录真实 Windows→Mac 文件通过与反向入站失败，本轮没有改写该结论。

## cdb9577 审查修复

默认计划不再写死同名策略。Wails 将持久全局设置及设备覆盖同时用于普通确认和免确认 inbox；优先级为设备覆盖、全局、`keep_both` 安全默认。策略变化只作用于新计划，恢复继续沿用原计划。
