// ABOUTME: p9.File wrappers that bridge fs.FS to 9P protocol.
// ABOUTME: Implements p9.File interface using templatefs for default implementations.

package ninep

import (
	"io"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

// p9Attacher implements p9.Attacher and returns the root directory
type p9Attacher struct {
	gameboy  *gb.Gameboy
	qids     *p9.QIDGenerator
	unlinked sync.Map // Tracks "deleted" files for tar extraction (key: "path/file", value: bool)
}

// NewP9Attacher creates a p9.Attacher for the virtual filesystem
func NewP9Attacher(gameboy *gb.Gameboy) p9.Attacher {
	return &p9Attacher{
		gameboy: gameboy,
		qids:    &p9.QIDGenerator{},
	}
}

// Attach implements p9.Attacher
func (a *p9Attacher) Attach() (p9.File, error) {
	logf("Attach() called - creating root directory")
	return &rootDir{
		attacher: a,
		qid:      a.qids.Get(p9.TypeDir),
	}, nil
}

// statfs provides StatFS implementation for all files
type statfs struct{}

func (statfs) StatFS() (p9.FSStat, error) {
	return p9.FSStat{
		Type:      0x01021997, /* V9FS_MAGIC */
		BlockSize: 4096,
	}, nil
}

// rootDir is the root directory file
type rootDir struct {
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

// Open implements p9.File.Open
func (d *rootDir) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	logf("rootDir.Open(mode=%v)", mode)
	if mode.Mode() == p9.ReadOnly {
		return d.qid, 4096, nil
	}
	logf("rootDir.Open() failed: write not permitted")
	return p9.QID{}, 0, syscall.EPERM
}

// Walk implements p9.File.Walk
func (d *rootDir) Walk(names []string) ([]p9.QID, p9.File, error) {
	logf("rootDir.Walk(names=%v)", names)
	if len(names) == 0 {
		return []p9.QID{d.qid}, d, nil
	}

	if len(names) > 1 {
		logf("rootDir.Walk() failed: multi-component walk not supported")
		return nil, nil, syscall.ENOENT
	}

	switch names[0] {
	case "README":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		logf("rootDir.Walk() -> README file")
		return []p9.QID{qid}, &readmeFile{qid: qid}, nil
	case "ctl":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		logf("rootDir.Walk() -> ctl file")
		return []p9.QID{qid}, newCtlFile(d.attacher.gameboy, qid), nil
	case "rom":
		qid := d.attacher.qids.Get(p9.TypeRegular)
		logf("rootDir.Walk() -> rom file")
		return []p9.QID{qid}, newROMFile(d.attacher.gameboy, qid), nil
	case "state":
		qid := d.attacher.qids.Get(p9.TypeDir)
		logf("rootDir.Walk() -> state directory")
		return []p9.QID{qid}, &stateDir{attacher: d.attacher, qid: qid}, nil
	case "meta":
		qid := d.attacher.qids.Get(p9.TypeDir)
		logf("rootDir.Walk() -> meta directory")
		return []p9.QID{qid}, &metaDir{attacher: d.attacher, qid: qid}, nil
	default:
		logf("rootDir.Walk() failed: '%s' not found", names[0])
		return nil, nil, syscall.ENOENT
	}
}

// GetAttr implements p9.File.GetAttr
func (d *rootDir) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return d.qid, req, p9.Attr{
		Mode:  p9.ModeDirectory | 0755,
		UID:   p9.UID(os.Getuid()),
		GID:   p9.GID(os.Getgid()),
		NLink: 2,
	}, nil
}

// Readdir implements p9.File.Readdir
func (d *rootDir) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	files := []struct {
		name string
		typ  p9.QIDType
	}{
		{"README", p9.TypeRegular},
		{"ctl", p9.TypeRegular},
		{"meta", p9.TypeDir},
		{"state", p9.TypeDir},
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

// readmeFile is a read-only static file
type readmeFile struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.ReadOnlyFile
	templatefs.NilCloser
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	qid    p9.QID
	reader *strings.Reader
}

// Walk implements p9.File.Walk
func (f *readmeFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

// Open implements p9.File.Open
func (f *readmeFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if mode == p9.ReadOnly {
		f.reader = strings.NewReader(readmeContent)
		return f.qid, 4096, nil
	}
	return p9.QID{}, 0, syscall.EPERM
}

// GetAttr implements p9.File.GetAttr
func (f *readmeFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0644,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		NLink:     1,
		Size:      uint64(len(readmeContent)),
		BlockSize: 4096,
	}, nil
}

// ReadAt implements p9.File.ReadAt
func (f *readmeFile) ReadAt(p []byte, offset int64) (int, error) {
	if f.reader == nil {
		return 0, syscall.EINVAL
	}
	return f.reader.ReadAt(p, offset)
}

