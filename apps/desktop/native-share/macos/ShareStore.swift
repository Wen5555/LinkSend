import Foundation
import UniformTypeIdentifiers
import Darwin

struct ShareDevice: Codable {
    let id: String
    let name: String
    let reachable: Bool
}

private struct DeviceSnapshot: Codable {
    let version: Int
    let revision: UInt64
    let generated_at: String
    let devices: [ShareDevice]
}

private struct ShareRequest: Codable {
    let version: Int
    let request_id: String
    let peer_id: String
    let paths: [String]
    let bookmarks: [String]
    let wait_for_peer: Bool
    let source: String
}

private struct PreparedSource {
    let path: String
    let bookmark: String
}

enum ShareStoreError: LocalizedError {
    case groupUnavailable, devicesUnavailable, invalidSelection, unsupportedItem, sourceUnavailable
    case requestConflict, storageFull

    var errorDescription: String? {
        switch self {
        case .groupUnavailable: return "LinkSend 共享容器不可用。此功能需要正式签名的 App Group。"
        case .devicesUnavailable: return "没有可用的已配对设备。请先打开 LinkSend 完成配对。"
        case .invalidSelection: return "目标设备已失效，请刷新后重试。"
        case .unsupportedItem: return "这次共享包含当前版本无法安全接管的项目。"
        case .sourceUnavailable: return "源文件授权已失效，请从来源应用重新共享。"
        case .requestConflict: return "系统重复使用了不同的共享请求，请重新发起。"
        case .storageFull: return "待处理共享已满，请打开 LinkSend 处理后重试。"
        }
    }
}

final class ShareStore {
    static let groupID = "group.com.linksend.desktop"
    static let maximumItems = 1024
    private let manager = FileManager.default
    let container: URL

    init(container override: URL? = nil) throws {
        guard let container = override ?? manager.containerURL(forSecurityApplicationGroupIdentifier: Self.groupID) else {
            throw ShareStoreError.groupUnavailable
        }
        self.container = container
    }

    static func resolveWaitForPeer(reachable: Bool, confirmed: Bool) throws -> Bool {
        if !reachable && !confirmed { throw ShareStoreError.invalidSelection }
        return !reachable && confirmed
    }

    func isAccepted(requestID: String) -> Bool {
        manager.fileExists(atPath: container.appendingPathComponent(
            "native-share-v1/accepted/" + requestID).path)
    }

    func devices() throws -> [ShareDevice] {
        let url = container.appendingPathComponent("native-share-v1/devices.json")
        let data = try readBounded(url, maximum: 256 * 1024)
        let snapshot = try JSONDecoder().decode(DeviceSnapshot.self, from: data)
        guard snapshot.version == 1 else { throw ShareStoreError.devicesUnavailable }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let generated = formatter.date(from: snapshot.generated_at)
            ?? ISO8601DateFormatter().date(from: snapshot.generated_at)
        let fresh = generated.map {
            let age = Date().timeIntervalSince($0)
            return age >= -5 && age <= 60
        } ?? false
        let devices = snapshot.devices.filter { !$0.id.isEmpty && !$0.name.isEmpty }.map {
            ShareDevice(id: $0.id, name: $0.name, reachable: fresh && $0.reachable)
        }
        guard !devices.isEmpty else { throw ShareStoreError.devicesUnavailable }
        return Array(devices.prefix(256))
    }

