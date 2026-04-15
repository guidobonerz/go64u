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
	"sort"
	"sync"
	"time"

	"drazil.de/go64u/commands"
	"drazil.de/go64u/config"
	"drazil.de/go64u/imaging"
	"drazil.de/go64u/network"
	"drazil.de/go64u/streams"
	"drazil.de/go64u/util"

	"gioui.org/app"
	"gioui.org/f32"
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
	window          *app.Window
	theme           *material.Theme
	otoCtx          *oto.Context
	devices         []deviceUI
	mu              sync.RWMutex
	selectedIdx     int    // index of the currently selected device card (-1 if none)
	hint            string // footer text, updated by layoutTopToolbar on hover
	toolbarHeightPx int    // measured toolbar height in physical pixels, for drop-hit geometry
	dropMu          sync.Mutex
	dropHits        []dropHit // rectangles (client-area px) of each visible monitor cell, updated each frame
}

// dropHit maps a monitor cell's on-screen rectangle (in Windows client-area
// pixels) to the originating device index. Used to route a file drop to the
// device whose monitor the drop landed on, independently of selection.
type dropHit struct {
	rect   image.Rectangle
	devIdx int
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
	resetBtn     widget.Clickable
	poweroffBtn  widget.Clickable
	cardClick    widget.Clickable
	pauseBtn     widget.Clickable
	paused       bool

	online      bool      // live device reachability status
	lastChecked time.Time // when the last online check completed
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
	colorRecording       = color.NRGBA{R: 255, G: 40, B: 40, A: 255}
	colorCasting         = color.NRGBA{R: 255, G: 215, B: 0, A: 255}
)

func Run() {
	go func() {
		a := newApp()
		go a.onlineCheckLoop()
		enableFileDrop("go64u - monitor", func(x, y int, data []byte) {
			go a.handleDrop(x, y, data)
		})

		a.run()
		os.Exit(0)
	}()
	app.Main()
}

// handleDrop routes a dropped file to the device whose monitor rect contains
// the drop point. Falls back to the currently selected device when the drop
// happens outside any visible monitor cell.
func (a *guiApp) handleDrop(x, y int, data []byte) {
	var devIdx = -1
	a.dropMu.Lock()
	for _, h := range a.dropHits {
		if x >= h.rect.Min.X && x < h.rect.Max.X && y >= h.rect.Min.Y && y < h.rect.Max.Y {
			devIdx = h.devIdx
			break
		}
	}
	a.dropMu.Unlock()

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

	// Use system sans-serif fonts (Helvetica / Arial / Segoe UI depending on platform)
	shaper := text.NewShaper()

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

	// 768x544 image pixels + room for audio panel + insets
	w.Option(app.Title("go64u - monitor"), app.Size(unit.Dp(800), unit.Dp(680)))

	// Start with the config's selected device highlighted (or first device)
	selectedIdx := 0
	selectedName := config.GetConfig().SelectedDevice
	for i := range devices {
		if devices[i].name == selectedName {
			selectedIdx = i
			break
		}
	}

	return &guiApp{
		window:      w,
		theme:       th,
		otoCtx:      otoCtx,
		devices:     devices,
		selectedIdx: selectedIdx,
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

// onlineCheckLoop polls all configured devices every 5 seconds to update
// their online status. Runs as a background goroutine for the entire app
// lifetime.
func (a *guiApp) onlineCheckLoop() {
	a.checkAllDevices() // immediate check on startup
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		a.checkAllDevices()
	}
}

// checkAllDevices probes all configured devices in parallel and updates
// their online state. Triggers a window redraw when any status changes.
func (a *guiApp) checkAllDevices() {
	var wg sync.WaitGroup
	var anyChanged bool
	var mu sync.Mutex
	for i := range a.devices {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			dev := &a.devices[idx]
			online := network.IsDeviceOnline(dev.device, 2*time.Second)
			a.mu.Lock()
			changed := dev.online != online || dev.lastChecked.IsZero()
			dev.online = online
			dev.lastChecked = time.Now()
			a.mu.Unlock()
			if changed {
				mu.Lock()
				anyChanged = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if anyChanged && a.window != nil {
		a.window.Invalidate()
	}
}

// layoutOnlineIndicator draws a small filled circle reflecting the device's
// online status: gray = not yet checked, green = online, red = offline.
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

	// Pre-pass: handle card-click selection BEFORE rendering any card, so all
	// cards see the same selectedIdx during this frame.
	for i := range a.devices {
		if a.devices[i].cardClick.Clicked(gtx) {
			a.selectedIdx = i
			config.GetConfig().SelectedDevice = a.devices[i].name
		}
	}

	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Top: context-dependent toolbar for the currently selected device
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := a.layoutTopToolbar(gtx)
				a.toolbarHeightPx = dims.Size.Y
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			// Middle: horizontal split -- cards on the left, video monitor on the right (monitor mode).
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				hasActiveVideo := false
				for i := range a.devices {
					if a.devices[i].videoActive {
						hasActiveVideo = true
						break
					}
				}

				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
					// Left column: vertical list of device cards
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
					// Spacer between left cards and right monitor
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					// Right column: video monitor (only when active in monitor mode)
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if hasActiveVideo {
							return a.layoutVideoMonitor(gtx)
						}
						return layout.Dimensions{}
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			// Footer: hint text for hovered toolbar icons
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutFooter(gtx)
			}),
		)
	})
}

