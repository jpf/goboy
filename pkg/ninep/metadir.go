// ABOUTME: Meta directory implementation for /meta tree navigation
// ABOUTME: Provides p9.File wrappers for derived views and visualizations
package ninep

import (
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
)

// metaDir is the /meta directory containing derived visualizations
type metaDir struct {
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

func (d *metaDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	logf("metaDir.Open(mode=%v)", mode)
	if mode.Mode() == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	logf("metaDir.Open() failed: write not permitted")
	return p9.QID{}, 0, syscall.EPERM
}

func (d *metaDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	logf("metaDir.Walk(names=%v)", names)
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		logf("metaDir.Walk() failed: multi-component walk not supported")
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "vram.png":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		logf("metaDir.Walk() -> vram.png file")
		return []p9.QID{qid}, newVRAMPNGFile(d.attacher.gameboy, qid), nil
	default:
		logf("metaDir.Walk() failed: '%s' not found", names[0])
		return nil, nil, syscall.ENOENT
	}
}

func (d *metaDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *metaDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"vram.png", p9.TypeRegular},
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
