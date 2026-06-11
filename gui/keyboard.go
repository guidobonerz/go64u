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

// Bit values used by OPTION keys to compose the keyboard's optionState mask.
// They match the `index` field on each OPTION entry in keyboard_layout.json
// and the constants in the original Java VirtualKeyboard.
const (
	optShift      = 1
	optCommodore  = 2
	optUpperLower = 4
	optReverse    = 8
	optCT         = 16
	optFrameColor = 32
	optBC         = 64
)

// Outer index into Key.DisplayOptions, derived from the SHIFT / COMMODORE bits.
const (
	dispNormal    = 0
	dispShift     = 1
	dispCommodore = 2
)

// Slot index into Key.CodeOptions, in resolution-priority order.
const (
	codePlain     = 0
	codeShift     = 1
	codeCommodore = 2
	codeCtrl      = 3
)

// C64ProTypeface is the font name we register the embedded C64 Pro Mono TTF
// under in the gioui shaper. Anything that needs to render PETSCII glyphs
// (symbol keys, displayOptions text) sets this typeface on its label.
const C64ProTypeface font.Typeface = "C64Pro"

// Key mirrors a single entry in keyboard_layout.json. Fields are tagged for
// json.Unmarshal; the JSON keys are camelCase.
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

// KeyRow is a single row of keys in the matrix.
type KeyRow struct {
	Keys []Key `json:"keys"`
}

// KeyMatrix is the full keyboard as decoded from JSON.
type KeyMatrix struct {
	MaxOptions int      `json:"maxOptions"`
	KeyRows    []KeyRow `json:"keyRows"`
}

// KeyEvent is what the keyboard fires to its listener when a key is pressed.
// Code is the resolved value from CodeOptions[*] given the current optionState,
// or -1 if the active slot is null (e.g. ctrl-only modifier with no codepoint).
type KeyEvent struct {
	Key  Key
	Code int
}

// keyCell is the per-key runtime state: the parsed Key plus a Clickable for
// momentary keys and a Bool for OPTION (toggleButton) keys.
type keyCell struct {
	key    Key
	click  widget.Clickable
	toggle widget.Bool
}

// VirtualKeyboard is a gioui widget that renders the C64 keyboard layout
// from keyboard_layout.json and dispatches KeyEvents to a listener.
//
// Construction parses the embedded JSON once. Layout is called every frame.
type VirtualKeyboard struct {
	matrix      KeyMatrix
	rows        [][]*keyCell
	optionState int
	listeners   []func(KeyEvent)

	// Indices used by HandlePhysicalKey. Built once in NewVirtualKeyboard.
	keyByText     map[string]*keyCell // by Key.Text (digits, letters, punctuation)
	keyByPhysName map[key.Name]*keyCell
	keyByASCII    map[rune]*keyCell // any cell whose CodeOptions hits printable ASCII

	// Active host keyboard layout. Set once at construction; we don't poll
	// for live layout switches.
	layout kbLayout
}

// isLetterName reports whether name is a single ASCII upper-case letter — the
// form gioui uses for letter keys (Name is unicode.ToUpper'd; Shift state is
// reported separately via Modifiers).
func isLetterName(name key.Name) bool {
	return len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z'
}

// kbLayout identifies the active host keyboard layout. We need to know it
// because gioui's Windows backend reports a fixed VK→Name string and never
// the typed glyph, so the same Name+Modifiers means different things on
// QWERTY vs QWERTZ.
type kbLayout int

const (
	layoutUS kbLayout = iota
	layoutDE
)

// hostSymbol holds the ASCII rune a host produces for a given gioui key.Name
// in the unshifted and shifted state. A zero rune means "no PETSCII mapping
// for this combination" (e.g. DE Shift+3 yields '§', which the C64 has no
// glyph for) — those events are silently dropped.
type hostSymbol struct{ plain, shifted rune }

// usSymbol kept as an alias so existing call sites stay readable.
type usSymbol = hostSymbol

// usHostSymbol covers the digit row and the OEM symbol keys on a US-QWERTY
// physical keyboard. We need this because gioui's Windows backend reports
// Name from a fixed VK→string table (see app/os_windows.go convertKeyCode):
// it is not layout-aware and not Shift-aware, so the same Name arrives for
// both `,` and `<`, both `[` and `{`, etc., with the actual Shift state
// only present in Modifiers. Worse, VK_OEM_PLUS (the `=`/`+` key) is hard-
// coded to Name "+" even unshifted, so we can't read Name as "the typed
// glyph" and have to translate explicitly here.
//
// US-only for now; non-US layouts will mis-map and need their own table.
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
	"+":  {'=', '+'}, // VK_OEM_PLUS — gioui labels the `=` key "+"
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

