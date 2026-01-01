package gui

import (
	"fmt"
	"sync"

	"drazil.de/go64u/config"
	"drazil.de/go64u/fonts"
	"drazil.de/go64u/streams"
	"drazil.de/go64u/util"

	"github.com/ebitengine/oto/v3"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/toolbox/v2/xos"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/align"
	"github.com/richardwilkes/unison/enums/paintstyle"
)

const playIcon = `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg width="100%" height="100%" viewBox="0 0 150 150" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" xml:space="preserve" xmlns:serif="http://www.serif.com/" style="fill-rule:evenodd;clip-rule:evenodd;stroke-linejoin:round;stroke-miterlimit:2;">
    <g transform="matrix(0.173272,0,0,0.173272,36.1881,30.6349)">
        <path d="M424.4,214.7L72.4,6.6C43.8,-10.3 0,6.1 0,47.9L0,464C0,501.5 40.7,524.1 72.4,505.3L424.4,297.3C455.8,278.8 455.9,233.2 424.4,214.7Z" style="fill:rgb(103,255,69);fill-rule:nonzero;"/>
    </g>
</svg>`
const stopIcon = `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg width="100%" height="100%" viewBox="0 0 150 150" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" xml:space="preserve" xmlns:serif="http://www.serif.com/" style="fill-rule:evenodd;clip-rule:evenodd;stroke-linejoin:round;stroke-miterlimit:2;">
    <g transform="matrix(0.198034,0,0,0.198034,30.6405,24.3034)">
        <path d="M400,32L48,32C21.5,32 0,53.5 0,80L0,432C0,458.5 21.5,480 48,480L400,480C426.5,480 448,458.5 448,432L448,80C448,53.5 426.5,32 400,32Z" style="fill:rgb(254,0,0);fill-rule:nonzero;"/>
    </g>
</svg>`

type startStream func(deviceName string)
type stopStream func(deviceName string)
type DrawingPanel struct {
	unison.Panel
	dataSourceName string
	data           []byte
}

var images map[string]*DrawingPanel

var synchronizer sync.RWMutex
var otoCtx *oto.Context
var fd unison.FontFaceDescriptor

func Run() {
	unison.Start(unison.StartupFinishedCallback(func() {
		_, err := build(unison.PrimaryDisplay().Usable.Point)
		xos.ExitIfErr(err)
	}))
}

func build(where geom.Point) (*unison.Window, error) {

	images = make(map[string]*DrawingPanel)

	op := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	}

	var readyChan chan struct{}
	var err error
	otoCtx, readyChan, err = oto.NewContext(op)
	if err != nil {
		panic(err)
	}

	<-readyChan

	wnd, err := unison.NewWindow("go64u - experimental gui")
	if err != nil {
		return nil, err
	}
	/*
		fontData, err := os.ReadFile("./C64_Pro_Mono-STYLE.ttf")
		if err != nil {
			panic(err)
		}
	*/
	customFontDescriptor, err := unison.RegisterFont(fonts.C64ProMonoSTYLE)
	if err != nil {
		panic(err)
	}
	fd = customFontDescriptor
	unison.DefaultButtonTheme.Font = fd.Face().Font(16)
	//unison.DefaultCheckBoxTheme.Font = fd.Face().Font(16)
	unison.DefaultLabelTheme.Font = fd.Face().Font(16)
	//unison.DefaultRadioButtonTheme.Font = fd.Face().Font(16)

	content := wnd.Content()

	content.SetBorder(unison.NewEmptyBorder(geom.NewInsets(10, 10, 10, 10)))
	content.SetLayout(&unison.FlexLayout{
		Columns:  1,
		HSpacing: unison.StdHSpacing,
		VSpacing: 10,
	})

	var index = 0
	for deviceName := range config.GetConfig().Devices {
		device := config.GetConfig().Devices[deviceName]
		panel := createControllerPanel(deviceName, device)
		panel.SetLayoutData(&unison.FlexLayoutData{
			VAlign: align.Middle,
			HGrab:  false,
		})
		content.AddChild(panel)
		if index < len(config.GetConfig().Devices)-1 {
			addSeparator(content)
			index++
		}
	}

	wnd.Pack()
	rect := wnd.FrameRect()
	rect.Point = where
	rect.Width = 500
	rect.Height = 500
	wnd.SetFrameRect(rect)
	wnd.ToFront()
	return wnd, nil
}

