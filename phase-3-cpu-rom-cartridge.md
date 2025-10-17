# Phase 3: CPU, ROM, and Cartridge State Implementation

## Overview

Phase 3 adds CPU state, ROM loading, and cartridge state/RAM to the 9P filesystem interface. This completes the core save state functionality, enabling users to save and restore complete gameplay state including CPU registers, program execution position, cartridge banking state, and game save data.

After this phase, users will have functional save states for most games. The remaining APU/PPU state (Phase 4) is needed only for perfect audio/video reproduction and advanced debugging.

## Goals

1. **CPU State Access**: Read/write all CPU registers and timer state
2. **ROM Loading**: Load new ROMs via filesystem writes
3. **Cartridge Metadata**: Read ROM header information
4. **Banking State**: Read/write MBC banking state (varies by cartridge type)
5. **Cartridge RAM**: Read/write game save data with offset-based access

## File Specifications

### `/rom` (Write-Only)

Write-only file for loading new ROMs:
```bash
cat ~/roms/tetris.gb > /mnt/goboy/rom
```

**Behavior:**
- Accepts full ROM data in a single write or multiple sequential writes
- Validates ROM size (minimum 32KB, power of 2, maximum reasonable size)
- On successful write:
  - Loads ROM into emulator
  - Resets emulator to power-on state
  - Loads corresponding .sav file if it exists
- Returns error immediately if ROM is invalid

**Implementation Notes:**
- No read support (read returns error)
- No offset-based writes (must write from beginning)
- Command: `rom-load` with full ROM data

### `state/cpu` (Text Format)

CPU registers and program state:
```
# Registers
AF=0x01B0
BC=0x0013
DE=0xD8A0
HL=0x014D

# Program State
SP=0xFFFE
PC=0x0100

# Timer
Divider=0x00AB
```

**Fields:**
- `AF`, `BC`, `DE`, `HL`: 16-bit register pairs (hex)
- `SP`: Stack pointer (hex)
- `PC`: Program counter (hex)
- `Divider`: Timer divider register (hex)

**Implementation Notes:**
- Reuse `memoryStateFS` pattern from Phase 2
- No validation (trust user per spec philosophy)
- Add `GetCPUState()` helper to Gameboy (no internal locking)
- Add `cpu-write` command handler

### `state/cartridge/info` (Read-Only Text)

ROM header metadata:
```
# ROM Information
title=POKEMON BLUE
type=MBC3
romSize=1048576
ramSize=32768

# Features
cgbSupport=0x00
sgbSupport=0x00
hasBattery=0x01

# Checksums
headerChecksum=0x3C
globalChecksum=0xB0A2
```

**Fields:**
- `title`: Game title from ROM header (ASCII string)
- `type`: Cartridge type (ROM, MBC1, MBC2, MBC3, MBC5)
- `romSize`: ROM size in bytes (decimal)
- `ramSize`: RAM size in bytes (decimal, 0 if none)
- `cgbSupport`: CGB compatibility flag (hex)
- `sgbSupport`: SGB compatibility flag (hex)
- `hasBattery`: Battery-backed RAM flag (hex, 0x00 or 0x01)
- `headerChecksum`: Header checksum (hex)
- `globalChecksum`: Global ROM checksum (hex)

**Implementation Notes:**
- Read-only (writes return error)
- Generated dynamically from loaded ROM
- Parse ROM header bytes at 0x0100-0x014F

### `state/cartridge/state` (Text Format, MBC-Dependent)

Cartridge banking and RTC state. Format varies by MBC type:

**Plain ROM (no banking):**
```
# No banking state
```

**MBC1:**
```
# Banking
romBank=0x01
ramBank=0x00
ramEnabled=0x01
romBanking=0x00
```

**MBC2:**
```
# Banking
romBank=0x01
ramBank=0x00
ramEnabled=0x01
```

**MBC3:**
```
# Banking
romBank=0x01
ramBank=0x00
ramEnabled=0x01

# RTC Registers
rtcSeconds=0x00
rtcMinutes=0x00
rtcHours=0x00
rtcDays=0x00
rtcControl=0x00

# RTC Latched
latchedSeconds=0x00
latchedMinutes=0x00
latchedHours=0x00
latchedDays=0x00
latchedControl=0x00
latched=0x00

# RTC Real-Time Reference
rtcTimestamp=0x0000000000000000
```

