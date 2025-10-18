// ABOUTME: State directory implementations for /state tree navigation.
// ABOUTME: Provides p9.File wrappers for state/memory directory hierarchy.

package ninep

import (
	"encoding/binary"
	"math"
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
	case "cartridge":
		qid := d.attacher.qids.Get(p9.TypeDir)
		return []p9.QID{qid}, &cartridgeDir{attacher: d.attacher, qid: qid}, nil
	case "apu":
		qid := d.attacher.qids.Get(p9.TypeDir)
		return []p9.QID{qid}, &apuDir{attacher: d.attacher, qid: qid}, nil
	case "ppu":
		qid := d.attacher.qids.Get(p9.TypeDir)
		return []p9.QID{qid}, &ppuDir{attacher: d.attacher, qid: qid}, nil
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
		{"apu", p9.TypeDir},
		{"cartridge", p9.TypeDir},
		{"cpu", p9.TypeRegular},
		{"memory", p9.TypeDir},
		{"ppu", p9.TypeDir},
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

// cartridgeDir is the /state/cartridge directory
type cartridgeDir struct {
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

func (d *cartridgeDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

func (d *cartridgeDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		return nil, nil, syscall.ENOENT
	}

	// Check if file has been "deleted" (for tar extraction)
	path := "state/cartridge/" + names[0]
	if _, deleted := d.attacher.unlinked.Load(path); deleted {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "info":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9CartridgeInfoFile(d.attacher.gameboy, qid), nil
	case "ram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb: d.attacher.gameboy,
			getMemory: func(gb *gb.Gameboy) []byte {
				cart := gb.GetCartridge()
				if cart == nil {
					return nil
				}
				return cart.GetRAM()
			},
			size:        -1, // Dynamic size based on cartridge type
			commandName: "cartridge-ram-write",
			name:        "ram",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	case "state":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9CartridgeStateFile(d.attacher.gameboy, qid), nil
	default:
		return nil, nil, syscall.ENOENT
	}
}

func (d *cartridgeDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *cartridgeDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"info", p9.TypeRegular},
		{"ram", p9.TypeRegular},
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
func (d *cartridgeDir) UnlinkAt(name string, flags uint32) error {
	// Accept unlink for files that exist
	switch name {
	case "info", "ram", "state":
		path := "state/cartridge/" + name
		d.attacher.unlinked.Store(path, true)
		return nil
	default:
		return syscall.ENOENT
	}
}

// Mkdir prevents directory creation
func (d *cartridgeDir) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// Create handles tar extraction by opening existing virtual files
func (d *cartridgeDir) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	// Remove from unlinked set (file is being "created")
	path := "state/cartridge/" + name
	d.attacher.unlinked.Delete(path)

	// Virtual files always exist, so "create" just opens them
	var file p9.File
	qid := d.attacher.qids.Get(p9.TypeRegular)

	switch name {
	case "info":
		file = newP9CartridgeInfoFile(d.attacher.gameboy, qid)
	case "ram":
		fsys := &binaryMemoryFS{
			gb: d.attacher.gameboy,
			getMemory: func(gb *gb.Gameboy) []byte {
				cart := gb.GetCartridge()
				if cart == nil {
					return nil
				}
				return cart.GetRAM()
			},
			size:        -1,
			commandName: "cartridge-ram-write",
			name:        "ram",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "state":
		file = newP9CartridgeStateFile(d.attacher.gameboy, qid)
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
func (d *cartridgeDir) Link(target p9.File, newname string) error {
	return syscall.EPERM
}

// Mknod prevents device node creation
func (d *cartridgeDir) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// RenameAt prevents file renaming
func (d *cartridgeDir) RenameAt(oldname string, newdir p9.File, newname string) error {
	return syscall.EPERM
}

// Symlink prevents symlink creation
func (d *cartridgeDir) Symlink(oldname string, newname string, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// FSync is a no-op for directories
func (d *cartridgeDir) FSync() error {
	return nil
}

// Rename prevents directory renaming
func (d *cartridgeDir) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// SetAttr prevents attribute changes on directories
func (d *cartridgeDir) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
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
			name:        "wram",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil

	case "oam":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetOAM()[:] },
			size:        0x100,
			commandName: "oam-write",
			name:        "oam",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil

	case "highram":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetHighRAM()[:] },
			size:        0x100,
			commandName: "highram-write",
			name:        "highram",
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
			name:        "wram",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "oam":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetOAM()[:] },
			size:        0x100,
			commandName: "oam-write",
			name:        "oam",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "highram":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetHighRAM()[:] },
			size:        0x100,
			commandName: "highram-write",
			name:        "highram",
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

// apuDir is the /state/apu directory
type apuDir struct {
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

func (d *apuDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

func (d *apuDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		return nil, nil, syscall.ENOENT
	}

	// Check if file has been "deleted" (for tar extraction)
	path := "state/apu/" + names[0]
	if _, deleted := d.attacher.unlinked.Load(path); deleted {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "state":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb: d.attacher.gameboy,
			getMemory: func(gb *gb.Gameboy) []byte {
				gb.Mu.RLock()
				defer gb.Mu.RUnlock()
				playing, memory, lVol, rVol, tickCounter := gb.GetAPUState()

				// Serialize APU state to binary
				data := make([]byte, 77)
				data[0] = playing
				copy(data[1:53], memory[:])
				binary.LittleEndian.PutUint64(data[53:61], math.Float64bits(lVol))
				binary.LittleEndian.PutUint64(data[61:69], math.Float64bits(rVol))
				binary.LittleEndian.PutUint64(data[69:77], math.Float64bits(tickCounter))
				return data
			},
			size:        77,
			commandName: "apu-state-write",
			name:        "state",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	default:
		return nil, nil, syscall.ENOENT
	}
}

func (d *apuDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *apuDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
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
func (d *apuDir) UnlinkAt(name string, flags uint32) error {
	switch name {
	case "state":
		path := "state/apu/" + name
		d.attacher.unlinked.Store(path, true)
		return nil
	default:
		return syscall.ENOENT
	}
}

// Mkdir prevents directory creation
func (d *apuDir) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// Create handles tar extraction by opening existing virtual files
func (d *apuDir) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	path := "state/apu/" + name
	d.attacher.unlinked.Delete(path)

	var file p9.File
	qid := d.attacher.qids.Get(p9.TypeRegular)

	switch name {
	case "state":
		fsys := &binaryMemoryFS{
			gb: d.attacher.gameboy,
			getMemory: func(gb *gb.Gameboy) []byte {
				gb.Mu.RLock()
				defer gb.Mu.RUnlock()
				playing, memory, lVol, rVol, tickCounter := gb.GetAPUState()

				// Serialize APU state to binary
				data := make([]byte, 77)
				data[0] = playing
				copy(data[1:53], memory[:])
				binary.LittleEndian.PutUint64(data[53:61], math.Float64bits(lVol))
				binary.LittleEndian.PutUint64(data[61:69], math.Float64bits(rVol))
				binary.LittleEndian.PutUint64(data[69:77], math.Float64bits(tickCounter))
				return data
			},
			size:        77,
			commandName: "apu-state-write",
			name:        "state",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	default:
		return nil, p9.QID{}, 0, syscall.ENOENT
	}

	_, iounit, err := file.Open(flags)
	if err != nil {
		return nil, p9.QID{}, 0, err
	}
	return file, qid, iounit, nil
}

// Link prevents hard link creation
func (d *apuDir) Link(target p9.File, newname string) error {
	return syscall.EPERM
}

// Mknod prevents device node creation
func (d *apuDir) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// RenameAt prevents file renaming
func (d *apuDir) RenameAt(oldname string, newdir p9.File, newname string) error {
	return syscall.EPERM
}

// Symlink prevents symlink creation
func (d *apuDir) Symlink(oldname string, newname string, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// FSync is a no-op for directories
func (d *apuDir) FSync() error {
	return nil
}

// Rename prevents directory renaming
func (d *apuDir) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// SetAttr prevents attribute changes on directories
func (d *apuDir) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM
}

// ppuDir is the /state/ppu directory
type ppuDir struct {
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

func (d *ppuDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

func (d *ppuDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		return nil, nil, syscall.ENOENT
	}

	// Check if file has been "deleted" (for tar extraction)
	path := "state/ppu/" + names[0]
	if _, deleted := d.attacher.unlinked.Load(path); deleted {
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "state":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		return []p9.QID{qid}, newP9PPUStateFile(d.attacher.gameboy, qid), nil
	case "tilescanline":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetTileScanline()[:] },
			size:        160,
			commandName: "ppu-tilescanline-write",
			name:        "tilescanline",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	case "bgpalette":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetBGPalette()[:] },
			size:        66,
			commandName: "ppu-bgpalette-write",
			name:        "bgpalette",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	case "spritepalette":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetSpritePalette()[:] },
			size:        66,
			commandName: "ppu-spritepalette-write",
			name:        "spritepalette",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	case "bgpriority":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetBGPriority()[:] },
			size:        2880,
			commandName: "ppu-bgpriority-write",
			name:        "bgpriority",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	case "screen":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetScreen()[:] },
			size:        69120,
			commandName: "ppu-screen-write",
			name:        "screen",
		}
		return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, fsys), nil
	default:
		return nil, nil, syscall.ENOENT
	}
}

