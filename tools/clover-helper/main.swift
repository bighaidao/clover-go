import Foundation
import PDFKit
import AppKit
import AVFoundation
import CoreServices
import CoreGraphics
import UniformTypeIdentifiers

// CloverGo 原生能力 helper。
// - 预编译：swiftc -O -o build/clover-helper tools/clover-helper/main.swift
//   （make bundle 自动执行，二进制随 .app 分发，运行时无需 Xcode CLT）
// - 解释执行（开发/测试回退）：swift tools/clover-helper/main.swift <cmd> ...
//
// 约定：结果走 stdout；失败信息走 stderr + 非零退出；水印单文件失败输出
// CLOVER_SKIP 行，由 Go 调用方统计上报。

func fail(_ msg: String, _ code: Int32 = 1) -> Never {
    fputs(msg + "\n", stderr)
    exit(code)
}

let argv = CommandLine.arguments
guard argv.count >= 2 else { fail("usage: clover-helper <cmd> [args...]") }
let cmd = argv[1]
let rest = Array(argv.dropFirst(2))

func parsePages(_ csv: String) -> [Int] {
    return csv.split(separator: ",").compactMap { Int($0.trimmingCharacters(in: .whitespaces)) }
}

switch cmd {
case "pdf-pagecount":
    guard rest.count == 1, let doc = PDFDocument(url: URL(fileURLWithPath: rest[0])) else {
        fail("cannot open pdf")
    }
    print(doc.pageCount)

case "pdf-extract":
    guard rest.count == 3, let src = PDFDocument(url: URL(fileURLWithPath: rest[0])) else {
        fail("cannot open src pdf")
    }
    let out = PDFDocument()
    for p in parsePages(rest[2]) where p >= 1 && p <= src.pageCount {
        if let page = src.page(at: p - 1) {
            out.insert(page, at: out.pageCount)
        }
    }
    guard out.write(to: URL(fileURLWithPath: rest[1])) else {
        fail("failed to write output pdf", 2)
    }

case "pdf-split":
    guard rest.count == 2, let src = PDFDocument(url: URL(fileURLWithPath: rest[0])) else {
        fail("cannot open src pdf")
    }
    let outDir = rest[1]
    let baseName = URL(fileURLWithPath: rest[0]).deletingPathExtension().lastPathComponent
    var failed = 0
    for i in 0..<src.pageCount {
        let out = PDFDocument()
        if let page = src.page(at: i) {
            out.insert(page, at: 0)
            let pageNum = String(format: "%03d", i + 1)
            let outPath = "\(outDir)/\(baseName)_page_\(pageNum).pdf"
            if !out.write(to: URL(fileURLWithPath: outPath)) {
                failed += 1
            }
        }
    }
    if failed > 0 {
        fail("failed to write \(failed) page file(s)", 2)
    }

case "pdf-export-images":
    guard rest.count == 5, let src = PDFDocument(url: URL(fileURLWithPath: rest[0])) else {
        fail("cannot open src pdf")
    }
    let outDir = rest[1]
    let isJpg = rest[2].lowercased() == "jpg" || rest[2].lowercased() == "jpeg"
    let dpi = Double(rest[3]) ?? 150
    let pages = parsePages(rest[4])
    let baseName = URL(fileURLWithPath: rest[0]).deletingPathExtension().lastPathComponent
    let scale = CGFloat(dpi) / 72.0

    for p in pages {
        guard p >= 1 && p <= src.pageCount, let page = src.page(at: p - 1) else { continue }
        let rect = page.bounds(for: .mediaBox)
        let w = max(1, Int(rect.width * scale))
        let h = max(1, Int(rect.height * scale))

        guard let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: w, pixelsHigh: h, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0) else { continue }

        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        let ctx = NSGraphicsContext.current?.cgContext
        ctx?.setFillColor(CGColor.white)
        ctx?.fill(CGRect(x: 0, y: 0, width: w, height: h))
        ctx?.scaleBy(x: scale, y: scale)
        page.draw(with: .mediaBox, to: ctx!)
        NSGraphicsContext.restoreGraphicsState()

        let pageNum = String(format: "%03d", p)
        let ext = isJpg ? "jpg" : "png"
        let properties: [NSBitmapImageRep.PropertyKey: Any] = isJpg
            ? [.compressionFactor: 0.85]
            : [:]
        let type: NSBitmapImageRep.FileType = isJpg ? .jpeg : .png
        if let data = rep.representation(using: type, properties: properties) {
            let outUrl = URL(fileURLWithPath: "\(outDir)/\(baseName)_page_\(pageNum).\(ext)")
            try? data.write(to: outUrl)
        }
    }

