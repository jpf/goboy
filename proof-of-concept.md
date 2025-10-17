# 9P Server Proof-of-Concept Implementation Guide

## Document Status

**Last Updated:** 2025-10-15
**Status:** Ready for implementation
**Changes from Original:**
- Clarified `Command` type location (in `pkg/gb` to avoid circular dependencies)
- Added `ProcessCommands()` method to `Gameboy` struct for proper encapsulation
- Updated Phase 2 to include `SetPaused()` method (even though `paused` field already exists)
- Fixed channel direction issues in command processing
- Added architecture decision documentation
- Added testing environment alternatives
- Added TDD requirements throughout all phases
- Added fix for togglePaused() race condition

## Overview

This document details the implementation plan for integrating a minimal 9P server into the GoBoy emulator. The proof-of-concept will serve a virtual filesystem over TCP with `/README` and `/ctl` files, establishing the threading model and command processing pattern for future expansion.

## Design Summary

**What we're building:**
- Minimal 9P server serving `/README` (static) and `/ctl` (dynamic control interface)
- Uses p9fskit to create virtual filesystem served over TCP
- Command channel pattern for pause/resume control (reset deferred to later)
- Mutex-protected reads of emulator state from 9P goroutine
- All state modifications happen on main goroutine at frame boundaries

**Threading model:**
- Main goroutine: existing game loop (`startGBLoop`)
- 9P server goroutine: handles network/file operations
- Shared state: `Gameboy` struct and its components

**Reference documents:**
- `9p-spec.md` - Complete interface specification
- `somename/dynamic-fskit-9p.go` - Working p9fskit example
- `savestate-architecture.md` - Emulator state documentation

---

## Lessons Learned from Testing

**Test implementation:** `somename/test-fskit-write.go` - Successfully demonstrated writable files over 9P

### Critical p9fskit Wrapper Requirements

The `somename/p9fskit/wrapper.go` requires these additions for write support:

1. **Add `Write()` method to `p9File`:**
   ```go
   // Write forwards to underlying file if it supports writing
   func (f *p9File) Write(p []byte) (n int, err error) {
       if writer, ok := f.File.(interface{ Write([]byte) (int, error) }); ok {
           return writer.Write(p)
       }
       return 0, &fs.PathError{Op: "write", Path: ".", Err: fs.ErrPermission}
   }
   ```

   The `p9File` wrapper embeds `fs.File`, which only includes `Read()`. Even though underlying files may implement `Write()`, the wrapper must explicitly forward write operations.

2. **Add `Chtimes()` method to `P9FS`:**
   ```go
   // Chtimes implements timestamp changing (no-op for virtual files)
   func (p *P9FS) Chtimes(name string, atime, mtime time.Time) error {
       // For virtual filesystems, we don't support changing timestamps
       // Just return success to avoid breaking standard file operations
       return nil
   }
   ```

   Shell redirects (like `echo foo > file`) attempt to set file timestamps after writing. Without this method, writes fail with I/O errors.

### Writable File Architecture Pattern

Files in `fskit.MapFS` must be implemented as filesystem types (not bare file types):

```go
// Each "file" is actually an FS that returns file instances on Open()
type ctlFS struct {
    mu       sync.RWMutex
    paused   bool
    commands []string
}

func (c *ctlFS) Open(name string) (fs.File, error) {
    if name != "." && name != "" {
        return nil, fs.ErrNotExist
    }
    return &ctlFile{parent: c}, nil
}

// The actual file references the parent FS for shared state
type ctlFile struct {
    parent  *ctlFS
    readPos int
}

func (f *ctlFile) Read(p []byte) (int, error) { /* use parent state */ }
func (f *ctlFile) Write(p []byte) (int, error) { /* modify parent state */ }
```

This pattern ensures:
- Each `Open()` creates a new file instance with its own read position
- All instances share the same underlying state through the parent FS
- Thread-safe access via mutex in the parent FS

### Control File Write Handling

Control files must handle empty writes gracefully:

```go
func (f *ctlFile) Write(p []byte) (n int, err error) {
    command := string(bytes.TrimSpace(p))

    // Ignore empty writes (from truncate operations)
    if command == "" {
        return len(p), nil
    }

    // Process actual commands...
}
```

