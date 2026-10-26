# 准确5ee9a40候选与最小验证

日期：2026-10-26。源码 `5ee9a40e6741749a4bc2d098c85a58e2cc9bd785`；候选版本仍0.6.2，以完整提交和BUILD-INFO区分。代码已推送工作分支与[草稿PR #13](https://github.com/Wen5555/LinkSend/pull/13)，没有合并main或发布Release/tag。

该源包含跨网修复、显式临时分享能力与入口门、生成绑定修正、可信Windows helper及普通私有缓存出口拒绝。尚未启用的Shell primitives后续工作不在该源内，Native自动删除仍关闭；整体R0–R6未完成。

## CI与Windows产物

push core40273165842、desktop40273165901，PR core40273168788、desktop40273168780及packages40273165815均attempt1/success；Windows job121410208098与两Mac包job均首次success。Mac仅核CI状态，没有下载或新增实机验证。708旧取消测试失败、850旧单绑定dirty失败继续保留。

Windows资产只下载一次，artifact12629720669，93,438,591B；Actions API、上传日志及下载SHA256一致：`2e7e27e74c5fd240fe351dbb488aa328ccf53c33d6c6f59253161f9c95665b45`。内外BUILD-INFO完整head/run/clean来源、主PE可信锚、helper实际PE/Go摘要、精确ZIP白名单和外层SUMS均通过；安装器只提取main/helper，与ZIP载荷逐字节hash相同。没有执行候选、安装器或系统clipboard。

| 载荷 | 字节 | SHA256 |
| --- | ---: | --- |
| Windows ZIP | 14,474,060 | `e3d0b62199fa14e9ef4e385832a5955aef5ca0d79413d16bc80dc0d53736fcaa` |
| Windows installer | 79,637,189 | `be7d5ae8502c757c151f0cf3761105df9bd6a48843a06bd6ee02861967d0af61` |
| LinkSend.exe | 25,117,696 | `b29bb269383873b98aeccf8254df94f9ddda8194a27741f52d0dd70b91270b53` |
| clipboard helper | 11,988,850 | `fa87f79928d9840caf59e031e5c696b11eefb7bce6c2a651b27ad975bdde102a` |

下载文件与实际静态checker输出路径、精确CI/asset元数据及机器断言见 `.artifacts/r2-helper-ci-5ee9a40/FINAL-RECEIPT.json`、`FINAL-ASSERTIONS.json`、`static-59acc19bf40f4a5aa8a221c63e3e08f7/verification.json`。source digest为 `12e1175cc090349c48826ec5d62d87f0622f01c8ede8186f6458f269373179a9`，source_clean=true；收据SHA `3279586a113d8a6ec7cbea6f27b890ea17732205ce29545d8ebed44be26c3f17`。

## Linux候选与复用范围

准确clean Linux rendezvous一次构建18,188,312B，SHA256 `92e3ce635246925e0492d3e092f3e6fe9615cf7fd9d8213cf89dfc16dc927a59`；真实buildinfo完整5ee/modified=false，实际Linux `-version` exit0/0.6.2。实际go list解析的6个repo生产包16文件与708/850输入逐项相同，复用708一次合成非空schema4→5及准确36兼容回退旅程，未重跑服务。原PID和结果仍归准确708，不改称5ee运行，见[迁移/回退证据](RELIABILITY-R5-SCHEMA5-FALLBACK-20261026.md)。

36普通实网双向/原任务恢复与36 Wails从Linux实收继续作为各自源的历史证据。本候选尚无新原生安装/退出、Explorer键盘粘贴、原Windows双端、受限双NAT、LAN/睡眠旅程；不以静态验包或CI替代它们。生产仍旧0.5.0/schema4，准确事务与待批准边界见[升级草案](RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)。
