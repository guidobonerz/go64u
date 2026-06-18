package gui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"drazil.de/go64u/commands"
	"drazil.de/go64u/config"
	"drazil.de/go64u/fonts"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/network"
	"drazil.de/go64u/streams"
	"drazil.de/go64u/util"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/io/event"
	"gioui.org/io/key"
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
	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type guiApp struct {
	window          *app.Window
	theme           *material.Theme
	otoCtx          *oto.Context
	devices         []deviceUI
	mu              sync.RWMutex
	selectedIdx     int
	hint            string
	toolbarHeightPx int
	dropMu          sync.Mutex
	dropHits        []dropHit

	keyboard        *VirtualKeyboard
	keyboardVisible bool
	keyboardBtn     widget.Clickable
	expandBtn       widget.Clickable
	expanded        bool // when true, show only the selected device's monitor full-size
	lastWindowW     unit.Dp
	lastWindowH     unit.Dp
	keyFocusTag     struct{}

	linuxFixedSize bool

	monStats monitorStats
}

// monitorPanelsLocked returns the device indices shown in the monitor grid.
// Normally every configured device gets a panel; in expansion mode only the
// selected device is shown, full-size. Caller holds a.mu.
func (a *guiApp) monitorPanelsLocked() []int {
	if a.expanded && a.selectedIdx >= 0 && a.selectedIdx < len(a.devices) {
		return []int{a.selectedIdx}
	}
	idx := make([]int, len(a.devices))
	for i := range a.devices {
		idx[i] = i
	}
	return idx
}

func (a *guiApp) monitorPanelCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.monitorPanelsLocked())
}

// monitorGrid lays panels out in up to 2 columns (a 2×n grid).
func monitorGrid(count int) (cols, rows int) {
	if count <= 0 {
		return 0, 0
	}
	cols = 1
	if count >= 2 {
		cols = 2
	}
	rows = (count + cols - 1) / cols
	return
}

// deviceTestCard returns the device's cached test pattern, building it (with a
// central device-name box) on first use.
func (a *guiApp) deviceTestCard(dev *deviceUI) paint.ImageOp {
	if !dev.testCardReady {
		name := dev.description

		dev.testCardOp = paint.NewImageOp(makeTestCard(name))
		dev.testCardOp.Filter = paint.FilterNearest
		dev.testCardReady = true
	}
	return dev.testCardOp
}

func makeTestCard(name string) *image.RGBA {
	const w, h = imaging.WIDTH, imaging.HEIGHT
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bars := []color.RGBA{
		{200, 200, 200, 255},
		{200, 200, 0, 255},
		{0, 200, 200, 255},
		{0, 200, 0, 255},
		{200, 0, 200, 255},
		{200, 0, 0, 255},
		{0, 0, 200, 255},
	}
	n := len(bars)
	for x := 0; x < w; x++ {
		c := bars[x*n/w]
		for y := 0; y < h; y++ {
			img.SetRGBA(x, y, c)
		}
	}
	drawNameBox(img, name)
	return img
}

// drawNameBox draws the device name in white text on a black box centered in
// the image.
func drawNameBox(img *image.RGBA, name string) {
	if name == "" {
		return
	}
	face := basicfont.Face7x13
	tw := xfont.MeasureString(face, name).Ceil()
	th := face.Metrics().Height.Ceil()
	if tw <= 0 || th <= 0 {
		return
	}

	// Render the text once at 1x into a temporary mask.
	tmp := image.NewRGBA(image.Rect(0, 0, tw, th))
	d := &xfont.Drawer{
		Dst:  tmp,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(name)

	const scale, pad = 2, 6
	dw, dh := tw*scale, th*scale
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	dx0 := (W - dw) / 2
	dy0 := (H - dh) / 2

	// Black box behind the text.
	black := color.RGBA{0, 0, 0, 255}
	for y := dy0 - pad; y < dy0+dh+pad; y++ {
		for x := dx0 - pad; x < dx0+dw+pad; x++ {
			if x >= 0 && x < W && y >= 0 && y < H {
				img.SetRGBA(x, y, black)
			}
		}
	}

	// White text, scaled up (nearest-neighbour).
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			if _, _, _, a := tmp.At(x, y).RGBA(); a == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px, py := dx0+x*scale+sx, dy0+y*scale+sy
					if px >= 0 && px < W && py >= 0 && py < H {
						img.SetRGBA(px, py, white)
					}
				}
			}
		}
	}
}

type monitorStats struct {
	lastReport time.Time
	lastEvent  time.Time
	events     int
	maxGap     time.Duration
	consumed   int
	starved    int
	trimmed    int
	arrived    atomic.Int64
}

func debugLogEnabled() bool {
	return strings.EqualFold(config.GetConfig().LogLevel, "debug")
}

type dropHit struct {
	rect   image.Rectangle
	devIdx int
}

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
	frameQueue   []monitorFrame
	shownFrame   monitorFrame
	nextFrameDue time.Time
	videoActive  bool
	audioPlaying bool
	audioStopCh  chan struct{}
	audioMonitor bool
	videoMonitor bool
	recording    bool
	casting      bool
	overlayOn    bool
	crtOn        bool
	recRenderer  *streams.StreamRenderer
	castRenderer *streams.StreamRenderer
	rawFrameCh   chan rawFrameMsg
	audioBtn     widget.Clickable
	videoBtn     widget.Clickable
	recBtn       widget.Clickable
	castBtn      widget.Clickable
	overlayBtn   widget.Clickable
	snapBtn      widget.Clickable
	crtBtn       widget.Clickable
	resetBtn     widget.Clickable
	poweroffBtn  widget.Clickable
	cardClick    widget.Clickable
	monitorClick widget.Clickable
	pauseBtn     widget.Clickable
	paused       bool

	testCardOp    paint.ImageOp // per-device test pattern with its name box (lazily built)
	testCardReady bool

	online      bool
	lastChecked time.Time
}