// deHostSymbol covers a German QWERTZ physical keyboard. Only entries that
// can produce something the C64 knows are listed; AltGr-reachable glyphs
// (`@` `[` `]` `{` `}` `|` `\`) are omitted because gioui's key.Event does
// not surface AltGr distinctly from Ctrl+Alt and we'd mis-route them.
var deHostSymbol = map[key.Name]hostSymbol{
	"0": {'0', '='}, // shift+0 = '='
	"1": {'1', '!'},
	"2": {'2', '"'}, // shift+2 = '"'
	"3": {'3', 0},   // shift+3 = '§', no C64 glyph
	"4": {'4', '$'},
	"5": {'5', '%'},
	"6": {'6', '&'},
	"7": {'7', '/'}, // shift+7 = '/'
	"8": {'8', '('},
	"9": {'9', ')'},
	"+": {'+', '*'}, // VK_OEM_PLUS on DE: '+' / '*'
	"-": {'-', '_'}, // VK_OEM_MINUS on DE: '-' / '_'
	",": {',', ';'}, // VK_OEM_COMMA on DE: ',' / ';'
	".": {'.', ':'}, // VK_OEM_PERIOD on DE: '.' / ':'
}

// hostSymbolTable returns the symbol-translation table for the active layout.
func hostSymbolTable(l kbLayout) map[key.Name]hostSymbol {
	if l == layoutDE {
		return deHostSymbol
	}
	return usHostSymbol
}

// NewVirtualKeyboard parses the embedded layout and builds the runtime cells.
// The optional listener is appended; additional listeners can be attached
// later via AddListener.
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

// SetLayout overrides the auto-detected host keyboard layout. Useful when
// the user wants to force a specific mapping (e.g. running a US layout on
// a DE-detected system, or vice versa).
func (vk *VirtualKeyboard) SetLayout(l kbLayout) { vk.layout = l }

// buildKeyIndices populates the lookup maps used by HandlePhysicalKey. KEY/
// COLOR/FUNCTION cells with non-empty Text become entries in keyByText; the
// hardcoded gioui-name → key.Name table maps special keys (RETURN, arrows,
// F1-F8, etc.) to whatever virtual cell the JSON declares with the matching
// Name or Text.
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
			// Index every printable-ASCII PETSCII this cell can emit.
			// First-write-wins so the plain (codeOptions[0]) cell wins
			// over a shift/commodore slot on a different cell.
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
					// Prefer the cell where this rune lives in the plain
					// slot — that's the "natural" home for the char.
					if slot == codePlain {
						vk.keyByASCII[r] = c
						_ = existing
					}
				}
			}
		}
	}

	// Map gioui's named keys onto the virtual cells. Some special keys
	// (RUN/STOP, RETURN) live on KEYs whose Name field is filled in the JSON;
	// arrows / cursor / HOME / CLEAR are FUNCTION rows likewise named.
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

// HandlePhysicalKey resolves a host-keyboard event to a virtual cell and fires
// the listener, mirroring a mouse click.
//
// Why this is more involved than it looks: gioui's Windows backend hands us
// Name from a fixed VK→string table that isn't layout- or Shift-aware
// (see usHostSymbol). We can't trust Name to be the typed glyph, so the
// routing is split:
//
//  1. Special keys (RETURN, arrows, F1..F8, HOME, INSERT, …) — direct
//     keyByPhysName lookup with the modifier bits OR'd into optionState.
//  2. Letters A..Z — keyByText lookup; ModShift selects the C64 shifted-
//     letter PETSCII slot (193..218) and Ctrl/Alt the Commodore/CT slot.
//  3. Ctrl/Alt held on a non-letter (host Ctrl+, → C64 Commodore+,) —
//     keyByText lookup with the same state path so the Commodore/CT slot
//     is reached.
//  4. Plain or Shift-only on a non-letter symbol — translate (Name, Shift)
//     through usHostSymbol to the ASCII rune the host actually produces,
//     then look up the C64 cell that emits that PETSCII (keyByASCII) and
//     fire the literal rune as Code.
//
// Returns true when the event matched a cell.
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

	// Ctrl/Alt on a non-letter: prefer the cell-by-name + state path so the
	// Commodore/CT PETSCII slot is reached (e.g. host Ctrl+, → C64
	// Commodore+, → 60). Falls through to the lookup table if no cell match.
	if mods&(key.ModCtrl|key.ModCommand|key.ModAlt) != 0 {
		if cell := vk.keyByText[string(name)]; cell != nil {
			vk.fire(KeyEvent{Key: cell.key, Code: cell.key.resolveCode(vk.modState(mods, true))})
			return true
		}
	}

	// Plain or Shift-only on a non-letter: translate to the ASCII the host
	// actually produces (via the active layout's table), then find the C64
	// cell whose PETSCII matches.
	if sym, ok := hostSymbolTable(vk.layout)[name]; ok {
		r := sym.plain
		if mods&key.ModShift != 0 {
			r = sym.shifted
		}
		if r == 0 {
			return false // shifted glyph has no C64 equivalent (e.g. DE '§')
		}
		if cell := vk.keyByASCII[r]; cell != nil {
			vk.fire(KeyEvent{Key: cell.key, Code: int(r)})
			return true
		}
		return false
	}
	return false
}

