package tray

import (
	"fmt"
	"time"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// Theme colors keep both the keycaps and attention cue readable in light and dark themes.
const shortcutsCSS = `
.shortcut-keycap {
 font-family: monospace;
 font-weight: bold;
 color: @theme_fg_color;
 background-color: shade(@theme_bg_color, 1.08);
 border: 1px solid alpha(@theme_fg_color, 0.22);
 border-bottom-width: 2px;
 border-radius: 5px;
 padding: 5px 8px;
}
.shortcut-section { color: @theme_fg_color; }
.shortcut-undo {
 background-color: alpha(@theme_selected_bg_color, 0.10);
 border-radius: 6px;
 padding: 9px 12px;
}
`

// Native MenuButton/Popover provide keyboard activation and Escape dismissal.
func (w *SettingsWindow) createConversionShortcutsButton() (*gtk.MenuButton, error) {
	var err error
	w.conversionShortcutsStyle, err = gtk.CssProviderNew()
	if err != nil {
		return nil, err
	}
	if err := w.conversionShortcutsStyle.LoadFromData(shortcutsCSS); err != nil {
		return nil, err
	}
	w.conversionShortcutsButton, err = gtk.MenuButtonNew()
	if err != nil {
		return nil, err
	}
	icon, iconErr := gtk.LabelNew(strShortcutsInfoIcon)
	if iconErr != nil {
		return nil, iconErr
	}
	w.shortcutsAttentionDot, err = gtk.LabelNew("•")
	if err != nil {
		return nil, err
	}
	w.shortcutsAttentionDot.SetOpacity(0)
	buttonContent, contentErr := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	if contentErr != nil {
		return nil, contentErr
	}
	buttonContent.SetHAlign(gtk.ALIGN_CENTER)
	buttonContent.PackStart(icon, false, false, 0)
	buttonContent.PackStart(w.shortcutsAttentionDot, false, false, 0)
	w.conversionShortcutsButton.Add(buttonContent)
	w.conversionShortcutsButton.SetTooltipText(strConversionShortcutsTitle)
	w.conversionShortcutsButton.SetRelief(gtk.RELIEF_NONE)
	w.conversionShortcutsButton.SetCanFocus(true)
	// Reserve room for the unread dot so changing a preset never shifts the controls.
	w.conversionShortcutsButton.SetSizeRequest(42, -1)
	if err := w.styleShortcutWidget(&w.conversionShortcutsButton.Widget, "shortcut-info"); err != nil {
		return nil, err
	}
	w.shortcutsFlashStyle, err = gtk.CssProviderNew()
	if err != nil {
		return nil, err
	}
	style, err := w.conversionShortcutsButton.GetStyleContext()
	if err != nil {
		return nil, err
	}
	style.AddProvider(w.shortcutsFlashStyle, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION+1)
	w.conversionShortcutsButton.Connect("destroy", w.stopShortcutsFlash)
	w.conversionShortcutsPopover, err = gtk.PopoverNew(w.conversionShortcutsButton)
	if err != nil {
		return nil, err
	}
	w.conversionShortcutsPopover.SetPosition(gtk.POS_BOTTOM)
	w.conversionShortcutsPopover.SetNoShowAll(true)
	w.conversionShortcutsPopover.Connect("show", func() { w.setShortcutsAttention(false) })
	content, err := w.createShortcutsContent()
	if err != nil {
		return nil, err
	}
	content.ShowAll()
	w.conversionShortcutsPopover.Add(content)
	w.conversionShortcutsButton.SetPopover(w.conversionShortcutsPopover)
	return w.conversionShortcutsButton, nil
}

func (w *SettingsWindow) styleShortcutWidget(widget *gtk.Widget, class string) error {
	style, err := widget.GetStyleContext()
	if err != nil {
		return err
	}
	style.AddProvider(w.conversionShortcutsStyle, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	style.AddClass(class)
	return nil
}

func (w *SettingsWindow) createShortcutsContent() (*gtk.Grid, error) {
	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(20)
	grid.SetRowSpacing(9)
	grid.SetMarginTop(16)
	grid.SetMarginBottom(16)
	grid.SetMarginStart(16)
	grid.SetMarginEnd(16)
	title, err := shortcutLabel("<span size='large' weight='bold'>" + glib.MarkupEscapeText(strConversionShortcutsTitle) + "</span>")
	if err != nil {
		return nil, err
	}
	grid.Attach(title, 0, 0, 2, 1)
	w.conversionShortcutsMode, err = shortcutLabel("")
	if err != nil {
		return nil, err
	}
	if err := w.styleShortcutWidget(&w.conversionShortcutsMode.Widget, "dim-label"); err != nil {
		return nil, err
	}
	grid.Attach(w.conversionShortcutsMode, 0, 1, 2, 1)
	for i, action := range []string{strShortcutsWord, strShortcutsLine, strShortcutsLayout, strShortcutsCase} {
		row := 3 + i
		if i >= 2 {
			row++
		}
		actionLabel, labelErr := shortcutLabel("<b>" + glib.MarkupEscapeText(action) + "</b>")
		if labelErr != nil {
			return nil, labelErr
		}
		grid.Attach(actionLabel, 0, row, 1, 1)
		keycap, keyErr := shortcutLabel("")
		if keyErr != nil {
			return nil, keyErr
		}
		if keyErr := w.styleShortcutWidget(&keycap.Widget, "shortcut-keycap"); keyErr != nil {
			return nil, keyErr
		}
		w.conversionShortcutKeys[i] = keycap
		grid.Attach(keycap, 1, row, 1, 1)
	}
	for i, section := range []string{strShortcutsTyped, strShortcutsSelected} {
		label, labelErr := shortcutLabel("<span size='small' weight='bold' letter_spacing='700'>" + glib.MarkupEscapeText(section) + "</span>")
		if labelErr != nil {
			return nil, labelErr
		}
		label.SetMarginTop(8)
		if labelErr := w.styleShortcutWidget(&label.Widget, "shortcut-section"); labelErr != nil {
			return nil, labelErr
		}
		grid.Attach(label, 0, 2+3*i, 2, 1)
	}
	w.conversionShortcutsNote, err = shortcutLabel(glib.MarkupEscapeText(strShortcutsDoubleTap))
	if err != nil {
		return nil, err
	}
	w.conversionShortcutsNote.SetNoShowAll(true)
	if err := w.styleShortcutWidget(&w.conversionShortcutsNote.Widget, "dim-label"); err != nil {
		return nil, err
	}
	grid.Attach(w.conversionShortcutsNote, 0, 8, 2, 1)
	undo, err := shortcutLabel("<b>↶  " + glib.MarkupEscapeText(strShortcutsUndoTitle) + "</b>\n" + glib.MarkupEscapeText(strShortcutsUndo))
	if err != nil {
		return nil, err
	}
	undo.SetMarginTop(8)
	undo.SetHAlign(gtk.ALIGN_FILL)
	if err := w.styleShortcutWidget(&undo.Widget, "shortcut-undo"); err != nil {
		return nil, err
	}
	grid.Attach(undo, 0, 9, 2, 1)
	return grid, nil
}

func shortcutLabel(markup string) (*gtk.Label, error) {
	label, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	label.SetMarkup(markup)
	label.SetHAlign(gtk.ALIGN_START)
	label.SetXAlign(0)
	return label, nil
}

func (w *SettingsWindow) updateConversionShortcuts() {
	if w.conversionModifiersCombo == nil || w.conversionShortcutKeys[0] == nil || w.convertKeyCombo.GetActive() < 0 {
		return
	}
	value := w.getConvertKeyValueForWarning()
	if value == "0" && w.conversionModifiersCombo.GetActive() != 0 {
		w.conversionModifiersCombo.SetActive(0)
	}
	custom := value != "0" && value != "custom"
	w.conversionModifiersCombo.SetSensitive(custom)
	keys := [4]string{"Shift ×2", "Shift + " + strShortcutsOtherShift + " ×2", "Ctrl + Shift ×2", "Ctrl + Shift + " + strShortcutsOtherShift + " ×2"}
	mode := w.convertKeyCombo.GetActiveText()
	if custom {
		line, selection := "Shift", "Ctrl"
		if w.conversionModifiersCombo.GetActive() == 1 {
			line, selection = "Ctrl", "Shift"
		}
		keys = [4]string{mode, line + " + " + mode, selection + " + " + mode, "Ctrl + Shift + " + mode}
		mode = fmt.Sprintf("%s · %s", mode, w.conversionModifiersCombo.GetActiveText())
	} else {
		w.setShortcutsAttention(false)
	}
	w.conversionShortcutsMode.SetText(mode)
	w.conversionShortcutsNote.SetVisible(!custom)
	for i, key := range keys {
		w.conversionShortcutKeys[i].SetText(key)
	}
}

func (w *SettingsWindow) onConversionPresetChanged() {
	w.updateConversionShortcuts()
	if w.loadingShortcutConfig || !w.conversionModifiersCombo.GetSensitive() {
		return
	}
	w.setShortcutsAttention(!w.conversionShortcutsPopover.GetVisible())
}

func (w *SettingsWindow) setShortcutsAttention(attention bool) {
	if w.conversionShortcutsButton == nil {
		return
	}
	style, err := w.conversionShortcutsButton.GetStyleContext()
	if err != nil {
		return
	}
	if attention {
		style.AddClass("shortcut-attention")
		w.shortcutsAttentionDot.SetOpacity(1)
		w.conversionShortcutsButton.SetTooltipText(strShortcutsChanged)
		w.startShortcutsFlash()
	} else {
		style.RemoveClass("shortcut-attention")
		w.stopShortcutsFlash()
		w.shortcutsAttentionDot.SetOpacity(0)
		w.conversionShortcutsButton.SetTooltipText(strConversionShortcutsTitle)
	}
}

// Drive this small attention cue independently of gtk-enable-animations: it is
// explicitly requested feedback, even on desktops with GTK transitions disabled.
func (w *SettingsWindow) startShortcutsFlash() {
	w.stopShortcutsFlash()
	started := time.Now()
	w.paintShortcutsFlash(1)
	w.shortcutsFlashSource = glib.TimeoutAdd(16, func() bool {
		progress := float64(time.Since(started)) / float64(900*time.Millisecond)
		if progress >= 1 {
			w.shortcutsFlashSource = 0
			w.paintShortcutsFlash(0)
			return false
		}
		// Fade the subtle outline smoothly without a bright hold.
		strength := (1 - progress) * (1 - progress)
		w.paintShortcutsFlash(strength)
		return true
	})
}

func (w *SettingsWindow) stopShortcutsFlash() {
	if w.shortcutsFlashSource != 0 {
		glib.SourceRemove(w.shortcutsFlashSource)
		w.shortcutsFlashSource = 0
	}
	if w.shortcutsFlashStyle != nil {
		w.paintShortcutsFlash(0)
	}
}

func (w *SettingsWindow) paintShortcutsFlash(strength float64) {
	css := ""
	if strength > 0 {
		css = fmt.Sprintf(`.shortcut-info {
   transition: none;
   border-color: alpha(@theme_selected_bg_color, %[1]f);
   box-shadow: 0 0 5px 1px alpha(@theme_selected_bg_color, %[2]f);
  }`, 0.20*strength, 0.35*strength)
	}
	// Only numeric values enter this fixed, locally validated CSS template.
	_ = w.shortcutsFlashStyle.LoadFromData(css)
}
