# Phase 2: Complete state/memory Implementation (Refactored)

## Overview

Phase 2 completes the `state/memory/` directory by adding the remaining memory files per the 9p-spec.md. This builds on Phase 1 but **eliminates code duplication** through generic implementations.

**Files to implement:**
- `state/memory/wram` (36KB binary) - Work RAM, 8 banks including unused gap
- `state/memory/oam` (256 bytes binary) - Object Attribute Memory (sprite data)
- `state/memory/highram` (256 bytes binary) - High RAM including hardware registers (0xFF00-0xFFFF)
- `state/memory/state` (4 bytes binary) - Memory banking state (VRAMBank, WRAMBank, hdmaLength, hdmaActive)

**Already implemented in Phase 1:**
- `state/memory/vram` (16KB binary) ✓

## Architecture (Refactored)

**Key improvement:** Instead of creating separate `wramfs`, `oamfs`, `highramfs` implementations (745+ lines of duplicated code), we use a **generic `binaryMemoryFS`** that works for all binary memory regions.

### Layers

1. **Business logic layer**: Generic `binaryMemoryFS`
   - Single fs.FS implementation for all fixed-size byte arrays
   - Configured via function pointers (getMemory, size, commandName)
   - Validates before queuing write commands

2. **Protocol layer**: Generic `p9BinaryFile`
   - Single p9.File wrapper for all binary memory files
   - Includes all required templatefs mixins

3. **Directory structure**: Update `statedirs.go`
   - Create configured instances for wram/oam/highram/state
   - Add entries to memoryDir Walk() and Readdir()

### Thread Safety & Command Queue

- **Reads:** Use RLock, accept brief race conditions (document this)
- **Writes:** Validate THEN queue command to CommandChan (buffer size 32)
- **Processing:** All commands applied at frame boundary under lock
- **Consistency:** Users must pause emulator for atomic snapshots

**Save/Restore via tar works perfectly:**
1. Pause emulator
2. Extract all files (queues 5 commands with full data copied)
3. Resume emulator → all commands drain at next frame boundary
4. Full state restored atomically in ~16ms

## Memory Layout Analysis

From `pkg/gb/memory.go`:

### WRAM [0x9000]byte (36KB)
```
Bank 0 (fixed):     0x0000-0x0FFF (4KB) - maps to GB address 0xC000-0xCFFF
Unused gap:         0x1000-0x1FFF (4KB) - implementation quirk, expose as-is
Bank 1 (switch):    0x2000-0x2FFF (4KB) - maps to GB address 0xD000-0xDFFF
Bank 2 (switch):    0x3000-0x3FFF (4KB)
Bank 3 (switch):    0x4000-0x4FFF (4KB)
Bank 4 (switch):    0x5000-0x5FFF (4KB)
Bank 5 (switch):    0x6000-0x6FFF (4KB)
Bank 6 (switch):    0x7000-0x7FFF (4KB)
Bank 7 (switch):    0x8000-0x8FFF (4KB)
```
**Decision:** Expose all 36KB including the gap for simplicity.

### OAM [0x100]byte (256 bytes)
- Object Attribute Memory for sprites
- GB address space: 0xFE00-0xFEA0 (160 bytes actively used)
- Full array is 256 bytes, expose all

### HighRAM [0x100]byte (256 bytes)
- Maps to GB address 0xFF00-0xFFFF
- Contains all hardware registers AND HRAM (0xFF80-0xFFFE)
- Includes timer registers, sound registers, PPU registers, etc.
- **WARNING:** These registers change every CPU cycle
- **Thread safety:** Accept brief races during reads (same as VRAM)

### Memory State (4 bytes binary)
From memory.go:
- `VRAMBank` (byte) - exported, offset 0
- `WRAMBank` (byte) - exported, offset 1
- `hdmaLength` (byte) - unexported, offset 2
- `hdmaActive` (byte) - unexported, offset 3 (0=false, 1=true)

