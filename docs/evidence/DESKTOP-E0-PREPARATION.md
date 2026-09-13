# E0 准备与恢复记录

2026-09-13（Asia/Shanghai）。这份记录只证明准备操作，不是 E0 验收结论。

- 执行任务：`01a096c7-6cfb-71b0-87aa-aa7ed781bb2f`，local，工作树 `C:/Users/Wen/.codex/worktrees/208a/Osend`。
- 总控：`01a096c4-2e59-7db3-a7a8-ec2a125409d6`。总控消息已核实运行配置 `gpt-6-astra / medium`；本任务 read_thread 未提供模型字段，不将本任务文字声明当作切换凭据。
- 正式分派标记：`E0-dispatch-v1`，审查补充：`E0-review-notes-v1`。执行 E0-01–05，暂不扩大 E1；允许中间提交供审查，未审提交不推送。
- 创建时 detached HEAD；`git switch -c codex/desktop-experience-upgrade` 退出 0，无本地分支占用。`git ls-remote origin refs/heads/main refs/heads/codex/desktop-experience-upgrade` 退出 0：远端 main 为 `3bcb73019d1ac6de1d9f341bb7d22e2813875081`，工作分支尚无远端引用。
- HEAD/main/origin main 同为上述提交，origin 为 `https://github.com/Wen5555/LinkSend.git`。
- M5 包源码 `d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06` 到当前基线仅 8 个文档文件变化（`git diff ... --stat` 退出 0）；不能因此把历史包测试扩写为新验收。

## 材料与工具

已读取路径上适用 AGENTS、完整执行提示词、完整方案、SPEC、完整 PROGRESS、PROTOCOL、SECURITY、ARCHITECTURE、ADR0001–0006 与 M5 RELEASE/EXECUTION/CI-MAC/MAC-SOURCE/CONTENT-BACKEND/CONTENT-SNAPSHOTS/CONTENT-WIRE、M4-M5 ACCEPTANCE-INTEGRATION。大输出截断处随后分段补读。`.codegraph` 不存在，按规则直接读源码。

两份原未跟踪材料已由 worktree setup 带入，未再次复制；Get-FileHash 核对与原目录逐份相同：

| 材料 | SHA256 |
|---|---|
| docs/prompts/DESKTOP-EXPERIENCE-GOAL.md | `c94125ff7939ada2a119c6e4de962e1e3aa6ecce98bafae6ff2290f778978d5a` |
| docs/DESKTOP-EXPERIENCE-IMPROVEMENT-PLAN.md | `a9301ab698fbb78a9578499ac09203f8bff8e7d7699c023e5d2e1847e56cbd39` |

setup 还带入 `.playwright-cli/`、`apps/desktop/build/ios/`、`apps/desktop/build/linux/`，均无关、保持原状、不得暂存/提交。不是执行任务主动复制。

Windows 11 专业版 x64，Version 10.0.26200 / Build 26200；PowerShell 7.6.5。沿用 `D:/apps/Osend/.tools/use-desktop-toolchain.ps1`，`go version`、`node --version`、`pnpm --version`、`wails3 version` 均退出 0：Go1.27.1 windows/amd64、Node24.21.0、pnpm12.4.1、Wails3 beta.18。准备时未安装依赖、未跑产品测试。

## 当前执行状态

全轮 22 条稳定 TODO 已落盘。正式 E0 分派已到达，继续 E0，不在准备结束时停工。
准确包复现/双模块测试/原生共享/物理双向网络均 NOT RUN；M5 历史记录保持原边界。

远程采用 codex-ssh-manager 技能；hk-main 与 nl-highdefense resolve/probe/audit 已退出 0。荷兰 alias 的 hostname 含 lax，地理位置不从 alias 推断；后续核实真实拓扑。其 audit 有 1 failed unit 与 rebootRequired=true，仅记录，不重启或修复无关服务。

Mac manager add 两次退出 1，错误 `Argument types do not match`，inventory 未更新；因此按已读取 hosts schema 仅更改登记 Mac 的 hostName，由 `10.234.14.15` 更新为 `10.234.39.151`，保留用户/密钥/端口/其余字段。本机 inventory 备份 `C:/Users/Wen/.codex/ssh-manager/hosts.pre-linksend-e0-20260913.json`；manager resolve 已确认新值，连接仍全部经 manager。旧地址 probe exit255 timeout 不代表新地址结果。未关闭 host-key 校验，未修改 SSH manager 实现。

当前原始只读日志：`.artifacts/desktop-experience/e0/`（ignored）。远程 durable job 尚未启动；部署/安装/推送 NOT RUN。

## E0-05 必须纳入的总控审查点

revoked 身份的新有效码重配需 incarnation/授权代际与重新入组事务；Revoke 权限检查与变更必须同事务，覆盖普通成员互撤竞态。同组幂等不重授文件免确认/剪贴板。旧邀请绑定 inviter 当前组和成员代际，跨组切换不复活旧组。目录独立保留已认证同步状态和 LAN 控制证据，不能 Nearby OR Online 或吞掉 serverErr。原生共享独立请求交接，不能 MergeDraftPaths+Show/Focus 冒充。

## 后续检查点（2026-09-13）

manager apply后Mac新地址probe/audit退出0；正式执行结果以同目录E0-BASELINE/NETWORK/WINDOWS-SHARE-PROTOTYPE/MAC-SHARE-PROTOTYPE/DECISIONS为准，上面的“尚未启动”是准备时状态。
任务中断后恢复时Windows测试PID260116已不存在；没有伪造正常退出码。现有工具列表已无send_message_to_thread/read_thread/wait_threads，发送增量消息尝试返回工具不可调用；总控此前的分派与基线检查消息仍有效。当前以工程文件和中间提交保留审查入口，不将未送达消息写成已审查。
本批允许较大独立闭环中间提交；准确包基线+原型边界+ADR提案提交后待总控审查，E0整体未完成。
