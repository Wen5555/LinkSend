# 香港主站部署记录与回退

目标主机：SSH alias `hk-main`（root，Debian 12，x86_64，SSH 端口 60851）。本轮使用本地凭证，通过 `codex-ssh-manager` 执行；未输出私钥、令牌或完整生产配置。

## 基线（已完成）

- 现有入口：`https://linksend.oooai.de/healthz`，返回 `status=ok`、`protocol_version=1`、`transport=quic`、`relay=false`。
- 当前 rendezvous：`/opt/linksend-lan-test/rendezvous`，监听 `*:443`；数据库为 `/opt/linksend-lan-test/data/server.db`；TLS 使用既有 origin 证书；3478/UDP 由 coturn 监听。
- 变更前二进制、配置、数据库和证书备份目录已在远端既有记录中保留；本轮新部署前仍需创建带时间戳的一致性 SQLite backup，并验证可读。

## 本轮代码变更

服务端固定码入口现在支持显式 `test_pairing_code = "orion123"` 与 `test_pairing_group = "<现有组 ID>"`。公共配置缺少目标组会被拒绝；固定码加入会通过正常 Ed25519 注册证明，复用已有身份幂等并升级为管理员，撤销身份不会复活。通用 `server.example.toml` 默认关闭；动态邀请码仍为高熵、10 分钟、一次性。

## 生产部署状态（2026-09-09 已完成）

已从任务分支候选构建并部署到 `hk-main`，服务端二进制 SHA256 为
`53445BE88943FBABDAA7AABFAF98E299AD9780B1084DAA764CF8804E48E5A338`。
变更前 SQLite 在线备份为
`/opt/linksend-lan-test/backups/20260909T133335Z-wails3/server.db`，已执行
`PRAGMA integrity_check` 并返回 `ok`；旧二进制、配置和证书副本保留在同一备份目录。
配置显式启用 `test_pairing_code = "orion123"` 和现有目标组
`73cde936c5bb2b3c60e275e4fa7c6e058c2dc5914425b378eacbee8b528a0344`，随后受控重启
rendezvous。重启前后 `/healthz` 均为 `status=ok`、`protocol_version=1`、`transport=quic`、
`relay=false`。

真实验证结果：`hk-win-wails3` 与 `hk-peer-wails3` 通过固定码加入并返回
`admin=true`、同一组；`hk-dynamic-wails3` 通过动态邀请码加入并保持
`admin=false`，重复消费被拒绝；重启后目标组持久化为 8 个成员、5 个管理员、0 个撤销成员。
固定码入口按授权保持开启。除 LinkSend 二进制/配置和本轮测试成员外，本轮未修改 DNS、证书、
防火墙或无关服务。

执行时应遵循：

```text
inspect → SQLite 在线 backup（校验可读）→ 上传候选并保留旧版本
→ 写入明确 test_pairing_code/group 配置 → 重启 rendezvous
→ health/WSS → 固定码真实加入与管理员只读权限 → 动态码
→ 双端在线及 QUIC 直连文件哈希 → 失败则恢复旧二进制/配置并重启
```

固定码入口在后续 Mac 测试期间保持开启；关闭入口只需移除这两个配置项并重启，不会撤销已加入管理员。
若需回退，恢复备份目录中的旧二进制/配置并重启；不要用旧数据库覆盖部署后的正常成员写入。
本轮成员标识、任务 ID 和传输证据已追加到 `docs/PROGRESS.md` 与 `docs/STAGE2B-20260909.md`，敏感内容留在受控远端。