Shell redirects (`echo pause > ctl`) perform two writes:
1. Truncate write (empty byte array)
2. Actual content write

Returning an error for the empty write causes the entire operation to fail.

### File Permissions

Files must report writable permissions (0666) in their `FileInfo.Mode()`:

```go
func (f *ctlFile) Stat() (fs.FileInfo, error) {
    return &ctlFileInfo{
        name: "ctl",
        size: int64(len(content)),
        mode: 0666, // Must include write bits
    }, nil
}
```

The 9P protocol checks file permissions before allowing write operations.

---

## Implementation Plan

### Phase 1: Set up dependencies and project structure

#### Task 1.1: Copy p9fskit into the project
- Copy `somename/p9fskit/` directory to `pkg/p9fskit/`
- Update package name from `p9fskit` to `p9fskit` (should already be correct)
- Update any internal imports if needed
- **IMPORTANT:** The wrapper already has the required `Write()` and `Chtimes()` methods added from testing
- Verify p9fskit builds: `go build ./pkg/p9fskit`

#### Task 1.2: Create pkg/ninep package
- Create directory: `mkdir -p pkg/ninep`
- Create stub files: `pkg/ninep/server.go` and `pkg/ninep/ctlfile.go`
- Add ABOUTME comments to each file:
  ```go
  // ABOUTME: 9P server implementation for exposing emulator state over network.
  // ABOUTME: Serves virtual filesystem using fskit and p9fskit wrapper.
  ```

#### Task 1.3: Update project dependencies
- Run: `go get github.com/hugelgupf/p9`
- Run: `go get github.com/u-root/uio`
- Run: `go get tractor.dev/wanix`
- Verify: `go mod tidy`
- Ensure all dependencies are resolved

---

### Phase 2: Add infrastructure to Gameboy struct

**Reference:** `pkg/gb/gameboy.go`

#### Task 2.1: Add synchronization fields to Gameboy
In `pkg/gb/gameboy.go`, add to the `Gameboy` struct:
```go
// 9P server support (exported for testing)
Mu          sync.RWMutex
CommandChan chan Command
```

**Note:**
- The `paused bool` field already exists in the Gameboy struct (line 30)
- There is an existing private `togglePaused()` method used by keyboard input; we're adding `SetPaused()` for explicit control from the 9P interface
- Fields are exported (capitalized) to allow test initialization from other packages

Define the `Command` type in `pkg/gb/gameboy.go` (placing it here avoids circular dependencies):
```go
// Command represents a control command from the 9P interface.
type Command struct {
    Name string
}
```

#### Task 2.2: Initialize command channel
In `setup()` method, add:
```go
gb.CommandChan = make(chan Command, 10)
```

#### Task 2.3: Add IsPaused() method
Add to `gameboy.go`:
```go
// IsPaused returns whether the emulator is currently paused.
// This method is safe to call from other goroutines.
func (gb *Gameboy) IsPaused() bool {
    gb.Mu.RLock()
    defer gb.Mu.RUnlock()
    return gb.paused
}
```

Also add methods to set/get paused state and command processing:
```go
// SetPaused sets the paused state of the emulator.
func (gb *Gameboy) SetPaused(paused bool) {
    gb.Mu.Lock()
    defer gb.Mu.Unlock()
    gb.paused = paused
}

// GetCommandChan returns the command channel for sending control commands.
// This returns a send-only channel for use by the 9P server goroutine.
func (gb *Gameboy) GetCommandChan() chan<- Command {
    return gb.CommandChan
}

// ProcessCommands drains and processes all pending commands from the 9P interface.
// This should be called at frame boundaries from the main game loop.
func (gb *Gameboy) ProcessCommands() {
    for {
        select {
        case cmd := <-gb.CommandChan:
            switch cmd.Name {
            case "pause":
                gb.SetPaused(true)
            case "resume":
                gb.SetPaused(false)
            default:
                // Unknown command, ignore
            }
        default:
            // No more commands
            return
        }
    }
}
```

#### Task 2.4: Fix togglePaused() race condition
The existing `togglePaused()` method (line 98-101) accesses `gb.paused` without mutex protection, creating a race condition with the 9P server. Update it to use the mutex:

