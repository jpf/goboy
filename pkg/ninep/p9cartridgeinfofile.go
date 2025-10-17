// ABOUTME: p9.File wrapper for cartridge info filesystem interface.
// ABOUTME: Bridges cartridgeInfoFS implementation to 9P protocol.

package ninep

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

// p9CartridgeInfoFile wraps cartridgeInfoFS to provide p9.File interface
type p9CartridgeInfoFile struct {
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
	infoFS  *cartridgeInfoFS
	file    fs.File
	opened  bool
}

func newP9CartridgeInfoFile(gameboy *gb.Gameboy, qid p9.QID) *p9CartridgeInfoFile {
	return &p9CartridgeInfoFile{
		qid:     qid,
		gameboy: gameboy,
		infoFS:  newCartridgeInfoFS(gameboy),
	}
}

func (f *p9CartridgeInfoFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9CartridgeInfoFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	var err error
	f.file, err = f.infoFS.Open(".")
	if err != nil {
		return p9.QID{}, 0, syscall.EIO
	}
	f.opened = true
	return f.qid, 4096, nil
}

func (f *p9CartridgeInfoFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0444, // Read-only
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		NLink:     1,
		Size:      0, // Dynamic size
		BlockSize: 4096,
	}, nil
}

func (f *p9CartridgeInfoFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	reader, ok := f.file.(interface {
		Read([]byte) (int, error)
	})
	if !ok {
		return 0, syscall.ENOSYS
	}

	return reader.Read(p)
}

func (f *p9CartridgeInfoFile) WriteAt(p []byte, offset int64) (int, error) {
	return 0, syscall.EPERM // Read-only file
}

func (f *p9CartridgeInfoFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM // Read-only file
}

func (f *p9CartridgeInfoFile) FSync() error {
	// Virtual file, no backing storage to sync
	return nil
}

func (f *p9CartridgeInfoFile) Remove() error {
	return syscall.EPERM
}

func (f *p9CartridgeInfoFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9CartridgeInfoFile) Close() error {
	if f.opened && f.file != nil {
		return f.file.Close()
	}
	return nil
}
