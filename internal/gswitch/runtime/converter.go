package runtime

// Action represents what conversion action to take
type Action int

const (
	ActionNone Action = iota
	ActionConvertWord
	ActionConvertAll
	// ActionDoubleShiftNoText: double-shift trigger fired but the buffer has
	// no text. Nothing to convert in the buffer, but the caller may act on
	// the trigger itself (Ctrl+DoubleShift converts the selection).
	ActionDoubleShiftNoText
)

// KeyEvent represents a key event in the buffer
type KeyEvent struct {
	Code  uint16
	Value int32
}

// Pattern represents a pattern element for matching
type Pattern struct {
	Code      int  // key code, or -1 for ANY_SHIFT
	Value     int  // expected value (0=up, 1=down)
	Condition bool // true = must match, false = must NOT match
}

const (
	K_UP     = 0
	K_DOWN   = 1
	K_REPEAT = 2

	ANY_SHIFT = -1 // special placeholder for any shift key
)

// DebugLogger is a function type for debug logging
type DebugLogger func(format string, args ...any)

// Converter handles key buffering and conversion logic
type Converter struct {
	ConvKey uint16   // key to trigger conversion (0 = double-shift)
	LSKeys  []uint16 // keys to switch layout

	buffer   []KeyEvent
	debugLog DebugLogger
}

// NewConverter creates a new Converter
func NewConverter() *Converter {
	return &Converter{
		buffer: make([]KeyEvent, 0, 256),
	}
}

// SetDebugLogger sets the debug logging function
func (c *Converter) SetDebugLogger(fn DebugLogger) {
	c.debugLog = fn
}

// log calls debugLog if set
func (c *Converter) log(format string, args ...any) {
	if c.debugLog != nil {
		c.debugLog(format, args...)
	}
}

// isShift checks if key is a shift key
func (c *Converter) isShift(code uint16) bool {
	return Shifts[code]
}

// isKey checks if key is a regular key (letter, number, etc.)
func (c *Converter) isKey(code uint16) bool {
	return Letters[code]
}

// isBackspace checks if key is backspace
func (c *Converter) isBackspace(code uint16) bool {
	return code == KEY_BACKSPACE
}

// isKiller checks if key should clear the buffer
func (c *Converter) isKiller(code uint16) bool {
	return BufKillers[code]
}

// isCtrl checks if key is a Ctrl key
func (c *Converter) isCtrl(code uint16) bool {
	return code == KEY_LEFTCTRL || code == KEY_RIGHTCTRL
}

func (c *Converter) isConvKey(code uint16) bool {
	return c.ConvKey != 0 && code == c.ConvKey
}

