// ABOUTME: Text-based filesystem for cartridge banking state (MBC-specific fields).
// ABOUTME: Type-switches on MBC type to provide appropriate state fields.

package ninep

import (
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/Humpheh/goboy/pkg/cart"
	"github.com/Humpheh/goboy/pkg/gb"
)

type cartridgeStateFS struct {
	gb *gb.Gameboy
}

func newCartridgeStateFS(gameboy *gb.Gameboy) *cartridgeStateFS {
	return &cartridgeStateFS{gb: gameboy}
}

func (fsys *cartridgeStateFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &cartridgeStateFile{parent: fsys}, nil
}

type cartridgeStateFile struct {
	parent  *cartridgeStateFS
	readPos int
}

func (f *cartridgeStateFile) Read(buf []byte) (int, error) {
	// Get cartridge
	f.parent.gb.Mu.RLock()
	cartridge := f.parent.gb.GetCartridge()
	if cartridge == nil {
		f.parent.gb.Mu.RUnlock()
		content := "# no cartridge loaded\n"
		if f.readPos >= len(content) {
			return 0, io.EOF
		}
		n := copy(buf, content[f.readPos:])
		f.readPos += n
		if f.readPos >= len(content) {
			return n, io.EOF
		}
		return n, nil
	}

	// Type-switch on MBC type to get appropriate state
	var content string
	switch controller := cartridge.BankingController.(type) {
	case *cart.ROM:
		// ROM-only has no banking state
		content = "# ROM-only cartridge (no banking state)\n"

	case *cart.MBC1:
		romBank, ramBank, ramEnabled, romBanking := controller.GetBankingState()
		ramEnabledByte := byte(0x00)
		if ramEnabled {
			ramEnabledByte = 0x01
		}
		romBankingByte := byte(0x00)
		if romBanking {
			romBankingByte = 0x01
		}
		content = fmt.Sprintf(`# Banking
romBank=0x%02x
ramBank=0x%02x
ramEnabled=0x%02x
romBanking=0x%02x
`, romBank, ramBank, ramEnabledByte, romBankingByte)

	case *cart.MBC2:
		romBank, _, ramEnabled := controller.GetBankingState()
		ramEnabledByte := byte(0x00)
		if ramEnabled {
			ramEnabledByte = 0x01
		}
		content = fmt.Sprintf(`# Banking
romBank=0x%02x
ramEnabled=0x%02x
`, romBank, ramEnabledByte)

	case *cart.MBC3:
		romBank, ramBank, ramEnabled := controller.GetBankingState()
		ramEnabledByte := byte(0x00)
		if ramEnabled {
			ramEnabledByte = 0x01
		}
		content = fmt.Sprintf(`# Banking
romBank=0x%02x
ramBank=0x%02x
ramEnabled=0x%02x
`, romBank, ramBank, ramEnabledByte)

		// Add RTC state if applicable
		rtc, latchedRtc, latched := controller.GetRTCState()
		latchedByte := byte(0x00)
		if latched {
			latchedByte = 0x01
		}
		content += fmt.Sprintf(`
# RTC
rtcSeconds=0x%02x
rtcMinutes=0x%02x
rtcHours=0x%02x
rtcDaysLow=0x%02x
rtcDaysHigh=0x%02x
latchedSeconds=0x%02x
latchedMinutes=0x%02x
latchedHours=0x%02x
latchedDaysLow=0x%02x
latchedDaysHigh=0x%02x
rtcLatched=0x%02x
`, rtc[0x08], rtc[0x09], rtc[0x0A], rtc[0x0B], rtc[0x0C],
			latchedRtc[0x08], latchedRtc[0x09], latchedRtc[0x0A], latchedRtc[0x0B], latchedRtc[0x0C],
			latchedByte)

	case *cart.MBC5:
		romBank, ramBank, ramEnabled := controller.GetBankingState()
		ramEnabledByte := byte(0x00)
		if ramEnabled {
			ramEnabledByte = 0x01
		}
		content = fmt.Sprintf(`# Banking
romBank=0x%02x
ramBank=0x%02x
ramEnabled=0x%02x
`, romBank, ramBank, ramEnabledByte)

	default:
		content = fmt.Sprintf("# unknown cartridge type: %T\n", controller)
	}

	f.parent.gb.Mu.RUnlock()

	// Handle offset reads
	if f.readPos >= len(content) {
		return 0, io.EOF
	}

	n := copy(buf, content[f.readPos:])
	f.readPos += n

	if f.readPos >= len(content) {
		return n, io.EOF
	}
	return n, nil
}

func (f *cartridgeStateFile) Write(data []byte) (int, error) {
	content := string(data)
	state := make(map[string]byte)

	// Parse key=value pairs
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and blank lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("line %d: invalid format (expected key=value)", lineNum+1)
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Parse value (hex or decimal)
		var value uint64
		var err error
		if strings.HasPrefix(valueStr, "0x") || strings.HasPrefix(valueStr, "0X") {
			value, err = strconv.ParseUint(valueStr[2:], 16, 32)
		} else {
			value, err = strconv.ParseUint(valueStr, 10, 32)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value %q: %v", lineNum+1, valueStr, err)
		}

		// Validate based on field name (MBC-agnostic validation)
		switch key {
		case "romBank":
			// ROM bank validation will be MBC-specific in handler
			state[key] = byte(value)

		case "ramBank":
			// RAM bank validation will be MBC-specific in handler
			state[key] = byte(value)

		case "ramEnabled":
			if value > 1 {
				return 0, fmt.Errorf("ramEnabled must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		case "romBanking":
			if value > 1 {
				return 0, fmt.Errorf("romBanking must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		case "rtcSeconds", "rtcMinutes", "rtcHours", "rtcDaysLow", "rtcDaysHigh",
			"latchedSeconds", "latchedMinutes", "latchedHours", "latchedDaysLow", "latchedDaysHigh":
			// RTC fields (MBC3 only)
			state[key] = byte(value)

		case "rtcLatched":
			if value > 1 {
				return 0, fmt.Errorf("rtcLatched must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		default:
			return 0, fmt.Errorf("unknown field: %s", key)
		}
	}

	// Queue command (only if we parsed at least one field)
	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:  "cartridge-state-write",
			State: state,
		}
	}

	return len(data), nil
}

func (f *cartridgeStateFile) Close() error {
	return nil
}

func (f *cartridgeStateFile) Stat() (fs.FileInfo, error) {
	return &cartridgeStateFileInfo{}, nil
}

type cartridgeStateFileInfo struct{}

func (fi *cartridgeStateFileInfo) Name() string       { return "state" }
func (fi *cartridgeStateFileInfo) Size() int64        { return 0 } // Dynamic size
func (fi *cartridgeStateFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *cartridgeStateFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *cartridgeStateFileInfo) IsDir() bool        { return false }
func (fi *cartridgeStateFileInfo) Sys() interface{}   { return nil }
