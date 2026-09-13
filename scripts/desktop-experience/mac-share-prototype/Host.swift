import Cocoa
import CryptoKit

final class HostDelegate: NSObject, NSApplicationDelegate {
    var window: NSWindow?
    func applicationDidFinishLaunching(_ notification: Notification) {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 440, height: 220),
            styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.title = "LinkSend E0 Share Host"
        let label = NSTextField(wrappingLabelWithString: "原生共享包级原型。请在 Finder 的共享菜单选择 LinkSend E0。没有设备列表或发送功能。")
        label.frame = NSRect(x: 20, y: 80, width: 400, height: 100)
        window.contentView?.addSubview(label)
        window.center(); window.makeKeyAndOrderFront(nil)
        self.window = window
        verifyCaptured()
    }
    func application(_ application: NSApplication, open urls: [URL]) { verifyCaptured() }
    func verifyCaptured() {
        guard let container = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: "group.com.linksend.e0.share"),
            let entries = try? FileManager.default.contentsOfDirectory(at: container,
                includingPropertiesForKeys: nil) else { return }
        for folder in entries {
            let receipt = folder.appendingPathComponent("captured.json")
            guard let bytes = try? Data(contentsOf: receipt),
                  let item = try? JSONSerialization.jsonObject(with: bytes) as? [String: Any],
                  let expected = item["sha256"] as? String else { continue }
            let file = folder.appendingPathComponent("fixture.payload")
            guard let data = try? Data(contentsOf: file), data.count <= 1024 * 1024 else { continue }
            let actual = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
            let result: [String: Any] = ["background_readable": true, "hash_matches": actual == expected,
                "host_pid": ProcessInfo.processInfo.processIdentifier, "bytes": data.count,
                "verified_at": ISO8601DateFormatter().string(from: Date())]
            try? JSONSerialization.data(withJSONObject: result, options: .prettyPrinted)
                .write(to: folder.appendingPathComponent("host-verified.json"), options: .atomic)
        }
    }
}
let application = NSApplication.shared
let delegate = HostDelegate()
application.delegate = delegate
application.setActivationPolicy(.regular)
application.run()
