# Phase 4: APU and PPU State Implementation

## Overview

Phase 4 adds audio (APU) and graphics (PPU) state to the 9P filesystem interface. This completes the 9P specification, enabling perfect audio/video reproduction across save states and providing advanced debugging capabilities.

After Phase 3, users already have functional save states for gameplay. Phase 4 adds:
- Audio state for continuous sound across save/restore
- Graphics state for mid-frame save states
- Screen buffer access for frame extraction and video recording
- Complete state tree for TAS (Tool-Assisted Speedrun) tools

## Goals

1. **APU State**: Volume, timing, and channel configuration
2. **APU Waveform**: Channel 3 custom waveform data
3. **Channel State**: Per-channel audio state (frequency, envelope, sweep, output)
4. **PPU State**: Graphics timing, flags, and global emulator state
5. **Screen Buffer**: RGB pixel data for current frame
6. **Priority Maps**: Background priority and scanline data
7. **Palette State**: CGB color palettes with special hex encoding

## File Specifications

### `state/apu/state` (Text Format)

APU global state:
```
# Volume
lVol=0x07
rVol=0x07

# Timing
tickCounter=0x0000
```

**Fields:**
- `lVol`: Left channel volume (0x00-0x07, hex)
- `rVol`: Right channel volume (0x00-0x07, hex)
- `tickCounter`: Audio timing counter (hex)

**Implementation Notes:**
- Reuse `memoryStateFS` pattern
- Add `GetAPUState()` helper to Gameboy
- Add `apu-state-write` command handler

### `state/apu/waveform` (Binary Format)

Channel 3 waveform RAM:
- **Size**: 32 bytes (0x20 bytes)
- **Content**: Custom waveform data for channel 3

**Implementation Notes:**
- Binary file from APU.waveformRam
- Use `binaryMemoryFS` pattern
- Add `GetWaveformRAM()` helper
- Add `waveform-write` command handler

### `state/apu/channel1` (Text Format)

Channel 1 state (with sweep):
```
# Frequency
frequency=0x0000
time=0x0000

# Amplitude
amplitude=0x00
duration=0xFFFFFFFF
length=0x00

# Envelope
envelopeVolume=0x00
envelopeTime=0x00
envelopeSteps=0x00
envelopeStepsInit=0x00
envelopeSamples=0x00
envelopeIncreasing=0x00

# Sweep
sweepTime=0x0000
sweepStepLen=0x00
sweepSteps=0x00
sweepStep=0x00
sweepIncrease=0x00

# Output
onL=0x00
onR=0x00
```

**Fields (15 total):**
- `frequency`: Current frequency (float64 as hex)
- `time`: Wave position (float64 as hex)
- `amplitude`: Current amplitude (float64 as hex)
- `duration`: Remaining duration in samples (int as hex, 0xFFFFFFFF = infinite)
- `length`: Length counter value (hex)
- `envelopeVolume`, `envelopeTime`, `envelopeSteps`, `envelopeStepsInit`, `envelopeSamples`: Envelope state
- `envelopeIncreasing`: Envelope direction (0x00 or 0x01)
- `sweepTime`: Sweep timing (float64 as hex)
- `sweepStepLen`, `sweepSteps`, `sweepStep`: Sweep counters
- `sweepIncrease`: Sweep direction (0x00 or 0x01)
- `onL`, `onR`: Output routing (0x00 or 0x01)

**Implementation Notes:**
- Channel 1 has sweep fields, channels 2-4 don't
- Float64 fields encoded as hex (see implementation section)
- `generator` function pointer cannot be serialized (reconstructed from sound registers)

### `state/apu/channel2`, `channel3`, `channel4` (Text Format)

Same as channel1 but without sweep section (13 fields instead of 15):
```
# Frequency
frequency=0x0000
time=0x0000

# Amplitude
amplitude=0x00
duration=0xFFFFFFFF
length=0x00

# Envelope
envelopeVolume=0x00
envelopeTime=0x00
envelopeSteps=0x00
envelopeStepsInit=0x00
envelopeSamples=0x00
envelopeIncreasing=0x00

# Output
onL=0x00
onR=0x00
```

**Implementation Notes:**
- Create generic `channelStateFS` with optional sweep fields
- Add `GetChannelState(n int)` helper returning channel n state

### `state/ppu/state` (Text Format)