func createToggleSVGButton(state1 *unison.SVG, state2 *unison.SVG, deviceName string, start startStream, stop stopStream) *unison.Panel {
	active := false

	panel := unison.NewPanel()
	panel.SetSizer(func(_ geom.Size) (minSize, prefSize, maxSize geom.Size) {
		minSize.Width = 40
		prefSize.Width = 40
		maxSize.Width = 40
		minSize.Height = 40
		prefSize.Height = 40
		maxSize.Height = 40
		return minSize, prefSize, maxSize
	})

	currentSVG := state1

	panel.DrawCallback = func(gc *unison.Canvas, rect geom.Rect) {
		currentSVG.DrawInRectPreservingAspectRatio(gc, rect, nil, nil)
	}

	panel.MouseDownCallback = func(where geom.Point, button, clickCount int, mod unison.Modifiers) bool {
		active = !active
		return true
	}
	panel.MouseUpCallback = func(where geom.Point, button int, mod unison.Modifiers) bool {
		if active {
			start(deviceName)
			currentSVG = state2
		} else {
			stop(deviceName)
			currentSVG = state1
		}
		panel.MarkForRedraw()
		return true
	}
	return panel
}

func addSeparator(parent *unison.Panel) {
	sep := unison.NewSeparator()
	sep.SetLayoutData(&unison.FlexLayoutData{
		HAlign: align.Fill,
		VAlign: align.Middle,
	})
	parent.AddChild(sep)
}

func createControllerPanel(deviceName string, device *config.Device) *unison.Panel {
	panel := unison.NewPanel()
	panel.SetLayout(&unison.FlexLayout{
		Columns:      3,
		EqualColumns: false,
		HSpacing:     unison.StdHSpacing * 2,
		VSpacing:     unison.StdVSpacing,
	})
	imagePanel := NewDrawingPanel(deviceName)
	images[deviceName] = imagePanel
	buttonPanel := createToggleSVGButton(unison.MustSVGFromContentString(playIcon), unison.MustSVGFromContentString(stopIcon), deviceName, func(deviceName string) {
		device := config.GetConfig().Devices[deviceName]
		device.AudioChannel = make(chan struct{})
		streams.AudioStart(device)
		audioReader := streams.AudioReader{
			Device:       device,
			AudioContext: otoCtx,
			StopChan:     device.AudioChannel,
			Renderer: func(data []byte) {
				synchronizer.Lock()
				panel := images[deviceName]
				if panel != nil {
					panel.data = data
					panel.MarkForRedraw()
				}
				defer synchronizer.Unlock()
			},
		}
		go audioReader.Read()
		fmt.Printf("stream on %s started\n", deviceName)
	}, func(deviceName string) {
		device := config.GetConfig().Devices[deviceName]
		if device.AudioChannel != nil {
			fmt.Printf("stream on %s stopped\n", deviceName)
			close(device.AudioChannel)
			device.AudioChannel = nil
		}
	})
	panel.AddChild(buttonPanel)
	panel.AddChild(imagePanel)
	createLabel(device.Description, panel)
	return panel
}

func createLabel(title string, parent *unison.Panel) *unison.Label {
	label := unison.NewLabel()
	label.SetTitle(title)
	parent.AddChild(label)
	return label
}

func NewDrawingPanel(dataSourcename string) *DrawingPanel {
	p := &DrawingPanel{}
	p.Self = p
	p.dataSourceName = dataSourcename
	p.SetSizer(func(_ geom.Size) (minSize, prefSize, maxSize geom.Size) {
		prefSize = geom.Size{Width: 60, Height: 25}
		return prefSize, prefSize, unison.MaxSize(prefSize)
	})
	p.DrawCallback = p.draw
	return p
}

func (p *DrawingPanel) draw(gc *unison.Canvas, rect geom.Rect) {

	paint := unison.NewPaint()
	paint.SetColor(unison.ARGB(1, 55, 55, 55))
	paint.SetStyle(paintstyle.Fill)
	gc.DrawRect(rect, paint)
	paint.SetColor(unison.ARGB(1, 255, 255, 255))
	paint.SetStyle(paintstyle.Fill)
	data := images[p.dataSourceName].data

	if data != nil {
		var x float32 = 0
		for i := 2; i < len(data)-4; i += 4 {
			v1 := float32(12 - (float64(util.GetSingedWord(i, data))/32768.0)*12.5)
			v2 := float32(12 - (float64(util.GetSingedWord(i+4, data))/32768.0)*12.5)
			gc.DrawLine(geom.NewPoint(x, v1), geom.NewPoint(x+1, v2), paint)
			x += 1
		}
	}
}