func (d *ppuDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

func (d *ppuDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"bgpalette", p9.TypeRegular},
		{"bgpriority", p9.TypeRegular},
		{"screen", p9.TypeRegular},
		{"spritepalette", p9.TypeRegular},
		{"state", p9.TypeRegular},
		{"tilescanline", p9.TypeRegular},
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
func (d *ppuDir) UnlinkAt(name string, flags uint32) error {
	switch name {
	case "bgpalette", "bgpriority", "screen", "spritepalette", "state", "tilescanline":
		path := "state/ppu/" + name
		d.attacher.unlinked.Store(path, true)
		return nil
	default:
		return syscall.ENOENT
	}
}

// Mkdir prevents directory creation
func (d *ppuDir) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// Create handles tar extraction by opening existing virtual files
func (d *ppuDir) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	path := "state/ppu/" + name
	d.attacher.unlinked.Delete(path)

	var file p9.File
	qid := d.attacher.qids.Get(p9.TypeRegular)

	switch name {
	case "state":
		file = newP9PPUStateFile(d.attacher.gameboy, qid)
	case "tilescanline":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetTileScanline()[:] },
			size:        160,
			commandName: "ppu-tilescanline-write",
			name:        "tilescanline",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "bgpalette":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetBGPalette()[:] },
			size:        66,
			commandName: "ppu-bgpalette-write",
			name:        "bgpalette",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "spritepalette":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetSpritePalette()[:] },
			size:        66,
			commandName: "ppu-spritepalette-write",
			name:        "spritepalette",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "bgpriority":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetBGPriority()[:] },
			size:        2880,
			commandName: "ppu-bgpriority-write",
			name:        "bgpriority",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	case "screen":
		fsys := &binaryMemoryFS{
			gb:          d.attacher.gameboy,
			getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetScreen()[:] },
			size:        69120,
			commandName: "ppu-screen-write",
			name:        "screen",
		}
		file = newP9BinaryFile(d.attacher.gameboy, qid, fsys)
	default:
		return nil, p9.QID{}, 0, syscall.ENOENT
	}

	_, iounit, err := file.Open(flags)
	if err != nil {
		return nil, p9.QID{}, 0, err
	}
	return file, qid, iounit, nil
}

// Link prevents hard link creation
func (d *ppuDir) Link(target p9.File, newname string) error {
	return syscall.EPERM
}

// Mknod prevents device node creation
func (d *ppuDir) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// RenameAt prevents file renaming
func (d *ppuDir) RenameAt(oldname string, newdir p9.File, newname string) error {
	return syscall.EPERM
}

// Symlink prevents symlink creation
func (d *ppuDir) Symlink(oldname string, newname string, uid p9.UID, gid p9.GID) (p9.QID, error) {
	return p9.QID{}, syscall.EPERM
}

// FSync is a no-op for directories
func (d *ppuDir) FSync() error {
	return nil
}

// Rename prevents directory renaming
func (d *ppuDir) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// SetAttr prevents attribute changes on directories
func (d *ppuDir) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM
}
