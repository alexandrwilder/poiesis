import AVFoundation
import AppKit
import CoreImage
import VideoToolbox

/// The camera, the microphone and the recording from one capture session: one owner for the
/// camera (docs/HOST.md). The camera's frames go to the display layer without a copy, but only
/// when the picture has moved and at most at the picture's rate: a still scene draws nothing,
/// so the window server has nothing to do. The same frames feed the mp4 writer while recording.
final class Camera: NSObject, AVCaptureVideoDataOutputSampleBufferDelegate, AVCaptureAudioDataOutputSampleBufferDelegate {
    let session = AVCaptureSession()
    let display = AVSampleBufferDisplayLayer()
    /// Messages for the core; safe from any thread.
    var send: ([String: Any]) -> Void = { _ in }

    private let queue = DispatchQueue(label: "poiesis.camera")
    private let videoOut = AVCaptureVideoDataOutput()
    private let audioOut = AVCaptureAudioDataOutput()
    private var device: AVCaptureDevice?
    private var configured = false
    private var meter: Timer?

    // touched only on queue
    private var pictureWanted = false
    private var pictureFPS = 15
    private var recording = false
    private var file: URL?
    private var writer: AVAssetWriter?
    private var videoIn: AVAssetWriterInput?
    private var audioIn: AVAssetWriterInput?
    private var firstTime = CMTime.invalid
    private var lastTime = CMTime.invalid
    private var requestedAt = CMTime.invalid // frames from before this are not part of the entry
    private var lastShown = CMTime.invalid
    private var lastLuma: [UInt8] = []

    override init() {
        display.videoGravity = .resizeAspectFill
        display.isHidden = true
        super.init()
    }

    // MARK: the picture

    /// On the main queue: show or rest the picture, with the look as a colour matrix.
    func setPicture(on: Bool, matrix: [Double]?, fps: Int) {
        display.filters = on ? Camera.filters(matrix) : nil
        display.isHidden = !on
        if !on { display.flushAndRemoveImage() }
        queue.async {
            self.lastLuma = [] // the first frame after this is always shown
            self.pictureWanted = on
            self.pictureFPS = fps > 0 ? fps : 15
            self.apply()
        }
    }

    /// The look: the core's matrix works on sRGB values, so the picture is taken out of the
    /// linear working space, through the matrix, and back. Nothing for the plain picture.
    static func filters(_ m: [Double]?) -> [CIFilter]? {
        guard let m, m.count == 12 else { return nil }
        let identity: [Double] = [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0]
        if zip(m, identity).allSatisfy({ abs($0 - $1) < 0.001 }) { return nil }
        guard let toSRGB = CIFilter(name: "CILinearToSRGBToneCurve"),
              let matrix = CIFilter(name: "CIColorMatrix"),
              let toLinear = CIFilter(name: "CISRGBToneCurveToLinear") else { return nil }
        matrix.setValue(CIVector(x: m[0], y: m[1], z: m[2], w: 0), forKey: "inputRVector")
        matrix.setValue(CIVector(x: m[4], y: m[5], z: m[6], w: 0), forKey: "inputGVector")
        matrix.setValue(CIVector(x: m[8], y: m[9], z: m[10], w: 0), forKey: "inputBVector")
        matrix.setValue(CIVector(x: 0, y: 0, z: 0, w: 1), forKey: "inputAVector")
        matrix.setValue(CIVector(x: m[3], y: m[7], z: m[11], w: 0), forKey: "inputBiasVector")
        return [toSRGB, matrix, toLinear]
    }

    // MARK: the recording

    func startRecording(to url: URL) {
        queue.async {
            guard !self.recording else { return }
            try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try? FileManager.default.removeItem(at: url)
            self.file = url
            self.recording = true // the writer starts with the first frame, which gives its size
            self.requestedAt = CMClockGetTime(self.session.synchronizationClock ?? CMClockGetHostTimeClock())
            self.apply()
        }
    }

    func stopRecording() {
        queue.async {
            guard self.recording else { return }
            self.recording = false
            let path = self.file?.path ?? ""
            guard let w = self.writer else {
                self.send(["t": "recording", "state": "stopped", "file": path, "seconds": 0])
                self.apply()
                return
            }
            let seconds = CMTimeGetSeconds(CMTimeSubtract(self.lastTime, self.firstTime))
            self.videoIn?.markAsFinished()
            self.audioIn?.markAsFinished()
            w.endSession(atSourceTime: self.lastTime)
            self.writer = nil
            self.videoIn = nil
            self.audioIn = nil
            w.finishWriting {
                if w.status != .completed {
                    self.send(["t": "error", "what": "record", "text": w.error?.localizedDescription ?? "the recording could not be finished"])
                }
                self.send(["t": "recording", "state": "stopped", "file": path, "seconds": seconds.isFinite ? seconds : 0])
            }
            self.apply()
        }
    }

