package theme

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// Huh returns a huh theme colored from the scheme's palette, so prompts match
// the rest of the CLI. It starts from huh's uncolored base and sets only
// colors; huh's layout and glyphs are kept.
func Huh(s Scheme) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		t := huh.ThemeBase(s.Theme.Dark)
		accent, green, red := s.Color(SlotAccent), s.Color(SlotGreen), s.Color(SlotRed)
		muted, subtle, fg := s.Color(SlotMuted), s.Color(SlotSubtle), s.Color(SlotFg)

		f := &t.Focused
		f.Base = f.Base.BorderForeground(subtle)
		f.Card = f.Base
		f.Title = f.Title.Foreground(accent).Bold(true)
		f.NoteTitle = f.NoteTitle.Foreground(accent).Bold(true)
		f.Description = f.Description.Foreground(muted)
		f.ErrorIndicator = f.ErrorIndicator.Foreground(red)
		f.ErrorMessage = f.ErrorMessage.Foreground(red)
		f.SelectSelector = f.SelectSelector.Foreground(accent)
		f.NextIndicator = f.NextIndicator.Foreground(accent)
		f.PrevIndicator = f.PrevIndicator.Foreground(accent)
		// The ansi theme's Fg is ANSI 15 (bright white), picked to reproduce
		// pre-theme defaults elsewhere — but here it would paint every option
		// bright white regardless of the terminal's own palette, which is
		// unreadable on a light background. Leaving it unset falls back to
		// the terminal's default text color instead. Named themes keep fg,
		// since their Fg is chosen to read well against their own Dark bit.
		if s.Theme.Name != Ansi.Name {
			f.Option = f.Option.Foreground(fg)
		}
		f.MultiSelectSelector = f.MultiSelectSelector.Foreground(accent)
		f.SelectedOption = f.SelectedOption.Foreground(green)
		f.SelectedPrefix = f.SelectedPrefix.Foreground(green)
		if s.Theme.Name != Ansi.Name {
			f.UnselectedOption = f.UnselectedOption.Foreground(fg)
		}
		f.TextInput.Cursor = f.TextInput.Cursor.Foreground(accent)
		f.TextInput.Placeholder = f.TextInput.Placeholder.Foreground(muted)
		f.TextInput.Prompt = f.TextInput.Prompt.Foreground(accent)

		t.Blurred = *f
		t.Blurred.Base = t.Blurred.Base.BorderStyle(lipglossHidden)
		t.Blurred.Card = t.Blurred.Base
		t.Blurred.Title = t.Blurred.Title.Foreground(muted).Bold(false)
		t.Blurred.NextIndicator = t.Blurred.NextIndicator.UnsetForeground()
		t.Blurred.PrevIndicator = t.Blurred.PrevIndicator.UnsetForeground()

		t.Help.ShortKey = t.Help.ShortKey.Foreground(muted)
		t.Help.ShortDesc = t.Help.ShortDesc.Foreground(subtle)
		t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(subtle)
		t.Help.FullKey = t.Help.FullKey.Foreground(muted)
		t.Help.FullDesc = t.Help.FullDesc.Foreground(subtle)
		t.Help.FullSeparator = t.Help.FullSeparator.Foreground(subtle)
		return t
	})
}

// lipglossHidden hides the blurred field's border while keeping its width, as
// huh's own themes do.
var lipglossHidden = lipgloss.HiddenBorder()
