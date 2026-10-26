# R5 生产升级事务草案

日期：2026-10-26。状态：**DRAFT / NOT_AUTHORIZED / NOT_DEPLOYED**。本文件准备审阅目标、备份、验证与回退；未执行生产备份、上传、配置修改、DB 迁移、服务 stop/start 或 release 切换。当前准确候选为 `5ee9a40` 的独立clean Linux构建；已分别解析708/850/5ee的Linux rendezvous真实编译依赖，全部输入相同，复用708一次合成库迁移/兼容回退，见[证据](RELIABILITY-R5-SCHEMA5-FALLBACK-20261026.md)。708测试失败与850Windows包失败保留。5ee push/PR的core、desktop及Windows包job均首次成功；Windows静态验包仍进行，整体packages未在本页确认成功。尚未取得生产变更授权。

## 目标与来源门

最终核对2026-10-26T05:28:25Z：5ee的push/PR core、desktop及三平台packages jobs均attempt1/success；Windows资产仅下载一次，Actions/API/上传日志/下载SHA一致，BUILD-INFO/clean来源、主PE锚、ZIP与安装器内main/helper同hash全部STATIC_PASS。未执行候选/安装器/clipboard，不据CI声称Mac原生实测。最终收据 `.artifacts/r2-helper-ci-5ee9a40/FINAL-RECEIPT.json`，准确Windows资产摘要与范围见[候选证据](RELIABILITY-CANDIDATE-5EE9A40.md)。下文“静态验包进行中”等句保留其先前观测时间，以此最终核对为准。准确候选、目标及合成库兼容回退已可审阅；生产变更仍需第四节要求的明确批准。

既有只读调查为 `.artifacts/crossnet-20261026/server-candidate-99a6c0e/DEPLOYMENT-REVIEW.md`（UTC 2026-10-25 21:26–21:29），后续公网 health 回执 `.artifacts/hosted-windows-peer-draft/production-health-root.json`（UTC 2026-10-26 00:42）。它们是历史基线，执行前必须重新核实，不能当作此刻 live 状态。

2026-10-26T04:54:45Z 已通过保存的 `jp-linksend` 别名只读刷新一次：原 unit active/running、PID283864、相同 current/live 镜像与下表SHA，标准IP SAN/TLS验证的本机health仍为0.5.0/relay=false/无分享，原schema4/quick_check=ok。配置SHA256 `2873dec4e49d7fcd1f38225dd5ce88508fd2492e31a8de85ba0df1092e0a19b0`、分享开关缺省false；未备份/上传/改配置/迁移或stop/start。脚本和退出0收据见 `.artifacts/r5-server-7084685/production-readonly-refresh.sh`、`PRODUCTION-READONLY-REFRESH.json`；远程证据 `/tmp/codex-ssh/job-r5-prod-readonly-refresh-20261026-01`。该观测供准确事务审阅，获批执行前仍须重核漂移。

另一次公网只读观测在2026-10-26T05:26:57Z：正常TLS验证的 `https://linksend.oooai.de/healthz` 返回HTTP200、0.5.0、无分享能力、relay=false，生产未改；收据 `.artifacts/r5-server-7084685/PUBLIC-HEALTH-REFRESH.json`。这是不同时间的health观测，没有再次读取unit/image/DB，不与04:54结果合成同轮证明。

850准确push/PR的core与desktop均首次成功；Windows包因缺失一个生成绑定字段而被source-clean gate拒绝，原失败保留。486283f仅同步该字段并通过一次前端构建，已随R2guard 5ee9a40推送；新准确5ee CI/Windows包待核。这里的850 standalone Linux候选来源及708合成库旅程不倒改为5ee实测，也不将Windows包失败记成server测试失败，详见[派生绑定证据](RELIABILITY-R5-GENERATED-CAPABILITY-BINDING.md)。

后续准确核对（保留上段当时状态）：5ee push core `40273165842`、desktop `40273165901` 与PR core `40273168788`、desktop `40273168780` 均attempt1/success；Windows job `121410208098`（run `40273165815`/attempt1）在2026-10-26T05:24:16Z首次success，包已上传。核对文件 `.artifacts/r2-helper-ci-5ee9a40/{ci-runs.json,windows-job.json}` 均绑定完整5ee；该次记录的packages整体仍in_progress，Windows静态验包仍由独立流程进行，不等待Mac或推测整体结果。

