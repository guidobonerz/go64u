package gui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/fonts"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/streams"
	"drazil.de/go64u/util"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ebitengine/oto/v3"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type guiApp struct {
	window  *app.Window
	theme   *material.Theme
	otoCtx  *oto.Context
	devices []deviceUI
	mu      sync.RWMutex
	monitor bool // --monitor mode: show video stream
}

// rawFrameMsg carries a raw paletted frame buffer together with the wall-clock
// time it was captured from the UDP source. The capture time flows through to
// the encoder so PTS reflects real time, independent of pipeline jitter/drops.
type rawFrameMsg struct {
	data        []byte
	captureTime time.Time
}

type deviceUI struct {
	name         string
	description  string
	device       *config.Device
	toggle       widget.Clickable
	active       bool
	waveform     []byte
	videoFrame   *image.NRGBA  // current video frame for monitor display
	videoActive  bool          // true when video stream is running
	audioPlaying bool          // true when audio reader is running
	audioStopCh  chan struct{} // stop channel for audio reader (independent of device channel)
	audioMonitor bool          // audio monitoring enabled
	videoMonitor bool          // video monitoring enabled
	recording    bool          // true when recording to file
	casting      bool          // true when streaming to platform
	overlayOn    bool          // overlay enabled for recording/casting
	crtOn        bool          // CRT monitor style enabled
	recRenderer  *streams.StreamRenderer
	castRenderer *streams.StreamRenderer
	rawFrameCh   chan rawFrameMsg // channel for feeding raw frames + capture timestamps to cast/rec renderers
	audioBtn     widget.Clickable
	videoBtn     widget.Clickable
	recBtn       widget.Clickable
	castBtn      widget.Clickable
	overlayBtn   widget.Clickable
	snapBtn      widget.Clickable
	crtBtn       widget.Clickable
}

var (
	colorBackground = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	colorText       = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	colorActive     = color.NRGBA{R: 103, G: 255, B: 69, A: 255}
	colorInactive   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorToggleOff  = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	colorStrike     = color.NRGBA{R: 254, G: 0, B: 0, A: 255}
	colorHoverWhite = color.NRGBA{R: 150, G: 255, B: 130, A: 255} // light green hover for white icons
	colorHoverGray  = color.NRGBA{R: 0, G: 160, B: 0, A: 255}     // dark green hover for gray icons
	colorWaveformBg = color.NRGBA{R: 55, G: 55, B: 55, A: 255}
	colorWaveformFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorSeparator  = color.NRGBA{R: 80, G: 80, B: 80, A: 255}

	iconPlay, _          = widget.NewIcon(icons.AVPlayArrow)
	iconStop, _          = widget.NewIcon(icons.AVStop)
	iconMusic, _         = widget.NewIcon(icons.AVVolumeUp)
	iconMute, _          = widget.NewIcon(icons.AVVolumeOff)
	iconEye, _           = widget.NewIcon(icons.ActionVisibility)
	iconEyeOff, _        = widget.NewIcon(icons.ActionVisibilityOff)
	iconRecord, _        = widget.NewIcon(icons.AVFiberManualRecord)
	iconCast, _          = widget.NewIcon(icons.HardwareCast)
	iconCastConnected, _ = widget.NewIcon(icons.HardwareCastConnected)
	iconOverlay, _       = widget.NewIcon(icons.MapsLayers)
	iconOverlayOff, _    = widget.NewIcon(icons.MapsLayersClear)
	iconCamera, _        = widget.NewIcon(icons.ImagePhotoCamera)
	iconCrt, _           = widget.NewIcon(icons.HardwareTV)
	colorRecording       = color.NRGBA{R: 255, G: 40, B: 40, A: 255}
)

func Run(monitor bool) {
	go func() {
		a := newApp(monitor)
		a.run()
		os.Exit(0)
	}()
	app.Main()
}