// modState OR's host modifier bits into the latched optionState. includeShift
// is true for letter and special-key paths where Shift is a real modifier;
// it is false for the non-letter lookup path where Shift is consumed by the
// usHostSymbol translation instead.
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

// AddListener registers an extra callback. Mirrors Java's addHitKeyListener.
func (vk *VirtualKeyboard) AddListener(l func(KeyEvent)) {
	if l != nil {
		vk.listeners = append(vk.listeners, l)
	}
}

// OptionState returns the current modifier bitmask (SHIFT|COMMODORE|...).
func (vk *VirtualKeyboard) OptionState() int { return vk.optionState }

// fire dispatches an event to all registered listeners.
func (vk *VirtualKeyboard) fire(ev KeyEvent) {
	for _, l := range vk.listeners {
		l(ev)
	}
}

// resolveDisplay picks the text to render on a key for the current optionState.
// For KEY cells we index DisplayOptions[outer][inner]; everything else just
// uses the raw Text field.
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

// resolveCode picks the active CodeOptions slot for the current optionState
// and parses it as an integer. Returns -1 when the slot is nil or unparseable.
// Priority: COMMODORE > SHIFT > CT > plain — matches the Java widget.
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

// iconByKeyName overrides the per-cell glyph for keys whose Name field has a
// material-icon equivalent: cursor FUNCTION keys render chevrons; SHIFT and
// COMMODORE OPTION keys render an arrow-up and a smiley-face icon.
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

// keyboard color palette — kept local to this file so tweaks don't leak into
// the rest of the gui package's color vocabulary.
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

// MaxWidthDp returns the natural width of the widest row in dp. Useful for
// sizing decisions outside of a frame (e.g. window resizing) where no gtx
// scale factor is available.
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

// MaxWidth returns MaxWidthDp converted to pixels at the current gtx scale.
// Callers that want to size a sibling widget (e.g. the video monitor) to
// match the keyboard's footprint can use this without having to render the
// keyboard.
func (vk *VirtualKeyboard) MaxWidth(gtx layout.Context) int {
	return gtx.Dp(vk.MaxWidthDp())
}

// Layout renders the keyboard at a fixed cell size (a size-1 key is exactly
// 25×25 dp); the widget's overall dimensions are derived from that, not from
// the available width. Height is rowsCount * rowH so the caller can place it
// as a Rigid child.
func (vk *VirtualKeyboard) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	rowH := gtx.Dp(unit.Dp(25))
	unitW := gtx.Dp(unit.Dp(25))
	gap := gtx.Dp(unit.Dp(2))

	totalW := vk.MaxWidth(gtx)
	if totalW <= 0 {
		return layout.Dimensions{}
	}

	// First pass: drain click / toggle events and update optionState. This
	// must happen before drawing so the same frame reflects the new state.
	vk.processInput(gtx)

	// Second pass: paint background, then walk rows and draw each cell.
	clipArea := clip.Rect{Max: image.Pt(totalW, rowH*len(vk.rows))}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, kbBg)
	clipArea.Pop()

	unitWf := float64(unitW)
	for r, row := range vk.rows {
		yTop := r * rowH
		// Use float accumulation for the X cursor so sub-pixel sizes (e.g. 0.3
		// fillers, 1.5 keys) don't drift. Snap to int when drawing.
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
			vk.drawCell(th, gtx, c, rect)
		}
	}

	return layout.Dimensions{Size: image.Pt(totalW, rowH*len(vk.rows))}
}

