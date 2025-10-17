// ABOUTME: p9.File wrapper for VRAM filesystem interface.
// ABOUTME: Bridges vramFS implementation to 9P protocol.

package ninep

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

// p9VramFile wraps vramFS to provide p9.File interface
type p9VramFile struct {
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
	vramfs  *vramFS
	file    fs.File
	opened  bool
}

func newP9VramFile(gameboy *gb.Gameboy, qid p9.QID) *p9VramFile {
	return &p9VramFile{
		qid:     qid,
		gameboy: gameboy,
		vramfs:  newVramFS(gameboy),
	}
}

func (f *p9VramFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9VramFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	var err error
	f.file, err = f.vramfs.Open(".")
	if err != nil {
		return p9.QID{}, 0, syscall.EIO
	}
	f.opened = true
	return f.qid, 4096, nil
}

func (f *p9VramFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0666,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		NLink:     1,
		Size:      0x4000,
		BlockSize: 4096,
	}, nil
}

func (f *p9VramFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		// Try to open
		var err error
		f.file, err = f.vramfs.Open(".")
		if err != nil {
			return 0, syscall.EINVAL
		}
		f.opened = true
	}

	reader, ok := f.file.(interface{ ReadAt([]byte, int64) (int, error) })
	if !ok {
		return 0, syscall.EINVAL
	}

	return reader.ReadAt(p, offset)
}

func (f *p9VramFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		// Try to open
		var err error
		f.file, err = f.vramfs.Open(".")
		if err != nil {
			return 0, syscall.EINVAL
		}
		f.opened = true
	}

	writer, ok := f.file.(interface{ WriteAt([]byte, int64) (int, error) })
	if !ok {
		return 0, syscall.EPERM
	}

	return writer.WriteAt(p, offset)
}

func (f *p9VramFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	// Allow truncate to size 0 (common before write)
	if valid.Size && attr.Size == 0 {
		return nil
	}
	return syscall.EPERM
}

func (f *p9VramFile) FSync() error {
	// Virtual file, no backing storage to sync
	return nil
}

func (f *p9VramFile) Remove() error {
	return syscall.EPERM
}

func (f *p9VramFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9VramFile) Close() error {
	if f.opened && f.file != nil {
		return f.file.Close()
	}
	return nil
}