func newApp(monitor bool) *guiApp {
	op := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	}
	otoCtx, readyChan, err := oto.NewContext(op)
	if err != nil {
		panic(err)
	}
	<-readyChan

	face, err := opentype.Parse(fonts.C64ProMonoSTYLE)
	if err != nil {
		panic(err)
	}
	fontFaces := []font.FontFace{{Font: face.Font(), Face: face}}
	shaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(fontFaces))

	th := material.NewTheme()
	th.Shaper = shaper
	th.TextSize = unit.Sp(16)
	th.Palette.Fg = colorText
	th.Palette.Bg = colorBackground

	devices := buildDeviceList()
	hasOverlay := config.GetConfig().Overlay.ImagePath != ""
	for i := range devices {
		devices[i].audioMonitor = true
		devices[i].videoMonitor = true
		devices[i].overlayOn = hasOverlay
	}

	w := new(app.Window)
	if monitor {
		// 768x544 image pixels + room for audio panel + insets
		w.Option(app.Title("go64u - monitor"), app.Size(unit.Dp(800), unit.Dp(680)))
	} else {
		w.Option(app.Title("go64u - experimental gui"), app.Size(unit.Dp(500), unit.Dp(500)))
	}

	return &guiApp{
		window:  w,
		theme:   th,
		otoCtx:  otoCtx,
		devices: devices,
		monitor: monitor,
	}
}

func buildDeviceList() []deviceUI {
	cfg := config.GetConfig()
	names := make([]string, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		names = append(names, name)
	}
	sort.Strings(names)

	devices := make([]deviceUI, 0, len(names))
	for _, name := range names {
		dev := cfg.Devices[name]
		devices = append(devices, deviceUI{
			name:        name,
			description: dev.Description,
			device:      dev,
		})
	}
	return devices
}

func (a *guiApp) run() {
	var ops op.Ops
	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.layoutRoot(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *guiApp) layoutRoot(gtx layout.Context) layout.Dimensions {
	// Dark background
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorBackground)

	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(a.devices)*2+2)
		for i := range a.devices {
			idx := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutDevice(gtx, idx)
			}))
			if i < len(a.devices)-1 {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSeparator(gtx)
				}))
			}
		}
		// Video monitor panel below audio devices (only if any video is active)
		hasActiveVideo := false
		for i := range a.devices {
			if a.devices[i].videoActive {
				hasActiveVideo = true
				break
			}
		}
		if a.monitor && hasActiveVideo {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutSeparator(gtx)
			}))
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutVideoMonitor(gtx)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *guiApp) layoutDevice(gtx layout.Context, index int) layout.Dimensions {
	dev := &a.devices[index]

	if dev.toggle.Clicked(gtx) {
		dev.active = !dev.active
		if dev.active {
			a.startAudio(dev)
		} else {
			a.stopAudio(dev)
		}
	}

	// Handle icon button clicks
	if a.monitor {
		if dev.audioBtn.Clicked(gtx) {
			dev.audioMonitor = !dev.audioMonitor
			if dev.active {
				if dev.audioMonitor && !dev.audioPlaying {
					a.startAudioReader(dev)
				} else if !dev.audioMonitor && dev.audioPlaying {
					a.stopAudioReader(dev)
				}
			}
		}
		if dev.videoBtn.Clicked(gtx) {
			dev.videoMonitor = !dev.videoMonitor
			if dev.active {
				if dev.videoMonitor && !dev.videoActive {
					a.startVideo(dev)
				} else if !dev.videoMonitor && dev.videoActive {
					a.stopVideo(dev)
				}
			}
		}
		if dev.crtBtn.Clicked(gtx) {
			dev.crtOn = !dev.crtOn
			if dev.castRenderer != nil {
				dev.castRenderer.SetCrt(dev.crtOn)
			}
			if dev.recRenderer != nil {
				dev.recRenderer.SetCrt(dev.crtOn)
			}
		}
		if dev.overlayBtn.Clicked(gtx) {
			dev.overlayOn = !dev.overlayOn
			// Apply to running cast/recording in real-time
			if dev.castRenderer != nil {
				dev.castRenderer.SetOverlay(dev.overlayOn)
			}
			if dev.recRenderer != nil {
				dev.recRenderer.SetOverlay(dev.overlayOn)
			}
		}
		if dev.recBtn.Clicked(gtx) {
			if dev.active {
				if !dev.recording {
					a.startRecording(dev)
				} else {
					a.stopRecording(dev)
				}
			}
		}
		if dev.castBtn.Clicked(gtx) {
			if dev.active {
				if !dev.casting {
					a.startCasting(dev)
				} else {
					a.stopCasting(dev)
				}
			}
		}
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutToggle(gtx, dev)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutWaveform(gtx, dev)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !a.monitor {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutIconButton(gtx, &dev.audioBtn, dev.audioMonitor, dev.active, iconMusic, iconMute)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutIconButton(gtx, &dev.videoBtn, dev.videoMonitor, dev.active, iconEye, iconEyeOff)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutSnapshotButton(gtx, dev)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutRecordButton(gtx, dev)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutCastButton(gtx, dev)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutIconButton(gtx, &dev.overlayBtn, dev.overlayOn, dev.active, iconOverlay, iconOverlayOff)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutIconButton(gtx, &dev.crtBtn, dev.crtOn, dev.active, iconCrt, iconCrt)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(a.theme, unit.Sp(16), dev.description)
			return lbl.Layout(gtx)
		}),
	)
}

