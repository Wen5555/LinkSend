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
        var devices = new StackPanel { MinWidth = 340 };
        var deviceScroll = new ScrollViewer { Content = devices, MaxHeight = 100, VerticalScrollBarVisibility = ScrollBarVisibility.Auto };
        var waitOffline = new CheckBox { Content = "设备离线时等待其上线", IsChecked = false, IsEnabled = false };
        var send = new Button { Content = "发送", IsDefault = true, IsEnabled = false, MinWidth = 90 };
        var addDevice = new Button { Content = "添加设备…", MinWidth = 90 };
        var refreshDevices = new Button { Content = "刷新设备", MinWidth = 80 };
        var cancel = new Button { Content = "取消", IsCancel = true, MinWidth = 90 };
        var buttons = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
        buttons.Children.Add(addDevice); buttons.Children.Add(refreshDevices); buttons.Children.Add(cancel); buttons.Children.Add(send);
        var panel = new StackPanel();
        panel.Children.Add(title); panel.Children.Add(summary); panel.Children.Add(deviceScroll); panel.Children.Add(waitOffline); panel.Children.Add(buttons);
        var window = new Window { Title = "LinkSend", Width = 440, Height = 280,
            ResizeMode = ResizeMode.NoResize, Content = new Border { Padding = new Thickness(24), Child = panel } };
        ShareOperation? operation = null;
        IReadOnlyList<IStorageItem> items = [];
        DeviceOption? selectedPeer = null;
        var requestID = Guid.NewGuid().ToString("N");
        var requestPersisted = false;
        async Task RefreshDevicesAsync()
        {
            _ = WakeLinkSend();
            var deadline = DateTime.UtcNow + TimeSpan.FromSeconds(2);
            var fresh = false;
            while (DateTime.UtcNow < deadline)
            {
                try { if (ShareJournal.DevicesFresh(ShareJournal.ProfileRoot)) { fresh = true; break; } }
                catch { }
                await Task.Delay(100);
            }
            devices.Children.Clear();
            selectedPeer = null; send.IsEnabled = false; waitOffline.IsEnabled = false; waitOffline.IsChecked = false;
            if (!fresh)
            {
                summary.Text = "设备状态尚未刷新。请确认 LinkSend 后台已启动，然后点击“刷新设备”。";
                return;
            }
            var options = ShareJournal.ReadDevices(ShareJournal.ProfileRoot)
                .Select(device => new DeviceOption(device.id, device.name, device.reachable)).ToList();
            if (options.Count == 0)
            {
                summary.Text = "没有已配对设备，请先点击“添加设备”。";
                return;
            }
            foreach (var option in options)
            {
                var target = new Button { Content = option.Label, HorizontalContentAlignment = HorizontalAlignment.Left,
                    Margin = new Thickness(0, 2, 0, 2) };
                target.Click += (_, _) =>
                {
                    selectedPeer = option;
                    waitOffline.IsEnabled = !option.Reachable;
                    waitOffline.IsChecked = false;
                    send.IsEnabled = true;
                    if (option.Reachable) send.RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
                };
                devices.Children.Add(target);
            }
            summary.Text = $"已选择 {items.Count} 个文件。在线设备可直接点击发送；离线等待必须单独确认。";
        }
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
                await RefreshDevicesAsync();
            }
            catch (Exception error) { Fail(operation, summary, error); }
        };
        send.Click += async (_, _) =>
        {
            send.IsEnabled = false; cancel.IsEnabled = false; devices.IsEnabled = false;
            try
            {
                if (operation is null || selectedPeer is not DeviceOption peer)
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
                ulong ownedBytes = 0;
                foreach (var item in items.Cast<StorageFile>())
                {
                    using var access = await item.OpenReadAsync();
                    if (!NeedsOwnedCopy(item))
                    {
                        paths.Add(item.Path);
                    }
                    else
                    {
                        Directory.CreateDirectory(ownedDirectory);
                        var target = Path.Combine(ownedDirectory, $"{index:D4}-{item.Name}");
                        using var input = access.AsStreamForRead();
                        ownedBytes = await CopyOwnedAsync(input, target, ownedBytes, 16UL * 1024 * 1024 * 1024);
                        paths.Add(target);
                    }
                    index++;
                }
                ShareJournal.Persist(ShareJournal.ProfileRoot, new ShareRequest(2, requestID, peer.ID,
                    paths, waitForPeer, "windows_share"));
                requestPersisted = true;
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
            catch (Exception error)
            {
                if (!requestPersisted)
                    try { Directory.Delete(Path.Combine(ShareJournal.ProfileRoot, "share-owned-v1", requestID), true); } catch { }
                summary.Text = error.Message;
                cancel.IsEnabled = true; send.IsEnabled = true; devices.IsEnabled = true;
            }
        };
        addDevice.Click += (_, _) =>
        {
            var executable = Path.Combine(AppContext.BaseDirectory, "LinkSend.exe");
            if (File.Exists(executable)) Process.Start(new ProcessStartInfo(executable) { UseShellExecute = false });
        };
        refreshDevices.Click += async (_, _) => await RefreshDevicesAsync();
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

    internal static bool NeedsOwnedCopy(StorageFile item)
    {
        if (string.IsNullOrWhiteSpace(item.Path) || !Path.IsPathFullyQualified(item.Path) || !File.Exists(item.Path))
            return true;
        var full = Path.GetFullPath(item.Path);
        var temp = Path.GetFullPath(Path.GetTempPath());
        return full.StartsWith(temp, StringComparison.OrdinalIgnoreCase)
            || (File.GetAttributes(full) & System.IO.FileAttributes.Temporary) != 0;
    }

    internal static async Task<ulong> CopyOwnedAsync(Stream input, string target, ulong current, ulong maximum)
    {
        try
        {
            using var output = new FileStream(target, FileMode.CreateNew, FileAccess.Write, FileShare.None,
                1024 * 1024, FileOptions.Asynchronous | FileOptions.WriteThrough);
            var buffer = new byte[1024 * 1024];
            while (true)
            {
                var read = await input.ReadAsync(buffer);
                if (read == 0) break;
                if ((ulong)read > maximum - current)
                    throw new InvalidOperationException("临时来源总大小超过 16 GiB，请分批共享。");
                await output.WriteAsync(buffer.AsMemory(0, read));
                current += (ulong)read;
            }
            await output.FlushAsync();
            output.Flush(true);
            return current;
        }
        catch
        {
            try { File.Delete(target); } catch { }
            throw;
        }
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
            var temporary = StorageFile.GetFileFromPathAsync(source).AsTask().GetAwaiter().GetResult();
            if (!Program.NeedsOwnedCopy(temporary)) throw new Exception("TEMPORARY_SOURCE_TEST_FAILED");
            var bounded = Path.Combine(root, "bounded-copy.bin");
            try
            {
                Program.CopyOwnedAsync(new MemoryStream(new byte[5]), bounded, 0, 4).GetAwaiter().GetResult();
                throw new Exception("COPY_LIMIT_TEST_FAILED");
            }
            catch (InvalidOperationException error) when (error.Message.Contains("16 GiB")) { }
            if (File.Exists(bounded)) throw new Exception("COPY_CLEANUP_TEST_FAILED");
            var request = new ShareRequest(2, "0123456789abcdef0123456789abcdef", "peer", [source], true, "windows_share");
            ShareJournal.Persist(root, request); ShareJournal.Persist(root, request);
            try { ShareJournal.Persist(root, request with { peer_id = "other" }); throw new Exception("CONFLICT_TEST_FAILED"); }
            catch (InvalidOperationException error) when (error.Message == "SHARE_REQUEST_CONFLICT") { }
            Console.WriteLine("PASS share_target_journal");
        }
        finally { if (Directory.Exists(root)) Directory.Delete(root, true); }
    }
}
