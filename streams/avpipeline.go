package streams

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
)

// videoFrameMsg carries a BGRA frame buffer through the encoder channel
// together with the wall-clock time it was captured at the source.
// Capture-time PTS makes timestamps independent of pipeline jitter & drops.
type videoFrameMsg struct {
	buf         []byte
	captureTime time.Time
}

// OutputPipeline encapsulates a complete astiav encoding+muxing pipeline
// for one output destination (RTMP stream or MP4 file).
type OutputPipeline struct {
	formatCtx     *astiav.FormatContext
	ioCtx         *astiav.IOContext
	videoCodecCtx *astiav.CodecContext
	audioCodecCtx *astiav.CodecContext
	videoStream   *astiav.Stream
	audioStream   *astiav.Stream
	videoFrame    *astiav.Frame  // reusable YUV420P destination frame
	srcFrame      *astiav.Frame  // reusable BGRA source frame
	audioFrame    *astiav.Frame  // reusable FLTP frame for encoder
	encFrame      *astiav.Frame  // conversion target S16->FLTP
	videoPacket   *astiav.Packet // used only from video encode goroutine
	audioPacket   *astiav.Packet // used only from audio goroutine
	swsCtx        *astiav.SoftwareScaleContext
	swrCtx        *astiav.SoftwareResampleContext
	audioFifo     *astiav.AudioFifo
	audioPts      int64
	frameCount    int64

	// Async muxing: encoded packets are queued for a dedicated muxer goroutine
	// that handles the (potentially blocking) RTMP network write.
	packetCh chan *astiav.Packet
	muxDone  chan struct{}
	startTime     time.Time // wall-clock start for PTS calculation
	fps           int
	lastVideoPts  int64 // ensures strictly monotonic PTS
	hasVideo      bool
	hasAudio      bool
	srcWidth      int // input BGRA dimensions (may differ from output)
	srcHeight     int
	width         int // output/encode dimensions
	height        int

	// Async video encoding: frames are queued and encoded in a background goroutine
	videoCh   chan videoFrameMsg // buffered channel of BGRA frames + capture timestamps
	videoDone chan struct{}      // closed when encode goroutine exits
	bufPool   sync.Pool     // pool of []byte buffers to avoid GC pressure
	closed        atomic.Bool // set when Close() is called; guards sends on videoCh
	forceIDR      atomic.Bool // request keyframe on next encoded frame
	headerWritten bool        // true after WriteHeader succeeds; guards WriteTrailer
}

// NewOutputPipeline creates a new encoding+muxing pipeline.
// srcW/srcH are the input BGRA dimensions (e.g. 384x272 native, or 1920x1080 with overlay).
// cfg.Width/Height are the output/encode dimensions (1920x1080).
// swscale handles upscaling + format conversion in one SIMD-optimized pass.
func NewOutputPipeline(url, formatName string, cfg StreamConfig, recordMode string, srcW, srcH int) (*OutputPipeline, error) {
	p := &OutputPipeline{
		srcWidth:  srcW,
		srcHeight: srcH,
		width:     cfg.Width,
		height:    cfg.Height,
	}

	p.hasVideo = recordMode != "audio"
	p.hasAudio = recordMode != "video"

	var err error
	p.formatCtx, err = astiav.AllocOutputFormatContext(nil, formatName, url)
	if err != nil {
		return nil, fmt.Errorf("alloc output format context: %w", err)
	}

	needGlobalHeader := p.formatCtx.OutputFormat().Flags().Has(astiav.IOFormatFlagGlobalheader)

	if p.hasVideo {
		if err := p.initVideoEncoder(cfg, needGlobalHeader); err != nil {
			p.Close()
			return nil, fmt.Errorf("init video encoder: %w", err)
		}
	}

	if p.hasAudio {
		if err := p.initAudioEncoder(needGlobalHeader); err != nil {
			p.Close()
			return nil, fmt.Errorf("init audio encoder: %w", err)
		}
	}

	p.ioCtx, err = astiav.OpenIOContext(url, astiav.NewIOContextFlags(astiav.IOContextFlagWrite), nil, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("open IO context for %s: %w", url, err)
	}
	p.formatCtx.SetPb(p.ioCtx)

	// For MP4: use fragmented format so the file is always readable,
	// even if the recording is interrupted without clean shutdown.
	headerDict := astiav.NewDictionary()
	defer headerDict.Free()
	// No special movflags -- standard MP4 with moov at end.
	// Most players seek fine with moov at end of file.
	if err := p.formatCtx.WriteHeader(headerDict); err != nil {
		p.Close()
		return nil, fmt.Errorf("write header: %w", err)
	}
	p.headerWritten = true

	p.videoPacket = astiav.AllocPacket()
	p.audioPacket = astiav.AllocPacket()
	p.startTime = time.Now()
	p.fps = cfg.FPS

	// Start dedicated muxer goroutine (decouples encoding from network I/O)
	p.packetCh = make(chan *astiav.Packet, 32) // ~0.6s buffer @ 50fps
	p.muxDone = make(chan struct{})
	go p.muxLoop()

	// Start async video encoder goroutine
	if p.hasVideo {
		bufSize := p.srcWidth * p.srcHeight * 4
		p.bufPool = sync.Pool{New: func() any { return make([]byte, bufSize) }}
		p.videoCh = make(chan videoFrameMsg, 8) // buffer up to 8 frames
		p.videoDone = make(chan struct{})
		go p.videoEncodeLoop()
	}

	return p, nil
}

