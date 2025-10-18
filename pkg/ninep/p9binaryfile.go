// ABOUTME: Generic p9.File wrapper for binary memory regions.
// ABOUTME: Delegates to binaryMemoryFS for all operations.

package ninep

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"

	"github.com/Humpheh/goboy/pkg/gb"
)

type p9BinaryFile struct {
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
	fsys    *binaryMemoryFS
	file    fs.File
	opened  bool
}

func newP9BinaryFile(gb *gb.Gameboy, qid p9.QID, fsys *binaryMemoryFS) *p9BinaryFile {
	return &p9BinaryFile{
		qid:     qid,
		gameboy: gb,
		fsys:    fsys,
	}
}

func (f *p9BinaryFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9BinaryFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
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

func (f *p9BinaryFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	// Get actual size (handles dynamic size -1 and nil memory)
	size := f.fsys.size
	if size == -1 {
		f.gameboy.Mu.RLock()
		mem := f.fsys.getMemory(f.gameboy)
		if mem != nil {
			size = int64(len(mem))
		} else {
			size = 0
		}
		f.gameboy.Mu.RUnlock()
	}

	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0666,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		Size:      uint64(size),
		NLink:     1,
		BlockSize: 4096,
	}, nil
}

func (f *p9BinaryFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	reader, ok := f.file.(interface {
		ReadAt([]byte, int64) (int, error)
	})
	if !ok {
		return 0, syscall.ENOSYS
	}

	return reader.ReadAt(p, offset)
}

func (f *p9BinaryFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	writer, ok := f.file.(interface {
		WriteAt([]byte, int64) (int, error)
	})
	if !ok {
		return 0, syscall.ENOSYS
	}

	return writer.WriteAt(p, offset)
}

func (f *p9BinaryFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	// Allow truncate to 0 (common with shell redirects)
	if valid.Size && attr.Size == 0 {
		return nil
	}
	return syscall.EPERM
}

func (f *p9BinaryFile) FSync() error {
	return nil
}

func (f *p9BinaryFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9BinaryFile) Close() error {
	if f.opened && f.file != nil {
		return f.file.Close()
	}
	return nil
}
