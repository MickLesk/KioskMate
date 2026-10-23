package hardware

import (
	"bufio"
	"fmt"
	"io"
	"math/big"
	"strings"
)

// Two problems are invisible from the Admin UI but explain most "the kiosk
// stopped reacting" reports on a Raspberry Pi: a weak power supply and a
// touchscreen the kernel never registered. Undervoltage makes the Pi throttle
// and drop USB devices, so both are reported together.

// Linux input event codes (include/uapi/linux/input-event-codes.h).
const (
	absX          = 0x00
	absMTPosition = 0x35
	btnTouch      = 0x14a
)

// touchDevices returns the names of input devices in /proc/bus/input/devices
// format that report touch: multi-touch positions, or absolute X with
// BTN_TOUCH (single-touch panels and most USB touch controllers). wordBits is
// the kernel's long size, which sets how the capability bitmaps are split.
func touchDevices(r io.Reader, wordBits uint) []string {
	var names []string
	var name, abs, key string
	flush := func() {
		if name != "" && isTouch(abs, key, wordBits) {
			names = append(names, name)
		}
		name, abs, key = "", "", ""
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "N: Name="):
			name = strings.Trim(strings.TrimPrefix(line, "N: Name="), `"`)
		case strings.HasPrefix(line, "B: ABS="):
			abs = strings.TrimPrefix(line, "B: ABS=")
		case strings.HasPrefix(line, "B: KEY="):
			key = strings.TrimPrefix(line, "B: KEY=")
		}
	}
	flush()
	return names
}

func isTouch(abs, key string, wordBits uint) bool {
	if abs == "" {
		return false
	}
	return bitSet(abs, absMTPosition, wordBits) || (bitSet(abs, absX, wordBits) && bitSet(key, btnTouch, wordBits))
}

// bitSet reads a kernel capability bitmap: space-separated hex words, most
// significant first, each one kernel long wide.
func bitSet(bitmap string, bit int, wordBits uint) bool {
	words := strings.Fields(bitmap)
	if len(words) == 0 {
		return false
	}
	value := new(big.Int)
	for _, word := range words {
		part, ok := new(big.Int).SetString(word, 16)
		if !ok {
			return false
		}
		value.Lsh(value, wordBits).Or(value, part)
	}
	return value.Bit(bit) == 1
}

// parseThrottled reads `vcgencmd get_throttled` output ("throttled=0x50005").
// Bits 0-3 are the current state, bits 16-19 latch since boot.
func parseThrottled(output string) (flags uint64, ok bool) {
	_, value, found := strings.Cut(strings.TrimSpace(output), "=")
	if !found {
		return 0, false
	}
	parsed, ok := new(big.Int).SetString(strings.TrimPrefix(strings.ToLower(value), "0x"), 16)
	if !ok || !parsed.IsUint64() {
		return 0, false
	}
	return parsed.Uint64(), true
}

// powerStatus combines the live hwmon alarm with the since-boot bits from
// vcgencmd. It returns nil when neither source exists (not a Raspberry Pi).
func powerStatus(alarm string, throttled string) map[string]any {
	out := map[string]any{}
	switch strings.TrimSpace(alarm) {
	case "1":
		out["undervoltage_now"] = true
	case "0":
		out["undervoltage_now"] = false
	}
	if flags, ok := parseThrottled(throttled); ok {
		out["throttled"] = fmt.Sprintf("0x%x", flags)
		if flags&0x1 != 0 {
			out["undervoltage_now"] = true
		} else if _, set := out["undervoltage_now"]; !set {
			out["undervoltage_now"] = false
		}
		out["undervoltage_since_boot"] = flags&0x10000 != 0
		out["throttled_now"] = flags&0x4 != 0
		out["throttled_since_boot"] = flags&0x40000 != 0
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