func (p *OutputPipeline) initVideoEncoder(cfg StreamConfig, globalHeader bool) error {
	// Try hardware encoders first (NVENC > QSV > AMF > x264 fallback)
	var codec *astiav.Codec
	encoderName := ""
	for _, name := range []string{"h264_nvenc", "h264_qsv", "h264_amf", "libx264"} {
		c := astiav.FindEncoderByName(name)
		if c != nil {
			codec = c
			encoderName = name
			break
		}
	}
	if codec == nil {
		return fmt.Errorf("no H.264 encoder found")
	}
	fmt.Printf("Using video encoder: %s\n", encoderName)

	p.videoCodecCtx = astiav.AllocCodecContext(codec)
	if p.videoCodecCtx == nil {
		return fmt.Errorf("failed to alloc video codec context")
	}

	p.videoCodecCtx.SetWidth(cfg.Width)
	p.videoCodecCtx.SetHeight(cfg.Height)
	p.videoCodecCtx.SetPixelFormat(astiav.PixelFormatYuv420P)
	p.videoCodecCtx.SetTimeBase(astiav.NewRational(1, cfg.FPS))
	p.videoCodecCtx.SetFramerate(astiav.NewRational(cfg.FPS, 1))
	p.videoCodecCtx.SetBitRate(parseBitrate(cfg.Bitrate))
	p.videoCodecCtx.SetRateControlMaxRate(parseBitrate(cfg.Bitrate))
	p.videoCodecCtx.SetRateControlBufferSize(9000000)
	p.videoCodecCtx.SetGopSize(cfg.FPS * 2)
	p.videoCodecCtx.SetMaxBFrames(0)

	// Threading only relevant for software encoders
	if encoderName == "libx264" {
		p.videoCodecCtx.SetThreadCount(0)
		p.videoCodecCtx.SetThreadType(astiav.ThreadTypeSlice)
	}

	if globalHeader {
		p.videoCodecCtx.SetFlags(p.videoCodecCtx.Flags().Add(astiav.CodecContextFlagGlobalHeader))
	}

	dict := astiav.NewDictionary()
	defer dict.Free()
	switch encoderName {
	case "h264_nvenc":
		// NVIDIA NVENC: low-latency CBR streaming preset
		dict.Set("preset", "p4", astiav.NewDictionaryFlags()) // p1=fastest .. p7=slowest
		dict.Set("tune", "ll", astiav.NewDictionaryFlags())   // low-latency
		dict.Set("rc", "cbr", astiav.NewDictionaryFlags())
		dict.Set("zerolatency", "1", astiav.NewDictionaryFlags())
		dict.Set("delay", "0", astiav.NewDictionaryFlags())
		dict.Set("profile", "main", astiav.NewDictionaryFlags())
	case "h264_qsv":
		dict.Set("preset", "veryfast", astiav.NewDictionaryFlags())
		dict.Set("look_ahead", "0", astiav.NewDictionaryFlags())
	case "h264_amf":
		dict.Set("usage", "lowlatency", astiav.NewDictionaryFlags())
		dict.Set("rc", "cbr", astiav.NewDictionaryFlags())
	default: // libx264
		dict.Set("preset", "veryfast", astiav.NewDictionaryFlags())
		dict.Set("tune", "zerolatency", astiav.NewDictionaryFlags())
	}

	if err := p.videoCodecCtx.Open(codec, dict); err != nil {
		return fmt.Errorf("open video codec %s: %w", encoderName, err)
	}

	p.videoStream = p.formatCtx.NewStream(nil)
	if p.videoStream == nil {
		return fmt.Errorf("failed to create video stream")
	}
	if err := p.videoCodecCtx.ToCodecParameters(p.videoStream.CodecParameters()); err != nil {
		return fmt.Errorf("copy video codec params: %w", err)
	}
	p.videoStream.SetTimeBase(p.videoCodecCtx.TimeBase())

	// swscale: srcWidth x srcHeight BGRA -> outputWidth x outputHeight YUV420P
	// When no overlay, src is native resolution (384x272), swscale upscales + converts in one pass.
	// When overlay, src = output = 1920x1080, swscale only does format conversion.
	var err error
	p.swsCtx, err = astiav.CreateSoftwareScaleContext(
		p.srcWidth, p.srcHeight, astiav.PixelFormatBgra,
		cfg.Width, cfg.Height, astiav.PixelFormatYuv420P,
		astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagFastBilinear),
	)
	if err != nil {
		return fmt.Errorf("create sws context: %w", err)
	}

	p.srcFrame = astiav.AllocFrame()
	p.srcFrame.SetWidth(p.srcWidth)
	p.srcFrame.SetHeight(p.srcHeight)
	p.srcFrame.SetPixelFormat(astiav.PixelFormatBgra)
	if err := p.srcFrame.AllocBuffer(0); err != nil {
		return fmt.Errorf("alloc src frame buffer: %w", err)
	}

	p.videoFrame = astiav.AllocFrame()
	p.videoFrame.SetWidth(cfg.Width)
	p.videoFrame.SetHeight(cfg.Height)
	p.videoFrame.SetPixelFormat(astiav.PixelFormatYuv420P)
	if err := p.videoFrame.AllocBuffer(0); err != nil {
		return fmt.Errorf("alloc video frame buffer: %w", err)
	}

	return nil
}

