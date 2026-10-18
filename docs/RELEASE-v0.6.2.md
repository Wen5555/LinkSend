# LinkSend v0.6.2

发布日期：2026-10-19。产品版本0.6.2，文件协议V1、relay=false。已正式发布并设为Latest，准确源码、CI、包版本及SHA256见下方和随包清单。

## 本次更新

- 自动剪贴板：修复同一局域网内纯剪贴板连接未启动通道、连接被额外10秒期限截断及旧“连接失败”状态残留。
- 复制文件：新增“同步复制的文件”独立开关；在两端开启后，支持Windows文件条目与Mac文件URL互传，校验完成后可在另一端粘贴。文件正文继续走认证QUIC与既有V1文件引擎。
- 大文件进度：大于等于32MiB时居中显示“正在复制”、准备状态或实际校验进度，可取消；少量文字、图片和小文件不弹窗。新复制、暂停或撤权会淘汰旧文件复制，取消后删除未完成缓存。
- 软件更新：设置新增手动检查GitHub正式版、下载进度/取消、SHA256校验及原生安装包入口；更新检查不依赖信令连接。关于页显示实际后端版本。

## 升级与使用

0.6.1及更早版本首次升级请从本次Release下载Windows安装器或Mac DMG，退出含托盘/菜单栏的旧程序后安装。0.6.2开始可在“设置 → 软件更新”使用检查和下载入口，安装仍由系统向导或DMG完成。

两端需分别开启自动剪贴板总开关及对应设备权限。复制文件还需单独开启“同步复制的文件”；原有文字/图片权限不自动授予文件权限，旧端需同时升级。暂停、离线和应用重启不会补发旧复制。源文件不会删除，也不会传播剪切操作。

任务库schema11→12迁移前创建并验证备份，保留旧授权行，不自动新增启用files授权。旧程序拒绝新schema；需要回退时使用迁移前备份。已完成的剪贴板文件暂存于profile/clipboard-files，供后续粘贴，目前不会自动清理。

## 验证与边界

候选通过两Go模块workspace/off测试、vet、build、相关race、前端125项/typecheck/lint/build及440像素布局检查。实际Windows↔Mac同一LAN系统剪贴板的文字、不同像素图片和文件条目双向验证通过；64MiB文件超过原lease期限后仍完成并校验哈希，两端出现真实进度，取消测试确认未完成缓存删除且剪贴板原文件路径保持。

上述物理结果属于记录中的本地候选，不移记为后续GitHub发行包已运行。其后文件完成状态与同时LAN拨号的会话冲突修复由确定性自动回归/race覆盖，未重新运行物理测试。Windows包未代码签名；Mac为ad-hoc签名、未公证，macOS13及Intel原生验收、Explorer/Finder键盘粘贴与双NAT缺项保留。未部署生产信令，也未改防火墙、路由、代理或宿主权限。

实现、命令、准确候选hash及测试校验器的颜色空间误判纠正见[剪贴板验证记录](evidence/DESKTOP-CLIPBOARD-LAN-FIX.md)、[软件更新验证](evidence/DESKTOP-GITHUB-UPDATES.md)、[ADR0015](adr/0015-file-clipboard-progress.md)。

## 发布来源与资产

- [Release](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.2)已公开，Latest，非Draft、非prerelease；tag精确指向`63c86c2c5ce183e1400d27ca0571610d50a42bdc`，PR [#11](https://github.com/Wen5555/LinkSend/pull/11)正常合并main。
- [三平台包CI](https://github.com/Wen5555/LinkSend/actions/runs/39157767929)与准确源码core/desktop检查全部success，六项发布资产的GitHub digest与本地一致。
- 两Mac DMG只读挂载检查主app/Share Extension版本、架构、最低OS元数据和严格签名通过；挂载已卸载，未启动app。

| 平台 | 资产 | SHA256 |
|---|---|---|
| windows-amd64 | [LinkSend-v0.6.2-windows-amd64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.zip](https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/LinkSend-v0.6.2-windows-amd64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.zip) | `37b02d23963e0538450ec4686b8740b5e6cf283a0949e42fbcbafac88a968e0c` |
| windows-amd64 | [LinkSend-v0.6.2-windows-amd64-installer-63c86c2c5ce183e1400d27ca0571610d50a42bdc.exe](https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/LinkSend-v0.6.2-windows-amd64-installer-63c86c2c5ce183e1400d27ca0571610d50a42bdc.exe) | `d4e5d364d33c5b66fe57331fe588a9772d46874f5d88fd22e5b510040fdd2651` |
| macos-arm64 | [LinkSend-v0.6.2-macos-arm64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.dmg](https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/LinkSend-v0.6.2-macos-arm64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.dmg) | `0c6113756eca43d25bacdf3951f0000d68fe9c295ce0c1535da4f19bccedbb40` |
| macos-amd64 | [LinkSend-v0.6.2-macos-amd64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.dmg](https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/LinkSend-v0.6.2-macos-amd64-63c86c2c5ce183e1400d27ca0571610d50a42bdc.dmg) | `193c661e76aa85d6338722b7b4aa2da15496de7279c0b5a76f519cf38eda9872` |
