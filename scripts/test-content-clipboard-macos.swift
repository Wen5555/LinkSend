// Creates only a named test pasteboard. The user's general clipboard is untouched.
// Usage: swift test-content-clipboard-macos.swift com.linksend.native-test.UUID png|tiff|clear
import Cocoa
import Foundation

guard CommandLine.arguments.count == 3,
      CommandLine.arguments[1].hasPrefix("com.linksend.native-test.") else {
    fputs("A dedicated com.linksend.native-test.* pasteboard and png|tiff|clear are required.\n", stderr)
    exit(2)
}
let board = NSPasteboard(name: NSPasteboard.Name(CommandLine.arguments[1]))
let action = CommandLine.arguments[2]
if action == "clear" {
    board.clearContents()
    board.releaseGlobally()
    exit(0)
}
guard action == "png" || action == "tiff",
      let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: 2, pixelsHigh: 2,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 8, bitsPerPixel: 32),
      let pixels = bitmap.bitmapData else { exit(2) }
for pixel in 0..<4 {
    pixels[pixel * 4] = 180
    pixels[pixel * 4 + 1] = 60
    pixels[pixel * 4 + 2] = 30
    pixels[pixel * 4 + 3] = 255
}
guard let data = bitmap.representation(using: action == "png" ? .png : .tiff, properties: [:]) else { exit(3) }
board.clearContents()
guard board.setData(data, forType: action == "png" ? .png : .tiff) else { exit(4) }
print("named_pasteboard=\(board.name.rawValue) type=\(action) bytes=\(data.count) general_clipboard_unchanged=true")