**Binary format:**
```
Offset 0: VRAMBank    (1 byte, valid: 0-1)
Offset 1: WRAMBank    (1 byte, valid: 0-7)
Offset 2: hdmaLength  (1 byte)
Offset 3: hdmaActive  (1 byte, 0=false, 1=true)
```

**Validation rules:**
- VRAMBank must be <= 1
- WRAMBank must be <= 7
- hdmaActive must be 0 or 1
- Validation happens BEFORE queuing command

## Implementation Steps (Strict TDD)

**TDD Discipline:** For each step, write the failing test FIRST, run it, THEN implement minimal code to pass, THEN refactor.

### Step 1: Increase CommandChan Buffer Size

**File: pkg/gb/gameboy.go**

**Test first:** `pkg/gb/gameboy_test.go`
```go
func TestCommandChan_BufferSize(t *testing.T) {
    gb := NewGameboy(...)
    // Verify buffer size is 32 by sending 32 commands without blocking
    for i := 0; i < 32; i++ {
        select {
        case gb.CommandChan <- Command{Name: "test"}:
            // Success
        default:
            t.Fatalf("CommandChan blocked at %d commands, expected buffer of 32", i)
        }
    }
}
```

Run test (should fail with buffer size 10).

**Implementation:**
```go
gb.CommandChan = make(chan Command, 32)
```

Run test (should pass).

### Step 2: Add Helper Methods (TDD)

**File: pkg/gb/gameboy.go**

**Tests first:** `pkg/gb/gameboy_test.go`
```go
func TestGetWRAM(t *testing.T) {
    gb := NewGameboy(...)
    wram := gb.GetWRAM()
    if wram == nil {
        t.Fatal("GetWRAM returned nil")
    }
    if len(wram) != 0x9000 {
        t.Fatalf("GetWRAM size = %d, want 0x9000", len(wram))
    }
}

func TestGetOAM(t *testing.T) { /* similar */ }
func TestGetHighRAM(t *testing.T) { /* similar */ }

func TestGetMemoryState(t *testing.T) {
    gb := NewGameboy(...)
    // Set known values
    gb.memory.VRAMBank = 1
    gb.memory.WRAMBank = 3
    gb.memory.hdmaLength = 0x42
    gb.memory.hdmaActive = true

    vram, wram, hdma, active := gb.GetMemoryState()
    if vram != 1 || wram != 3 || hdma != 0x42 || active != true {
        t.Fatalf("GetMemoryState = (%d, %d, %d, %v), want (1, 3, 0x42, true)",
            vram, wram, hdma, active)
    }
}
```

