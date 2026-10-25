import Cocoa
import FlutterMacOS
import WebRTC
import flutter_webrtc

class MainFlutterWindow: NSWindow {
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)

    // Concurrent background RPC scenarios use the native CoreAudio backend.
    // Its Peer teardown avoids the voice-processing engine reconfiguration wait.
    guard let webRtc = FlutterWebRTCPlugin.sharedSingleton(),
      webRtc.peerConnectionFactory == nil else {
      fputs("giztest: WebRTC factory must be configured before Dart starts\n", stderr)
      exit(1)
    }
    let factory = RTCPeerConnectionFactory(
      audioDeviceModuleType: .platformDefault,
      bypassVoiceProcessing: false,
      encoderFactory: RTCDefaultVideoEncoderFactory(),
      decoderFactory: RTCDefaultVideoDecoderFactory(),
      audioProcessingModule: nil
    )
    factory.audioDeviceModule.observer = webRtc
    webRtc.peerConnectionFactory = factory


    super.awakeFromNib()

    // The hidden harness window never triggers viewWillAppear to start Dart.
    if !flutterViewController.engine.run(withEntrypoint: nil) {
      fputs("giztest: failed to start the Flutter engine\n", stderr)
      exit(1)
    }
  }
}
