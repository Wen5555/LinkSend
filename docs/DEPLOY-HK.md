# 香港测试主站部署与回滚

香港入口是长期测试主站，不承载生产身份或生产可用性承诺。目标 SSH alias 为 `hk-main`（Debian 12 amd64，SSH 端口由 manager inventory 管理），服务位于 `/opt/linksend-lan-test`。所有远程连接必须通过 `codex-ssh-manager` 执行 resolve → probe → audit-host；不得把密码、令牌、固定码、私钥或完整配置复制进提示词、脚本和证据。

## 当前基线（本轮部署前）

- 公共入口：`https://linksend.oooai.de/healthz`，旧服务返回 `status=ok`、协议 V1、`transport=quic`、`relay=false`，尚未报告产品版本。
- 二进制：`/opt/linksend-lan-test/rendezvous`，部署前 SHA256 为 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`。
- 数据库：`/opt/linksend-lan-test/data/server.db`；既有 Caddy/TLS 与 coturn STUN 由原服务配置管理。
- 旧服务在同一设备完成/拒绝后立即重试仍 FAIL；这是未部署源码修复，不是外部环境阻塞。

## 版本同步策略

从 `0.2.0` 起，每次产品版本更新允许同步更新香港测试主站，但授权不取消事务安全门槛：

```text
inspect → backup → change → verify → rollback-ready
```

只替换 LinkSend 测试服务二进制及确有需要的配置/schema；不得顺带修改防火墙、路由、DNS、代理、证书或用户正式身份数据。旧二进制可以从运行路径卸下，但必须保留在新的时间戳备份目录，直到新版本验证完成并可一条事务回滚。部署资产必须来自真实 commit，记录 `go version -m` revision、`vcs.modified=false`、产品版本、大小和 SHA256。

## 每次部署步骤

1. manager resolve/probe/audit-host；只读检查 systemd unit、PID、监听、health、磁盘、当前二进制 SHA256/构建信息和数据库 schema/integrity。
2. 创建 `/opt/linksend-lan-test/backups/<UTC>-v<version>/`，权限最小化；复制当前二进制、unit 和脱敏配置，使用 SQLite 在线备份并执行 `PRAGMA integrity_check`。
3. 上传到同目录受控临时文件，核对本地/远端 SHA256、ELF amd64、`--version` 和 Go build revision；不直接覆盖正在执行的 inode。
4. 原子替换二进制，重启既有 unit；核对新 PID、端口、journal 无稳定错误，并检查 `/healthz.version=0.2.0`、`capabilities.product_version=0.2.0`、`protocol_version=1`、`relay=false`。
5. 使用隔离测试身份验证动态邀请、WSS、完成/拒绝/失败/取消后的立即新连接；健康 QUIC 在信令短断时不应被服务端误杀。
6. 若任何门槛失败，停止新服务、恢复同一备份目录中的二进制/配置（仅在迁移实际发生且验证需要时恢复数据库），重启并核对 PID/health/SHA256。不得用运行中的 WAL/SHM 覆盖数据库。

## 配置与数据边界

测试固定码只有同时指定 `test_pairing_group` 才能启用；通用 `server.example.toml` 保持关闭。知道共享码的访问者可取得该测试组管理员权限，因此应保留限流、成员审计和移除两个配置项后的关闭方案。动态邀请仍为高熵、10 分钟、一次性。

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

准确回滚：先核对当前 PID 的 `/proc/<pid>/exe` 指向运行路径，TERM 并等待退出；将 `.../20260911T101714Z-v0.2.0-4b7ccf6/rendezvous.pre-v0.2.0` 以 mode 755 安装回 `/opt/linksend-lan-test/rendezvous`，按原 `--config /opt/linksend-lan-test/server-public.toml` 用 `nohup` 启动，原子更新 PID 文件，再核对旧 SHA、443 listener 和 health。配置与 schema 未改变，通常不应恢复旧数据库；只有明确要撤销部署后成员写入时才停服并使用已验证在线备份。
