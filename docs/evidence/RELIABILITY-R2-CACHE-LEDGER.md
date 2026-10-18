# R2：缓存持久所有权首批（2026-10-19）

基线 `bed8d2c651c9f8c7512b3884a7c096b2efaa387c`。本批实现私有持久记录和引擎句柄来源，尚不实现自动扫描/删除已发布对象，不代表R2完整完成。

## 行为

- 复用现有profile锁与`desktopStore`共享的SQLite连接，在既有metadata表的`clipboard-cache/v1/<id>`存储；无新数据库、schema迁移或文件wire变化，旧程序忽略该命名空间。
- 记录绑定设备身份、用途、随机对象ID、cache/object目录文件身份、运行代次、peer、创建时间、revision及状态；实际manifest/摘要、接收计划摘要、stage/part身份在accept/body前持久化。标准Ed25519签名、严格版本/未知字段/大小/尾随数据校验，CAS更新并回读；损坏/更新冲突不覆盖或静默重新认领。
- 新增Go-only StorageReady：从引擎实际持有的Root/Stage/File句柄提供来源，在首次checkpoint和后续plan变更均调用，错误沿既有RECEIVE_PLAN_PERSIST_FAILED终止，正文尚未发送。part名字对应的inode必须与引擎打开的文件相同，不能用可被替换的绝对路径认领用户文件。
- 验证完成后persist verified，原生发布前先persist publishing，再重验授权、取消、generation、deadline和可见路径；DB I/O不放入权限/剪贴板状态锁。实际原生调用后，成功记published，结果不确定记protected；最终状态落盘失败也保留原先publishing意图和已验证文件。
- 当前worker未尝试原生写入时，确认自身对象已移除后才忘记记录；清理失败记abandoned。跨运行代次只能读取旧记录，不能重启旧复制或把未知的native-start状态当作false。本轮仍没有发布对象的回收许可。

## 实际验证与失败

- 初次StorageReady接线只进入后续plan更新，漏了构造器的首次checkpoint，导致安全门拒绝verified而未发布；失败保留`r2-ledger-handles.log`等首轮日志。修复为共用notifyPlanReady后，首次和后续路径均纳入同一前置门，不放宽所有权或等待期限。
- 真实认证QUIC夹具验证：进入原生adapter时DB已经有publishing及完整manifest/stage/part来源；完成后新revision的published可读；未知原生结果持久为protected；磁盘拒绝的未发布残留及其记录均清理。
- 安全负例：签名改动、未来记录版本、尾随JSON、重复认领、旧CAS覆盖/删除均拒绝；把另一个用户文件句柄伪装为stage part时拒绝且该文件内容仍可读。SQLite trigger注入publishing写失败时，OS不收到路径，未发布对象和误导记录均不遗留。
- 重启测试关闭旧Service后重新New：正常profile使用实际DPAPI/受限identity文件重新读取密钥，旧签名记录及目录身份保持；旧publishing快照不能重新开始原生写入或因RAM标记丢失被忘记；无clipboard job重建或旧复制重放。
- 文件引擎门测试通过（3轮，`r2-ledger-transfer.log`，3.920秒）：storage观察器拒绝时，接收错误保留，发送方收到既有持久化错误，实际ChunkSent=0，正式目标文件不存在。net.Pipe只证明引擎时序，不充当P2P网络证据。
- 最终定向普通3轮退出0（16.306秒，`r2-ledger-final-regression.log`）；app组合race3轮退出0（29.358秒，`r2-ledger-app-race.log`），文件引擎门/Root race3轮退出0（6.851秒，`r2-ledger-transfer-race.log`）。
- 两模块workspace/off的`go test ./...`、`go vet ./...`、`go build ./...`共12条命令全部退出0，日志`r2-ledger-{core|desktop}-{workspace|off}-{test|vet|build}.log`。核心workspace app96.007秒、transfer11.827秒。前端未改，复用bed8d2c的127项/typecheck/lint/build及准确CI；新代码CI推送后另核对，不预记PASS。

日志位于`.artifacts/reliability-20261019/`。源码测试使用专属临时profile/文件和内存native adapter，无日常剪贴板、安装、生产或宿主网络改动。

## 下一步

将已认证记录接入同owner活动引用、启动对账、有界安全回收与退避；先取得真实原生交接/读取结束证明，未知发布对象继续保护。不得把boot/TTL当作许可；不得把本批“重启读到元数据”写成系统剪贴板路径重启后的实际粘贴通过。R1跨网、R2真实键盘粘贴及R3–R6仍按唯一TODO必做。
