# 2026-10-18 日本猫猫机信令重建证据

当前任务状态统一见 [TODO](../DESKTOP-EXPERIENCE-TODO.md)，操作契约见[部署说明](../DEPLOY-JP.md)。本页记录命令、实际结果和限制，不将服务部署等同于整个 B 完成。

## 来源与首次部署

服务源码 `d5412642e49460413fdda249f77d03647075f41c`，五项 CI 及三平台包构建通过。源归档 SHA256 `13d69b85062a2a2dcba506081bfa7648b1df7b70fca93d09a8f415610868af6b`，在 Git 仓库外用 Go1.27.1 / linux-amd64 / CGO0 构建，二进制 SHA256 `688702887cf7b87351d81384e941a352e13c3cd4701e437757f0922241e83d45`。内部 archive 目录仍被 Go 找到父 Git 的两份 dirty 构建均未部署。

部署资产来源 `b6db1b2d1fe706b1a0c60bc80215946ced0f48e3`；受管部署 job `/tmp/codex-ssh/job-linksend-ae-jp-deploy-b6db1b2-20261018-01`。原 Nginx 配置先备份/回读/校验，实际备份目录 `/var/backups/linksend/deploy-20261017T194725Z`。后端严格 TLS、health 与 relay=false 通过，但 reload 后立刻探测 Origin 返回404，job exit22；自动停新服务、取消本任务站点链接，保留新库。随后只读核对：原主配置/wildcard 原字节不变，新库 devices=0/groups=0，服务inactive，未切DNS。

受限恢复 job `/tmp/codex-ssh/job-linksend-ae-jp-recover-b6db1b2-20261018-01` 先比较现有单元/站点/二进制的所有权与来源，再备份并启用已有专属资源；不重跑首次安装或覆盖数据库。第2次 Origin 探测通过，支持 reload 异步就绪这一判断；首次安装器补入有界就绪等待。恢复备份 `/var/backups/linksend/recover-20261017T195003Z`，含受限配置归档/校验；现有其他站点字节保持一致。

## 当前真实服务与 HTTPS/WSS

- 独立用户 `linksend`；应用只监听 `127.0.0.1:8787`，持久库 `/var/lib/linksend/control.db`。Nginx 精确域名 `linksend.oooai.de`，不替换既有 wildcard。
- Nginx→后端使用专属 CA 与 `linksend-backend.internal` 名称严格校验；外层复用已核验 `*.oooai.de` Origin 证书。应用未启用 insecure loopback、loopback candidates 或固定测试配对码。bootstrap secret/私钥只留主机受限文件，未回传。
- 主服务与每日备份 timer active；独立 journal namespace 保留/大小/限频配置已安装。备份工具和配置恢复归档均位于主机受限路径；单机备份不是异机灾备。

`scripts/controlprobe` 复用生产 signaling.Client，通过专用 profile、签名HTTP、真实WSS执行检查；只输出结果元数据，邀请码只在内存中传递。Origin 阶段使用显式 CA+单客户端 dial override（保持主机名校验），不改DNS/宿主路由、不跳过TLS。工具来源 SHA256 `7b96cb0462c3ee719a5b01a2874bd0f850d72023e56ef04461064a0d8683ecb8`，Linux probe `36f790784e74954ffafa9cf4797977879894e59828ec05c62296a39d510558be`，生产核心固定 d541264。

Origin setup job `.../job-linksend-ae-jp-control-setup-origin-20261018-01`：正常 Initialize→一次性邀请码→Join，2成员身份/组一致、两路WSS认证及在线、已消费邀请码准确返回 PAIRING_CODE_USED，PASS。它验证控制面，不授予文件免确认/剪贴板权限，不替代桌面明确同意与文件传输。

## Cloudflare 精确入口变更

切换前 read-back 为 A `109.66.88.204`、proxied=true、TTL1；整区SSL为Full，http_config_settings无entrypoint。原值和回退计划保存于本机 `.artifacts/ae-20261018/cloudflare-before.json`（SHA256 `807e90be3ed8c78f35fd101d22435dfe2360e0b96ca2a6095e9c4e8048f3d62d`）。

