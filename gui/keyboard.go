package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"drazil.de/go64u/util"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"golang.org/x/exp/shiny/materialdesign/icons"
)

//go:embed keyboard_layout.json
var keyboardLayoutJSON []byte

const (
	optShift      = 1
	optCommodore  = 2
	optUpperLower = 4
	optReverse    = 8
	optCT         = 16
	optFrameColor = 32
	optBC         = 64
)

const (
	dispNormal    = 0
	dispShift     = 1
	dispCommodore = 2
)

const (
	codePlain     = 0
	codeShift     = 1
	codeCommodore = 2
	codeCtrl      = 3
)

const C64ProTypeface font.Typeface = "C64Pro"

type Key struct {
	ID             int        `json:"id"`
	Type           string     `json:"type"`
	Name           string     `json:"name"`
	Text           string     `json:"text"`
	Symbol         bool       `json:"symbol"`
	ToggleButton   bool       `json:"toggleButton"`
	Size           float64    `json:"size"`
	Index          int        `json:"index"`
	CodeOptions    []*string  `json:"codeOptions"`
	DisplayOptions [][]string `json:"displayOptions"`
}

type KeyRow struct {
	Keys []Key `json:"keys"`
}

type KeyMatrix struct {
	MaxOptions int      `json:"maxOptions"`
	KeyRows    []KeyRow `json:"keyRows"`
}

type KeyEvent struct {
	Key  Key
	Code int
}

type keyCell struct {
	key    Key
	click  widget.Clickable
	toggle widget.Bool
}

type VirtualKeyboard struct {
	matrix      KeyMatrix
	rows        [][]*keyCell
	optionState int
	listeners   []func(KeyEvent)

	keyByText     map[string]*keyCell
	keyByPhysName map[key.Name]*keyCell
	keyByASCII    map[rune]*keyCell

	layout kbLayout

	cacheOps   op.Ops
	cacheCall  op.CallOp
	cacheValid bool
	cacheState int
	cacheRowH  int
	cacheUnitW int
	cacheGap   int
}

func (vk *VirtualKeyboard) invalidateCache() {
	vk.cacheValid = false
}

func isLetterName(name key.Name) bool {
	return len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z'
}

type kbLayout int

const (
	layoutUS kbLayout = iota
	layoutDE
)

type hostSymbol struct{ plain, shifted rune }

type usSymbol = hostSymbol

var usHostSymbol = map[key.Name]usSymbol{
	"1":  {'1', '!'},
	"2":  {'2', '@'},
	"3":  {'3', '#'},
	"4":  {'4', '$'},
	"5":  {'5', '%'},
	"6":  {'6', '^'},
	"7":  {'7', '&'},
	"8":  {'8', '*'},
	"9":  {'9', '('},
	"0":  {'0', ')'},
	"-":  {'-', '_'},
	"+":  {'=', '+'},
	"[":  {'[', '{'},
	"]":  {']', '}'},
	";":  {';', ':'},
	"'":  {'\'', '"'},
	",":  {',', '<'},
	".":  {'.', '>'},
	"/":  {'/', '?'},
	"\\": {'\\', '|'},
	"`":  {'`', '~'},
}

var deHostSymbol = map[key.Name]hostSymbol{
	"0": {'0', '='},
	"1": {'1', '!'},
	"2": {'2', '"'},
	"3": {'3', 0},
	"4": {'4', '$'},
	"5": {'5', '%'},
	"6": {'6', '&'},
	"7": {'7', '/'},
	"8": {'8', '('},
	"9": {'9', ')'},
	"+": {'+', '*'},
	"-": {'-', '_'},
	",": {',', ';'},
	".": {'.', ':'},
}

func hostSymbolTable(l kbLayout) map[key.Name]hostSymbol {
	if l == layoutDE {
		return deHostSymbol
	}
	return usHostSymbol
}

