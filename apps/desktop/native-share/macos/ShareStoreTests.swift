import Cocoa

@main
enum ShareStoreTests {
    static func main() throws {
        let manager = FileManager.default
        let root = manager.temporaryDirectory.appendingPathComponent("linksend-share-store-" + UUID().uuidString)
        defer { try? manager.removeItem(at: root) }
        try manager.createDirectory(at: root.appendingPathComponent("native-share-v1"),
            withIntermediateDirectories: true)
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let generated = formatter.string(from: Date())
        try Data("{\"version\":1,\"revision\":1,\"generated_at\":\"\(generated)\",\"devices\":[{\"id\":\"peer\",\"name\":\"测试设备\",\"reachable\":true}]}".utf8)
            .write(to: root.appendingPathComponent("native-share-v1/devices.json"))
        let store = try ShareStore(container: root)
        guard let device = try store.devices().first, device.id == "peer", device.reachable else {
            fatalError("fresh fractional device snapshot")
        }
        guard try ShareStore.resolveWaitForPeer(reachable: true, confirmed: false) == false else {
            fatalError("online wait")
        }
        do {
            _ = try ShareStore.resolveWaitForPeer(reachable: false, confirmed: false)
            fatalError("offline confirmation")
        } catch ShareStoreError.invalidSelection {}
        guard try ShareStore.resolveWaitForPeer(reachable: false, confirmed: true) else {
            fatalError("offline wait")
        }
        let request = "0123456789abcdef0123456789abcdef"
        let source = root.appendingPathComponent("fixture.txt")
        try Data("fixture".utf8).write(to: source)
        guard let provider = NSItemProvider(contentsOf: source) else { fatalError("provider") }
        var prepared: Result<([String], [String]), Error>?
        store.prepare([provider], requestID: "abcdefabcdefabcdefabcdefabcdefab") { prepared = $0 }
        let deadline = Date(timeIntervalSinceNow: 5)
        while prepared == nil && Date() < deadline {
            RunLoop.current.run(until: Date(timeIntervalSinceNow: 0.05))
        }
        guard let prepared else { throw ShareStoreError.sourceUnavailable }
        let (preparedPaths, preparedBookmarks) = try prepared.get()
        guard preparedPaths.count == 1, preparedBookmarks.count == 1,
              manager.fileExists(atPath: preparedPaths[0]) else { fatalError("provider handoff") }
        try store.persist(requestID: request, peerID: "peer", paths: [source.path], bookmarks: [""])
        try store.persist(requestID: request, peerID: "peer", paths: [source.path], bookmarks: [""])
        do {
            try store.persist(requestID: request, peerID: "other", paths: [source.path], bookmarks: [""])
            fatalError("conflict accepted")
        } catch ShareStoreError.requestConflict { }
        guard manager.fileExists(atPath: root.appendingPathComponent("desktop-activations-v1/" + request + ".json").path) else {
            fatalError("request missing")
        }
        for index in 1..<64 {
            let requestID = String(repeating: "0", count: 30) + String(format: "%02x", index)
            try store.persist(requestID: requestID, peerID: "peer", paths: [source.path], bookmarks: [""])
        }
        let journal = root.appendingPathComponent("desktop-activations-v1")
        let savedRequest = journal.appendingPathComponent(request + ".json")
        let original = try Data(contentsOf: savedRequest)
        try store.persist(requestID: request, peerID: "peer", paths: [source.path], bookmarks: [""])
        do {
            try store.persist(requestID: request, peerID: "other", paths: [source.path], bookmarks: [""])
            fatalError("full journal accepted conflicting replay")
        } catch ShareStoreError.requestConflict {}
        do {
            try store.persist(requestID: String(repeating: "f", count: 32), peerID: "peer",
                              paths: [source.path], bookmarks: [""])
            fatalError("journal capacity exceeded")
        } catch ShareStoreError.storageFull {}
        let entries = try manager.contentsOfDirectory(at: journal, includingPropertiesForKeys: nil)
        guard entries.filter({ $0.pathExtension == "json" }).count == 64,
              try Data(contentsOf: savedRequest) == original else { fatalError("full replay changed journal") }
        print("PASS share_store_devices")
        print("PASS share_store_provider_handoff")
        print("PASS share_store_idempotency")
        print("PASS share_store_conflict")
        print("PASS share_store_full_journal_retry")
    }
}
