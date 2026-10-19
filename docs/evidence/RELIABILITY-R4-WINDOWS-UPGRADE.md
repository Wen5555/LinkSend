# R4 Windows 覆盖安装与原生诊断（2026-10-19）

环境：Windows amd64，PowerShell 7；隔离安装目录及两端 profile 位于 `.artifacts/reliability-20261019/r4-windows-01/`。这是同机认证 QUIC 旅程，不是物理 LAN 或跨 NAT 证据。R4 双平台完整验收仍未完成。

## 准确来源

| 对象 | 来源 / SHA256 |
|---|---|
| 发布基线 | `63c86c2c5ce183e1400d27ca0571610d50a42bdc`，0.6.2 |
| 基线安装器 | `d4e5d364d33c5b66fe57331fe588a9772d46874f5d88fd22e5b510040fdd2651` |
| 基线安装后 EXE | `20d90832880a193d25f83b0ff36ade4c25a1c821f01ae9232d4a05cb61972899` |
| 候选 | `01b5c16c74b2867f1bb349d3c8fdb287932fb1b4`，0.6.2 |
| 候选安装器 | `5c2ebd61d224ef8e910da4559b66d1ea626d34bbaf60692ccd6e671d7e7a6fd3` |
| 候选安装后 EXE | `1845f510eb293fbe7e4d6b8a48b6802694c810057c9d08ba26e98f355967152d` |

候选 core [39175664054](https://github.com/Wen5555/LinkSend/actions/runs/39175664054)、desktop [39175663979](https://github.com/Wen5555/LinkSend/actions/runs/39175663979)、三平台包 [39175664039](https://github.com/Wen5555/LinkSend/actions/runs/39175664039) 均 success，Windows/Mac arm64/Mac amd64 包 job 全部通过。下载 artifact digest、BUILD-INFO、版本与内部 payload 核验见本地 `packages-01b5c16/PACKAGE-VERIFICATION.json`、`artifact-verification.json`，未发布 Release 或合并 main。

## 实际旅程及证据

- 受管 `windows-install-transaction.ps1 -Action Backup|Install|Restore` 保存并比较 HKCU 64 位卸载登记以及桌面、开始菜单、SendTo 三个精确快捷方式；每次 Install 校验安装器与安装后 EXE，并记录事务 intent。两次 `/S /D=<isolated-root>` 安装及最后 `/S` 卸载退出码均 0。
- 基线真 GUI 保存名称“R4 覆盖升级验收”、`retain_transfer_queue=true`；中文三文件与空目录经显式接收确认后完成，三个 SHA256 匹配。文件选择器选入中文草稿，后续取消保持；原生首次关闭 Escape 取消保持偏好与草稿。
- 第一次 256MiB 真实传输已完成，两端终态完成，不能当作活动退出。第二次同一自有源以 256KiB/s 限速重新发送，显式接收后 GUI 已验证 4MiB，随后 8/16MiB。真实 Alt+F4 → 退出应用 → 继续任务，进程和草稿仍在、正文继续增长；第二次 Alt+F4 → 保存并退出正常结束，在落库时保留 20MiB 已验证数据。没有强杀或伪调确认回调。
- 覆盖到候选前后逐项比较 identity、trust、preferences、task-history SQLite 文件哈希，安装器没有改动 profile。候选真实启动、第二实例正常退出并唤回原窗口；原身份/名称、中文草稿、两条完成记录及原任务 ID 的 recovering/20MiB/可恢复入口保持。没有用旧备份覆盖新业务写入，任务库仍 schema12。
- 候选原生设置 → 导出本地诊断 → Windows SaveFile，指定隔离目录保存成功；报告 1565B，SHA256 `8faea8a0ddd4ea098e7198d7f8ab0cdb317a62d93d1031ba88fcee2249e676bf`，可解析 JSON，三条临时编号任务，`relay_available=false`，没有驱动器路径、对端身份或源文件名。第二次指定另一唯一文件名后真实 Escape 取消，界面“已取消导出”，目标不存在。
- 候选原任务 → 展开“连接与任务诊断” → “打开诊断与导出”，真实导航至设置诊断分类。旧记录尝试时间与方法准确显示未知；成功历史未沿用连接错误。发现恢复阶段仍显示 `restart_recovery_available`，后续修复单独验证，不能倒记候选已有修复。

本地原始证据在 `native/`：`baseline-functional-checkpoint.json`、`m4-peer-subset-result.json`、`before-baseline-exit-peer.json`、`baseline-exited-profile-hashes.json`、`candidate-process.json`、`candidate-second-instance.json`、`candidate-restored-accessibility.txt`、`candidate-native-diagnostics.json`、`candidate-diagnostics-verification.json`、`candidate-export-cancel-accessibility.txt`、`candidate-task-diagnostic-accessibility.txt`、`candidate-task-to-diagnostics-accessibility.txt`、`active-peer-final.json`。原生进程日志与早期失败保留，私有 profile 不提交。

## 收尾和准确边界

候选通过真实“保存并退出”正常关闭；受管发送端通过自有 stop 文件正常结束。原协调夹具有 30 分钟期限。续起同库协调服务的命令被自动审批拒绝（仅返回 `blocked by policy`），没有换入口重试；准备的替代 helper 未运行。原协调夹具随后自行到期，信令 health error 属于明确测试环境到期，不记产品故障。

最终 `windows-install-transaction.ps1 -Action Restore` 退出0，输出 `RESTORED_VERIFIED`，事务 stage=restored：测试 EXE 已由受管卸载器移除，原卸载登记及三个快捷方式逐项与测试前快照一致。隔离 profile、源文件、恢复数据及证据保留，不递归清理；不影响日常安装/profile/剪贴板或宿主网络。

覆盖安装、状态保持、基线活动保存退出、候选原生诊断旅程通过；候选活动传输退出后完整续传、SendTo/拖放实际发送、Mac 替换和活动退出仍 NOT_RUN / BLOCKED_BY_EXTERNAL_ENV。R1 物理双机、睡眠与跨网、R2 系统键盘粘贴及完整自动回收、R5 临时分享与 R6 收尾均仍必做，不能由本记录抵扣。