// p9CtlFile wraps ctlFile to provide p9.File interface
type p9CtlFile struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NilCloser
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	qid      p9.QID
	gameboy  *gb.Gameboy
	ctlfs    *ctlFS
	opened   bool
	offset   int64
}

func newCtlFile(gameboy *gb.Gameboy, qid p9.QID) *p9CtlFile {
	return &p9CtlFile{
		qid:     qid,
		gameboy: gameboy,
		ctlfs:   newCtlFS(gameboy),
	}
}

// Walk implements p9.File.Walk
func (f *p9CtlFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

// Open implements p9.File.Open
func (f *p9CtlFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	f.opened = true
	f.offset = 0
	return f.qid, 4096, nil
}

// GetAttr implements p9.File.GetAttr
func (f *p9CtlFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	// Get current status to calculate size
	file, _ := f.ctlfs.Open(".")
	defer file.Close()
	info, _ := file.Stat()

	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0666,
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		NLink:     1,
		Size:      uint64(info.Size()),
		BlockSize: 4096,
	}, nil
}

// ReadAt implements p9.File.ReadAt
func (f *p9CtlFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	// Open fresh ctlFile for reading
	file, err := f.ctlfs.Open(".")
	if err != nil {
		return 0, syscall.EIO
	}
	defer file.Close()

	// ctlFile doesn't support offset, so we need to read and discard
	if offset > 0 {
		discard := make([]byte, offset)
		if _, err := file.Read(discard); err != nil && err != io.EOF {
			return 0, syscall.EIO
		}
	}

	n, err := file.Read(p)
	if err == io.EOF {
		return n, nil
	}
	return n, err
}

// WriteAt implements p9.File.WriteAt
func (f *p9CtlFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	// Open fresh ctlFile for writing
	file, err := f.ctlfs.Open(".")
	if err != nil {
		return 0, syscall.EIO
	}
	defer file.Close()

	// ctlFile expects full write at once
	if writer, ok := file.(interface{ Write([]byte) (int, error) }); ok {
		return writer.Write(p)
	}
	return 0, syscall.EPERM
}

// SetAttr implements p9.File.SetAttr (for truncate operations)
func (f *p9CtlFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	// Allow truncate to size 0 (common before write)
	if valid.Size && attr.Size == 0 {
		return nil
	}
	return syscall.EPERM
}

// FSync implements p9.File.FSync
func (f *p9CtlFile) FSync() error {
	// Virtual file, no backing storage to sync
	return nil
}

// Remove implements p9.File.Remove
func (f *p9CtlFile) Remove() error {
	return syscall.EPERM
}

// Rename implements p9.File.Rename
func (f *p9CtlFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

// p9ROMFile implements p9.File for ROM loading
type p9ROMFile struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	qid     p9.QID
	gameboy *gb.Gameboy
	romfs   *romFS
	romfile *romFile
	opened  bool
}

func newROMFile(gameboy *gb.Gameboy, qid p9.QID) *p9ROMFile {
	return &p9ROMFile{
		qid:     qid,
		gameboy: gameboy,
		romfs:   newROMFS(gameboy),
	}
}

// Walk implements p9.File.Walk
func (f *p9ROMFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

// Open implements p9.File.Open
func (f *p9ROMFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	f.opened = true
	// Open underlying romFile for writing
	fsFile, _ := f.romfs.Open(".")
	f.romfile = fsFile.(*romFile)
	return f.qid, 4096, nil
}

// GetAttr implements p9.File.GetAttr
func (f *p9ROMFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0222, // Write-only
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		Size:      0,
		BlockSize: 4096,
	}, nil
}

// SetAttr implements p9.File.SetAttr
func (f *p9ROMFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return nil
}

// ReadAt implements p9.File.ReadAt
func (f *p9ROMFile) ReadAt(p []byte, offset int64) (int, error) {
	// ROM file is write-only
	return 0, syscall.EPERM
}

// WriteAt implements p9.File.WriteAt
func (f *p9ROMFile) WriteAt(p []byte, offset int64) (int, error) {
	if !f.opened || f.romfile == nil {
		return 0, syscall.EINVAL
	}
	// romFile.Write() handles buffering
	return f.romfile.Write(p)
}

// Close implements p9.File.Close
func (f *p9ROMFile) Close() error {
	if f.romfile != nil {
		return f.romfile.Close()
	}
	return nil
}

// FSync implements p9.File.FSync
func (f *p9ROMFile) FSync() error {
	return nil
}

// Remove implements p9.File.Remove
func (f *p9ROMFile) Remove() error {
	return syscall.EPERM
}

// Rename implements p9.File.Rename
func (f *p9ROMFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}
