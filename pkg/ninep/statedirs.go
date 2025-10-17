// ABOUTME: State directory implementations for /state tree navigation.
// ABOUTME: Provides p9.File wrappers for state/memory directory hierarchy.

package ninep

import (
	"os"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"

	"github.com/Humpheh/goboy/pkg/gb"
)

// stateDir is the /state directory
type stateDir struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotSymlinkFile
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

	// Check if file has been "deleted" (for tar extraction)
	path := "state/" + names[0]
	if _, deleted := d.attacher.unlinked.Load(path); deleted {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "memory":
		qid := d.attacher.qids.Get(p9.TypeDir)
		return []p9.QID{qid}, &memoryDir{attacher: d.attacher, qid: qid}, nil
	case "cpu":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9CPUFile(d.attacher.gameboy, qid), nil
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
		{"cpu", p9.TypeRegular},
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

// UnlinkAt marks files as deleted for tar extraction support.
// Virtual files can't actually be deleted, but we track them to hide from Walk.
func (d *stateDir) UnlinkAt(name string, flags uint32) error {
	// Accept unlink for files that exist
	switch name {
	case "cpu":
		path := "state/" + name
		d.attacher.unlinked.Store(path, true)
		return nil
	default:
		return syscall.ENOENT
	}
}

// Mkdir prevents directory creation
func (d *stateDir) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// Create handles tar extraction by opening existing virtual files
func (d *stateDir) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	// Virtual files always exist, so "create" just opens them
	switch name {
	case "cpu":
		// Remove from unlinked set (file is being "created")
		path := "state/" + name
		d.attacher.unlinked.Delete(path)

		qid := d.attacher.qids.Get(p9.TypeRegular)
		file := newP9CPUFile(d.attacher.gameboy, qid)
		// Open the file for writing
		_, iounit, err := file.Open(flags)
		if err != nil {
			return nil, p9.QID{}, 0, err
		}
		return file, qid, iounit, nil
	default:
		return nil, p9.QID{}, 0, syscall.ENOENT
	}
}

// Link prevents hard link creation
func (d *stateDir) Link(target p9.File, newname string) error {
	return syscall.EPERM
}

// Mknod prevents device node creation
func (d *stateDir) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// RenameAt prevents file renaming
func (d *stateDir) RenameAt(oldname string, newdir p9.File, newname string) error {
	return syscall.EPERM
}

// Symlink prevents symlink creation
func (d *stateDir) Symlink(oldname string, newname string, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// FSync is a no-op for directories
func (d *stateDir) FSync() error {
	return nil
}

// Rename prevents directory renaming
func (d *stateDir) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// SetAttr prevents attribute changes on directories
func (d *stateDir) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM
}

// memoryDir is the /state/memory directory
type memoryDir struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotSymlinkFile
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

	// Check if file has been "deleted" (for tar extraction)
	path := "state/memory/" + names[0]
	if _, deleted := d.attacher.unlinked.Load(path); deleted {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "vram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9VramFile(d.attacher.gameboy, qid), nil

	case "wram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetWRAM()[:] },
			size:        0x9000,
			commandName: "wram-write",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil

	case "oam":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetOAM()[:] },
			size:        0x100,
			commandName: "oam-write",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil

	case "highram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetHighRAM()[:] },
			size:        0x100,
			commandName: "highram-write",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil

	case "state":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9MemoryStateFile(d.attacher.gameboy, qid), nil

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
		{"wram", p9.TypeRegular},
		{"oam", p9.TypeRegular},
		{"highram", p9.TypeRegular},
		{"state", p9.TypeRegular},
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

// UnlinkAt marks files as deleted for tar extraction support.
// Virtual files can't actually be deleted, but we track them to hide from Walk.
func (d *memoryDir) UnlinkAt(name string, flags uint32) error {
	// Accept unlink for files that exist
	switch name {
	case "vram", "wram", "oam", "highram", "state":
		path := "state/memory/" + name
		d.attacher.unlinked.Store(path, true)
		return nil
	default:
		return syscall.ENOENT
	}
}

// Mkdir prevents directory creation
func (d *memoryDir) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// Create handles tar extraction by opening existing virtual files
func (d *memoryDir) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	// Remove from unlinked set (file is being "created")
	path := "state/memory/" + name
	d.attacher.unlinked.Delete(path)

	// Virtual files always exist, so "create" just opens them
	var file p9.File
	qid := d.attacher.qids.Get(p9.TypeRegular)

	switch name {
	case "vram":
		file = newP9VramFile(d.attacher.gameboy, qid)
	case "wram":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetWRAM()[:] },
			size:        0x9000,
			commandName: "wram-write",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "oam":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetOAM()[:] },
			size:        0x100,
			commandName: "oam-write",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "highram":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetHighRAM()[:] },
			size:        0x100,
			commandName: "highram-write",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "state":
		file = newP9MemoryStateFile(d.attacher.gameboy, qid)
	default:
		return nil, p9.QID{}, 0, syscall.ENOENT
	}

	// Open the file for writing
	_, iounit, err := file.Open(flags)
	if err != nil {
		return nil, p9.QID{}, 0, err
	}
	return file, qid, iounit, nil
}

// Link prevents hard link creation
func (d *memoryDir) Link(target p9.File, newname string) error {
	return syscall.EPERM
}

// Mknod prevents device node creation
func (d *memoryDir) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// RenameAt prevents file renaming
func (d *memoryDir) RenameAt(oldname string, newdir p9.File, newname string) error {
	return syscall.EPERM
}

// Symlink prevents symlink creation
func (d *memoryDir) Symlink(oldname string, newname string, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// FSync is a no-op for directories
func (d *memoryDir) FSync() error {
	return nil
}

// Rename prevents directory renaming
func (d *memoryDir) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// SetAttr prevents attribute changes on directories
func (d *memoryDir) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM
}