func (p *OutputPipeline) initAudioEncoder(globalHeader bool) error {
	codec := astiav.FindEncoder(astiav.CodecIDAac)
	if codec == nil {
		return fmt.Errorf("AAC encoder not found")
	}

	p.audioCodecCtx = astiav.AllocCodecContext(codec)
	if p.audioCodecCtx == nil {
		return fmt.Errorf("failed to alloc audio codec context")
	}

	p.audioCodecCtx.SetSampleRate(48000)
	p.audioCodecCtx.SetSampleFormat(astiav.SampleFormatFltp)
	p.audioCodecCtx.SetChannelLayout(astiav.ChannelLayoutStereo)
	p.audioCodecCtx.SetBitRate(128000)
	p.audioCodecCtx.SetTimeBase(astiav.NewRational(1, 48000))

	if globalHeader {
		p.audioCodecCtx.SetFlags(p.audioCodecCtx.Flags().Add(astiav.CodecContextFlagGlobalHeader))
	}

	if err := p.audioCodecCtx.Open(codec, nil); err != nil {
		return fmt.Errorf("open audio codec: %w", err)
	}

	p.audioStream = p.formatCtx.NewStream(nil)
	if p.audioStream == nil {
		return fmt.Errorf("failed to create audio stream")
	}
	if err := p.audioCodecCtx.ToCodecParameters(p.audioStream.CodecParameters()); err != nil {
		return fmt.Errorf("copy audio codec params: %w", err)
	}
	p.audioStream.SetTimeBase(p.audioCodecCtx.TimeBase())

	frameSize := p.audioCodecCtx.FrameSize()
	if frameSize <= 0 {
		frameSize = 1024
	}

	p.audioFifo = astiav.AllocAudioFifo(astiav.SampleFormatFltp, 2, frameSize*2)
	if p.audioFifo == nil {
		return fmt.Errorf("failed to alloc audio FIFO")
	}

	p.swrCtx = astiav.AllocSoftwareResampleContext()
	if p.swrCtx == nil {
		return fmt.Errorf("failed to alloc swr context")
	}

	p.audioFrame = astiav.AllocFrame()
	p.audioFrame.SetSampleFormat(astiav.SampleFormatFltp)
	p.audioFrame.SetSampleRate(48000)
	p.audioFrame.SetChannelLayout(astiav.ChannelLayoutStereo)
	p.audioFrame.SetNbSamples(frameSize)
	if err := p.audioFrame.AllocBuffer(0); err != nil {
		return fmt.Errorf("alloc audio frame buffer: %w", err)
	}

	p.encFrame = astiav.AllocFrame()
	p.encFrame.SetSampleFormat(astiav.SampleFormatFltp)
	p.encFrame.SetSampleRate(48000)
	p.encFrame.SetChannelLayout(astiav.ChannelLayoutStereo)

	return nil
}

