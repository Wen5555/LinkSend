# Desktop M0 香港部署预检与回滚准备

本记录对应六项桌面功能的 M0。目标产品版本为 `0.5.0`，计划测试预发布标签为 `v0.5.0-m0`；该标签不表示 M1–M6 或六项原生验收完成。当前状态是 **PRECHECK PASS / TRANSACTION PREPARED / DEPLOYMENT NOT_RUN / RELEASE NOT_RUN**。本记录生成时，尚未拿到目标提交对应的已验证 Linux 二进制，未替换或重启远端服务。

## 2026-09-12 实际只读预检

经 `codex-ssh-manager` 对唯一 alias `hk-main` 执行 resolve → probe → audit-host，再运行有界只读脚本。连接用户为 `root`，主机 `109.66.88.204`，端口由 manager inventory 管理。未读取或输出完整配置、配对码、令牌、私钥或成员内容。

| 检查 | 实际结果 |
| --- | --- |
| resolve / probe / audit-host | PASS，三个命令退出码均为 0 |
| 预检时间 | `2026-09-12T04:36:19.165396+00:00`，北京时间 12:36:19 |
| 操作系统 | Debian，Linux `6.1.0-50-cloud-amd64`，x86_64 |
| systemd unit | `linksend-rendezvous.service`，`active/running`，`ExecMainStatus=0` |
| unit 文件 | `/etc/systemd/system/linksend-rendezvous.service` |
| MainPID | `394992`，`/proc/<pid>/exe` 与运行二进制路径一致 |
| 运行二进制 | `/opt/linksend-lan-test/rendezvous`，ELF amd64，16,655,126 bytes |
| 当前产品 | `LinkSend rendezvous 0.4.0` |
| 当前源码 | `7303f34260c09b8ad71c117ee2d6837fac4dfe46`，`vcs.modified=false` |
| 当前 Go | 二进制内构建元数据 `go1.26.5`；远端 PATH 未安装 Go 命令 |
| 当前二进制 SHA256 | `227e2ec9fb029fcc8a568637376318e4eae4099f1cc2ce79bc7224557f07fa45` |
| 配置 SHA256 | `20a940907782d0baec8745e9e3fe45bed68053568f0b3f6d325eeffdd77299c0` |
| 数据库版本 | **`schema_version` 表为 `[2]`；`PRAGMA user_version` 为 `0`** |
| 数据库完整性 | `PRAGMA integrity_check=ok`，使用 `mode=ro` 与 `query_only` |
| 邀请表 | 19 条；已存在 `used_by/used_at` 字段，仅查询计数和列结构 |
| origin 与公网 health | 均 `status=ok`、`version=0.4.0`、`product_version=0.4.0`、协议 V1、QUIC、`relay=false` |
| 监听 | TCP 443 属于 MainPID；UDP 3478 存在 |
| 续期 timer | `linksend-origin-renew.timer` 为 active |
| 最近 30 分钟致命日志 | 匹配 panic/fatal 的记录数为 0；未输出原始日志 |
| 系统状态 | 失败 systemd units=0；无需重启；根文件系统使用约 22%，可用 63,413,010,432 bytes |
| 所需远端工具 | python3、curl、systemctl、sha256sum、flock、install 均可用 |

这里的数据库 schema 与桌面 `task-history.sqlite`、本地 `trust.json` schema 是三个独立版本。不能把服务端的 `PRAGMA user_version=0` 当作缺失 schema 2，也不能在 M0 部署时顺带迁移生产库。

远端证据保留于：

```text
/tmp/codex-ssh/linksend-desktop-m0-preflight-20260912T043606Z
```

本地预检脚本：`.artifacts/desktop-six-features/inspect-hk-m0.sh`。manager `run-script` 返回 `ok=true`、`exitCode=0`、空 stderr。manager 为运行脚本创建了自己的 `/tmp/codex-ssh/...` 作业文件；应用二进制、配置、数据库及服务状态均未变更。

## 已准备的事务

本地材料位于 `.artifacts/desktop-six-features/`：

- `deploy-m0.ps1`：本地核对 Linux amd64、完整 commit、`vcs.modified=false`、SHA256，默认只生成可审查脚本；只有显式 `-Execute` 才通过 manager 上传候选并执行。
- `deploy-m0.remote.sh.in`：固定作用域的 systemd 事务模板，部署锁防止本事务重复并发；不使用旧 PID 文件或 nohup 启动模式。
- `validate-m0-transaction.py`：离线语法及控制流程验证，不导入模板，不执行远端 I/O。

事务严格保持服务 schema 2、协议 V1、`relay=false`，只替换应用二进制。步骤为：

