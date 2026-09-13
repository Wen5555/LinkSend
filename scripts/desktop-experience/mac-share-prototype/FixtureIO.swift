import Foundation
import Darwin

enum FixtureIO {
    static let payloadLimit = 1024 * 1024
    static let receiptLimit = 16 * 1024

    static func error(_ code: String) -> NSError {
        NSError(domain: code, code: Int(errno))
    }

    // Never allocate the entire file before enforcing the limit. O_NONBLOCK
    // prevents a substituted FIFO from hanging before the regular-file check.
    static func readBounded(_ url: URL, limit: Int) throws -> Data {
        guard limit >= 0, limit <= payloadLimit else { throw error("E0InvalidLimit") }
        let descriptor = Darwin.open(url.path, O_RDONLY | O_NOFOLLOW | O_NONBLOCK)
        guard descriptor >= 0 else { throw error("E0OpenRead") }
        let handle = FileHandle(fileDescriptor: descriptor, closeOnDealloc: true)
        defer { try? handle.close() }
        var info = stat()
        guard fstat(descriptor, &info) == 0, (info.st_mode & S_IFMT) == S_IFREG else {
            throw error("E0RegularFileRequired")
        }
        var result = Data()
        while let chunk = try handle.read(upToCount: min(65536, limit - result.count + 1)), !chunk.isEmpty {
            guard chunk.count <= limit - result.count else { throw error("E0FixtureLimit") }
            result.append(chunk)
        }
        return result
    }
}

// Single owner of one exclusive 0700 request directory. Cleanup does not scan
// the container or recursively remove a directory and its possible new files.
final class OwnedFixtureRequest {
    let directory: URL
    private let descriptor: Int32
    private var files: [String: (dev_t, ino_t)] = [:]

    init(container: URL, request: String) throws {
        guard UUID(uuidString: request) != nil else { throw FixtureIO.error("E0RequestID") }
        directory = container.appendingPathComponent(request, isDirectory: true)
        guard mkdir(directory.path, 0o700) == 0 else { throw FixtureIO.error("E0ExclusiveDirectory") }
        descriptor = Darwin.open(directory.path, O_RDONLY | O_DIRECTORY | O_NOFOLLOW)
        guard descriptor >= 0 else { throw FixtureIO.error("E0DirectoryOpen") }
    }

    deinit { Darwin.close(descriptor) }

    func write(_ name: String, data: Data) throws {
        guard ["fixture.payload", "captured.json", "open-result.json"].contains(name) else {
            throw FixtureIO.error("E0OwnedName")
        }
        let limit = name == "fixture.payload" ? FixtureIO.payloadLimit : FixtureIO.receiptLimit
        guard data.count <= limit else { throw FixtureIO.error("E0FixtureLimit") }
        let file = openat(descriptor, name, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW, 0o600)
        guard file >= 0 else { throw FixtureIO.error("E0ExclusiveFile") }
        let handle = FileHandle(fileDescriptor: file, closeOnDealloc: true)
        defer { try? handle.close() }
        var info = stat()
        guard fstat(file, &info) == 0 else { throw FixtureIO.error("E0FileIdentity") }
        files[name] = (info.st_dev, info.st_ino)
        try handle.write(contentsOf: data)
        try handle.synchronize()
    }

    func cleanupUnpublished() -> [String] {
        var retained: [String] = []
        for (name, expected) in files {
            var current = stat()
            if fstatat(descriptor, name, &current, AT_SYMLINK_NOFOLLOW) != 0 {
                if errno != ENOENT { retained.append(name) }
                continue
            }
            guard current.st_dev == expected.0, current.st_ino == expected.1,
                  (current.st_mode & S_IFMT) == S_IFREG else {
                retained.append(name)
                continue
            }
            if unlinkat(descriptor, name, 0) != 0 { retained.append(name) }
        }
        files.removeAll()
        // Keep the empty directory; never remove unrelated or replaced content.
        return retained
    }
}