// layoutFooter renders a thin status strip at the bottom of the window that
// displays the current hover hint (or a blank reserved line when nothing is
// hovered). a.hint is updated each frame in layoutTopToolbar.
func (a *guiApp) layoutFooter(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		text := a.hint
		if text == "" {
			text = " " // reserve the line height so the footer doesn't collapse
		}
		lbl := material.Label(a.theme, unit.Sp(13), text)
		lbl.Color = colorToggleOff
		return lbl.Layout(gtx)
	})
}

// layoutGrayIcon renders a plain gray-tinted material icon with a fixed
// square size. Used to show toolbar buttons in a disabled look when the
// selected device is offline. No clickable wrapper — so hover tracking is
// not available in this state.
func layoutGrayIcon(gtx layout.Context, icon *widget.Icon, sizeDp unit.Dp) layout.Dimensions {
	size := gtx.Dp(sizeDp)
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	icon.Layout(gtx, colorToggleOff)
	return layout.Dimensions{Size: image.Pt(size, size)}
}

// layoutTopToolbar renders a single horizontal toolbar at the top of the window
// that operates on the currently selected device. Handles click events for all
// toolbar buttons (play/stop, pause, monitor toggles, reset, poweroff).
func (a *guiApp) layoutTopToolbar(gtx layout.Context) layout.Dimensions {
	if a.selectedIdx < 0 || a.selectedIdx >= len(a.devices) {
		return layout.Dimensions{}
	}
	dev := &a.devices[a.selectedIdx]
	enabled := dev.online

	// Always drain click events so they don't queue up while disabled.
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

	// Update footer hint from current hover state.
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
	case dev.resetBtn.Hovered():
		a.hint = "Reset the machine"
	case dev.poweroffBtn.Hovered():
		a.hint = "Power off the machine"
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
			// Stop any active streams/casts/recordings before powering off,
			// then mark the device offline immediately so the toolbar disables
			// without waiting for the next online-check tick.
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

	// Re-read online state AFTER click handlers run, so a power-off click
	// (which flips dev.online to false) immediately disables the render path
	// below instead of waiting for the next frame.
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

// layoutDeviceCard renders a single device as a rounded card with:
//   - Top: device label + online indicator
//   - Bottom: waveform
//
// Button click handling for the toolbar now lives in layoutTopToolbar; the card
// body is purely presentational plus the cardClick for selection (handled in
// the pre-pass in layoutRoot).
func (a *guiApp) layoutDeviceCard(gtx layout.Context, index int) layout.Dimensions {
	dev := &a.devices[index]

	selected := a.selectedIdx == index
	borderCol := colorSeparator
	if selected {
		borderCol = colorInactive // white
	}

	// Draw card background + border, then content on top (wrapped in cardClick)
	return dev.cardClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Constraints.Min
				borderRadius := gtx.Dp(unit.Dp(8))
				borderWidth := gtx.Dp(unit.Dp(3))
				// Background fill

				rr := clip.RRect{
					Rect: image.Rect(0, 0, sz.X, sz.Y),
					SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
				}
				defer rr.Push(gtx.Ops).Pop()
				paint.Fill(gtx.Ops, colorCardBg)

				// Border stroke

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
						// Row 1: device label (left) + online indicator (top-right corner)
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

	// Overall clip for the whole waveform area
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()

	// Two separate backgrounds (left + right) with a visible gap between them
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
		// Scale x so samples span the full channelW, regardless of sample count.
		sampleCount := (len(data) - 6) / 4
		if sampleCount < 1 {
			sampleCount = 1
		}
		xStep := float32(channelW) / float32(sampleCount)

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
	// Collect active video streams with their originating device index so we
	// can highlight the selected device's monitor when multiple are shown.
	a.mu.RLock()
	var activeFrames []*image.NRGBA
	var activeIdx []int
	for i := range a.devices {
		if a.devices[i].videoActive && a.devices[i].videoFrame != nil {
			activeFrames = append(activeFrames, a.devices[i].videoFrame)
			activeIdx = append(activeIdx, i)
		}
	}
	a.mu.RUnlock()

	count := len(activeFrames)
	if count == 0 {
		count = 1 // reserve space for at least one panel
	}

	totalW := gtx.Constraints.Max.X
	gap := gtx.Dp(unit.Dp(10))
	totalGap := gap * (count - 1)
	cellW := (totalW - totalGap) / count
	cellH := cellW * imaging.HEIGHT / imaging.WIDTH
	sz := image.Pt(totalW, cellH)
	gtx.Constraints = layout.Exact(sz)

	monitorOriginX := gtx.Dp(unit.Dp(10)) + gtx.Dp(unit.Dp(280)) + gtx.Dp(unit.Dp(10))
	monitorOriginY := gtx.Dp(unit.Dp(10)) + a.toolbarHeightPx + gtx.Dp(unit.Dp(10))

	hits := make([]dropHit, 0, len(activeFrames))

	borderWidth := gtx.Dp(unit.Dp(3))
	borderRadius := gtx.Dp(unit.Dp(8))
	for i, frame := range activeFrames {
		absX := monitorOriginX + i*(cellW+gap)
		absY := monitorOriginY
		hits = append(hits, dropHit{
			rect:   image.Rect(absX, absY, absX+cellW, absY+cellH),
			devIdx: activeIdx[i],
		})
		offsetX := i * (cellW + gap)
		stack := op.Offset(image.Pt(offsetX, 0)).Push(gtx.Ops)

		borderCol := colorSeparator
		if len(activeFrames) > 1 && activeIdx[i] == a.selectedIdx {
			borderCol = colorInactive
		}
		rrect := clip.RRect{
			Rect: image.Rect(0, 0, cellW, cellH),
			SE:   borderRadius, SW: borderRadius, NE: borderRadius, NW: borderRadius,
		}
		borderStack := clip.Stroke{Path: rrect.Path(gtx.Ops), Width: float32(borderWidth)}.Op().Push(gtx.Ops)
		paint.Fill(gtx.Ops, borderCol)
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

	a.dropMu.Lock()
	a.dropHits = hits
	a.dropMu.Unlock()

	return layout.Dimensions{Size: sz}
}

const videoMonitorScale = 3

// guiVideoRenderer implements streams.Renderer for the in-app video monitor:
// it decodes raw paletted frames into a pre-scaled NRGBA buffer, applies the
// optional CRT scanline effect, and forwards raw frames to the cast/rec pipe.
type guiVideoRenderer struct {
	app         *guiApp
	dev         *deviceUI
	reusableImg *imaging.ReusablePalettedImage
	lut         [16]color.NRGBA
	nrgbaFrame  *image.NRGBA
}

func newGuiVideoRenderer(a *guiApp, dev *deviceUI) *guiVideoRenderer {
	r := &guiVideoRenderer{
		app:         a,
		dev:         dev,
		reusableImg: imaging.NewReusablePalettedImage(),
		nrgbaFrame:  image.NewNRGBA(image.Rect(0, 0, imaging.WIDTH*videoMonitorScale, imaging.HEIGHT*videoMonitorScale)),
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
	srcStride := palImg.Stride
	srcPix := palImg.Pix
	dstStride := r.nrgbaFrame.Stride
	dstPix := r.nrgbaFrame.Pix

	// Only show local scanlines in single-display mode (one device active)
	activeCount := 0
	r.app.mu.RLock()
	for i := range r.app.devices {
		if r.app.devices[i].videoActive {
			activeCount++
		}
	}
	r.app.mu.RUnlock()
	crtOn := r.dev.crtOn && activeCount == 1

	const scale = videoMonitorScale
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
				c := r.lut[srcPix[srcRow+x]&0x0F]
				rr := byte(uint16(c.R) * f / 255)
				g := byte(uint16(c.G) * f / 255)
				b := byte(uint16(c.B) * f / 255)
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

	r.app.mu.Lock()
	r.dev.videoFrame = r.nrgbaFrame
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
