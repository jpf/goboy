// ABOUTME: APU state fs.FS implementation for 9P filesystem interface.
// ABOUTME: Provides read/write access to audio processing unit state (77 bytes binary format).

package ninep

import (
	"encoding/binary"
	"io"
	"io/fs"
	"math"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

const apuStateSize = 77 // 1 + 52 + 8 + 8 + 8

// apuStateFS implements fs.FS for APU state access
type apuStateFS struct {
	mu sync.RWMutex
	gb *gb.Gameboy
}

func newAPUStateFS(gameboy *gb.Gameboy) *apuStateFS {
	return &apuStateFS{gb: gameboy}
}

func (a *apuStateFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &apuStateFile{parent: a}, nil
}

// apuStateFile provides read/write access to APU state.
//
// Binary format (77 bytes):
//   - Byte 0: playing flag (0 or 1)
//   - Bytes 1-52: memory array (APU registers 0xFF10-0xFF43)
//   - Bytes 53-60: lVol (float64, little-endian)
//   - Bytes 61-68: rVol (float64, little-endian)
//   - Bytes 69-76: tickCounter (float64, little-endian)
//
// Thread safety: Reads acquire Gameboy.Mu.RLock(). Writes are queued
// via command channel and applied at frame boundaries.
type apuStateFile struct {
	parent  *apuStateFS
	readPos int
}

// serializeAPUState converts APU state to binary format
func serializeAPUState(playing byte, memory [52]byte, lVol, rVol, tickCounter float64) []byte {
	data := make([]byte, apuStateSize)

	data[0] = playing
	copy(data[1:53], memory[:])
	binary.LittleEndian.PutUint64(data[53:61], math.Float64bits(lVol))
	binary.LittleEndian.PutUint64(data[61:69], math.Float64bits(rVol))
	binary.LittleEndian.PutUint64(data[69:77], math.Float64bits(tickCounter))

	return data
}

func (f *apuStateFile) Read(p []byte) (n int, err error) {
	// Lock emulator state for consistent read
	f.parent.gb.Mu.RLock()
	defer f.parent.gb.Mu.RUnlock()

	if f.readPos >= apuStateSize {
		return 0, io.EOF
	}

	// Get current APU state
	playing, memory, lVol, rVol, tickCounter := f.parent.gb.GetAPUState()
	data := serializeAPUState(playing, memory, lVol, rVol, tickCounter)

	n = copy(p, data[f.readPos:])
	f.readPos += n
	return n, nil
}

func (f *apuStateFile) Write(p []byte) (n int, err error) {
	// Queue write command for frame boundary processing
	cmd := gb.Command{
		Name:   "apu-state-write",
		Offset: f.readPos,
		Data:   make([]byte, len(p)),
	}
	copy(cmd.Data, p)

	f.parent.gb.GetCommandChan() <- cmd
	f.readPos += len(p)
	return len(p), nil
}

func (f *apuStateFile) Close() error {
	return nil
}

func (f *apuStateFile) Stat() (fs.FileInfo, error) {
	return &apuStateFileInfo{}, nil
}

// ReadAt implements offset-based reads for surgical state inspection
func (f *apuStateFile) ReadAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 || offset > apuStateSize {
		return 0, &fs.PathError{Op: "read", Path: "state", Err: fs.ErrInvalid}
	}

	f.parent.gb.Mu.RLock()
	defer f.parent.gb.Mu.RUnlock()

	// Get current APU state
	playing, memory, lVol, rVol, tickCounter := f.parent.gb.GetAPUState()
	data := serializeAPUState(playing, memory, lVol, rVol, tickCounter)

	n = copy(p, data[offset:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt implements offset-based writes for surgical state modification
func (f *apuStateFile) WriteAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 || offset > apuStateSize {
		return 0, &fs.PathError{Op: "write", Path: "state", Err: fs.ErrInvalid}
	}

	cmd := gb.Command{
		Name:   "apu-state-write",
		Offset: int(offset),
		Data:   make([]byte, len(p)),
	}
	copy(cmd.Data, p)

	f.parent.gb.GetCommandChan() <- cmd
	return len(p), nil
}

type apuStateFileInfo struct{}

func (fi *apuStateFileInfo) Name() string       { return "state" }
func (fi *apuStateFileInfo) Size() int64        { return apuStateSize }
func (fi *apuStateFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *apuStateFileInfo) ModTime() time.Time { return time.Now() }
func (fi *apuStateFileInfo) IsDir() bool        { return false }
func (fi *apuStateFileInfo) Sys() interface{}   { return nil }
