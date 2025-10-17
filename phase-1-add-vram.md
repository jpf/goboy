# Phase 1: Add state/memory/vram to 9P Interface (REVISED)

## Overview

This phase implements the first state file in the 9P filesystem: `/state/memory/vram`. Uses the fskit/p9kit pattern to simplify implementation.

## Architecture Summary

### Existing Infrastructure

- ✅ `/ctl` implemented (pause/resume)
- ✅ `/README` implemented
- ✅ 9P server running
- ✅ Command channel and ProcessCommands() at frame boundaries
- ✅ Gameboy.Mu RWMutex for pause state

### VRAM Structure (from codebase)

**Location:** `pkg/gb/memory.go`
```go
type Memory struct {
    VRAM     [0x4000]byte  // 16KB total
    VRAMBank byte          // Current bank (0 or 1)
}
```

**Properties:**
- Size: 16KB (0x4000 bytes)
- Banking: 2 banks for CGB mode (8KB each)
- Exposed as: Single flat 16KB file (both banks concatenated)
  - Bytes 0x0000-0x1FFF: Bank 0
  - Bytes 0x2000-0x3FFF: Bank 1

### Architecture Decision: Use fskit/p9kit Pattern

**Current implementation** (in `pkg/ninep/p9file.go`):
- Manually implements p9.File interface for everything
- Lots of boilerplate (Walk, GetAttr, Open, Close, etc.)
- Inconsistent: readmeFile uses p9.File directly, ctlFile uses fs.FS wrapper

**New pattern** (from `somename/test-fskit-write.go`):
- Use `fskit.MapFS` for directory structure
- Custom fs.FS implementations for dynamic files (like ctlFS)
- Wrap with `p9fskit.NewP9FS` for Sys() compatibility
- Use `p9kit.Attacher` to bridge fs.FS to p9.Attacher

**Benefits:**
- Much less boilerplate
- Standard fs.FS interface (more idiomatic Go)
- Easier to test (can test fs.FS without 9P)
- Consistent pattern for all files

## Thread Safety Approach

**Decision: Accept brief read races during VRAM reads**

**Rationale:**
- VRAM reads will use `Gameboy.Mu.RLock()` for consistency
- Main loop doesn't currently lock for PPU reads
- Adding locks to PPU would be invasive and impact performance
- Brief races during microsecond-level reads are acceptable for a debugging tool
- Write safety is guaranteed via command queue at frame boundaries

**Documentation:**
- Add comment in vramfs.go explaining race window
- Update README to note that pausing before reads eliminates races

## Implementation Plan

### 1. Add Dependencies

**File:** `go.mod`

Add:
```
tractor.dev/wanix v0.0.0-20251015063142-b2d32ec00236
```

Copy `pkg/p9fskit/wrapper.go` from production copy (already exists).

### 2. Expand Command Struct

**File:** `pkg/gb/gameboy.go`

**Current:**
```go
type Command struct {
    Name string
}
```

**Updated:**
```go
type Command struct {
    Name   string
    Offset int64  // For memory writes
    Data   []byte // For memory writes
}
```

**Update ProcessCommands():**
```go
func (gb *Gameboy) ProcessCommands() {
    for {
        select {
        case cmd := <-gb.CommandChan:
            switch cmd.Name {
            case "pause":
                gb.SetPaused(true)
            case "resume":
                gb.SetPaused(false)
            case "vram-write":
                gb.Mu.Lock()
                copy(gb.memory.VRAM[cmd.Offset:], cmd.Data)
                gb.Mu.Unlock()
            default:
                // Unknown command, ignore
            }
        default:
            return
        }
    }
}
```

### 3. Create VRAM fs.FS Implementation

**File:** `pkg/ninep/vramfs.go`

