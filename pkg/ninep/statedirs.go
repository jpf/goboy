// ABOUTME: State directory implementations for /state tree navigation.
// ABOUTME: Provides p9.File wrappers for state/memory directory hierarchy.

package ninep

import (
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
)

// stateDir is the /state directory
type stateDir struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotSymlinkFile
	templatefs.ReadOnlyDir
	templatefs.IsDir
	templatefs.NilCloser
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	attacher *p9Attacher
	qid      p9.QID
}

func (d *stateDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

func (d *stateDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "memory":
		qid := d.attacher.qids.Get(p9.TypeDir)
		return []p9.QID{qid}, &memoryDir{attacher: d.attacher, qid: qid}, nil
	default:
		return nil, nil, syscall.ENOENT
	}
}

func (d *stateDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *stateDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"memory", p9.TypeDir},
	}

	if offset >= uint64(len(files)) {
		return nil, nil
	}

	var dirents []p9.Dirent
	end := int(offset) + int(count)
	if end > len(files) {
		end = len(files)
	}

	for i, file := range files[offset:end] {
		dirents = append(dirents, p9.Dirent{
			QID:    d.attacher.qids.Get(file.typ),
			Type:   file.typ,
			Offset: offset + uint64(i) + 1,
			Name:   file.name,
		})
	}
	return dirents, nil
}

// memoryDir is the /state/memory directory
type memoryDir struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotSymlinkFile
	templatefs.ReadOnlyDir
	templatefs.IsDir
	templatefs.NilCloser
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	attacher *p9Attacher
	qid      p9.QID
}

func (d *memoryDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

func (d *memoryDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "vram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9VramFile(d.attacher.gameboy, qid), nil
	default:
		return nil, nil, syscall.ENOENT
	}
}

func (d *memoryDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *memoryDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"vram", p9.TypeRegular},
	}

	if offset >= uint64(len(files)) {
		return nil, nil
	}

	var dirents []p9.Dirent
	end := int(offset) + int(count)
	if end > len(files) {
		end = len(files)
	}

	for i, file := range files[offset:end] {
		dirents = append(dirents, p9.Dirent{
			QID:    d.attacher.qids.Get(file.typ),
			Type:   file.typ,
			Offset: offset + uint64(i) + 1,
			Name:   file.name,
		})
	}
	return dirents, nil
}