Run tests (should fail - methods don't exist).

**Implementation:**
```go
// GetWRAM returns pointer to WRAM array for 9P access
func (gb *Gameboy) GetWRAM() *[0x9000]byte {
    return &gb.memory.WRAM
}

// GetOAM returns pointer to OAM array for 9P access
func (gb *Gameboy) GetOAM() *[0x100]byte {
    return &gb.memory.OAM
}

// GetHighRAM returns pointer to HighRAM array for 9P access
func (gb *Gameboy) GetHighRAM() *[0x100]byte {
    return &gb.memory.HighRAM
}

// GetMemoryState returns memory banking and DMA state for 9P access
func (gb *Gameboy) GetMemoryState() (vramBank, wramBank, hdmaLength byte, hdmaActive bool) {
    gb.Mu.RLock()
    defer gb.Mu.RUnlock()
    return gb.memory.VRAMBank, gb.memory.WRAMBank, gb.memory.hdmaLength, gb.memory.hdmaActive
}
```

Run tests (should pass).

### Step 3: Implement Command Handlers (TDD)

**File: pkg/gb/gameboy.go - Expand ProcessCommands()**

**Tests first:** `pkg/gb/gameboy_9p_test.go`
```go
func TestProcessCommands_WramWrite(t *testing.T) {
    gb := setupTestGameboy(t)

    // Set initial values
    gb.memory.WRAM[0x100] = 0x00
    gb.memory.WRAM[0x101] = 0x00

    // Queue write command
    gb.CommandChan <- gb.Command{
        Name:   "wram-write",
        Offset: 0x100,
        Data:   []byte{0xAB, 0xCD},
    }

    // Process commands
    gb.ProcessCommands()

    // Verify write applied
    if gb.memory.WRAM[0x100] != 0xAB || gb.memory.WRAM[0x101] != 0xCD {
        t.Fatalf("WRAM[0x100:0x102] = %02x %02x, want AB CD",
            gb.memory.WRAM[0x100], gb.memory.WRAM[0x101])
    }
}

func TestProcessCommands_OamWrite(t *testing.T) { /* similar pattern */ }
func TestProcessCommands_HighramWrite(t *testing.T) { /* similar pattern */ }

func TestProcessCommands_MemoryStateWrite(t *testing.T) {
    gb := setupTestGameboy(t)

    // Queue memorystate write (binary: vram=1, wram=3, hdma=0x42, active=1)
    gb.CommandChan <- gb.Command{
        Name:   "memorystate-write",
        Offset: 0,
        Data:   []byte{0x01, 0x03, 0x42, 0x01},
    }

    gb.ProcessCommands()

    // Verify state updated
    if gb.memory.VRAMBank != 1 || gb.memory.WRAMBank != 3 ||
       gb.memory.hdmaLength != 0x42 || gb.memory.hdmaActive != true {
        t.Fatalf("Memory state = (%d, %d, %d, %v), want (1, 3, 0x42, true)",
            gb.memory.VRAMBank, gb.memory.WRAMBank,
            gb.memory.hdmaLength, gb.memory.hdmaActive)
    }
}
```

Run tests (should fail - case statements don't exist).

**Implementation:**
```go
// In ProcessCommands() switch statement, add:
case "wram-write":
    gb.Mu.Lock()
    copy(gb.memory.WRAM[cmd.Offset:], cmd.Data)
    gb.Mu.Unlock()

case "oam-write":
    gb.Mu.Lock()
    copy(gb.memory.OAM[cmd.Offset:], cmd.Data)
    gb.Mu.Unlock()

case "highram-write":
    gb.Mu.Lock()
    copy(gb.memory.HighRAM[cmd.Offset:], cmd.Data)
    gb.Mu.Unlock()

case "memorystate-write":
    // Decode 4-byte binary format
    if len(cmd.Data) != 4 {
        // Silently ignore malformed commands
        break
    }
    gb.Mu.Lock()
    gb.memory.VRAMBank = cmd.Data[0]
    gb.memory.WRAMBank = cmd.Data[1]
    gb.memory.hdmaLength = cmd.Data[2]
    gb.memory.hdmaActive = cmd.Data[3] != 0
    gb.Mu.Unlock()
```

Run tests (should pass).

### Step 4: Implement Generic binaryMemoryFS (TDD)

**File: pkg/ninep/binarymemoryfs.go**

**Tests first:** `pkg/ninep/binarymemoryfs_test.go`

Write comprehensive tests covering:
- `TestBinaryMemoryFS_Open` - verify file opens successfully
- `TestBinaryMemoryFile_Read` - sequential reads
- `TestBinaryMemoryFile_ReadEOF` - read past end
- `TestBinaryMemoryFile_ReadAt` - random access reads
- `TestBinaryMemoryFile_ReadAt_OutOfBounds` - bounds checking
- `TestBinaryMemoryFile_Write` - sequential writes queue commands
- `TestBinaryMemoryFile_WriteAt` - random access writes queue commands
- `TestBinaryMemoryFile_WriteAt_OutOfBounds` - reject out of bounds
- `TestBinaryMemoryFile_Stat` - correct size and mode
- `TestBinaryMemoryFile_MultipleReads` - read position tracking
- `TestBinaryMemoryFile_Validation_VRAMBank` - reject VRAMBank > 1
- `TestBinaryMemoryFile_Validation_WRAMBank` - reject WRAMBank > 7
- `TestBinaryMemoryFile_Validation_HdmaActive` - reject > 1

Model tests after `vramfs_test.go` but make them generic.

Run tests (should fail - binarymemoryfs.go doesn't exist).

**Implementation:** `pkg/ninep/binarymemoryfs.go`
```go
// ABOUTME: Generic fs.FS implementation for binary memory regions.
// ABOUTME: Provides read/write interface to any fixed-size byte array with validation.

package ninep

import (
    "errors"
    "io"
    "io/fs"
    "sync"
    "time"

    "github.com/tophertimzen/goboy/pkg/gb"
)

type binaryMemoryFS struct {
    mu          sync.RWMutex
    gb          *gb.Gameboy
    getMemory   func(*gb.Gameboy) []byte  // Returns slice of memory region
    size        int64
    commandName string
    validator   func(offset int64, data []byte) error  // Optional validation
}

type binaryMemoryFile struct {
    parent  *binaryMemoryFS
    readPos int64
}

func (fsys *binaryMemoryFS) Open(name string) (fs.File, error) {
    if name != "." && name != "" {
        return nil, fs.ErrNotExist
    }
    return &binaryMemoryFile{parent: fsys, readPos: 0}, nil
}

func (f *binaryMemoryFile) Stat() (fs.FileInfo, error) {
    return &binaryMemoryFileInfo{
        name: "memory",
        size: f.parent.size,
    }, nil
}

func (f *binaryMemoryFile) Read(buf []byte) (int, error) {
    n, err := f.ReadAt(buf, f.readPos)
    f.readPos += int64(n)
    return n, err
}

func (f *binaryMemoryFile) ReadAt(buf []byte, offset int64) (int, error) {
    if offset < 0 {
        return 0, errors.New("negative offset")
    }
    if offset >= f.parent.size {
        return 0, io.EOF
    }

    f.parent.mu.RLock()
    mem := f.parent.getMemory(f.parent.gb)
    f.parent.gb.Mu.RLock()
    n := copy(buf, mem[offset:])
    f.parent.gb.Mu.RUnlock()
    f.parent.mu.RUnlock()

    if n == 0 && len(buf) > 0 {
        return 0, io.EOF
    }
    return n, nil
}

func (f *binaryMemoryFile) Write(data []byte) (int, error) {
    n, err := f.WriteAt(data, f.readPos)
    f.readPos += int64(n)
    return n, err
}

func (f *binaryMemoryFile) WriteAt(data []byte, offset int64) (int, error) {
    if offset < 0 {
        return 0, errors.New("negative offset")
    }
    if offset >= f.parent.size {
        return 0, errors.New("offset beyond file size")
    }
    if offset+int64(len(data)) > f.parent.size {
        return 0, errors.New("write beyond file size")
    }

    // Validate before queuing (if validator provided)
    if f.parent.validator != nil {
        if err := f.parent.validator(offset, data); err != nil {
            return 0, err
        }
    }

    // Queue write command
    dataCopy := make([]byte, len(data))
    copy(dataCopy, data)

    f.parent.gb.CommandChan <- gb.Command{
        Name:   f.parent.commandName,
        Offset: int(offset),
        Data:   dataCopy,
    }

    return len(data), nil
}

func (f *binaryMemoryFile) Close() error {
    return nil
}

type binaryMemoryFileInfo struct {
    name string
    size int64
}

func (fi *binaryMemoryFileInfo) Name() string       { return fi.name }
func (fi *binaryMemoryFileInfo) Size() int64        { return fi.size }
func (fi *binaryMemoryFileInfo) Mode() fs.FileMode  { return 0666 }
func (fi *binaryMemoryFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *binaryMemoryFileInfo) IsDir() bool        { return false }
func (fi *binaryMemoryFileInfo) Sys() interface{}   { return nil }
```

Run tests (should pass).

### Step 5: Implement Generic p9BinaryFile (TDD)

**File: pkg/ninep/p9binaryfile.go**

**Tests:** Most testing happens at fs.FS layer. p9BinaryFile is a thin wrapper, so basic smoke tests suffice. Can add if needed.

**Implementation:** Model after `p9vramfile.go` but make generic.

```go
// ABOUTME: Generic p9.File wrapper for binary memory regions.
// ABOUTME: Delegates to binaryMemoryFS for all operations.

package ninep

import (
    "io/fs"
    "syscall"

    "github.com/hugelgupf/p9/p9"
    "github.com/hugelgupf/p9/fsimpl/templatefs"
    "github.com/tophertimzen/goboy/pkg/gb"
)

type p9BinaryFile struct {
    statfs
    p9.DefaultWalkGetAttr
    templatefs.NilCloser
    templatefs.NotDirectoryFile
    templatefs.NotSymlinkFile
    templatefs.NoopRenamed
    templatefs.XattrUnimplemented
    templatefs.NotLockable

    qid     p9.QID
    gameboy *gb.Gameboy
    fsys    *binaryMemoryFS
    file    fs.File
    opened  bool
}

func newP9BinaryFile(gb *gb.Gameboy, qid p9.QID, fsys *binaryMemoryFS) *p9BinaryFile {
    return &p9BinaryFile{
        qid:     qid,
        gameboy: gb,
        fsys:    fsys,
    }
}

func (f *p9BinaryFile) Walk(names []string) ([]p9.QID, p9.File, error) {
    if len(names) == 0 {
        return []p9.QID{f.qid}, f, nil
    }
    return nil, nil, syscall.ENOTDIR
}

func (f *p9BinaryFile) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
    if f.opened {
        return p9.QID{}, 0, syscall.EINVAL
    }

    file, err := f.fsys.Open(".")
    if err != nil {
        return p9.QID{}, 0, err
    }

    f.file = file
    f.opened = true
    return f.qid, 8192, nil
}

func (f *p9BinaryFile) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
    return f.qid, p9.AttrMask{}, p9.Attr{
        Mode:   p9.ModeRegular | 0666,
        Size:   uint64(f.fsys.size),
        NLink:  1,
    }, nil
}

func (f *p9BinaryFile) ReadAt(p []byte, offset int64) (int, error) {
    if !f.opened {
        return 0, syscall.EINVAL
    }

    reader, ok := f.file.(interface {
        ReadAt([]byte, int64) (int, error)
    })
    if !ok {
        return 0, syscall.ENOSYS
    }

    return reader.ReadAt(p, offset)
}

func (f *p9BinaryFile) WriteAt(p []byte, offset int64) (int, error) {
    if !f.opened {
        return 0, syscall.EINVAL
    }

    writer, ok := f.file.(interface {
        WriteAt([]byte, int64) (int, error)
    })
    if !ok {
        return 0, syscall.ENOSYS
    }

    return writer.WriteAt(p, offset)
}

func (f *p9BinaryFile) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
    // Allow truncate to 0 (common with shell redirects)
    if valid.Size && attr.Size == 0 {
        return nil
    }
    return syscall.EPERM
}

func (f *p9BinaryFile) FSync() error {
    return nil
}

func (f *p9BinaryFile) Remove() error {
    return syscall.EPERM
}

func (f *p9BinaryFile) Rename(dir p9.File, name string) error {
    return syscall.EPERM
}

func (f *p9BinaryFile) Close() error {
    if f.opened && f.file != nil {
        return f.file.Close()
    }
    return nil
}
```

### Step 6: Create Configured Instances in statedirs.go

**File: pkg/ninep/statedirs.go**

Create helper functions that return configured binaryMemoryFS instances:

```go
func newWramFS(gb *gb.Gameboy) *binaryMemoryFS {
    return &binaryMemoryFS{
        gb:          gb,
        getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetWRAM()[:] },
        size:        0x9000,
        commandName: "wram-write",
    }
}

func newOamFS(gb *gb.Gameboy) *binaryMemoryFS {
    return &binaryMemoryFS{
        gb:          gb,
        getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetOAM()[:] },
        size:        0x100,
        commandName: "oam-write",
    }
}

func newHighramFS(gb *gb.Gameboy) *binaryMemoryFS {
    return &binaryMemoryFS{
        gb:          gb,
        getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetHighRAM()[:] },
        size:        0x100,
        commandName: "highram-write",
    }
}

func newMemoryStateFS(gb *gb.Gameboy) *binaryMemoryFS {
    return &binaryMemoryFS{
        gb:   gb,
        getMemory: func(gb *gb.Gameboy) []byte {
            vram, wram, hdma, active := gb.GetMemoryState()
            activeByte := byte(0)
            if active {
                activeByte = 1
            }
            return []byte{vram, wram, hdma, activeByte}
        },
        size:        4,
        commandName: "memorystate-write",
        validator: func(offset int64, data []byte) error {
            // Validate memory state writes
            if len(data) != 4 {
                return errors.New("memorystate must be exactly 4 bytes")
            }
            if data[0] > 1 {
                return errors.New("VRAMBank must be 0 or 1")
            }
            if data[1] > 7 {
                return errors.New("WRAMBank must be 0-7")
            }
            if data[3] > 1 {
                return errors.New("hdmaActive must be 0 or 1")
            }
            return nil
        },
    }
}
```

**Update memoryDir.Walk():**
```go
switch names[0] {
case "vram":
    qid := d.attacher.qids.Get(p9.TypeRegular)
    return []p9.QID{qid}, newP9VramFile(d.attacher.gameboy, qid), nil
case "wram":
    qid := d.attacher.qids.Get(p9.TypeRegular)
    return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, newWramFS(d.attacher.gameboy)), nil
case "oam":
    qid := d.attacher.qids.Get(p9.TypeRegular)
    return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, newOamFS(d.attacher.gameboy)), nil
case "highram":
    qid := d.attacher.qids.Get(p9.TypeRegular)
    return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, newHighramFS(d.attacher.gameboy)), nil
case "state":
    qid := d.attacher.qids.Get(p9.TypeRegular)
    return []p9.QID{qid}, newP9BinaryFile(d.attacher.gameboy, qid, newMemoryStateFS(d.attacher.gameboy)), nil
default:
    return nil, nil, syscall.ENOENT
}
```

**Update memoryDir.Readdir():**
```go
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
```

### Step 7: Update README Content

**File: pkg/ninep/server.go**

Update `readmeContent` constant:

```
STRUCTURE
---------
/README                  This file
/ctl                     Control interface (read status, write commands)
/state/                  Emulator state directory
  memory/
    vram                 Video RAM (16KB binary, both banks)
    wram                 Work RAM (36KB binary, 8 banks + gap)
    oam                  Object Attribute Memory (256B binary)
    highram              High RAM + hardware registers (256B binary)
    state                Memory banking state (4B binary)

STATE FILES
-----------
state/memory/vram        Video RAM (16KB binary, both banks concatenated)
                         Bytes 0x0000-0x1FFF: Bank 0
                         Bytes 0x2000-0x3FFF: Bank 1 (CGB only)

state/memory/wram        Work RAM (36KB binary, 8 banks)
                         Bytes 0x0000-0x0FFF: Bank 0 (fixed)
                         Bytes 0x1000-0x1FFF: Unused gap (implementation detail)
                         Bytes 0x2000-0x2FFF: Bank 1 (switchable)
                         Bytes 0x3000-0x3FFF: Bank 2
                         Bytes 0x4000-0x4FFF: Bank 3
                         Bytes 0x5000-0x5FFF: Bank 4
                         Bytes 0x6000-0x6FFF: Bank 5
                         Bytes 0x7000-0x7FFF: Bank 6
                         Bytes 0x8000-0x8FFF: Bank 7

state/memory/oam         Object Attribute Memory (256 bytes binary)
                         Sprite data at 0xFE00-0xFE9F (160 bytes used)

state/memory/highram     High RAM + hardware registers (256 bytes binary)
                         Maps to GB address 0xFF00-0xFFFF
                         Includes timer, sound, PPU, and other I/O registers
                         WARNING: Registers change every CPU cycle

state/memory/state       Memory banking state (4 bytes binary)
                         Offset 0: VRAMBank (0-1)
                         Offset 1: WRAMBank (0-7)
                         Offset 2: hdmaLength (0-255)
                         Offset 3: hdmaActive (0=false, 1=true)

EXAMPLES
--------
# Save complete memory state
echo pause > /mnt/goboy/ctl
tar -czf state.tar.gz -C /mnt/goboy/state memory/
echo resume > /mnt/goboy/ctl

# Restore memory state
echo pause > /mnt/goboy/ctl
tar -xzf state.tar.gz -C /mnt/goboy/state
echo resume > /mnt/goboy/ctl

# Read memory banking state (4 bytes binary)
xxd -p /mnt/goboy/state/memory/state
# Output: 00010000 = vram:0, wram:1, hdma:0, active:0

# Change WRAM bank to 3
echo -n '\x00\x03\x00\x00' > /mnt/goboy/state/memory/state

# Modify sprite data
echo pause > /mnt/goboy/ctl
echo -n '\x10\x20\x30\x40' | dd of=/mnt/goboy/state/memory/oam bs=1 seek=0 conv=notrunc
echo resume > /mnt/goboy/ctl

# Dump all memory regions
xxd /mnt/goboy/state/memory/wram > wram.hex
xxd /mnt/goboy/state/memory/oam > oam.hex
xxd /mnt/goboy/state/memory/highram > highram.hex

NOTES
-----
- Reads are instantaneous and reflect live state
- Brief race conditions possible during reads (pause for consistency)
- HighRAM contains hardware registers that change every CPU cycle
- Writes are validated THEN queued (applied at next frame boundary ~16ms)
- Invalid writes return errors: VRAMBank>1, WRAMBank>7, hdmaActive>1
- Pause emulator for atomic save/restore operations
- Multiple emulator instances can run on different ports
```

### Step 8: Run All Tests

```bash
# Run with race detector
go test -race ./pkg/ninep/...
go test -race ./pkg/gb/...

# Build to verify no compilation errors
go build ./cmd/goboy/...
```

### Step 9: Manual Integration Testing

Test with running emulator:

```bash
# Start emulator with 9P server
./goboy --9p-port 5640 sml_V1_1.gb

# In another terminal, mount
diod -n -e 'localhost:5640' /mnt/goboy

# Test WRAM
xxd /mnt/goboy/state/memory/wram | head
echo pause > /mnt/goboy/ctl
echo -n '\xFF\xFF\xFF\xFF' | dd of=/mnt/goboy/state/memory/wram bs=1 seek=0x100 conv=notrunc
echo resume > /mnt/goboy/ctl

# Test OAM
xxd /mnt/goboy/state/memory/oam

# Test HighRAM
xxd /mnt/goboy/state/memory/highram | head

# Test memory state (binary format)
xxd -p /mnt/goboy/state/memory/state
echo -n '\x00\x03\x00\x00' > /mnt/goboy/state/memory/state
xxd -p /mnt/goboy/state/memory/state  # Verify change

# Test validation
echo -n '\x02\x00\x00\x00' > /mnt/goboy/state/memory/state  # Should fail (VRAMBank > 1)
echo -n '\x00\x08\x00\x00' > /mnt/goboy/state/memory/state  # Should fail (WRAMBank > 7)

# Test save/restore
echo pause > /mnt/goboy/ctl
tar -czf state.tar.gz -C /mnt/goboy/state memory/
# Modify some memory
tar -xzf state.tar.gz -C /mnt/goboy/state  # Restore
echo resume > /mnt/goboy/ctl
```

## Files to Create/Modify

### New Files (3 total, down from 12)
- `pkg/ninep/binarymemoryfs.go` (~150 lines)
- `pkg/ninep/binarymemoryfs_test.go` (~450 lines comprehensive tests)
- `pkg/ninep/p9binaryfile.go` (~140 lines)

### Modified Files (4 total)
- `pkg/gb/gameboy.go` - Increase CommandChan buffer, add helper methods, add 4 command handlers (~60 lines added)
- `pkg/gb/gameboy_test.go` - Add helper method tests (~80 lines added)
- `pkg/gb/gameboy_9p_test.go` - Add command handler tests (~120 lines added)
- `pkg/ninep/statedirs.go` - Add 4 constructor functions, update Walk()/Readdir() (~100 lines added)
- `pkg/ninep/server.go` - Update README content (~100 lines modified)

**Total new/modified code: ~1200 lines (down from ~2000+ lines)**

## Success Criteria

1. All tests pass with race detector enabled
2. Build succeeds with no compilation errors
3. Manual testing verifies:
   - Can read all memory files via 9P
   - Writes queue commands correctly
   - Validation rejects invalid bank values
   - Binary memory state format works
   - Save/restore via tar works atomically
   - Directory listings show all 5 files in state/memory/
4. README accurately documents all memory files

## Estimated Effort

**Refactored approach with strict TDD:**
- CommandChan buffer + tests: 20 min
- Helper methods + tests: 30 min
- Command handlers + tests: 45 min
- Generic binaryMemoryFS + comprehensive tests: 2 hours
- Generic p9BinaryFile: 45 min
- Configured instances + directory updates: 30 min
- README updates: 20 min
- Manual integration testing: 1 hour
- Buffer for debugging: 2 hours

**Total: ~8 hours** (with TDD discipline and proper testing)

## Key Architecture Decisions

1. **Generic over specific** - Single binaryMemoryFS eliminates ~700 lines of duplication
2. **Binary over text** - Memory state is 4 bytes (vram, wram, hdma, active) for consistency
3. **Validate before queue** - Errors returned synchronously to 9P client
4. **Accept races on reads** - Document limitation, users pause for consistency
5. **Command queue for all writes** - No exceptions, consistent architecture
6. **Expose WRAM gap** - Simple, honest, no offset translation needed
7. **Buffer size 32** - Supports tar extraction + headroom

## Notes & Lessons from Phase 1

1. **p9kit doesn't work** - use hybrid fs.FS + p9.File approach ✓
2. **Required mixins** - MUST include XattrUnimplemented and NotLockable ✓
3. **Helper methods pattern** - Add GetXXX() methods to Gameboy for clean access ✓
4. **Command queue pattern** - All writes via CommandChan, processed at frame boundaries ✓
5. **Thread safety** - RLock for reads, command queue for writes ✓
6. **Test comprehensively** - Cover all operations, bounds checking, validation ✓
7. **Race detector** - Always run tests with `-race` flag ✓
8. **NEW: Don't duplicate code** - Use generics/configuration when patterns repeat ✓
9. **NEW: TDD discipline** - Write failing test FIRST, implement, refactor ✓
10. **NEW: Validate early** - Check inputs before queuing async commands ✓

## Future Phases

After Phase 2, `state/memory/` will be complete. Future phases can reuse binaryMemoryFS pattern:
- Phase 3: `state/cpu` (registers - could use binaryMemoryFS!)
- Phase 4: `state/cartridge/` (info, state, ram)
- Phase 5: `state/apu/` (sound state)
- Phase 6: `state/ppu/` (PPU state, screen buffer)
- Phase 7: Control commands (rom loading, reset)