var (
	colorBackground = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	colorText       = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	colorActive     = color.NRGBA{R: 103, G: 255, B: 69, A: 255}
	colorInactive   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorToggleOff  = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	colorStrike     = color.NRGBA{R: 254, G: 0, B: 0, A: 255}
	colorHoverWhite = color.NRGBA{R: 150, G: 255, B: 130, A: 255}
	colorHoverGray  = color.NRGBA{R: 0, G: 160, B: 0, A: 255}
	colorWaveformBg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	colorWaveformFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorSeparator  = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	colorCardBg     = color.NRGBA{R: 45, G: 45, B: 45, A: 255}
	colorButtonBg   = color.NRGBA{R: 70, G: 70, B: 70, A: 255}

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
	iconPause, _         = widget.NewIcon(icons.AVPause)
	iconReset, _         = widget.NewIcon(icons.NavigationRefresh)
	iconPower, _         = widget.NewIcon(icons.ActionPowerSettingsNew)
	iconKeyboard, _      = widget.NewIcon(icons.HardwareKeyboard)
	iconExpand, _        = widget.NewIcon(icons.NavigationFullscreen)
	iconExpandExit, _    = widget.NewIcon(icons.NavigationFullscreenExit)
	colorRecording       = color.NRGBA{R: 255, G: 40, B: 40, A: 255}
	colorCasting         = color.NRGBA{R: 255, G: 215, B: 0, A: 255}
)

func Run() {
	go func() {
		a := newApp()
		go a.onlineCheckLoop()
		enableFileDrop("go64u - monitor", func(x, y int, name string, data []byte) {
			go a.handleDrop(x, y, name, data)
		})

		a.run()
		os.Exit(0)
	}()
	app.Main()
}

func (a *guiApp) handleDrop(x, y int, name string, data []byte) {
	var devIdx = -1
	a.dropMu.Lock()
	for _, h := range a.dropHits {
		if x >= h.rect.Min.X && x < h.rect.Max.X && y >= h.rect.Min.Y && y < h.rect.Max.Y {
			devIdx = h.devIdx
			break
		}
	}
	a.dropMu.Unlock()

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))

	if isDiskImage(ext) {
		device := a.dropTargetDevice(devIdx)
		if device == nil {
			return
		}
		a.mountAndAutoload(device, name)
		return
	}

	if devIdx < 0 {
		commands.Run(data)
		return
	}
	device := a.devices[devIdx].device
	network.SendHttpRequest(&network.HttpConfig{
		URL:     fmt.Sprintf("http://%s/v1/runners:run_prg", device.IpAddress),
		Method:  http.MethodPost,
		Payload: data,
	})
}

func isDiskImage(ext string) bool {
	switch ext {
	case "d64", "g64", "d71", "g71", "d81":
		return true
	}
	return false
}

// dropTargetDevice resolves the device a drop should act on: the panel that was
// hit, or the currently selected device when the drop landed outside any panel.
func (a *guiApp) dropTargetDevice(devIdx int) *config.Device {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if devIdx < 0 {
		devIdx = a.selectedIdx
	}
	if devIdx < 0 || devIdx >= len(a.devices) {
		return nil
	}
	return a.devices[devIdx].device
}

func (a *guiApp) mountAndAutoload(device *config.Device, filename string) {
	commands.UnmountDiskImage("A")
	time.Sleep(750 * time.Millisecond)
	commands.Reset()
	time.Sleep(1200 * time.Millisecond)
	commands.MountDiskImage([]string{"A", filename})

	time.Sleep(750 * time.Millisecond)
	codes := append([]byte(`LOAD"*",8,1`), 13)
	codes = append(codes, []byte("RUN")...)
	codes = append(codes, 13)
	sendKeystrokes(device.IpAddress, codes)
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

	c64Face, err := opentype.Parse(fonts.C64ProMonoSTYLE)
	if err != nil {
		panic(err)
	}
	shaper := text.NewShaper(text.WithCollection([]font.FontFace{
		{Font: font.Font{Typeface: C64ProTypeface}, Face: c64Face},
	}))

	th := material.NewTheme()
	th.Shaper = shaper
	th.TextSize = unit.Sp(16)
	th.Palette.Fg = colorText
	th.Palette.Bg = colorBackground

	devices := BuildDeviceList()
	hasOverlay := config.GetConfig().Overlay.ImagePath != ""
	for i := range devices {
		devices[i].audioMonitor = true
		devices[i].videoMonitor = true
		devices[i].overlayOn = hasOverlay
		devices[i].crtOn = devices[i].device.CrtMode
	}

	kb, err := NewVirtualKeyboard(nil)
	if err != nil {
		panic(err)
	}

	maxW, maxH := computeTargetSize(kb, len(devices), true)

	w := new(app.Window)
	opts := []app.Option{
		app.Title("go64u - monitor"),
		app.Size(maxW, maxH),
	}
	if runtime.GOOS == "linux" {
		opts = append(opts, app.MinSize(maxW, maxH), app.MaxSize(maxW, maxH))
	}
	w.Option(opts...)

	selectedIdx := 0
	selectedName := config.GetConfig().SelectedDevice
	for i := range devices {
		if devices[i].name == selectedName {
			selectedIdx = i
			break
		}
	}

	a := &guiApp{
		window:          w,
		theme:           th,
		otoCtx:          otoCtx,
		devices:         devices,
		selectedIdx:     selectedIdx,
		keyboard:        kb,
		keyboardVisible: true,
		linuxFixedSize:  runtime.GOOS == "linux",
		// A single configured device has no cards to switch between, so start in
		// expand mode and show its monitor full-size.
		expanded: len(devices) == 1,
	}
	// Wire the keyboard after the app exists so it can route to the selected device.
	kb.AddListener(KeyboardListener(kb, a))
	return a
}

// selectedDeviceIP returns the IP address of the currently selected device,
// or "" if none is selected. The virtual/physical keyboard targets this device.
func (a *guiApp) selectedDeviceIP() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.selectedIdx >= 0 && a.selectedIdx < len(a.devices) {
		return a.devices[a.selectedIdx].device.IpAddress
	}
	return ""
}

// indexOf returns the index of dev within a.devices, or -1 if not found.
func (a *guiApp) indexOf(dev *deviceUI) int {
	for i := range a.devices {
		if &a.devices[i] == dev {
			return i
		}
	}
	return -1
}

