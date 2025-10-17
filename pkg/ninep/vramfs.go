// ABOUTME: VRAM fs.FS implementation for 9P filesystem interface.
// ABOUTME: Provides read/write access to emulator VRAM with offset support.

package ninep

import (
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

// vramFS implements fs.FS for VRAM access
type vramFS struct {
	mu sync.RWMutex
	gb *gb.Gameboy
}

func newVramFS(gameboy *gb.Gameboy) *vramFS {
	return &vramFS{gb: gameboy}
}

func (v *vramFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &vramFile{parent: v}, nil
}

// vramFile provides read/write access to VRAM
//
// Thread safety: Reads acquire Gameboy.Mu.RLock() for consistency, but the
// PPU does not lock during rendering. This means reads may see briefly
// inconsistent data (mixing values from different frames) during the
// microseconds while data is being read. For guaranteed consistency, pause
// the emulator before reading.
//
// Writes are queued via the command channel and applied at frame boundaries,
// guaranteeing write safety.
type vramFile struct {
	parent  *vramFS
	readPos int
}

func (f *vramFile) Read(p []byte) (n int, err error) {
	// Lock emulator state for consistent read
	f.parent.gb.Mu.RLock()
	defer f.parent.gb.Mu.RUnlock()

	if f.readPos >= 0x4000 {
		return 0, io.EOF
	}

	vram := f.parent.gb.GetVRAM()
	n = copy(p, vram[f.readPos:])
	f.readPos += n
	return n, nil
}

func (f *vramFile) Write(p []byte) (n int, err error) {
	// Queue write command for frame boundary processing
	cmd := gb.Command{
		Name:   "vram-write",
		Offset: int64(f.readPos),
		Data:   make([]byte, len(p)),
	}
	copy(cmd.Data, p)

	f.parent.gb.GetCommandChan() <- cmd
	f.readPos += len(p)
	return len(p), nil
}

func (f *vramFile) Close() error {
	return nil
}

func (f *vramFile) Stat() (fs.FileInfo, error) {
	return &vramFileInfo{}, nil
}

// ReadAt implements offset-based reads for surgical VRAM inspection
func (f *vramFile) ReadAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 || offset > 0x4000 {
		return 0, &fs.PathError{Op: "read", Path: "vram", Err: fs.ErrInvalid}
	}

	f.parent.gb.Mu.RLock()
	defer f.parent.gb.Mu.RUnlock()

	vram := f.parent.gb.GetVRAM()
	n = copy(p, vram[offset:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt implements offset-based writes for surgical VRAM modification
func (f *vramFile) WriteAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 || offset > 0x4000 {
		return 0, &fs.PathError{Op: "write", Path: "vram", Err: fs.ErrInvalid}
	}

	cmd := gb.Command{
		Name:   "vram-write",
		Offset: offset,
		Data:   make([]byte, len(p)),
	}
	copy(cmd.Data, p)

	f.parent.gb.GetCommandChan() <- cmd
	return len(p), nil
}

type vramFileInfo struct{}

func (fi *vramFileInfo) Name() string       { return "vram" }
func (fi *vramFileInfo) Size() int64        { return 0x4000 }
func (fi *vramFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *vramFileInfo) ModTime() time.Time { return time.Now() }
func (fi *vramFileInfo) IsDir() bool        { return false }
func (fi *vramFileInfo) Sys() interface{}   { return nil }