// muxLoop runs in a dedicated goroutine and writes encoded packets to the
// output (file/RTMP). Decouples the encoder from network I/O -- if Twitch's
// ingest server is slow, this goroutine blocks instead of the encoder.
func (p *OutputPipeline) muxLoop() {
	defer close(p.muxDone)
	for pkt := range p.packetCh {
		if err := p.formatCtx.WriteInterleavedFrame(pkt); err != nil {
			fmt.Printf("mux: write packet: %v\n", err)
		}
		pkt.Free() // release cloned packet reference
	}
}

// sendPacket clones the given packet and queues it for the muxer goroutine.
// Blocks if the channel is full -- dropping packets (especially keyframes)
// would cause severe visual stuttering until the next keyframe.
func (p *OutputPipeline) sendPacket(pkt *astiav.Packet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = nil // pipeline shutting down
		}
	}()

	if p.closed.Load() {
		return nil
	}
	cloned := pkt.Clone()
	p.packetCh <- cloned
	return nil
}

// videoEncodeLoop runs in a background goroutine. It reads BGRA frames from
// the channel, encodes them, and writes packets to the muxer. This decouples
// the render loop from the (potentially slow) x264 encoding, mirroring the
// old pipe-to-FFmpeg approach.
func (p *OutputPipeline) videoEncodeLoop() {
	defer close(p.videoDone)

	for msg := range p.videoCh {
		if err := p.srcFrame.Data().SetBytes(msg.buf, 1); err != nil {
			fmt.Printf("video encode: set bytes: %v\n", err)
			p.bufPool.Put(msg.buf)
			continue
		}

		// Return buffer to pool immediately after copy into AVFrame
		p.bufPool.Put(msg.buf)

		if err := p.swsCtx.ScaleFrame(p.srcFrame, p.videoFrame); err != nil {
			fmt.Printf("video encode: scale: %v\n", err)
			continue
		}

		// Capture-time PTS: independent of pipeline jitter & frame drops.
		// PTS reflects when the frame was actually captured at the source,
		// not when the encoder happened to process it.
		elapsed := msg.captureTime.Sub(p.startTime)
		pts := elapsed.Nanoseconds() * int64(p.fps) / 1_000_000_000
		if pts <= p.lastVideoPts {
			pts = p.lastVideoPts + 1 // strict monotonic
		}
		p.lastVideoPts = pts
		p.videoFrame.SetPts(pts)
		p.frameCount++

		// Force keyframe if requested (e.g. after overlay toggle)
		if p.forceIDR.CompareAndSwap(true, false) {
			p.videoFrame.SetPictureType(astiav.PictureTypeI)
		} else {
			p.videoFrame.SetPictureType(astiav.PictureTypeNone)
		}

		if err := p.videoCodecCtx.SendFrame(p.videoFrame); err != nil {
			fmt.Printf("video encode: send frame: %v\n", err)
			continue
		}

		if err := p.receiveVideoPackets(); err != nil {
			fmt.Printf("video encode: receive/write: %v\n", err)
		}
	}
}

