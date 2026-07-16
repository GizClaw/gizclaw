import AVFoundation
import Flutter
import UIKit
import WebRTC
import flutter_webrtc

@main
@objc class AppDelegate: FlutterAppDelegate {
  private var pcmAudioLevelStreamHandler: PCMAudioLevelStreamHandler?

  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    GeneratedPluginRegistrant.register(with: self)
    if let controller = window?.rootViewController as? FlutterViewController {
      let handler = PCMAudioLevelStreamHandler()
      let channel = FlutterEventChannel(
        name: "com.gizclaw.opensource/pcm_audio_levels",
        binaryMessenger: controller.binaryMessenger
      )
      channel.setStreamHandler(handler)
      pcmAudioLevelStreamHandler = handler
    }
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }
}

private enum PCMAudioDirection {
  case input
  case output
}

private final class PCMAudioLevelRenderer: NSObject, RTCAudioRenderer {
  private let direction: PCMAudioDirection
  private let onLevel: (PCMAudioDirection, Double) -> Void

  init(
    direction: PCMAudioDirection,
    onLevel: @escaping (PCMAudioDirection, Double) -> Void
  ) {
    self.direction = direction
    self.onLevel = onLevel
  }

  func render(pcmBuffer: AVAudioPCMBuffer) {
    guard
      let channels = pcmBuffer.int16ChannelData,
      pcmBuffer.frameLength > 0
    else {
      onLevel(direction, 0)
      return
    }

    let frameCount = Int(pcmBuffer.frameLength)
    let channelCount = Int(pcmBuffer.format.channelCount)
    var sumSquares = 0.0
    var peak = 0.0
    for channel in 0..<channelCount {
      let samples = channels[channel]
      for frame in 0..<frameCount {
        let sample = Double(samples[frame]) / Double(Int16.max)
        sumSquares += sample * sample
        peak = max(peak, abs(sample))
      }
    }
    let sampleCount = Double(frameCount * channelCount)
    let rms = sqrt(sumSquares / sampleCount)
    onLevel(direction, min(1, max(rms, peak * 0.25)))
  }
}

private final class PCMAudioLevelStreamHandler: NSObject, FlutterStreamHandler {
  private let lock = NSLock()
  private lazy var inputRenderer = PCMAudioLevelRenderer(
    direction: .input,
    onLevel: recordLevel
  )
  private lazy var outputRenderer = PCMAudioLevelRenderer(
    direction: .output,
    onLevel: recordLevel
  )
  private var displayLink: CADisplayLink?
  private var eventSink: FlutterEventSink?
  private var pendingInput = 0.0
  private var pendingOutput = 0.0

  func onListen(
    withArguments arguments: Any?,
    eventSink events: @escaping FlutterEventSink
  ) -> FlutterError? {
    eventSink = events
    let audioManager = AudioManager.sharedInstance()
    audioManager.addLocalAudioRenderer(inputRenderer)
    audioManager.renderPreProcessingAdapter.add(outputRenderer)
    let link = CADisplayLink(target: self, selector: #selector(emitLevels))
    link.add(to: .main, forMode: .common)
    displayLink = link
    return nil
  }

  func onCancel(withArguments arguments: Any?) -> FlutterError? {
    displayLink?.invalidate()
    displayLink = nil
    let audioManager = AudioManager.sharedInstance()
    audioManager.removeLocalAudioRenderer(inputRenderer)
    audioManager.renderPreProcessingAdapter.remove(outputRenderer)
    eventSink = nil
    return nil
  }

  private func recordLevel(direction: PCMAudioDirection, level: Double) {
    lock.lock()
    switch direction {
    case .input:
      pendingInput = max(pendingInput, level)
    case .output:
      pendingOutput = max(pendingOutput, level)
    }
    lock.unlock()
  }

  @objc private func emitLevels() {
    lock.lock()
    let input = pendingInput
    let output = pendingOutput
    pendingInput = 0
    pendingOutput = 0
    lock.unlock()
    eventSink?(["input": input, "output": output])
  }
}
