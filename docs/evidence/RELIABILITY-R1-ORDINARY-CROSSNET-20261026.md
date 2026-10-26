# R1 准确36普通跨网双向与原任务恢复

2026-10-26，按用户最新要求只做各向一次最小功能旅程和一条必要恢复，不重复压力、race或完整矩阵。双方是本机Windows amd64与JP公共Linux amd64；未操作GUI/系统剪贴板，不接日常profile、生产组或scene05。

## 来源与隔离

准确核心为 `36cfce3d2e8f3d5eed8ab6588c5d172bc05f4d73`，复用既有clean checkout的独立ignored包构建。Windows/Linux验证器与原 `cmd/rendezvous` 的 `go version -m` 均为该revision、`vcs.modified=false`、Go1.27.1；构建后源码checkout仍clean。ignored driver独立记hash，不称为该core提交自带测试源码。

复用此前已实网运行的wanprobe，只增加进程内私有CA、零字节夹具及一条原Task Pause/Resume调用，不改产品代码/协议：

| 对象 | SHA256 |
|---|---|
| 双向baseline driver源码 | `e93b686415093625accc54cc53c3c87ddb3ca06246ef63ebc878ab9cb70fb95f` |
| Windows baseline exe | `378f9a3acbfc033b7e899f194208e113d686d0bee8597289961859017959053b` |
| Linux baseline exe | `b585c7560f5fc8cab6fde81c5a1b82ab54ab7bed156bc2a7468119e12b9786e8` |
| Linux原rendezvous | `c1d0db7f90bae90e299d6be40983891823e4001b73c14667d9b48dfb298ac20a` |
| 单次恢复driver源码 | `31e4424fe6ae8d45ca72f108eec38ccc5bccc2846c315e180ae67ab0bf3f2c28` |
| Windows恢复exe | `9fffee40b886ccf70bfea2883c85159fe24a8ac7860660625269be0f9b14be34` |

新私有协调服务只监听JP `127.0.0.1:18447`，新SSH本机18447转发仅承载HTTPS/WSS控制metadata。私有CA只追加到driver进程的正常TLS信任池，保留hostname验证；没有设置系统CA、跳TLS、公开新端口或修改宿主防火墙/路由/代理/DNS。文件仍为Windows实际接口↔JP公共接口上的Pion/QUIC UDP，relay=false；不是SSH/TCP文件中继。

JP新root `/tmp/linksend-ordinary36-20261026-01` 为0700，Windows使用独立私有profile。仅本测试Windows Initialize新组、创建短期邀请，经受管SSH交给本测试Linux Join；双方读取当前成员并固定公钥。没有用bootstrap、静态测试码、生产组、scene05的18445或其任何凭据。

## 一次双向最小结果

夹具是4文件，包括16MiB文件、中文文件/路径、零字节文件，另含空目录；共7个文件/目录条目、16,777,310B。接收回调仅对这两个自有隔离身份的一次测试清单明确同意，不写默认免确认或剪贴板授权。

| 方向 | 发送/接收流程耗时 | 真实结果 |
|---|---|---|
| Windows→JP | 171622 / 172498 ms | 两端Completed/exit0，7/7集合与SHA一致，TransferID及manifest digest相同，srflx→host |
| JP→Windows | 31146 / 41850 ms | 两端Completed/exit0，7/7集合与SHA一致，TransferID及manifest digest相同，host→srflx |

两方向双方TLS1.3（772）、ALPN `linksend/1`、relay=false，method实际为 `direct_unknown`。按实际结果保留该method；这是Windows单边映射↔公共Linux，不推断双NAT/原Windows双端现场，也不从该耗时宣称性能改善。对应原始结果和逐路径比较在 `BIDIRECTIONAL-VERIFICATION.json`，mismatches均为空。

## 唯一原任务恢复主路径

复用原自有Windows源和原身份/授权，只创建一个StartSend任务。在实际verified **4,194,398B**（4MiB加已确认小文件）时显式 `PauseTask`，等同一任务 `paused/can_resume=true`，受控接收进程保留原目标/checkpoint并退出；随后只调用一次 `ResumeTask`，没有StartSend第二任务或改用重新发送。

- 原始及最终TaskID均 `20261026T022955.052083800Z-00000001`。
- 原始、恢复及接收方TransferID均 `9a954c59e395de5054e261d9d564fd7d`；双方manifest digest保持。
- attempt从 `dd008498c2a6ea7e2dd88bb171e7a9b6` 进入 `f555be56dd7f233c196714955584d08a`。
- 最终发送task `completed`，verified=committed=16,777,310、bilateral_confirmed=true、TLS772/ALPN linksend/1/relay=false；应用层retransmitted=0，不外推为UDP零重传。
- Linux重新打开原 `received-02` 目标及checkpoint后完成，仍为原TransferID，7/7条目/hash一致，无额外副本。最终双方exit0；接收最后阶段elapsed133167ms。
- 第一次低层Receive因受控Pause退出1、`DIRECT_FAILED`及Pion关闭socket日志真实保留在 `jp-recovery-paused.json` 和作业stderr；这不是未诱导的连接故障，也没有被改记为第一次成功。最终job只在第二阶段完成后退出0。

`RECOVERY-VERIFICATION.json` 汇总绑定、逻辑字节、hash与真实终态；Windows原始SQLite task snapshot及接收checkpoint/源/副本保留。该结果是headless核心API，不替代Wails原生恢复、物理网络切换或睡眠唤醒。

## 作业与精确收尾

SSH别名为 `jp-linksend`。所有request均以同名固定ID保存，job路径为 `/tmp/codex-ssh/job-<requestId>`；完整返回及read cursor保存在证据根对应JSON，观察未重启已有job：

- `ls-ordinary36-preflight-20261026-01`、`ls-ordinary36-prepare-20261026-01`、`ls-ordinary36-join-20261026-01`：finished/exit0。
- `ls-ordinary36-receive-20261026-01`、`ls-ordinary36-send-20261026-01`、`ls-ordinary36-recovery-20261026-01`：最终finished/exit0，受控恢复首段exit1另保留。
- `ls-ordinary36-coordinator-20261026-01`：原worker3005366，实际server3005367；核对实际exe与父进程后SIGTERM，server及job正常exit0。
- `ls-ordinary36-retire-20261026-01`：finished/exit0，仅删本root的CA/server私钥、CSR/serial和邀请文件，保留profile/DB/自有源/接收副本。

Windows显式revoke仅本隔离Linux peer，exit0；私有coordinator的本地组/audit保留，不称删除组。SSH forward owned PID67508经过实际image和开始时间核验后停止，本机18447无Listen，自有数据/forward进程为0。该跨shell forward数值退出码不可观察，`forward-exit.json`保持null，不补造0。

首次收尾谓词因PowerShell自动日期转换及Windows路径大小写严格比较安全中止，未误杀；随后用保留字符串日期、UTC ticks和OrdinalIgnoreCase image比较核验后退场。它是收尾脚本谓词问题，不是产品网络缺陷。本轮未发现需要改产品源码的缺陷，没有重试GUI、GHA或扩大验证。

本地原始证据根：`D:/apps/Osend/.artifacts/ordinary-crossnet-36-20261026`，含baseline/recovery driver、三份buildinfo、两方向结果、恢复snapshot、job/request/cursor、撤销及退役回执。临时邀请/私钥不在仓库或日志。只关闭R1本次普通双向/受控原任务恢复子项；真实LAN、睡眠/网络变化、Windows双端现场及其他R0–R6必做项不倒记完成。