```go
// togglePaused switches the paused state of the execution.
func (gb *Gameboy) togglePaused() {
    gb.Mu.Lock()
    defer gb.Mu.Unlock()
    gb.paused = !gb.paused
}
```

This ensures all access to `paused` is mutex-protected, whether from keyboard input or 9P commands.

#### Task 2.5: Write unit tests for command processing
Create `pkg/gb/gameboy_9p_test.go` following TDD:

**Test 1: IsPaused() returns correct state**
```go
func TestIsPaused(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()

    // Initially not paused
    assert.False(t, gb.IsPaused())

    // Pause and verify
    gb.SetPaused(true)
    assert.True(t, gb.IsPaused())

    // Resume and verify
    gb.SetPaused(false)
    assert.False(t, gb.IsPaused())
}
```

**Test 2: ProcessCommands() handles pause command**
```go
func TestProcessCommandsPause(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()

    // Send pause command
    gb.commandChan <- Command{Name: "pause"}
    gb.ProcessCommands()

    assert.True(t, gb.IsPaused())
}
```

**Test 3: ProcessCommands() handles resume command**
```go
func TestProcessCommandsResume(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()
    gb.SetPaused(true)

    // Send resume command
    gb.commandChan <- Command{Name: "resume"}
    gb.ProcessCommands()

    assert.False(t, gb.IsPaused())
}
```

**Test 4: ProcessCommands() handles multiple commands**
```go
func TestProcessCommandsMultiple(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()

    // Queue multiple commands
    gb.commandChan <- Command{Name: "pause"}
    gb.commandChan <- Command{Name: "resume"}
    gb.commandChan <- Command{Name: "pause"}

    gb.ProcessCommands()

    assert.True(t, gb.IsPaused())
}
```

**Test 5: ProcessCommands() ignores unknown commands**
```go
func TestProcessCommandsUnknown(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()

    gb.commandChan <- Command{Name: "invalid"}
    gb.ProcessCommands()

    // Should not crash, state unchanged
    assert.False(t, gb.IsPaused())
}
```

**Test 6: Thread safety of paused field access**
```go
func TestPausedThreadSafety(t *testing.T) {
    gb := &Gameboy{}
    gb.setup()

    done := make(chan bool)

    // Writer goroutine
    go func() {
        for i := 0; i < 1000; i++ {
            gb.SetPaused(i%2 == 0)
        }
        done <- true
    }()

    // Reader goroutine
    go func() {
        for i := 0; i < 1000; i++ {
            _ = gb.IsPaused()
        }
        done <- true
    }()

    // Toggle goroutine
    go func() {
        for i := 0; i < 1000; i++ {
            gb.togglePaused()
        }
        done <- true
    }()

    <-done
    <-done
    <-done
}
```

Run with: `go test -race ./pkg/gb`

---

### Phase 3: Implement pkg/ninep package

**Reference:** `9p-spec.md` lines 117-143 for ctl interface specification

#### Task 3.1: Create constants file
Create `pkg/ninep/constants.go`:
```go
// ABOUTME: Constants for 9P control interface.
// ABOUTME: Defines control command names that can be sent to the emulator.

package ninep

// Command name constants
const (
    CommandPause  = "pause"
    CommandResume = "resume"
    CommandReset  = "reset" // Not yet implemented
)
```

**Note:** The `Command` type itself is defined in `pkg/gb/gameboy.go` to avoid circular dependencies.

#### Task 3.2: Implement ctl file
Create `pkg/ninep/ctlfile.go`:

**Architecture (see "Lessons Learned" section above):**
- Implement `ctlFS` type with `fs.FS` interface (contains shared state)
- Implement `ctlFile` type with `fs.File` interface (references parent ctlFS)
- Each `Open()` creates a new `ctlFile` instance with separate read position
- All instances share state through the parent `ctlFS`

**Imports needed:**
```go
import (
    "bytes"
    "fmt"
    "io"
    "io/fs"
    "sync"
    "time"

    "github.com/Humpheh/goboy/pkg/gb"
)
```

**ctlFS structure:**
```go
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
```

**ctlFile structure:**
```go
type ctlFile struct {
    parent  *ctlFS
    readPos int
}
```