case "make-test-pdf":
    // 测试辅助：生成 N 页的纯色测试 PDF（main.swift 自身能力，避免 Go 侧再拼 Swift 源码）。
    guard rest.count == 2, let pages = Int(rest[1]), pages > 0 else { fail("make-test-pdf: bad args") }
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: 100, pixelsHigh: 100, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    let img = NSImage(size: NSSize(width: 100, height: 100))
    img.addRepresentation(rep)
    let doc = PDFDocument()
    for _ in 0..<pages {
        if let p = PDFPage(image: img) {
            doc.insert(p, at: doc.pageCount)
        }
    }
    guard doc.write(to: URL(fileURLWithPath: rest[0])) else { fail("failed to write test pdf", 2) }

case "make-test-image":
    // 测试辅助：生成纯色测试图片（jpg/png）。
    guard rest.count == 4, let w = Int(rest[1]), let h = Int(rest[2]), w > 0, h > 0 else { fail("make-test-image: bad args") }
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: w, pixelsHigh: h, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    let img = NSImage(size: NSSize(width: w, height: h))
    img.addRepresentation(rep)
    guard let tiff = img.tiffRepresentation, let r = NSBitmapImageRep(data: tiff) else { fail("make-test-image: rep failed") }
    let type: NSBitmapImageRep.FileType = rest[3].lowercased() == "png" ? .png : .jpeg
    guard let data = r.representation(using: type, properties: [:]) else { fail("make-test-image: encode failed") }
    do {
        try data.write(to: URL(fileURLWithPath: rest[0]))
    } catch {
        fail("make-test-image: write failed")
    }

case "pdf-merge":
    guard rest.count >= 2 else { fail("no pdfs to merge") }
    let dst = rest[0]
    let sources = Array(rest.dropFirst())
    let out = PDFDocument()
    for path in sources {
        if let src = PDFDocument(url: URL(fileURLWithPath: path)) {
            for i in 0..<src.pageCount {
                if let page = src.page(at: i) {
                    out.insert(page, at: out.pageCount)
                }
            }
        }
    }
    guard out.write(to: URL(fileURLWithPath: dst)) else {
        fail("failed to write merged pdf", 2)
    }

