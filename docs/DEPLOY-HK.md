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

本轮 `0.2.0` 的实际备份目录、部署二进制 revision/SHA256、健康与立即重试结果将在事务完成后追加到本文件和 PROGRESS；完成前继续标记旧服务 FAIL。