1. 核对当前旧 SHA、活动 unit、MainPID 对应 executable、候选 ELF/commit/modified/version/SHA、数据库结构/完整性、origin 与公网 health 和备份空间。基线变化就停止并重新预检。
2. 在 `/opt/linksend-lan-test/backups/<UTC>-desktop-m0-<commit>/` 建立 0700 目录，备份旧二进制、0600 配置、unit 和 SQLite 在线备份；验证内容哈希、数据库完整性及 schema，并写入不含秘密的 manifest。打印确切备份路径后才停止旧服务。
3. 停止既有 unit，同目录暂存、fsync、原子替换候选，再启动既有 unit。
4. 验证新 MainPID/executable、二进制 SHA、未变配置/unit、数据库 schema/integrity、TCP 443 归属、origin 和公网的版本/能力、此次启动后的 fatal/panic。
5. 任何变更或验证失败自动停止新服务、从同一备份恢复旧二进制、启动并复核。**正常回滚保留数据库中的部署后成员/邀请写入**，不盲目覆盖 WAL/SHM 或回退数据库。
6. 若发现意外 schema 不兼容，保持服务停止，报告 `STOPPED_SCHEMA_REVIEW_REQUIRED` 及在线备份位置；先核实新写入边界，再决定恢复方案。此情况不能宣称自动回滚成功。

部署成功记录只证明版本替换与服务检查。独立配对、WSS 生命周期和客户端混合版本行为仍需另行运行，脚本会明确输出 `NOT_RUN_REQUIRES_INDEPENDENT_ACCEPTANCE`。

## 精确提交就绪后的执行方式

下列参数必须替换为主线程提供的已提交源码构建资产；不是可直接运行的已完成部署记录。

```powershell
. .tools/use-desktop-toolchain.ps1
$candidatePath = '<Linux amd64 二进制绝对路径>'
$sourceCommit = '<40位已提交commit>'
$candidateSha256 = '<64位实测SHA256>'

# 先生成并审阅，无远端改动。
& .artifacts/desktop-six-features/deploy-m0.ps1 `
  -BinaryPath $candidatePath -Commit $sourceCommit -Sha256 $candidateSha256

# 审阅后执行用户已授权的里程碑部署。
& .artifacts/desktop-six-features/deploy-m0.ps1 `
  -BinaryPath $candidatePath -Commit $sourceCommit -Sha256 $candidateSha256 -Execute
```

执行器在独立本地证据目录保存精确参数、Go build info 和各个 manager 返回值。保留输出中的 `REMOTE_JOB` 与 `BACKUP_PATH`；断线先通过 manager `tail-job/resume-job` 读取已有作业，不直接重复部署。

同一二进制已部署时返回 `ALREADY_CURRENT`。执行器不会上传覆盖现有远端候选，也不会修改防火墙、路由、DNS、代理、证书、续期脚本、coturn 或其他服务。

## 部署后手工发起同一事务回滚

若独立行为验收发现问题，可使用部署时保留的同一候选路径、commit 和 SHA，指定输出的确切备份目录。执行器不会再次上传候选：

```powershell
& .artifacts/desktop-six-features/deploy-m0.ps1 `
  -BinaryPath $candidatePath -Commit $sourceCommit -Sha256 $candidateSha256 `
  -RollbackBackup '/opt/linksend-lan-test/backups/<实际UTC>-desktop-m0-<实际commit>' `
  -Execute
```

回滚首先核对 backup manifest、备份二进制 SHA、目标提交和当前运行哈希，拒绝覆盖部署后被其他事务更换的版本。旧版本已恢复时只做验证并返回 `ALREADY_RESTORED`。配置没有在本事务内修改，因此不会用旧配置覆盖之后的独立变更；配置发生变化会要求重新审阅。

## 本次材料验证与限制

- PowerShell AST 语法解析：PASS。
- Python 远端模板语法编译：PASS。
- 离线事务控制检查：成功、相同版本、备份失败不变更、验证失败仅回滚一次、schema 不兼容停止且不替换二进制/数据库，共 5 项 PASS。
- 首次尝试调用 PATH 中的 `python` 实际命中 WindowsApps 占位程序，退出 1、没有完成验证；随后改用 Codex bundled Python，验证命令退出 0。上述 PASS 均来自后一次真实运行。
- 未在真实远端执行部署脚本，未制造线上故障测试自动回滚；离线控制检查不能替代 systemd/SQLite/网络现场验收。
- 此刻没有 M0 新二进制哈希、新 PID、新备份路径、发布资产或 Release URL，故全部保留 NOT_RUN；待精确 commit 和资产就绪后在本页追加实际事务与独立复核结果。
