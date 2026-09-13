using System.IO;
using System.Security.Cryptography;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using Windows.ApplicationModel;
using Windows.ApplicationModel.Activation;
using Windows.ApplicationModel.DataTransfer;
using Windows.ApplicationModel.DataTransfer.ShareTarget;
using Windows.Storage;

namespace LinkSend.SharePrototype;

// E0 package-only feasibility tool. It has no device list or network sender.
internal static class Program
{
    private static readonly string Root = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
        "LinkSend.E0.SharePrototype");
    [STAThread]
    public static void Main()
    {
        Directory.CreateDirectory(Root);
        var app = new Application();
        var status = new TextBlock { Text = "正在验证系统共享激活…", TextWrapping = TextWrapping.Wrap };
        var window = new Window { Title = "LinkSend E0 共享原型", Width = 440, Height = 320,
            Content = new Border { Padding = new Thickness(24), Child = status } };
        window.Loaded += async (_, _) =>
        {
            var request = Guid.NewGuid().ToString("N");
            ShareOperation? operation = null;
            var ownedPaths = new List<string>();
            bool captured = false;
            try
            {
                var activation = AppInstance.GetActivatedEventArgs();
                var identity = Package.Current.Id.FullName;
                if (activation is not ShareTargetActivatedEventArgs args)
                {
                    status.Text = "安装身份已验证。请从文件管理器的系统共享选择此原型。";
                    WriteReceipt(request, new { request, identity, kind = activation?.Kind.ToString(),
                        share_operation = false });
                    return;
                }
                operation = args.ShareOperation;
                operation.ReportStarted();
                if (!operation.Data.Contains(StandardDataFormats.StorageItems))
                    throw new InvalidOperationException("STORAGE_ITEMS_REQUIRED");
                var items = await operation.Data.GetStorageItemsAsync();
                if (items.Count is < 1 or > 128) throw new InvalidOperationException("ITEM_LIMIT");
                var folder = Path.Combine(Root, request);
                Directory.CreateDirectory(folder);
                var receipts = new List<object>();
                ulong total = 0;
                foreach (var item in items)
                {
                    if (item is not StorageFile file) throw new InvalidOperationException("FILE_REQUIRED");
                    var info = await file.GetBasicPropertiesAsync();
                    const ulong limit = 64UL * 1024 * 1024;
                    if (info.Size > limit - total) throw new InvalidOperationException("E0_COPY_LIMIT");
                    // E0 small-fixture copy explicitly tests source authorization lifetime.
                    // E3 must prefer transferable access and avoid copying large local files.
                    var destination = Path.Combine(folder, receipts.Count.ToString("D4") + ".payload");
                    using var source = await file.OpenStreamForReadAsync();
                    using (var target = new FileStream(destination, FileMode.CreateNew, FileAccess.Write, FileShare.None))
                    {
                        ownedPaths.Add(destination);
                        var buffer = new byte[64 * 1024];
                        ulong copied = 0;
                        while (true)
                        {
                            var count = await source.ReadAsync(buffer);
                            if (count == 0) break;
                            if ((ulong)count > limit - total || (ulong)count > info.Size - copied)
                                throw new InvalidOperationException("SOURCE_SIZE_CHANGED_OR_COPY_LIMIT");
                            await target.WriteAsync(buffer.AsMemory(0, count));
                            copied += (ulong)count;
                            total += (ulong)count;
                        }
                        var after = await file.GetBasicPropertiesAsync();
                        if (copied != info.Size || after.Size != info.Size || after.DateModified != info.DateModified)
                            throw new InvalidOperationException("SOURCE_CHANGED");
                        target.Flush(true);
                    }
                    using var owned = File.OpenRead(destination);
                    receipts.Add(new { name = file.Name, size = info.Size,
                        sha256 = Convert.ToHexString(SHA256.HashData(owned)).ToLowerInvariant(),
                        owned_path = destination });
                }
                WriteReceipt(request + "-captured", new { request, identity, kind = "ShareTarget",
                    share_operation = true, items = receipts, captured_at = DateTimeOffset.UtcNow });
                captured = true;
                operation.ReportDataRetrieved();
                operation.ReportCompleted();
                WriteReceipt(request + "-completed", new { request, system_report_completed = true });
                status.Text = $"已通过真实 ShareOperation 接管 {items.Count} 个测试文件。\n这是包级原型，尚未接入设备或发送。";
            }
            catch (Exception error)
            {
                status.Text = "原型未通过：" + error.Message;
                string? reportError = null;
                try { operation?.ReportError("LinkSend E0 共享原型未完成"); }
                catch (Exception reportFailure) { reportError = reportFailure.ToString(); }
                var cleanupErrors = new List<string>();
                // Retain durably captured requests even if the system report fails.
                // Before that point, remove only files exclusively created by this request.
                if (!captured)
                    foreach (var path in ownedPaths)
                        try { File.Delete(path); }
                        catch (Exception cleanupFailure) { cleanupErrors.Add(cleanupFailure.Message); }
                try { WriteReceipt(request + "-failed", new { request, failed = true, captured,
                    error = error.ToString(), report_error = reportError, cleanup_errors = cleanupErrors }); }
                catch (Exception receiptFailure) { status.Text += "\n回执保存失败：" + receiptFailure.Message; }
            }
        };
        app.Run(window);
    }

    private static void WriteReceipt(string request, object value)
    {
        var path = Path.Combine(Root, request + ".json");
        using var file = new FileStream(path, FileMode.CreateNew, FileAccess.Write, FileShare.Read);
        JsonSerializer.Serialize(file, value, new JsonSerializerOptions { WriteIndented = true });
        file.Flush(true);
    }
}