```go
package ninep

import (
    "io"
    "io/fs"
    "sync"
    "time"

    "github.com/Humpheh/goboy/pkg/gb"
)

// vramFS implements fs.FS for VRAM access
type vramFS struct {
    mu sync.RWMutex
    gb *gb.Gameboy
}

func newVramFS(gameboy *gb.Gameboy) *vramFS {
    return &vramFS{gb: gameboy}
}

func (v *vramFS) Open(name string) (fs.File, error) {
    if name != "." && name != "" {
        return nil, fs.ErrNotExist
    }
    return &vramFile{parent: v}, nil
}

// vramFile provides read/write access to VRAM
type vramFile struct {
    parent  *vramFS
    readPos int
}

func (f *vramFile) Read(p []byte) (n int, err error) {
    // Lock emulator state for consistent read
    f.parent.gb.Mu.RLock()
    defer f.parent.gb.Mu.RUnlock()

    if f.readPos >= 0x4000 {
        return 0, io.EOF
    }

    n = copy(p, f.parent.gb.memory.VRAM[f.readPos:])
    f.readPos += n
    return n, nil
}

func (f *vramFile) Write(p []byte) (n int, err error) {
    // Queue write command for frame boundary processing
    cmd := gb.Command{
        Name:   "vram-write",
        Offset: int64(f.readPos),
        Data:   make([]byte, len(p)),
    }
    copy(cmd.Data, p)

    f.parent.gb.GetCommandChan() <- cmd
    f.readPos += len(p)
    return len(p), nil
}

func (f *vramFile) Close() error {
    return nil
}

func (f *vramFile) Stat() (fs.FileInfo, error) {
    return &vramFileInfo{}, nil
}

// Implement ReadAt for offset-based reads
func (f *vramFile) ReadAt(p []byte, offset int64) (n int, err error) {
    if offset < 0 || offset > 0x4000 {
        return 0, &fs.PathError{Op: "read", Path: "vram", Err: fs.ErrInvalid}
    }

    f.parent.gb.Mu.RLock()
    defer f.parent.gb.Mu.RUnlock()

    n = copy(p, f.parent.gb.memory.VRAM[offset:])
    if n < len(p) {
        return n, io.EOF
    }
    return n, nil
}

// Implement WriteAt for offset-based writes
func (f *vramFile) WriteAt(p []byte, offset int64) (n int, err error) {
    if offset < 0 || offset > 0x4000 {
        return 0, &fs.PathError{Op: "write", Path: "vram", Err: fs.ErrInvalid}
    }

    cmd := gb.Command{
        Name:   "vram-write",
        Offset: offset,
        Data:   make([]byte, len(p)),
    }
    copy(cmd.Data, p)

    f.parent.gb.GetCommandChan() <- cmd
    return len(p), nil
}

type vramFileInfo struct{}

func (fi *vramFileInfo) Name() string       { return "vram" }
func (fi *vramFileInfo) Size() int64        { return 0x4000 }
func (fi *vramFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *vramFileInfo) ModTime() time.Time { return time.Now() }
func (fi *vramFileInfo) IsDir() bool        { return false }
func (fi *vramFileInfo) Sys() interface{}   { return nil }
```

### 4. Refactor Server to Use fskit Pattern

**File:** `pkg/ninep/server.go`

**Current approach:** Uses p9Attacher that returns custom p9.File implementations

**New approach:** Build fs.FS tree and wrap with p9kit.Attacher

