using System.IO;
using System.Text.Json;
using System.Threading;

namespace LinkSend.ShareTarget;

internal sealed record ShareDevice(string id, string name, bool reachable);
internal sealed record DeviceSnapshot(int version, ulong revision, string generated_at, List<ShareDevice> devices);
internal sealed record ShareRequest(int version, string request_id, string peer_id, List<string> paths,
    bool wait_for_peer, string source);

internal static class ShareJournal
{
    internal const int MaximumItems = 1024;

    internal static bool ResolveWaitForPeer(bool reachable, bool confirmed)
    {
        if (!reachable && !confirmed)
            throw new InvalidOperationException("OFFLINE_WAIT_CONFIRMATION_REQUIRED");
        return !reachable && confirmed;
    }

    internal static bool IsAccepted(string profileRoot, string requestID) =>
        File.Exists(Path.Combine(profileRoot, "native-share-v1", "accepted", requestID));
    internal static string ProfileRoot => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "LinkSend");

    internal static List<ShareDevice> ReadDevices(string profileRoot)
    {
        var snapshot = ReadSnapshot(profileRoot);
        var fresh = SnapshotFresh(snapshot);
        return snapshot.devices.Where(device => !string.IsNullOrWhiteSpace(device.id)
            && !string.IsNullOrWhiteSpace(device.name))
            .Select(device => device with { reachable = fresh && device.reachable }).ToList();
    }

    internal static bool DevicesFresh(string profileRoot) => SnapshotFresh(ReadSnapshot(profileRoot));

    private static bool SnapshotFresh(DeviceSnapshot snapshot)
    {
        if (!DateTimeOffset.TryParse(snapshot.generated_at, out var generated)) return false;
        var age = DateTimeOffset.UtcNow - generated.ToUniversalTime();
        return age >= TimeSpan.FromSeconds(-5) && age <= TimeSpan.FromMinutes(1);
    }

    private static DeviceSnapshot ReadSnapshot(string profileRoot)
    {
        var path = Path.Combine(profileRoot, "native-share-v1", "devices.json");
        if ((File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
            throw new InvalidDataException("DEVICE_SNAPSHOT_REPARSE_POINT");
        using var file = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read,
            64 * 1024, FileOptions.SequentialScan);
        if (file.Length is < 2 or > 256 * 1024) throw new InvalidDataException("DEVICE_SNAPSHOT_SIZE");
        var snapshot = JsonSerializer.Deserialize<DeviceSnapshot>(file)
            ?? throw new InvalidDataException("DEVICE_SNAPSHOT_INVALID");
        if (snapshot.version != 1 || snapshot.devices.Count > 256)
            throw new InvalidDataException("DEVICE_SNAPSHOT_VERSION");
        return snapshot;
    }

    internal static void Persist(string profileRoot, ShareRequest request)
    {
        if (request.version != 2 || request.source != "windows_share"
            || request.request_id.Length != 32 || request.request_id.Any(character => !Uri.IsHexDigit(character) || char.IsUpper(character))
            || string.IsNullOrWhiteSpace(request.peer_id) || request.paths.Count is < 1 or > MaximumItems
            || request.paths.Any(path => !Path.IsPathFullyQualified(path) || path.Contains('\0')))
            throw new InvalidDataException("SHARE_REQUEST_INVALID");
        var directory = Path.Combine(profileRoot, "desktop-activations-v1");
        Directory.CreateDirectory(directory);
        var info = new DirectoryInfo(directory);
        if ((info.Attributes & FileAttributes.ReparsePoint) != 0)
            throw new InvalidDataException("SHARE_JOURNAL_REPARSE_POINT");
        var lockPath = Path.Combine(directory, ".lock");
        using var journalLock = AcquireLock(lockPath, TimeSpan.FromSeconds(2));
        if (Directory.EnumerateFiles(directory, "*.json").Take(65).Count() >= 64)
            throw new InvalidOperationException("SHARE_JOURNAL_FULL");
        var data = JsonSerializer.SerializeToUtf8Bytes(request);
        if (data.Length > 1024 * 1024) throw new InvalidDataException("SHARE_REQUEST_SIZE");
        var final = Path.Combine(directory, request.request_id + ".json");
        if (File.Exists(final))
        {
            var saved = File.ReadAllBytes(final);
            if (saved.AsSpan().SequenceEqual(data)) return;
            throw new InvalidOperationException("SHARE_REQUEST_CONFLICT");
        }
        var temporary = Path.Combine(directory, request.request_id + ".pending");
        using (var file = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None,
            64 * 1024, FileOptions.WriteThrough))
        {
            file.Write(data);
            file.Flush(true);
        }
        File.Move(temporary, final);
    }

    private static FileStream AcquireLock(string path, TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        while (true)
        {
            try { return new FileStream(path, FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.None); }
            catch (IOException) when (DateTime.UtcNow < deadline) { Thread.Sleep(10); }
        }
    }
}