case "watermark":
    // argv: text tile opacity angle fontSize r g b extsCSV files...
    guard rest.count >= 10 else { fail("watermark: missing args") }
    let watermarkText = rest[0]
    let isTile = rest[1] == "1"
    let opacity = CGFloat(Double(rest[2]) ?? 0.35)
    let angleDeg = CGFloat(Double(rest[3]) ?? -30)
    let fontSize = CGFloat(Double(rest[4]) ?? 24)
    let r = CGFloat(Double(rest[5]) ?? 0.5)
    let g = CGFloat(Double(rest[6]) ?? 0.5)
    let b = CGFloat(Double(rest[7]) ?? 0.5)
    let targetExts = rest[8].split(separator: ",").map(String.init)
    let filePaths = Array(rest.dropFirst(9))
    guard filePaths.count == targetExts.count else { fail("watermark: ext/file count mismatch") }

    let font = NSFont.systemFont(ofSize: fontSize, weight: .bold)
    let color = NSColor(calibratedRed: r, green: g, blue: b, alpha: opacity)
    let attrs: [NSAttributedString.Key: Any] = [
        .font: font,
        .foregroundColor: color
    ]
    let attrStr = NSAttributedString(string: watermarkText, attributes: attrs)
    let strSize = attrStr.size()
    let fm = FileManager.default

    for (idx, path) in filePaths.enumerated() {
        let targetExt = targetExts[idx]
        guard let img = NSImage(contentsOfFile: path) else {
            print("CLOVER_SKIP"); continue
        }
        let imgSize = img.size
        guard imgSize.width > 0, imgSize.height > 0 else {
            print("CLOVER_SKIP"); continue
        }

        let outImg = NSImage(size: imgSize)
        outImg.lockFocus()
        img.draw(at: .zero, from: NSRect(origin: .zero, size: imgSize), operation: .sourceOver, fraction: 1.0)
        let ctx = NSGraphicsContext.current?.cgContext
        drawWatermarkLayer(ctx, attrStr, strSize, imgSize, isTile, angleDeg)
        outImg.unlockFocus()

        let dir = URL(fileURLWithPath: path).deletingLastPathComponent().path
        let base = URL(fileURLWithPath: path).deletingPathExtension().lastPathComponent
        var outPath = "\(dir)/\(base)_watermarked.\(targetExt)"
        var seq = 2
        while fm.fileExists(atPath: outPath) {
            outPath = "\(dir)/\(base)_watermarked-\(seq).\(targetExt)"
            seq += 1
        }

        guard let tiff = outImg.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff) else {
            print("CLOVER_SKIP"); continue
        }
        var data: Data? = nil
        if targetExt == "png" {
            data = rep.representation(using: .png, properties: [:])
        } else {
            data = rep.representation(using: .jpeg, properties: [.compressionFactor: 0.90])
        }
        if data == nil {
            print("CLOVER_SKIP"); continue
        }
        do {
            try data!.write(to: URL(fileURLWithPath: outPath))
        } catch {
            print("CLOVER_SKIP"); continue
        }
    }

case "finder-favs":
    guard rest.count == 1,
          let data = try? Data(contentsOf: URL(fileURLWithPath: rest[0])),
          let unarchived = try? NSKeyedUnarchiver.unarchiveTopLevelObjectWithData(data) as? NSDictionary,
          let items = unarchived["items"] as? [NSObject] else {
        fail("cannot read sfl3")
    }
    for item in items {
        if let bookmarkData = item.value(forKey: "Bookmark") as? Data {
            var isStale = false
            if let url = try? URL(resolvingBookmarkData: bookmarkData, options: .withoutUI, relativeTo: nil, bookmarkDataIsStale: &isStale) {
                let p = url.path
                if !p.isEmpty {
                    print("\(url.lastPathComponent)\t\(p)")
                }
            }
        }
    }

case "video-thumb":
    guard rest.count >= 2 else { fail("video-thumb: bad args") }
    let srcUrl = URL(fileURLWithPath: rest[0])
    let outUrl = URL(fileURLWithPath: rest[1])
    let maxPx = rest.count >= 3 ? (Double(rest[2]) ?? 512) : 512
    let asset = AVURLAsset(url: srcUrl)
    let gen = AVAssetImageGenerator(asset: asset)
    gen.appliesPreferredTrackTransform = true
    gen.maximumSize = CGSize(width: maxPx, height: maxPx)
    let time = CMTime(seconds: 1.0, preferredTimescale: 600)
    var extractedCG: CGImage? = nil
    if let cg = try? gen.copyCGImage(at: time, actualTime: nil) {
        extractedCG = cg
    } else if let cg = try? gen.copyCGImage(at: .zero, actualTime: nil) {
        extractedCG = cg
    }
    guard let cgImage = extractedCG else {
        fail("video-thumb: extract frame failed")
    }
    let rep = NSBitmapImageRep(cgImage: cgImage)
    guard let pngData = rep.representation(using: .png, properties: [:]) else {
        fail("video-thumb: encode png failed")
    }
    do {
        try pngData.write(to: outUrl)
    } catch {
        fail("video-thumb: write failed")
    }

