# R5 准确 Linux 构建与合成库兼容回退

日期：2026-10-26。范围：准确 `708468579c2d969f9e028d52c06eee4defb43164` standalone rendezvous 的一次最小 Linux 功能验证，以及850/当前 `5ee9a40e6741749a4bc2d098c85a58e2cc9bd785` 的准确重建和真实rendezvous依赖输入核对。**本次构建/合成库验证没有生产访问/部署、SSH、外网请求、GUI、系统剪贴板或文件数据面重测；R5不因此整体完成。** 708测试失败和850Windows包失败保留；准确5ee core/desktop与Windows包job已首次成功，静态验包仍进行，不冒充整体packages成功或生产部署证据。

## 准确构建

从本机仓库建立独立 detached checkout `C:/Users/Wen/.codex/supervision/r5-server-7084685-20261026-01/source`；构建前后 `git status --porcelain --untracked-files=normal` 均为空，HEAD 为完整708。以已有 Go1.27.1 执行一次：

```text
GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0
go build -trimpath -o <outside-source>/rendezvous-linux-amd64 ./cmd/rendezvous
go version -m <outside-source>/rendezvous-linux-amd64
```

构建 exit0，18,188,312B、SHA256 `4ec005a23c5da98ae77367924e1b1c41efbec34f58ddec396b86835bc09c4de6`。真实 buildinfo 含 `vcs.revision=708468579c2d969f9e028d52c06eee4defb43164`、`vcs.modified=false`、linux/amd64、CGO0、trimpath。Linux 实际 `-version` 返回 `LinkSend rendezvous 0.6.2`。二进制在该监督目录；收据 `.artifacts/r5-server-7084685/BUILD-RECEIPT.json`、完整 `GO-BUILD-INFO.txt`、`build.log`。

后续850只改两个 `*_test.go` 和三份docs，没有产品输入变化。用完整 `git ls-tree -rz` 的mode/path/blob列表比较，排除docs/和`*_test.go`后所有tracked文件完全相同，SHA256 `d6890c344e21f3ef984f409f77264f8f7e69944fa3e8016f361055dd5063edfd`。同一独立checkout干净切至准确850，以同工具链/参数构建到源外新文件 `rendezvous-linux-amd64-850b9d1`：exit0、18,188,312B、SHA256 `6efa08b5919113281d1fcc8c81f7b63316fc0492df33de9dfe930c273252a784`，真实buildinfo完整850/modified=false，构建前后clean；实际Linux `-version` exit0/0.6.2。新构建收据 `BUILD-RECEIPT-850b9d1.json`、`GO-BUILD-INFO-850b9d1.txt` 均位于同artifact目录。复用下面708旅程，没有重跑850服务、迁移或矩阵；原708 PID/结果仍绑定708。

## 当前5ee准确候选与实际依赖复用

5ee包含后续R2发送/缓存guard和生成TS修正，不能直接用「只改测试」或模块名称推测server不受影响。因此在同一独立clean checkout分别切到准确708、850、5ee，以 `GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go list -deps -json ./cmd/rendezvous` 取得三次真实依赖图，逐个解析repo目录实际选中的Go/C/汇编/系统对象/embed等生产输入并加上go.mod/go.sum。三版实际repo生产依赖均为cmd/rendezvous、identity、protocol、server、sharegrant、store六包，16个输入文件、本目标无embed。输入列表、git mode/path/blob、实际字节SHA以及外部模块版本/go.sum全部相同；完整规范输入SHA256 `c0d6665d5f8a6d60208ef4e117ddeb835a5dba188370fa942a1644136e79e2bb`。全量JSON与每文件详情在 `.artifacts/r5-server-7084685/RENDEZVOUS-DEPENDENCY-EQUIVALENCE-5ee9a40.json` 及三份 `go-list-<sha>.json`。