func (a *guiApp) layoutToggle(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(40))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	return dev.toggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if dev.active {
			iconStop.Layout(gtx, colorStrike)
		} else {
			iconPlay.Layout(gtx, colorActive)
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

// layoutIconButton renders a clickable material icon. Uses colorActive when on,
// colorInactive when off. Video uses visibility/visibility_off icons.
func (a *guiApp) layoutIconButton(gtx layout.Context, btn *widget.Clickable, active bool, streamActive bool, iconOn, iconOff *widget.Icon) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	hovered := btn.Hovered()
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !streamActive {
			iconOff.Layout(gtx, colorToggleOff)
		} else if active {
			if hovered {
				iconOn.Layout(gtx, colorHoverWhite)
			} else {
				iconOn.Layout(gtx, colorInactive)
			}
		} else {
			if hovered {
				iconOff.Layout(gtx, colorHoverGray)
			} else {
				iconOff.Layout(gtx, colorToggleOff)
			}
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (a *guiApp) layoutWaveform(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	w := gtx.Dp(unit.Dp(140))
	h := gtx.Dp(unit.Dp(40))
	gap := gtx.Dp(unit.Dp(4))
	sz := image.Pt(w, h)
	gtx.Constraints = layout.Exact(sz)

	// Background
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorWaveformBg)

	a.mu.RLock()
	data := dev.waveform
	a.mu.RUnlock()

	if len(data) > 8 {
		halfH := float32(h) / 2
		channelW := (w - gap) / 2

		// Build left channel path first
		var pathLeft clip.Path
		pathLeft.Begin(gtx.Ops)
		first := true
		var x float32
		for i := 2; i < len(data)-4; i += 4 {
			v := halfH - (float32(util.GetSingedWord(i, data))/32768.0)*halfH
			v = float32(math.Max(0, math.Min(float64(h), float64(v))))
			if first {
				pathLeft.MoveTo(f32.Pt(x, v))
				first = false
			} else {
				pathLeft.LineTo(f32.Pt(x, v))
			}
			x += 1
			if int(x) >= channelW {
				break
			}
		}
		specLeft := pathLeft.End()
		func() {
			defer clip.Stroke{Path: specLeft, Width: 1}.Op().Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, colorWaveformFg)
		}()

		// Build right channel path
		var pathRight clip.Path
		pathRight.Begin(gtx.Ops)
		first = true
		x = 0
		offsetX := float32(channelW + gap)
		for i := 4; i < len(data)-4; i += 4 {
			v := halfH - (float32(util.GetSingedWord(i, data))/32768.0)*halfH
			v = float32(math.Max(0, math.Min(float64(h), float64(v))))
			if first {
				pathRight.MoveTo(f32.Pt(offsetX+x, v))
				first = false
			} else {
				pathRight.LineTo(f32.Pt(offsetX+x, v))
			}
			x += 1
			if int(x) >= channelW {
				break
			}
		}
		specRight := pathRight.End()
		func() {
			defer clip.Stroke{Path: specRight, Width: 1}.Op().Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, colorWaveformFg)
		}()
	}

	return layout.Dimensions{Size: sz}
}

