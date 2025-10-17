// ABOUTME: Generic fs.FS implementation for binary memory regions.
// ABOUTME: Provides read/write interface to any fixed-size byte array with validation.

package ninep

import (
	"errors"
	"io"
	"io/fs"
	"math"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

type binaryMemoryFS struct {
	gb          *gb.Gameboy
	getMemory   func(*gb.Gameboy) []byte // Returns slice of memory region
	size        int64
	commandName string
}

type binaryMemoryFile struct {
	parent  *binaryMemoryFS
	readPos int64
}

func (fsys *binaryMemoryFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &binaryMemoryFile{parent: fsys, readPos: 0}, nil
}

func (f *binaryMemoryFile) Stat() (fs.FileInfo, error) {
	size := f.parent.size

	// For dynamic size (-1), get actual memory size
	if size == -1 {
		f.parent.gb.Mu.RLock()
		mem := f.parent.getMemory(f.parent.gb)
		if mem != nil {
			size = int64(len(mem))
		} else {
			size = 0
		}
		f.parent.gb.Mu.RUnlock()
	}

	return &binaryMemoryFileInfo{
		name: "memory",
		size: size,
	}, nil
}

func (f *binaryMemoryFile) Read(buf []byte) (int, error) {
	n, err := f.ReadAt(buf, f.readPos)
	f.readPos += int64(n)
	return n, err
}

func (f *binaryMemoryFile) ReadAt(buf []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, errors.New("negative offset")
	}

	// Get memory region (may be nil for cartridges without RAM)
	f.parent.gb.Mu.RLock()
	mem := f.parent.getMemory(f.parent.gb)

	// Handle nil memory (e.g., ROM-only cartridge with no RAM)
	if mem == nil {
		f.parent.gb.Mu.RUnlock()
		return 0, io.EOF
	}

	// Check bounds using actual memory size if size is dynamic (-1)
	size := f.parent.size
	if size == -1 {
		size = int64(len(mem))
	}

	if offset >= size {
		f.parent.gb.Mu.RUnlock()
		return 0, io.EOF
	}

	n := copy(buf, mem[offset:])
	f.parent.gb.Mu.RUnlock()

	if n == 0 && len(buf) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *binaryMemoryFile) Write(data []byte) (int, error) {
	n, err := f.WriteAt(data, f.readPos)
	f.readPos += int64(n)
	return n, err
}

func (f *binaryMemoryFile) WriteAt(data []byte, offset int64) (int, error) {
	// Validate offset fits in int (for Go slice indexing)
	if offset > math.MaxInt || offset < 0 {
		return 0, errors.New("offset out of range")
	}

	// Check if memory exists (e.g., ROM-only cartridge has no RAM)
	f.parent.gb.Mu.RLock()
	mem := f.parent.getMemory(f.parent.gb)
	if mem == nil {
		f.parent.gb.Mu.RUnlock()
		return 0, errors.New("no RAM available (ROM-only cartridge)")
	}

	// Check bounds using actual memory size if size is dynamic (-1)
	size := f.parent.size
	if size == -1 {
		size = int64(len(mem))
	}
	f.parent.gb.Mu.RUnlock()

	if offset >= size {
		return 0, errors.New("offset beyond file size")
	}
	if offset+int64(len(data)) > size {
		return 0, errors.New("write beyond file size")
	}

	// Queue write command (data copied to prevent caller modification)
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	f.parent.gb.CommandChan <- gb.Command{
		Name:   f.parent.commandName,
		Offset: int(offset),
		Data:   dataCopy,
	}

	return len(data), nil
}

func (f *binaryMemoryFile) Close() error {
	return nil
}

type binaryMemoryFileInfo struct {
	name string
	size int64
}

func (fi *binaryMemoryFileInfo) Name() string       { return fi.name }
func (fi *binaryMemoryFileInfo) Size() int64        { return fi.size }
func (fi *binaryMemoryFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *binaryMemoryFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *binaryMemoryFileInfo) IsDir() bool        { return false }
func (fi *binaryMemoryFileInfo) Sys() interface{}   { return nil }
