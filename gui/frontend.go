package gui

import (
	"fmt"
	"log/slog"

	"drazil.de/go64u/config"
	"github.com/richardwilkes/toolbox/fatal"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/align"
	"github.com/richardwilkes/unison/enums/check"
)

func Run() {
	unison.Start(unison.StartupFinishedCallback(func() {
		_, err := build(unison.PrimaryDisplay().Usable.Point)
		fatal.IfErr(err)
	}))
}

func build(where geom.Point) (*unison.Window, error) {

	wnd, err := unison.NewWindow("go64u - experimental gui")
	if err != nil {
		return nil, err
	}

	content := wnd.Content()
	content.SetBorder(unison.NewEmptyBorder(geom.NewInsets(10, 10, 10, 10)))
	content.SetLayout(&unison.FlexLayout{
		Columns:  1,
		HSpacing: unison.StdHSpacing,
		VSpacing: 10,
	})

	panel := createCheckBoxPanel()
	panel.SetLayoutData(&unison.FlexLayoutData{
		VAlign: align.Middle,
		HGrab:  true,
	})
	content.AddChild(panel)

	addSeparator(content)
	panel = createButtonsPanel()
	panel.SetLayoutData(&unison.FlexLayoutData{
		VAlign: align.Middle,
		HGrab:  true,
	})
	content.AddChild(panel)

	wnd.Pack()
	rect := wnd.FrameRect()
	rect.Point = where
	rect.Width = 500
	rect.Height = 500
	wnd.SetFrameRect(rect)
	wnd.ToFront()
	return wnd, nil
}

func createButtonsPanel() *unison.Panel {
	panel := unison.NewPanel()
	panel.SetLayout(&unison.FlowLayout{
		HSpacing: unison.StdHSpacing,
		VSpacing: unison.StdVSpacing,
	})

	for _, title := range []string{"Start Stream", "Start Recording"} {
		createButton(title, panel)
	}
	return panel
}

func createButton(title string, panel *unison.Panel) *unison.Button {
	btn := unison.NewButton()
	btn.SetTitle(title)
	btn.ClickCallback = func() { slog.Info(title) }
	btn.Tooltip = unison.NewTooltipWithText(fmt.Sprintf("Tooltip for: %s", title))
	btn.SetLayoutData(align.Middle)
	panel.AddChild(btn)
	return btn
}

func addSeparator(parent *unison.Panel) {
	sep := unison.NewSeparator()
	sep.SetLayoutData(&unison.FlexLayoutData{
		HAlign: align.Fill,
		VAlign: align.Middle,
	})
	parent.AddChild(sep)
}

func createCheckBoxPanel() *unison.Panel {
	panel := unison.NewPanel()
	panel.SetLayout(&unison.FlexLayout{
		Columns:  1,
		HSpacing: unison.StdHSpacing * 2,
		VSpacing: unison.StdVSpacing,
	})

	group := unison.NewGroup()
	for deviceName := range config.GetConfig().Devices {
		device := config.GetConfig().Devices[deviceName]
		cb := createRadioButton(device.Description, panel, group)
		cb.ClientData()["id"] = deviceName
		cb.ClientData()[deviceName] = device.Description
	}
	createCheckBox("Start stream automatically", check.Off, panel)

	return panel
}
func createCheckBox(title string, initialState check.Enum, panel *unison.Panel) *unison.CheckBox {
	cb := unison.NewCheckBox()
	cb.SetTitle(title)
	cb.State = initialState
	cb.ClickCallback = func() {
		slog.Info("checkbox clicked", "title", title)
	}
	panel.AddChild(cb)
	return cb
}

func createRadioButton(title string, panel *unison.Panel, group *unison.Group) *unison.RadioButton {
	rb := unison.NewRadioButton()
	rb.SetTitle(title)

	rb.ClickCallback = func() {
		fmt.Printf("%s clicked\n", rb.ClientData()["id"])
		//slog.Info("radio button clicked", "title", title)
	}
	panel.AddChild(rb)
	group.Add(rb)
	return rb
}
