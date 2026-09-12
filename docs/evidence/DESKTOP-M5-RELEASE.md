# M5 合并、发布与部署回执

已按用户后续要求收尾至M5。`main`于2026-09-12从`fbfc250`快进到`d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06`并推送；后续只有本发布记录等文档补录，不移动发行tag。M6已有收件箱代码与局部证据保留，不继续其完整验收。

Release：[v0.5.0-m5](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-m5)，发布时间`2026-09-12T10:13:32Z`。
Tag精确指向`d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06`；`draft=false`、`prerelease=true`，发布时明确`latest=false`。

## 发行证据

- core [34686655741](https://github.com/Wen5555/LinkSend/actions/runs/34686655741)：success。
- desktop [34686655847](https://github.com/Wen5555/LinkSend/actions/runs/34686655847)：success。
- packaging [34686655743](https://github.com/Wen5555/LinkSend/actions/runs/34686655743)：Windows amd64、Mac arm64、Mac amd64全部success。
- 官方artifact外层SHA、四包SHA、BUILD-INFO来源、Windows ZIP内部清单/EXE版本全部核对；发布前后逐项验证GitHub返回的6个asset digest与本地一致。
- Windows精确CI EXE SHA256 `e08d8dce9cda57edb3fb8b55743d3b8e3928919841ff9a2bd14582c5087b67ff`：真实Wails窗口、M5内容草稿保存到Go/SQLite、正常退出0。未在这次精确包检查中访问剪贴板。证据`scripts/desktop-native/m1-native-tests/run-20260912T100848882Z/`；三类型完整原生交互属于M5-EXECUTION记录的较早测试快照。
- Mac精确CI ARM窗口及正常退出0，两DMG的架构/最低系统/签名核对通过；详见 [Mac CI证据](DESKTOP-M5-CI-MAC.md)。独立源码全套检查见 [Mac源码证据](DESKTOP-M5-MAC-SOURCE.md)。

| Release资产 | SHA256 |
|---|---|
| Windows便携ZIP | `dfc10e70d924b4cac302c844d862413e31e2826aef41e1543ad47f4559500cbe` |
| Windows NSIS安装器 | `cc1d5aad9adbd93131f385c41d326b794aca525f719eab1afb95df0b3f0d63f5` |
| macOS ARM DMG | `4138435bf52cb73718acafdba68924e98c23dac794265402e2a399549d8e962c` |
| macOS Intel DMG | `0786c3a1cc4741e040170cbbd6d8bab1936123950a373730483d3dd20f58b942` |
| RELEASE-MANIFEST.json | `73af1fe3bb5d26617999962ee195baed03a6ac467299d1c8ba4b7828c8e2a08f` |
| SHA256SUMS.txt | `60bcfd02ec24fde926ec4c54141dfb3a3c6a54d6f5c5271ad58bd539f39cf756` |

本地发布核对目录`.artifacts/desktop-six-features/m5-ci/`保留PACKAGE-VERIFICATION、官方artifact收据及GITHUB-PUBLISHED-VERIFICATION。

## 香港最终部署

目标`hk-main`，`/opt/linksend-lan-test/rendezvous`，systemd `linksend-rendezvous.service`。
从独立干净d0c4a4b工作树构建Go1.27.1/Linux amd64，`vcs.modified=false`。

- 二进制SHA256：`0923d9dfca11028859449eee7bc00026ca3e2616d5ed32411a3b42f216bd318c`。
- 原版本文件为62628d0源码的`df50ed9460e7615fb2ab189d58bbdd2911db934176abf5a812d90a3c1834abbc`。
- 备份：`/opt/linksend-lan-test/backups/20260912T094925.182790Z-desktop-m0-d0c4a4b13ddc`。名称中的m0沿用事务工具命名。
- 部署作业：`/tmp/codex-ssh/linksend-desktop-m0-deploy-20260912T094912Z`，退出0；inspect/可读在线备份/原子替换/重启/verify全部PASS，没有触发回滚。
- 独立复核：`/tmp/codex-ssh/desktop-m5-independent-hk-20260912T095041Z`，运行PID399706、文件SHA、443归属、数据库integrity及公网health全部PASS。
- 服务端数据库保持schema_version表`[2]`（PRAGMA user_version仍0），产品0.5.0/V1/QUIC/relay=false。客户端任务库schema5与服务端schema2是不同数据库。
- Windows独立公网WSS4轮：正常连断、立即重连、未注册身份1008拒绝、再次正常1000关闭全PASS；没有新建成员、没有发送正文，不称已完成公网文件传输或认证配对测试。

需要回滚时沿用事务执行器指定该精确backup和目标hash；只恢复已验证旧二进制，不用部署前数据库覆盖新业务写入。

## 停止边界

M5实现与本次测试预发布已经交付，本轮不再推进M6。原Mac应用PID47950和用户未跟踪目录保留；本次GUI、peer、fixture及Mac测试mount均已结束/卸载。

仍保留真实限制：Mac13及Intel实机未运行、Mac完整菜单/通知点击缺项、反向物理路由失败、MASQUERADE-only双NAT失败、签名/公证和完整安装矩阵未完成。剪贴板验收工具最后一次恢复未确认，已告知用户并在M5-EXECUTION如实记录；不能将这些边界从测试预发布中隐去。
