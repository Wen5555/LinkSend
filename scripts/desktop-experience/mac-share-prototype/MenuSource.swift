import Cocoa

final class MenuSource: NSObject, NSApplicationDelegate, NSSharingServiceDelegate {
    private var window: NSWindow!
    private var picker: NSSharingServicePicker!
    private var menu: NSMenu!
    private var service: NSSharingService?
    private var serviceResult: Int32 = 2 // No confirmed completion yet.
    private let file = URL(fileURLWithPath: "/Users/wen/linksend-desktop-experience/e0/share-fixture.txt")

    private func schedule(_ seconds: TimeInterval, _ action: @escaping () -> Void) {
        // A menu enters nested event tracking. Main-queue blocks cannot depend
        // on another main-queue block to finish that synchronous tracking call.
        let timer = Timer(timeInterval: seconds, repeats: false) { _ in action() }
        RunLoop.main.add(timer, forMode: .common)
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 420, height: 220),
            styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.title = "LinkSend E0 系统共享测试源"
        let label = NSTextField(wrappingLabelWithString: "仅共享本轮隔离测试文件。先展示系统共享菜单，再独立验证扩展服务激活。")
        label.frame = NSRect(x: 20, y: 80, width: 380, height: 110)
        window.contentView?.addSubview(label)
        window.center(); window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        picker = NSSharingServicePicker(items: [file])
        menu = NSMenu(title: "E0")
        let nativeItem = picker.standardShareMenuItem
        menu.addItem(nativeItem)
        print("NATIVE_SHARE_MENU_ITEM=\(nativeItem.title) ACTION=\(String(describing: nativeItem.action))")
        fflush(stdout)
        if !CommandLine.arguments.contains("--service-only") {
            schedule(1) {
                print("SHOW_NATIVE_SHARE_MENU")
                self.menu.popUp(positioning: nil, at: NSPoint(x: 40, y: 60), in: self.window.contentView)
                print("NATIVE_SHARE_MENU_CLOSED")
            }
            schedule(4) { self.menu.cancelTracking() }
            schedule(5) {
                self.menu.performActionForItem(at: 0)
                print("STANDARD_SHARE_MENU_ACTION_DISPATCHED")
                fflush(stdout)
            }
        }
        if CommandLine.arguments.contains("--service-only") {
            schedule(1) {
                // Separate system-service activation, not a fabricated click in Finder.
                let services = NSSharingService.sharingServices(forItems: [self.file])
                for service in services { print("SYSTEM_SERVICE=\(service.title)") }
                guard let selected = services.first(where: { $0.title == "LinkSend E0" }) else {
                    print("E0_SERVICE_MISSING"); exit(2)
                }
                self.service = selected
                selected.delegate = self
                print("PERFORM_REAL_SERVICE_FROM_NSAPPLICATION")
                selected.perform(withItems: [self.file])
            }
        }
        schedule(30) {
            print("SOURCE_TEST_FINISHED")
            if CommandLine.arguments.contains("--service-only") { exit(self.serviceResult) }
            NSApp.terminate(nil)
        }
    }
    func sharingService(_ sharingService: NSSharingService, didShareItems items: [Any]) {
        serviceResult = 0
        print("SYSTEM_SHARE_COMPLETED")
    }
    func sharingService(_ sharingService: NSSharingService, didFailToShareItems items: [Any], error: Error) {
        serviceResult = 1
        let native = error as NSError
        print("SYSTEM_SHARE_FAILED=\(native.domain)/\(native.code): \(native.localizedDescription)")
    }
}

@main
enum MenuSourceMain {
    static func main() {
        let application = NSApplication.shared
        let delegate = MenuSource()
        application.delegate = delegate
        application.setActivationPolicy(.regular)
        withExtendedLifetime(delegate) { application.run() }
    }
}
