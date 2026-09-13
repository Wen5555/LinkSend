using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using Windows.ApplicationModel;
using Windows.ApplicationModel.Activation;
using Windows.ApplicationModel.DataTransfer;
using Windows.ApplicationModel.DataTransfer.ShareTarget;
using Windows.Storage;

namespace LinkSend.ShareTarget;

internal static class Program
{
    [STAThread]
    public static void Main(string[] args)
    {
        if (args.SequenceEqual(["--self-test"])) { SelfTest.Run(); return; }
        var application = new Application();
        var title = new TextBlock { Text = "使用 LinkSend 发送", FontSize = 20, FontWeight = FontWeights.SemiBold };
        var summary = new TextBlock { Text = "正在读取系统共享…", TextWrapping = TextWrapping.Wrap };
        var devices = new ComboBox { MinWidth = 340, DisplayMemberPath = "Label" };
        var waitOffline = new CheckBox { Content = "设备离线时等待其上线", IsChecked = false, IsEnabled = false };
        var send = new Button { Content = "发送", IsDefault = true, IsEnabled = false, MinWidth = 90 };
        var addDevice = new Button { Content = "添加设备…", MinWidth = 90 };
        var cancel = new Button { Content = "取消", IsCancel = true, MinWidth = 90 };
        var buttons = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
        buttons.Children.Add(addDevice); buttons.Children.Add(cancel); buttons.Children.Add(send);
        var panel = new StackPanel();
        panel.Children.Add(title); panel.Children.Add(summary); panel.Children.Add(devices); panel.Children.Add(waitOffline); panel.Children.Add(buttons);
        var window = new Window { Title = "LinkSend", Width = 440, Height = 280,
            ResizeMode = ResizeMode.NoResize, Content = new Border { Padding = new Thickness(24), Child = panel } };
        ShareOperation? operation = null;
        IReadOnlyList<IStorageItem> items = [];
        var requestID = Guid.NewGuid().ToString("N");
        window.Loaded += async (_, _) =>
        {
            try
            {
                _ = Package.Current.Id.FullName;
                if (AppInstance.GetActivatedEventArgs() is not ShareTargetActivatedEventArgs activated)
                {
                    summary.Text = "请从 Windows 系统共享面板选择 LinkSend。"; return;
                }
                operation = activated.ShareOperation;
                operation.ReportStarted();
                if (!operation.Data.Contains(StandardDataFormats.StorageItems))
                    throw new InvalidDataException("这次共享没有文件项目。");
                items = await operation.Data.GetStorageItemsAsync();
                if (items.Count is < 1 or > ShareJournal.MaximumItems || items.Any(item => item is not StorageFile))
                    throw new InvalidDataException("请一次共享 1 至 1024 个文件；当前系统入口不接收文件夹。");
                var options = ShareJournal.ReadDevices(ShareJournal.ProfileRoot)
                    .Select(device => new DeviceOption(device.id, device.name, device.reachable)).ToList();
                if (options.Count == 0) throw new InvalidOperationException("没有可用的已配对设备，请先打开 LinkSend 完成配对。");
                devices.ItemsSource = options; devices.SelectedIndex = -1;
                summary.Text = $"已选择 {items.Count} 个文件。选择目标后点击发送；离线等待必须单独确认。";
            }
            catch (Exception error) { Fail(operation, summary, error); }
        };
        send.Click += async (_, _) =>
        {
            send.IsEnabled = false; cancel.IsEnabled = false; devices.IsEnabled = false;
            try
            {
                if (operation is null || devices.SelectedItem is not DeviceOption peer)
                    throw new InvalidOperationException("目标设备已失效，请重新共享。");
                bool waitForPeer;
                try { waitForPeer = ShareJournal.ResolveWaitForPeer(peer.Reachable, waitOffline.IsChecked == true); }
                catch (InvalidOperationException error) when (error.Message == "OFFLINE_WAIT_CONFIRMATION_REQUIRED")
                {
                    throw new InvalidOperationException("该设备当前离线；如需加入待发送队列，请先勾选等待上线。");
                }
                var paths = new List<string>(items.Count);
                var ownedDirectory = Path.Combine(ShareJournal.ProfileRoot, "share-owned-v1", requestID);
                var index = 0;
                foreach (var item in items.Cast<StorageFile>())
                {
                    using var access = await item.OpenReadAsync();
                    if (!string.IsNullOrWhiteSpace(item.Path) && Path.IsPathFullyQualified(item.Path)
                        && File.Exists(item.Path))
                    {
                        paths.Add(item.Path);
                    }
                    else
                    {
                        Directory.CreateDirectory(ownedDirectory);
                        var owned = await StorageFolder.GetFolderFromPathAsync(ownedDirectory);
                        var copied = await item.CopyAsync(owned, $"{index:D4}-{item.Name}", NameCollisionOption.FailIfExists);
                        paths.Add(copied.Path);
                    }
                    index++;
                }
                ShareJournal.Persist(ShareJournal.ProfileRoot, new ShareRequest(2, requestID, peer.ID,
                    paths, waitForPeer, "windows_share"));
                operation.ReportDataRetrieved();
                var woke = WakeLinkSend();
                var deadline = DateTime.UtcNow + TimeSpan.FromSeconds(2);
                while (woke && !ShareJournal.IsAccepted(ShareJournal.ProfileRoot, requestID)
                    && DateTime.UtcNow < deadline)
                    await Task.Delay(50);
                var accepted = ShareJournal.IsAccepted(ShareJournal.ProfileRoot, requestID);
                operation.ReportCompleted();
                summary.Text = accepted ? "已加入 LinkSend 发送队列。"
                    : woke ? "请求已保存，LinkSend 后台尚未确认接手；可打开应用查看。"
                    : "请求已保存；下次打开 LinkSend 时继续。";
                window.Close();
            }
            catch (Exception error) { Fail(operation, summary, error); cancel.IsEnabled = true; }
        };
        devices.SelectionChanged += (_, _) =>
        {
            var peer = devices.SelectedItem as DeviceOption;
            send.IsEnabled = peer is not null;
            waitOffline.IsEnabled = peer is { Reachable: false };
            waitOffline.IsChecked = false;
        };
        addDevice.Click += (_, _) =>
        {
            var executable = Path.Combine(AppContext.BaseDirectory, "LinkSend.exe");
            if (File.Exists(executable)) Process.Start(new ProcessStartInfo(executable) { UseShellExecute = false });
        };
        cancel.Click += (_, _) => window.Close();
        application.Run(window);
    }