```go
package ninep

import (
    "fmt"
    "net"

    "github.com/hugelgupf/p9/p9"
    "tractor.dev/wanix/fs/fskit"
    "tractor.dev/wanix/fs/p9kit"

    "github.com/Humpheh/goboy/pkg/gb"
    "github.com/Humpheh/goboy/pkg/p9fskit"
)

const readmeContent = `... (same as before) ...`

func Start(gameboy *gb.Gameboy, port int) error {
    // Build virtual filesystem tree
    virtualFS := fskit.MapFS{
        "README": fskit.RawNode([]byte(readmeContent)),
        "ctl":    newCtlFS(gameboy),
        "state": fskit.MapFS{
            "memory": fskit.MapFS{
                "vram": newVramFS(gameboy),
            },
        },
    }

    // Wrap for 9P compatibility (adds Sys() data)
    wrappedFS := p9fskit.NewP9FS(virtualFS)

    // Create TCP listener
    addr := fmt.Sprintf(":%d", port)
    serverSocket, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("failed to listen on port %d: %w", port, err)
    }

    // Create p9 server with p9kit attacher
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

### 5. Migrate ctlFile to fs.FS (Already Done)

The current `pkg/ninep/ctlfile.go` already uses fs.FS pattern. No changes needed except ensuring it works with the new server structure.

### 6. Remove Old p9file.go Implementation

**File:** `pkg/ninep/p9file.go`

Delete this file once migration is complete. The new fskit-based approach replaces all the manual p9.File implementations.

### 7. Update README Content

Add VRAM documentation to readmeContent in server.go:

```
STRUCTURE
---------
/README                  This file
/ctl                     Control interface (read status, write commands)
/state/                  Emulator state directory
  memory/
    vram                 Video RAM (16KB binary)

STATE FILES
-----------
state/memory/vram        Video RAM (16KB binary, both banks concatenated)
                         Bytes 0x0000-0x1FFF: Bank 0
                         Bytes 0x2000-0x3FFF: Bank 1 (CGB only)

EXAMPLES
--------
# Read VRAM (live, may have brief race conditions)
xxd /mnt/goboy/state/memory/vram | head

# Surgical edit (pause for consistency)
echo pause > /mnt/goboy/ctl
echo -n '\xFF\xFF' | dd of=/mnt/goboy/state/memory/vram bs=1 seek=1024 conv=notrunc
echo resume > /mnt/goboy/ctl

# Full VRAM corruption (visual test)
dd if=/dev/random of=/mnt/goboy/state/memory/vram bs=16384 count=1
```

## Testing Strategy

### Unit Tests

**File:** `pkg/ninep/vramfs_test.go`

```go
package ninep