// processInput walks every cell, drains its events, and rebuilds optionState
// from the current toggle states. OPTION key events fire as the state changes;
// non-OPTION key clicks fire once per click.
func (vk *VirtualKeyboard) processInput(gtx layout.Context) {
	newState := 0
	for _, row := range vk.rows {
		for _, c := range row {
			k := c.key
			if k.Type == "OPTION" {
				// Update fires both on press and release; we track value.
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
		// Notify listeners that the modifier state changed by emitting a
		// synthetic event for each OPTION key currently latched. Mirrors the
		// Java widget's behaviour of forwarding OPTION presses to listeners.
		for _, row := range vk.rows {
			for _, c := range row {
				if c.key.Type == "OPTION" && c.toggle.Value {
					vk.fire(KeyEvent{Key: c.key, Code: c.key.resolveCode(vk.optionState)})
				}
			}
		}
	}
}

// drawCell draws a single key into rect. FILLER cells are skipped.
func (vk *VirtualKeyboard) drawCell(th *material.Theme, gtx layout.Context, c *keyCell, rect image.Rectangle) {
	if c.key.Type == "FILLER" {
		return
	}

	// Background fill (color depends on type; COLOR keys use the C64 palette).
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
		// Pick black/white text for legibility against the swatch.
		if luminance(bg) > 140 {
			textCol = kbTextOnLight
		} else {
			textCol = kbText
		}
	}

	radius := gtx.Dp(unit.Dp(4))
	rrSize := image.Pt(rect.Dx(), rect.Dy())

	// Translate ops origin into the cell's top-left so the clickable + bg + text
	// all share local coordinates.
	defer op.Offset(rect.Min).Push(gtx.Ops).Pop()
	{
		rr := clip.RRect{
			Rect: image.Rect(0, 0, rrSize.X, rrSize.Y),
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}
		stack := rr.Push(gtx.Ops)
		paint.Fill(gtx.Ops, bg)
		stack.Pop()

		// Border
		strokeRR := clip.RRect{
			Rect: image.Rect(0, 0, rrSize.X, rrSize.Y),
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}
		strokeStack := clip.Stroke{Path: strokeRR.Path(gtx.Ops), Width: 1}.Op().Push(gtx.Ops)
		paint.Fill(gtx.Ops, kbBorder)
		strokeStack.Pop()
	}

	// Wrap the cell's content in the appropriate clickable / toggle widget
	// so hit-testing is constrained to the cell rectangle.
	cellGtx := gtx
	cellGtx.Constraints = layout.Exact(rrSize)

	contents := func(gtx layout.Context) layout.Dimensions {
		// COLOR swatches and the space bar render as pure surfaces with no
		// label / icon overlay.
		if c.key.Type == "COLOR" || c.key.Text == "SPACE" {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}
		// Special keys whose Name has a material-icon override (cursor
		// chevrons on FUNCTION rows; SHIFT / COMMODORE on OPTION rows).
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

// layoutCellIcon renders a centered icon scaled to ~70% of the cell's smaller
// dimension. Used for cursor chevrons on the FUNCTION keys.
func layoutCellIcon(gtx layout.Context, ic *widget.Icon, col color.NRGBA) layout.Dimensions {
	cell := gtx.Constraints.Min
	side := min(cell.X, cell.Y) * 7 / 10
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(side, side))
		ic.Layout(gtx, col)
		return layout.Dimensions{Size: image.Pt(side, side)}
	})
}

// layoutCellLabel renders the key text centered in the cell, switching to the
// C64 typeface for symbol keys and KEY cells whose displayed text comes from
// DisplayOptions (which uses Private-Use-Area glyphs).
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

// paletteColor returns the C64 palette color at idx as NRGBA. Falls back to
// black when idx is out of range.
func paletteColor(idx int) color.NRGBA {
	pal := util.GetPalette()
	if idx < 0 || idx >= len(pal) {
		return kbColorBoxBlack
	}
	r, g, b, a := pal[idx].RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// luminance is a quick perceptual brightness estimate (0-255) for picking
// readable text colors against arbitrary palette swatches.
func luminance(c color.NRGBA) int {
	return (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000
}