**Read format (from 9p-spec.md lines 120-129):**
```
running

pause - pause emulation
resume - resume emulation
reset - reset to power-on state (not yet implemented)
```

**Write implementation example:**
```go
func (f *ctlFile) Write(p []byte) (n int, err error) {
    command := string(bytes.TrimSpace(p))

    // Ignore empty writes (from truncate operations)
    if command == "" {
        return len(p), nil
    }

    // Send command to emulator
    switch command {
    case CommandPause, CommandResume:
        f.parent.gb.GetCommandChan() <- gb.Command{Name: command}
    case CommandReset:
        return 0, fmt.Errorf("reset not yet implemented")
    default:
        return 0, fmt.Errorf("unknown command: %s", command)
    }

    return len(p), nil
}
```

**Implementation notes:**
- Store reference to `*gb.Gameboy` in ctlFS
- Use `gb.IsPaused()` for reading status
- Use `gb.GetCommandChan()` for sending commands
- Send `gb.Command` (not `ninep.Command`) since the type is defined in pkg/gb
- Implement `fs.FileInfo` for Stat() with appropriate size, mode 0666 (writable)
- Thread-safe access via mutex in ctlFS

**TDD Approach:**
Write tests in `pkg/ninep/ctlfile_test.go` BEFORE implementing the ctlFile. Each test should fail initially, then pass after implementation.

#### Task 3.2b: Write unit tests for ctl file
Create `pkg/ninep/ctlfile_test.go`:

```go
// ABOUTME: Unit tests for control file implementation.
// ABOUTME: Tests read/write operations and command processing via ctl file.

package ninep

import (
    "io"
    "io/fs"
    "sync"
    "testing"

    "github.com/Humpheh/goboy/pkg/gb"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// testGameboy creates a minimal Gameboy instance for testing
func testGameboy() *gb.Gameboy {
    gameboy := &gb.Gameboy{}
    // Initialize only the fields needed for 9P testing
    gameboy.CommandChan = make(chan gb.Command, 10)
    return gameboy
}

func TestCtlFileRead_Running(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    buf := make([]byte, 1024)
    n, err := file.Read(buf)
    require.NoError(t, err)

    content := string(buf[:n])
    assert.Contains(t, content, "running")
    assert.Contains(t, content, "pause - pause emulation")
    assert.Contains(t, content, "resume - resume emulation")
}

func TestCtlFileRead_Paused(t *testing.T) {
    gameboy := testGameboy()
    gameboy.SetPaused(true)

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    buf := make([]byte, 1024)
    n, err := file.Read(buf)
    require.NoError(t, err)

    content := string(buf[:n])
    assert.Contains(t, content, "paused")
}

func TestCtlFileWrite_Pause(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    // Cast to writer interface
    writer, ok := file.(interface{ Write([]byte) (int, error) })
    require.True(t, ok, "ctlFile should implement Write")

    n, err := writer.Write([]byte("pause\n"))
    require.NoError(t, err)
    assert.Equal(t, 6, n)

    // Process the command
    gameboy.ProcessCommands()

    assert.True(t, gameboy.IsPaused())
}

func TestCtlFileWrite_Resume(t *testing.T) {
    gameboy := testGameboy()
    gameboy.SetPaused(true)

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    writer, ok := file.(interface{ Write([]byte) (int, error) })
    require.True(t, ok)

    n, err := writer.Write([]byte("resume"))
    require.NoError(t, err)
    assert.Equal(t, 6, n)

    gameboy.ProcessCommands()
    assert.False(t, gameboy.IsPaused())
}

func TestCtlFileWrite_EmptyWrite(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    writer, ok := file.(interface{ Write([]byte) (int, error) })
    require.True(t, ok)

    // Empty write (from truncate) should succeed
    n, err := writer.Write([]byte(""))
    require.NoError(t, err)
    assert.Equal(t, 0, n)

    // State should be unchanged
    assert.False(t, gameboy.IsPaused())
}

func TestCtlFileWrite_UnknownCommand(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    writer, ok := file.(interface{ Write([]byte) (int, error) })
    require.True(t, ok)

    _, err = writer.Write([]byte("invalid"))
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown command")
}

func TestCtlFileMultipleReads(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    // First read
    buf1 := make([]byte, 10)
    n1, err := file.Read(buf1)
    require.NoError(t, err)
    assert.Greater(t, n1, 0)

    // Second read should continue from where first left off
    buf2 := make([]byte, 100)
    n2, err := file.Read(buf2)
    if err != io.EOF {
        require.NoError(t, err)
    }

    // Concatenate reads should equal full content
    fullContent := string(buf1[:n1]) + string(buf2[:n2])
    assert.Contains(t, fullContent, "running")
}

func TestCtlFileStat(t *testing.T) {
    gameboy := testGameboy()

    ctlfs := newCtlFS(gameboy)
    file, err := ctlfs.Open(".")
    require.NoError(t, err)
    defer file.Close()

    info, err := file.Stat()
    require.NoError(t, err)

    assert.Equal(t, "ctl", info.Name())
    assert.False(t, info.IsDir())
    assert.Equal(t, fs.FileMode(0666), info.Mode().Perm())
    assert.Greater(t, info.Size(), int64(0))
}
```