import (
    "io"
    "testing"

    "github.com/Humpheh/goboy/pkg/gb"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestVramFS_Open(t *testing.T) {
    gameboy := &gb.Gameboy{}
    vfs := newVramFS(gameboy)

    // Open "." should succeed
    f, err := vfs.Open(".")
    require.NoError(t, err)
    require.NotNil(t, f)
    defer f.Close()

    // Open other names should fail
    _, err = vfs.Open("invalid")
    assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestVramFile_Read(t *testing.T) {
    // Create gameboy with known VRAM data
    gameboy, err := gb.New("test.gb") // Need test ROM
    require.NoError(t, err)

    // Write known pattern to VRAM
    gameboy.Mu.Lock()
    for i := 0; i < 0x4000; i++ {
        gameboy.memory.VRAM[i] = byte(i & 0xFF)
    }
    gameboy.Mu.Unlock()

    // Open and read
    vfs := newVramFS(gameboy)
    f, err := vfs.Open(".")
    require.NoError(t, err)
    defer f.Close()

    // Read first 256 bytes
    buf := make([]byte, 256)
    n, err := f.Read(buf)
    assert.NoError(t, err)
    assert.Equal(t, 256, n)

    // Verify pattern
    for i := 0; i < 256; i++ {
        assert.Equal(t, byte(i), buf[i])
    }
}

func TestVramFile_Write(t *testing.T) {
    gameboy := &gb.Gameboy{}
    gameboy.CommandChan = make(chan gb.Command, 10)

    vfs := newVramFS(gameboy)
    f, err := vfs.Open(".")
    require.NoError(t, err)
    defer f.Close()

    // Write some data
    data := []byte{0xFF, 0xFE, 0xFD, 0xFC}
    n, err := f.(interface{ Write([]byte) (int, error) }).Write(data)
    assert.NoError(t, err)
    assert.Equal(t, 4, n)

    // Verify command was queued
    select {
    case cmd := <-gameboy.CommandChan:
        assert.Equal(t, "vram-write", cmd.Name)
        assert.Equal(t, int64(0), cmd.Offset)
        assert.Equal(t, data, cmd.Data)
    default:
        t.Fatal("Expected command in channel")
    }
}

func TestVramFile_ReadAt(t *testing.T) {
    gameboy := &gb.Gameboy{}
    gameboy.Mu = sync.RWMutex{}

    // Write known pattern
    gameboy.Mu.Lock()
    for i := 0; i < 0x4000; i++ {
        gameboy.memory.VRAM[i] = byte(i & 0xFF)
    }
    gameboy.Mu.Unlock()

    vfs := newVramFS(gameboy)
    f, err := vfs.Open(".")
    require.NoError(t, err)
    defer f.Close()

    // Read at offset 0x1000
    buf := make([]byte, 16)
    n, err := f.(interface{ ReadAt([]byte, int64) (int, error) }).ReadAt(buf, 0x1000)
    assert.NoError(t, err)
    assert.Equal(t, 16, n)

    // Verify values
    for i := 0; i < 16; i++ {
        assert.Equal(t, byte((0x1000+i)&0xFF), buf[i])
    }
}

func TestVramFile_WriteAt(t *testing.T) {
    gameboy := &gb.Gameboy{}
    gameboy.CommandChan = make(chan gb.Command, 10)

    vfs := newVramFS(gameboy)
    f, err := vfs.Open(".")
    require.NoError(t, err)
    defer f.Close()

    // Write at offset 0x800
    data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
    n, err := f.(interface{ WriteAt([]byte, int64) (int, error) }).WriteAt(data, 0x800)
    assert.NoError(t, err)
    assert.Equal(t, 4, n)

    // Verify command
    select {
    case cmd := <-gameboy.CommandChan:
        assert.Equal(t, "vram-write", cmd.Name)
        assert.Equal(t, int64(0x800), cmd.Offset)
        assert.Equal(t, data, cmd.Data)
    default:
        t.Fatal("Expected command in channel")
    }
}

func TestVramFile_Stat(t *testing.T) {
    gameboy := &gb.Gameboy{}
    vfs := newVramFS(gameboy)
    f, err := vfs.Open(".")
    require.NoError(t, err)
    defer f.Close()

    info, err := f.Stat()
    require.NoError(t, err)

    assert.Equal(t, "vram", info.Name())
    assert.Equal(t, int64(0x4000), info.Size())
    assert.Equal(t, fs.FileMode(0666), info.Mode())
    assert.False(t, info.IsDir())
}
```

**File:** `pkg/gb/gameboy_test.go`

```go
func TestProcessCommands_VramWrite(t *testing.T) {
    // Create gameboy
    gb := &Gameboy{}
    gb.memory = &Memory{}
    gb.CommandChan = make(chan Command, 10)

    // Initialize VRAM to zeros
    for i := range gb.memory.VRAM {
        gb.memory.VRAM[i] = 0
    }

    // Queue vram-write command
    data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
    gb.CommandChan <- Command{
        Name:   "vram-write",
        Offset: 0x1000,
        Data:   data,
    }

    // Process commands
    gb.ProcessCommands()

    // Verify write
    assert.Equal(t, byte(0xAA), gb.memory.VRAM[0x1000])
    assert.Equal(t, byte(0xBB), gb.memory.VRAM[0x1001])
    assert.Equal(t, byte(0xCC), gb.memory.VRAM[0x1002])
    assert.Equal(t, byte(0xDD), gb.memory.VRAM[0x1003])
}
```

### Integration Tests

**Manual test with 9P mount (Linux VM required):**

```bash
# 1. Start goboy with 9P server
./goboy --9p-port 9998 sml_V1_1.gb

# 2. In Linux VM:
mkdir -p /tmp/goboy
sudo mount -t 9p -o trans=tcp,port=9998 <MAC_IP> /tmp/goboy

# 3. Test directory structure
ls -la /tmp/goboy
# Should show: README, ctl, state/

ls -la /tmp/goboy/state/memory/
# Should show: vram

# 4. Test VRAM read
xxd /tmp/goboy/state/memory/vram | head -20
stat /tmp/goboy/state/memory/vram
# Size should be 16384 bytes

# 5. Test offset write
echo pause > /tmp/goboy/ctl
echo -n '\xFF\xFF\xFF\xFF' | dd of=/tmp/goboy/state/memory/vram bs=1 seek=1024 conv=notrunc
echo resume > /tmp/goboy/ctl

# 6. Verify write
dd if=/tmp/goboy/state/memory/vram bs=1 skip=1024 count=4 | xxd
# Should show: FF FF FF FF

# 7. Test visual corruption
echo pause > /tmp/goboy/ctl
dd if=/dev/random of=/tmp/goboy/state/memory/vram bs=1024 count=4
echo resume > /tmp/goboy/ctl
# Emulator should show graphical corruption
```

## Files to Create/Modify

### New Files
- `pkg/ninep/vramfs.go` - VRAM fs.FS implementation
- `pkg/ninep/vramfs_test.go` - Unit tests
- `pkg/p9fskit/wrapper.go` - Already exists (copy from somename/)

### Modified Files
- `go.mod` - Add tractor.dev/wanix dependency
- `pkg/gb/gameboy.go` - Expand Command struct, add vram-write handler
- `pkg/ninep/server.go` - Refactor to use fskit.MapFS pattern
- `pkg/ninep/ctlfile.go` - Minor changes to work with new server structure

### Deleted Files
- `pkg/ninep/p9file.go` - Replaced by fskit pattern

## Known Limitations & Caveats

### 1. Race Conditions on Reads

**Issue:** VRAM reads use `Gameboy.Mu.RLock()` but PPU doesn't lock during rendering.

**Impact:** Brief corruption possible during reads (microseconds).

**Mitigation:**
- Pause emulator before reading for guaranteed consistency
- Document in code and README

### 2. Write Latency

**Issue:** Writes queued and applied at next frame boundary (~16ms at 60fps).

**Impact:** Changes are not instantaneous.

**Mitigation:** This is by design per 9p-spec.md

### 3. No Validation

**Issue:** Invalid VRAM data will cause graphical corruption.

**Impact:** Writing garbage corrupts emulator state.

**Mitigation:** Trust the user (per spec philosophy)

## TDD Implementation Order

1. **Add dependencies**
   - Add tractor.dev/wanix to go.mod
   - Verify p9fskit wrapper exists

2. **Test & implement Command struct expansion**
   - Write test for ProcessCommands with vram-write
   - Expand Command struct
   - Implement vram-write handler
   - Verify tests pass

3. **Test & implement vramFS**
   - Write fs.FS interface tests
   - Implement vramFS/vramFile
   - Verify all tests green

4. **Refactor server to use fskit**
   - Update server.go to build fskit.MapFS tree
   - Test with existing ctl/README files
   - Verify existing functionality works

5. **Add VRAM to filesystem tree**
   - Add vram to state/memory/ in fskit.MapFS
   - Test manually with 9P mount

6. **Manual integration testing**
   - Follow manual test procedure
   - Verify reads, writes, offset writes
   - Document any issues

## Success Criteria

- [ ] All unit tests pass
- [ ] `go test ./pkg/ninep/...` passes with `-race` flag
- [ ] Can mount 9P filesystem and navigate to `/state/memory/vram`
- [ ] Can read 16384 bytes from vram file
- [ ] Can write full file (16KB) successfully
- [ ] Can write at arbitrary offset (surgical edit)
- [ ] Writes applied at frame boundary
- [ ] No data races detected
- [ ] README updated with documentation
- [ ] Code comments explain race trade-off

## Estimated Complexity

- **Low risk:** vramFS follows proven ctlFS pattern
- **Medium risk:** Server refactoring (but test-fskit-write.go proves pattern works)
- **Low risk:** Command expansion (straightforward)

**Benefits over original plan:**
- ~50% less boilerplate code
- More idiomatic Go (fs.FS instead of p9.File)
- Easier to test
- Consistent pattern for future files

Total estimated effort: ~3-4 hours (reduced from 4-6 hours)