func NewVirtualKeyboard(listener func(KeyEvent)) (*VirtualKeyboard, error) {
	vk := &VirtualKeyboard{}
	if err := json.Unmarshal(keyboardLayoutJSON, &vk.matrix); err != nil {
		return nil, fmt.Errorf("parse keyboard_layout.json: %w", err)
	}
	vk.rows = make([][]*keyCell, len(vk.matrix.KeyRows))
	id := 0
	for r, row := range vk.matrix.KeyRows {
		cells := make([]*keyCell, len(row.Keys))
		for i, k := range row.Keys {
			k.ID = id
			id++
			cells[i] = &keyCell{key: k}
		}
		vk.rows[r] = cells
	}
	if listener != nil {
		vk.listeners = append(vk.listeners, listener)
	}
	vk.layout = detectHostLayout()
	vk.buildKeyIndices()
	return vk, nil
}

func (vk *VirtualKeyboard) SetLayout(l kbLayout) { vk.layout = l }

func (vk *VirtualKeyboard) buildKeyIndices() {
	vk.keyByText = map[string]*keyCell{}
	vk.keyByASCII = map[rune]*keyCell{}
	byJSONName := map[string]*keyCell{}
	for _, row := range vk.rows {
		for _, c := range row {
			t := c.key.Type
			if t == "KEY" || t == "FUNCTION" || t == "COLOR" {
				if c.key.Text != "" {
					if _, dup := vk.keyByText[c.key.Text]; !dup {
						vk.keyByText[c.key.Text] = c
					}
				}
				if c.key.Name != "" {
					byJSONName[c.key.Name] = c
				}
			}

			if t == "KEY" {
				for slot, opt := range c.key.CodeOptions {
					if opt == nil {
						continue
					}
					n, err := strconv.Atoi(strings.TrimSpace(*opt))
					if err != nil || n < 32 || n > 126 {
						continue
					}
					r := rune(n)
					existing, ok := vk.keyByASCII[r]
					if !ok {
						vk.keyByASCII[r] = c
						continue
					}

					if slot == codePlain {
						vk.keyByASCII[r] = c
						_ = existing
					}
				}
			}
		}
	}

	pick := func(jsonName string) *keyCell { return byJSONName[jsonName] }
	pickText := func(t string) *keyCell { return vk.keyByText[t] }
	vk.keyByPhysName = map[key.Name]*keyCell{
		key.NameReturn:         pick("RETURN"),
		key.NameEnter:          pick("RETURN"),
		key.NameSpace:          pickText("SPACE"),
		key.NameLeftArrow:      pick("LEFT"),
		key.NameRightArrow:     pick("RIGHT"),
		key.NameUpArrow:        pick("UP"),
		key.NameDownArrow:      pick("DOWN"),
		key.NameDeleteBackward: pick("DELETE"),
		key.NameDeleteForward:  pick("INSERT"),
		key.NameHome:           pick("HOME"),
		key.NameEscape:         pick("RUN/STOP"),
		key.NameF1:             pickText("F1"),
		key.NameF2:             pickText("F2"),
		key.NameF3:             pickText("F3"),
		key.NameF4:             pickText("F4"),
		key.NameF5:             pickText("F5"),
		key.NameF6:             pickText("F6"),
		key.NameF7:             pickText("F7"),
		key.NameF8:             pickText("F8"),
	}
}