func (a *guiApp) layoutSeparator(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	width := gtx.Constraints.Max.X
	sz := image.Pt(width, height)
	inset := layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5)}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, colorSeparator)
		return layout.Dimensions{Size: sz}
	})
}

func (a *guiApp) layoutVideoMonitor(gtx layout.Context) layout.Dimensions {
	// Collect active video streams
	a.mu.RLock()
	var activeFrames []*image.NRGBA
	for i := range a.devices {
		if a.devices[i].videoActive && a.devices[i].videoFrame != nil {
			activeFrames = append(activeFrames, a.devices[i].videoFrame)
		}
	}
	a.mu.RUnlock()

	count := len(activeFrames)
	if count == 0 {
		count = 1 // reserve space for at least one panel
	}

	// Split available width evenly across active streams with 10px gap
	totalW := gtx.Constraints.Max.X
	gap := gtx.Dp(unit.Dp(10))
	totalGap := gap * (count - 1)
	cellW := (totalW - totalGap) / count
	cellH := cellW * imaging.HEIGHT / imaging.WIDTH
	sz := image.Pt(totalW, cellH)
	gtx.Constraints = layout.Exact(sz)

	// Draw each active frame side by side with rounded border
	borderWidth := gtx.Dp(unit.Dp(3))
	borderRadius := gtx.Dp(unit.Dp(8))
	for i, frame := range activeFrames {
		offsetX := i * (cellW + gap)
		stack := op.Offset(image.Pt(offsetX, 0)).Push(gtx.Ops)

		// Rounded border
		rrect := clip.RRect{
			Rect: image.Rect(0, 0, cellW, cellH),
			SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
		}
		borderStack := clip.Stroke{Path: rrect.Path(gtx.Ops), Width: float32(borderWidth)}.Op().Push(gtx.Ops)
		paint.Fill(gtx.Ops, colorSeparator)
		borderStack.Pop()

		// Clip image to rounded rect
		clipStack := clip.RRect{
			Rect: image.Rect(0, 0, cellW, cellH),
			SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
		}.Push(gtx.Ops)

		scaleX := float32(cellW) / float32(frame.Bounds().Dx())
		scaleY := float32(cellH) / float32(frame.Bounds().Dy())
		imgOp := paint.NewImageOp(frame)
		imgOp.Add(gtx.Ops)
		t := f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scaleX, scaleY))
		op.Affine(t).Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		clipStack.Pop()
		stack.Pop()
	}

	return layout.Dimensions{Size: sz}
}

