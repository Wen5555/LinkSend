# 香港测试主站部署与回滚

香港入口是长期测试主站，不承载生产身份或生产可用性承诺。目标 SSH alias 为 `hk-main`（Debian 12 amd64，SSH 端口由 manager inventory 管理），服务位于 `/opt/linksend-lan-test`。所有远程连接必须通过 `codex-ssh-manager` 执行 resolve → probe → audit-host；不得把密码、令牌、固定码、私钥或完整配置复制进提示词、脚本和证据。

## 当前基线（Desktop M1，v0.5.0）

2026-09-12 北京时间 13:49，已从精确提交 `2f2656d940c96e142d5639b2bc3012ab0ce5fb6e` 完成 M1 事务部署。
运行 MainPID `396692`，SHA256 `299fef50fec3e4c84194b546b8ce8366a71033939a8ab810961e1fc6a347db06`，
Go1.27.1、`vcs.modified=false`；schema2、origin/public health、443、STUN、续期和最近 fatal=0 独立检查 PASS。
备份 `/opt/linksend-lan-test/backups/20260912T054903.070440Z-desktop-m0-2f2656d940c9`，
部署及独立作业路径、回滚边界见 [M1 部署记录](evidence/DESKTOP-M1-DEPLOYMENT.md)。
用户已决定 M0/M1 合并测试预发布，不再单独发布 m0 标签。

## 历史基线（Desktop M0）

- 2026-09-12 北京时间 12:46 完成用户授权的 M0 测试主站事务部署；产品为 `0.5.0`，计划测试预发布标签为 `v0.5.0-m0`。它只对应六项桌面功能的 M0 前置工作，不代表 M1–M6 或六项原生验收完成；本节不声明 Release 已发布。
- `linksend-rendezvous.service` 运行 `/opt/linksend-lan-test/rendezvous`，部署后及独立复核 MainPID 均为 `395859`。资产来自精确干净提交 `b3d5fc7b6acf49dfc07671d774e2a7f20c28cc94`，Go `1.27.1`、Linux amd64、`vcs.modified=false`；17,880,415 bytes，SHA256 `631db1cd5c5079a11c1b8a90bfed4eb0a6591c5ff6907203c3d71f4197a145a3`。
- 备份 `/opt/linksend-lan-test/backups/20260912T044648.234152Z-desktop-m0-b3d5fc7b6acf` 包含可读的旧二进制、配置、unit、SQLite 在线备份和 manifest。旧版本为 `0.4.0`；事务门槛全部通过，无需触发自动回滚，没有回退数据库。
- 控制数据库的真实版本位于 `schema_version` 表，值为 `2`；`PRAGMA user_version` 仍为 `0`。两者均未变化，`integrity_check=ok`，邀请表仍为 19 条、`used_by/used_at` 存在。桌面任务库或 trust 文件版本不能混用于服务数据库。
- origin 与公网 health 独立复核均返回产品 `0.5.0`、协议 V1、QUIC、`relay=false`；443 由 MainPID 监听，UDP 3478 保持存在；续期 timer active，最近 30 分钟 fatal/panic=0。配置和 unit 哈希均未变，没有修改证书、续期脚本、coturn 或主机网络。
- 公开 WSS 另做 4 轮实际连接/快速关闭/重连检查；未注册随机身份认证返回 close 1008，正常关闭为 1000，之后立即重连仍可收到真实 0.5.0 hello。未创建成员/profile，也未传输正文；不等同于认证后配对、传输、跨 NAT 或原生 UI 验收。
- 部署 job：`/tmp/codex-ssh/linksend-desktop-m0-deploy-20260912T044635Z`；独立复核 job：`/tmp/codex-ssh/linksend-desktop-m0-independent-20260912T044754Z`。完整实际命令、版本哈希、限制和确切回滚入口见 [M0 部署证据](evidence/DESKTOP-M0-DEPLOYMENT.md)。
- 回滚使用该备份中的旧二进制并保留当前 schema 2 数据；执行器的 `-RollbackBackup` 会先核对部署 manifest、当前新 SHA、旧备份 SHA 和配置，避免覆盖后续其他事务。若意外出现不兼容 schema，应停服审阅新写入，不能自动用旧数据库抹掉部署后成员数据。

## 上一基线（v0.4.0）

- 该版本部署后的公共入口 `https://linksend.oooai.de/healthz` 返回产品 `0.4.0`、协议 V1、`transport=quic`、`relay=false`。
- `linksend-rendezvous.service` 管理 `/opt/linksend-lan-test/rendezvous`。该版本二进制来自干净提交 `7303f34260c09b8ad71c117ee2d6837fac4dfe46`，`vcs.modified=false`，SHA256 为 `227e2ec9fb029fcc8a568637376318e4eae4099f1cc2ce79bc7224557f07fa45`。
- 控制数据库为 schema 2、`PRAGMA integrity_check=ok`，邀请表包含 `used_by` 和 `used_at`；公网测试确认 TTL=600 秒、同身份重复加入幂等成功、其他身份复用返回 `PAIRING_CODE_USED`。
- `linksend-origin-renew.timer` 保持启用。续期脚本使用 Python 解析 Cloudflare API JSON、强制 HTTP/1.1 并进行有界重试；更新证书后通过 `systemctl restart linksend-rendezvous.service` 重启，不再依赖缺失的 `jq`、模糊 `pkill` 或从 systemd oneshot 派生后台进程。