// Push adds a key event to the buffer
// Returns true if buffer was modified
func (c *Converter) Push(code uint16, value int32) bool {
	// Clear buffer on killer keys (except Ctrl which is used for selection conversion)
	if c.isKiller(code) && !c.isCtrl(code) && value != K_REPEAT {
		c.ClearBuffer()
		return true
	}

	// Prevent unbounded buffer growth
	if len(c.buffer) >= MaxKeyBufSize {
		// Remove oldest half of the buffer to make room
		copy(c.buffer, c.buffer[MaxKeyBufSize/2:])
		c.buffer = c.buffer[:MaxKeyBufSize/2]
	}

	// Handle convert key (if not using double-shift)
	if c.ConvKey != 0 && code == c.ConvKey && value != K_REPEAT {
		c.buffer = append(c.buffer, KeyEvent{Code: code, Value: value})
		return true
	}

	// Handle shift keys
	if c.isShift(code) && value != K_REPEAT {
		c.buffer = append(c.buffer, KeyEvent{Code: code, Value: value})
		return true
	}

	// Handle backspace
	//nolint:nestif // backspace handling requires checking multiple conditions
	if c.isBackspace(code) && value != K_UP {
		// Remove the most recent text key (ignore shift + convkey triggers)
		for i := len(c.buffer) - 1; i >= 0; i-- {
			if !c.isShift(c.buffer[i].Code) && !c.isConvKey(c.buffer[i].Code) {
				c.buffer = append(c.buffer[:i], c.buffer[i+1:]...)
				break
			}
		}

		// Remove trailing shift down/up pairs
		for len(c.buffer) >= 2 {
			n := len(c.buffer)
			last := c.buffer[n-1]
			prev := c.buffer[n-2]

			if c.isShift(last.Code) && last.Value == K_UP &&
				c.isShift(prev.Code) && prev.Value == K_DOWN &&
				last.Code == prev.Code {
				c.buffer = c.buffer[:n-2]
			} else {
				break
			}
		}

		// Remove double shift artifacts (shift1 down, shift2 down, shift1 up, shift2 up)
		for len(c.buffer) >= 4 {
			n := len(c.buffer)
			if c.isShift(c.buffer[n-1].Code) && c.buffer[n-1].Value == K_UP &&
				c.isShift(c.buffer[n-2].Code) && c.buffer[n-2].Value == K_UP &&
				c.isShift(c.buffer[n-3].Code) && c.buffer[n-3].Value == K_DOWN &&
				c.isShift(c.buffer[n-4].Code) && c.buffer[n-4].Value == K_DOWN {
				c.buffer = c.buffer[:n-4]
			} else {
				break
			}
		}

		return true
	}

	// Handle regular keys (only on key down, ignore repeat)
	if c.isKey(code) && value == K_DOWN {
		c.buffer = append(c.buffer, KeyEvent{Code: code, Value: K_DOWN})
		return true
	}

	return false
}

// Process checks if conversion should be triggered
// Returns the action to take
func (c *Converter) Process() Action {
	if len(c.buffer) == 0 {
		return ActionNone
	}

	if c.ConvKey == 0 {
		// Double-shift mode (default)
		return c.processDoubleShift()
	}

	// Custom convert key mode
	return c.processConvKey()
}

// processDoubleShift handles double-shift conversion triggers
func (c *Converter) processDoubleShift() Action {
	n := len(c.buffer)

	// Count non-shift keys in buffer to check if there's text to convert
	hasText := false
	for _, ev := range c.buffer {
		if !c.isShift(ev.Code) {
			hasText = true
			break
		}
	}

	// 1. Double shift without other shift pressed -> convert word
	// Pattern: shift1↓ shift1↑ shift1↓ shift1↑ (same shift key pressed twice)
	// Need at least 4 shift events at the end of buffer AND some text before
	if n >= 4 && hasText && c.isDoubleShiftPattern() {
		c.trimBuffer()
		return ActionConvertWord
	}

	// 2. Double shift with other shift pressed -> convert all
	// Pattern: shift1↓ shift2↓ shift2↑ shift2↓ shift2↑ shift1↑
	// (one shift held while the other is double-tapped)
	if n >= 6 && hasText && c.isDoubleShiftWithHeldPattern() {
		c.trimBuffer()
		return ActionConvertAll
	}

	// 3. Double shift with only shifts in the buffer (no letters):
	// no buffer conversion to do, but report the trigger so the caller can
	// still act on it (Ctrl+DoubleShift -> selection conversion)
	if !hasText && c.isDoubleShiftPattern() {
		c.trimBuffer()
		return ActionDoubleShiftNoText
	}

	return ActionNone
}