case "finder-takeover":
    let mode = rest.first ?? "status"
    let cloverID = "com.clover.app"
    let finderID = "com.apple.finder"

    func runDefaults(_ args: [String]) {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/defaults")
        task.arguments = args
        try? task.run()
        task.waitUntilExit()
    }

    switch mode {
    case "enable":
        var appPath = "/Applications/CloverGo.app"
        if rest.count >= 2 && rest[1].hasSuffix(".app") {
            appPath = rest[1]
        }
        let lsreg = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
        if FileManager.default.isExecutableFile(atPath: lsreg) {
            let task = Process()
            task.executableURL = URL(fileURLWithPath: lsreg)
            task.arguments = ["-f", appPath]
            try? task.run()
            task.waitUntilExit()
        }

        let appURL = URL(fileURLWithPath: appPath)
        let types: [UTType] = [.folder, .directory, .volume]
        let group = DispatchGroup()
        for type in types {
            group.enter()
            NSWorkspace.shared.setDefaultApplication(at: appURL, toOpen: type) { _ in
                group.leave()
            }
            _ = LSSetDefaultRoleHandlerForContentType(type.identifier as CFString, .viewer, cloverID as CFString)
            _ = LSSetDefaultRoleHandlerForContentType(type.identifier as CFString, .all, cloverID as CFString)
        }
        _ = group.wait(timeout: .now() + 1.5)

        runDefaults(["write", "-g", "NSFileViewer", "-string", cloverID])
        print("ENABLED")

    case "disable":
        runDefaults(["delete", "-g", "NSFileViewer"])
        let finderURL = URL(fileURLWithPath: "/System/Library/CoreServices/Finder.app")
        let types: [UTType] = [.folder, .directory, .volume]
        let group = DispatchGroup()
        for type in types {
            group.enter()
            NSWorkspace.shared.setDefaultApplication(at: finderURL, toOpen: type) { _ in
                group.leave()
            }
            _ = LSSetDefaultRoleHandlerForContentType(type.identifier as CFString, .viewer, finderID as CFString)
            _ = LSSetDefaultRoleHandlerForContentType(type.identifier as CFString, .all, finderID as CFString)
        }
        _ = group.wait(timeout: .now() + 1.5)
        print("DISABLED")

    case "status":
        let global = UserDefaults.standard.persistentDomain(forName: UserDefaults.globalDomain)
        let viewer = (global?["NSFileViewer"] as? String) ?? ""
        let handler = (LSCopyDefaultRoleHandlerForContentType("public.folder" as CFString, .viewer)?.takeRetainedValue() as String?) ?? ""
        if viewer.caseInsensitiveCompare(cloverID) == .orderedSame {
            print("ENABLED\t\(viewer)\t\(handler)")
        } else {
            print("DISABLED\t\(viewer)\t\(handler)")
        }

    default:
        fail("finder-takeover: unknown mode \(mode)")
    }

case "finder-watch":
    let parentPID: pid_t = rest.first.flatMap { Int32($0) } ?? 0
    let app = NSApplication.shared
    app.setActivationPolicy(.prohibited)
    let watcher = FinderRedirectDaemon(parentPID: parentPID)
    app.delegate = watcher
    app.run()

case "finder-reveal":
    // Select a path in the real Finder, bypassing NSFileViewer.
    //
    // NSWorkspace.activateFileViewerSelectingURLs: resolves the file viewer
    // through NSFileViewer, so once CloverGo has taken over it comes straight
    // back to CloverGo. Talking to Finder.app directly with an Apple Event
    // skips that lookup entirely.
    //
    // CloverGoWatch would otherwise treat this Finder window as a Dock click
    // and close it, so it is told to stand aside first.
    guard rest.count >= 1, !rest[0].isEmpty else {
        fail("finder-reveal: missing path")
    }
    let target = URL(fileURLWithPath: rest[0]).standardizedFileURL
    guard FileManager.default.fileExists(atPath: target.path) else {
        fail("finder-reveal: no such path \(target.path)")
    }

    DistributedNotificationCenter.default().postNotificationName(
        Notification.Name("com.clover.app.finderPassThrough"),
        object: nil,
        userInfo: nil,
        deliverImmediately: true
    )

    let source = """
    tell application "Finder"
      activate
      reveal (POSIX file "\(target.path)" as alias)
    end tell
    """
    var error: NSDictionary?
    guard let script = NSAppleScript(source: source) else {
        fail("finder-reveal: cannot compile script")
    }
    _ = script.executeAndReturnError(&error)
    if let error {
        let message = (error[NSAppleScript.errorMessage] as? String) ?? "\(error)"
        fail("finder-reveal: \(message)")
    }
    print("OK")