// isSelected reports whether dev is the currently selected device.
func (a *guiApp) isSelected(idx int) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.selectedIdx == idx
}

func (a *guiApp) onlineCheckLoop() {
	a.checkAllDevices()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		a.checkAllDevices()
	}
}

func (a *guiApp) checkAllDevices() {
	var wg sync.WaitGroup
	var anyChanged bool
	var toStart []int
	var mu sync.Mutex
	for i := range a.devices {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			dev := &a.devices[idx]
			online := network.IsDeviceOnline(dev.device, 2*time.Second)
			a.mu.Lock()
			wasOnline := dev.online
			changed := dev.online != online || dev.lastChecked.IsZero()
			dev.online = online
			dev.lastChecked = time.Now()
			active := dev.active
			a.mu.Unlock()
			if changed {
				mu.Lock()
				anyChanged = true
				// Auto-start streaming when a device transitions to online.
				if online && !wasOnline && !active && dev.device.IfOnlineAutostart {
					toStart = append(toStart, idx)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, idx := range toStart {
		a.autoStart(&a.devices[idx])
	}
	if anyChanged && a.window != nil {
		a.window.Invalidate()
	}
}

// autoStart begins audio+video streaming for a device that just came online,
// mirroring a click on its play button. No-op if already active.
func (a *guiApp) autoStart(dev *deviceUI) {
	if dev.active {
		return
	}
	dev.active = true
	a.startAudio(dev)
	fmt.Printf("Auto-started streams for %s (online)\n", dev.name)
}

func (a *guiApp) layoutOnlineIndicator(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(10))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	var col color.NRGBA
	a.mu.RLock()
	switch {
	case dev.lastChecked.IsZero():
		col = colorToggleOff
	case dev.online:
		col = colorActive
	default:
		col = colorStrike
	}
	a.mu.RUnlock()

	rr := clip.RRect{
		Rect: image.Rect(0, 0, size, size),
		SE:   size / 2, SW: size / 2, NE: size / 2, NW: size / 2,
	}
	defer rr.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func (a *guiApp) run() {
	var ops op.Ops

	var ftN int
	var ftLayout, ftLayoutMax, ftSubmit, ftSubmitMax time.Duration
	ftReport := time.Now()
	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			t0 := time.Now()
			a.layoutRoot(gtx)
			t1 := time.Now()
			e.Frame(gtx.Ops)
			t2 := time.Now()

			ftN++
			ftLayout += t1.Sub(t0)
			ftLayoutMax = max(ftLayoutMax, t1.Sub(t0))
			ftSubmit += t2.Sub(t1)
			ftSubmitMax = max(ftSubmitMax, t2.Sub(t1))
			if t2.Sub(ftReport) >= time.Second && ftN > 0 {
				if debugLogEnabled() {
					fmt.Printf("[frame] n=%d layout avg=%.1fms max=%.1fms | submit avg=%.1fms max=%.1fms\n",
						ftN,
						float64(ftLayout.Microseconds())/float64(ftN)/1000.0,
						float64(ftLayoutMax.Microseconds())/1000.0,
						float64(ftSubmit.Microseconds())/float64(ftN)/1000.0,
						float64(ftSubmitMax.Microseconds())/1000.0)
				}
				ftN, ftLayout, ftLayoutMax, ftSubmit, ftSubmitMax = 0, 0, 0, 0, 0
				ftReport = t2
			}
		}
	}
}

func computeTargetSize(kb *VirtualKeyboard, monitorPanels int, keyboardVisible bool) (w, h unit.Dp) {
	const (
		insetH      = unit.Dp(20)
		insetW      = unit.Dp(20)
		cardsW      = unit.Dp(280)
		columnGapW  = unit.Dp(10)
		toolbarH    = unit.Dp(80)
		topSpacerH  = unit.Dp(10)
		kbSpacerH   = unit.Dp(10)
		kbH         = unit.Dp(7 * 25)
		botSpacerH  = unit.Dp(6)
		separatorH  = unit.Dp(1)
		footSpacerH = unit.Dp(4)
		footerH     = unit.Dp(20)
		defaultW    = unit.Dp(800)
	)

	// With a single configured device the left device-card column is hidden, so
	// it must not be reserved in the window width either.
	leftW := cardsW + columnGapW
	if monitorPanels <= 1 {
		leftW = 0
	}
	rightW := defaultW - insetW - leftW
	if kb != nil {
		if kbW := kb.MaxWidthDp(); kbW > rightW {
			rightW = kbW
		}
	}
	w = insetW + leftW + rightW
	if w < defaultW {
		w = defaultW
	}

	h = insetH + toolbarH + topSpacerH + botSpacerH + separatorH + footSpacerH + footerH
	if monitorPanels > 0 {
		cols, rows := monitorGrid(monitorPanels)
		gapDp := unit.Dp(10)
		cellW := (rightW - unit.Dp(cols-1)*gapDp) / unit.Dp(cols)
		cellH := unit.Dp(float32(cellW) * float32(imaging.HEIGHT) / float32(imaging.WIDTH))
		h += unit.Dp(rows)*cellH + unit.Dp(rows-1)*gapDp
	}
	if keyboardVisible {
		h += kbSpacerH + kbH
	}
	return w, h
}

func (a *guiApp) syncWindowSize() {
	if a.linuxFixedSize {
		return
	}
	// Size for the full device grid regardless of expansion state, so toggling
	// expand never changes the computed size — otherwise expanding then
	// reverting would snap the window back and clobber a manual resize.
	a.mu.RLock()
	panels := len(a.devices)
	a.mu.RUnlock()
	w, h := computeTargetSize(a.keyboard, panels, a.keyboardVisible)
	if w == a.lastWindowW && h == a.lastWindowH {
		return
	}
	a.lastWindowW = w
	a.lastWindowH = h
	a.window.Option(app.Size(w, h))
}

func (a *guiApp) layoutRoot(gtx layout.Context) layout.Dimensions {

	a.syncWindowSize()

	event.Op(gtx.Ops, &a.keyFocusTag)
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: &a.keyFocusTag},
			key.Filter{Focus: &a.keyFocusTag, Optional: key.ModShift | key.ModCtrl | key.ModAlt | key.ModCommand},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press || a.keyboard == nil {
			continue
		}
		a.keyboard.HandlePhysicalKey(ke.Name, ke.Modifiers)
	}
	if !gtx.Focused(&a.keyFocusTag) {
		gtx.Execute(key.FocusCmd{Tag: &a.keyFocusTag})
	}

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorBackground)

	for i := range a.devices {
		if a.devices[i].cardClick.Clicked(gtx) {
			a.selectedIdx = i
			config.GetConfig().SelectedDevice = a.devices[i].name
			a.window.Invalidate() // redraw now so the selection highlight updates immediately
		}
	}

	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := a.layoutTopToolbar(gtx)
				a.toolbarHeightPx = dims.Size.Y
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				showMonitor := a.monitorPanelCount() > 0
				// Hide the device-card column when only one device is configured;
				// there is nothing to switch between and the monitor uses the full
				// width instead.
				showCards := len(a.devices) > 1

				monitorCol := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if showMonitor {
								return a.layoutVideoMonitor(gtx)
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !a.keyboardVisible || a.keyboard == nil {
								return layout.Dimensions{}
							}
							return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return a.keyboard.Layout(a.theme, gtx)
							})
						}),
					)
				})

				if !showCards {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx, monitorCol)
				}

				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cardW := gtx.Dp(unit.Dp(280))
						gtx.Constraints.Max.X = cardW
						gtx.Constraints.Min.X = cardW
						children := make([]layout.FlexChild, 0, len(a.devices)*2)
						for i := range a.devices {
							idx := i
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.layoutDeviceCard(gtx, idx)
							}))
							if i < len(a.devices)-1 {
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx)
								}))
							}
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
					}),

					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),

					monitorCol,
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(unit.Dp(1))
				sz := image.Pt(gtx.Constraints.Max.X, h)
				defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
				paint.Fill(gtx.Ops, colorSeparator)
				return layout.Dimensions{Size: sz}
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutFooter(gtx)
			}),
		)
	})
}