// isDoubleShiftPattern checks if the buffer ends with a double-tap of the SAME shift key
func (c *Converter) isDoubleShiftPattern() bool {
	n := len(c.buffer)
	if n < 4 {
		return false
	}

	// Get the last 4 events
	e1 := c.buffer[n-4] // first shift down
	e2 := c.buffer[n-3] // first shift up
	e3 := c.buffer[n-2] // second shift down
	e4 := c.buffer[n-1] // second shift up

	// All must be the SAME shift key
	if !c.isShift(e1.Code) || !c.isShift(e2.Code) || !c.isShift(e3.Code) || !c.isShift(e4.Code) {
		return false
	}

	// Must be the same shift key for all 4 events
	if e1.Code != e2.Code || e2.Code != e3.Code || e3.Code != e4.Code {
		return false
	}

	// Check the pattern: down, up, down, up
	if e1.Value != K_DOWN || e2.Value != K_UP || e3.Value != K_DOWN || e4.Value != K_UP {
		return false
	}

	// Make sure there's no other shift held before this pattern
	if n > 4 {
		prev := c.buffer[n-5]
		if c.isShift(prev.Code) && prev.Value == K_DOWN {
			return false // Another shift is held - this should be ConvertAll, not ConvertWord
		}
	}

	return true
}

// isDoubleShiftWithHeldPattern checks for: shift1↓ shift2↓ shift2↑ shift2↓ shift2↑ shift1↑
func (c *Converter) isDoubleShiftWithHeldPattern() bool {
	n := len(c.buffer)
	if n < 6 {
		return false
	}

	// Get the last 6 events
	e1 := c.buffer[n-6] // shift1 down (held)
	e2 := c.buffer[n-5] // shift2 down (first tap)
	e3 := c.buffer[n-4] // shift2 up
	e4 := c.buffer[n-3] // shift2 down (second tap)
	e5 := c.buffer[n-2] // shift2 up
	e6 := c.buffer[n-1] // shift1 up (released)

	// All must be shift keys
	if !c.isShift(e1.Code) || !c.isShift(e2.Code) || !c.isShift(e3.Code) ||
		!c.isShift(e4.Code) || !c.isShift(e5.Code) || !c.isShift(e6.Code) {
		return false
	}

	// e1 and e6 must be the same shift (the held one)
	if e1.Code != e6.Code {
		return false
	}

	// e2, e3, e4, e5 must be the same shift (the tapped one)
	if e2.Code != e3.Code || e3.Code != e4.Code || e4.Code != e5.Code {
		return false
	}

	// The held shift and tapped shift must be different
	if e1.Code == e2.Code {
		return false
	}

	// Check the pattern: down, up, down, up, up
	if e1.Value != K_DOWN || e2.Value != K_DOWN || e3.Value != K_UP ||
		e4.Value != K_DOWN || e5.Value != K_UP || e6.Value != K_UP {
		return false
	}

	return true
}

// processConvKey handles custom convert key triggers
func (c *Converter) processConvKey() Action {
	convKey := int(c.ConvKey)

	// 1. Convert key without shift -> convert word
	if c.bufferMatchesPattern([]Pattern{
		{ANY_SHIFT, K_DOWN, false}, // no shift is down
		{convKey, K_DOWN, true},
		{convKey, K_UP, true},
	}) {
		c.trimBuffer()
		return ActionConvertWord
	}

	// 2. Convert key with shift pressed -> convert all
	if c.bufferMatchesPattern([]Pattern{
		{ANY_SHIFT, K_DOWN, true},
		{convKey, K_DOWN, true},
		{convKey, K_UP, true},
		{ANY_SHIFT, K_UP, true},
	}) {
		c.trimBuffer()
		return ActionConvertAll
	}

	// 3. Convert key with shift released before conv_key
	if c.bufferMatchesPattern([]Pattern{
		{ANY_SHIFT, K_DOWN, true},
		{convKey, K_DOWN, true},
		{ANY_SHIFT, K_UP, true},
		{convKey, K_UP, true},
	}) {
		c.trimBuffer()
		return ActionConvertAll
	}

	// 4. Just switch layout if buffer is empty (only convert key)
	if len(c.buffer) == 2 && c.bufferMatchesPattern([]Pattern{
		{convKey, K_DOWN, true},
		{convKey, K_UP, true},
	}) {
		c.trimBuffer()
		return ActionConvertAll
	}

	return ActionNone
}