    func prepare(_ providers: [NSItemProvider], requestID: String,
                 completion: @escaping (Result<([String], [String]), Error>) -> Void) {
        guard !providers.isEmpty, providers.count <= Self.maximumItems else {
            completion(.failure(ShareStoreError.unsupportedItem)); return
        }
        let owned = container.appendingPathComponent("share-owned-v1/" + requestID, isDirectory: true)
        do { try manager.createDirectory(at: owned, withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]) }
        catch { completion(.failure(error)); return }
        let group = DispatchGroup()
        let lock = NSLock()
        var results = Array<PreparedSource?>(repeating: nil, count: providers.count)
        var firstError: Error?
        var ownedBytes: Int64 = 0
        let maximumOwnedBytes: Int64 = 16 * 1024 * 1024 * 1024
        let providerSlots = DispatchSemaphore(value: 4)
        for (index, provider) in providers.enumerated() {
            guard let type = provider.registeredTypeIdentifiers.first(where: {
                $0 != UTType.fileURL.identifier && UTType($0)?.conforms(to: .data) == true
            }) ?? provider.registeredTypeIdentifiers.first(where: {
                UTType($0)?.conforms(to: .data) == true
            }) else {
                lock.lock(); if firstError == nil { firstError = ShareStoreError.unsupportedItem }; lock.unlock()
                continue
            }
            group.enter()
            let receive: (URL?, Bool, Error?) -> Void = { url, inPlace, error in
                defer { providerSlots.signal(); group.leave() }
                do {
                    guard let url else { throw error ?? ShareStoreError.sourceUnavailable }
                    let values = try url.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey])
                    guard values.isRegularFile == true, values.isSymbolicLink != true else {
                        throw ShareStoreError.unsupportedItem
                    }
                    let scoped = url.startAccessingSecurityScopedResource()
                    defer { if scoped { url.stopAccessingSecurityScopedResource() } }
                    let prepared: PreparedSource
                    if inPlace {
                        let bookmark = try url.bookmarkData(options: [.withSecurityScope, .securityScopeAllowOnlyReadAccess],
                            includingResourceValuesForKeys: nil, relativeTo: nil)
                        prepared = PreparedSource(path: url.path, bookmark: bookmark.base64EncodedString())
                    } else {
                        let target = owned.appendingPathComponent(String(format: "%04d-", index) + url.lastPathComponent)
                        try self.copyTemporaryRepresentation(from: url, to: target, expectedSize: values.fileSize) { bytes in
                            lock.lock()
                            defer { lock.unlock() }
                            guard bytes <= maximumOwnedBytes - ownedBytes else { return false }
                            ownedBytes += bytes
                            return true
                        }
                        prepared = PreparedSource(path: target.path, bookmark: "")
                    }
                    lock.lock(); results[index] = prepared; lock.unlock()
                } catch {
                    lock.lock(); if firstError == nil { firstError = error }; lock.unlock()
                }
            }
            // The advertised fileURL UTI says what the value is, not how long
            // its authorization survives the provider callback. The API's
            // actual inPlace result decides bookmark versus owned copy.
            DispatchQueue.global(qos: .userInitiated).async {
                providerSlots.wait()
                provider.loadInPlaceFileRepresentation(forTypeIdentifier: type, completionHandler: receive)
            }
        }
        group.notify(queue: .main) {
            if let error = firstError {
                try? self.manager.removeItem(at: owned)
                completion(.failure(error)); return
            }
            let prepared = results.compactMap { $0 }
            guard prepared.count == providers.count else {
                completion(.failure(ShareStoreError.sourceUnavailable)); return
            }
            completion(.success((prepared.map(\.path), prepared.map(\.bookmark))))
        }
    }

    func cleanup(requestID: String) {
        guard requestID.range(of: "^[0-9a-f]{32}$", options: .regularExpression) != nil else { return }
        try? manager.removeItem(at: container.appendingPathComponent("share-owned-v1/" + requestID))
    }

    func persist(requestID: String, peerID: String, paths: [String], bookmarks: [String],
                 waitForPeer: Bool = false) throws {
        guard requestID.range(of: "^[0-9a-f]{32}$", options: .regularExpression) != nil,
              !peerID.isEmpty, paths.count == bookmarks.count else { throw ShareStoreError.invalidSelection }
        let directory = container.appendingPathComponent("desktop-activations-v1", isDirectory: true)
        try manager.createDirectory(at: directory, withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700])
        let lockURL = directory.appendingPathComponent(".lock")
        let descriptor = open(lockURL.path, O_CREAT | O_RDWR | O_CLOEXEC, 0o600)
        guard descriptor >= 0 else { throw POSIXError(.init(rawValue: errno) ?? .EIO) }
        defer { flock(descriptor, LOCK_UN); close(descriptor) }
        let lockDeadline = Date(timeIntervalSinceNow: 2)
        while flock(descriptor, LOCK_EX | LOCK_NB) != 0 {
            guard errno == EWOULDBLOCK, Date() < lockDeadline else {
                throw POSIXError(.init(rawValue: errno) ?? .EIO)
            }
            usleep(10_000)
        }
        let existing = try manager.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)
        guard existing.count <= 128,
              existing.filter({ $0.pathExtension == "json" }).count < 64 else {
            throw ShareStoreError.storageFull
        }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(ShareRequest(version: 2, request_id: requestID, peer_id: peerID,
            paths: paths, bookmarks: bookmarks, wait_for_peer: waitForPeer, source: "macos_share"))
        guard data.count <= 1024 * 1024 else { throw ShareStoreError.storageFull }
        let final = directory.appendingPathComponent(requestID + ".json")
        if manager.fileExists(atPath: final.path) {
            if try readBounded(final, maximum: 1024 * 1024) == data { return }
            throw ShareStoreError.requestConflict
        }
        let temporary = directory.appendingPathComponent(requestID + ".pending")
        try data.write(to: temporary, options: .withoutOverwriting)
        let file = try FileHandle(forWritingTo: temporary)
        try file.synchronize(); try file.close()
        try manager.moveItem(at: temporary, to: final)
    }

    private func copyTemporaryRepresentation(from source: URL, to target: URL, expectedSize: Int?,
                                             reserve: (Int64) -> Bool) throws {
        guard !manager.fileExists(atPath: target.path) else { throw ShareStoreError.requestConflict }
        guard manager.createFile(atPath: target.path, contents: nil, attributes: [.posixPermissions: 0o600]) else {
            throw ShareStoreError.sourceUnavailable
        }
        do {
            let input = try FileHandle(forReadingFrom: source)
            let output = try FileHandle(forWritingTo: target)
            defer { try? input.close(); try? output.close() }
            var copied: Int64 = 0
            while true {
                let chunk = try input.read(upToCount: 1024 * 1024) ?? Data()
                if chunk.isEmpty { break }
                copied += Int64(chunk.count)
                guard reserve(Int64(chunk.count)) else { throw ShareStoreError.storageFull }
                try output.write(contentsOf: chunk)
            }
            try output.synchronize()
        } catch {
            try? manager.removeItem(at: target)
            throw error
        }
        let values = try target.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey])
        guard values.isRegularFile == true, values.isSymbolicLink != true,
              expectedSize == nil || values.fileSize == expectedSize else {
            try? manager.removeItem(at: target)
            throw ShareStoreError.sourceUnavailable
        }
    }

    private func readBounded(_ url: URL, maximum: Int) throws -> Data {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        let data = try handle.read(upToCount: maximum + 1) ?? Data()
        guard data.count <= maximum else { throw ShareStoreError.storageFull }
        return data
    }
}