func (a *guiApp) layoutFooter(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		text := a.hint
		if text == "" {
			text = " "
		}
		lbl := material.Label(a.theme, unit.Sp(13), text)
		lbl.Color = colorToggleOff
		return lbl.Layout(gtx)
	})
}

func layoutGrayIcon(gtx layout.Context, icon *widget.Icon, sizeDp unit.Dp) layout.Dimensions {
	size := gtx.Dp(sizeDp)
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	icon.Layout(gtx, colorToggleOff)
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func (a *guiApp) layoutTopToolbar(gtx layout.Context) layout.Dimensions {
	if a.selectedIdx < 0 || a.selectedIdx >= len(a.devices) {
		return layout.Dimensions{}
	}
	dev := &a.devices[a.selectedIdx]
	enabled := dev.online

	toggleClicked := dev.toggle.Clicked(gtx)
	pauseClicked := dev.pauseBtn.Clicked(gtx)
	resetClicked := dev.resetBtn.Clicked(gtx)
	poweroffClicked := dev.poweroffBtn.Clicked(gtx)
	audioClicked := dev.audioBtn.Clicked(gtx)
	videoClicked := dev.videoBtn.Clicked(gtx)
	crtClicked := dev.crtBtn.Clicked(gtx)
	overlayClicked := dev.overlayBtn.Clicked(gtx)
	recClicked := dev.recBtn.Clicked(gtx)
	castClicked := dev.castBtn.Clicked(gtx)

	if a.keyboardBtn.Clicked(gtx) {
		a.keyboardVisible = !a.keyboardVisible
	}
	if a.expandBtn.Clicked(gtx) {
		a.expanded = !a.expanded
		a.window.Invalidate()
	}

	a.hint = ""
	switch {
	case dev.toggle.Hovered():
		a.hint = "Start / stop the Ultimate stream"
	case dev.pauseBtn.Hovered():
		a.hint = "Pause / resume the machine"
	case dev.audioBtn.Hovered():
		a.hint = "Toggle local audio monitoring"
	case dev.videoBtn.Hovered():
		a.hint = "Toggle local video monitoring"
	case dev.snapBtn.Hovered():
		a.hint = "Take a screenshot"
	case dev.recBtn.Hovered():
		a.hint = "Record the stream to a file"
	case dev.castBtn.Hovered():
		a.hint = "Cast the stream to a streaming platform"
	case dev.overlayBtn.Hovered():
		a.hint = "Toggle overlay image on cast / recording"
	case dev.crtBtn.Hovered():
		a.hint = "Toggle CRT effect on cast / recording"
	case a.expandBtn.Hovered():
		a.hint = "Expand the selected monitor to full size"
	case dev.resetBtn.Hovered():
		a.hint = "Reset the machine"
	case dev.poweroffBtn.Hovered():
		a.hint = "Power off the machine"
	case a.keyboardBtn.Hovered():
		a.hint = "Toggle virtual keyboard"
	}

	if enabled {
		if toggleClicked {
			dev.active = !dev.active
			if dev.active {
				a.startAudio(dev)
			} else {
				a.stopAudio(dev)
			}
		}

		if audioClicked {
			dev.audioMonitor = !dev.audioMonitor
			if dev.active {
				if dev.audioMonitor && !dev.audioPlaying {
					a.startAudioReader(dev)
				} else if !dev.audioMonitor && dev.audioPlaying {
					a.stopAudioReader(dev)
				}
			}
		}
		if videoClicked {
			dev.videoMonitor = !dev.videoMonitor
			if dev.active {
				if dev.videoMonitor && !dev.videoActive {
					a.startVideo(dev)
				} else if !dev.videoMonitor && dev.videoActive {
					a.stopVideo(dev)
				}
			}
		}
		if crtClicked {
			dev.crtOn = !dev.crtOn
			if dev.castRenderer != nil {
				dev.castRenderer.SetCrt(dev.crtOn)
			}
			if dev.recRenderer != nil {
				dev.recRenderer.SetCrt(dev.crtOn)
			}
		}
		if overlayClicked {
			dev.overlayOn = !dev.overlayOn
			if dev.castRenderer != nil {
				dev.castRenderer.SetOverlay(dev.overlayOn)
			}
			if dev.recRenderer != nil {
				dev.recRenderer.SetOverlay(dev.overlayOn)
			}
		}
		if recClicked {
			if dev.active {
				if !dev.recording {
					a.startRecording(dev)
				} else {
					a.stopRecording(dev)
				}
			}
		}
		if castClicked {
			if dev.active {
				if !dev.casting {
					a.startCasting(dev)
				} else {
					a.stopCasting(dev)
				}
			}
		}

		if resetClicked {
			commands.Reset()
		}
		if poweroffClicked {

			if dev.casting {
				a.stopCasting(dev)
			}
			if dev.recording {
				a.stopRecording(dev)
			}
			if dev.videoActive {
				a.stopVideo(dev)
			}
			if dev.active {
				a.stopAudio(dev)
			}
			// Clear active so that when the device powers back on the online
			// check sees an inactive device and re-triggers autostart. Without
			// this, active stays true, the autostart gate (!active) fails, and
			// the test card stays up instead of the resumed video.
			dev.active = false
			commands.PowerOff()
			dev.online = false
			dev.lastChecked = time.Now()
		}
		if pauseClicked {
			dev.paused = !dev.paused
			if dev.paused {
				commands.Pause()
			} else {
				commands.Resume()
			}
		}
	}

	enabled = dev.online

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			borderRadius := gtx.Dp(unit.Dp(8))
			borderWidth := gtx.Dp(unit.Dp(3))

			rr := clip.RRect{
				Rect: image.Rect(0, 0, sz.X, sz.Y),
				SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
			}
			defer rr.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, colorCardBg)

			strokeRR := clip.RRect{
				Rect: image.Rect(0, 0, sz.X, sz.Y),
				SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
			}
			defer clip.Stroke{Path: strokeRR.Path(gtx.Ops), Width: float32(borderWidth)}.Op().Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, colorSeparator)

			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconPlay, unit.Dp(40))
						}
						return a.layoutToggle(gtx, dev)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconPause, unit.Dp(28))
						}
						return a.layoutActionIconButton(gtx, &dev.pauseBtn, iconPause, dev.paused)
					}),

					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutActionIconButton(gtx, &a.keyboardBtn, iconKeyboard, a.keyboardVisible)
					}),

					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconMute, unit.Dp(28))
						}
						return a.layoutIconButton(gtx, &dev.audioBtn, dev.audioMonitor, dev.active, iconMusic, iconMute)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconEyeOff, unit.Dp(28))
						}
						return a.layoutIconButton(gtx, &dev.videoBtn, dev.videoMonitor, dev.active, iconEye, iconEyeOff)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconCamera, unit.Dp(28))
						}
						return a.layoutSnapshotButton(gtx, dev)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconRecord, unit.Dp(28))
						}
						return a.layoutRecordButton(gtx, dev)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconCast, unit.Dp(28))
						}
						return a.layoutCastButton(gtx, dev)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconOverlayOff, unit.Dp(28))
						}
						return a.layoutIconButton(gtx, &dev.overlayBtn, dev.overlayOn, dev.active, iconOverlay, iconOverlayOff)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconCrt, unit.Dp(28))
						}
						return a.layoutIconButton(gtx, &dev.crtBtn, dev.crtOn, dev.active, iconCrt, iconCrt)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// View-only toggle: always enabled and white; the icon
						// reflects the current expand state.
						icon := iconExpand
						if a.expanded {
							icon = iconExpandExit
						}
						return a.layoutIconButton(gtx, &a.expandBtn, true, true, icon, icon)
					}),

					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutWaveform(gtx, dev)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconReset, unit.Dp(28))
						}
						return a.layoutActionIconButton(gtx, &dev.resetBtn, iconReset, false)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !enabled {
							return layoutGrayIcon(gtx, iconPower, unit.Dp(28))
						}
						return a.layoutActionIconButton(gtx, &dev.poweroffBtn, iconPower, false)
					}),
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		}),
	)
}