根据 [Cloudflare 配置规则SSL说明](https://developers.cloudflare.com/rules/configuration-rules/settings/#ssl)，只新增 `http.host eq "linksend.oooai.de"` 的 `ssl=strict` 规则，整区Full原值不动。规则 ref `linksend_origin_strict_20261018`，ruleset `e2e2d4fde5ce4f3494a49112d34d75fa` / rule `21b66a3316b5464eaf7e46887bd24a28`；API回读确认仅此规则。需要回退时先核对 ref/表达式/ID，再只删除该任务规则，保留并发新增项。

源站业务与严格证书先通过后，仅 PATCH `linksend.oooai.de` 现有 A 为 `85.149.217.165`，保留proxied/TTL/其他字段。DNS记录ID `ce4582f802b9073b9e4d947db886503e`；精确回退为其备份旧值，不能覆盖其他记录。未改宿主防火墙/路由/代理或其他域名。

刚切换时公共 health 成功但 membership 返回401；没有因此重置profile或更换identity。随后 direct-origin 同一profile通过，再在公共路径验证同一组/身份和两路WSS通过；约3分钟的切换窗口与传播延迟相符，未将该推断升级为已抓包证明。对照 job `.../job-linksend-ae-jp-control-origin-public-compare-20261018-01`：公共HTTP/2 200、Cloudflare DYNAMIC、两路WSS PASS。Windows 独立 CLI fixture 的公共 HTTPS health 同样通过。

## 重启与恢复演练

job `/tmp/codex-ssh/job-linksend-ae-jp-restart-backup-drill-20261018-01`：主服务 PID变化后，通过公共TLS/WSS复核2成员、组、incarnation、密钥均保持，PASS。在线备份完整性/校验及不覆盖恢复到新目录通过，源快照 SHA256 `2ee0f5320345b3f8204f836094336e60fd3099333d9e46375b50db7f4437b4e5`。

但首轮独立恢复服务启动 FAIL：`unable to open database file (14)`。辅助文件检查实际发现恢复主DB已归linksend，`-shm`/`-wal`仍为root600；受限用户读取也失败。旧恢复进程已退出、8788无监听，生产服务和库保持运行。该轮不能标端到端恢复通过，后续撤销测试也尚未执行。

新增回归在Windows准确复现备份残留 `.partial-*.db-wal` 的FAIL。修复只将完成的备份/恢复**副本**归一为DELETE journal，并对已完成副本用immutable只读校验；活跃源继续读真实WAL，不修改其journal模式。普通恢复拒绝覆盖，复制失败仅回收自己创建的目标和sidecar。Windows及Debian新版回归均PASS，并确认源仍WAL；Debian job `.../job-linksend-ae-jp-backup-fix-preflight-20261018-02`。

## 修复后的服务级恢复与撤销闭环

新版helper来源 `b8ad80961804e1c2e329ffa0f04da2bf1c93fcb8`，SHA256 `05dfe7f4536d55bddb832b72eeb7257c633a4bf68626eb43ef392dfb4489a677`。安装先核对旧工具与b6来源一致、backup unit未运行，备份/回读后原子替换；立即对活跃生产库备份，确认2成员且副本无WAL/SHM。job `.../job-linksend-ae-jp-backup-helper-install-b8ad809-20261018-01`，备份 `/var/backups/linksend/backup-helper-fix-b8ad809-20261017T200513Z`；服务/备份timer保持active。

`.../job-linksend-ae-jp-backup-drill-v2-20261018-01` 完整exit0：在线快照SHA256 `d7b5cf90f2fe17815dbb3651edb19ab3a847122f328500faeabf15471a1a9dc1`，恢复到新目录 `/var/backups/linksend/restore-drill-20261017T200516Z`。以同一非特权linksend用户启动独立8788服务，严格TLS/WSS验证2成员、组/incarnation/公钥与原fixture一致；停止transient unit后8788无监听，生产库没有被替换。随后在公共API真实撤销B，成员从2变1、B认证被拒绝；重启主服务后继续拒绝B，PASS。

最新撤销状态再生成快照并重做独立服务：`.../job-linksend-ae-jp-revoked-restore-drill-20261018-03` 完整exit0，快照SHA256 `13aa56167771365bce72382eb82efaab48cf29416f07e5f1b836b595cee0b6c7`，隔离目录 `/var/backups/linksend/revoked-restore-drill-20261017T200648Z`。恢复服务与公共主服务均看到1成员并拒绝已撤销身份；测试unit停止，无8788残留，主服务最终PID283864、User=linksend、LogNamespace=linksend、active，备份timer active。

这里校验备份文件SHA256与恢复后的逻辑身份/撤销状态；服务按原配置重新启用WAL后，物理DB字节变化不等于恢复数据损坏。没有改生产目录权限、覆盖主库或放开身份校验。

## 当前限制与资源

服务实际运行，公共HTTPS/WSS、主服务重启、完整隔离恢复、撤销跨重启/最新快照恢复已验。客户端旧profile灾后恢复、旧队列/撤销上下文隔离、STUN、真实双NAT、双机原生文件旅程仍未闭合。STUN仅查到Debian候选coturn4.6.1-2，当前未安装且3478无监听，不能称跨NAT已恢复。Mac系统curl公共HTTPS本轮报LibreSSL SSL_ERROR_SYSCALL（job `.../job-linksend-ae-mac-public-health-20261018-01`），该平台公网旅程不计PASS，继续做进程级IPv4/解析诊断。

Windows准确包owned PID135152、Mac owned PID31256均已停止，剪贴板master=false、测试接收目录为空，保留profile和包供后续核对。Mac UI辅助访问拒绝、Windows UI点击未形成可验证导航，未当作原生旅程PASS。生产专属服务/备份timer是用户要求的持续资源，不随测试进程停止。

Mac单进程对照 `curl -4 --noproxy '*'` 仍以28/SSL connection timeout失败；job `/tmp/codex-ssh/job-linksend-ae-mac-public-health-v4-20261018-01` 已终态。未改系统代理/TUN/路由，也未跳过证书。下一步先核对该域名在Mac的实际解析与指定CF IPv4的进程级对照；不能把JP/Windows公共成功迁移为Mac成功。

最终CI回读：b8ad809的core push/PR `39065123673` / `39065127909`、desktop push/PR `39065123733` / `39065127766`、三平台包 `39065123767` 均success。包job Windows amd64与Mac arm64/amd64均通过；它们是该提交构建证据，未替代上述d541264原生包边界或剩余物理网络验收。