func (vk *VirtualKeyboard) HandlePhysicalKey(name key.Name, mods key.Modifiers) bool {
	if cell := vk.keyByPhysName[name]; cell != nil {
		vk.fire(KeyEvent{Key: cell.key, Code: cell.key.resolveCode(vk.modState(mods, true))})
		return true
	}

	if isLetterName(name) {
		if cell := vk.keyByText[string(name)]; cell != nil {
			vk.fire(KeyEvent{Key: cell.key, Code: cell.key.resolveCode(vk.modState(mods, true))})
			return true
		}
		return false
	}

	if mods&(key.ModCtrl|key.ModCommand|key.ModAlt) != 0 {
		if cell := vk.keyByText[string(name)]; cell != nil {
			vk.fire(KeyEvent{Key: cell.key, Code: cell.key.resolveCode(vk.modState(mods, true))})
			return true
		}
	}

	if sym, ok := hostSymbolTable(vk.layout)[name]; ok {
		r := sym.plain
		if mods&key.ModShift != 0 {
			r = sym.shifted
		}
		if r == 0 {
			return false
		}
		if cell := vk.keyByASCII[r]; cell != nil {
			vk.fire(KeyEvent{Key: cell.key, Code: int(r)})
			return true
		}
		return false
	}
	return false
}

func (vk *VirtualKeyboard) modState(mods key.Modifiers, includeShift bool) int {
	state := vk.optionState
	if includeShift && mods&key.ModShift != 0 {
		state |= optShift
	}
	if mods&(key.ModCtrl|key.ModCommand) != 0 {
		state |= optCommodore
	}
	if mods&key.ModAlt != 0 {
		state |= optCT
	}
	return state
}

func (vk *VirtualKeyboard) AddListener(l func(KeyEvent)) {
	if l != nil {
		vk.listeners = append(vk.listeners, l)
	}
}

func (vk *VirtualKeyboard) OptionState() int { return vk.optionState }

func (vk *VirtualKeyboard) fire(ev KeyEvent) {
	for _, l := range vk.listeners {
		l(ev)
	}
}

func (k *Key) resolveDisplay(state int) string {
	if k.Type != "KEY" || len(k.DisplayOptions) == 0 {
		return k.Text
	}
	outer := dispNormal
	switch {
	case state&optCommodore != 0:
		outer = dispCommodore
	case state&optShift != 0:
		outer = dispShift
	}
	if outer >= len(k.DisplayOptions) {
		outer = 0
	}
	inner := 0
	if state&optUpperLower != 0 {
		inner |= 1
	}
	if state&optReverse != 0 {
		inner |= 2
	}
	row := k.DisplayOptions[outer]
	if inner >= len(row) {
		inner = 0
	}
	if len(row) == 0 {
		return k.Text
	}
	return row[inner]
}

func (k *Key) resolveCode(state int) int {
	if len(k.CodeOptions) == 0 {
		return -1
	}
	idx := codePlain
	switch {
	case state&optCommodore != 0 && len(k.CodeOptions) > codeCommodore && k.CodeOptions[codeCommodore] != nil:
		idx = codeCommodore
	case state&optShift != 0 && len(k.CodeOptions) > codeShift && k.CodeOptions[codeShift] != nil:
		idx = codeShift
	case state&optCT != 0 && len(k.CodeOptions) > codeCtrl && k.CodeOptions[codeCtrl] != nil:
		idx = codeCtrl
	}
	if idx >= len(k.CodeOptions) || k.CodeOptions[idx] == nil {
		return -1
	}
	v, err := strconv.Atoi(strings.TrimSpace(*k.CodeOptions[idx]))
	if err != nil {
		return -1
	}
	return v
}

var iconByKeyName = func() map[string]*widget.Icon {
	m := map[string]*widget.Icon{}
	for name, data := range map[string][]byte{
		"UP":          icons.HardwareKeyboardArrowUp,
		"DOWN":        icons.HardwareKeyboardArrowDown,
		"LEFT":        icons.HardwareKeyboardArrowLeft,
		"RIGHT":       icons.HardwareKeyboardArrowRight,
		"SHIFT":       icons.NavigationArrowUpward,
		"COMMODORE":   icons.SocialSentimentSatisfied,
		"REVERSE":     icons.ActionInvertColors,
		"UPPER_LOWER": icons.ActionSwapVert,
		"CLEAR":       icons.ToggleCheckBoxOutlineBlank,
		"HOME":        icons.ActionHome,
		"INSERT":      icons.ContentAdd,
		"DELETE":      icons.NavigationArrowBack,
		"RETURN":      icons.HardwareKeyboardReturn,
	} {
		if ic, err := widget.NewIcon(data); err == nil {
			m[name] = ic
		}
	}
	return m
}()