    private func beginWriting(_ sample: CMSampleBuffer, at time: CMTime) {
        guard let url = file else { return }
        do {
            let w = try AVAssetWriter(outputURL: url, fileType: .mp4)
            w.shouldOptimizeForNetworkUse = true // the index at the front
            let video = AVAssetWriterInput(mediaType: .video, outputSettings: [
                AVVideoCodecKey: AVVideoCodecType.h264,
                AVVideoWidthKey: 1280,
                AVVideoHeightKey: 720,
                AVVideoScalingModeKey: AVVideoScalingModeResizeAspectFill,
                AVVideoCompressionPropertiesKey: [
                    AVVideoAverageBitRateKey: 1_000_000,
                    AVVideoExpectedSourceFrameRateKey: 30,
                    AVVideoMaxKeyFrameIntervalKey: 60,
                ],
                AVVideoEncoderSpecificationKey: [
                    kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder as String: true,
                ],
            ])
            video.expectsMediaDataInRealTime = true
            let audio = AVAssetWriterInput(mediaType: .audio, outputSettings: [
                AVFormatIDKey: kAudioFormatMPEG4AAC,
                AVNumberOfChannelsKey: 1,
                AVSampleRateKey: 48000,
                AVEncoderBitRateKey: 128_000,
            ])
            audio.expectsMediaDataInRealTime = true
            guard w.canAdd(video), w.canAdd(audio) else { throw CameraError.writer("the recording could not be set up") }
            w.add(video)
            w.add(audio)
            guard w.startWriting() else { throw w.error ?? CameraError.writer("the recording could not start") }
            w.startSession(atSourceTime: time)
            writer = w
            videoIn = video
            audioIn = audio
            firstTime = time
            lastTime = time
            send(["t": "recording", "state": "started"])
        } catch {
            recording = false
            send(["t": "error", "what": "record", "text": error.localizedDescription])
        }
    }

    func captureOutput(_ output: AVCaptureOutput, didOutput sample: CMSampleBuffer, from connection: AVCaptureConnection) {
        let time = CMSampleBufferGetPresentationTimeStamp(sample)
        if output === videoOut && pictureWanted { showIfMoved(sample, at: time) }
        guard recording else { return }
        if output === videoOut {
            if writer == nil {
                guard CMTimeCompare(time, requestedAt) >= 0 else { return } // a frame from before the start
                beginWriting(sample, at: time)
            }
            guard let v = videoIn, v.isReadyForMoreMediaData else { return }
            if v.append(sample) { lastTime = time }
        } else if output === audioOut {
            guard let a = audioIn, a.isReadyForMoreMediaData, CMTimeCompare(time, firstTime) >= 0 else { return }
            a.append(sample)
        }
    }

    /// Shows a frame when the picture has moved since the last one shown, at most at the
    /// picture's rate: a sparse sample of the brightness plane against the last one, as the
    /// core does in the terminal (frameMoved in tui_record.go).
    private func showIfMoved(_ sample: CMSampleBuffer, at time: CMTime) {
        if lastShown.isValid, CMTimeGetSeconds(CMTimeSubtract(time, lastShown)) < 1.0 / Double(max(pictureFPS, 1)) - 0.005 { return }
        guard let pixels = CMSampleBufferGetImageBuffer(sample) else { return }
        CVPixelBufferLockBaseAddress(pixels, .readOnly)
        defer { CVPixelBufferUnlockBaseAddress(pixels, .readOnly) }
        guard let base = CVPixelBufferGetBaseAddressOfPlane(pixels, 0) else { return }
        let count = CVPixelBufferGetBytesPerRowOfPlane(pixels, 0) * CVPixelBufferGetHeightOfPlane(pixels, 0)
        let bytes = base.assumingMemoryBound(to: UInt8.self)
        var luma = [UInt8]()
        luma.reserveCapacity(count / 97 + 1)
        var i = 0
        while i < count { luma.append(bytes[i]); i += 97 }
        if luma.count == lastLuma.count {
            var sum = 0
            for k in 0..<luma.count { sum += abs(Int(luma[k]) - Int(lastLuma[k])) }
            if sum / max(luma.count, 1) < 3 { return } // still: nothing to draw
        }
        lastLuma = luma
        lastShown = time
        if let list = CMSampleBufferGetSampleAttachmentsArray(sample, createIfNecessary: true), CFArrayGetCount(list) > 0 {
            let attachments = unsafeBitCast(CFArrayGetValueAtIndex(list, 0), to: CFMutableDictionary.self)
            CFDictionarySetValue(attachments, Unmanaged.passUnretained(kCMSampleAttachmentKey_DisplayImmediately).toOpaque(),
                                 Unmanaged.passUnretained(kCFBooleanTrue).toOpaque())
        }
        if display.status == .failed { display.flush() }
        display.enqueue(sample)
    }

