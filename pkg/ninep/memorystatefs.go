// ABOUTME: Text-based filesystem for memory banking state (VRAMBank, WRAMBank, HDMA).
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

type memoryStateFS struct {
	gb *gb.Gameboy
}

func newMemoryStateFS(gameboy *gb.Gameboy) *memoryStateFS {
	return &memoryStateFS{gb: gameboy}
}

func (fsys *memoryStateFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &memoryStateFile{parent: fsys}, nil
}

type memoryStateFile struct {
	parent  *memoryStateFS
	readPos int
}

func (f *memoryStateFile) Read(buf []byte) (int, error) {
	// Generate text representation of current state
	f.parent.gb.Mu.RLock()
	vram, wram, hdma, active := f.parent.gb.GetMemoryState()
	f.parent.gb.Mu.RUnlock()

	activeByte := byte(0x00)
	if active {
		activeByte = 0x01
	}

	content := fmt.Sprintf(`# Banking
VRAMBank=0x%02x
WRAMBank=0x%02x

# DMA
hdmaLength=0x%02x
hdmaActive=0x%02x
`, vram, wram, hdma, activeByte)

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

func (f *memoryStateFile) Write(data []byte) (int, error) {
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
			value, err = strconv.ParseUint(valueStr[2:], 16, 8)
		} else {
			value, err = strconv.ParseUint(valueStr, 10, 8)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value %q: %v", lineNum+1, valueStr, err)
		}

		// Validate and store
		switch key {
		case "VRAMBank":
			if value > 1 {
				return 0, fmt.Errorf("VRAMBank must be 0-1, got %d", value)
			}
			state[key] = byte(value)

		case "WRAMBank":
			if value > 7 {
				return 0, fmt.Errorf("WRAMBank must be 0-7, got %d", value)
			}
			state[key] = byte(value)

		case "hdmaLength":
			state[key] = byte(value)

		case "hdmaActive":
			if value > 1 {
				return 0, fmt.Errorf("hdmaActive must be 0 or 1, got %d", value)
			}
			state[key] = byte(value)

		default:
			return 0, fmt.Errorf("unknown field: %s", key)
		}
	}

	// Queue command (only if we parsed at least one field)
	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:  "memorystate-write",
			State: state,
		}
	}

	return len(data), nil
}

func (f *memoryStateFile) Close() error {
	return nil
}

func (f *memoryStateFile) Stat() (fs.FileInfo, error) {
	return &memoryStateFileInfo{}, nil
}

type memoryStateFileInfo struct{}

func (fi *memoryStateFileInfo) Name() string       { return "state" }
func (fi *memoryStateFileInfo) Size() int64        { return 0 } // Dynamic size
func (fi *memoryStateFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *memoryStateFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memoryStateFileInfo) IsDir() bool        { return false }
func (fi *memoryStateFileInfo) Sys() interface{}   { return nil }
