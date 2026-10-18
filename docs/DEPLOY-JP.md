# 日本猫猫机 LinkSend 信令部署

本任务目标主机 `85.149.217.165`（SSH alias `jp-linksend`）；精确入口 `linksend.oooai.de`。原控制库已丢失，使用新持久库；不从客户端缓存恢复旧组或管理员身份。当前部署状态和实际收据统一见 [TODO](DESKTOP-EXPERIENCE-TODO.md)，本页描述可审查的操作契约。

## 源码与 TLS

`deploy/systemd/install-fresh-jp.sh BINARY SOURCE_COMMIT BINARY_SHA256` 只接受首次安装；任何已有目录、单元、用户名或端口冲突均退出，须先检查原 job，不能直接重跑未知结果。

服务器二进制从已验证提交的 `git archive` 导出到 **Git 仓库外** 构建，记录提交、归档 SHA256、Go 工具链和二进制 SHA256。Git 子目录内的导出仍可能被 Go 标记为父仓库 dirty，不能据此声称准确来源。当前服务源码候选为 `d5412642e49460413fdda249f77d03647075f41c`，Go1.27.1、linux/amd64、CGO_ENABLED=0；对应源码测试及 CI 已通过。

应用只监听 `127.0.0.1:8787`，使用专属本地 CA 签发的服务证书；Nginx 必须校验该 CA 与 `linksend-backend.internal` 主机名。外层复用已核验的 `*.oooai.de` Cloudflare Origin 证书，精确站点不覆盖既有 wildcard 站点。严格校验证书的源站探针成功后才切换该单一 DNS 记录，并通过公共 HTTPS/WSS 做业务验证。Origin CA 只用于明确的源站验证，不冒充系统公共 CA。

配置保持 `allow_insecure_loopback=false`、`allow_loopback_candidates=false`，不启用固定配对码、TURN 或文件中继。服务 `relay=false`，正文不进入信令。密钥、bootstrap 环境文件、数据库和备份均不进 Git，也不输出正文。

## 备份、保留与回退

安装器先保存当前 Nginx 主配置及 wildcard 配置到主机上的 `0700` 部署备份目录，回读/校验；新增站点通过 `nginx -t` 才 reload。完成后再次比较原配置原字节，防止误改其他站点。

服务为专属无登录用户 `linksend`，systemd 管理；私有持久目录 `/var/lib/linksend`。日志使用独立 journal namespace，持久日志32MiB、临时日志8MiB、保留7天并限频；该站点禁用 access log，独立 error log 配置 logrotate。未改全局 journal、安全策略、宿主防火墙、路由或代理。

`/opt/linksend/tools/linksend-backup.py` 使用 SQLite 在线 backup API，不复制活跃 DB/WAL 原文件。完成副本转为DELETE journal，并以immutable方式只读校验，避免验证生成root sidecar影响非特权恢复；活跃源的WAL模式保持。每份先做 integrity_check / foreign_key_check，再发布 checksum 收据；每日 timer 保留14份校验有效的本工具备份。未完成、损坏、符号链接或非本工具命名文件不参与自动清理。单机备份能用于隔离恢复，不能称异机灾备。

```sh
python3 /opt/linksend/tools/linksend-backup.py backup \
  --database /var/lib/linksend/control.db \
  --directory /var/backups/linksend/database --keep 14
python3 /opt/linksend/tools/linksend-backup.py restore \
  --backup /var/backups/linksend/database/<已校验备份>.db \
  --destination /var/backups/linksend/<新隔离目录>/control.db
```

restore 拒绝覆盖任何已有目标。恢复演练必须用新隔离目录及独立进程/监听，不替换运行中的控制库。部署失败时安装器停本项目服务/定时器、只移除自己新增且 target 一致的站点 symlink，验证并重载原 Nginx；配置、二进制、日志和新库均保留作恢复证据。已发生新写入后，后续回退应保留新库、回退兼容二进制，不能以空库或早期备份覆盖业务。

每次改动均保存 manager jobPath/requestId。重连先使用 job_status/read_job；观测超时不是任务停止，不重复启动部署。

## 独立 STUN-only

`deploy/coturn/install-stun-only-jp.sh LOCAL_IPV4 PROBE PROBE_SHA256 SOURCE_COMMIT` 只接受首次安装。先精确安装发行版 coturn 并保持标准 `coturn.service` masked/inactive，再以独立 `linksend-stun.service` / `turnserver` 用户启动；不把该用户加入能读取信令 TLS 私钥的 linksend 组。

配置绑定单个本机公共 IPv4 的 UDP3478，使用成熟 coturn 的 `stun-only`，关闭 TCP/TLS/DTLS/CLI/RFC5780。它不提供 TURN、文件代理或信令功能。复用项目有界 journal namespace，不改全局日志、宿主防火墙、路由或代理。

安装前核对端口/用户/目标不存在、probe SHA256及来源提交，保存发行版版本和全部部署资产到 `/var/backups/linksend/stun-<UTC时间>` 并回读校验。安装后检查 unit、唯一 UDP 监听、实际 STUN Binding 与 TURN Allocate 负例；失败停止并禁用本任务 unit，保留配置和备份供检查，不删除已有数据。成功后仍须在外部机器验证 UDP，再切 `stun.oooai.de` 精确 A（DNS-only），保留原记录用于回退。

`scripts/stunprobe` 复用生产 `connectivity.Endpoint` / Pion 收集候选，保持共享 ICE/QUIC UDP 所有权；只输出候选和计数元数据。另一个控制 socket 先验证正确 transaction/fingerprint 的 Binding 成功，再检查有效 Allocate 无成功响应；无响应只表示本次2秒窗口没有分配证据。构建须注入准确 `main.sourceCommit`，禁止用该探针冒充双 NAT 文件传输通过。

```sh
stunprobe -bind <本机真实IPv4> -stun stun:stun.oooai.de:3478 -timeout 10s
```

`-require-srflx=false` 仅用于服务本机的同地址自检；外部验收保留默认 true。自动测试覆盖 TURN 成功分配误报、Binding失败、错误transaction、损坏fingerprint和取消，不用本机fixture替代外部UDP结果。
