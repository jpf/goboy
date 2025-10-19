// ABOUTME: p9.File wrapper for vram.png virtual file (read/write PNG visualization)
// ABOUTME: Provides bidirectional PNG conversion for VRAM tile data editing
package ninep

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"

	"github.com/Humpheh/goboy/pkg/gb"
)

// vrampngFS implements fs.FS for the virtual vram.png file
type vrampngFS struct {
	gameboy *gb.Gameboy
}

func newVRAMPNGFS(gb *gb.Gameboy) *vrampngFS {
	return &vrampngFS{gameboy: gb}
}

func (f *vrampngFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &vrampngFile{gameboy: f.gameboy}, nil
}

// vrampngFile implements fs.File for vram.png operations
type vrampngFile struct {
	gameboy *gb.Gameboy
	data    *bytes.Reader // Cached PNG data for reads
}

func (f *vrampngFile) Stat() (fs.FileInfo, error) {
	// Estimate PNG size (actual size varies with compression)
	// For 16KB VRAM, PNG is roughly 20KB
	estimatedSize := int64(20000)

	return &vrampngFileInfo{
		name: "vram.png",
		size: estimatedSize,
		mode: 0666,
	}, nil
}

type vrampngFileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (fi *vrampngFileInfo) Name() string       { return fi.name }
func (fi *vrampngFileInfo) Size() int64        { return fi.size }
func (fi *vrampngFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *vrampngFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *vrampngFileInfo) IsDir() bool        { return false }
func (fi *vrampngFileInfo) Sys() interface{}   { return nil }

func (f *vrampngFile) Read(p []byte) (int, error) {
	// Generate PNG on first read
	if f.data == nil {
		f.gameboy.Mu.RLock()
		vram := f.gameboy.GetVRAM()
		pngData, err := VRAMToPNG(vram[:])
		f.gameboy.Mu.RUnlock()

		if err != nil {
			return 0, err
		}

		f.data = bytes.NewReader(pngData)
	}

	return f.data.Read(p)
}

func (f *vrampngFile) Write(p []byte) (int, error) {
	// Accumulate all write data
	// Note: 9P may call Write multiple times for a single file write
	var buf bytes.Buffer
	buf.Write(p)

	// For simplicity, convert immediately
	// In production, we might buffer until Close()
	vramData, err := PNGToVRAM(buf.Bytes(), 0x4000)
	if err != nil {
		return 0, err
	}

	// Queue VRAM write command
	f.gameboy.CommandChan <- gb.Command{
		Name: "vram-write",
		Data: vramData,
	}

	return len(p), nil
}

func (f *vrampngFile) Close() error {
	return nil
}

// p9VRAMPNGFile wraps vrampngFS for 9P protocol
type p9VRAMPNGFile struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NilCloser
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	qid     p9.QID
	gameboy *gb.Gameboy
	fsys    *vrampngFS
	file    fs.File
	opened  bool

	// Buffer for accumulating write data
	writeBuffer bytes.Buffer
}

func newVRAMPNGFile(gb *gb.Gameboy, qid p9.QID) *p9VRAMPNGFile {
	return &p9VRAMPNGFile{
		qid:     qid,
		gameboy: gb,
		fsys:    newVRAMPNGFS(gb),
	}
}

func (f *p9VRAMPNGFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9VRAMPNGFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if f.opened {
		return p9.QID{}, 0, syscall.EINVAL
	}

	file, err := f.fsys.Open(".")
	if err != nil {
		return p9.QID{}, 0, err
	}

	f.file = file
	f.opened = true
	f.writeBuffer.Reset()
	return f.qid, 8192, nil
}

func (f *p9VRAMPNGFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	// Estimate PNG size for attribute queries
	estimatedSize := uint64(20000)

	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0666,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		Size:      estimatedSize,
		NLink:     1,
		BlockSize: 4096,
	}, nil
}

func (f *p9VRAMPNGFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	// Generate PNG from current VRAM state
	f.gameboy.Mu.RLock()
	vram := f.gameboy.GetVRAM()
	pngData, err := VRAMToPNG(vram[:])
	f.gameboy.Mu.RUnlock()

	if err != nil {
		return 0, syscall.EIO
	}

	// Handle offset-based reads
	if offset >= int64(len(pngData)) {
		return 0, io.EOF
	}

	n := copy(p, pngData[offset:])
	return n, nil
}

func (f *p9VRAMPNGFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	// Accumulate all writes into buffer
	// 9P writes may come in chunks
	if offset == 0 {
		f.writeBuffer.Reset()
	}

	f.writeBuffer.Write(p)
	return len(p), nil
}

func (f *p9VRAMPNGFile) Close() error {
	if !f.opened {
		return nil
	}

	// If we accumulated write data, process it now
	if f.writeBuffer.Len() > 0 {
		vramData, err := PNGToVRAM(f.writeBuffer.Bytes(), 0x4000)
		if err != nil {
			// Log error but don't fail close
			logf("vram.png write failed: %v", err)
			return err
		}

		// Queue VRAM write command
		f.gameboy.CommandChan <- gb.Command{
			Name: "vram-write",
			Data: vramData,
		}
	}

	f.opened = false
	f.writeBuffer.Reset()
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

func (f *p9VRAMPNGFile) FSync() error {
	return nil
}

func (f *p9VRAMPNGFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9VRAMPNGFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	// Allow truncate to 0 (common with shell redirects)
	if valid.Size && attr.Size == 0 {
		return nil
	}
	return syscall.EPERM
}
