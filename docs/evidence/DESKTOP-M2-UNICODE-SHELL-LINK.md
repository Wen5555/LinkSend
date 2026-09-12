# M2 Windows Unicode SendTo 修复

2026-09-12。M3候选408287c的核心与打包CI成功，但desktop run34680294809仍在 `TestSendToNativeShortcutRoundTripInIsolatedDirectory` 失败：WScript.Shell设置中文TargetPath返回COM Exception。不能将上次临时目录/释放时机改进当作该问题已解决。

本机对a6aa1f6候选的同一测试只临时增加非BMP字符 `🛰`（运行后原文件恢复），真实复现相同 `set TargetPath` exception，exit1。原测试目标只包含当前系统代码页可表达的中文，因此此前本地多轮通过没有覆盖这个边界。

实现改用 Windows 原生 IShellLinkW + IPersistFile。方法顺序、UTF16参数、HRESULT以及GUID由本机Microsoft Windows SDK10.0.26100的ShObjIdl_core.h、objidl.h、ShlGuid.h核实；COM STA锁线程，释放接口后才发布/读取。路径、参数、工作目录和说明全用wide接口；读回SLGP_RAWPATH，不调用Resolve、不执行目标。现有marker/target所有权、no-replace发布和卸载保护保持。

实际Windows验证：原中文目标定向race10轮PASS；补入非BMP目标后同套race10轮PASS（2.013s），桌面vet PASS。命令 `GOWORK=off go test -race ./... -run 'TestSendTo|TestNativeWindowsDrive' -count=10`；已有foreign入口和其他安装不可覆盖/卸载测试继续通过。这些是实际系统COM roundtrip，不等于用户在Explorer菜单点击，新的精确CI仍需重跑。