PPU and global emulator state:
```
# Timing
scanlineCounter=0x0000

# Flags
screenCleared=0x00

# CGB Mode
cgbMode=0x00
currentSpeed=0x00
prepareSpeed=0x00

# Input
inputMask=0xFF

# Timing State
timerCounter=0x0000

# Interrupts
interruptsEnabling=0x00
interruptsOn=0x00
halted=0x00
```

**Fields (10 total):**
- `scanlineCounter`: PPU timing counter (hex)
- `screenCleared`: Screen clear flag (0x00 or 0x01)
- `cgbMode`: CGB mode active (0x00 or 0x01)
- `currentSpeed`: CPU speed (1 or 2 for CGB double speed)
- `prepareSpeed`: Prepared speed for speed switch (0-2)
- `inputMask`: Button press mask (hex)
- `timerCounter`: Timer counter state (hex)
- `interruptsEnabling`: Interrupt enable in progress (0x00 or 0x01)
- `interruptsOn`: Interrupts enabled (0x00 or 0x01)
- `halted`: CPU halted state (0x00 or 0x01)

**Implementation Notes:**
- Mix of PPU-specific and global Gameboy state
- Add `GetPPUState()` helper to Gameboy
- Add `ppu-state-write` command handler

### `state/ppu/screen` (Binary Format)

Current frame RGB data:
- **Size**: 69,120 bytes (160 × 144 × 3)
- **Format**: Row-major, top-to-bottom, left-to-right
- **Pixel Format**: 3 bytes per pixel (R, G, B), each 0-255

**Memory Layout:**
```
Pixel [0,0]:   bytes 0-2   (R, G, B)
Pixel [1,0]:   bytes 3-5   (R, G, B)
...
Pixel [159,0]: bytes 477-479 (R, G, B)
Pixel [0,1]:   bytes 480-482 (R, G, B)
...
Pixel [159,143]: bytes 69117-69119 (R, G, B)
```

**Implementation Notes:**
- Flatten PreparedData [160][144][3] → []byte[69120]
- Add `GetScreenData()` helper returning flattened slice
- Add `screen-write` command handler to restore data
- Use `binaryMemoryFS` pattern

### `state/ppu/bgpriority` (Binary Format)

Background priority map:
- **Size**: 23,040 bytes (160 × 144)
- **Format**: Row-major, one byte per pixel
- **Values**: 0x00 (false) or 0x01 (true)

**Implementation Notes:**
- Flatten bgPriority [160][144]bool → []byte[23040]
- Convert bool to byte: true=0x01, false=0x00
- Add `GetBGPriority()` helper
- Add `bgpriority-write` command handler

### `state/ppu/tilescanline` (Binary Format)

Current scanline tile indices:
- **Size**: 160 bytes
- **Content**: Tile color indices for current scanline

**Implementation Notes:**
- Direct access to tileScanline [160]uint8
- Add `GetTileScanline()` helper
- Add `tilescanline-write` command handler

### `state/ppu/bgpalette` and `state/ppu/spritepalette` (Text Format)

CGB palette state:
```
# Palette Data
index=0x00
autoIncrement=0x00

# Colors (64 bytes as hex pairs)
data=0x00010203...3E3F
```

**Fields:**
- `index`: Current palette index (0x00-0x3F, hex)
- `autoIncrement`: Auto-increment on write (0x00 or 0x01)
- `data`: 64 palette bytes as 128 hex digits (no 0x prefix)

**Palette Data Format:**
- 64 bytes of color data
- Encoded as 128 hex digits: "000102030405...3D3E3F"
- Each byte becomes 2 hex chars: byte 0x1A → "1A"
- NO "0x" prefix on data field

**Example:**
```
data=0001020304050607...3D3E3F
```

**Implementation Notes:**
- Special hex string encoding (different from other fields)
- Parse: Split string into 2-char chunks, parse each as hex
- Format: fmt.Sprintf("%02X", byte) for each, concatenate
- Add `GetBGPalette()` and `GetSpritePalette()` helpers
- Add `bgpalette-write` and `spritepalette-write` handlers

### `state/ppu/dmgpalette` (Text Format)

DMG palette selection:
```
currentPalette=0x00
```

**Field:**
- `currentPalette`: Selected DMG palette index (hex)

