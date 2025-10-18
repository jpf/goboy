// ABOUTME: Control file implementation for pause/resume/reset commands.
// ABOUTME: Implements fs.FS and fs.File interfaces for dynamic control interface.

package ninep

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

// ctlFS implements fs.FS and returns ctlFile instances
type ctlFS struct {
	mu sync.RWMutex
	gb *gb.Gameboy
}

func newCtlFS(gameboy *gb.Gameboy) *ctlFS {
	return &ctlFS{gb: gameboy}
}

func (c *ctlFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &ctlFile{parent: c}, nil
}

// ctlFile acts as a control interface - writes trigger actions, reads return status
type ctlFile struct {
	parent  *ctlFS
	readPos int
}

func (f *ctlFile) Read(p []byte) (n int, err error) {
	// Generate current status
	status := "running"
	if f.parent.gb.IsPaused() {
		status = "paused"
	}

	content := fmt.Sprintf("%s\n\npause - pause emulation\nresume - resume emulation\nreset - reset to power-on state\n", status)

	// Read from current position
	if f.readPos >= len(content) {
		return 0, io.EOF
	}

	n = copy(p, content[f.readPos:])
	f.readPos += n

	return n, nil
}

func (f *ctlFile) Write(p []byte) (n int, err error) {
	command := string(bytes.TrimSpace(p))

	// Ignore empty writes (from truncate operations)
	if command == "" {
		return len(p), nil
	}

	// Send command to emulator
	switch command {
	case CommandPause, CommandResume, CommandReset:
		f.parent.gb.GetCommandChan() <- gb.Command{Name: command}
	default:
		return 0, fmt.Errorf("unknown command: %s", command)
	}

	return len(p), nil
}

func (f *ctlFile) Close() error {
	return nil
}

func (f *ctlFile) Stat() (fs.FileInfo, error) {
	// Calculate current content size
	status := "running"
	if f.parent.gb.IsPaused() {
		status = "paused"
	}
	content := fmt.Sprintf("%s\n\npause - pause emulation\nresume - resume emulation\nreset - reset to power-on state\n", status)

	return &ctlFileInfo{
		name: "ctl",
		size: int64(len(content)),
		mode: 0666,
	}, nil
}

type ctlFileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (fi *ctlFileInfo) Name() string       { return fi.name }
func (fi *ctlFileInfo) Size() int64        { return fi.size }
func (fi *ctlFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *ctlFileInfo) ModTime() time.Time { return time.Now() }
func (fi *ctlFileInfo) IsDir() bool        { return false }
func (fi *ctlFileInfo) Sys() interface{}   { return nil }
