# R5 生产公网一次小文件后验

日期：2026-10-26。已部署服务与本次两端核心均为准确 `5ee9a40e6741749a4bc2d098c85a58e2cc9bd785`（产品0.6.2）。修正隔离验证器观测偏差后，新私有profile完成一次 Windows → JP公共Linux、单文件1024B临时分享。双方本机完成、hash、永久授权隔离及退场均已读回。范围限headless核心；Windows双端、双NAT、Wails安装/键盘粘贴及完整R0–R6保持原验收边界。

## 准确来源与结果

验证器从准确5ee独立clean源码以Go1.27.1、GOWORK=off、CGO0构建Windows/Linux，真实buildinfo均为该revision且modified=false。验证器为ignored辅助源码，独立SHA256 `197e4743cd8d22c41bd90e20ea833dcd168ee6a9aa1cc1fdc18d7619de313e6f`；Windows镜像 `3f2b25fcebe6e10d67bf9100cfbbc74465c0cd4ccec442eb849705cf4f9b5cdd`，Linux镜像 `4c8bb5443bbbf888be457aa9a1ed231455a2a1779f2d04a9bc8db18aa7d329c8`。构建证据 `.artifacts/r5-production-5ee/PUBLIC-SHARE-BUILD-RECEIPT-V2.json`。

产品ShareAPI和两阶段WSS全部直连 `https://linksend.oooai.de`，使用正常证书链/主机名校验。自有JP loopback harness只交接测试身份pin、分享码与阶段回执，经SSH送到Windows；不代理ShareAPI、WSS或正文。实际执行Create→Claim→发送方核对领取身份并Approve→接收方加密预览后明确ConfirmSave→原Pion/QUIC文件引擎。身份、分享码及harness token仅保存在repo外私有supervision/服务器私有测试目录，未输出或写入版本化材料。

| 项目 | Windows发送方 / Linux接收方实际结果 |
| --- | --- |
| 本轮run | `469db01fdf495a24`；Windows UTC07:48:45.012→07:49:04.049，actor exit0/非强退 |
| 文件与清单 | 1文件1024B；manifest2条（目录与文件），双方摘要 `a08abdd32e31e410291081c8d41d2b8ffa945e202accbcd755e4eec3faaf0d60` |
| 文件SHA256 | 源与Linux实际接收文件均 `ff29997907aed0fcb4456a2e4b450fa978bfbcae5e79678279cfd70f928b03b1` |
| 目录树SHA256 | 双方 `941ac0fc0615b2d2357def1026a38f389daa4c6886adfdcf44f410ffb3781c0d` |
| 原文件任务 | 双方TransferID `385a882abcaead1d4b1e9b4429e7a728`；本机task/share与服务grant均completed，bilateral=true |
| 完整性与落盘 | 双方verified/committed均1024；接收方实际文件再次独立读回hash一致 |
| 数据连接 | TLS1.3、ALPN linksend/1、relay=false；Windows srflx ↔ Linux host；method实际为direct_unknown |
| 公网控制观测 | 每端25次产品HTTPS transport请求，其中2次分享WSS升级，正常TLS；harness仅metadata |
| 永久授权隔离 | 两端trust=0、clipboard grants=0；两端普通成员请求实际HTTP401；没有新建永久组 |
| 系统DNS兼容 | Windows核心既有安全DoH4次/4成功；Linux系统解析可用，DoH0次 |

两端受控stdout的完整PASS JSON逐字段相等；原始回执、实际文件读回与所有退出码汇总在 `.artifacts/r5-production-5ee/PUBLIC-SHARE-PASS-RECEIPT-V2.json`。

## 首尝偏差与收尾

首尝v1验证器SHA `ce2f1358e5897b286c7872bcfae032eee63bcac556073058977aac8629f16469` 的全局HTTP audit主动拒绝产品origin/harness之外的请求，因而拦住核心原有安全DoH。Windows系统STUN A解析实际为 `198.18.0.186` / `198.18.0.165`，需要该fallback；首个preview ICE在CHECK_TIMEOUT结束，双端exit1、文件字节0，第二transfer scope未创建。本次自有grant已明确revoke，v1源码、FAIL与诊断保留于 `PUBLIC-SHARE-FAILURE-RECEIPT.json`，该受验证器影响的结果不作为产品故障结论。

随后仅对两个已配置STUN各发送一次Pion绑定请求，无重传/网络设置修改：正常TLS Cloudflare DoH解析主STUN为 `109.66.88.204:3478`（1请求/0响应超时），备用Cloudflare为 `162.159.207.0:3478`（1请求/1合法响应，实际映射 `188.253.118.237:4865`）。记录为 `STUN-ONCE-PROBE.json`。只把v2 audit改为被动观测，不修改生产核心；获得备用STUN可达证据后，按授权新建fresh02及新grant完成上述唯一1KiB后验，未复活旧grant或扩展方向/矩阵。

fresh02 Linux receiver与harness原typed SSH job均finished/exit0；Windows actor正常exit0，专用SSH forward主动停止exit-1。原PID均已结束，127.0.0.1:18461监听关闭；grant已completed，无活跃测试任务。本次只读退场核对显示生产unit仍active/PID3007149，本子任务未重启服务或改配置/DB/防火墙/路由/代理/DNS。源、接收文件、profile和受限目录证据保留，不清理用户数据。Linux stderr保留182B原Pion关闭UDP日志，不记成空stderr。

原作业可按 `/tmp/codex-ssh/job-r5-public-share-5ee-{harness,receiver}-20261026-02` 回查；最终只读收尾为 `job-r5-public-share-5ee-final-observe-fixed-root-20261026-02`，exit0。收尾脚本首次把包含引擎私有记录的接收根整体误作fixture而断言失败；修正为核对明确fixture子树后通过，没有重传或修改接收内容，原只读失败保留。接续入口 `.artifacts/r5-production-5ee/PUBLIC-SHARE-RECOVERY-V2.json` 已标明退场，不重启。
