import Foundation
import Darwin

@main
enum FixtureIOTests {
    static func expectFailure(_ name: String, _ action: () throws -> Void) throws {
        do { try action() }
        catch { print("PASS " + name); return }
        throw NSError(domain: "EXPECTED_FAILURE_" + name, code: 1)
    }

    static func main() throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("linksend-e0-safeio-" + UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: false)
        // This UUID directory was exclusively created above and contains only
        // these disposable fixtures, never the user's profile or receive folder.
        defer { try? FileManager.default.removeItem(at: root) }
        let file = root.appendingPathComponent("source")
        let exact = Data(repeating: 0x61, count: FixtureIO.payloadLimit)
        try exact.write(to: file, options: .withoutOverwriting)
        let exactRead = try FixtureIO.readBounded(file, limit: FixtureIO.payloadLimit)
        precondition(exactRead == exact)
        print("PASS exact_payload_limit")
        let append = try FileHandle(forWritingTo: file)
        try append.seekToEnd(); try append.write(contentsOf: Data([0x62])); try append.close()
        try expectFailure("grown_payload_rejected") { _ = try FixtureIO.readBounded(file, limit: FixtureIO.payloadLimit) }
        try expectFailure("oversized_receipt_rejected") { _ = try FixtureIO.readBounded(file, limit: FixtureIO.receiptLimit) }
        let link = root.appendingPathComponent("link")
        try FileManager.default.createSymbolicLink(at: link, withDestinationURL: file)
        try expectFailure("symlink_rejected") { _ = try FixtureIO.readBounded(link, limit: FixtureIO.payloadLimit) }
        let fifo = root.appendingPathComponent("fifo")
        precondition(mkfifo(fifo.path, 0o600) == 0)
        try expectFailure("fifo_rejected_without_blocking") { _ = try FixtureIO.readBounded(fifo, limit: 1) }

        let requestID = UUID().uuidString
        let a = try OwnedFixtureRequest(container: root, request: requestID)
        let b = try OwnedFixtureRequest(container: root, request: UUID().uuidString)
        try a.write("fixture.payload", data: Data("partial".utf8))
        try b.write("fixture.payload", data: Data("other request".utf8))
        let foreign = a.directory.appendingPathComponent("foreign.txt")
        try Data("preserve".utf8).write(to: foreign, options: .withoutOverwriting)
        try expectFailure("duplicate_request_rejected") { _ = try OwnedFixtureRequest(container: root, request: requestID) }
        try expectFailure("duplicate_file_rejected") { try a.write("fixture.payload", data: Data()) }
        precondition(a.cleanupUnpublished().isEmpty)
        precondition(!FileManager.default.fileExists(atPath: a.directory.appendingPathComponent("fixture.payload").path))
        precondition(FileManager.default.fileExists(atPath: foreign.path))
        precondition(FileManager.default.fileExists(atPath: b.directory.appendingPathComponent("fixture.payload").path))
        print("PASS cleanup_preserves_foreign_and_other_request")

        let original = b.directory.appendingPathComponent("fixture.payload")
        try FileManager.default.moveItem(at: original, to: b.directory.appendingPathComponent("moved-original"))
        try Data("replacement".utf8).write(to: original, options: .withoutOverwriting)
        precondition(b.cleanupUnpublished() == ["fixture.payload"])
        let replacement = try FixtureIO.readBounded(original, limit: 100)
        precondition(replacement == Data("replacement".utf8))
        print("PASS cleanup_preserves_replacement_identity")
        try expectFailure("path_name_rejected") { try b.write("../escape", data: Data()) }
    }
}
