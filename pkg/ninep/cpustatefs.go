// ABOUTME: Text-based filesystem for CPU state (registers, PC, SP, Divider).
// ABOUTME: Exposes CPU registers and program counter for save/restore operations.

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

type cpuStateFS struct {
	gb *gb.Gameboy
}

func newCPUStateFS(gameboy *gb.Gameboy) *cpuStateFS {
	return &cpuStateFS{gb: gameboy}
}

func (fsys *cpuStateFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &cpuStateFile{parent: fsys}, nil
}

type cpuStateFile struct {
	parent  *cpuStateFS
	readPos int
}

func (f *cpuStateFile) Read(buf []byte) (int, error) {
	// Generate text representation of current CPU state
	f.parent.gb.Mu.RLock()
	af, bc, de, hl, sp, pc, divider := f.parent.gb.GetCPUState()
	f.parent.gb.Mu.RUnlock()

	content := fmt.Sprintf(`# Registers
AF=0x%04X
BC=0x%04X
DE=0x%04X
HL=0x%04X

# Program State
SP=0x%04X
PC=0x%04X

# Timer
Divider=0x%04X
`, af, bc, de, hl, sp, pc, divider)

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

func (f *cpuStateFile) Write(data []byte) (int, error) {
	content := string(data)
	state := make(map[string]uint16)

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
			value, err = strconv.ParseUint(valueStr[2:], 16, 16)
		} else {
			value, err = strconv.ParseUint(valueStr, 10, 16)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value %q: %v", lineNum+1, valueStr, err)
		}

		// No validation per spec philosophy - trust user
		switch key {
		case "AF", "BC", "DE", "HL", "SP", "PC", "Divider":
			state[key] = uint16(value)
		default:
			return 0, fmt.Errorf("line %d: unknown field %q", lineNum+1, key)
		}
	}

	// Queue command (only if we parsed at least one field)
	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:     "cpu-write",
			CPUState: state,
		}
	}

	return len(data), nil
}

func (f *cpuStateFile) Close() error {
	return nil
}

func (f *cpuStateFile) Stat() (fs.FileInfo, error) {
	return &cpuStateFileInfo{}, nil
}

type cpuStateFileInfo struct{}

func (fi *cpuStateFileInfo) Name() string       { return "cpu" }
func (fi *cpuStateFileInfo) Size() int64        { return 0 } // Dynamic size
func (fi *cpuStateFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *cpuStateFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *cpuStateFileInfo) IsDir() bool        { return false }
func (fi *cpuStateFileInfo) Sys() interface{}   { return nil }
