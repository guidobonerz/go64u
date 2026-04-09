package gui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sort"
	"sync"

	"drazil.de/go64u/config"
	"drazil.de/go64u/fonts"
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
)

type guiApp struct {
	window  *app.Window
	theme   *material.Theme
	otoCtx  *oto.Context
	devices []deviceUI
	mu      sync.RWMutex
}

type deviceUI struct {
	name        string
	description string
	device      *config.Device
	toggle      widget.Clickable
	active      bool
	waveform    []byte
}

var (
	colorBackground = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	colorText       = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	colorPlayIcon   = color.NRGBA{R: 103, G: 255, B: 69, A: 255}
	colorStopIcon   = color.NRGBA{R: 254, G: 0, B: 0, A: 255}
	colorWaveformBg = color.NRGBA{R: 55, G: 55, B: 55, A: 255}
	colorWaveformFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorSeparator  = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
)

func Run() {
	go func() {
		a := newApp()
		a.run()
		os.Exit(0)
	}()
	app.Main()
}

func newApp() *guiApp {
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

	w := new(app.Window)
	w.Option(app.Title("go64u - experimental gui"), app.Size(unit.Dp(500), unit.Dp(500)))

	return &guiApp{
		window:  w,
		theme:   th,
		otoCtx:  otoCtx,
		devices: devices,
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
		children := make([]layout.FlexChild, 0, len(a.devices)*2)
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
			drawStopIcon(gtx, size)
		} else {
			drawPlayIcon(gtx, size)
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func drawPlayIcon(gtx layout.Context, size int) {
	// Draw a right-pointing triangle (play)
	margin := float32(size) * 0.15
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(f32.Pt(margin, margin))
	path.LineTo(f32.Pt(float32(size)-margin, float32(size)/2))
	path.LineTo(f32.Pt(margin, float32(size)-margin))
	path.Close()
	defer clip.Outline{Path: path.End()}.Op().Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorPlayIcon)
}

func drawStopIcon(gtx layout.Context, size int) {
	// Draw a rounded rectangle (stop)
	margin := float32(size) * 0.15
	r := float32(size) * 0.08
	rect := clip.RRect{
		Rect: image.Rect(int(margin), int(margin), size-int(margin), size-int(margin)),
		SE:   int(r), SW: int(r), NE: int(r), NW: int(r),
	}
	defer rect.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorStopIcon)
}

func (a *guiApp) layoutWaveform(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	w := gtx.Dp(unit.Dp(120))
	h := gtx.Dp(unit.Dp(40))
	sz := image.Pt(w, h)
	gtx.Constraints = layout.Exact(sz)

	// Background
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorWaveformBg)

	// Waveform
	a.mu.RLock()
	data := dev.waveform
	a.mu.RUnlock()

	if data != nil && len(data) > 8 {
		halfH := float32(h) / 2
		var path clip.Path
		path.Begin(gtx.Ops)

		first := true
		var x float32
		for i := 2; i < len(data)-4; i += 4 {
			v := halfH - (float32(util.GetSingedWord(i, data))/32768.0)*halfH
			v = float32(math.Max(0, math.Min(float64(h), float64(v))))
			if first {
				path.MoveTo(f32.Pt(x, v))
				first = false
			} else {
				path.LineTo(f32.Pt(x, v))
			}
			x += 1
			if int(x) >= w {
				break
			}
		}
		spec := path.End()
		defer clip.Stroke{Path: spec, Width: 1}.Op().Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, colorWaveformFg)
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

func (a *guiApp) startAudio(dev *deviceUI) {
	dev.device.AudioChannel = make(chan struct{})
	streams.AudioStart(dev.device)
	audioReader := streams.AudioReader{
		Device:       dev.device,
		AudioContext: a.otoCtx,
		StopChan:     dev.device.AudioChannel,
		Renderer: func(data []byte) {
			a.mu.Lock()
			dev.waveform = data
			a.mu.Unlock()
			a.window.Invalidate()
		},
	}
	go audioReader.Read()
	fmt.Printf("stream on %s started\n", dev.name)
}

func (a *guiApp) stopAudio(dev *deviceUI) {
	if dev.device.AudioChannel != nil {
		fmt.Printf("stream on %s stopped\n", dev.name)
		close(dev.device.AudioChannel)
		dev.device.AudioChannel = nil
	}
	streams.AudioStop(dev.device)
}