var (
	kbBg            = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	kbKeyBg         = color.NRGBA{R: 70, G: 70, B: 70, A: 255}
	kbFunctionBg    = color.NRGBA{R: 95, G: 95, B: 95, A: 255}
	kbOptionBg      = color.NRGBA{R: 60, G: 60, B: 80, A: 255}
	kbOptionOn      = color.NRGBA{R: 103, G: 255, B: 69, A: 255}
	kbBorder        = color.NRGBA{R: 25, G: 25, B: 25, A: 255}
	kbText          = color.NRGBA{R: 230, G: 230, B: 230, A: 255}
	kbTextOnLight   = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	kbColorBoxBlack = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
)

func (vk *VirtualKeyboard) MaxWidthDp() unit.Dp {
	maxRowSize := 0.0
	for _, row := range vk.rows {
		var sum float64
		for _, c := range row {
			sum += c.key.Size
		}
		if sum > maxRowSize {
			maxRowSize = sum
		}
	}
	return unit.Dp(maxRowSize * 25)
}

func (vk *VirtualKeyboard) MaxWidth(gtx layout.Context) int {
	return gtx.Dp(vk.MaxWidthDp())
}

func (vk *VirtualKeyboard) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rowH := gtx.Dp(unit.Dp(25))
	unitW := gtx.Dp(unit.Dp(25))
	gap := gtx.Dp(unit.Dp(2))

	totalW := vk.MaxWidth(gtx)
	if totalW <= 0 {
		return layout.Dimensions{}
	}

	vk.processInput(gtx)

	if !vk.cacheValid || vk.cacheState != vk.optionState ||
		vk.cacheRowH != rowH || vk.cacheUnitW != unitW || vk.cacheGap != gap {
		vk.cacheOps.Reset()
		rec := op.Record(&vk.cacheOps)
		recGtx := gtx
		recGtx.Ops = &vk.cacheOps

		clipArea := clip.Rect{Max: image.Pt(totalW, rowH*len(vk.rows))}.Push(recGtx.Ops)
		paint.Fill(recGtx.Ops, kbBg)
		clipArea.Pop()

		unitWf := float64(unitW)
		for r, row := range vk.rows {
			yTop := r * rowH

			var xF float64
			for _, c := range row {
				x0 := int(xF)
				xF += c.key.Size * unitWf
				x1 := int(xF)
				w := x1 - x0
				if w <= 0 {
					continue
				}
				rect := image.Rect(x0+gap/2, yTop+gap/2, x1-gap/2, yTop+rowH-gap/2)
				vk.drawCell(th, recGtx, c, rect)
			}
		}

		vk.cacheCall = rec.Stop()
		vk.cacheValid = true
		vk.cacheState = vk.optionState
		vk.cacheRowH, vk.cacheUnitW, vk.cacheGap = rowH, unitW, gap
	}
	vk.cacheCall.Add(gtx.Ops)

	return layout.Dimensions{Size: image.Pt(totalW, rowH*len(vk.rows))}
}

func (vk *VirtualKeyboard) processInput(gtx layout.Context) {
	newState := 0
	for _, row := range vk.rows {
		for _, c := range row {
			k := c.key
			if k.Type == "OPTION" {

				_ = c.toggle.Update(gtx)
				if c.toggle.Value {
					newState |= k.Index
				}
				continue
			}
			if c.click.Clicked(gtx) {
				vk.fire(KeyEvent{Key: c.key, Code: c.key.resolveCode(vk.optionState)})
			}
		}
	}
	if newState != vk.optionState {
		vk.optionState = newState

		for _, row := range vk.rows {
			for _, c := range row {
				if c.key.Type == "OPTION" && c.toggle.Value {
					vk.fire(KeyEvent{Key: c.key, Code: c.key.resolveCode(vk.optionState)})
				}
			}
		}
	}
}

