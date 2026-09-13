import Cocoa

@objc(LinkSendShareViewController)
final class ShareViewController: NSViewController {
    private let devicePicker = NSPopUpButton(frame: .zero, pullsDown: false)
    private let status = NSTextField(wrappingLabelWithString: "正在读取已配对设备…")
    private let sendButton = NSButton(title: "发送", target: nil, action: nil)
    private let cancelButton = NSButton(title: "取消", target: nil, action: nil)
    private var devices: [ShareDevice] = []
    private var store: ShareStore?
    private var requestID = UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased()
    private var finished = false

    override func loadView() {
        view = NSView(frame: NSRect(x: 0, y: 0, width: 420, height: 220))
        let title = NSTextField(labelWithString: "使用 LinkSend 发送")
        title.font = .systemFont(ofSize: 18, weight: .semibold)
        title.frame = NSRect(x: 24, y: 172, width: 372, height: 28)
        devicePicker.frame = NSRect(x: 24, y: 124, width: 372, height: 30)
        status.frame = NSRect(x: 24, y: 66, width: 372, height: 46)
        cancelButton.target = self; cancelButton.action = #selector(cancel)
        cancelButton.frame = NSRect(x: 214, y: 20, width: 86, height: 34)
        sendButton.target = self; sendButton.action = #selector(send)
        sendButton.keyEquivalent = "\r"
        sendButton.isEnabled = false
        sendButton.frame = NSRect(x: 310, y: 20, width: 86, height: 34)
        for control in [title, devicePicker, status, cancelButton, sendButton] { view.addSubview(control) }
        loadDevices()
    }

    private func loadDevices() {
        do {
            let store = try ShareStore()
            let devices = try store.devices()
            self.store = store; self.devices = devices
            devicePicker.removeAllItems()
            for device in devices {
                devicePicker.addItem(withTitle: device.name + (device.reachable ? "" : "（离线，加入队列）"))
            }
            let count = (extensionContext?.inputItems as? [NSExtensionItem] ?? [])
                .flatMap { $0.attachments ?? [] }.count
            status.stringValue = "已选择 \(count) 个文件。选择设备后发送，离线设备会进入待发送队列。"
            sendButton.isEnabled = count > 0
        } catch {
            finish(error: error)
        }
    }

    @objc private func cancel() {
        finish(error: NSError(domain: NSCocoaErrorDomain, code: NSUserCancelledError,
            userInfo: [NSLocalizedDescriptionKey: "用户取消共享"]))
    }

    @objc private func send() {
        guard !finished, let store, devices.indices.contains(devicePicker.indexOfSelectedItem) else {
            finish(error: ShareStoreError.invalidSelection); return
        }
        let providers = (extensionContext?.inputItems as? [NSExtensionItem] ?? [])
            .flatMap { $0.attachments ?? [] }
        let peer = devices[devicePicker.indexOfSelectedItem]
        sendButton.isEnabled = false; cancelButton.isEnabled = false; devicePicker.isEnabled = false
        status.stringValue = "正在安全接管文件授权…"
        store.prepare(providers, requestID: requestID) { [weak self] result in
            guard let self else { return }
            do {
                let (paths, bookmarks) = try result.get()
                try store.persist(requestID: requestID, peerID: peer.id, paths: paths, bookmarks: bookmarks)
                finished = true
                status.stringValue = "正在通知 LinkSend 后台…"
                var completionIssued = false
                let complete: (Bool) -> Void = { [weak self] opened in
                    DispatchQueue.main.async {
                        guard let self, !completionIssued else { return }
                        completionIssued = true
                        self.status.stringValue = opened ? "已交给 LinkSend 后台。" : "请求已保存；下次打开 LinkSend 时继续。"
                        self.extensionContext?.completeRequest(returningItems: nil)
                    }
                }
                if let wake = URL(string: "linksend-share://handoff/" + requestID) {
                    extensionContext?.open(wake, completionHandler: complete)
                    DispatchQueue.main.asyncAfter(deadline: .now() + 2) { complete(false) }
                } else {
                    complete(false)
                }
            } catch {
                store.cleanup(requestID: requestID)
                finish(error: error)
            }
        }
    }

    private func finish(error: Error) {
        guard !finished else { return }
        finished = true
        status.stringValue = error.localizedDescription
        extensionContext?.cancelRequest(withError: error)
    }
}
