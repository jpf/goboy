// ABOUTME: Text-based filesystem for PPU internal state (scanlineCounter, screenCleared, cgbMode).
// ABOUTME: Follows 9p-spec.md format with key=value pairs and validation.

package ninep

import (
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

type ppuStateFS struct {
	gb *gb.Gameboy
}

func newPPUStateFS(gameboy *gb.Gameboy) *ppuStateFS {
	return &ppuStateFS{gb: gameboy}
}

func (fsys *ppuStateFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &ppuStateFile{parent: fsys}, nil
}

type ppuStateFile struct {
	parent  *ppuStateFS
	readPos int
}

func (f *ppuStateFile) Read(buf []byte) (int, error) {
	// Generate text representation of current state
	f.parent.gb.Mu.RLock()
	scanlineCounter, screenCleared, cgbMode := f.parent.gb.GetPPUState()
	f.parent.gb.Mu.RUnlock()

	screenClearedByte := byte(0x00)
	if screenCleared {
		screenClearedByte = 0x01
	}

	cgbModeByte := byte(0x00)
	if cgbMode {
		cgbModeByte = 0x01
	}

	content := fmt.Sprintf(`# PPU State
scanlineCounter=0x%03x
screenCleared=0x%02x
cgbMode=0x%02x
`, scanlineCounter, screenClearedByte, cgbModeByte)

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

func (f *ppuStateFile) Write(data []byte) (int, error) {
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
			value, err = strconv.ParseUint(valueStr[2:], 16, 16) // 16-bit for scanlineCounter
		} else {
			value, err = strconv.ParseUint(valueStr, 10, 16)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value %q: %v", lineNum+1, valueStr, err)
		}

		// Validate and store
		switch key {
		case "scanlineCounter":
			if value > 456 {
				return 0, fmt.Errorf("scanlineCounter must be 0-456, got %d", value)
			}
			// Store as two bytes (low and high)
			state["scanlineCounter"] = byte(value & 0xFF)
			state["scanlineCounter_high"] = byte((value >> 8) & 0xFF)

		case "screenCleared":
			if value > 1 {
				return 0, fmt.Errorf("screenCleared must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		case "cgbMode":
			if value > 1 {
				return 0, fmt.Errorf("cgbMode must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		default:
			return 0, fmt.Errorf("unknown field: %s", key)
		}
	}

	// Queue command (only if we parsed at least one field)
	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:  "ppu-state-write",
			State: state,
		}
	}

	return len(data), nil
}

func (f *ppuStateFile) Close() error {
	return nil
}

func (f *ppuStateFile) Stat() (fs.FileInfo, error) {
	return &ppuStateFileInfo{}, nil
}

type ppuStateFileInfo struct{}

func (fi *ppuStateFileInfo) Name() string       { return "state" }
func (fi *ppuStateFileInfo) Size() int64        { return 0 } // Dynamic size
func (fi *ppuStateFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *ppuStateFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *ppuStateFileInfo) IsDir() bool        { return false }
func (fi *ppuStateFileInfo) Sys() interface{}   { return nil }