Run tests with: `go test ./pkg/ninep`

#### Task 3.3: Implement 9P server
Create `pkg/ninep/server.go`:

**Reference:** `somename/dynamic-fskit-9p.go` for p9fskit usage pattern

```go
// ABOUTME: 9P server implementation for exposing emulator state over network.
// ABOUTME: Serves virtual filesystem using fskit and p9fskit wrapper.

package ninep

import (
    "fmt"
    "net"

    "github.com/hugelgupf/p9/p9"
    "github.com/Humpheh/goboy/pkg/gb"
    "github.com/Humpheh/goboy/pkg/p9fskit"
    "tractor.dev/wanix/fs/fskit"
    "tractor.dev/wanix/fs/p9kit"
)

// Start launches the 9P server on the specified port.
// Returns error if server fails to start.
func Start(gameboy *gb.Gameboy, port int) error {
    // Build virtual filesystem
    virtualFS := fskit.MapFS{
        "README": fskit.RawNode([]byte(readmeContent)),
        "ctl":    newCtlFS(gameboy), // Note: ctlFS, not ctlFile
    }

    // Wrap with p9fskit for syscall.Stat_t support
    wrappedFS := p9fskit.NewP9FS(virtualFS)

    // Create TCP listener
    addr := fmt.Sprintf(":%d", port)
    serverSocket, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("failed to listen on port %d: %w", port, err)
    }

    // Create and serve 9P server
    server := p9.NewServer(p9kit.Attacher(wrappedFS))

    // Launch in goroutine
    go func() {
        if err := server.Serve(serverSocket); err != nil {
            fmt.Printf("9P server error: %v\n", err)
        }
    }()

    return nil
}
```

#### Task 3.4: Write README content
In `server.go`, add `readmeContent` constant based on `9p-spec.md` lines 396-466, adapted for proof-of-concept:

```go
const readmeContent = `GoBoy 9P Interface (Proof-of-Concept)
========================================

This filesystem exposes the Game Boy emulator's control interface for
inspection and manipulation using standard Unix tools.

STRUCTURE
---------
/README              This file
/ctl                 Control interface (read status, write commands)

CONTROL COMMANDS
----------------
Read 'ctl' to see current status:
  cat ctl

Write commands to 'ctl':
  echo pause > ctl   - Pause emulation
  echo resume > ctl  - Resume emulation

STATUS
------
This is a proof-of-concept implementation. Future versions will include:
- Complete emulator state under /state/ directory
- Save state functionality (tar -czf savestate.tar.gz state/)
- Reset command support
- ROM loading capability

NOTES
-----
- Reads are instantaneous and reflect live state
- Writes are applied at next frame boundary (~16ms latency)
- Multiple emulator instances can run on different ports

See 9p-spec.md for the complete interface specification.
`
```

---

### Phase 4: Wire 9P server into main program

**Reference:** `cmd/goboy/main.go`

#### Task 4.1: Add command processing to startGBLoop
In `cmd/goboy/main.go`, modify `startGBLoop()` function.

After the `monitor.Render(&gameboy.PreparedData)` call, add:

```go
// Process any pending commands from 9P interface
gameboy.ProcessCommands()
```

**Note:** The `ProcessCommands()` method should already be added in Phase 2. This method drains all pending commands from the command channel and applies them to the emulator state.

#### Task 4.2: Add --9p-port flag
In `cmd/goboy/main.go`, add to the flag definitions:

```go
var (
    mute    = flag.Bool("mute", false, "mute sound output")
    dmgMode = flag.Bool("dmg", false, "set to force dmg mode")

    ninepPort = flag.Int("9p-port", 0, "enable 9P server on port (0 = disabled)")

    cpuprofile  = flag.String("cpuprofile", "", "write cpu profile to file (debugging)")
    vsyncOff    = flag.Bool("disableVsync", false, "set to disable vsync (debugging)")
    stepThrough = flag.Bool("stepthrough", false, "step through opcodes (debugging)")
    unlocked    = flag.Bool("unlocked", false, "if to unlock the cpu speed (debugging)")
)
```

#### Task 4.3: Start 9P server conditionally
In `cmd/goboy/main.go`, in the `start()` function after creating the gameboy instance:

```go
// Start 9P server if requested
if *ninepPort != 0 {
    if err := ninep.Start(gameboy, *ninepPort); err != nil {
        log.Printf("Failed to start 9P server: %v", err)
    } else {
        printNinePInstructions(*ninepPort)
    }
}
```

Add the helper function to print testing instructions:

```go
func printNinePInstructions(port int) {
    fmt.Printf("\n=== 9P Server Started on port %d ===\n\n", port)
    fmt.Println("To test from Linux VM:")
    fmt.Println()
    fmt.Println("1. Mount the filesystem:")
    fmt.Println("   mkdir -p /tmp/goboy")
    fmt.Printf("   sudo mount -t 9p -o trans=tcp,port=%d <HOST_IP> /tmp/goboy\n", port)
    fmt.Println()
    fmt.Println("2. Test reading files:")
    fmt.Println("   cat /tmp/goboy/README")
    fmt.Println("   cat /tmp/goboy/ctl")
    fmt.Println()
    fmt.Println("3. Test pause command:")
    fmt.Println("   echo pause > /tmp/goboy/ctl")
    fmt.Println("   cat /tmp/goboy/ctl    # should show 'paused'")
    fmt.Println()
    fmt.Println("4. Test resume command:")
    fmt.Println("   echo resume > /tmp/goboy/ctl")
    fmt.Println("   cat /tmp/goboy/ctl    # should show 'running'")
    fmt.Println()
    fmt.Println("Press Ctrl+C to stop the emulator when done testing.")
    fmt.Println()
}
```

Don't forget to add the import:
```go
import (
    // ... existing imports ...
    "github.com/Humpheh/goboy/pkg/ninep"
)
```

---

### Phase 5: Test the integration

#### Task 5.1: Build and run with 9P enabled
```bash
go build ./cmd/goboy
./goboy --9p-port 5640 roms/tetris.gb
```

Verify:
- Emulator starts normally
- GUI window opens
- 9P server starts without errors
- Instructions are printed to console

#### Task 5.2: Test from Linux VM
Follow the printed instructions to:
1. Mount the filesystem
2. Read README and ctl files
3. Test pause/resume commands
4. Verify emulator responds correctly

#### Task 5.3: Test edge cases
- Start emulator without `--9p-port` flag (verify normal operation)
- Try to start two emulators on same port (verify error handling)
- Write invalid commands to ctl (verify error messages)
- Connect multiple clients simultaneously

#### Task 5.4: Document results
Note any issues, unexpected behavior, or improvements needed.

---

## Implementation Checklist

### Phase 1: Dependencies
- [ ] Copy p9fskit to pkg/p9fskit
- [ ] Create pkg/ninep package structure
- [ ] Update go.mod with dependencies