// EncodeVideoFrame queues a BGRA frame for async encoding.
// A copy is made so the caller can reuse the buffer immediately.
// RequestKeyframe forces the next encoded video frame to be an IDR keyframe.
func (p *OutputPipeline) RequestKeyframe() {
	p.forceIDR.Store(true)
}

func (p *OutputPipeline) EncodeVideoFrame(bgraData []byte, captureTime time.Time) (err error) {
	if !p.hasVideo || p.closed.Load() {
		return nil
	}

	// Guard against send-on-closed-channel race between Close() and this call
	defer func() {
		if r := recover(); r != nil {
			err = nil // pipeline shutting down, ignore
		}
	}()

	if captureTime.IsZero() {
		captureTime = time.Now()
	}

	buf := p.bufPool.Get().([]byte)
	copy(buf, bgraData)

	select {
	case p.videoCh <- videoFrameMsg{buf: buf, captureTime: captureTime}:
	default:
		p.bufPool.Put(buf)
	}
	return nil
}

func (p *OutputPipeline) receiveVideoPackets() error {
	for {
		p.videoPacket.Unref()
		err := p.videoCodecCtx.ReceivePacket(p.videoPacket)
		if err != nil {
			if errors.Is(err, astiav.ErrEagain) || errors.Is(err, astiav.ErrEof) {
				return nil
			}
			return fmt.Errorf("receive video packet: %w", err)
		}
		p.videoPacket.SetStreamIndex(p.videoStream.Index())
		p.videoPacket.RescaleTs(p.videoCodecCtx.TimeBase(), p.videoStream.TimeBase())

		if err := p.sendPacket(p.videoPacket); err != nil {
			return fmt.Errorf("queue video packet: %w", err)
		}
	}
}

// EncodeAudio buffers incoming PCM s16le data and encodes full AAC frames.
func (p *OutputPipeline) EncodeAudio(pcmData []byte) error {
	if !p.hasAudio || p.closed.Load() {
		return nil
	}

	nbSamples := len(pcmData) / 4 // 2ch * 2bytes
	if nbSamples <= 0 {
		return nil
	}

	inputFrame := astiav.AllocFrame()
	defer inputFrame.Free()
	inputFrame.SetSampleFormat(astiav.SampleFormatS16)
	inputFrame.SetSampleRate(48000)
	inputFrame.SetChannelLayout(astiav.ChannelLayoutStereo)
	inputFrame.SetNbSamples(nbSamples)
	if err := inputFrame.AllocBuffer(0); err != nil {
		return fmt.Errorf("alloc input audio frame: %w", err)
	}
	if err := inputFrame.Data().SetBytes(pcmData, 1); err != nil {
		return fmt.Errorf("set audio input bytes: %w", err)
	}

	p.encFrame.SetNbSamples(0)
	if err := p.swrCtx.ConvertFrame(inputFrame, p.encFrame); err != nil {
		return fmt.Errorf("convert audio S16->FLTP: %w", err)
	}

	if _, err := p.audioFifo.Write(p.encFrame); err != nil {
		return fmt.Errorf("write audio FIFO: %w", err)
	}

	frameSize := p.audioCodecCtx.FrameSize()
	if frameSize <= 0 {
		frameSize = 1024
	}

	for p.audioFifo.Size() >= frameSize {
		p.audioFrame.SetNbSamples(frameSize)
		if _, err := p.audioFifo.Read(p.audioFrame); err != nil {
			return fmt.Errorf("read audio FIFO: %w", err)
		}
		p.audioFrame.SetPts(p.audioPts)
		p.audioPts += int64(frameSize)

		if err := p.audioCodecCtx.SendFrame(p.audioFrame); err != nil {
			return fmt.Errorf("send audio frame: %w", err)
		}

		if err := p.receiveAudioPackets(); err != nil {
			return err
		}
	}

	return nil
}