    // MARK: the session

    /// On queue: run the camera while the picture shows or something records, always at thirty
    /// frames a second, so a recording starts without the camera changing pace. What reaches the
    /// screen is set by showIfMoved, not by the camera's rate.
    private func apply() {
        let needed = pictureWanted || recording
        if needed && !configured {
            guard authorized() else { return }
            configure()
        }
        guard configured else { return }
        if needed {
            if !session.isRunning { session.startRunning() }
            setRate(30)
            DispatchQueue.main.async { self.startMeter() }
        } else if session.isRunning {
            session.stopRunning()
            DispatchQueue.main.async { self.meter?.invalidate(); self.meter = nil }
        }
    }

    private func authorized() -> Bool {
        for (kind, name) in [(AVMediaType.video, "camera"), (AVMediaType.audio, "microphone")] {
            switch AVCaptureDevice.authorizationStatus(for: kind) {
            case .authorized:
                continue
            case .notDetermined:
                AVCaptureDevice.requestAccess(for: kind) { granted in
                    if granted { self.queue.async { self.apply() } }
                }
                return false
            default:
                let place = name == "camera" ? "Camera" : "Microphone"
                send(["t": "error", "what": name, "text": "the \(name) is not allowed: System Settings › Privacy & Security › \(place)"])
                return false
            }
        }
        return true
    }

    private func configure() {
        session.beginConfiguration()
        session.sessionPreset = .hd1280x720
        if let camera = AVCaptureDevice.default(for: .video), let input = try? AVCaptureDeviceInput(device: camera), session.canAddInput(input) {
            session.addInput(input)
            device = camera
        } else {
            send(["t": "error", "what": "camera", "text": "no camera was found"])
        }
        if let mic = AVCaptureDevice.default(for: .audio), let input = try? AVCaptureDeviceInput(device: mic), session.canAddInput(input) {
            session.addInput(input)
        }
        // the camera's own size and format: the display shows it as it is, and the writer's
        // hardware encoder scales it to 1280x720 on the way into the file
        videoOut.videoSettings = [kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange]
        videoOut.alwaysDiscardsLateVideoFrames = true
        videoOut.setSampleBufferDelegate(self, queue: queue)
        if session.canAddOutput(videoOut) { session.addOutput(videoOut) }
        // one fixed audio format, so a microphone that changes its own cannot garble the file
        audioOut.audioSettings = [
            AVFormatIDKey: kAudioFormatLinearPCM,
            AVSampleRateKey: 48000,
            AVNumberOfChannelsKey: 1,
            AVLinearPCMBitDepthKey: 16,
            AVLinearPCMIsFloatKey: false,
            AVLinearPCMIsNonInterleaved: false,
        ]
        audioOut.setSampleBufferDelegate(self, queue: queue)
        if session.canAddOutput(audioOut) { session.addOutput(audioOut) }
        session.commitConfiguration()
        configured = true
    }

    /// The nearest frame rate the camera's current format offers.
    private func setRate(_ fps: Int) {
        guard let d = device else { return }
        let ranges = d.activeFormat.videoSupportedFrameRateRanges
        guard let range = ranges.first(where: { Double(fps) >= $0.minFrameRate && Double(fps) <= $0.maxFrameRate }) ?? ranges.last else { return }
        let rate = max(range.minFrameRate, min(range.maxFrameRate, Double(fps)))
        let duration = CMTime(value: 1000, timescale: CMTimeScale(rate * 1000))
        guard (try? d.lockForConfiguration()) != nil else { return }
        d.activeVideoMinFrameDuration = duration
        d.activeVideoMaxFrameDuration = duration
        d.unlockForConfiguration()
    }

    /// The microphone level for the meter, ten times a second while the session runs.
    private func startMeter() {
        guard meter == nil else { return }
        meter = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
            guard let self, self.session.isRunning,
                  let channel = self.audioOut.connection(with: .audio)?.audioChannels.first else { return }
            self.send(["t": "level", "db": Double(channel.averagePowerLevel)])
        }
    }
}

enum CameraError: LocalizedError {
    case writer(String)
    var errorDescription: String? {
        switch self { case .writer(let s): return s }
    }
}
