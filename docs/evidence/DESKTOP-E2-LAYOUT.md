# E2 四视图与渲染证据

主导航固定为传输、记录、设备、设置。传输页内分发送文件、待发送、进行中；工作区采用单主滚动区，队列、任务、设备目录和接收清单有界滚动。设备页接入空组初始化、配对码、身份冲突后的显式切组、删除、附近 LAN 请求及对端同意/拒绝。接收弹窗有焦点圈、Tab 循环、Esc 收起详情和粘底操作区；reduced motion 关闭动效。

2026-09-13 用 `@playwright/cli` 对 Vite production build 实际渲染，控制台 0 error（唯一 warning 是 Wails 提示浏览器仅用于 UI preview）：

- 1100×720 传输首屏：[截图](assets/e2/e2-transfer-1100x720.png)，SHA256 `5814b30a582275a2dc23d747e97cf0d170f11ed86941b9d7b0e571c98a97a2fc`。
- 960×640 设置：[截图](assets/e2/e2-settings-960x640.png)，SHA256 `aa07d7f95d191bcfd1a781d5d423d797c4515d13b7519ab30573585dbca10020`。
- 960×640 深色并启用 reduced motion：[截图](assets/e2/e2-settings-dark-reduced-960x640.png)，SHA256 `c753232965b0bede424e20b70e16dfec17de31296eaed1ea04bc15e0f67c9974`。
- 将 960×640 分别折算为 125% 的 768×512 和 150% 的 640×427 CSS 可用区；两者 `documentElement.scrollWidth == innerWidth`、四个主导航均存在：[125%](assets/e2/e2-transfer-960x640-at-125-percent.png)、[150%](assets/e2/e2-settings-960x640-at-150-percent.png)。
- 键盘 Tab 后活动元素为“文件接收”设置分类按钮，截图显示 `:focus-visible` 焦点圈。150% 检查注入长中文本用于溢出检查；移动布局隐藏侧栏 profile，页面无水平溢出。

浏览器预览不执行后端传输，不能替代 Wails 准确包或网络结果。真实文件数据面沿用 E1 的独立证据；本轮准确 Windows/macOS 包的缩放与原生键盘矩阵仍 NOT RUN。