func (p *OutputPipeline) receiveAudioPackets() error {
	for {
		p.audioPacket.Unref()
		err := p.audioCodecCtx.ReceivePacket(p.audioPacket)
		if err != nil {
			if errors.Is(err, astiav.ErrEagain) || errors.Is(err, astiav.ErrEof) {
				return nil
			}
			return fmt.Errorf("receive audio packet: %w", err)
		}
		p.audioPacket.SetStreamIndex(p.audioStream.Index())
		p.audioPacket.RescaleTs(p.audioCodecCtx.TimeBase(), p.audioStream.TimeBase())

		if err := p.sendPacket(p.audioPacket); err != nil {
			return fmt.Errorf("queue audio packet: %w", err)
		}
	}
}

func (p *OutputPipeline) Close() error {
	// Stop accepting new input frames (EncodeVideoFrame/EncodeAudio will return).
	// The muxer goroutine is still allowed to receive flush packets below.
	p.closed.Store(true)

	// 1. Stop video encoder goroutine (drains videoCh)
	if p.videoCh != nil {
		close(p.videoCh)
		<-p.videoDone
	}

	// 2. Flush encoders -- produces final packets that still need to pass
	//    through packetCh to the muxer goroutine. sendPacket checks closed,
	//    so we need to allow sends during flush: temporarily re-enable.
	p.closed.Store(false)
	if p.videoCodecCtx != nil {
		p.videoCodecCtx.SendFrame(nil)
		if p.videoPacket != nil {
			p.receiveVideoPackets()
		}
	}
	if p.audioCodecCtx != nil {
		p.audioCodecCtx.SendFrame(nil)
		if p.audioPacket != nil {
			p.receiveAudioPackets()
		}
	}
	p.closed.Store(true)

	// 3. Close packet channel and wait for muxer goroutine to drain
	if p.packetCh != nil {
		close(p.packetCh)
		<-p.muxDone
	}

	// 4. Write trailer (muxer finished, safe to finalize output)
	if p.formatCtx != nil && p.headerWritten {
		p.formatCtx.WriteTrailer()
	}

	if p.videoPacket != nil {
		p.videoPacket.Free()
	}
	if p.audioPacket != nil {
		p.audioPacket.Free()
	}
	if p.audioFifo != nil {
		p.audioFifo.Free()
	}
	if p.swrCtx != nil {
		p.swrCtx.Free()
	}
	if p.encFrame != nil {
		p.encFrame.Free()
	}
	if p.audioFrame != nil {
		p.audioFrame.Free()
	}
	if p.swsCtx != nil {
		p.swsCtx.Free()
	}
	if p.srcFrame != nil {
		p.srcFrame.Free()
	}
	if p.videoFrame != nil {
		p.videoFrame.Free()
	}
	if p.audioCodecCtx != nil {
		p.audioCodecCtx.Free()
	}
	if p.videoCodecCtx != nil {
		p.videoCodecCtx.Free()
	}
	if p.ioCtx != nil {
		p.ioCtx.Close()
	}
	if p.formatCtx != nil {
		p.formatCtx.Free()
	}

	return nil
}

func parseBitrate(s string) int64 {
	var val int64
	var suffix string
	fmt.Sscanf(s, "%d%s", &val, &suffix)
	switch suffix {
	case "k", "K":
		return val * 1000
	case "m", "M":
		return val * 1000000
	default:
		return val
	}
}