| 目标 | 既有记录 / 事务范围 |
| --- | --- |
| SSH alias / public origin | `jp-linksend` / `https://linksend.oooai.de` |
| 唯一 unit / 服务身份 | `linksend.service` / `linksend:linksend` |
| current / 原 release | `/opt/linksend/current` → `/opt/linksend/releases/d5412642e49460413fdda249f77d03647075f41c` |
| 原 live/disk SHA256 | `688702887cf7b87351d81384e941a352e13c3cd4701e437757f0922241e83d45`，产品 0.5.0 |
| config / DB | `/etc/linksend/server.toml` / `/var/lib/linksend/control.db`，既有 schema4、WAL、quick_check=ok |
| backend listener | TLS `127.0.0.1:8787`，原公证书 IP SAN 校验；公网仍走原代理 |
| 当前准确构建候选 | `5ee9a40e6741749a4bc2d098c85a58e2cc9bd785`；独立 clean checkout、Go1.27.1/linux-amd64/CGO0/trimpath，真实 `vcs.modified=false`；18,188,312B、SHA256 `92e3ce635246925e0492d3e092f3e6fe9615cf7fd9d8213cf89dfc16dc927a59`；实际生产依赖输入与708/850相同；准确core/desktop与Windows job首次成功，Windows静态验包仍进行 |
| 新 release / 私有事务备份 | `/opt/linksend/releases/5ee9a40e6741749a4bc2d098c85a58e2cc9bd785` / `/var/backups/linksend/<实际UTC>-r5-5ee9a40`，均不得覆盖已有目录 |
| 唯一计划配置变化 | 将既有键 `experimental_share_control` 明确设为 true；其余配置、TLS、unit、环境、代理/DNS/防火墙/路由保持原值 |

最终5ee本机二进制位于 `C:/Users/Wen/.codex/supervision/r5-server-7084685-20261026-01/rendezvous-linux-amd64-5ee9a40`；独立源目录 `source/` 构建前后tracked/untracked均clean，buildinfo真实绑定完整5ee revision。以既有 Go1.27.1、`GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath` 准确构建一次，收据 `.artifacts/r5-server-7084685/BUILD-RECEIPT-5ee9a40.json`；Linux实际 `-version` exit0为 `LinkSend rendezvous 0.6.2`。

按相同linux/CGO0环境分别在准确708、850、5ee执行 `go list -deps -json ./cmd/rendezvous`，解析实际选中的repo内生产源/汇编/系统对象/embed文件及go.mod/go.sum，而非按模块名推测无关。三版均为cmd/rendezvous、identity、protocol、server、sharegrant、store六个repo包、16个输入文件（本目标无embed），每个mode/path/blob及实际字节SHA完全相同，外部模块版本和go.sum同样未变；完整规范输入摘要 `c0d6665d5f8a6d60208ef4e117ddeb835a5dba188370fa942a1644136e79e2bb`，见 `.artifacts/r5-server-7084685/RENDEZVOUS-DEPENDENCY-EQUIVALENCE-5ee9a40.json`。据此复用708一次迁移/兼容回退，无850或5ee服务场景重跑。旧708与850二进制、各自构建收据及真实708 PID/结果继续保留，不倒改为5ee实测。dirty工作区或仅目录名不构成准确来源。

本次仅在已有 WSL Ubuntu 上运行 127.0.0.1 自有服务与 `/tmp/linksend-r5-migration-7084685-20261026-01` 合成数据，没有 SSH、公网请求或生产访问。合成库原有 groups/devices/invitations/membership_requests 为 1/2/1/1 条，准确708迁移后原表摘要不变，唯一0600自动备份完整保留schema4；原成员签名请求仍成功，新未入组身份只创建一条分享。准确36随后在关闭分享的同一schema5工作库上启动，原成员仍可认证，分享426/SHARE_UNSUPPORTED，全部原表与新增分享行不变。三个server均SIGTERM exit0、listener关闭。该验证不代替下述授权后生产一致性副本及真实TLS/公网后验。

## 授权后拟执行的事务