func (a *guiApp) layoutDeviceCard(gtx layout.Context, index int) layout.Dimensions {
	dev := &a.devices[index]

	selected := a.selectedIdx == index
	borderCol := colorSeparator
	if selected {
		borderCol = colorInactive
	}

	return dev.cardClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Constraints.Min
				borderRadius := gtx.Dp(unit.Dp(8))
				borderWidth := gtx.Dp(unit.Dp(3))

				rr := clip.RRect{
					Rect: image.Rect(0, 0, sz.X, sz.Y),
					SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
				}
				defer rr.Push(gtx.Ops).Pop()
				paint.Fill(gtx.Ops, colorCardBg)

				strokeRR := clip.RRect{
					Rect: image.Rect(0, 0, sz.X, sz.Y),
					SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
				}
				defer clip.Stroke{Path: strokeRR.Path(gtx.Ops), Width: float32(borderWidth)}.Op().Push(gtx.Ops).Pop()
				paint.Fill(gtx.Ops, borderCol)

				return layout.Dimensions{Size: sz}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,

						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(18), dev.description)
									lbl.Color = colorText
									return lbl.Layout(gtx)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return a.layoutOnlineIndicator(gtx, dev)
								}),
							)
						}),
					)
				})
			}),
		)
	})
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

func (a *guiApp) layoutActionIconButton(gtx layout.Context, btn *widget.Clickable, icon *widget.Icon, highlighted bool) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	hovered := btn.Hovered()
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var col color.NRGBA
		switch {
		case hovered:
			col = colorHoverWhite
		case highlighted:
			col = colorActive
		default:
			col = colorInactive
		}
		icon.Layout(gtx, col)
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

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
	w := gtx.Constraints.Max.X
	h := gtx.Dp(unit.Dp(40))
	gap := gtx.Dp(unit.Dp(6))
	sz := image.Pt(w, h)
	gtx.Constraints = layout.Exact(sz)

	halfH := float32(h) / 2
	channelW := (w - gap) / 2

	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()

	func() {
		defer clip.Rect{Max: image.Pt(channelW, h)}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, colorWaveformBg)
	}()
	func() {
		defer clip.Rect{Min: image.Pt(channelW+gap, 0), Max: image.Pt(2*channelW+gap, h)}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, colorWaveformBg)
	}()

	a.mu.RLock()
	data := dev.waveform
	a.mu.RUnlock()

	if len(data) > 8 {

		sampleCount := (len(data) - 6) / 4
		if sampleCount < 1 {
			sampleCount = 1
		}
		xStep := float32(channelW) / float32(sampleCount)

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
			x += xStep
			if x >= float32(channelW) {
				break
			}
		}
		specLeft := pathLeft.End()
		func() {
			defer clip.Stroke{Path: specLeft, Width: 1}.Op().Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, colorWaveformFg)
		}()

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
			x += xStep
			if x >= float32(channelW) {
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

func (a *guiApp) layoutVideoMonitor(gtx layout.Context) layout.Dimensions {

	const framePeriod = 20 * time.Millisecond
	stats := &a.monStats

	// One panel per online/streaming device (plus the selected one), each
	// showing its own live stream or the test pattern. Laid out in a 2×n grid.
	type panel struct {
		idx int
		op  paint.ImageOp
	}
	var panels []panel
	anyLive := false

	a.mu.Lock()
	for _, i := range a.monitorPanelsLocked() {
		dev := &a.devices[i]
		confirmedOffline := !dev.online && !dev.lastChecked.IsZero()
		pop := a.deviceTestCard(dev)
		live := false
		if dev.videoActive {
			if !gtx.Now.Before(dev.nextFrameDue) {
				if len(dev.frameQueue) > 0 {
					dev.shownFrame = dev.frameQueue[0]
					dev.frameQueue = dev.frameQueue[1:]
					if gtx.Now.Sub(dev.nextFrameDue) > 5*framePeriod {
						dev.nextFrameDue = gtx.Now
					}
					dev.nextFrameDue = dev.nextFrameDue.Add(framePeriod)
					stats.consumed++
				} else {
					stats.starved++
				}
			}
			if n := len(dev.frameQueue); n > 2 {
				dev.frameQueue = dev.frameQueue[n-2:]
				stats.trimmed += n - 2
			}
			if dev.shownFrame.img != nil && !confirmedOffline {
				pop = dev.shownFrame.op
				live = true
			}
		}
		panels = append(panels, panel{idx: i, op: pop})
		if live {
			anyLive = true
		}
	}
	a.mu.Unlock()

	if len(panels) == 0 {
		return layout.Dimensions{}
	}

	// Keep redrawing at display rate while any panel shows live video (pacing).
	if anyLive {
		gtx.Execute(op.InvalidateCmd{})

		stats.events++
		if !stats.lastEvent.IsZero() {
			if gap := gtx.Now.Sub(stats.lastEvent); gap > stats.maxGap {
				stats.maxGap = gap
			}
		}
		stats.lastEvent = gtx.Now
		if stats.lastReport.IsZero() {
			stats.lastReport = gtx.Now
		} else if gtx.Now.Sub(stats.lastReport) >= time.Second {
			if debugLogEnabled() {
				fmt.Printf("[monitor] ui=%d/s maxgap=%.1fms arrived=%d consumed=%d starved=%d trimmed=%d\n",
					stats.events, float64(stats.maxGap.Microseconds())/1000.0,
					stats.arrived.Swap(0), stats.consumed, stats.starved, stats.trimmed)
			} else {
				stats.arrived.Store(0)
			}
			stats.events, stats.consumed, stats.starved, stats.trimmed = 0, 0, 0, 0
			stats.maxGap = 0
			stats.lastReport = gtx.Now
		}
	} else {
		stats.lastEvent = time.Time{}
		stats.lastReport = time.Time{}
	}

	cols, rows := monitorGrid(len(panels))

	totalW := gtx.Constraints.Max.X
	if a.keyboard != nil && a.keyboardVisible {
		if minW := a.keyboard.MaxWidth(gtx); totalW < minW {
			totalW = minW
		}
	}
	gap := gtx.Dp(unit.Dp(10))
	cellW := (totalW - (cols-1)*gap) / cols
	cellH := cellW * imaging.HEIGHT / imaging.WIDTH
	totalH := rows*cellH + (rows-1)*gap
	sz := image.Pt(totalW, totalH)
	gtx.Constraints = layout.Exact(sz)

	monitorOriginX := gtx.Dp(unit.Dp(10)) + gtx.Dp(unit.Dp(280)) + gtx.Dp(unit.Dp(10))
	monitorOriginY := gtx.Dp(unit.Dp(10)) + a.toolbarHeightPx + gtx.Dp(unit.Dp(10))

	hits := make([]dropHit, 0, len(panels))
	borderWidth := gtx.Dp(unit.Dp(3))
	borderRadius := gtx.Dp(unit.Dp(8))
	for n, p := range panels {
		col := n % cols
		row := n / cols
		offX := col * (cellW + gap)
		offY := row * (cellH + gap)
		hits = append(hits, dropHit{
			rect:   image.Rect(monitorOriginX+offX, monitorOriginY+offY, monitorOriginX+offX+cellW, monitorOriginY+offY+cellH),
			devIdx: p.idx,
		})

		// Clicking a monitor panel selects that device (same as clicking its
		// card), so the keyboard/audio follow what the user is looking at.
		devIdx := p.idx
		if a.devices[devIdx].monitorClick.Clicked(gtx) {
			a.selectedIdx = devIdx
			config.GetConfig().SelectedDevice = a.devices[devIdx].name
			a.window.Invalidate() // redraw now so the selection border updates immediately
		}

		stack := op.Offset(image.Pt(offX, offY)).Push(gtx.Ops)

		borderCol := colorSeparator
		if len(panels) > 1 && devIdx == a.selectedIdx {
			borderCol = colorInactive
		}
		rrect := clip.RRect{
			Rect: image.Rect(0, 0, cellW, cellH),
			SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
		}
		borderStack := clip.Stroke{Path: rrect.Path(gtx.Ops), Width: float32(borderWidth)}.Op().Push(gtx.Ops)
		paint.Fill(gtx.Ops, borderCol)
		borderStack.Pop()

		clipStack := clip.RRect{
			Rect: image.Rect(0, 0, cellW, cellH),
			SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
		}.Push(gtx.Ops)

		fsz := p.op.Size()
		scaleX := float32(cellW) / float32(fsz.X)
		scaleY := float32(cellH) / float32(fsz.Y)
		p.op.Add(gtx.Ops)
		t := f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scaleX, scaleY))
		// Scope the scale transform so it doesn't leak past clipStack.Pop into
		// the click overlay below — an unscoped Add would scale/shift the
		// panel's hit area and make monitor clicks miss.
		aff := op.Affine(t).Push(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		aff.Pop()

		clipStack.Pop()

		// Transparent click overlay covering the cell so clicking the monitor
		// selects this device. Registered after drawing so it sits on top for
		// hit-testing; it draws nothing itself.
		cgtx := gtx
		cgtx.Constraints = layout.Exact(image.Pt(cellW, cellH))
		a.devices[devIdx].monitorClick.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(cellW, cellH)}
		})

		stack.Pop()
	}

	a.dropMu.Lock()
	a.dropHits = hits
	a.dropMu.Unlock()

	return layout.Dimensions{Size: sz}
}