准确5ee以同Go1.27.1/linux-amd64/CGO0/trimpath构建一次，构建前后clean：18,188,312B、SHA256 `92e3ce635246925e0492d3e092f3e6fe9615cf7fd9d8213cf89dfc16dc927a59`，真实buildinfo完整5ee/modified=false；已有WSL执行该新二进制 `-version` exit0/0.6.2。文件为 `C:/Users/Wen/.codex/supervision/r5-server-7084685-20261026-01/rendezvous-linux-amd64-5ee9a40`，独立构建收据 `BUILD-RECEIPT-5ee9a40.json`、`GO-BUILD-INFO-5ee9a40.txt` 同存上述artifact目录。依据实际相同输入复用708迁移/回退，未重复5ee服务、迁移或完整矩阵；下述实际服务PID与数据库结果继续明确归于708。

准确5ee CI已读回 `.artifacts/r2-helper-ci-5ee9a40/ci-runs.json` 和 `windows-job.json`：push core `40273165842`/desktop `40273165901` 与PR core `40273168788`/desktop `40273168780` 均attempt1/success；Windows job `121410208098`（run `40273165815`/attempt1）2026-10-26T05:24:16Z首次success，含构建/hash/上传成功。该观测整体packages仍in_progress，独立Windows静态验包仍进行；没有等待Mac或把Windows job成功冒充整体结果。生产只读刷新由root另行记录在[草案](RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)，本子任务未SSH或变更生产。

## 一次合成库旅程

使用本机已有 WSL Ubuntu（实际 Linux 6.6.87.2/amd64），不安装运行时、不修改 `.wslconfig` 或网络。执行一次 `wsl -d Ubuntu --exec python3 /mnt/d/apps/Osend/.artifacts/r5-server-7084685/migration-smoke.py`，exit0。该脚本独立记录hash，逐个启动准确36与708的真实Linux二进制，服务仅绑定动态 `127.0.0.1`，不启动TLS或公网入口。

1. 准确36以关闭分享配置创建自有schema4，正常退出后仅写入合成的 groups/devices/invitations/membership_requests（1/2/1/1条）。两个设备公钥由本次内存Ed25519身份产生，未读取日常身份/凭据。用SQLite Backup API取得独立工作副本；原schema4保留不动。
2. 708显式开启分享，从该工作副本实际执行4→5。health声明 `share_grants_v1=true`、ready=true、relay=false；原成员真实签名 `GET /v1/devices` 返回200及原两个成员。独立未入组身份实际签名创建一条分享，返回200/offered；没有插入永久成员。
3. 708正常结束后核对唯一 `pre-share-v5-<nonce>.sqlite`：mode0600、schema4、quick_check=ok、foreign_key_check为空，四张永久表数量/规范摘要均等于原schema4。工作库schema5、quick_check=ok、原表全部相同，新增一条真实分享。
4. 准确36以明确关闭分享配置打开该同一schema5工作库，普通health及原成员签名GET仍200/两个成员；`POST /v1/shares/status` 明确426/`SHARE_UNSUPPORTED`，没有share capability/control。退出后schema5、永久表与新增分享行的完整规范摘要均未变化；没有回灌schema4或覆盖新数据，也没有第二份迁移备份。

三个临时服务均 `SIGTERM → exit0`、forced=false，三个loopback listener均已关闭。自有合成DB、自动备份与日志留在WSL `/tmp/linksend-r5-migration-7084685-20261026-01`，没有用户文件或生产对象；不清理它们冒充回退。完整读回与各进程结果在 `.artifacts/r5-server-7084685/MIGRATION-FALLBACK-RECEIPT.json`。

回退使用已有准确36文件 `C:/Users/Wen/.codex/supervision/ordinary36-20261026-01/rendezvous-linux`，实际SHA256 `c1d0db7f90bae90e299d6be40983891823e4001b73c14667d9b48dfb298ac20a`。它只作为本次已实测schema5兼容候选；不代表原0.5.0/schema4-only服务可直接回退，也不证明任意生产DB状态均兼容。生产live重核、一致性副本、最终备份、准确成功CI、TLS/公网与授权仍见[未部署草案](RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)。
