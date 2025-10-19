// ABOUTME: ROM filesystem for loading new ROM files via 9P interface.
// ABOUTME: Buffers ROM writes, validates on close, then sends rom-load command.

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

// romFS implements fs.FS for ROM file writes
type romFS struct {
	mu sync.RWMutex
	gb *gb.Gameboy
}

func newROMFS(gameboy *gb.Gameboy) *romFS {
	return &romFS{gb: gameboy}
}

func (r *romFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &romFile{
		parent: r,
		buffer: &bytes.Buffer{},
	}, nil
}

// romFile buffers ROM writes and validates on Close()
type romFile struct {
	parent *romFS
	buffer *bytes.Buffer
}

func (f *romFile) Read(p []byte) (n int, err error) {
	// ROM file is write-only
	return 0, fmt.Errorf("ROM file is write-only")
}

func (f *romFile) Write(p []byte) (n int, err error) {
	return f.buffer.Write(p)
}

func (f *romFile) Close() error {
	// Empty write is OK - no-op
	if f.buffer.Len() == 0 {
		return nil
	}

	romData := f.buffer.Bytes()

	// Validate ROM
	if err := validateROM(romData); err != nil {
		return err
	}

	// Make a copy for the command (buffer will be garbage collected)
	dataCopy := make([]byte, len(romData))
	copy(dataCopy, romData)

	// Queue rom-load command
	f.parent.gb.GetCommandChan() <- gb.Command{
		Name: "rom-load",
		Data: dataCopy,
	}

	return nil
}

func (f *romFile) Stat() (fs.FileInfo, error) {
	return &romFileInfo{
		name: "rom",
		size: 0, // Write-only file reports size 0
		mode: 0222,
	}, nil
}

type romFileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (fi *romFileInfo) Name() string       { return fi.name }
func (fi *romFileInfo) Size() int64        { return fi.size }
func (fi *romFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *romFileInfo) ModTime() time.Time { return time.Now() }
func (fi *romFileInfo) IsDir() bool        { return false }
func (fi *romFileInfo) Sys() interface{}   { return nil }

// validateROM checks ROM size and header checksum
func validateROM(rom []byte) error {
	// Check size bounds
	const minSize = 32 * 1024      // 32KB
	const maxSize = 8 * 1024 * 1024 // 8MB

	if len(rom) < minSize {
		return fmt.Errorf("ROM too small: %d bytes (minimum 32KB)", len(rom))
	}
	if len(rom) > maxSize {
		return fmt.Errorf("ROM too large: %d bytes (maximum 8MB)", len(rom))
	}

	// Validate header checksum (bytes 0x0134-0x014C)
	checksum := byte(0)
	for addr := 0x0134; addr <= 0x014C; addr++ {
		checksum = checksum - rom[addr] - 1
	}

	if checksum != rom[0x014D] {
		return fmt.Errorf("invalid ROM header checksum: got 0x%02X, expected 0x%02X", rom[0x014D], checksum)
	}

	return nil
}