const videoMonitorScale = 3

type monitorFrame struct {
	op  paint.ImageOp
	img *image.RGBA
}

const monitorBufs = 5

type guiVideoRenderer struct {
	app         *guiApp
	dev         *deviceUI
	reusableImg *imaging.ReusablePalettedImage
	lut         [16]color.NRGBA

	nativeFrames [monitorBufs]*image.RGBA
	scaledFrames [monitorBufs]*image.RGBA
	backIdx      int

	// CRT bloom scratch buffers (native-resolution RGB), lazily allocated.
	glowSrc []byte // thresholded bright pixels
	glowTmp []byte // separable-blur intermediate
	glowBuf []byte // blurred glow added during compose
}

func newGuiVideoRenderer(a *guiApp, dev *deviceUI) *guiVideoRenderer {
	r := &guiVideoRenderer{
		app:         a,
		dev:         dev,
		reusableImg: imaging.NewReusablePalettedImage(),
	}
	for i, c := range util.GetPalette() {
		if i >= 16 {
			break
		}
		cr, cg, cb, ca := c.RGBA()
		r.lut[i] = color.NRGBA{R: uint8(cr >> 8), G: uint8(cg >> 8), B: uint8(cb >> 8), A: uint8(ca >> 8)}
	}
	return r
}