func (a *guiApp) startVideo(dev *deviceUI) {
	if dev.videoActive {
		return
	}
	dev.videoActive = true
	streams.VideoStart(dev.device)

	reusableImg := imaging.NewReusablePalettedImage()
	palette := util.GetPalette()

	go func() {
		socket := dev.device.VideoUdpConnection
		dataBuffer := make([]byte, 780)
		imgBuf := make([]byte, imaging.SIZE/2)
		count := 0
		offset := 0
		capture := false

		// Pre-build NRGBA palette LUT
		var lut [16]color.NRGBA
		for i, c := range palette {
			if i >= 16 {
				break
			}
			r, g, b, aa := c.RGBA()
			lut[i] = color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(aa >> 8)}
		}

		// Pre-scaled frame for crisp nearest-neighbor display
		// Scale up to a reasonable size; Gio handles final dp-to-pixel mapping
		const scale = 3
		displayW := imaging.WIDTH * scale
		displayH := imaging.HEIGHT * scale
		nrgbaFrame := image.NewNRGBA(image.Rect(0, 0, displayW, displayH))

		for dev.videoActive && socket != nil {
			_, _, err := socket.ReadFromUDP(dataBuffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				break
			}

			linenumber := util.GetWordFromArray(4, dataBuffer)

			// Copy packet data first (bit-15 packet is LAST of current frame)
			if capture && offset+len(dataBuffer[12:]) <= len(imgBuf) {
				n := copy(imgBuf[offset:], dataBuffer[12:])
				offset += n
				count++
			}

			// Bit 15 = last packet of frame. Emit completed frame, start new one.
			if linenumber&0x8000 == 0x8000 {
				if capture && count == 68 {
					// Decode paletted image and nearest-neighbor scale into NRGBA
					palImg := reusableImg.Decode(imgBuf[:offset])
					srcStride := palImg.Stride
					srcPix := palImg.Pix
					dstStride := nrgbaFrame.Stride
					dstPix := nrgbaFrame.Pix

					// Only show local scanlines in single-display mode (one device active)
					activeCount := 0
					a.mu.RLock()
					for i := range a.devices {
						if a.devices[i].videoActive {
							activeCount++
						}
					}
					a.mu.RUnlock()
					crtOn := dev.crtOn && activeCount == 1
					// Sinusoidal CRT scanline profile: brightest at scanline center,
					// smoothly fading to minBright at edges. pixelH = scale (3 here).
					var factors [scale]uint16
					if crtOn {
						const minBright = 0.45
						pixelH := float32(scale)
						for i := range scale {
							fracY := float32(i) / pixelH
							bell := float32(math.Sin(float64(fracY) * math.Pi))
							f := minBright + (1.0-minBright)*bell
							factors[i] = uint16(f * 255)
						}
					} else {
						for i := range scale {
							factors[i] = 255
						}
					}
					for y := range imaging.HEIGHT {
						srcRow := y * srcStride
						for sy := range scale {
							dstRow := (y*scale + sy) * dstStride
							f := factors[sy]
							for x := range imaging.WIDTH {
								c := lut[srcPix[srcRow+x]&0x0F]
								r := byte(uint16(c.R) * f / 255)
								g := byte(uint16(c.G) * f / 255)
								b := byte(uint16(c.B) * f / 255)
								for sx := range scale {
									off := dstRow + (x*scale+sx)*4
									dstPix[off] = r
									dstPix[off+1] = g
									dstPix[off+2] = b
									dstPix[off+3] = c.A
								}
							}
						}
					}

					a.mu.Lock()
					dev.videoFrame = nrgbaFrame
					a.mu.Unlock()
					a.window.Invalidate()

					// Feed raw frame to cast/rec goroutine (non-blocking, outside lock)
					if dev.rawFrameCh != nil {
						rawCopy := make([]byte, offset)
						copy(rawCopy, imgBuf[:offset])
						msg := rawFrameMsg{data: rawCopy, captureTime: time.Now()}
						select {
						case dev.rawFrameCh <- msg:
						default:
						}
					}
				}

				capture = true
				count = 0
				offset = 0
			}
		}
	}()
}

func (a *guiApp) stopVideo(dev *deviceUI) {
	if !dev.videoActive {
		return
	}
	dev.videoActive = false
	streams.VideoStop(dev.device)
	a.mu.Lock()
	dev.videoFrame = nil
	a.mu.Unlock()
}

