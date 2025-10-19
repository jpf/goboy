// ABOUTME: Button input filesystem for /state/buttons file.
// ABOUTME: Supports reading current button state and writing press/release commands.

package ninep

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

// buttonsFS implements fs.FS for the /state/buttons file
type buttonsFS struct {
	mu sync.RWMutex
	gb *gb.Gameboy
}

func newButtonsFS(gameboy *gb.Gameboy) *buttonsFS {
	return &buttonsFS{gb: gameboy}
}

func (f *buttonsFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &buttonsFile{parent: f}, nil
}

// buttonsFile implements fs.File for button input
type buttonsFile struct {
	parent  *buttonsFS
	readPos int
}

func (f *buttonsFile) Read(p []byte) (n int, err error) {
	// Get current button state
	f.parent.gb.Mu.RLock()
	inputMask := f.parent.gb.GetInputMask()
	f.parent.gb.Mu.RUnlock()

	// Build list of currently pressed buttons
	var pressed []string
	buttons := []struct {
		button gb.Button
		name   string
	}{
		{gb.ButtonA, "a"},
		{gb.ButtonB, "b"},
		{gb.ButtonSelect, "select"},
		{gb.ButtonStart, "start"},
		{gb.ButtonRight, "right"},
		{gb.ButtonLeft, "left"},
		{gb.ButtonUp, "up"},
		{gb.ButtonDown, "down"},
	}

	for _, btn := range buttons {
		// Button is pressed if bit is 0 (cleared)
		if (inputMask & (1 << byte(btn.button))) == 0 {
			pressed = append(pressed, btn.name)
		}
	}

	// Format output: space-separated list of pressed buttons
	content := strings.Join(pressed, " ")
	if len(pressed) > 0 {
		content += "\n"
	}

	// Read from current position
	if f.readPos >= len(content) {
		return 0, io.EOF
	}

	n = copy(p, content[f.readPos:])
	f.readPos += n

	return n, nil
}

func (f *buttonsFile) Write(p []byte) (n int, err error) {
	input := strings.TrimSpace(string(p))

	// Ignore empty writes
	if input == "" {
		return len(p), nil
	}

	// Parse input: "press a", "release b", or just "a" (momentary)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return len(p), nil
	}

	var action string
	var buttonName string

	if len(parts) == 1 {
		// Momentary press: just "a" means press+release
		action = "momentary"
		buttonName = parts[0]
	} else if len(parts) == 2 {
		// Explicit action: "press a" or "release b"
		action = parts[0]
		buttonName = parts[1]
	} else {
		return 0, fmt.Errorf("invalid button command format: %s", input)
	}

	// Map button name to Button value
	button, ok := parseButtonName(buttonName)
	if !ok {
		return 0, fmt.Errorf("unknown button: %s", buttonName)
	}

	// Queue button command(s)
	switch action {
	case "press":
		f.parent.gb.GetCommandChan() <- gb.Command{
			Name: "button-press",
			Data: []byte{byte(button)},
		}
	case "release":
		f.parent.gb.GetCommandChan() <- gb.Command{
			Name: "button-release",
			Data: []byte{byte(button)},
		}
	case "momentary":
		// Queue both press and release
		f.parent.gb.GetCommandChan() <- gb.Command{
			Name: "button-press",
			Data: []byte{byte(button)},
		}
		f.parent.gb.GetCommandChan() <- gb.Command{
			Name: "button-release",
			Data: []byte{byte(button)},
		}
	default:
		return 0, fmt.Errorf("invalid action: %s (use 'press' or 'release')", action)
	}

	return len(p), nil
}

func (f *buttonsFile) Close() error {
	return nil
}

func (f *buttonsFile) Stat() (fs.FileInfo, error) {
	// Get current button state for size calculation
	f.parent.gb.Mu.RLock()
	inputMask := f.parent.gb.GetInputMask()
	f.parent.gb.Mu.RUnlock()

	// Count pressed buttons
	var pressed int
	for i := 0; i < 8; i++ {
		if (inputMask & (1 << i)) == 0 {
			pressed++
		}
	}

	// Approximate size (button names + spaces)
	size := pressed * 8 // rough average

	return &buttonsFileInfo{
		name: "buttons",
		size: int64(size),
		mode: 0666,
	}, nil
}

type buttonsFileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (fi *buttonsFileInfo) Name() string       { return fi.name }
func (fi *buttonsFileInfo) Size() int64        { return fi.size }
func (fi *buttonsFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *buttonsFileInfo) ModTime() time.Time { return time.Now() }
func (fi *buttonsFileInfo) IsDir() bool        { return false }
func (fi *buttonsFileInfo) Sys() interface{}   { return nil }

// parseButtonName converts button name string to Button value
func parseButtonName(name string) (gb.Button, bool) {
	switch strings.ToLower(name) {
	case "a":
		return gb.ButtonA, true
	case "b":
		return gb.ButtonB, true
	case "select":
		return gb.ButtonSelect, true
	case "start":
		return gb.ButtonStart, true
	case "up":
		return gb.ButtonUp, true
	case "down":
		return gb.ButtonDown, true
	case "left":
		return gb.ButtonLeft, true
	case "right":
		return gb.ButtonRight, true
	default:
		return 0, false
	}
}