func (r *guiVideoRenderer) Init() error                 { return nil }
func (r *guiVideoRenderer) GetRunMode() streams.RunMode { return streams.Loop }
func (r *guiVideoRenderer) GetFPS() int                 { return 50 }
func (r *guiVideoRenderer) GetContext() context.Context { return context.Background() }

func (r *guiVideoRenderer) Render(data []byte) bool {
	if !r.dev.videoActive {
		return false
	}

	palImg := r.reusableImg.Decode(data)

	// CRT is strictly per-device: each device's frames are rendered with that
	// device's own toggle, independent of how many devices are streaming.
	crtOn := r.dev.crtOn

	r.backIdx = (r.backIdx + 1) % monitorBufs
	var frame *image.RGBA
	if crtOn {
		frame = r.renderScaled(palImg)
	} else {
		frame = r.renderNative(palImg)
	}

	imgOp := paint.NewImageOp(frame)
	if !crtOn {

		imgOp.Filter = paint.FilterNearest
	}

	r.app.monStats.arrived.Add(1)
	r.app.mu.Lock()
	q := append(r.dev.frameQueue, monitorFrame{op: imgOp, img: frame})
	if len(q) > 3 {
		q = q[len(q)-3:]
	}
	r.dev.frameQueue = q
	r.app.mu.Unlock()
	r.app.window.Invalidate()

	if r.dev.rawFrameCh != nil {
		rawCopy := make([]byte, len(data))
		copy(rawCopy, data)
		select {
		case r.dev.rawFrameCh <- rawFrameMsg{data: rawCopy, captureTime: time.Now()}:
		default:
		}
	}

	return true
}

func (r *guiVideoRenderer) renderNative(palImg *image.Paletted) *image.RGBA {
	frame := r.nativeFrames[r.backIdx]
	if frame == nil {
		frame = image.NewRGBA(image.Rect(0, 0, imaging.WIDTH, imaging.HEIGHT))
		r.nativeFrames[r.backIdx] = frame
	}
	srcPix := palImg.Pix
	srcStride := palImg.Stride
	dstPix := frame.Pix
	dstStride := frame.Stride
	for y := range imaging.HEIGHT {
		srcRow := y * srcStride
		dstRow := y * dstStride
		for x := range imaging.WIDTH {
			c := r.lut[srcPix[srcRow+x]&0x0F]
			off := dstRow + x*4
			dstPix[off] = c.R
			dstPix[off+1] = c.G
			dstPix[off+2] = c.B
			dstPix[off+3] = c.A
		}
	}
	return frame
}

// CRT bloom tuning.
const (
	glowThresh    = 40 // luminance above which a pixel contributes to the glow
	glowRadius    = 1  // native-resolution box-blur radius
	glowIntensity = 90 // additive glow strength, out of 256
)

