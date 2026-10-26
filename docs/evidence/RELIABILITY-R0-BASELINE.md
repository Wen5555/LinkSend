# R0 实施基线（2026-10-19，Asia/Shanghai）

用户本轮已启动 Goal；范围与授权以实施计划和启动提示词为准。本记录为基线证据，动态状态只维护唯一 TODO。

## 本地与 Git

- `git log -4 --oneline`：HEAD `f8358ce9b15477c5b18f69998c4e58eda2e6791`，程序源码 `63c86c2c5ce183e1400d27ca0571610d50a42bdc`；分支 `codex/reliability-sharing-plan-20261019`。`git ls-remote origin` 确认远端同分支 SHA 一致、main 为 `20d875189bce9300c62239c19bd4a369612a12ac`（退出码0）。
- `git status --porcelain=v1`：无 tracked 修改；保留 `.playwright-cli/`、`apps/desktop/build/ios/`、`apps/desktop/build/linux/`，不清理、不暂存。
- `. ./.tools/use-desktop-toolchain.ps1` 后 `go version` / `node --version` / `pnpm --version` 为 Go1.27.1 windows/amd64、Node24.21.0、pnpm12.4.1（退出码0）。初始 PATH 中 Node22/pnpm11 不用于产品验证。
- 根与 desktop 保持两个 Go 模块，Pion ICE v4.4.2、quic-go v0.62.0、Wails beta.18；前端版本0.6.2。未发现 `.codegraph/`，本轮从源码定位，不新建索引。
- 后续补核 `gh release view v0.6.2` 与 `git ls-remote origin refs/tags/v0.6.2`（均退出0）：公开、非draft/非prerelease，tag和targetCommitish均为 `63c86c2c5ce183e1400d27ca0571610d50a42bdc`，是本轮HEAD祖先；四程序资产及两份元数据有GitHub SHA256 digest。本次仅核对来源/资产元数据，不把它当作重新下载或安装验证。
- README 当前摘要仍写0.5.0，修正为源码实际0.6.2；历史版本记录不变。本增量仅文档，不运行无关应用测试；提交前执行 `git diff --check` 和本地链接检查。

## 远程检查（只读）

使用保存别名和管理器，不读取用户剪贴板、凭据或用户文件正文，不停止既有进程。

| 目标 | 实际结果 | 恢复证据 |
|---|---|---|
| `mac-test-102342413` | SSH probe退出0；`wen` / Darwin arm64，macOS26.5 build25F71，en0为192.168.1.243/24；存在 `/Applications/LinkSend.app` 进程65291，保持运行；系统有clang/xcodebuild/python3，默认PATH无Go/Node/pnpm | request `ls-r0-mac-20261019-001`，job `/tmp/codex-ssh/job-ls-r0-mac-20261019-001`，脚本退出0 |
| `jp-linksend` | SSH任务退出0，Linux x86_64/root；`linksend.service` 与 `linksend-stun.service` active，备份timer active；未改变服务或数据 | request `ls-r0-jp-20261019-001`，job `/tmp/codex-ssh/job-ls-r0-jp-20261019-001`，脚本退出0 |
| `tw-hinet-nat` | probe退出255、管理器分类unknown且无stderr；本次未确认可达，不将其当作可用跨网测试端，也不据此推断网络失败原因 | 仅probe，无启动作业 |

本地检查脚本位于忽略目录 `.artifacts/reliability-20261019/`。后续原生构建、LAN业务、真实跨网、系统粘贴和安装旅程都尚未由本基线证明；SSH可达不代替业务可用。

R2只读补查：request `ls-r2-mac-preflight-20261019-001` / job `/tmp/codex-ssh/job-ls-r2-mac-preflight-20261019-001` 退出0。加入已有Homebrew路径后可见Go1.26.4、Node26.0.0、pnpm11.3.0，与仓库锁定版本不一致；不用于声称合规候选构建。`kern.bootsessionuuid`可读且两次连续读取相同，只证明API在当前系统可用，不证明真实重启或缓存回收。未触碰日常进程、剪贴板或安装。

## 复用与下一步

复用现有目录过滤、多设备队列、限速、托盘快发、检索重发、文件剪贴板和手动更新。先沿连接与会话生命周期定位可复现缺陷，保持身份、公钥固定、正文路径及 `relay=false`。Mac日常进程和profile保持，后续构建/任务使用专属目录及隔离profile；生产服务替换、宿主防火墙/路由/代理/DNS均未执行。