case "launch-at-login":
    let mode = rest.first ?? "status"
    let appPath = rest.count >= 2 ? rest[1] : "/Applications/CloverGo.app"
    let label = "com.clover.app.loginitem"

    func plistURL() -> URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/LaunchAgents")
            .appendingPathComponent("\(label).plist")
    }

    func runLaunchctl(_ args: [String]) {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        task.arguments = args
        task.standardOutput = FileHandle.nullDevice
        task.standardError = FileHandle.nullDevice
        try? task.run()
        task.waitUntilExit()
    }

    // A LaunchAgent is used rather than SMAppService.mainApp because this
    // helper runs as its own executable: SMAppService.mainApp registers the
    // bundle of the calling process, which here is the helper, not CloverGo.
    // The agent works on every macOS version CloverGo supports and points
    // straight at the app bundle.
    switch mode {
    case "enable":
        let url = plistURL()
        try? FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        let plist: [String: Any] = [
            "Label": label,
            "ProgramArguments": [appPath],
            "RunAtLoad": true,
            "KeepAlive": false,
            "ProcessType": "Interactive",
        ]
        guard let data = try? PropertyListSerialization.data(
            fromPropertyList: plist, format: .xml, options: 0
        ), (try? data.write(to: url, options: .atomic)) != nil else {
            fail("launch-at-login: cannot write \(url.path)")
        }
        runLaunchctl(["unload", url.path])
        runLaunchctl(["load", url.path])
        print("ENABLED")

    case "disable":
        let url = plistURL()
        runLaunchctl(["unload", url.path])
        try? FileManager.default.removeItem(at: url)
        print("DISABLED")

    case "status":
        let enabled = FileManager.default.fileExists(atPath: plistURL().path)
        print(enabled ? "ENABLED" : "DISABLED")

    default:
        fail("launch-at-login: unknown mode \(mode)")
    }

default:
    fail("unknown command: \(cmd)")
}

func drawWatermarkLayer(_ ctx: CGContext?, _ attrStr: NSAttributedString, _ strSize: CGSize, _ imgSize: CGSize, _ isTile: Bool, _ angleDeg: CGFloat) {
    if isTile {
        let stepX = max(180, strSize.width * 1.5)
        let stepY = max(120, strSize.height * 4.0)
        var y: CGFloat = -imgSize.height * 0.2
        while y < imgSize.height * 1.2 {
            var x: CGFloat = -imgSize.width * 0.2
            while x < imgSize.width * 1.2 {
                ctx?.saveGState()
                ctx?.translateBy(x: x, y: y)
                ctx?.rotate(by: angleDeg * .pi / 180.0)
                attrStr.draw(at: NSPoint(x: -strSize.width / 2, y: -strSize.height / 2))
                ctx?.restoreGState()
                x += stepX
            }
            y += stepY
        }
    } else {
        ctx?.saveGState()
        ctx?.translateBy(x: imgSize.width / 2, y: imgSize.height / 2)
        ctx?.rotate(by: angleDeg * .pi / 180.0)
        attrStr.draw(at: NSPoint(x: -strSize.width / 2, y: -strSize.height / 2))
        ctx?.restoreGState()
    }
}

// MARK: - NewFinder-style Finder Interception Daemon