## v0.4.0 实际事务（2026-09-12）

- 版本部署前 PID `391298`、产品 `0.3.0`、schema 2、21 条邀请；备份 `/opt/linksend-lan-test/backups/20260912T015209Z-v0.4.0-7303f34` 包含旧二进制、配置和通过完整性检查的在线/停服数据库备份。
- 部署后 PID `392632`；版本、二进制 SHA256、443 listener、公网 health、schema、邀请记录数、配置哈希和 recent fatal=0 全部通过。独立复核 job：`/tmp/codex-ssh/verify-linksend-v040-independent-20260912T015340Z`。
- 公网配对行为复核：服务端相对 TTL=600 秒；同一 Ed25519 身份首次与重复消费均成功；其他身份复用同一码返回 `PAIRING_CODE_USED`。
- 额外审计发现旧 `linksend-origin-renew.service` 因 `jq: not found` 连续失败，旧证书将在 2026-09-15 到期。首次修复运行遇到 Cloudflare HTTP/2 `PROTOCOL_ERROR`，未改动证书；第二次签发成功后又暴露 systemd oneshot 会清理其后台 rendezvous 子进程，主站随即使用已验证的 `0.4.0` 二进制恢复。
- 最终将 rendezvous 迁移为独立 systemd service，并让续期任务只负责原子更新证书和重启该 service。迁移/续期前备份为 `/root/linksend-lan-test-backups/20260912T022048Z-systemd-renew-v3`；真实续期后 PID `393922`、证书有效至 2026-09-19、定时器 active、443 与公网 health PASS、失败单元数为 0。最终独立复核 job：`/tmp/codex-ssh/verify-hk-v040-after-systemd-20260912T022110Z`。
- 当前回滚入口：停止并禁用 `linksend-rendezvous.service`，从上述迁移备份恢复续期脚本与证书；若只回滚应用版本，则保留 systemd 管理和 schema 2，替换为版本部署备份中的旧二进制后 `systemctl restart linksend-rendezvous.service`。只有明确需要撤销数据库写入且应用已停止时才恢复数据库备份。

## 历史基线（2026-09-09）