func (vk *VirtualKeyboard) drawCell(th *material.Theme, gtx layout.Context, c *keyCell, rect image.Rectangle) {
	if c.key.Type == "FILLER" {
		return
	}

	bg := kbKeyBg
	textCol := kbText
	switch c.key.Type {
	case "FUNCTION":
		bg = kbFunctionBg
	case "OPTION":
		bg = kbOptionBg
		if c.toggle.Value {
			bg = kbOptionOn
			textCol = kbTextOnLight
		}
	case "COLOR":
		bg = paletteColor(c.key.Index)

		if luminance(bg) > 140 {
			textCol = kbTextOnLight
		} else {
			textCol = kbText
		}
	}

	radius := gtx.Dp(unit.Dp(4))
	rrSize := image.Pt(rect.Dx(), rect.Dy())

	defer op.Offset(rect.Min).Push(gtx.Ops).Pop()
	{
		rr := clip.RRect{
			Rect: image.Rect(0, 0, rrSize.X, rrSize.Y),
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}
		stack := rr.Push(gtx.Ops)
		paint.Fill(gtx.Ops, bg)
		stack.Pop()

		strokeRR := clip.RRect{
			Rect: image.Rect(0, 0, rrSize.X, rrSize.Y),
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}
		strokeStack := clip.Stroke{Path: strokeRR.Path(gtx.Ops), Width: 1}.Op().Push(gtx.Ops)
		paint.Fill(gtx.Ops, kbBorder)
		strokeStack.Pop()
	}

	cellGtx := gtx
	cellGtx.Constraints = layout.Exact(rrSize)

	contents := func(gtx layout.Context) layout.Dimensions {

		if c.key.Type == "COLOR" || c.key.Text == "SPACE" {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}

		if ic, ok := iconByKeyName[c.key.Name]; ok {
			return layoutCellIcon(gtx, ic, textCol)
		}
		return layoutCellLabel(th, gtx, &c.key, c.key.resolveDisplay(vk.optionState), textCol)
	}

	if c.key.Type == "OPTION" {
		c.toggle.Layout(cellGtx, func(gtx layout.Context) layout.Dimensions {
			return contents(gtx)
		})
		return
	}
	c.click.Layout(cellGtx, func(gtx layout.Context) layout.Dimensions {
		return contents(gtx)
	})
}

func layoutCellIcon(gtx layout.Context, ic *widget.Icon, col color.NRGBA) layout.Dimensions {
	cell := gtx.Constraints.Min
	side := min(cell.X, cell.Y) * 7 / 10
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(side, side))
		ic.Layout(gtx, col)
		return layout.Dimensions{Size: image.Pt(side, side)}
	})
}

func layoutCellLabel(th *material.Theme, gtx layout.Context, k *Key, txt string, col color.NRGBA) layout.Dimensions {
	if txt == "" {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	useC64 := k.Symbol || (k.Type == "KEY" && len(k.DisplayOptions) > 0)
	size := unit.Sp(12)
	if k.Type == "FUNCTION" || k.Type == "COLOR" || k.Type == "OPTION" {
		size = unit.Sp(11)
	}
	if useC64 {
		size = unit.Sp(16)
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, size, txt)
		lbl.Color = col
		lbl.Alignment = text.Middle
		lbl.MaxLines = 1
		if useC64 {
			lbl.Font.Typeface = C64ProTypeface
		}
		return lbl.Layout(gtx)
	})
}

func paletteColor(idx int) color.NRGBA {
	pal := util.GetPalette()
	if idx < 0 || idx >= len(pal) {
		return kbColorBoxBlack
	}
	r, g, b, a := pal[idx].RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func luminance(c color.NRGBA) int {
	return (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000
}
