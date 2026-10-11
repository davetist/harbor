import AppKit
import WebKit

struct RunnerStatus: Decodable {
    let installed: Bool
    let running: Bool
    let url: String?
    let dataDir: String?
}

@main
final class HarborApplication: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate {
    private var window: NSWindow!
    private var web: WKWebView?
    private var statusItem: NSStatusItem!
    private var stateItem: NSMenuItem!
    private var runnerURL: URL!
    private var localURL: URL?
    private var dataURL: URL?
    private var busy = false

    static func main() {
        let app = NSApplication.shared
        let delegate = HarborApplication()
        app.delegate = delegate
        app.setActivationPolicy(.regular)
        app.run()
        withExtendedLifetime(delegate) {}
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        runnerURL = Bundle.main.bundleURL.appendingPathComponent("Contents/Resources/harbor")
        NSApp.appearance = NSAppearance(named: .darkAqua)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1040, height: 740), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "Harbor"
        window.minSize = NSSize(width: 760, height: 560)
        window.backgroundColor = NSColor(calibratedWhite: 0.04, alpha: 1)
        window.isReleasedWhenClosed = false
        window.center()
        makeMenus()
        showWindow()
        openHarbor()
    }

    private func item(_ title: String, _ action: Selector, _ key: String = "") -> NSMenuItem {
        let result = NSMenuItem(title: title, action: action, keyEquivalent: key)
        result.target = self
        return result
    }

    private func makeMenus() {
        let main = NSMenu()
        let appMenu = NSMenu(title: "Harbor")
        appMenu.addItem(item("About Harbor", #selector(about)))
        appMenu.addItem(.separator())
        appMenu.addItem(item("Open Harbor", #selector(openHarbor), "o"))
        appMenu.addItem(item("Open in Browser", #selector(openBrowser)))
        appMenu.addItem(item("Show Ship Data", #selector(showData)))
        appMenu.addItem(item("Show Service Logs", #selector(showLogs)))
        appMenu.addItem(.separator())
        appMenu.addItem(item("Stop Harbor…", #selector(stopHarbor)))
        appMenu.addItem(item("Uninstall Harbor…", #selector(uninstallHarbor)))
        appMenu.addItem(.separator())
        appMenu.addItem(item("Quit Harbor Interface", #selector(quitInterface), "q"))
        let root = NSMenuItem()
        root.submenu = appMenu
        main.addItem(root)
        let file = NSMenu(title: "File")
        file.addItem(item("Close Window", #selector(closeWindow), "w"))
        let fileRoot = NSMenuItem(title: "File", action: nil, keyEquivalent: "")
        fileRoot.submenu = file
        main.addItem(fileRoot)
        // Web forms, the Dojo and native pickers use the standard editing commands.
        let edit = NSMenu(title: "Edit")
        for (title, selector, key) in [("Undo", "undo:", "z"), ("Cut", "cut:", "x"), ("Copy", "copy:", "c"), ("Paste", "paste:", "v"), ("Select All", "selectAll:", "a")] {
            edit.addItem(NSMenuItem(title: title, action: Selector(selector), keyEquivalent: key))
        }
        let editRoot = NSMenuItem(title: "Edit", action: nil, keyEquivalent: "")
        editRoot.submenu = edit
        main.addItem(editRoot)
        NSApp.mainMenu = main
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.button?.title = "Harbor"
        let statusMenu = NSMenu()
        stateItem = NSMenuItem(title: "Harbor is starting", action: nil, keyEquivalent: "")
        statusMenu.addItem(stateItem)
        statusMenu.addItem(item("Open Harbor", #selector(openHarbor)))
        statusMenu.addItem(item("Stop Harbor…", #selector(stopHarbor)))
        statusMenu.addItem(.separator())
        statusMenu.addItem(item("Quit Interface · Ships Keep Running", #selector(quitInterface)))
        statusItem.menu = statusMenu
    }

    private func showWindow() {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    private func message(_ title: String, detail: String, retry: Bool = false) {
        web = nil
        let view = NSView()
        view.wantsLayer = true
        view.layer?.backgroundColor = NSColor(calibratedWhite: 0.04, alpha: 1).cgColor
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .centerX
        stack.spacing = 20
        stack.translatesAutoresizingMaskIntoConstraints = false
        let icon = NSImageView(image: NSApp.applicationIconImage)
        icon.translatesAutoresizingMaskIntoConstraints = false
        icon.widthAnchor.constraint(equalToConstant: 88).isActive = true
        icon.heightAnchor.constraint(equalToConstant: 88).isActive = true
        stack.addArrangedSubview(icon)
        let heading = NSTextField(labelWithString: title)
        heading.font = .systemFont(ofSize: 23, weight: .regular)
        stack.addArrangedSubview(heading)
        let text = NSTextField(wrappingLabelWithString: detail)
        text.alignment = .center
        text.textColor = .secondaryLabelColor
        text.preferredMaxLayoutWidth = 500
        stack.addArrangedSubview(text)
        if busy {
            let spinner = NSProgressIndicator()
            spinner.style = .spinning
            spinner.startAnimation(nil)
            stack.addArrangedSubview(spinner)
        }
        if retry {
            stack.addArrangedSubview(NSButton(title: "Open Harbor", target: self, action: #selector(openHarbor)))
            stack.addArrangedSubview(NSButton(title: "Background App Settings", target: self, action: #selector(backgroundSettings)))
        }
        view.addSubview(stack)
        NSLayoutConstraint.activate([stack.centerXAnchor.constraint(equalTo: view.centerXAnchor), stack.centerYAnchor.constraint(equalTo: view.centerYAnchor), stack.widthAnchor.constraint(lessThanOrEqualToConstant: 540)])
        window.contentView = view
    }

    private func execute(_ arguments: [String]) throws -> Data {
        let process = Process()
        process.executableURL = runnerURL
        process.arguments = arguments
        let output = Pipe()
        let errors = Pipe()
        process.standardOutput = output
        process.standardError = errors
        try process.run()
        // Output from these management commands is small; service logs belong to
        // launchd and never enter the interface or this command's pipe.
        let result = output.fileHandleForReading.readDataToEndOfFile()
        let failure = errors.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        if process.terminationStatus != 0 {
            throw NSError(domain: "Harbor", code: Int(process.terminationStatus), userInfo: [NSLocalizedDescriptionKey: String(data: failure, encoding: .utf8) ?? "Harbor could not complete this action."])
        }
        return result
    }

    private func operation(_ title: String, arguments: [String], completion: @escaping () -> Void) {
        guard !busy else { showWindow(); return }
        busy = true
        showWindow()
        message(title, detail: "Harbor waits for running ship operations to finish safely.")
        DispatchQueue.global(qos: .userInitiated).async {
            do {
                _ = try self.execute(arguments)
                DispatchQueue.main.async { self.busy = false; completion() }
            } catch {
                DispatchQueue.main.async {
                    self.busy = false
                    self.stateItem.title = "Harbor needs attention"
                    self.message("Harbor needs attention", detail: error.localizedDescription, retry: true)
                }
            }
        }
    }

    @objc private func openHarbor() {
        showWindow()
        if let web = web { window.contentView = web; return }
        operation("Starting Harbor", arguments: ["install", "--upgrade"]) {
            self.operation("Opening Harbor", arguments: ["service", "start"]) {
                do {
                    let status = try JSONDecoder().decode(RunnerStatus.self, from: self.execute(["service", "status", "--json"]))
                    guard status.running, let value = status.url, let url = URL(string: value) else { throw NSError(domain: "Harbor", code: 1, userInfo: [NSLocalizedDescriptionKey: "The background runner is not ready."]) }
                    self.localURL = url
                    self.dataURL = status.dataDir.map { URL(fileURLWithPath: $0) }
                    self.stateItem.title = "Harbor is running · Ships stay running when closed"
                    self.loadInterface(url)
                } catch { self.message("Harbor needs attention", detail: error.localizedDescription, retry: true) }
            }
        }
    }

    private func loadInterface(_ url: URL) {
        let config = WKWebViewConfiguration()
        let view = WKWebView(frame: .zero, configuration: config)
        view.navigationDelegate = self
        view.uiDelegate = self
        view.setValue(false, forKey: "drawsBackground")
        web = view
        window.contentView = view
        view.load(URLRequest(url: url))
    }

    private func confirm(_ title: String, detail: String, button: String) -> Bool {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = detail
        alert.addButton(withTitle: button)
        alert.addButton(withTitle: "Cancel")
        return alert.runModal() == .alertFirstButtonReturn
    }

    @objc private func stopHarbor() {
        guard !busy, confirm("Stop Harbor and its ships?", detail: "Ship data is retained. Open Harbor to start it again. Closing the window keeps ships running.", button: "Stop Harbor") else { return }
        operation("Stopping Harbor", arguments: ["service", "stop"]) {
            self.stateItem.title = "Harbor is stopped"
            self.message("Harbor is stopped", detail: "Your ship data is safe. Open Harbor to start it again.", retry: true)
        }
    }

    @objc private func uninstallHarbor() {
        guard !busy, confirm("Uninstall Harbor?", detail: "Harbor and its ships will stop. The background service will be removed and this app moved to the Trash. Ship data, exports and configuration will be retained.", button: "Uninstall Harbor") else { return }
        operation("Removing Harbor", arguments: ["service", "uninstall"]) {
            do { _ = try FileManager.default.trashItem(at: Bundle.main.bundleURL, resultingItemURL: nil); NSApp.terminate(nil) }
            catch { self.message("Background service removed", detail: "Your ship data is retained. Move Harbor to the Trash to finish removing the app.") }
        }
    }

    @objc private func openBrowser() { if let url = localURL { NSWorkspace.shared.open(url) } }
    @objc private func showData() { if let url = dataURL { NSWorkspace.shared.open(url) } }
    @objc private func showLogs() { NSWorkspace.shared.open(URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Library/Logs/Harbor")) }
    @objc private func backgroundSettings() { NSWorkspace.shared.open(URL(string: "x-apple.systempreferences:com.apple.LoginItems-Settings.extension")!) }
    @objc private func quitInterface() { NSApp.terminate(nil) }
    @objc private func closeWindow() { window.performClose(nil) }
    @objc private func about() { NSApp.orderFrontStandardAboutPanel(options: [.version: Bundle.main.object(forInfoDictionaryKey: "HarborReleaseVersion") ?? ""]) }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool { showWindow(); return true }

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = action.request.url else { decisionHandler(.cancel); return }
        if action.targetFrame?.isMainFrame == true, let local = localURL, (url.host != local.host || url.port != local.port || url.scheme != local.scheme), url.scheme != "about" {
            NSWorkspace.shared.open(url)
            decisionHandler(.cancel)
        } else { decisionHandler(action.shouldPerformDownload ? .download : .allow) }
    }
    func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse, decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) { decisionHandler(response.canShowMIMEType ? .allow : .download) }
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? { if let url = action.request.url { NSWorkspace.shared.open(url) }; return nil }
    func webView(_ webView: WKWebView, runOpenPanelWith parameters: WKOpenPanelParameters, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping ([URL]?) -> Void) {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = parameters.allowsMultipleSelection
        panel.canChooseDirectories = parameters.allowsDirectories
        panel.beginSheetModal(for: window) { completionHandler($0 == .OK ? panel.urls : nil) }
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        completionHandler(confirm("Harbor", detail: message, button: "Continue"))
    }
    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let alert = NSAlert()
        alert.messageText = "Harbor"
        alert.informativeText = message
        alert.runModal()
        completionHandler()
    }
    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { download.delegate = self }
    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { download.delegate = self }
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = suggestedFilename
        panel.beginSheetModal(for: window) { completionHandler($0 == .OK ? panel.url : nil) }
    }
}
