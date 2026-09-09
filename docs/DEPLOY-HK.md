# 香港主站部署记录与回退

目标主机：SSH alias `hk-main`（root，Debian 12，x86_64，SSH 端口 60851）。本轮使用本地凭证，通过 `codex-ssh-manager` 执行；未输出私钥、令牌或完整生产配置。

## 基线（已完成）

- 现有入口：`https://linksend.oooai.de/healthz`，返回 `status=ok`、`protocol_version=1`、`transport=quic`、`relay=false`。
- 当前 rendezvous：`/opt/linksend-lan-test/rendezvous`，监听 `*:443`；数据库为 `/opt/linksend-lan-test/data/server.db`；TLS 使用既有 origin 证书；3478/UDP 由 coturn 监听。
- 变更前二进制、配置、数据库和证书备份目录已在远端既有记录中保留；本轮新部署前仍需创建带时间戳的一致性 SQLite backup，并验证可读。

## 本轮代码变更

服务端固定码入口现在支持显式 `test_pairing_code = "orion123"` 与 `test_pairing_group = "<现有组 ID>"`。公共配置缺少目标组会被拒绝；固定码加入会通过正常 Ed25519 注册证明，复用已有身份幂等并升级为管理员，撤销身份不会复活。通用 `server.example.toml` 默认关闭；动态邀请码仍为高熵、10 分钟、一次性。

## 生产部署状态

本次会话已完成本地代码和 Wails 3 构建，但尚未在香港执行二进制替换、数据库写入或服务重启：`NOT_RUN`。原因是当前工作树尚未形成可审查的 Linux 生产候选 commit，且需要先从实际数据库读取并确认目标组 ID，再执行一体化备份/变更/验证脚本。不能把既有健康检查扩大为本轮固定码管理员或文件传输证据。

执行时应遵循：

```text
inspect → SQLite 在线 backup（校验可读）→ 上传候选并保留旧版本
→ 写入明确 test_pairing_code/group 配置 → 重启 rendezvous
→ health/WSS → 固定码真实加入与管理员只读权限 → 动态码
→ 双端在线及 QUIC 直连文件哈希 → 失败则恢复旧二进制/配置并重启
```

固定码入口在本轮生产验收完成前保持开启；关闭入口只需移除这两个配置项并重启，不会撤销已加入管理员。生产备份路径、候选哈希、成员 ID、任务 ID 和回退结果应追加到 `docs/PROGRESS.md`，敏感内容留在受控远端。
