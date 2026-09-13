import Cocoa

@main
enum ShareStoreTests {
    static func main() throws {
        let manager = FileManager.default
        let root = manager.temporaryDirectory.appendingPathComponent("linksend-share-store-" + UUID().uuidString)
        defer { try? manager.removeItem(at: root) }
        try manager.createDirectory(at: root.appendingPathComponent("native-share-v1"),
            withIntermediateDirectories: true)
        try Data("{\"version\":1,\"revision\":1,\"generated_at\":\"test\",\"devices\":[{\"id\":\"peer\",\"name\":\"测试设备\",\"reachable\":true}]}".utf8)
            .write(to: root.appendingPathComponent("native-share-v1/devices.json"))
        let store = try ShareStore(container: root)
        guard try store.devices().first?.id == "peer" else { fatalError("device snapshot") }
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
        print("PASS share_store_devices")
        print("PASS share_store_provider_handoff")
        print("PASS share_store_idempotency")
        print("PASS share_store_conflict")
    }
}
