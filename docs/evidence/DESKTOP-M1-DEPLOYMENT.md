# M0/M1 合并候选的香港部署

2026-09-12，用户授权按阶段自动推送、部署和发布，并决定 M0 与 M1 合并为测试预发布。
本次服务端已部署并独立复核；桌面发布包仍须通过独立包校验，不能从服务端健康推导全部桌面验收通过。

| 项目 | 实际证据 |
|---|---|
| 精确源码 | `2f2656d940c96e142d5639b2bc3012ab0ce5fb6e` |
| 版本 / 工具链 | 产品 `0.5.0` / Go `1.27.1` / Linux amd64 / `vcs.modified=false` |
| 二进制 SHA256 | `299fef50fec3e4c84194b546b8ce8366a71033939a8ab810961e1fc6a347db06` |
| 操作目标 | `hk-main`，`root@109.66.88.204`，`/opt/linksend-lan-test/rendezvous` |
| 原运行文件 | M0 SHA `631db1cd5c5079a11c1b8a90bfed4eb0a6591c5ff6907203c3d71f4197a145a3` |
| 新进程 | systemd `linksend-rendezvous.service`，PID `396692`，active/running |
| 备份 | `/opt/linksend-lan-test/backups/20260912T054903.070440Z-desktop-m0-2f2656d940c9` |
| 部署作业 | `/tmp/codex-ssh/linksend-desktop-m0-deploy-20260912T054850Z` |
| 独立复核作业 | `/tmp/codex-ssh/linksend-desktop-m1-independent-20260912T055602Z` |

复用了经过检查的事务执行器，名称中的 `m0` 是工具名，不是本次源码版本。
实际执行 `deploy-m0.ps1 -BinaryPath .artifacts/desktop-six-features/rendezvous-m1`
并明确传入本表 Commit、SHA256、ExpectedOldSha256 与 `-Execute`，退出 0。
事务执行 inspect → 在线备份并核对可读 → 原子替换 → systemd 重启 → 独立核验；未触发回滚。
本地原始日志保存于 `.artifacts/desktop-six-features/deploy-m0-20260912T054826940Z-2f2656d940c9/`。

独立进程于 `2026-09-12T05:56:15Z` 重新检查运行文件来源/哈希、443 的进程归属、
origin 和公网 health、配置哈希、续期 timer、数据库及日志：全部 PASS。
数据库 `schema_version=[2]`、`PRAGMA user_version=0`，`integrity_check=ok`；没有错误地把后者当未迁移。
配置 SHA256 仍为 `20a940907782d0baec8745e9e3fe45bed68053568f0b3f6d325eeffdd77299c0`，
最近 30 分钟 fatal/panic=0，STUN 3478/UDP 仍存在，V1/QUIC/`relay=false`。

Windows Node 原生 WebSocket 在 `05:56:21Z` 独立完成：两次正常连断、未注册随机身份被 1008 拒绝、
立即再次连接并正常 1000 关闭。4/4 PASS；没有创建成员或发送文件正文。
这不是认证后的配对、文件传输、物理双机或 NAT 验收。

需要回滚时使用同一事务执行器和上述精确备份目录的 `-RollbackBackup`。
默认仅恢复已校验的旧二进制；不会用部署前数据库覆盖之后的新业务写入。
