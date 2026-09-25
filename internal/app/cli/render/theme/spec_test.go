package theme

import "testing"

func TestParseColorRef(t *testing.T) {
	t.Parallel()
	ok := []string{"fg", "accent", "none", "#fff", "#FF79C6", "0", "15", "255"}
	for _, s := range ok {
		if _, err := ParseColorRef(s); err != nil {
			t.Errorf("ParseColorRef(%q) error = %v, want nil", s, err)
		}
	}
	bad := []string{"", "purple", "#ff", "#gggggg", "256", "-1", "1.5"}
	for _, s := range bad {
		if _, err := ParseColorRef(s); err == nil {
			t.Errorf("ParseColorRef(%q) error = nil, want an error", s)
		}
	}
}

func TestSpecOverIsFieldByField(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	base := Spec{Fg: Ref(SlotGreen), Bold: &yes, Faint: &yes}
	top := Spec{Faint: &no, Italic: &yes}
	got := top.Over(base)
	if got.Fg != base.Fg {
		t.Error("Over dropped the inherited Fg")
	}
	if got.Bold == nil || !*got.Bold {
		t.Error("Over dropped the inherited Bold")
	}
	if got.Faint == nil || *got.Faint {
		t.Error("explicit Faint=false did not clear the inherited Faint")
	}
	if got.Italic == nil || !*got.Italic {
		t.Error("Over dropped the top Italic")
	}
}