**Implementation Notes:**
- Single field file
- Add `GetDMGPalette()` helper
- Add `dmgpalette-write` command handler

## Implementation Approach

### Architecture Decisions

1. **Float64 Encoding**: Channel state has float64 fields (frequency, time, amplitude, sweepTime)
   - Encode as uint64 hex using `math.Float64bits()`
   - Decode using `math.Float64frombits()`
   - Format: `frequency=0x4090000000000000` (hex representation of float64 bits)

2. **Palette Hex String**: Special encoding for 64-byte palette data
   - Different from other hex fields (no "0x" prefix, raw hex string)
   - Must parse/format carefully

3. **Screen Flattening**: 3D array to 1D for binary file access
   - Row-major order: x innermost, y middle, color outermost
   - Index calculation: `(y * 160 * 3) + (x * 3) + c`

4. **Generic Channel FS**: Single implementation for all 4 channels
   - Pass channel number to constructor
   - Conditional inclusion of sweep fields for channel 1

5. **Function Pointer Limitation**: Channel.generator cannot be serialized
   - Document limitation in README
   - Reconstruct from sound register state on restore
   - User must restore sound registers via state/memory/highram

### Code Organization

**New Files to Create:**
- `pkg/ninep/apustatefs.go` - APU state filesystem
- `pkg/ninep/apustatefs_test.go` - APU state tests
- `pkg/ninep/channelstatefs.go` - Channel state filesystem (generic for all 4)
- `pkg/ninep/channelstatefs_test.go` - Channel state tests
- `pkg/ninep/ppustatefs.go` - PPU state filesystem
- `pkg/ninep/ppustatefs_test.go` - PPU state tests
- `pkg/ninep/palettestatefs.go` - Palette state filesystem with hex encoding
- `pkg/ninep/palettestatefs_test.go` - Palette state tests
- `pkg/ninep/p9apustatefile.go` - p9.File wrapper for APU state
- `pkg/ninep/p9channelfile.go` - p9.File wrapper for channel state
- `pkg/ninep/p9ppustatefile.go` - p9.File wrapper for PPU state
- `pkg/ninep/p9palettefile.go` - p9.File wrapper for palette state
- `pkg/ninep/apudirs.go` - APU directory structure
- `pkg/ninep/ppudirs.go` - PPU directory structure

**Files to Modify:**
- `pkg/gb/gameboy.go` - Add 10 helper methods and 10 command handlers
- `pkg/gb/gameboy_test.go` - Test new helpers and handlers
- `pkg/ninep/statedirs.go` - Add apu/ and ppu/ directories
- `pkg/ninep/server.go` - Update README with APU/PPU files

## Detailed Implementation Steps

### Step 1: Add APU Helper Methods to Gameboy

**File**: `pkg/gb/gameboy.go`

```go
// GetAPUState returns APU volume and timing state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetAPUState() (lVol, rVol float64, tickCounter float64) {
	return gb.sound.lVol, gb.sound.rVol, gb.sound.tickCounter
}

// GetWaveformRAM returns pointer to channel 3 waveform RAM for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetWaveformRAM() []byte {
	return gb.sound.waveformRam
}

// GetChannelState returns state for audio channel n (1-4) for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetChannelState(n int) *apu.Channel {
	switch n {
	case 1:
		return gb.sound.chn1
	case 2:
		return gb.sound.chn2
	case 3:
		return gb.sound.chn3
	case 4:
		return gb.sound.chn4
	default:
		return nil
	}
}
```

**Design Notes:**
- No internal locking (follows Phase 1/2 pattern)
- GetChannelState returns pointer for direct field access
- Caller holds lock for consistency

### Step 2: Add PPU Helper Methods to Gameboy

**File**: `pkg/gb/gameboy.go`

