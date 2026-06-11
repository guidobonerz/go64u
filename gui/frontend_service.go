package gui

import (
	"fmt"
	"sort"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"
	"drazil.de/go64u/util"
)

func SendKeyboardSequence(command uint16, sequence []byte) {
	payload := make([]byte, len(sequence)+4)
	copy(payload[:], util.GetWordArray(command))
	copy(payload[2:], util.GetWordArray(uint16(len(sequence))))
	copy(payload[4:], sequence[:])
	network.SendTcpData(payload, "10.100.200.230")
}

// sendKeystrokes wraps a 0xff03 keystroke frame and writes it as a single
// TCP packet — the common case for KEY/FUNCTION/COLOR(state<32) events.
func sendKeystrokes(codes []byte) {
	SendKeyboardSequence(0xff03, codes)
}

// buildPoke returns a single 0xff06 memory-poke frame
// (opcode + length=3 + 2-byte addr + 1-byte data = 7 bytes), without
// writing it. Multiple frames can be concatenated and sent via writeFrames
// to mirror the Java buildCommand+write pattern.
func buildPoke(addr uint16, data byte) []byte {
	frame := make([]byte, 7)
	copy(frame[0:2], util.GetWordArray(0xff06))
	copy(frame[2:4], util.GetWordArray(3))
	copy(frame[4:6], util.GetWordArray(addr))
	frame[6] = data
	return frame
}

// writeFrames concatenates pre-built frames and ships them as one TCP write.
func writeFrames(frames ...[]byte) {
	total := 0
	for _, f := range frames {
		total += len(f)
	}
	buf := make([]byte, 0, total)
	for _, f := range frames {
		buf = append(buf, f...)
	}
	network.SendTcpData(buf, "10.100.200.230")
}

// keyboardListener returns the KeyEvent handler attached to the
// VirtualKeyboard. Mirrors the Java sendKeyboardSequence(Key) logic:
// KEY/FUNCTION events and "normal-mode" COLOR events become 0xff03
// keystrokes (with named buttons RUN/LIST/DIR/LOAD * expanding to BASIC
// commands and code==3 triggering the RUN/STOP poke pair); COLOR events
// while FRAME or BC option is latched become $D020/$D021 pokes.
func KeyboardListener(kb *VirtualKeyboard) func(KeyEvent) {
	return func(ev KeyEvent) {
		k := ev.Key
		state := kb.OptionState()
		code := ev.Code
		fmt.Printf("vkb: %s/%q -> code=%d state=0x%02x\n", k.Type, k.Text, code, state)

		if k.Type == "KEY" || k.Type == "FUNCTION" ||
			(k.Type == "COLOR" && state < optFrameColor) {

			if code == 3 {
				writeFrames(buildPoke(0x0314, 0x7b), buildPoke(0x0091, 127))
				writeFrames(buildPoke(0x0314, 0x31), buildPoke(0x0091, 255))
				return
			}

			var codes []byte
			switch k.Name {
			case "RUN":
				codes = append([]byte("RUN"), 13)
			case "LIST":
				codes = append([]byte("LIST"), 13)
			case "DIR":
				codes = append([]byte(`LOAD"$",8`), 13)
			case "LOAD":
				codes = append([]byte(`LOAD"*",8,1`), 13)
			case "AUTO":
				codes = append(append(append(append([]byte{},
					[]byte(`LOAD"*",8,1`)...), 13),
					[]byte(`RUN`)...), 13)

			default:
				if code < 0 {
					return
				}
				codes = []byte{byte(code & 0xff)}
			}
			sendKeystrokes(codes)
			return
		}

		if k.Type == "COLOR" {
			var frames [][]byte
			if state&optFrameColor != 0 {
				frames = append(frames, buildPoke(0xd020, byte(k.Index)))
			}
			if state&optBC != 0 {
				frames = append(frames, buildPoke(0xd021, byte(k.Index)))
			}
			if len(frames) > 0 {
				writeFrames(frames...)
			}
		}
	}
}

func BuildDeviceList() []deviceUI {
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
