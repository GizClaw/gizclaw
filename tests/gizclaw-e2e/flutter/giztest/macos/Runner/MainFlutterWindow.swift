import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)

    super.awakeFromNib()

    // The hidden harness window never triggers viewWillAppear to start Dart.
    if !flutterViewController.engine.run(withEntrypoint: nil) {
      fputs("giztest: failed to start the Flutter engine\n", stderr)
      exit(1)
    }
  }
}
