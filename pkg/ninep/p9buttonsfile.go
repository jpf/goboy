// ABOUTME: p9.File wrapper for buttons file (read/write button state).
// ABOUTME: Delegates to buttonsFS for all operations.

package ninep

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"

	"github.com/Humpheh/goboy/pkg/gb"
)

type p9ButtonsFile struct {
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
	fsys    *buttonsFS
	file    fs.File
	opened  bool
}

func newButtonsFile(gb *gb.Gameboy, qid p9.QID) *p9ButtonsFile {
	return &p9ButtonsFile{
		qid:     qid,
		gameboy: gb,
		fsys:    newButtonsFS(gb),
	}
}

func (f *p9ButtonsFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9ButtonsFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if f.opened {
		return p9.QID{}, 0, syscall.EINVAL
	}

	file, err := f.fsys.Open(".")
	if err != nil {
		return p9.QID{}, 0, err
	}

	f.file = file
	f.opened = true
	return f.qid, 8192, nil
}

func (f *p9ButtonsFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0666,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		Size:      0, // Dynamic size based on pressed buttons
		NLink:     1,
		BlockSize: 4096,
	}, nil
}

func (f *p9ButtonsFile) ReadAt(p []byte, offset int64) (int, error) {
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

func (f *p9ButtonsFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	writer, ok := f.file.(interface {
		Write([]byte) (int, error)
	})
	if !ok {
		return 0, syscall.ENOSYS
	}

	return writer.Write(p)
}

func (f *p9ButtonsFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	// Allow truncate to 0 (common with shell redirects)
	if valid.Size && attr.Size == 0 {
		return nil
	}
	return syscall.EPERM
}

func (f *p9ButtonsFile) FSync() error {
	return nil
}

func (f *p9ButtonsFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9ButtonsFile) Close() error {
	if f.opened && f.file != nil {
		return f.file.Close()
	}
	return nil
}