```go
// GetPPUState returns PPU and global emulator state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetPPUState() (
	scanlineCounter int,
	screenCleared bool,
	cgbMode bool,
	currentSpeed int,
	prepareSpeed int,
	inputMask byte,
	timerCounter int,
	interruptsEnabling bool,
	interruptsOn bool,
	halted bool,
) {
	return gb.scanlineCounter, gb.screenCleared, gb.IsCGB(),
		gb.getSpeed(), gb.prepareSpeed, gb.inputMask, gb.timerCounter,
		gb.interruptsEnabling, gb.interruptsOn, gb.halted
}

// GetScreenData returns flattened RGB screen data for 9P access.
// Returns 69120 bytes (160×144×3).
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetScreenData() []byte {
	data := make([]byte, 160*144*3)
	idx := 0
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			data[idx] = gb.PreparedData[x][y][0]   // R
			data[idx+1] = gb.PreparedData[x][y][1] // G
			data[idx+2] = gb.PreparedData[x][y][2] // B
			idx += 3
		}
	}
	return data
}

// GetBGPriority returns flattened background priority map for 9P access.
// Returns 23040 bytes (160×144), 0x00=false, 0x01=true.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetBGPriority() []byte {
	data := make([]byte, 160*144)
	idx := 0
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			if gb.bgPriority[x][y] {
				data[idx] = 0x01
			} else {
				data[idx] = 0x00
			}
			idx++
		}
	}
	return data
}

// GetTileScanline returns pointer to current scanline tile data for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetTileScanline() []byte {
	return gb.tileScanline[:]
}

// GetBGPalette returns CGB background palette state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetBGPalette() (index byte, autoIncrement bool, data []byte) {
	return gb.bgPalette.Index, gb.bgPalette.Inc, gb.bgPalette.Palette
}

// GetSpritePalette returns CGB sprite palette state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetSpritePalette() (index byte, autoIncrement bool, data []byte) {
	return gb.spritePalette.Index, gb.spritePalette.Inc, gb.spritePalette.Palette
}

// GetDMGPalette returns DMG palette selection for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetDMGPalette() byte {
	return gb.currentPalette
}
```

**Design Notes:**
- Screen and bgPriority helpers allocate and flatten arrays
- Palette helpers return slices for direct access
- All follow no-internal-locking pattern

### Step 3: Implement apuStateFS

**File**: `pkg/ninep/apustatefs.go`

```go
// ABOUTME: fs.FS implementation for APU state file (text format).
// ABOUTME: Exposes APU volume and timing state.

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"strconv"
	"strings"

	"github.com/Humpheh/goboy/pkg/gb"
)

type apuStateFS struct {
	gb *gb.Gameboy
}

func newAPUStateFS(gb *gb.Gameboy) *apuStateFS {
	return &apuStateFS{gb: gb}
}

func (fsys *apuStateFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &apuStateFile{parent: fsys}, nil
}

type apuStateFile struct {
	parent *apuStateFS
	buf    *bytes.Reader
}

func (f *apuStateFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.gb.Mu.RLock()
		lVol, rVol, tickCounter := f.parent.gb.GetAPUState()
		f.parent.gb.Mu.RUnlock()

		// Encode float64 as hex bits
		content := fmt.Sprintf(`# Volume
lVol=0x%016X
rVol=0x%016X

# Timing
tickCounter=0x%016X
`, math.Float64bits(lVol), math.Float64bits(rVol), math.Float64bits(tickCounter))

		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *apuStateFile) Write(data []byte) (int, error) {
	content := string(data)
	state := make(map[string]float64)

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("line %d: invalid format", lineNum+1)
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Parse as uint64 hex, then convert to float64
		var bits uint64
		var err error
		if strings.HasPrefix(valueStr, "0x") {
			bits, err = strconv.ParseUint(valueStr[2:], 16, 64)
		} else {
			return 0, fmt.Errorf("line %d: expected hex value with 0x prefix", lineNum+1)
		}
		if err != nil {
			return 0, fmt.Errorf("line %d: invalid value: %v", lineNum+1, err)
		}

		value := math.Float64frombits(bits)

		switch key {
		case "lVol", "rVol", "tickCounter":
			state[key] = value
		default:
			return 0, fmt.Errorf("line %d: unknown field %q", lineNum+1, key)
		}
	}

	if len(state) > 0 {
		f.parent.gb.CommandChan <- gb.Command{
			Name:     "apu-state-write",
			APUState: state,
		}
	}

	return len(data), nil
}

func (f *apuStateFile) Close() error {
	return nil
}

func (f *apuStateFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: "state", size: 0, mode: 0666}, nil
}
```

**Design Notes:**
- Float64 encoded as hex bits using `math.Float64bits()`
- Requires "0x" prefix on values
- 16-digit hex format (%016X) for 64-bit values

### Step 4: Implement channelStateFS (Generic for All 4 Channels)