**MBC5:**
```
# Banking
romBank=0x0001
ramBank=0x00
ramEnabled=0x01
```

**Implementation Notes:**
- Use runtime type detection to determine MBC type
- Type assertion or type switch on `BankingController` interface
- Validation: Range check banking values against ROM/RAM sizes
- RTC timestamp field exposed but not used (GoBoy doesn't implement RTC advancement)

### `state/cartridge/ram` (Binary Format)

Cartridge RAM (game save data):

**Sizes by Type:**
- Plain ROM: 0 bytes (no file)
- MBC2: 8KB (0x2000 bytes)
- MBC1: 32KB (0x8000 bytes)
- MBC3: 32KB (0x8000 bytes)
- MBC5: 128KB (0x20000 bytes)

**Implementation Notes:**
- Binary file with offset-based read/write support
- Currently `BankingController` only has `GetSaveData()`/`LoadSaveData()` (full dumps)
- **Need to add**: `GetRAM() []byte` method to interface for direct RAM access
- Implement in all MBC types (ROM returns nil/empty slice)
- Use `binaryMemoryFS` pattern from Phase 2

## Implementation Approach

### Architecture Decisions

1. **CPU State**: Follows memory state pattern from Phase 2
   - Helper method returns values, no internal locking
   - Text-based key=value format
   - Command handler applies changes at frame boundary

2. **ROM Loading**: New write-only pattern
   - Accumulate writes in buffer
   - Validate and load on file close
   - Trigger reset and .sav load

3. **Cartridge Info**: Read-only dynamic generation
   - Parse ROM header on each read
   - No caching (always current)
   - No write support

4. **MBC State**: Runtime type detection
   - Type switch on BankingController
   - Generate format based on actual type
   - Validate ranges on write

5. **RAM Access**: Interface extension
   - Add `GetRAM()` to BankingController interface
   - All implementations return RAM slice or nil
   - Zero-copy access via binaryMemoryFS

### Code Organization

**New Files to Create:**
- `pkg/ninep/cpustatefs.go` - CPU state filesystem
- `pkg/ninep/cpustatefs_test.go` - CPU state tests
- `pkg/ninep/romfs.go` - ROM loading filesystem
- `pkg/ninep/romfs_test.go` - ROM loading tests
- `pkg/ninep/cartridgeinofs.go` - Cartridge info filesystem
- `pkg/ninep/cartridgeinofs_test.go` - Cartridge info tests
- `pkg/ninep/cartridgestatefs.go` - Cartridge state filesystem
- `pkg/ninep/cartridgestatefs_test.go` - Cartridge state tests
- `pkg/ninep/p9cpufile.go` - p9.File wrapper for CPU state
- `pkg/ninep/p9romfile.go` - p9.File wrapper for ROM loading
- `pkg/ninep/p9cartridgeinfofile.go` - p9.File wrapper for cartridge info
- `pkg/ninep/p9cartridgestatefile.go` - p9.File wrapper for cartridge state
- `pkg/ninep/cartridgedirs.go` - Directory structures for cartridge/

**Files to Modify:**
- `pkg/gb/gameboy.go` - Add helper methods and command handlers
- `pkg/gb/gameboy_test.go` - Add tests for new helpers/handlers
- `pkg/cart/controller.go` - Add `GetRAM()` to interface
- `pkg/cart/rom.go` - Implement `GetRAM()` (return nil)
- `pkg/cart/mbc1.go` - Implement `GetRAM()`, add state getters
- `pkg/cart/mbc2.go` - Implement `GetRAM()`, add state getters
- `pkg/cart/mbc3.go` - Implement `GetRAM()`, add state getters
- `pkg/cart/mbc5.go` - Implement `GetRAM()`, add state getters
- `pkg/ninep/attacher.go` - Add ROM file to root, CPU to state/, cartridge/ dir
- `pkg/ninep/statedirs.go` - Add cpu file to Walk/Readdir
- `pkg/ninep/server.go` - Update README with new files

## Detailed Implementation Steps

### Step 1: Add GetCPUState Helper to Gameboy

**File**: `pkg/gb/gameboy.go`

Add helper method:
```go
// GetCPUState returns CPU register and timer state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetCPUState() (af, bc, de, hl, sp, pc uint16, divider int) {
	return gb.cpu.AF.HiLo(), gb.cpu.BC.HiLo(), gb.cpu.DE.HiLo(),
		gb.cpu.HL.HiLo(), gb.cpu.SP.HiLo(), gb.cpu.PC, gb.cpu.Divider
}
```

**Why no internal locking**: Follows Phase 1/2 pattern. Caller holds lock to ensure consistency across multiple reads.

### Step 2: Implement cpuStateFS

**File**: `pkg/ninep/cpustatefs.go`

```go
// ABOUTME: fs.FS implementation for CPU state file (text format).
// ABOUTME: Exposes CPU registers, program counter, and timer state.

package ninep

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"github.com/Humpheh/goboy/pkg/gb"
)

type cpuStateFS struct {
	gb *gb.Gameboy
}

func newCPUStateFS(gb *gb.Gameboy) *cpuStateFS {
	return &cpuStateFS{gb: gb}
}

func (fsys *cpuStateFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &cpuStateFile{parent: fsys}, nil
}

type cpuStateFile struct {
	parent *cpuStateFS
	buf    *bytes.Reader
}

func (f *cpuStateFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.parent.gb.Mu.RLock()
		af, bc, de, hl, sp, pc, divider := f.parent.gb.GetCPUState()
		f.parent.parent.gb.Mu.RUnlock()

		content := fmt.Sprintf(`# Registers
AF=0x%04X
BC=0x%04X
DE=0x%04X
HL=0x%04X

# Program State
SP=0x%04X
PC=0x%04X

# Timer
Divider=0x%04X
`, af, bc, de, hl, sp, pc, divider)

		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *cpuStateFile) Write(data []byte) (int, error) {
	content := string(data)
	state := make(map[string]uint16)

	// Parse key=value pairs
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("line %d: invalid format (expected key=value)", lineNum+1)
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		var value uint64
		var err error
		if strings.HasPrefix(valueStr, "0x") || strings.HasPrefix(valueStr, "0X") {
			value, err = strconv.ParseUint(valueStr[2:], 16, 16)
		} else {
			value, err = strconv.ParseUint(valueStr, 10, 16)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value %q: %v", lineNum+1, valueStr, err)
		}

		switch key {
		case "AF", "BC", "DE", "HL", "SP", "PC", "Divider":
			state[key] = uint16(value)
		default:
			return 0, fmt.Errorf("line %d: unknown field %q", lineNum+1, key)
		}
	}

	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:      "cpu-write",
			CPUState:  state,
		}
	}

	return len(data), nil
}

func (f *cpuStateFile) Close() error {
	return nil
}

func (f *cpuStateFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: "cpu", size: 0, mode: 0666}, nil
}
```

**Design Notes:**
- Follows memoryStateFS pattern from Phase 2
- No validation (trust user)
- Supports hex (0x) and decimal values
- Ignores comments and blank lines

### Step 3: Add cpu-write Command Handler

**File**: `pkg/gb/gameboy.go`

In `ProcessCommands()`, add:
```go
case "cpu-write":
	gb.Mu.Lock()
	if val, ok := cmd.CPUState["AF"]; ok {
		gb.cpu.AF.Set(val)
	}
	if val, ok := cmd.CPUState["BC"]; ok {
		gb.cpu.BC.Set(val)
	}
	if val, ok := cmd.CPUState["DE"]; ok {
		gb.cpu.DE.Set(val)
	}
	if val, ok := cmd.CPUState["HL"]; ok {
		gb.cpu.HL.Set(val)
	}
	if val, ok := cmd.CPUState["SP"]; ok {
		gb.cpu.SP.Set(val)
	}
	if val, ok := cmd.CPUState["PC"]; ok {
		gb.cpu.PC = val
	}
	if val, ok := cmd.CPUState["Divider"]; ok {
		gb.cpu.Divider = int(val)
	}
	gb.Mu.Unlock()
```

Update `Command` struct:
```go
type Command struct {
	Name     string
	Offset   int
	Data     []byte
	State    map[string]byte       // For memory state writes
	CPUState map[string]uint16     // NEW: For CPU state writes
}
```

### Step 4: Implement romFS for Write-Only ROM Loading

**File**: `pkg/ninep/romfs.go`

```go
// ABOUTME: fs.FS implementation for ROM loading file (write-only).
// ABOUTME: Accepts ROM data writes and triggers emulator ROM load on close.

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/Humpheh/goboy/pkg/gb"
)

type romFS struct {
	gb *gb.Gameboy
}

func newRomFS(gb *gb.Gameboy) *romFS {
	return &romFS{gb: gb}
}

func (fsys *romFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &romFile{parent: fsys, buf: &bytes.Buffer{}}, nil
}

type romFile struct {
	parent *romFS
	buf    *bytes.Buffer
}

func (f *romFile) Read(p []byte) (int, error) {
	return 0, fs.ErrPermission // Write-only
}

func (f *romFile) Write(data []byte) (int, error) {
	return f.buf.Write(data)
}

func (f *romFile) Close() error {
	if f.buf.Len() == 0 {
		return nil // No data written
	}

	romData := f.buf.Bytes()

	// Basic validation
	if len(romData) < 32*1024 {
		return fmt.Errorf("ROM too small: %d bytes (minimum 32KB)", len(romData))
	}
	if len(romData) > 8*1024*1024 {
		return fmt.Errorf("ROM too large: %d bytes (maximum 8MB)", len(romData))
	}

	// Queue ROM load command
	dataCopy := make([]byte, len(romData))
	copy(dataCopy, romData)

	f.parent.gb.CommandChan <- gb.Command{
		Name: "rom-load",
		Data: dataCopy,
	}

	return nil
}

func (f *romFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: "rom", size: 0, mode: 0222}, nil // Write-only
}
```

**Design Notes:**
- Accumulates writes in buffer
- Validates on Close()
- Queues rom-load command with full ROM data

### Step 5: Add rom-load Command Handler

**File**: `pkg/gb/gameboy.go`

```go
case "rom-load":
	// ROM loading happens synchronously (not at frame boundary)
	// because it's a major state change
	gb.Mu.Lock()

	// TODO: Need to understand existing ROM load flow
	// This is a placeholder - needs to call into cart.Load()
	// and handle .sav file loading

	// Rough approach:
	// 1. Load ROM into cartridge
	// 2. Reset emulator state
	// 3. Load .sav file if exists

	gb.Mu.Unlock()
```

**Implementation Challenge**: Need to investigate existing ROM loading code in `pkg/cart/controller.go` to understand:
- How to reload ROM without restarting emulator
- How .sav files are loaded
- Whether reset needs special handling

### Step 6: Implement cartridgeInfoFS for Read-Only Metadata

**File**: `pkg/ninep/cartridgeinofs.go`

```go
// ABOUTME: fs.FS implementation for cartridge info file (read-only).
// ABOUTME: Exposes ROM header metadata parsed from loaded cartridge.

package ninep

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"

	"github.com/Humpheh/goboy/pkg/gb"
)

type cartridgeInfoFS struct {
	gb *gb.Gameboy
}

func newCartridgeInfoFS(gb *gb.Gameboy) *cartridgeInfoFS {
	return &cartridgeInfoFS{gb: gb}
}

func (fsys *cartridgeInfoFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &cartridgeInfoFile{parent: fsys}, nil
}

type cartridgeInfoFile struct {
	parent *cartridgeInfoFS
	buf    *bytes.Reader
}

func (f *cartridgeInfoFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.gb.Mu.RLock()

		// TODO: Access cartridge and parse ROM header
		// Fields at ROM offsets:
		// 0x0134-0x0143: Title (16 bytes, ASCII)
		// 0x0147: Cartridge type
		// 0x0148: ROM size
		// 0x0149: RAM size
		// 0x0143: CGB flag
		// 0x0146: SGB flag
		// 0x014D: Header checksum
		// 0x014E-0x014F: Global checksum

		// Example output:
		content := fmt.Sprintf(`# ROM Information
title=EXAMPLE GAME
type=MBC3
romSize=1048576
ramSize=32768

# Features
cgbSupport=0x00
sgbSupport=0x00
hasBattery=0x01

# Checksums
headerChecksum=0x3C
globalChecksum=0xB0A2
`)

		f.parent.gb.Mu.RUnlock()
		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *cartridgeInfoFile) Write(data []byte) (int, error) {
	return 0, fs.ErrPermission // Read-only
}

func (f *cartridgeInfoFile) Close() error {
	return nil
}

func (f *cartridgeInfoFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: "info", size: 0, mode: 0444}, nil // Read-only
}
```

**Implementation Notes:**
- Parse ROM header from cartridge ROM data
- Need to access `gb.memory.cart` or similar
- Map MBC type byte to string (MBC1, MBC2, MBC3, MBC5, ROM)

### Step 7: Add GetRAM() to BankingController Interface

**File**: `pkg/cart/controller.go`

Update interface:
```go
type BankingController interface {
	Read(address uint16) byte
	WriteROM(address uint16, value byte)
	WriteRAM(address uint16, value byte)
	GetSaveData() []byte
	LoadSaveData(data []byte)
	GetRAM() []byte // NEW: Direct RAM access for 9P offset-based operations
}
```

Implement in all types:

**ROM (no RAM):**
```go
func (r *ROM) GetRAM() []byte {
	return nil
}
```

**MBC1, MBC3, MBC5:**
```go
func (r *MBC1) GetRAM() []byte {
	return r.ram
}
```

**MBC2:**
```go
func (r *MBC2) GetRAM() []byte {
	return r.ram
}
```

### Step 8: Add MBC State Getter Methods

Add to each MBC type:

**MBC1:**
```go
func (r *MBC1) GetBankingState() (romBank uint32, ramBank uint32, ramEnabled bool, romBanking bool) {
	return r.romBank, r.ramBank, r.ramEnabled, r.romBanking
}
```

**MBC2:**
```go
func (r *MBC2) GetBankingState() (romBank uint32, ramBank uint32, ramEnabled bool) {
	return r.romBank, r.ramBank, r.ramEnabled
}
```

**MBC3:**
```go
func (r *MBC3) GetBankingState() (romBank uint32, ramBank uint32, ramEnabled bool) {
	return r.romBank, r.ramBank, r.ramEnabled
}

func (r *MBC3) GetRTCState() (rtc, latchedRtc []byte, latched bool) {
	return r.rtc, r.latchedRtc, r.latched
}
```

**MBC5:**
```go
func (r *MBC5) GetBankingState() (romBank uint32, ramBank uint32, ramEnabled bool) {
	return r.romBank, r.ramBank, r.ramEnabled
}
```

### Step 9: Implement cartridgeStateFS with MBC Type Detection

**File**: `pkg/ninep/cartridgestatefs.go`

```go
// ABOUTME: fs.FS implementation for cartridge state file (text format).
// ABOUTME: Format varies by MBC type (ROM, MBC1, MBC2, MBC3, MBC5).

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/Humpheh/goboy/pkg/cart"
)

type cartridgeStateFS struct {
	gb *gb.Gameboy
}

func newCartridgeStateFS(gb *gb.Gameboy) *cartridgeStateFS {
	return &cartridgeStateFS{gb: gb}
}

func (fsys *cartridgeStateFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &cartridgeStateFile{parent: fsys}, nil
}

type cartridgeStateFile struct {
	parent *cartridgeStateFS
	buf    *bytes.Reader
}

func (f *cartridgeStateFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.gb.Mu.RLock()

		// Type switch to detect MBC type
		var content string
		controller := f.parent.gb.GetCartridge() // TODO: Need accessor method

		switch c := controller.(type) {
		case *cart.ROM:
			content = "# No banking state\n"

		case *cart.MBC1:
			romBank, ramBank, ramEnabled, romBanking := c.GetBankingState()
			content = fmt.Sprintf(`# Banking
romBank=0x%02X
ramBank=0x%02X
ramEnabled=0x%02X
romBanking=0x%02X
`, romBank, ramBank, boolToByte(ramEnabled), boolToByte(romBanking))

		case *cart.MBC2:
			romBank, ramBank, ramEnabled := c.GetBankingState()
			content = fmt.Sprintf(`# Banking
romBank=0x%02X
ramBank=0x%02X
ramEnabled=0x%02X
`, romBank, ramBank, boolToByte(ramEnabled))

		case *cart.MBC3:
			romBank, ramBank, ramEnabled := c.GetBankingState()
			rtc, latchedRtc, latched := c.GetRTCState()
			content = fmt.Sprintf(`# Banking
romBank=0x%02X
ramBank=0x%02X
ramEnabled=0x%02X

# RTC Registers
rtcSeconds=0x%02X
rtcMinutes=0x%02X
rtcHours=0x%02X
rtcDays=0x%02X
rtcControl=0x%02X

# RTC Latched
latchedSeconds=0x%02X
latchedMinutes=0x%02X
latchedHours=0x%02X
latchedDays=0x%02X
latchedControl=0x%02X
latched=0x%02X

# RTC Real-Time Reference
rtcTimestamp=0x0000000000000000
`, romBank, ramBank, boolToByte(ramEnabled),
   rtc[0], rtc[1], rtc[2], rtc[3], rtc[4],
   latchedRtc[0], latchedRtc[1], latchedRtc[2], latchedRtc[3], latchedRtc[4],
   boolToByte(latched))

		case *cart.MBC5:
			romBank, ramBank, ramEnabled := c.GetBankingState()
			content = fmt.Sprintf(`# Banking
romBank=0x%04X
ramBank=0x%02X
ramEnabled=0x%02X
`, romBank, ramBank, boolToByte(ramEnabled))

		default:
			content = "# Unknown cartridge type\n"
		}

		f.parent.gb.Mu.RUnlock()
		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *cartridgeStateFile) Write(data []byte) (int, error) {
	// Parse and validate based on MBC type
	// Queue cartridge-state-write command
	// TODO: Implementation similar to memoryStateFS
	return len(data), nil
}

func (f *cartridgeStateFile) Close() error {
	return nil
}

func (f *cartridgeStateFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: "state", size: 0, mode: 0666}, nil
}

func boolToByte(b bool) byte {
	if b {
		return 0x01
	}
	return 0x00
}
```

**Design Notes:**
- Runtime type detection via type switch
- Different output format per MBC type
- RTC timestamp always 0 (not implemented in GoBoy)

### Step 10: Add cartridge-state-write Command Handler

**File**: `pkg/gb/gameboy.go`

```go
case "cartridge-state-write":
	gb.Mu.Lock()

	// Type switch on cartridge type
	controller := gb.GetCartridge()

	switch c := controller.(type) {
	case *cart.MBC1:
		// Apply state updates for MBC1
		// TODO: Need SetBankingState() methods on MBC types

	case *cart.MBC2:
		// Apply state for MBC2

	case *cart.MBC3:
		// Apply state and RTC for MBC3

	case *cart.MBC5:
		// Apply state for MBC5
	}

	gb.Mu.Unlock()
```

Update `Command` struct:
```go
type Command struct {
	Name           string
	Offset         int
	Data           []byte
	State          map[string]byte
	CPUState       map[string]uint16
	CartridgeState map[string]interface{} // NEW: For cartridge state writes
}
```

### Step 11: Implement Cartridge RAM File

**File**: `pkg/ninep/cartridgeramfs.go`

Reuse `binaryMemoryFS` pattern:
```go
func newCartridgeRAMFS(gb *gb.Gameboy) *binaryMemoryFS {
	return &binaryMemoryFS{
		gb: gb,
		getMemory: func(gb *gb.Gameboy) []byte {
			controller := gb.GetCartridge()
			return controller.GetRAM() // May return nil for ROM-only
		},
		size:        -1, // Dynamic size
		commandName: "cartridge-ram-write",
	}
}
```

**Design Notes:**
- Size is dynamic (varies by MBC type)
- Returns nil for ROM-only carts (file doesn't exist)
- Reuses existing binaryMemoryFS infrastructure

### Step 12: Add cartridge-ram-write Command Handler

**File**: `pkg/gb/gameboy.go`

```go
case "cartridge-ram-write":
	gb.Mu.Lock()
	controller := gb.GetCartridge()
	ram := controller.GetRAM()
	if ram != nil {
		copy(ram[cmd.Offset:], cmd.Data)
	}
	gb.Mu.Unlock()
```

### Step 13: Update Directory Structures

**File**: `pkg/ninep/statedirs.go`

Add CPU file to state/ directory Walk:
```go
case "cpu":
	qid := d.attacher.qids.Get(p9.TypeRegular)
	return []p9.QID{qid}, newP9CPUFile(d.attacher.gameboy, qid), nil
```

Add cartridge directory:
```go
case "cartridge":
	qid := d.attacher.qids.Get(p9.TypeDir)
	return []p9.QID{qid}, newP9CartridgeDir(d.attacher.gameboy, d.attacher, qid), nil
```

**New File**: `pkg/ninep/cartridgedirs.go`

Implement cartridge directory with info, state, ram children.

### Step 14: Add ROM File to Root Attacher

**File**: `pkg/ninep/attacher.go`

Add rom file to root directory Walk:
```go
case "rom":
	qid := a.qids.Get(p9.TypeRegular)
	return []p9.QID{qid}, newP9RomFile(a.gameboy, qid), nil
```

### Step 15: Update README

**File**: `pkg/ninep/server.go`

Add to README content:
```
/rom                     Write ROM data to load new game
/state/
  cpu                    CPU registers and program counter
  cartridge/
    info                 ROM information (read-only)
    state                Cartridge banking and RTC state
    ram                  Cartridge RAM (size varies, binary)
```

Add examples:
```
CPU STATE
---------
Read program counter:
  grep PC state/cpu

Modify program counter:
  echo 'PC=0x0150' > state/cpu

CARTRIDGE SAVE STATES
---------------------
Backup cartridge RAM:
  cp state/cartridge/ram ~/backup.sav

Restore cartridge RAM:
  cp ~/backup.sav state/cartridge/ram

View cartridge info:
  cat state/cartridge/info

ROM LOADING
-----------
Load a new ROM:
  echo pause > ctl
  cat ~/roms/tetris.gb > rom
  echo resume > ctl
```

### Step 16: Write Comprehensive Tests

**Tests to Write:**

1. **cpustatefs_test.go**: Test CPU state read/write, parsing, validation
2. **romfs_test.go**: Test ROM loading with valid/invalid ROMs
3. **cartridgeinofs_test.go**: Test info parsing for each MBC type
4. **cartridgestatefs_test.go**: Test state read/write for each MBC type
5. **gameboy_test.go**: Test command handlers with race detector

**Test Coverage:**
- Each MBC type separately (ROM, MBC1, MBC2, MBC3, MBC5)
- CPU state with various register values
- ROM validation (too small, too large, valid)
- Cartridge state parsing and validation
- RAM access with different sizes
- Race detector on all tests

## Testing Strategy

### Unit Tests

**CPU State:**
- Read with various register values
- Write with hex and decimal values
- Partial updates (only some registers)
- Invalid field names
- Invalid values
- Comments and blank lines

**ROM Loading:**
- Valid ROM (32KB, 1MB, 2MB)
- Invalid ROM (too small, too large)
- Empty write
- Multiple writes (append behavior)

**Cartridge Info:**
- Each MBC type
- Title parsing (ASCII, truncation)
- Size calculations
- Checksum formatting

**Cartridge State:**
- Each MBC type separately
- Read with different banking states
- Write with valid values
- Write with invalid values (out of range)
- MBC3 RTC state
- Partial updates

**Cartridge RAM:**
- Read from different offsets
- Write to different offsets
- Write beyond bounds
- Different RAM sizes (8KB, 32KB, 128KB)
- ROM-only cart (no RAM)

### Integration Tests

After unit tests pass:
1. Mount 9P filesystem
2. Load various ROM types
3. Read/write CPU state
4. Read cartridge info
5. Save/restore cartridge RAM
6. Load new ROM via /rom file

### Race Detection

Run all tests with `-race` flag:
```bash
go test -race ./pkg/ninep/... ./pkg/gb/...
```

## Success Criteria

1. ✅ All unit tests pass with 100% pass rate
2. ✅ All tests pass with race detector (`-race` flag)
3. ✅ CPU state can be read and modified
4. ✅ ROMs can be loaded via /rom file
5. ✅ Cartridge info displays correctly for all MBC types
6. ✅ Cartridge state can be saved and restored
7. ✅ Cartridge RAM can be backed up and restored
8. ✅ Documentation updated with examples
9. ✅ Code follows existing patterns from Phase 1/2
10. ✅ No code duplication

## Known Limitations

1. **RTC Timestamp**: MBC3 rtcTimestamp field is exposed but not used (GoBoy doesn't implement RTC advancement based on wall-clock time)

2. **ROM Loading**: May require emulator pause for safe operation (document in README)

3. **No Validation**: CPU and cartridge state accept any values per spec philosophy (user responsible for correctness)

4. **MBC4**: Not supported by GoBoy, treated as MBC1

## Estimated Effort

**Total**: 12-17 hours

**Breakdown:**
- CPU state: 2 hours
- ROM loading: 4-6 hours (including investigation of existing code)
- Cartridge info: 2 hours
- Cartridge state: 3-4 hours (MBC type variation)
- Cartridge RAM: 1-2 hours (mostly reuse)
- Testing: 3-4 hours (comprehensive coverage across all MBC types)

## Next Steps

After Phase 3 completion:
1. Manual integration testing with real ROMs
2. Test save/restore workflow
3. Verify ROM loading works correctly
4. Document any quirks or limitations discovered
5. Proceed to Phase 4 (APU/PPU state) if needed
