# R5 一次性临时分享：Windows 与公共 Linux 双向真实文件证据

日期：2026-10-26（Asia/Shanghai；原始命令日志使用 UTC）。

准确源码 `701e23b067c38747813b84fa9c93cee6d19e0901` 的隔离 Go 验证器，已完成 Windows → JP 公共 Linux、JP 公共 Linux → Windows 两方向的真实一次性分享。每次双方本机 task/share 与服务器 grant 均为 `completed`，文件树 SHA256 相同，未增加永久信任或剪贴板授权。

这是应用核心真实数据通路证据。控制面使用本任务自有 SSH 测试通道；当前结果不满足 Windows ↔ Windows、受限双 NAT、Windows ↔ Mac 或准确 Wails 发行包的实机验收，也不将 R5 或完整 Goal 标记完成。

## 准确来源与验证器

验证器从准确提交的干净受管 checkout 构建。测试源码复制到被忽略的 `.artifacts/share-crossnet-draft/driver`，不增加生产接口。Go `debug.ReadBuildInfo` 的真实记录为：

```text
vcs.revision=701e23b067c38747813b84fa9c93cee6d19e0901
vcs.modified=false
driver_source_sha256=50876aa1c6ada41f8e313a712f839127f63eb40176e19366cfb45e1bdbed5e3b
```

未通过 linker flags 改写来源或 modified 状态。coordinator 与两个 actor 均要求相同的真实构建 stamp；测试驱动源码摘要单独记录，不冒充提交内生产代码。

| 本地构建产物 | 字节数 | SHA256 |
| --- | ---: | --- |
| `share-crossnet-driver-linux-amd64` | 26146475 | `4e73f3fc6c7b158a05deb4ad117adc2c1e47ddfb4b6da723c3ccf87a96f64eff` |
| `share-crossnet-driver-windows-amd64.exe` | 26457088 | `3b1b03346fd18acb7fc2b1eaec6b34f2c84623967aa3f609cd050afdc5576a47` |
| `source-701e23b.tar.gz` | 21750049 | `6b75ccf1a355ed6c63c1f7dc8b45adc5e74a84926536c2da4307344396cf8609` |

以上摘要来自 `build-receipt.json`，两侧运行回执均再次报告同一 commit 和 `source_modified=false`。

## 拓扑与授权路径

- Windows UDP 绑定真实本地单播地址 `192.168.1.122` 的临时端口；JP Linux 绑定 `85.149.217.165` 的临时端口。Pion ICE 选择的候选类型为 Windows `srflx`、Linux `host`。
- 独立 coordinator 仅监听 JP 的 `127.0.0.1:18443`，使用测试 TLS 证书。双方 canonical origin 同为 `https://127.0.0.1:18443`；本任务创建的 SSH `-L` 将 Windows 回环 TCP 端口转发到该回环监听。HTTPS/WSS 签名协调和测试 harness 的控制 metadata 经此通道。
- 三个进程均未启用 loopback candidate。文件正文由真实 Pion ICE / QUIC 的独立 UDP 端点传输，不经过 SSH、HTTP、JavaScript IPC 或第三方 relay。没有新增端口映射，也没有修改宿主防火墙、路由或代理。
- 测试 CA 仅追加到验证器进程的 TLS transport。证书 SAN 包含 `127.0.0.1`，保留证书链与主机名校验；未使用 `InsecureSkipVerify`，未更改宿主根信任。
- 每个方向使用新的独立私有 profile、coordinator runtime 和一次性分享。分享码及角色身份 pins 只通过 token 保护的测试控制 metadata 在内存中交接，不进入 stdout 或版本化文档。

驱动实际调用 `CreateTemporaryShare` → `ClaimTemporaryShare` → sender `RefreshTemporaryShare` → 精确 `ApproveTemporaryShare(ClaimID, ReceiverPublicKey, Generation)` → 双方显式 `StartTemporaryShare`。sender 先将 pending receiver 的身份与独立登记的 receiver pin 比对；receiver 同样核对 sender 身份。receiver 收到加密清单后，先断言目标目录仍为空，再显式调用 `ConfirmTemporaryShareSave(..., keep_both)`。未调用永久配对、信任或剪贴板授权接口，也未读取用户剪贴板。

## 两方向结果

fixture 包含 16 MiB payload、31 字节中文说明、零字节文件和嵌套空目录：共三个文件、manifest 六条 entry、一个空目录，总文件字节 `16777247`。

| 方向 | run_id | 实际文件结果 | 进程退出证据 |
| --- | --- | --- | --- |
| Windows sender → Linux receiver | `b85a592c2157816e` | 双方输出 `PASS`，task/local share/grant 均 `completed` | Linux actor 的 typed SSH job 终态为 exit 0；Windows 后台 sender 输出 PASS 且 PID 已结束，但启动父 shell 已结束，exit code 为 `NOT_OBSERVABLE`，不补记 0 |
| Linux sender → Windows receiver | `6358b24072cf4c2c` | 双方输出 `PASS`，task/local share/grant 均 `completed` | Linux actor 的 typed SSH job 终态为 exit 0；Windows receiver 的前台命令 exit 0，60418 ms |

