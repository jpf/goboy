// Package p9fskit provides wrappers to make fskit.MapFS compatible with 9P protocol on BSD/Darwin systems
package p9fskit

import (
	"context"
	"io/fs"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var (
	// Global inode counter for virtual files
	nextInode uint64 = 1000000
)

// P9FS wraps an fs.FS to provide syscall.Stat_t data for 9P compatibility
type P9FS struct {
	fs.FS
	dev uint64
	uid uint32
	gid uint32
}

// NewP9FS creates a new 9P-compatible filesystem wrapper
func NewP9FS(fsys fs.FS) *P9FS {
	return &P9FS{
		FS:  fsys,
		dev: 1, // Virtual device ID
		uid: uint32(os.Getuid()),
		gid: uint32(os.Getgid()),
	}
}

// Chtimes implements timestamp changing (no-op for virtual files)
func (p *P9FS) Chtimes(name string, atime, mtime time.Time) error {
	// For virtual filesystems, we don't support changing timestamps
	// Just return success to avoid breaking standard file operations
	return nil
}

// WithDevice sets the device ID for this filesystem
func (p *P9FS) WithDevice(dev uint64) *P9FS {
	p.dev = dev
	return p
}

// WithUID sets the default UID for virtual files
func (p *P9FS) WithUID(uid uint32) *P9FS {
	p.uid = uid
	return p
}

// WithGID sets the default GID for virtual files
func (p *P9FS) WithGID(gid uint32) *P9FS {
	p.gid = gid
	return p
}

// Open wraps files to provide Sys() data
func (p *P9FS) Open(name string) (fs.File, error) {
	return p.OpenContext(context.Background(), name)
}

// OpenContext wraps files to provide Sys() data
func (p *P9FS) OpenContext(ctx context.Context, name string) (fs.File, error) {
	var file fs.File
	var err error

	// Try OpenContext if available
	if opener, ok := p.FS.(interface {
		OpenContext(context.Context, string) (fs.File, error)
	}); ok {
		file, err = opener.OpenContext(ctx, name)
	} else {
		file, err = p.FS.Open(name)
	}

	if err != nil {
		return nil, err
	}

	return &p9File{
		File: file,
		p9fs: p,
	}, nil
}

// Stat returns file info with Sys() data
func (p *P9FS) Stat(name string) (fs.FileInfo, error) {
	return p.StatContext(context.Background(), name)
}

// StatContext returns file info with Sys() data
func (p *P9FS) StatContext(ctx context.Context, name string) (fs.FileInfo, error) {
	var info fs.FileInfo
	var err error

	// Try StatContext if available
	if stater, ok := p.FS.(interface {
		StatContext(context.Context, string) (fs.FileInfo, error)
	}); ok {
		info, err = stater.StatContext(ctx, name)
	} else if stater, ok := p.FS.(fs.StatFS); ok {
		info, err = stater.Stat(name)
	} else {
		// Fall back to opening and stating
		file, err := p.OpenContext(ctx, name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		info, err = file.Stat()
	}

	if err != nil {
		return nil, err
	}

	return p.wrapFileInfo(info), nil
}

// p9File wraps an fs.File to provide Sys() data
type p9File struct {
	fs.File
	p9fs *P9FS
}

func (f *p9File) Stat() (fs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return f.p9fs.wrapFileInfo(info), nil
}

// Write forwards to underlying file if it supports writing
func (f *p9File) Write(p []byte) (n int, err error) {
	if writer, ok := f.File.(interface{ Write([]byte) (int, error) }); ok {
		return writer.Write(p)
	}
	return 0, &fs.PathError{Op: "write", Path: ".", Err: fs.ErrPermission}
}

// ReadDir wraps directory entries to provide Sys() data
func (f *p9File) ReadDir(n int) ([]fs.DirEntry, error) {
	// Check if underlying file supports ReadDir
	dirFile, ok := f.File.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: ".", Err: fs.ErrInvalid}
	}

	entries, err := dirFile.ReadDir(n)
	if err != nil {
		return nil, err
	}

	// Wrap each entry
	wrapped := make([]fs.DirEntry, len(entries))
	for i, entry := range entries {
		wrapped[i] = &p9DirEntry{
			DirEntry: entry,
			p9fs:     f.p9fs,
		}
	}

	return wrapped, nil
}

// p9DirEntry wraps fs.DirEntry to provide Sys() data
type p9DirEntry struct {
	fs.DirEntry
	p9fs *P9FS
}

func (e *p9DirEntry) Info() (fs.FileInfo, error) {
	info, err := e.DirEntry.Info()
	if err != nil {
		return nil, err
	}
	return e.p9fs.wrapFileInfo(info), nil
}

// p9FileInfo wraps fs.FileInfo to provide syscall.Stat_t via Sys()
type p9FileInfo struct {
	fs.FileInfo
	stat *syscall.Stat_t
}

func (p *P9FS) wrapFileInfo(info fs.FileInfo) fs.FileInfo {
	// If it already has proper Sys() data, return as-is
	if info.Sys() != nil {
		if _, ok := info.Sys().(*syscall.Stat_t); ok {
			return info
		}
	}

	// Create synthetic syscall.Stat_t
	now := time.Now()
	modTime := info.ModTime()
	if modTime.IsZero() {
		modTime = now
	}

	// Generate a unique inode
	ino := atomic.AddUint64(&nextInode, 1)

	// Convert file mode to Unix mode
	mode := uint32(info.Mode().Perm())
	if info.IsDir() {
		mode |= unix.S_IFDIR
	} else if info.Mode()&fs.ModeSymlink != 0 {
		mode |= unix.S_IFLNK
	} else {
		mode |= unix.S_IFREG
	}

	stat := &syscall.Stat_t{
		Dev:     int32(p.dev),
		Ino:     ino,
		Nlink:   1,
		Mode:    uint16(mode),
		Uid:     p.uid,
		Gid:     p.gid,
		Rdev:    int32(0),
		Size:    info.Size(),
		Blksize: 4096,
		Blocks:  (info.Size() + 511) / 512,
	}

	// Set timestamps
	atimeNsec := modTime.UnixNano()
	mtimeNsec := modTime.UnixNano()
	ctimeNsec := now.UnixNano()

	stat.Atimespec = syscall.NsecToTimespec(atimeNsec)
	stat.Mtimespec = syscall.NsecToTimespec(mtimeNsec)
	stat.Ctimespec = syscall.NsecToTimespec(ctimeNsec)

	return &p9FileInfo{
		FileInfo: info,
		stat:     stat,
	}
}

func (p *p9FileInfo) Sys() interface{} {
	return p.stat
}

// fsAttacher implements p9.Attacher interface
type fsAttacher struct {
	fsys *P9FS
}

// NewAttacher creates a p9.Attacher for the given filesystem
func NewAttacher(fsys *P9FS) interface {
	Attach() (fs.File, error)
} {
	return &fsAttacher{fsys: fsys}
}

// Attach returns the root of the filesystem
func (a *fsAttacher) Attach() (fs.File, error) {
	return a.fsys.Open(".")
}