- 公共入口：`https://linksend.oooai.de/healthz`，旧服务返回 `status=ok`、协议 V1、`transport=quic`、`relay=false`，尚未报告产品版本。
- 二进制：`/opt/linksend-lan-test/rendezvous`，部署前 SHA256 为 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`。
- 数据库：`/opt/linksend-lan-test/data/server.db`；既有 Caddy/TLS 与 coturn STUN 由原服务配置管理。
- 旧服务在同一设备完成/拒绝后立即重试仍 FAIL；这是未部署源码修复，不是外部环境阻塞。

## 版本同步策略

从 `0.4.0` 起，每次产品版本更新允许同步更新香港测试主站，但授权不取消事务安全门槛：

```text
inspect → backup → change → verify → rollback-ready
```

只替换 LinkSend 测试服务二进制及确有需要的配置/schema；不得顺带修改防火墙、路由、DNS、代理或用户正式身份数据。证书生命周期应作为独立事务处理并单独验证回滚。旧二进制可以从运行路径卸下，但必须保留在新的时间戳备份目录，直到新版本验证完成并可一条事务回滚。部署资产必须来自真实 commit，记录 `go version -m` revision、`vcs.modified=false`、产品版本、大小和 SHA256。

## 每次部署步骤

1. manager resolve/probe/audit-host；只读检查 systemd unit、PID、监听、health、磁盘、当前二进制 SHA256/构建信息和数据库 schema/integrity。
2. 创建 `/opt/linksend-lan-test/backups/<UTC>-v<version>/`，权限最小化；复制当前二进制、unit 和脱敏配置，使用 SQLite 在线备份并执行 `PRAGMA integrity_check`。
3. 上传到同目录受控临时文件，核对本地/远端 SHA256、ELF amd64、`--version` 和 Go build revision；不直接覆盖正在执行的 inode。
4. 原子替换二进制，重启既有 unit；核对新 PID、端口、journal 无稳定错误，并检查 `/healthz.version=<目标版本>`、`capabilities.product_version=<目标版本>`、`protocol_version=1`、`relay=false`。
5. 使用隔离测试身份验证动态邀请、WSS、完成/拒绝/失败/取消后的立即新连接；健康 QUIC 在信令短断时不应被服务端误杀。
6. 若任何门槛失败，停止新服务、恢复同一备份目录中的二进制/配置（仅在迁移实际发生且验证需要时恢复数据库），重启并核对 PID/health/SHA256。不得用运行中的 WAL/SHM 覆盖数据库。

## 配置与数据边界

测试固定码只有同时指定 `test_pairing_group` 才能启用；通用 `server.example.toml` 保持关闭。知道共享码的访问者可取得该测试组管理员权限，因此应保留限流、成员审计和移除两个配置项后的关闭方案。动态邀请使用 40-bit 随机短码、10 分钟有效期、一次性消费与同身份幂等重试。

schema 变化必须在部署记录中给出迁移前备份、前后 `user_version`、兼容范围和准确恢复步骤。通常回滚二进制不应回退已经产生新业务写入的数据库；只有 schema 不兼容且已确认数据边界时，才从部署前在线备份恢复。

## `0.2.0` 实际事务（2026-09-11）

- manager 前置 `resolve`、`probe`、`audit-host` 全部 PASS；发现一个既有 `linksend-origin-renew.service` 失败，但当前 rendezvous、443/TCP 与 3478/UDP 均正常。首次只读脚本错误假定 systemd unit 且远端缺少 `file`，退出 127，无任何变更；随后确认 rendezvous 实际由 `/opt/linksend-lan-test/rendezvous-public.pid` 与 `nohup` 管理。
- 部署源码 commit：`4b7ccf66f89c9720d9b5970e5d610b453048d69c`。Linux amd64 资产由独立干净 clone、`GOWORK=off` 构建；本地 `go version -m` 为该 revision、`vcs.modified=false`。大小 11,309,218 bytes，SHA256 `c549cdbf1bd461d6f583302f96700471000aacba1fed912c66d6e74912f30247`。
- 变更前 PID `352572`，旧 SHA256 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`；配置 SHA256 `20a940907782d0baec8745e9e3fe45bed68053568f0b3f6d325eeffdd77299c0`，数据库 `integrity_check=ok`、`user_version=0`。
- 部署备份：`/opt/linksend-lan-test/backups/20260911T101714Z-v0.2.0-4b7ccf6`。其中保留旧二进制的验证副本和从运行路径移出的 `rendezvous.uninstalled`、0600 配置、SQLite 在线备份与 SHA256 清单；备份数据库 integrity=ok。
- 事务成功后新 PID `381064`，运行路径只保留 `0.2.0` 新二进制；配置 SHA 未变化，数据库 integrity=ok。内外网 health 均返回 `version=0.2.0`、`product_version=0.2.0`、`protocol_version=1`、`transport=quic`、`relay=false`。
- 公网行为验证在同一物理 `eth0` 上使用两个隔离 profile：连续两次完成后立即重连，以及拒绝后立即重连并完成均 PASS；每轮 2,097,152 bytes，内容 `cmp`/SHA256 一致，服务 PID/SHA 未变化。运行时 profile、邀请、临时 CLI 和正文已删除；B 由 API 撤销，A 因禁止自撤销在 SQLite 在线备份后精确撤销。清理备份为 `/opt/linksend-lan-test/backups/20260911T102329Z-v0.2.0-lifecycle-cleanup`，数据库 integrity=ok。
- 关键 manager jobs：部署 `/tmp/codex-ssh/linksend-v020-deploy-20260911T101701Z`；公网生命周期 `/tmp/codex-ssh/linksend-v020-public-lifecycle-20260911T102053Z`；测试管理员清理 `/tmp/codex-ssh/linksend-v020-revoke-test-admin-20260911T102316Z`。日志不包含邀请、固定码、私钥或文件正文。

旧服务“立即重试”FAIL 已由部署后的测试主站行为复核消除。未修改 DNS、路由、防火墙、代理、Caddy/TLS、coturn、固定码配置或非 LinkSend 服务。

部署后的分支继续产生 `fcc48ebfee7ce34ae73567c9cf0ab9fd0f00cbfe`（任务暂停迟到进度修复）、`a88553180bd1defac5b236d75fe4dff5046734dd`（客户端 QUIC 正常终态关闭修复）和最终 `v0.2.0` tag `426d58b6ab62ab7213475007305c0a403955c00f`；这些后续提交没有修改 `cmd/rendezvous`、`internal/server`、`internal/signaling`、协议版本、服务配置或 schema。因此测试主站仍准确记录为从干净 commit `4b7ccf66…` 构建的产品 `0.2.0`，不是谎报为桌面 Release SHA。以后产品版本变化或服务端依赖变化时按本页事务重新同步；纯客户端/桌面/文档提交不为追求 SHA 外观而无意义重启服务。

准确回滚：先核对当前 PID 的 `/proc/<pid>/exe` 指向运行路径，TERM 并等待退出；将 `.../20260911T101714Z-v0.2.0-4b7ccf6/rendezvous.pre-v0.2.0` 以 mode 755 安装回 `/opt/linksend-lan-test/rendezvous`，按原 `--config /opt/linksend-lan-test/server-public.toml` 用 `nohup` 启动，原子更新 PID 文件，再核对旧 SHA、443 listener 和 health。配置与 schema 未改变，通常不应恢复旧数据库；只有明确要撤销部署后成员写入时才停服并使用已验证在线备份。