**File**: `pkg/ninep/channelstatefs.go`

```go
// ABOUTME: fs.FS implementation for audio channel state files (text format).
// ABOUTME: Generic implementation for channels 1-4, with optional sweep for channel 1.

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"

	"github.com/Humpheh/goboy/pkg/gb"
)

type channelStateFS struct {
	gb      *gb.Gameboy
	channel int // 1-4
}

func newChannelStateFS(gb *gb.Gameboy, channel int) *channelStateFS {
	return &channelStateFS{gb: gb, channel: channel}
}

func (fsys *channelStateFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &channelStateFile{parent: fsys}, nil
}

type channelStateFile struct {
	parent *channelStateFS
	buf    *bytes.Reader
}

func (f *channelStateFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.gb.Mu.RLock()
		chn := f.parent.gb.GetChannelState(f.parent.channel)
		f.parent.gb.Mu.RUnlock()

		if chn == nil {
			return 0, fmt.Errorf("invalid channel: %d", f.parent.channel)
		}

		// Build content with all fields
		var content string
		content += fmt.Sprintf(`# Frequency
frequency=0x%016X
time=0x%016X

# Amplitude
amplitude=0x%016X
duration=0x%08X
length=0x%02X

# Envelope
envelopeVolume=0x%02X
envelopeTime=0x%02X
envelopeSteps=0x%02X
envelopeStepsInit=0x%02X
envelopeSamples=0x%02X
envelopeIncreasing=0x%02X

`,
			math.Float64bits(chn.frequency),
			math.Float64bits(chn.time),
			math.Float64bits(chn.amplitude),
			uint32(chn.duration),
			chn.length,
			chn.envelopeVolume,
			chn.envelopeTime,
			chn.envelopeSteps,
			chn.envelopeStepsInit,
			chn.envelopeSamples,
			boolToByte(chn.envelopeIncreasing))

		// Add sweep section only for channel 1
		if f.parent.channel == 1 {
			content += fmt.Sprintf(`# Sweep
sweepTime=0x%016X
sweepStepLen=0x%02X
sweepSteps=0x%02X
sweepStep=0x%02X
sweepIncrease=0x%02X

`,
				math.Float64bits(chn.sweepTime),
				chn.sweepStepLen,
				chn.sweepSteps,
				chn.sweepStep,
				boolToByte(chn.sweepIncrease))
		}

		content += fmt.Sprintf(`# Output
onL=0x%02X
onR=0x%02X
`, boolToByte(chn.onL), boolToByte(chn.onR))

		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *channelStateFile) Write(data []byte) (int, error) {
	// Parse and queue channel-write command
	// TODO: Implementation similar to apuStateFS
	return len(data), nil
}

func (f *channelStateFile) Close() error {
	return nil
}

func (f *channelStateFile) Stat() (fs.FileInfo, error) {
	name := fmt.Sprintf("channel%d", f.parent.channel)
	return &fileInfo{name: name, size: 0, mode: 0666}, nil
}

func boolToByte(b bool) byte {
	if b {
		return 0x01
	}
	return 0x00
}
```

**Design Notes:**
- Generic implementation for all 4 channels
- Conditional sweep section for channel 1
- Float64, int, and byte fields with appropriate hex formats
- Duration is int but output as uint32 hex (0xFFFFFFFF for -1/infinite)

### Step 5: Implement paletteStateFS with Special Hex Encoding

**File**: `pkg/ninep/palettestatefs.go`

```go
// ABOUTME: fs.FS implementation for CGB palette state files (text format).
// ABOUTME: Special hex string encoding for 64-byte palette data field.

package ninep

import (
	"bytes"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/Humpheh/goboy/pkg/gb"
)

type paletteStateFS struct {
	gb     *gb.Gameboy
	isSprite bool // true for sprite palette, false for background
}

func newPaletteStateFS(gb *gb.Gameboy, isSprite bool) *paletteStateFS {
	return &paletteStateFS{gb: gb, isSprite: isSprite}
}

func (fsys *paletteStateFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return &paletteStateFile{parent: fsys}, nil
}

type paletteStateFile struct {
	parent *paletteStateFS
	buf    *bytes.Reader
}

func (f *paletteStateFile) Read(p []byte) (int, error) {
	if f.buf == nil {
		f.parent.gb.Mu.RLock()

		var index byte
		var autoIncrement bool
		var data []byte

		if f.parent.isSprite {
			index, autoIncrement, data = f.parent.gb.GetSpritePalette()
		} else {
			index, autoIncrement, data = f.parent.gb.GetBGPalette()
		}

		f.parent.gb.Mu.RUnlock()

		// Encode 64 bytes as 128 hex digits
		hexData := ""
		for _, b := range data {
			hexData += fmt.Sprintf("%02X", b)
		}

		content := fmt.Sprintf(`# Palette Data