func (r *guiVideoRenderer) renderScaled(palImg *image.Paletted) *image.RGBA {
	const scale = videoMonitorScale
	const w, h = imaging.WIDTH, imaging.HEIGHT
	frame := r.scaledFrames[r.backIdx]
	if frame == nil {
		frame = image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
		r.scaledFrames[r.backIdx] = frame
	}

	var factors [scale]uint16
	const minBright = 0.45
	pixelH := float32(scale)
	for i := range scale {
		fracY := float32(i) / pixelH
		bell := float32(math.Sin(float64(fracY) * math.Pi))
		f := minBright + (1.0-minBright)*bell
		factors[i] = uint16(f * 255)
	}

	srcPix := palImg.Pix
	srcStride := palImg.Stride

	// Build a thresholded "bright pixels" buffer at native resolution and blur
	// it; bright areas (text/graphics) then bleed a soft halo into the dark
	// scanline gaps, the classic CRT phosphor glow.
	if r.glowSrc == nil {
		r.glowSrc = make([]byte, w*h*3)
		r.glowTmp = make([]byte, w*h*3)
		r.glowBuf = make([]byte, w*h*3)
	}
	const span = 255 - glowThresh
	for y := range h {
		srcRow := y * srcStride
		for x := range w {
			c := r.lut[srcPix[srcRow+x]&0x0F]
			lum := (int(c.R)*54 + int(c.G)*183 + int(c.B)*19) >> 8
			o := (y*w + x) * 3
			if lum > glowThresh {
				wgt := lum - glowThresh // 0..span
				r.glowSrc[o] = byte(int(c.R) * wgt / span)
				r.glowSrc[o+1] = byte(int(c.G) * wgt / span)
				r.glowSrc[o+2] = byte(int(c.B) * wgt / span)
			} else {
				r.glowSrc[o], r.glowSrc[o+1], r.glowSrc[o+2] = 0, 0, 0
			}
		}
	}
	boxBlurRGB(r.glowSrc, r.glowTmp, r.glowBuf, w, h, glowRadius)

	dstPix := frame.Pix
	dstStride := frame.Stride
	for y := range h {
		srcRow := y * srcStride
		glowRow := y * w * 3
		for sy := range scale {
			dstRow := (y*scale + sy) * dstStride
			f := factors[sy]
			for x := range w {
				c := r.lut[srcPix[srcRow+x]&0x0F]
				gi := glowRow + x*3
				rr := clamp8(int(uint16(c.R)*f/255) + int(r.glowBuf[gi])*glowIntensity>>8)
				g := clamp8(int(uint16(c.G)*f/255) + int(r.glowBuf[gi+1])*glowIntensity>>8)
				b := clamp8(int(uint16(c.B)*f/255) + int(r.glowBuf[gi+2])*glowIntensity>>8)
				for sx := range scale {
					off := dstRow + (x*scale+sx)*4
					dstPix[off] = rr
					dstPix[off+1] = g
					dstPix[off+2] = b
					dstPix[off+3] = c.A
				}
			}
		}
	}
	return frame
}

func clamp8(v int) byte {
	if v > 255 {
		return 255
	}
	return byte(v)
}

// boxBlurRGB runs a separable box blur of the given radius over an RGB buffer,
// using tmp as the horizontal-pass scratch and writing the result to dst.
func boxBlurRGB(src, tmp, dst []byte, w, h, radius int) {
	div := 2*radius + 1
	// Horizontal pass: src -> tmp
	for y := range h {
		row := y * w * 3
		for x := range w {
			var sr, sg, sb int
			for k := -radius; k <= radius; k++ {
				xx := x + k
				if xx < 0 {
					xx = 0
				} else if xx >= w {
					xx = w - 1
				}
				o := row + xx*3
				sr += int(src[o])
				sg += int(src[o+1])
				sb += int(src[o+2])
			}
			o := row + x*3
			tmp[o] = byte(sr / div)
			tmp[o+1] = byte(sg / div)
			tmp[o+2] = byte(sb / div)
		}
	}
	// Vertical pass: tmp -> dst
	for y := range h {
		for x := range w {
			var sr, sg, sb int
			for k := -radius; k <= radius; k++ {
				yy := y + k
				if yy < 0 {
					yy = 0
				} else if yy >= h {
					yy = h - 1
				}
				o := (yy*w + x) * 3
				sr += int(tmp[o])
				sg += int(tmp[o+1])
				sb += int(tmp[o+2])
			}
			o := (y*w + x) * 3
			dst[o] = byte(sr / div)
			dst[o+1] = byte(sg / div)
			dst[o+2] = byte(sb / div)
		}
	}
}

func (a *guiApp) startVideo(dev *deviceUI) {
	if dev.videoActive {
		return
	}
	dev.videoActive = true
	streams.VideoStart(dev.device)

	reader := &streams.VideoReader{
		Device:   dev.device,
		Renderer: newGuiVideoRenderer(a, dev),
	}
	go reader.Read()
}

func (a *guiApp) stopVideo(dev *deviceUI) {
	if !dev.videoActive {
		return
	}
	dev.videoActive = false
	streams.VideoStop(dev.device)
	a.mu.Lock()
	dev.frameQueue = nil
	dev.shownFrame = monitorFrame{}
	a.mu.Unlock()
}

func (a *guiApp) layoutSnapshotButton(gtx layout.Context, dev *deviceUI) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	if dev.snapBtn.Clicked(gtx) && dev.active {

		a.mu.RLock()
		frame := dev.shownFrame.img
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
		switch {
		case dev.casting:
			iconCastConnected.Layout(gtx, colorCasting)
		case hovered:
			iconCast.Layout(gtx, colorHoverWhite)
		default:
			iconCast.Layout(gtx, colorInactive)
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
	devIdx := a.indexOf(dev)
	var lastWaveform time.Time
	audioReader := streams.AudioReader{
		Device:       dev.device,
		AudioContext: a.otoCtx,
		StopChan:     dev.audioStopCh,
		// Only the selected device is audible; others still read & forward
		// audio (waveform, recording, casting) but play silently.
		ShouldPlay: func() bool { return a.isSelected(devIdx) },
		Renderer: func(data []byte) {

			a.mu.RLock()
			rec := dev.recRenderer
			cast := dev.castRenderer
			a.mu.RUnlock()
			if rec != nil {
				rec.WriteAudio(data)
			}
			if cast != nil {
				cast.WriteAudio(data)
			}

			if time.Since(lastWaveform) >= 33*time.Millisecond {
				lastWaveform = time.Now()
				wf := make([]byte, len(data))
				copy(wf, data)
				a.mu.Lock()
				dev.waveform = wf
				a.mu.Unlock()
				a.window.Invalidate()
			}
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

	if dev.videoMonitor {
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
