package gui

import "testing"

func TestKeyboardLayoutParses(t *testing.T) {
	vk, err := NewVirtualKeyboard(nil)
	if err != nil {
		t.Fatalf("NewVirtualKeyboard: %v", err)
	}
	if len(vk.rows) == 0 {
		t.Fatal("no rows parsed")
	}

	wantTypes := map[string]int{}
	for _, row := range vk.rows {
		for _, c := range row {
			wantTypes[c.key.Type]++
		}
	}
	for _, want := range []string{"FILLER", "FUNCTION", "KEY", "COLOR", "OPTION"} {
		if wantTypes[want] == 0 {
			t.Errorf("expected at least one %q key in layout, got none", want)
		}
	}

	var sawShift, sawCBM bool
	for _, row := range vk.rows {
		for _, c := range row {
			if c.key.Type == "OPTION" && c.key.Name == "SHIFT" {
				sawShift = true
				if c.key.Index != optShift {
					t.Errorf("SHIFT.index = %d, want %d", c.key.Index, optShift)
				}
			}
			if c.key.Type == "OPTION" && c.key.Name == "COMMODORE" {
				sawCBM = true
				if c.key.Index != optCommodore {
					t.Errorf("COMMODORE.index = %d, want %d", c.key.Index, optCommodore)
				}
			}
		}
	}
	if !sawShift {
		t.Error("SHIFT OPTION key not found in layout")
	}
	if !sawCBM {
		t.Error("COMMODORE OPTION key not found in layout")
	}
}