index=0x%02X
autoIncrement=0x%02X

# Colors (64 bytes as hex pairs)
data=%s
`, index, boolToByte(autoIncrement), hexData)

		f.buf = bytes.NewReader([]byte(content))
	}

	return f.buf.Read(p)
}

func (f *paletteStateFile) Write(data []byte) (int, error) {
	content := string(data)
	state := make(map[string]interface{})

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("line %d: invalid format", lineNum+1)
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		switch key {
		case "index", "autoIncrement":
			// Parse as single byte hex
			var value uint64
			var err error
			if strings.HasPrefix(valueStr, "0x") {
				value, err = strconv.ParseUint(valueStr[2:], 16, 8)
			} else {
				value, err = strconv.ParseUint(valueStr, 10, 8)
			}
			if err != nil {
				return 0, fmt.Errorf("line %d: invalid value: %v", lineNum+1, err)
			}
			state[key] = byte(value)

		case "data":
			// Parse hex string (128 chars → 64 bytes)
			// No "0x" prefix expected
			if len(valueStr) != 128 {
				return 0, fmt.Errorf("line %d: data must be 128 hex digits, got %d", lineNum+1, len(valueStr))
			}

			paletteData := make([]byte, 64)
			for i := 0; i < 64; i++ {
				hexPair := valueStr[i*2 : i*2+2]
				value, err := strconv.ParseUint(hexPair, 16, 8)
				if err != nil {
					return 0, fmt.Errorf("line %d: invalid hex at position %d: %v", lineNum+1, i*2, err)
				}
				paletteData[i] = byte(value)
			}
			state[key] = paletteData

		default:
			return 0, fmt.Errorf("line %d: unknown field %q", lineNum+1, key)
		}
	}

	if len(state) > 0 {
		cmdName := "bgpalette-write"
		if f.parent.isSprite {
			cmdName = "spritepalette-write"
		}

		f.parent.gb.CommandChan <- gb.Command{
			Name:         cmdName,
			PaletteState: state,
		}
	}

	return len(data), nil
}

func (f *paletteStateFile) Close() error {
	return nil
}

func (f *paletteStateFile) Stat() (fs.FileInfo, error) {
	name := "bgpalette"
	if f.parent.isSprite {
		name = "spritepalette"
	}
	return &fileInfo{name: name, size: 0, mode: 0666}, nil
}
```

**Design Notes:**
- Special hex encoding: 64 bytes → 128 hex chars (no "0x" prefix on data field)
- Parse by splitting into 2-char chunks
- Validate length (must be exactly 128 chars)
- isSprite flag differentiates between bg and sprite palettes

### Step 6: Implement ppuStateFS

**File**: `pkg/ninep/ppustatefs.go`

Similar to cpuStateFS and apuStateFS, implements text-based state file for PPU fields.

```go
// ABOUTME: fs.FS implementation for PPU state file (text format).
// ABOUTME: Exposes PPU timing, flags, and global emulator state.

// Implementation follows same pattern as cpuStateFS and apuStateFS
// 10 fields: scanlineCounter, screenCleared, cgbMode, currentSpeed,
// prepareSpeed, inputMask, timerCounter, interruptsEnabling,
// interruptsOn, halted
```

### Step 7: Implement Binary File Helpers

Screen, bgpriority, tilescanline, and waveform all use `binaryMemoryFS` pattern:

```go
func newScreenFS(gb *gb.Gameboy) *binaryMemoryFS {
	return &binaryMemoryFS{
		gb:          gb,
		getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetScreenData() },
		size:        69120,
		commandName: "screen-write",
	}
}

func newBGPriorityFS(gb *gb.Gameboy) *binaryMemoryFS {
	return &binaryMemoryFS{
		gb:          gb,
		getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetBGPriority() },
		size:        23040,
		commandName: "bgpriority-write",
	}
}

func newTileScanlineFS(gb *gb.Gameboy) *binaryMemoryFS {
	return &binaryMemoryFS{
		gb:          gb,
		getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetTileScanline() },
		size:        160,
		commandName: "tilescanline-write",
	}
}

func newWaveformFS(gb *gb.Gameboy) *binaryMemoryFS {
	return &binaryMemoryFS{
		gb:          gb,
		getMemory:   func(gb *gb.Gameboy) []byte { return gb.GetWaveformRAM() },
		size:        32,
		commandName: "waveform-write",
	}
}
```

### Step 8: Add Command Handlers

**File**: `pkg/gb/gameboy.go`

Update `Command` struct:
```go
type Command struct {
	Name           string
	Offset         int
	Data           []byte
	State          map[string]byte
	CPUState       map[string]uint16
	APUState       map[string]float64      // NEW
	ChannelState   map[string]interface{}  // NEW (mixed types)
	PaletteState   map[string]interface{}  // NEW (mixed types)
	PPUState       map[string]interface{}  // NEW (mixed types)
}
```

Add handlers in `ProcessCommands()`:

```go
case "apu-state-write":
	gb.Mu.Lock()
	if val, ok := cmd.APUState["lVol"]; ok {
		gb.sound.lVol = val
	}
	if val, ok := cmd.APUState["rVol"]; ok {
		gb.sound.rVol = val
	}
	if val, ok := cmd.APUState["tickCounter"]; ok {
		gb.sound.tickCounter = val
	}
	gb.Mu.Unlock()

case "waveform-write":
	gb.Mu.Lock()
	copy(gb.sound.waveformRam[cmd.Offset:], cmd.Data)
	gb.Mu.Unlock()

case "channel-write":
	// TODO: Apply channel state from cmd.ChannelState
	// Need to parse channel number and apply fields

case "ppu-state-write":
	// TODO: Apply PPU state from cmd.PPUState

case "screen-write":
	gb.Mu.Lock()
	// Unflatten data into PreparedData[160][144][3]
	idx := 0
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			gb.PreparedData[x][y][0] = cmd.Data[idx]
			gb.PreparedData[x][y][1] = cmd.Data[idx+1]
			gb.PreparedData[x][y][2] = cmd.Data[idx+2]
			idx += 3
		}
	}
	gb.Mu.Unlock()

case "bgpriority-write":
	gb.Mu.Lock()
	idx := 0
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			gb.bgPriority[x][y] = cmd.Data[idx] != 0x00
			idx++
		}
	}
	gb.Mu.Unlock()

case "tilescanline-write":
	gb.Mu.Lock()
	copy(gb.tileScanline[:], cmd.Data)
	gb.Mu.Unlock()

case "bgpalette-write":
	// TODO: Apply palette state

case "spritepalette-write":
	// TODO: Apply palette state

case "dmgpalette-write":
	// TODO: Apply DMG palette
```

### Step 9: Create Directory Structures

**File**: `pkg/ninep/apudirs.go`

Implement `state/apu/` directory with Walk() returning:
- state file
- waveform file
- channel1 file
- channel2 file
- channel3 file
- channel4 file

**File**: `pkg/ninep/ppudirs.go`

Implement `state/ppu/` directory with Walk() returning:
- state file
- screen file
- bgpriority file
- tilescanline file
- bgpalette file
- spritepalette file
- dmgpalette file

### Step 10: Update statedirs.go

**File**: `pkg/ninep/statedirs.go`

Add to state/ directory Walk:
```go
case "apu":
	qid := d.attacher.qids.Get(p9.TypeDir)
	return []p9.QID{qid}, newP9APUDir(d.attacher.gameboy, d.attacher, qid), nil

case "ppu":
	qid := d.attacher.qids.Get(p9.TypeDir)
	return []p9.QID{qid}, newP9PPUDir(d.attacher.gameboy, d.attacher, qid), nil
```

### Step 11: Update README

**File**: `pkg/ninep/server.go`

Add to README:
```
state/apu/state          Audio processing unit state
state/apu/waveform       Channel 3 waveform RAM (32B binary)
state/apu/channel1-4     Individual sound channel state
state/ppu/state          Picture processing unit and execution state
state/ppu/screen         Current frame RGB data (160x144x3 binary)
state/ppu/bgpriority     Background priority map (160x144 binary)
state/ppu/tilescanline   Current scanline buffer (160B binary)
state/ppu/bgpalette      CGB background palette
state/ppu/spritepalette  CGB sprite palette
state/ppu/dmgpalette     DMG palette selection
```