final class FinderRedirectDaemon: NSObject, NSApplicationDelegate {
    private let parentPID: pid_t
    private let cloverBundleID = "com.clover.app"
    private let finderBundleID = "com.apple.finder"
    private var isRedirectingFinder = false
    private var lastFinderRedirectAt: Date?
    private var pollTimer: Timer?
    private var pendingWorkItem: DispatchWorkItem?
    private var didWarnAutomation = false
    /// True while CloverGo itself asked for the real Finder (the "在访达中显示"
    /// menu entry). The redirect stays suspended for the whole Finder session
    /// it opens, not for a fixed number of seconds: a timer would close the
    /// window out from under a user who is still browsing it.
    private var finderPassThroughActive = false
    /// When the pass-through started, so a session that never ends can still
    /// be recovered from if the user leaves Finder open forever.
    private var finderPassThroughStartedAt: Date?
    /// Finder's window count when the pass-through began. The session ends
    /// once the user has closed those windows again.
    private var finderWindowBaseline = 0
    /// True until Finder has actually drawn the window the reveal asked for.
    /// The baseline cannot be taken before that: at reveal time Finder may
    /// still have no windows, and an empty count would end the session at once.
    private var finderPassThroughAwaitingWindow = false

    init(parentPID: pid_t) {
        self.parentPID = parentPID
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let center = NSWorkspace.shared.notificationCenter
        center.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] note in
            self?.handleWorkspaceAppNote(note)
        }
        center.addObserver(
            forName: NSWorkspace.didLaunchApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] note in
            self?.handleWorkspaceAppNote(note)
        }
        center.addObserver(
            forName: NSWorkspace.didUnhideApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] note in
            self?.handleWorkspaceAppNote(note)
        }
        // CloverGo's own "在访达中显示" entry asks for the real Finder. The
        // distributed notification tells this daemon to stand aside so the
        // window it opens is not closed again a frame later.
        DistributedNotificationCenter.default().addObserver(
            self,
            selector: #selector(handleFinderPassThrough(_:)),
            name: Notification.Name("com.clover.app.finderPassThrough"),
            object: nil
        )

        let timer = Timer(timeInterval: 0.12, repeats: true) { [weak self] _ in
            self?.tick()
        }
        RunLoop.main.add(timer, forMode: .common)
        self.pollTimer = timer
    }

    @objc private func handleFinderPassThrough(_ note: Notification) {
        finderPassThroughActive = true
        finderPassThroughStartedAt = Date()
        // The baseline cannot be taken here: Finder usually has no window yet
        // at the moment the reveal is requested, and a zero baseline would end
        // the session on the very next tick, before the window is on screen.
        finderPassThroughAwaitingWindow = true
        finderWindowBaseline = 0
        // Drop anything already queued, so a redirect scheduled a moment ago
        // does not close the window CloverGo just asked for.
        pendingWorkItem?.cancel()
        pendingWorkItem = nil
    }

    /// True while CloverGo's own reveal is meant to stay visible in Finder.
    ///
    /// The session ends when the user closes the Finder windows the reveal
    /// opened, or after a generous safety timeout in case they leave Finder
    /// open for good. Until then, Dock clicks on Finder are left alone: the
    /// user is deliberately using Finder.
    private func isPassThroughActive() -> Bool {
        guard finderPassThroughActive else { return false }

        let openWindows = finderBrowserWindowCount()

        if finderPassThroughAwaitingWindow {
            // Wait for Finder to draw the window the reveal asked for. Until
            // it does, stay suspended rather than redirecting an empty Finder.
            if openWindows > 0 {
                finderPassThroughAwaitingWindow = false
                finderWindowBaseline = 0
                finderPassThroughStartedAt = Date()
            } else if let started = finderPassThroughStartedAt,
                      Date().timeIntervalSince(started) > 30 {
                // Finder never opened a window (the reveal failed, or the user
                // cancelled). Give up so Dock clicks are intercepted again.
                endPassThrough()
                return false
            }
            return true
        }

        if openWindows <= finderWindowBaseline {
            // The user closed the windows the reveal opened.
            endPassThrough()
            return false
        }
        if let started = finderPassThroughStartedAt,
           Date().timeIntervalSince(started) > 600 {
            // Ten minutes without closing them: assume the session is over.
            endPassThrough()
            return false
        }
        return true
    }

    private func endPassThrough() {
        finderPassThroughActive = false
        finderPassThroughAwaitingWindow = false
        finderPassThroughStartedAt = nil
        finderWindowBaseline = 0
    }

    /// Number of Finder browser windows currently on screen, using the same
    /// filter as the redirect check.
    private func finderBrowserWindowCount() -> Int {
        guard let finderPID = NSRunningApplication
            .runningApplications(withBundleIdentifier: finderBundleID)
            .first?
            .processIdentifier else { return 0 }

        guard let info = CGWindowListCopyWindowInfo(
            [.optionOnScreenOnly, .excludeDesktopElements],
            kCGNullWindowID
        ) as? [[String: Any]] else { return 0 }

        let screenH = NSScreen.main?.frame.height ?? 900
        let screenW = NSScreen.main?.frame.width ?? 1400
        var count = 0
        for win in info {
            guard let pid = win[kCGWindowOwnerPID as String] as? pid_t, pid == finderPID else { continue }
            guard (win[kCGWindowLayer as String] as? Int ?? 0) == 0 else { continue }
            guard let bounds = win[kCGWindowBounds as String] as? [String: Any] else { continue }
            let height = (bounds["Height"] as? NSNumber)?.doubleValue ?? 0
            let width = (bounds["Width"] as? NSNumber)?.doubleValue ?? 0
            if height < 120 || width < 160 { continue }
            if height >= screenH - 40 && width >= screenW - 40 { continue }
            count += 1
        }
        return count
    }

    private func tick() {
        // Exit immediately if parent CloverGo process has exited
        if parentPID > 0 && kill(parentPID, 0) != 0 {
            exit(0)
        }
        guard !isRedirectingFinder else { return }
        guard !isPassThroughActive() else { return }
        guard finderLooksLikeItHasBrowserWindows() else { return }
        scheduleFinderRedirect(settle: 0)
    }

    private func handleWorkspaceAppNote(_ note: Notification) {
        guard let app = note.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
              app.bundleIdentifier == finderBundleID else { return }
        guard !isPassThroughActive() else { return }
        scheduleFinderRedirect(settle: 0.04)
    }

    private func scheduleFinderRedirect(settle: TimeInterval) {
        pendingWorkItem?.cancel()
        let work = DispatchWorkItem { [weak self] in
            self?.redirectFinderActivationToClover()
        }
        pendingWorkItem = work
        if settle <= 0 {
            DispatchQueue.main.async(execute: work)
        } else {
            DispatchQueue.main.asyncAfter(deadline: .now() + settle, execute: work)
        }
    }

    private func finderLooksLikeItHasBrowserWindows() -> Bool {
        let finderPID = NSRunningApplication
            .runningApplications(withBundleIdentifier: finderBundleID)
            .first?
            .processIdentifier
        guard let finderPID else { return false }

        guard let info = CGWindowListCopyWindowInfo(
            [.optionOnScreenOnly, .excludeDesktopElements],
            kCGNullWindowID
        ) as? [[String: Any]] else {
            return false
        }

        let screenH = NSScreen.main?.frame.height ?? 900
        let screenW = NSScreen.main?.frame.width ?? 1400

        for win in info {
            guard let pid = win[kCGWindowOwnerPID as String] as? pid_t, pid == finderPID else { continue }
            let layer = win[kCGWindowLayer as String] as? Int ?? 0
            guard layer == 0 else { continue }
            guard let bounds = win[kCGWindowBounds as String] as? [String: Any] else { continue }
            let height = (bounds["Height"] as? NSNumber)?.doubleValue ?? 0
            let width = (bounds["Width"] as? NSNumber)?.doubleValue ?? 0
            if height < 120 || width < 160 { continue }
            if height >= screenH - 40 && width >= screenW - 40 { continue }
            return true
        }
        return false
    }

    private func redirectFinderActivationToClover() {
        guard !isRedirectingFinder else { return }
        if let last = lastFinderRedirectAt, Date().timeIntervalSince(last) < 0.18 {
            return
        }

        let frontIsFinder = NSWorkspace.shared.frontmostApplication?.bundleIdentifier == finderBundleID
        guard frontIsFinder || finderLooksLikeItHasBrowserWindows() else { return }

        isRedirectingFinder = true
        lastFinderRedirectAt = Date()

        forceActivateClover()

        let context = probeFinderContext()
        closeFinderWindowsViaAppleScript()
        applyFinderContext(context)

        if context.selectPaths.isEmpty && context.folderPath == nil {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) { [weak self] in
                guard let self else { return }
                let again = self.probeFinderContext()
                if !again.selectPaths.isEmpty || again.folderPath != nil {
                    self.closeFinderWindowsViaAppleScript()
                    self.applyFinderContext(again)
                    self.forceActivateClover()
                }
                self.isRedirectingFinder = false
            }
        } else {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) { [weak self] in
                self?.isRedirectingFinder = false
            }
        }
    }

    private struct FinderContext {
        var folderPath: String?
        var selectPaths: [String]
    }

    private func runAppleScript(_ source: String) -> (string: String?, error: NSDictionary?) {
        var error: NSDictionary?
        guard let script = NSAppleScript(source: source) else {
            return (nil, ["status": "compile failed"] as NSDictionary)
        }
        let result = script.executeAndReturnError(&error)
        if let error {
            return (result.stringValue, error)
        }
        return (result.stringValue, nil)
    }

    private func probeFinderContext() -> FinderContext {
        let source = """
        tell application "Finder"
          set output to ""
          try
            if (count of Finder windows) > 0 then
              set sel to the selection
              if (count of sel) > 0 then
                repeat with s in sel
                  try
                    set output to output & POSIX path of (s as alias) & linefeed
                  end try
                end repeat
                if output is not "" then return "SEL:" & output
              end if
              set t to target of front Finder window
              return "DIR:" & POSIX path of (t as alias)
            end if
          end try
          return ""
        end tell
        """
        let (raw, _) = runAppleScript(source)
        guard let text = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !text.isEmpty else {
            return FinderContext(folderPath: nil, selectPaths: [])
        }

        if text.hasPrefix("SEL:") {
            let body = String(text.dropFirst(4))
            let paths = body.split(separator: "\n").map {
                String($0).trimmingCharacters(in: .whitespacesAndNewlines)
            }.filter { !$0.isEmpty }
            return FinderContext(folderPath: nil, selectPaths: paths)
        }
        if text.hasPrefix("DIR:") {
            let path = String(text.dropFirst(4)).trimmingCharacters(in: .whitespacesAndNewlines)
            guard !path.isEmpty else { return FinderContext(folderPath: nil, selectPaths: []) }
            return FinderContext(folderPath: path, selectPaths: [])
        }
        return FinderContext(folderPath: nil, selectPaths: [])
    }

    private func closeFinderWindowsViaAppleScript() {
        _ = runAppleScript("""
        tell application "Finder"
          try
            close every window
          end try
        end tell
        """)
    }

    private func applyFinderContext(_ context: FinderContext) {
        if let firstSelect = context.selectPaths.first {
            // Strip trailing slash unless root
            let cleaned = firstSelect.count > 1 && firstSelect.hasSuffix("/")
                ? String(firstSelect.dropLast())
                : firstSelect
            openCloverURL(action: "reveal", path: cleaned)
        } else if let folder = context.folderPath {
            let cleaned = folder.count > 1 && folder.hasSuffix("/")
                ? String(folder.dropLast())
                : folder
            openCloverURL(action: "open", path: cleaned)
        } else {
            forceActivateClover()
        }
    }

    private func openCloverURL(action: String, path: String) {
        var comps = URLComponents()
        comps.scheme = "fclover"
        comps.host = action
        comps.queryItems = [URLQueryItem(name: "path", value: path)]
        guard let url = comps.url else { return }
        let config = NSWorkspace.OpenConfiguration()
        config.activates = true
        NSWorkspace.shared.open(url, configuration: config) { _, _ in }
        forceActivateClover()
    }

    private func forceActivateClover() {
        if let app = NSRunningApplication.runningApplications(withBundleIdentifier: cloverBundleID).first {
            app.activate(options: [.activateIgnoringOtherApps])
        } else if FileManager.default.fileExists(atPath: "/Applications/CloverGo.app") {
            let config = NSWorkspace.OpenConfiguration()
            config.activates = true
            NSWorkspace.shared.openApplication(at: URL(fileURLWithPath: "/Applications/CloverGo.app"), configuration: config, completionHandler: nil)
        }
    }
}