1. **只读重核并阻断漂移。** 精确核 unit/current/live exe/hash、listener 所有权、config 安全字段、TLS/public health、schema/quick_check、备份与 release 空间。若 release/hash/config/schema 变化，先更新审阅，不覆盖外部变化。只读读取明文安全配置字段，不打印环境、私钥、bootstrap token、码或成员正文。
2. **先准备兼容回退和私有备份。** 获批后才建新 `0700` 事务目录、`umask 077`；完整字节备份原 config、unit 和旧二进制，在服务器私有目录回读 hash/权限，不下载进仓库。使用 SQLite Backup API 从 live WAL 连接取得一致性快照；不可裸 cp 活跃 control.db，或用 immutable 忽略 WAL。备份发布前核 integrity/foreign_key/schema，保存摘要及永久表逻辑计数/内部规范摘要，不导出成员记录。
3. **副本上先走完整升级与回退。** 保护目录中建独立工作副本，用准确候选、专属 loopback 临时监听和隔离 config 开启分享；只能改副本。确认唯一 schema4→5 迁移、自动 `pre-share-v5-<nonce>.sqlite` 备份完整有效、永久 devices/groups/invitations/membership_requests 不变、health 双能力 true/relay=false、现有普通路径兼容。用 schema5-compatible 回退候选且开关 false 打开该升级副本：保留数据，分享明确 unsupported，普通 health/成员结构可用。副本验证未通过不得进入生产切换。
4. **停止唯一 unit，取最终一致性备份。** 有界确认该 unit 的进程与 8787 监听已结束，再做 final DB backup、schema/完整性和 hash 回读；保存最终 config hash。失败时不切 current；在未迁移前可恢复原 unit。停止控制面可能打断协商；健康已建 P2P 不经此服务，仍不能承诺所有阶段无感。
5. **一次原子切换。** 校验上传 staging 的准确镜像后安装到从未存在的新 release，同卷 rename 发布。原 config 以受限临时文件只变明确开关，核对其余解析值/原字节变化范围，恢复原 owner/mode；原子替换前重查 checkpoint。用同目录临时 symlink 原子换 current，启动原 unit，不改代理或网络。候选 startup 才执行真实 DB 的唯一备份与4→5迁移；失败保留阶段和备份，不盲重跑。
6. **独立后验。** 核真实 MainPID image/path/hash、原监听归属、标准 TLS 主机名与公证书校验、本机和公网 health 的准确版本、`share_grants_v1=true`、ready=true、relay=false；只读核 schema5/quick_check、迁移备份和永久表一致性。普通既有身份必须仍可认证；临时独立身份不能枚举成员或打开普通 WS。授权中的两端小文件旅程须仍明确两次同意、独立 QUIC/hash/双方本机 Completion、永久 trust/剪贴板权限零增加，不能以 health 代替它。原 Windows↔Windows 故障及双 NAT 证据继续单列。

SQLite 自动迁移备份与显式 final backup 都须留存，不清理旧 release、身份、用户文件、DB、WAL/SHM 或业务历史。不调用生产 bootstrap/join 当作只读探针；若需有副作用的隔离 probe，纳入最终明确授权。

## 回退边界

启用分享后控制库为 schema5，原 0.5.0 和旧 99a6c0e 的 schema4-only 二进制不能直接打开；**不能仅把 current 指回旧版本**。第一回退是当前 schema5-compatible 候选关闭明确分享开关并保留现 DB；若候选本身不可用，使用预先验证的 schema5-compatible fallback，恢复原 config 的关闭状态后保留所有新数据。

现有准确 fallback 来源候选是 `36cfce3d2e8f3d5eed8ab6588c5d172bc05f4d73` 的 standalone rendezvous：本机文件 `C:/Users/Wen/.codex/supervision/ordinary36-20261026-01/rendezvous-linux`，18,221,930B、SHA256 `c1d0db7f90bae90e299d6be40983891823e4001b73c14667d9b48dfb298ac20a`；保存的 build info 为 Go1.27.1/linux-amd64/CGO0、该 revision、modified=false。本次已实际验证其在708升级后的自有非空合成schema5工作库上关闭分享启动、普通签名成员请求、明确unsupported及新增分享行保留，见[兼容回退证据](RELIABILITY-R5-SCHEMA5-FALLBACK-20261026.md)。**尚未访问或验证生产升级副本**，因此仍须完成授权后的上述副本门，才能选为生产事务的实际 fallback。关闭时它不广告新能力，当前客户端会明确 unsupported。

回退前只操作已核对的 unit/current；若外部修改，停止自动动作。保留失败 DB/备份与两个 release，在已确认无进程持有时原子切到已验 fallback，再验 schema5/永久表、真实 image/hash、TLS/listener 与普通 health。关闭能力不等于撤回已建立 P2P 的在途字节或对任意活跃 grant 即时撤权，须记录实际受影响任务，不伪记全局取消完成。

只有另行取得明确数据回退授权，且证明 final backup 后没有需保留的新成员/业务写入，才可规划恢复旧 schema4 数据。未知、损坏或已接受新写入时保留当前库，不能用早期备份覆盖它；DB/WAL/SHM 恢复须作为独立受审事务。