### Phase 2: Gameboy Infrastructure
- [ ] Add Command type to Gameboy file
- [ ] Add Mu and CommandChan fields to Gameboy struct (exported for testing)
- [ ] Initialize CommandChan in setup()
- [ ] Add IsPaused() method
- [ ] Add SetPaused() method
- [ ] Add GetCommandChan() method (returns send-only channel)
- [ ] Add ProcessCommands() method
- [ ] Fix togglePaused() race condition (add mutex)
- [ ] Write unit tests for command processing (gameboy_9p_test.go)
  - [ ] Test IsPaused()
  - [ ] Test ProcessCommands() pause command
  - [ ] Test ProcessCommands() resume command
  - [ ] Test ProcessCommands() multiple commands
  - [ ] Test ProcessCommands() unknown commands
  - [ ] Test thread safety with race detector
- [ ] Run tests: `go test -race ./pkg/gb`

### Phase 3: ninep Package
- [ ] Create constants file with command names (constants.go)
- [ ] Write unit tests for ctl file FIRST (ctlfile_test.go) - TDD
  - [ ] Test ctlFile read when running
  - [ ] Test ctlFile read when paused
  - [ ] Test ctlFile write pause command
  - [ ] Test ctlFile write resume command
  - [ ] Test ctlFile empty write handling
  - [ ] Test ctlFile unknown command error
  - [ ] Test ctlFile multiple reads
  - [ ] Test ctlFile Stat()
- [ ] Implement ctlFS struct with Open() method (ctlfile.go)
- [ ] Implement ctlFile struct (ctlfile.go)
- [ ] Implement ctlFile.Read() with dynamic status
- [ ] Implement ctlFile.Write() with empty write handling
- [ ] Implement ctlFile.Stat() with mode 0666
- [ ] Implement ctlFile.Close()
- [ ] Implement ctlFileInfo struct
- [ ] Run tests: `go test ./pkg/ninep` (should all pass now)
- [ ] Implement Start() function (server.go)
- [ ] Add README content constant

### Phase 4: Main Program Integration
- [ ] Call gameboy.ProcessCommands() in startGBLoop()
- [ ] Add --9p-port flag
- [ ] Add printNinePInstructions() helper
- [ ] Start 9P server conditionally in start()
- [ ] Add ninep import

### Phase 5: Testing
- [ ] Build and run with 9P enabled
- [ ] Test from Linux VM
- [ ] Test edge cases
- [ ] Document results

---

## Future Expansion

After proof-of-concept is complete and working:

1. **Add state reading**
   - Implement `/state/cpu` file with register values
   - Reference: `9p-spec.md` lines 164-179

2. **Add state writing**
   - Implement command queue for state writes
   - Apply writes at frame boundaries
   - Reference: `9p-spec.md` lines 38-41

3. **Add reset command**
   - Implement proper ROM reload and state reset
   - Handle battery-backed RAM correctly

4. **Expand state exposure**
   - Add memory files (vram, wram, etc.)
   - Add cartridge state
   - Add APU and PPU state
   - Reference: `9p-spec.md` for complete structure

---

## Notes

- Keep changes minimal for proof-of-concept
- Follow existing code style in the codebase
- Add TODO comments where functionality is deferred
- Test thoroughly before expanding
- Refer to `9p-spec.md` for complete specification details
- Reference implementation: `somename/test-fskit-write.go` demonstrates the complete writable file pattern

## Architecture Decisions

### Command Type Location
The `Command` type is defined in `pkg/gb/gameboy.go` rather than `pkg/ninep` to avoid circular dependencies:
- `pkg/ninep` needs to import `pkg/gb` to access the `Gameboy` struct
- `pkg/gb` needs the `Command` type for the `commandChan` field
- Placing `Command` in `pkg/gb` breaks the circular dependency

### Command Processing Location
The `ProcessCommands()` method is defined on the `Gameboy` struct rather than as a free function in `main.go`:
- It needs access to the private `commandChan` field
- It encapsulates command processing logic with the emulator state
- It maintains proper encapsulation and prevents exposing internal channels

## Testing Environment

The proof-of-concept includes instructions for testing from a Linux VM using the 9P mount command. Alternative testing approaches:

1. **Linux VM** (recommended): Use `mount -t 9p` for native 9P filesystem support
2. **macOS with 9pfuse**: Install and use 9pfuse for FUSE-based 9P mounting
3. **Go client**: Write a simple Go program using the p9 library to connect and test operations

The implementation itself is platform-agnostic and will work with any 9P client.