// bufferMatchesPattern checks if the tail of buffer matches a pattern
// Special handling: if the first pattern element has Condition=false,
// it checks the element BEFORE the pattern (not part of the pattern itself)
func (c *Converter) bufferMatchesPattern(pattern []Pattern) bool {
	if len(pattern) == 0 {
		return false
	}

	// Check if first element is a "must NOT match" condition
	// This means we check the element BEFORE the actual pattern
	actualPatternStart := 0
	if !pattern[0].Condition {
		actualPatternStart = 1
	}

	actualPatternLen := len(pattern) - actualPatternStart
	if len(c.buffer) < actualPatternLen {
		return false
	}

	startIdx := len(c.buffer) - actualPatternLen

	// Check the "must NOT match" condition (element before the pattern)
	//nolint:nestif // pattern matching requires nested condition checks
	if actualPatternStart == 1 {
		p := pattern[0]
		// Check if there's an element before the pattern
		if startIdx > 0 {
			ev := c.buffer[startIdx-1]
			var matches bool
			if p.Code == ANY_SHIFT {
				matches = c.isShift(ev.Code) && int(ev.Value) == p.Value
			} else {
				matches = int(ev.Code) == p.Code && int(ev.Value) == p.Value
			}
			// Condition is false, so if it matches, the pattern fails
			if matches {
				return false
			}
		}
		// If there's no element before, the "not match" condition is satisfied
	}

	// Check the actual pattern
	for i := actualPatternStart; i < len(pattern); i++ {
		p := pattern[i]
		ev := c.buffer[startIdx+(i-actualPatternStart)]

		var matches bool
		if p.Code == ANY_SHIFT {
			matches = c.isShift(ev.Code) && int(ev.Value) == p.Value
		} else {
			matches = int(ev.Code) == p.Code && int(ev.Value) == p.Value
		}

		if matches != p.Condition {
			return false
		}
	}

	return true
}

// trimBuffer removes trailing non-key events, but preserves shift release after a key
func (c *Converter) trimBuffer() {
	beforeLen := len(c.buffer)

	for len(c.buffer) > 0 {
		last := c.buffer[len(c.buffer)-1]

		// Always trim trailing convert-key events (they are a trigger, not text)
		if c.isConvKey(last.Code) {
			c.buffer = c.buffer[:len(c.buffer)-1]
			continue
		}

		if c.isKey(last.Code) {
			break
		}

		// Keep shift up if it follows a key (possibly with convkey events in-between).
		// This prevents "stuck shift" when user typed with shift held, then pressed ConvKey,
		// and the shift release comes after ConvKey events.
		if c.isShift(last.Code) && last.Value == K_UP {
			j := len(c.buffer) - 2
			for j >= 0 && c.isConvKey(c.buffer[j].Code) {
				j--
			}
			if j >= 0 && c.isKey(c.buffer[j].Code) {
				break
			}
		}

		c.buffer = c.buffer[:len(c.buffer)-1]
	}

	c.log("trimBuffer: length %d -> %d", beforeLen, len(c.buffer))
}

// HasText checks if buffer contains any non-shift keys
func (c *Converter) HasText() bool {
	for _, ev := range c.buffer {
		if !c.isShift(ev.Code) && !c.isConvKey(ev.Code) {
			return true
		}
	}
	return false
}

