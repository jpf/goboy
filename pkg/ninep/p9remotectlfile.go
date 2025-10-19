// ABOUTME: p9.File wrapper for remote-control.py script (read-only).
// ABOUTME: Serves the Python remote control script for download via 9P.

package ninep

import (
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
)

const remoteControlScript = `#!/usr/bin/env python3
"""Remote control for GoBoy via 9P filesystem.

Keys: z=A x=B space=select enter=start arrows=directions q=quit
Each key press toggles the button (press/release).
"""

import sys
import tty
import termios
from pathlib import Path

BUTTONS_FILE = Path("/mnt/gb/state/buttons")

# Key mappings
KEY_MAP = {
    'z': 'a',
    'x': 'b',
    '\r': 'start',  # Enter
    ' ': 'select',
    '\x1b[A': 'up',     # Up arrow
    '\x1b[B': 'down',   # Down arrow
    '\x1b[C': 'right',  # Right arrow
    '\x1b[D': 'left',   # Left arrow
}

def read_key():
    """Read a single keypress including escape sequences."""
    fd = sys.stdin.fileno()
    old_settings = termios.tcgetattr(fd)
    try:
        tty.setraw(fd)
        ch = sys.stdin.read(1)

        # Handle escape sequences (arrow keys)
        if ch == '\x1b':
            ch += sys.stdin.read(2)

        return ch
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)

def main():
    """Main remote control loop."""
    pressed = set()

    print("GoBoy Remote Control")
    print("z=A x=B space=select enter=start arrows=directions q=quit")
    print()

    while True:
        key = read_key()

        if key == 'q':
            # Release all pressed buttons before quitting
            for button in pressed:
                BUTTONS_FILE.write_text(f"release {button}\n")
            break

        if key not in KEY_MAP:
            continue

        button = KEY_MAP[key]

        if button in pressed:
            # Release button
            BUTTONS_FILE.write_text(f"release {button}\n")
            pressed.remove(button)
            print(f"Released: {button}")
        else:
            # Press button
            BUTTONS_FILE.write_text(f"press {button}\n")
            pressed.add(button)
            print(f"Pressed:  {button}")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\nExiting...")
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
`

type p9RemoteCtlFile struct {
	statfs
	p9.DefaultWalkGetAttr
	templatefs.NilCloser
	templatefs.NotDirectoryFile
	templatefs.NotSymlinkFile
	templatefs.NoopRenamed
	templatefs.XattrUnimplemented
	templatefs.NotLockable

	qid      p9.QID
	readPos  int
	opened   bool
}

func newRemoteCtlFile(qid p9.QID) *p9RemoteCtlFile {
	return &p9RemoteCtlFile{
		qid: qid,
	}
}

func (f *p9RemoteCtlFile) Walk(names []string) ([]p9.QID, p9.File, error) {
	if len(names) == 0 {
		return []p9.QID{f.qid}, f, nil
	}
	return nil, nil, syscall.ENOTDIR
}

func (f *p9RemoteCtlFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	if f.opened {
		return p9.QID{}, 0, syscall.EINVAL
	}

	// Read-only file
	if mode&p9.WriteOnly != 0 || mode&p9.ReadWrite != 0 {
		return p9.QID{}, 0, syscall.EPERM
	}

	f.opened = true
	f.readPos = 0
	return f.qid, 8192, nil
}

func (f *p9RemoteCtlFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return f.qid, req, p9.Attr{
		Mode:      p9.ModeRegular | 0555, // Read-only + executable
		UID:       p9.UID(os.Getuid()),
		GID:       p9.GID(os.Getgid()),
		Size:      uint64(len(remoteControlScript)),
		NLink:     1,
		BlockSize: 4096,
	}, nil
}

func (f *p9RemoteCtlFile) ReadAt(p []byte, offset int64) (int, error) {
	if !f.opened {
		return 0, syscall.EINVAL
	}

	if offset >= int64(len(remoteControlScript)) {
		return 0, syscall.EINVAL
	}

	n := copy(p, remoteControlScript[offset:])
	return n, nil
}

func (f *p9RemoteCtlFile) WriteAt(p []byte, offset int64) (int, error) {
	return 0, syscall.EPERM
}

func (f *p9RemoteCtlFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	return syscall.EPERM
}

func (f *p9RemoteCtlFile) FSync() error {
	return nil
}

func (f *p9RemoteCtlFile) Rename(directory p9.File, name string) error {
	return syscall.EPERM
}

func (f *p9RemoteCtlFile) Close() error {
	f.opened = false
	return nil
}

// remoteCtlFileInfo implements fs.FileInfo for remote-control.py
type remoteCtlFileInfo struct{}

func (fi *remoteCtlFileInfo) Name() string       { return "remote-control.py" }
func (fi *remoteCtlFileInfo) Size() int64        { return int64(len(remoteControlScript)) }
func (fi *remoteCtlFileInfo) Mode() fs.FileMode  { return 0555 }
func (fi *remoteCtlFileInfo) ModTime() time.Time { return time.Now() }
func (fi *remoteCtlFileInfo) IsDir() bool        { return false }
func (fi *remoteCtlFileInfo) Sys() interface{}   { return nil }