反向 Windows 命令收据的 UTC 时间为 `2026-10-25T20:54:59.1594014+00:00` 至 `2026-10-25T20:55:59.5756943+00:00`。正向 Windows 仅有后台 owner 与 stdout 记录，不能据此计算完整命令耗时或恢复退出码。

四个 actor 回执核对结果如下：

| 检查 | 实际记录 |
| --- | --- |
| 源与接收树 SHA256 | `09d86cf53e1dcfd3a5ac90ed0dd3aa01755e785aca167fe5de79d53701991ef5`，两方向双方一致 |
| verified / committed bytes | 每个 actor 均 `16777247`；sender 的 sent、receiver 的 received 同为 `16777247` |
| 空目录 | 双方均为 1；receiver 的 `preview_destination_empty=true` |
| 完成依据 | 本机 task=`completed`、本机 share=`completed`、grant=`completed`、`bilateral_confirmed=true` |
| 数据传输 | `transport=quic`、`tls_version=772`（TLS 1.3）、`alpn=linksend/1` |
| 候选与方法 | Windows `srflx` ↔ Linux `host`；产品字段实际为 `connection_method=direct_unknown`，保留原值，不改写为更具体的方法 |
| relay | 双方 `false` |
| 永久授权 | 每个 actor 的 `permanent_trust_rows=0`、`clipboard_grant_rows=0`、`group_membership_denied=true` |

Linux 两个 stderr 文件各保留两条 Pion 关闭 UDP 后的 `use of closed network connection` 日志。它们不是空 stderr；typed SSH job 实际 exit 0，双方完成、落盘与哈希断言仍均通过。没有删除这些日志或将其隐藏为“无错误输出”。

## 原始回执与任务标识

本机证据根为 `.artifacts/share-crossnet-draft/candidate-701e23b/`。这是被忽略的测试产物目录，普通仓库克隆不会自动获得原始产物。该目录包含：

| 回执 | 用途 |
| --- | --- |
| `build-receipt.json` | 准确源码 stamp、验证器与源码归档摘要 |
| `windows-forward-owner.json`、`windows-forward.stdout.jsonl` | 正向 Windows 后台 owner / 双方 PASS 字段；不包含可观察的退出码 |
| `linux-forward.stdout.log`、`linux-forward.stderr.log` | 正向 Linux 校验、双方结果与原始 stderr |
| `windows-reverse-command.json`、`windows-reverse.stdout.jsonl` | 反向 Windows 前台 exit 0、60418 ms 与双方结果 |
| `linux-reverse.stdout.log`、`linux-reverse.stderr.log` | 反向 Linux 校验、双方结果与原始 stderr |
| `windows-closure.json`、`linux-closure.json` | 自有通道、进程、测试 listener 与临时凭据收尾 |

远程 actor 与 coordinator 的终态由原执行会话的 typed SSH job 状态确认。可按以下 request ID 回查对应 `/tmp/codex-ssh/job-*` 任务，不以 stdout 的 PASS 字样替代 job exit 状态：

| 角色 | request ID | 原会话终态 |
| --- | --- | --- |
| 正向 Linux receiver | `ls-r5-wan-receiver-forward-20261026-01` | finished / exit 0 |
| 反向 Linux sender | `ls-r5-wan-sender-reverse-20261026-01` | finished / exit 0 |
| 正向 coordinator | `ls-r5-wan-coordinator-forward-20261026-01` | exit 0 |
| 反向 coordinator | `ls-r5-wan-coordinator-reverse-20261026-01` | exit 0 |
| Linux 收尾 | `ls-r5-wan-close-20261026-01` | exit 0 |

## 收尾与证据边界

Windows 自有 SSH child 由 `Stop-Process` 主动停止，观察到 exit `-1`；不将主动终止写成自然 exit 0。helper 已结束，本地测试 listener 数为 0。Linux 收尾回执记录测试 listener 已关闭、自有 driver process 列表为空，两个 coordinator 均已结束。

临时测试 TLS 私钥、harness token 和 runtime 文件已按精确路径删除。两侧 fixture 源、接收文件及私有 profile 证据保留，没有递归清理测试根或用户资料。Windows 回执记录 `host_network_changes=false`，Linux 回执记录 `production_changes=false`。

公网 `18443` 的先行 TLS 预检实际超时，因此本轮使用上述回环 coordinator / SSH 控制面方案，不能声称生产公网 HTTPS 入口已验证。Windows Schannel curl 对该测试 Ed25519 握手报 35；同一测试证书的 Python 正常链/主机名校验取得 health 200，随后 Go 完整真实流程通过，不能把 curl 客户端能力差异记为产品 TLS 故障。

`dual_nat_proven=false` 保留在两方向原始回执中。JP 为公共 Linux endpoint，此证据不证明两个受限 NAT，也不证明用户暂时无法试用的准确 Windows ↔ Windows 候选。验证器直接调用 Go 应用 API，没有覆盖准确 Wails 包的原生交互、安装或升级旅程，也没有覆盖 Windows ↔ Mac。

这些真实网络结果只归属于 `701e23b067c38747813b84fa9c93cee6d19e0901`。后续 `103ba61c13ba02a02a0037bd891c2a64777cec29` 仅含 native 测试 profile scanner 的修正，不把本轮结果移记成该提交或新发行包的实测；其新 CI 在本证据整理时仍待运行结果，不预记通过。
