# Desktop E5 交付记录

## 2026-09-19 E5-A 0c59 发布包物理文件增量

`v0.5.0-desktop-preview.1` 保持既有 tag、发布资产与 PR #8；本批不重发 release、不移动 tag、不合并 main。`0c59ae255d631b496a3e483e9c558f2998ee314c` 的准确 Windows/macOS arm64 包已完成物理包→包双向 1 MiB 文件与反向连续 session reuse，完整来源、哈希、task/session、journal fixture 与系统激活边界见 [E5 验收记录](DESKTOP-E5-ACCEPTANCE.md)。

本批完成阶段证据、TODO、恢复提示与 Draft PR 同步；HK 继续运行已接受的 `387b57c` / schema 4，NL 双 NAT 结论与既有签名/公证/App Group 暂缓项均未改写。用户已授权并实际执行一次双端系统剪贴板最小尝试：首条 Windows→Mac 文本的 Mac 系统消费者断言失败，窗口已关闭，双方总开关与全部 grant 已关闭，未恢复剪贴板。链接、图片、反向、防回环和文件并存没有执行；精确命令、job 和收尾证据见 [E5 验收记录](DESKTOP-E5-ACCEPTANCE.md)。

日期：2026-09-14。当前源码候选提交：`1fa21b4073e12d40e87dc6e23f3ec4d67e5e1819`；完整准确包实机基线仍来自 `387b57c76596975d0da61f01e060e0cc2940b0b5`。

## GitHub 状态

- 工作分支：`codex/desktop-experience-upgrade`。
- Draft PR：[#8](https://github.com/Wen5555/LinkSend/pull/8)。
- core push `34792565801`、core PR `34792567181`、desktop push `34792565789`、desktop PR `34792567186` 均为 `success`。
- packages run `34792565786` 的 `windows-amd64`、`macos-arm64`、`macos-amd64` 均为 `success`；准确 artifact ID 和摘要见 [E5 联合验收](DESKTOP-E5-ACCEPTANCE.md)。
- Actions 仅有既有 Node 20 action runtime 被平台强制切换到 Node 24 的 warning；没有失败检查。
- 兼容诊断增量 `1fa21b4` 的 core push `34802741501`、core PR `34802743668`、desktop push `34802741511`、desktop PR `34802743662` 和 packages `34802741572` 均为 `success`；PR/远端 head 一致。新三平台 artifact ID 与外层摘要已由 GitHub API 核对，下载在暂停命令后停止，包内核验待恢复。

## 当前交付边界

`387b57c` 是 E4 源码验收通过后进入 E5 的固定候选。三平台 artifact 已下载并通过包内外摘要、BUILD-INFO、架构和 Windows ZIP 布局核验；arm64 DMG 已在物理 Mac 完成隔离启动与恢复检查，Windows portable 已完成 125% DPI 首次/二次启动、原生关闭选择持久化、5 次启动/内存基线和原生 min-size 钳制；M5/当前控制面双向拒绝已用 clean 二进制隔离验证。香港控制面和荷兰隔离双 NAT 已完成，准确包物理双向文件与系统剪贴板仍按 E5 事实矩阵推进。

当前没有合并 `main`，也没有创建正式 Release。用户已经授权本目标中的阶段 push、事务部署和 prerelease；若后续候选因 E5 缺陷变化，所有包、摘要、部署来源和本页提交号必须一起刷新，不能沿用 `387b57c` 的包级结果。

暂停收尾时无活跃本地或远程测试作业。packages run `34802741572` 的恢复下载目录为 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e5-ci-34802741572`，当前为空；下一次启动后下载三份 artifact 并执行 `scripts/verify-milestone-packages.ps1`。Mac 需唤醒或提供新地址，Windows 需退出 `Screen-saver` 后才复测原生 Tab，系统剪贴板需隔离可丢弃环境或明确临时覆盖条件；这些条件与逐项入口见 [E5 联合验收](DESKTOP-E5-ACCEPTANCE.md#暂停时外部条件与恢复矩阵)。

## 远程事务账本

| 主机 | 类型 | 状态 | 证据/恢复 |
|---|---|---|---|
| `mac-test-102342413` | 准确包隔离验证 | PASS/PARTIAL | 完成 job `...004708Z`、失败保留 job `...004906Z`、修正后完成 job `...004932Z`；未替换 `/Applications` |
| `hk-main` | 生产控制面 | PASS | 候选 `2c9c352a...6372`、PID 553926、DB schema 4；部署 job `...014829Z`，独立 verify `...014925Z`，备份 `/opt/linksend-lan-test/backups/20260914T014842.005473007Z-desktop-e5-387b57c` |
| `nl-highdefense` | 隔离双 NAT | PASS | 8 MiB、同 session、`relay=false`；run `...022524Z`，独立 verify `...022658Z`，脱敏包 `4be226d1...b86b` |

香港首次只读脚本 `/tmp/codex-ssh/linksend-e5-hk-readonly-20260913T220853Z` 因远端 Go 不在 PATH、Python HTTP 返回 403 而失败；未产生变更。后续只读 r2 基线成功。生产候选随后按 inspect→backup→change→verify 完成，独立复核通过。

## 香港 schema 4 回退边界

部署时生成的旧二进制备份可读，但旧 M5 二进制不能打开迁移后的 schema 4。隔离兼容演练 job `/tmp/codex-ssh/linksend-e5-hk-rollback-compat-drill-20260914T022857Z` 使用 live DB 的在线副本运行旧端，得到 exit 2 / `unsupported control database schema version`；线上 service PID、运行哈希和 DB schema 4 保持不变，隔离副本在退出时删除。

因此部署 job 输出的旧二进制安装命令已标记为 superseded，不能在当前数据库上执行。后续安全恢复流程为：

1. 保持当前健康 `387b57c` 服务运行，或在确需维护时先对 live schema 4 DB 做在线备份并校验。
2. 在该 schema 4 副本上验证候选恢复二进制能启动、读取成员并通过 health/WSS 行为检查。
3. 只替换为已证明兼容 schema 4 的二进制并重启，继续使用 live DB；独立检查文件哈希、PID、443、DB integrity/schema 与公网 health。
4. 失败时恢复上一个已证明兼容 schema 4 的二进制。部署前 schema 2 DB 仅作审计证据，不自动恢复，不覆盖部署后的新成员或授权写入。

## 荷兰实验交付

实验使用准确 `387b57c` Linux rendezvous/CLI，并用 `pion/stun/v4` fixture 提供 STUN Binding。运行前确认没有同名 namespace；脚本 cleanup 增加本轮 namespace 所有权门闩，重入检查不会删除其他作业资源。退出后独立复核确认 namespace、veth、进程和敏感材料均已清理，宿主地址/路由/规则前后相同。脱敏 archive 经 manager 下载后再次哈希一致，存放于本地 supervision 目录，不提交身份或正文。
