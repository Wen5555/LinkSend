# R2 文件新复制与续期交错（2026-10-26）

## 实际失败

准确eccb903和a22f691的GitHub core race步骤均在TestClipboardFilesInsufficientSpacePreservesExistingFiles失败：原disk_full之后新复制没有恢复，20秒后双方仍只有上一条file_copy_disk_full记录。没有DATA RACE报告，不能将其当作竞态检查器告警或未证实的偶发测试。原始日志ci-{eccb903,a22f691}-core-failed.log保留于D:/apps/Osend/.artifacts/crossnet-20261026。

因果缺陷：旧失败的异步取消通知会续期同权限lease。若新复制已经ObserveLocalEvent推进OS generation，而原生文件路径尚在读取，新lease会以这个新generation作为baseline。原实现只取最新lease，随后BindScoped拒绝baseline不早于复制的绑定，文件路径静默continue，因此根本没有第二个job；等待更久或释放更多磁盘不能修复。

新增真实认证QUIC夹具与ReadFiles屏障，强制续期恰在读取期间到达。旧实现0.29s稳定FAIL（新复制无job），日志clipboard-renewal-before.log、exit1保留，不靠增加deadline或跳过用例消除失败。

## 修复与安全边界

State新增OutboundForObservation，仅为当前本机origin/boot/seq、revision及OS generation的观察选取baseline更早且仍有效的同peer/session/generation/kind重叠lease。文件复制在读取前与准备后均用该查询，最终BindScoped仍重验owner、期限与权限；不能让无旧有效lease的复制借新lease补发。

同权限续期不再吞掉已经观察的这一次新复制。权限变化/撤权、暂停、新复制、过期、错误peer/session/generation仍拒绝；旧复制不因重连或续期恢复发送。没有修改缓存容量、空间预留、回收/删除实现、期限、文件引擎、wire/schema或用户文件保护，没有增加任何缓存设置。

## 验证

- 新屏障回归、原CI失败用例及状态负例普通10轮通过：clipboardsync0.796s、app10.514s，exit0。
- 相关clipboard文件/缓存/reservation/state回归race3轮通过：clipboardsync1.568s、app75.648s、命令84256ms，exit0。状态负例覆盖到期、权限变化、新复制、暂停、无旧lease、不同peer/session/generation。
- 最后双模块workspace/off分别test -count=1 ./...、vet ./...、build ./...共12命令全部exit0，core106644/108627ms、desktop7759/7721ms。clipboard-renewal-final-*.log包含退出码和实际耗时，覆盖最终签名客户端case folding修复，未用早期矩阵替代。准确提交CI仍须核对，不把旧CI失败改为通过。

日志根D:/apps/Osend/.artifacts/crossnet-20261026：clipboard-renewal-{before,after-test,after-race}.log与clipboard-renewal-final-*.log。定向真实QUIC/原生adapter夹具不替代Windows↔Mac物理键盘粘贴、准确候选或完整自动回收验收，R2和整体Goal保持未完成。