// Convert generates the sequence of keys to emit for conversion
func (c *Converter) Convert(action Action) []KeyEvent {
	// Don't do anything if there's no text to convert
	if !c.HasText() {
		c.log("convert: no text to convert")
		return nil
	}

	actionName := "word"
	if action == ActionConvertAll {
		actionName = "all"
	}
	c.log("convert: action=%s bufferLen=%d", actionName, len(c.buffer))

	// Pre-allocate result slice with estimated capacity
	result := make([]KeyEvent, 0, len(c.buffer)*3+len(c.LSKeys)*2)

	// Switch layout
	for _, key := range c.LSKeys {
		result = append(result, KeyEvent{Code: key, Value: K_DOWN})
	}
	for i := len(c.LSKeys) - 1; i >= 0; i-- {
		result = append(result, KeyEvent{Code: c.LSKeys[i], Value: K_UP})
	}

	startIndex := 0

	// Find start of last word for ConvertWord
	if action == ActionConvertWord {
		// Skip trailing SPACE and ENTER
		i := len(c.buffer) - 1
		for i >= 0 && (c.buffer[i].Code == KEY_SPACE ||
			c.buffer[i].Code == KEY_ENTER ||
			c.buffer[i].Code == KEY_KPENTER) {
			i--
		}

		// Move back to start of word
		for i >= 0 && c.buffer[i].Code != KEY_SPACE &&
			c.buffer[i].Code != KEY_ENTER &&
			c.buffer[i].Code != KEY_KPENTER {
			i--
		}

		startIndex = i + 1
	}

	// Find start of line for ConvertAll
	if action == ActionConvertAll {
		i := len(c.buffer) - 1

		// Skip trailing ENTER
		for i >= 0 && (c.buffer[i].Code == KEY_ENTER ||
			c.buffer[i].Code == KEY_KPENTER) {
			i--
		}

		// Move back to start of line
		for i >= 0 && c.buffer[i].Code != KEY_ENTER &&
			c.buffer[i].Code != KEY_KPENTER {
			i--
		}

		startIndex = i + 1
	}

	c.log("convert: startIndex=%d (converting %d of %d buffer events)", startIndex, len(c.buffer)-startIndex, len(c.buffer))

	// Count and send backspaces for each key
	backspaceCount := 0
	for i := startIndex; i < len(c.buffer); i++ {
		if !c.isShift(c.buffer[i].Code) && !c.isConvKey(c.buffer[i].Code) {
			backspaceCount++
			result = append(result,
				KeyEvent{Code: KEY_BACKSPACE, Value: K_DOWN},
				KeyEvent{Code: KEY_BACKSPACE, Value: K_UP})
		}
	}
	c.log("convert: backspaces=%d", backspaceCount)

	replayCount := 0
	for i := startIndex; i < len(c.buffer); i++ {
		if c.isConvKey(c.buffer[i].Code) {
			continue
		}
		replayCount++
	}
	c.log("convert: replay events=%d", replayCount)

	// Replay the buffer
	shiftDown := make(map[uint16]bool, 2)
	for i := startIndex; i < len(c.buffer); i++ {
		if c.isConvKey(c.buffer[i].Code) {
			continue
		}
		if c.isShift(c.buffer[i].Code) {
			switch c.buffer[i].Value {
			case K_DOWN:
				shiftDown[c.buffer[i].Code] = true
			case K_UP:
				shiftDown[c.buffer[i].Code] = false
			}
		}
		result = append(result, c.buffer[i])
		if !c.isShift(c.buffer[i].Code) {
			result = append(result, KeyEvent{Code: c.buffer[i].Code, Value: K_UP})
		}
	}

	// Ensure shifts are released even if conversion was triggered before we observed shift-up.
	extraShiftReleases := 0
	for shiftKey, down := range shiftDown {
		if down {
			result = append(result, KeyEvent{Code: shiftKey, Value: K_UP})
			extraShiftReleases++
		}
	}

	c.log("convert: total emit events=%d (layout_switch=%d, backspaces=%d, replay+keyups=%d, extra_shift_releases=%d)",
		len(result), len(c.LSKeys)*2, backspaceCount*2, len(result)-len(c.LSKeys)*2-backspaceCount*2-extraShiftReleases, extraShiftReleases)

	return result
}

// GetBuffer returns a copy of the current buffer
func (c *Converter) GetBuffer() []KeyEvent {
	buf := make([]KeyEvent, len(c.buffer))
	copy(buf, c.buffer)
	return buf
}

// ClearBuffer clears the key buffer
func (c *Converter) ClearBuffer() {
	c.buffer = c.buffer[:0]
}

// BufferLen returns the buffer length
func (c *Converter) BufferLen() int {
	return len(c.buffer)
}