Add examples:
```
AUDIO STATE
-----------
View audio state:
  cat state/apu/state

View channel 1 state:
  cat state/apu/channel1

Backup audio state:
  tar -czf audio.tar.gz state/apu/

GRAPHICS STATE
--------------
Extract current frame:
  cp state/ppu/screen frame.rgb

View PPU state:
  cat state/ppu/state

Modify palette:
  echo 'index=0x00' > state/ppu/bgpalette
```

### Step 12: Write Comprehensive Tests

**Test Files:**
- `apustatefs_test.go`: Test APU state read/write, float64 encoding
- `channelstatefs_test.go`: Test all 4 channels, sweep conditional
- `palettestatefs_test.go`: Test palette hex encoding/decoding (128 chars)
- `ppustatefs_test.go`: Test PPU state fields
- `gameboy_test.go`: Test all command handlers with race detector

**Test Coverage:**
- APU state with various float64 values
- All 4 channels (especially channel 1 sweep fields)
- Palette hex encoding edge cases (wrong length, invalid hex)
- Screen flattening/unflattening
- BGPriority bool→byte conversion
- Race detector on all tests

## Testing Strategy

### Unit Tests

**APU State:**
- Float64 encoding/decoding
- Volume values (0.0-1.0)
- Tick counter values
- Invalid hex values

**Channel State:**
- All 4 channels separately
- Channel 1 with sweep fields
- Float64 fields (frequency, time, amplitude, sweepTime)
- Int fields (duration, including -1/infinite)
- Bool fields (envelope/sweep/output flags)

**Palette State:**
- 128-char hex string parsing
- Wrong length (127, 129 chars)
- Invalid hex chars
- index and autoIncrement fields
- Partial updates

**PPU State:**
- Various flag combinations
- Scanline counter values
- Speed values (1, 2 for CGB)

**Binary Files:**
- Screen: Read/write, verify RGB values
- BGPriority: Bool conversion, read/write
- TileScanline: Read/write 160 bytes
- Waveform: Read/write 32 bytes

### Integration Tests

1. Mount 9P filesystem
2. Read/write APU state
3. Read/write all 4 channels
4. Extract screen buffer
5. Modify and restore palette
6. Save/restore complete APU/PPU state

### Race Detection

```bash
go test -race ./pkg/ninep/... ./pkg/gb/...
```

## Success Criteria

1. ✅ All unit tests pass (100% pass rate)
2. ✅ All tests pass with race detector
3. ✅ APU state can be saved and restored
4. ✅ All 4 channels save/restore correctly
5. ✅ Screen buffer can be extracted
6. ✅ Palette hex encoding works correctly
7. ✅ PPU state fields accessible
8. ✅ Documentation updated
9. ✅ Code follows Phase 1/2/3 patterns
10. ✅ No code duplication

## Known Limitations

1. **Function Pointer**: Channel.generator cannot be serialized
   - Documented in README
   - Must restore sound registers via state/memory/highram after channel restore
   - Generator reconstructed from register state

2. **Frame Sequencer**: GoBoy doesn't implement 512Hz audio frame sequencer
   - State files capture timing state that exists
   - Audio may not be bit-perfect after restore for timing-sensitive games

3. **Palette Data Format**: Special encoding (128 hex chars) differs from other hex fields
   - May confuse users expecting "0x" prefix
   - Clearly documented in README

## Estimated Effort

**Total**: 13-17 hours

**Breakdown:**
- APU state: 2 hours
- APU waveform: 1 hour
- Channel state: 3-4 hours (float64 encoding, sweep conditional)
- PPU state: 2 hours
- Screen/priority/scanline: 2-3 hours (flattening/unflattening)
- Palette state: 3-4 hours (special hex encoding)
- Testing: 3-4 hours (comprehensive coverage, palette edge cases)

## Next Steps

After Phase 4 completion:
1. Complete 9P spec implementation
2. Manual integration testing with real games
3. Test save/restore with audio continuity
4. Extract frames for video recording
5. Benchmark performance impact of 9P server
6. Document any limitations discovered
