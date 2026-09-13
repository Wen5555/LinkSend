import Cocoa
import UniformTypeIdentifiers
import CryptoKit

@objc(LinkSendE0ShareViewController)
final class ShareViewController: NSViewController {
    private let message = NSTextField(wrappingLabelWithString: "E0 原生共享原型：仅验证测试文件授权和 App Group 交接，不发送。")
    private var started = false
    override func loadView() {
        NSLog("E0_LOAD_VIEW")
        view = NSView(frame: NSRect(x: 0, y: 0, width: 420, height: 240))
        message.frame = NSRect(x: 20, y: 100, width: 380, height: 110)
        view.addSubview(message)
        let button = NSButton(title: "接管测试文件", target: self, action: #selector(capture))
        button.frame = NSRect(x: 20, y: 40, width: 180, height: 36)
        view.addSubview(button)
        DispatchQueue.main.async { [weak self] in self?.capture() }
    }
    override func viewDidLoad() {
        super.viewDidLoad()
        // E0 only: activating this explicitly named prototype takes ownership
        // of its single small fixture; it never sends to any device.
        DispatchQueue.main.async { [weak self] in self?.capture() }
    }
    private func failRequest(_ text: String, code: Int) {
        message.stringValue = text
        extensionContext?.cancelRequest(withError: NSError(domain: "LinkSendShare", code: code,
            userInfo: [NSLocalizedDescriptionKey: text]))
    }
    @objc private func capture() {
        guard !started else { return }
        started = true
        NSLog("E0_CAPTURE_ENTER")
        guard let container = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: "group.com.linksend.e0.share") else {
            failRequest("App Group 容器不可用；需要有效签名与授权。", code: 1)
            return
        }
        NSLog("E0_CONTAINER_READY")
        let providers = (extensionContext?.inputItems as? [NSExtensionItem] ?? [])
            .flatMap { $0.attachments ?? [] }
        let request = UUID().uuidString
        let diagnostic: [String: Any] = ["request": request, "provider_count": providers.count,
            "types": providers.map { $0.registeredTypeIdentifiers }]
        do {
            NSLog("E0_DIAGNOSTIC_WRITE_BEGIN")
            try JSONSerialization.data(withJSONObject: diagnostic, options: .prettyPrinted)
                .write(to: container.appendingPathComponent(request + "-started.json"), options: .atomic)
            NSLog("E0_DIAGNOSTIC_WRITE_COMPLETE")
        } catch {
            NSLog("E0_DIAGNOSTIC_WRITE_FAILED: %@", error.localizedDescription)
            message.stringValue = "无法写入 App Group：" + error.localizedDescription
            return
        }
        guard providers.count == 1, let provider = providers.first,
              let type = provider.registeredTypeIdentifiers.first(where: {
                  UTType($0)?.conforms(to: .data) == true
              }) else {
            failRequest("原型仅接受一个小测试文件。", code: 2)
            return
        }
        let fileURL = provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier)
        let receive: (URL?, Error?) -> Void = { [weak self] source, error in
            var owned: OwnedFixtureRequest?
            do {
                guard let source else { throw error ?? NSError(domain: "E0MissingFile", code: 1) }
                let scoped = source.startAccessingSecurityScopedResource()
                defer { if scoped { source.stopAccessingSecurityScopedResource() } }
                let capture = try OwnedFixtureRequest(container: container, request: request)
                owned = capture
                let folder = capture.directory
                // Take ownership before this completion handler returns, as required by NSItemProvider.
                let target = folder.appendingPathComponent("fixture.payload")
                let bytes = try FixtureIO.readBounded(source, limit: FixtureIO.payloadLimit)
                try capture.write("fixture.payload", data: bytes)
                let receipt: [String: Any] = ["request": request, "bytes": bytes.count,
                    "sha256": SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined(),
                    "source_representation": source.path, "owned_path": target.path,
                    "representation_kind": fileURL ? "security_scoped_file_url" : "temporary_file_representation",
                    "security_scope_started": scoped,
                    "extension_pid": ProcessInfo.processInfo.processIdentifier]
                try capture.write("captured.json", data: JSONSerialization.data(
                    withJSONObject: receipt, options: [.prettyPrinted, .sortedKeys]))
                DispatchQueue.main.async {
                    self?.message.stringValue = "文件已在表示回调内接管。正在验证后台唤起。"
                    self?.extensionContext?.open(URL(string: "linksend-e0-share://handoff/" + request)!) { opened in
                        let result: [String: Any] = ["request": request, "open_returned": opened]
                        try? capture.write("open-result.json", data: JSONSerialization.data(
                            withJSONObject: result, options: .prettyPrinted))
                        DispatchQueue.main.async {
                            self?.message.stringValue = opened ? "后台唤起请求已接受。" : "系统未允许该扩展唤起后台；接管回执已保留。"
                            self?.extensionContext?.completeRequest(returningItems: nil)
                        }
                    }
                }
            } catch {
                let retained = owned?.cleanupUnpublished() ?? []
                let failure: [String: Any] = ["request": request, "error": error.localizedDescription,
                    "type": type, "file_url": fileURL, "cleanup_retained": retained]
                try? JSONSerialization.data(withJSONObject: failure, options: .prettyPrinted)
                    .write(to: container.appendingPathComponent(request + "-failed.json"), options: .atomic)
                DispatchQueue.main.async {
                    self?.failRequest("原型失败：" + error.localizedDescription, code: 3)
                }
            }
        }
        if fileURL {
            provider.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) { item, error in
                receive(item as? URL, error)
            }
        } else {
            provider.loadFileRepresentation(forTypeIdentifier: type, completionHandler: receive)
        }
    }
}
