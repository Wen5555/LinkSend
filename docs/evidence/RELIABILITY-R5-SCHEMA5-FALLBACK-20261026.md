# R5 准确 Linux 构建与合成库兼容回退

日期：2026-10-26。范围：准确 `708468579c2d969f9e028d52c06eee4defb43164` standalone rendezvous 的一次最小 Linux 功能验证，以及测试/文档修正后 `850b9d115ac2a2e460a0678039a1642cd2c1de9b` 的准确重建和相同产品输入核对。**没有生产访问/部署、SSH、外网请求、GUI、系统剪贴板或文件数据面重测；R5 不因此整体完成。** 708整体CI的真实失败保留，850准确CI仍待，不能把本记录作为CI成功或部署可用证据。

## 准确构建

从本机仓库建立独立 detached checkout `C:/Users/Wen/.codex/supervision/r5-server-7084685-20261026-01/source`；构建前后 `git status --porcelain --untracked-files=normal` 均为空，HEAD 为完整708。以已有 Go1.27.1 执行一次：

```text
GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0
go build -trimpath -o <outside-source>/rendezvous-linux-amd64 ./cmd/rendezvous
go version -m <outside-source>/rendezvous-linux-amd64
```

构建 exit0，18,188,312B、SHA256 `4ec005a23c5da98ae77367924e1b1c41efbec34f58ddec396b86835bc09c4de6`。真实 buildinfo 含 `vcs.revision=708468579c2d969f9e028d52c06eee4defb43164`、`vcs.modified=false`、linux/amd64、CGO0、trimpath。Linux 实际 `-version` 返回 `LinkSend rendezvous 0.6.2`。二进制在该监督目录；收据 `.artifacts/r5-server-7084685/BUILD-RECEIPT.json`、完整 `GO-BUILD-INFO.txt`、`build.log`。

后续850只改两个 `*_test.go` 和三份docs，没有产品输入变化。用完整 `git ls-tree -rz` 的mode/path/blob列表比较，排除docs/和`*_test.go`后所有tracked文件完全相同，SHA256 `d6890c344e21f3ef984f409f77264f8f7e69944fa3e8016f361055dd5063edfd`。同一独立checkout干净切至准确850，以同工具链/参数构建到源外新文件 `rendezvous-linux-amd64-850b9d1`：exit0、18,188,312B、SHA256 `6efa08b5919113281d1fcc8c81f7b63316fc0492df33de9dfe930c273252a784`，真实buildinfo完整850/modified=false，构建前后clean；实际Linux `-version` exit0/0.6.2。新构建收据 `BUILD-RECEIPT-850b9d1.json`、`GO-BUILD-INFO-850b9d1.txt` 均位于同artifact目录。复用下面708旅程，没有重跑850服务、迁移或矩阵；原708 PID/结果仍绑定708。

## 一次合成库旅程

使用本机已有 WSL Ubuntu（实际 Linux 6.6.87.2/amd64），不安装运行时、不修改 `.wslconfig` 或网络。执行一次 `wsl -d Ubuntu --exec python3 /mnt/d/apps/Osend/.artifacts/r5-server-7084685/migration-smoke.py`，exit0。该脚本独立记录hash，逐个启动准确36与708的真实Linux二进制，服务仅绑定动态 `127.0.0.1`，不启动TLS或公网入口。

1. 准确36以关闭分享配置创建自有schema4，正常退出后仅写入合成的 groups/devices/invitations/membership_requests（1/2/1/1条）。两个设备公钥由本次内存Ed25519身份产生，未读取日常身份/凭据。用SQLite Backup API取得独立工作副本；原schema4保留不动。
2. 708显式开启分享，从该工作副本实际执行4→5。health声明 `share_grants_v1=true`、ready=true、relay=false；原成员真实签名 `GET /v1/devices` 返回200及原两个成员。独立未入组身份实际签名创建一条分享，返回200/offered；没有插入永久成员。
3. 708正常结束后核对唯一 `pre-share-v5-<nonce>.sqlite`：mode0600、schema4、quick_check=ok、foreign_key_check为空，四张永久表数量/规范摘要均等于原schema4。工作库schema5、quick_check=ok、原表全部相同，新增一条真实分享。
4. 准确36以明确关闭分享配置打开该同一schema5工作库，普通health及原成员签名GET仍200/两个成员；`POST /v1/shares/status` 明确426/`SHARE_UNSUPPORTED`，没有share capability/control。退出后schema5、永久表与新增分享行的完整规范摘要均未变化；没有回灌schema4或覆盖新数据，也没有第二份迁移备份。

三个临时服务均 `SIGTERM → exit0`、forced=false，三个loopback listener均已关闭。自有合成DB、自动备份与日志留在WSL `/tmp/linksend-r5-migration-7084685-20261026-01`，没有用户文件或生产对象；不清理它们冒充回退。完整读回与各进程结果在 `.artifacts/r5-server-7084685/MIGRATION-FALLBACK-RECEIPT.json`。

回退使用已有准确36文件 `C:/Users/Wen/.codex/supervision/ordinary36-20261026-01/rendezvous-linux`，实际SHA256 `c1d0db7f90bae90e299d6be40983891823e4001b73c14667d9b48dfb298ac20a`。它只作为本次已实测schema5兼容候选；不代表原0.5.0/schema4-only服务可直接回退，也不证明任意生产DB状态均兼容。生产live重核、一致性副本、最终备份、准确成功CI、TLS/公网与授权仍见[未部署草案](RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)。