func (a *guiApp) layoutSnapshotButton(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	if dev.snapBtn.Clicked(gtx) && dev.active {
		// Save current video frame as screenshot
		a.mu.RLock()
		frame := dev.videoFrame
		a.mu.RUnlock()
		if frame != nil {
			go func() {
				path := fmt.Sprintf("%sscreenshot_%s.png",
					config.GetConfig().ScreenshotFolder,
					time.Now().Format("2006-01-02_15-04-05"))
				f, err := os.Create(path)
				if err != nil {
					fmt.Printf("Screenshot failed: %v\n", err)
					return
				}
				defer f.Close()
				if err := png.Encode(f, frame); err != nil {
					fmt.Printf("Screenshot encode failed: %v\n", err)
					return
				}
				fmt.Printf("Screenshot saved: %s\n", path)
			}()
		}
	}

	hovered := dev.snapBtn.Hovered()
	return dev.snapBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if dev.active {
			if hovered {
				iconCamera.Layout(gtx, colorHoverWhite)
			} else {
				iconCamera.Layout(gtx, colorInactive)
			}
		} else {
			iconCamera.Layout(gtx, colorToggleOff)
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (a *guiApp) layoutCastButton(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	hovered := dev.castBtn.Hovered()
	return dev.castBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !dev.active {
			iconCast.Layout(gtx, colorToggleOff)
		} else if dev.casting {
			if hovered {
				iconCastConnected.Layout(gtx, colorHoverWhite)
			} else {
				iconCastConnected.Layout(gtx, colorInactive)
			}
		} else {
			if hovered {
				iconCast.Layout(gtx, colorHoverGray)
			} else {
				iconCast.Layout(gtx, colorToggleOff)
			}
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (a *guiApp) layoutRecordButton(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	hovered := dev.recBtn.Hovered()
	return dev.recBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !dev.active {
			iconRecord.Layout(gtx, colorToggleOff)
		} else if dev.recording {
			iconRecord.Layout(gtx, colorRecording)
		} else {
			if hovered {
				iconRecord.Layout(gtx, colorHoverWhite)
			} else {
				iconRecord.Layout(gtx, colorInactive)
			}
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (a *guiApp) startRecording(dev *deviceUI) {
	if dev.recording {
		return
	}

	recordMode := "both"
	if !dev.videoMonitor {
		recordMode = "audio"
	} else if !dev.audioMonitor {
		recordMode = "video"
	}

	recordPath := fmt.Sprintf("%sstream_%s.mp4",
		config.GetConfig().RecordingFolder,
		time.Now().Format("2006-01-02_15-04-05"))

	renderer := &streams.StreamRenderer{
		ScaleFactor: 100,
		Fps:         50,
		LogLevel:    config.GetConfig().LogLevel,
		RecordPath:  recordPath,
		RecordMode:  recordMode,
	}

	if err := renderer.Init(); err != nil {
		fmt.Printf("Recording init failed: %v\n", err)
		return
	}
	renderer.SetOverlay(dev.overlayOn)
	renderer.SetCrt(dev.crtOn)

	a.mu.Lock()
	dev.recRenderer = renderer
	a.mu.Unlock()
	dev.recording = true
	a.ensureRawFrameCh(dev)

	fmt.Printf("Recording started: %s (mode: %s)\n", recordPath, recordMode)
}

// ensureRawFrameCh creates the raw frame channel and consumer goroutine
// if not already running. Used by both cast and recording.
func (a *guiApp) ensureRawFrameCh(dev *deviceUI) {
	if dev.rawFrameCh != nil {
		return
	}
	dev.rawFrameCh = make(chan rawFrameMsg, 4)
	go func() {
		for msg := range dev.rawFrameCh {
			a.mu.RLock()
			rec := dev.recRenderer
			cast := dev.castRenderer
			a.mu.RUnlock()
			if rec != nil {
				rec.RenderAt(msg.data, msg.captureTime)
			}
			if cast != nil {
				cast.RenderAt(msg.data, msg.captureTime)
			}
		}
	}()
}

func (a *guiApp) stopRawFrameCh(dev *deviceUI) {
	if dev.rawFrameCh != nil && !dev.recording && !dev.casting {
		close(dev.rawFrameCh)
		dev.rawFrameCh = nil
	}
}

func (a *guiApp) startCasting(dev *deviceUI) {
	if dev.casting {
		return
	}

	targets := config.GetConfig().StreamingTargets
	url, ok := targets["twitch"]
	if !ok {
		fmt.Println("No 'twitch' streaming target configured")
		return
	}
	targetName := "twitch"

	renderer := &streams.StreamRenderer{
		ScaleFactor: 100,
		Fps:         50,
		Url:         url,
		LogLevel:    config.GetConfig().LogLevel,
	}

	if err := renderer.Init(); err != nil {
		fmt.Printf("Cast init failed: %v\n", err)
		return
	}
	renderer.SetOverlay(dev.overlayOn)
	renderer.SetCrt(dev.crtOn)

	a.mu.Lock()
	dev.castRenderer = renderer
	a.mu.Unlock()
	dev.casting = true
	a.ensureRawFrameCh(dev)
	fmt.Printf("Casting started to %s (%s)\n", targetName, url)
}

func (a *guiApp) stopCasting(dev *deviceUI) {
	if !dev.casting || dev.castRenderer == nil {
		return
	}
	dev.casting = false
	renderer := dev.castRenderer
	a.mu.Lock()
	dev.castRenderer = nil
	a.mu.Unlock()
	a.stopRawFrameCh(dev)
	renderer.Shutdown()
	fmt.Println("Casting stopped")
}

func (a *guiApp) stopRecording(dev *deviceUI) {
	if !dev.recording || dev.recRenderer == nil {
		return
	}
	dev.recording = false
	renderer := dev.recRenderer
	a.mu.Lock()
	dev.recRenderer = nil
	a.mu.Unlock()
	a.stopRawFrameCh(dev)
	renderer.Shutdown()
	fmt.Println("Recording stopped")
}

func (a *guiApp) startAudioReader(dev *deviceUI) {
	if dev.audioPlaying {
		return
	}
	dev.audioPlaying = true
	dev.audioStopCh = make(chan struct{})
	audioReader := streams.AudioReader{
		Device:       dev.device,
		AudioContext: a.otoCtx,
		StopChan:     dev.audioStopCh,
		Renderer: func(data []byte) {
			a.mu.Lock()
			dev.waveform = data
			// Feed audio to recorder/caster if active
			if dev.recording && dev.recRenderer != nil {
				dev.recRenderer.WriteAudio(data)
			}
			if dev.casting && dev.castRenderer != nil {
				dev.castRenderer.WriteAudio(data)
			}
			a.mu.Unlock()
			a.window.Invalidate()
		},
	}
	go audioReader.Read()
}

func (a *guiApp) stopAudioReader(dev *deviceUI) {
	if !dev.audioPlaying {
		return
	}
	dev.audioPlaying = false
	if dev.audioStopCh != nil {
		close(dev.audioStopCh)
		dev.audioStopCh = nil
	}
	a.mu.Lock()
	dev.waveform = nil
	a.mu.Unlock()
}

func (a *guiApp) startAudio(dev *deviceUI) {
	dev.device.AudioChannel = make(chan struct{})
	streams.AudioStart(dev.device)

	if dev.audioMonitor {
		a.startAudioReader(dev)
	}
	fmt.Printf("stream on %s started\n", dev.name)

	if a.monitor && dev.videoMonitor {
		a.startVideo(dev)
	}
}

func (a *guiApp) stopAudio(dev *deviceUI) {
	a.stopCasting(dev)
	a.stopRecording(dev)
	if dev.videoActive {
		a.stopVideo(dev)
	}
	a.stopAudioReader(dev)

	if dev.device.AudioChannel != nil {
		fmt.Printf("stream on %s stopped\n", dev.name)
		close(dev.device.AudioChannel)
		dev.device.AudioChannel = nil
	}
	streams.AudioStop(dev.device)
}