    private static bool WakeLinkSend()
    {
        var executable = Path.Combine(AppContext.BaseDirectory, "LinkSend.exe");
        if (!File.Exists(executable)) return false;
        return Process.Start(new ProcessStartInfo(executable, "--native-share-background") { UseShellExecute = false,
            WorkingDirectory = AppContext.BaseDirectory }) is not null;
    }

    private static void Fail(ShareOperation? operation, TextBlock status, Exception error)
    {
        status.Text = error.Message;
        try { operation?.ReportError("LinkSend 未接收这次共享：" + error.Message); } catch { }
    }

    private sealed record DeviceOption(string ID, string Name, bool Reachable)
    {
        public string Label => Name + (Reachable ? "" : "（离线，加入队列）");
    }
}

internal static class SelfTest
{
    internal static void Run()
    {
        var root = Path.Combine(Path.GetTempPath(), "linksend-share-target-" + Guid.NewGuid().ToString("N"));
        try
        {
            Directory.CreateDirectory(Path.Combine(root, "native-share-v1"));
            File.WriteAllText(Path.Combine(root, "native-share-v1", "devices.json"),
                "{\"version\":1,\"revision\":1,\"generated_at\":\"" + DateTimeOffset.UtcNow.ToString("O") + "\",\"devices\":[{\"id\":\"peer\",\"name\":\"设备\",\"reachable\":true}]}");
            if (ShareJournal.ReadDevices(root).Single().id != "peer") throw new Exception("DEVICE_TEST_FAILED");
            if (ShareJournal.ResolveWaitForPeer(true, false)) throw new Exception("ONLINE_WAIT_TEST_FAILED");
            try { ShareJournal.ResolveWaitForPeer(false, false); throw new Exception("OFFLINE_CONFIRM_TEST_FAILED"); }
            catch (InvalidOperationException error) when (error.Message == "OFFLINE_WAIT_CONFIRMATION_REQUIRED") { }
            if (!ShareJournal.ResolveWaitForPeer(false, true)) throw new Exception("OFFLINE_WAIT_TEST_FAILED");
            var source = Path.Combine(root, "file.txt"); File.WriteAllText(source, "fixture");
            var request = new ShareRequest(2, "0123456789abcdef0123456789abcdef", "peer", [source], true, "windows_share");
            ShareJournal.Persist(root, request); ShareJournal.Persist(root, request);
            try { ShareJournal.Persist(root, request with { peer_id = "other" }); throw new Exception("CONFLICT_TEST_FAILED"); }
            catch (InvalidOperationException error) when (error.Message == "SHARE_REQUEST_CONFLICT") { }
            Console.WriteLine("PASS share_target_journal");
        }
        finally { if (Directory.Exists(root)) Directory.Delete(root, true); }
    }
}
