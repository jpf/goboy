# Save State Architecture

This document details all mutable state in the goboy emulator that would need to be serialized for save state support.

## CPU State

Located in `pkg/gb/cpu.go`

- **Registers**: AF, BC, DE, HL, SP (all `register` type with 16-bit values) - `cpu.go:58-64`
  - Each register has a `value` field (uint16) and a `mask` field (uint16)
  - AF register uses mask 0xFFF0 to prevent lower bits from being set
- **Program Counter**: PC (uint16) - `cpu.go:63`
- **Divider**: Divider (int) - `cpu.go:66`

## Memory State

Located in `pkg/gb/memory.go`

### Core Memory Arrays
- **HighRAM**: `[0x100]byte` - Contains hardware registers (0xFF00-0xFFFF) - `memory.go:31`
- **VRAM**: `[0x4000]byte` - Video RAM with banking support (2 banks of 0x2000 each) - `memory.go:33`
- **VRAMBank**: Current VRAM bank index (byte, 0-1) - `memory.go:35`
- **WRAM**: `[0x9000]byte` - Working RAM with banking (banks 0-7) - `memory.go:38`
- **WRAMBank**: Current WRAM bank index (byte, 1-7) - `memory.go:40`
- **OAM**: `[0x100]byte` - Object Attribute Memory (sprite data) - `memory.go:42`

### DMA State
- **hdmaLength**: Current HDMA transfer length (byte) - `memory.go:45`
- **hdmaActive**: Whether HDMA is currently active (bool) - `memory.go:46`

## Cartridge State

Located in `pkg/cart/`

The cartridge state varies by Memory Bank Controller (MBC) type. The ROM data itself does not need to be saved, only the banking state and RAM contents.

### ROM (rom.go)
No state to save - no banking or RAM support.

### MBC1 (mbc1.go)
- **romBank**: Current ROM bank (uint32) - `mbc1.go:15`
- **ram**: Cartridge RAM `[0x8000]byte` - `mbc1.go:17`
- **ramBank**: Current RAM bank (uint32) - `mbc1.go:18`
- **ramEnabled**: Whether RAM is enabled (bool) - `mbc1.go:19`
- **romBanking**: ROM vs RAM banking mode (bool) - `mbc1.go:21`

### MBC2 (mbc2.go)
- **romBank**: Current ROM bank (uint32) - `mbc2.go:15`
- **ram**: Cartridge RAM `[0x2000]byte` - `mbc2.go:17`
- **ramEnabled**: Whether RAM is enabled (bool) - `mbc2.go:18`

### MBC3 (mbc3.go)
- **romBank**: Current ROM bank (uint32) - `mbc3.go:18`
- **ram**: Cartridge RAM `[0x8000]byte` - `mbc3.go:20`
- **ramBank**: Current RAM bank (uint32) - `mbc3.go:21`
- **ramEnabled**: Whether RAM is enabled (bool) - `mbc3.go:22`
- **rtc**: Real-time clock registers `[0x10]byte` - `mbc3.go:24`
- **latchedRtc**: Latched RTC values `[0x10]byte` - `mbc3.go:25`
- **latched**: RTC latch state (bool) - `mbc3.go:26`

### MBC5 (mbc5.go)
- **romBank**: Current ROM bank (uint32) - `mbc5.go:15`
- **ram**: Cartridge RAM `[0x20000]byte` - `mbc5.go:17`
- **ramBank**: Current RAM bank (uint32) - `mbc5.go:18`
- **ramEnabled**: Whether RAM is enabled (bool) - `mbc5.go:19`

## PPU/Graphics State

Located in `pkg/gb/gameboy.go` and `pkg/gb/ppu.go`

### Rendering State
- **scanlineCounter**: Cycles until next scanline (int) - `gameboy.go:41`
- **screenData**: Current frame being rendered `[160][144][3]uint8` - `gameboy.go:36`
- **bgPriority**: Background priority flags `[160][144]bool` - `gameboy.go:37`
- **tileScanline**: Current scanline tile colors `[160]uint8` - `gameboy.go:40`
- **screenCleared**: Whether screen has been cleared (bool) - `gameboy.go:42`

### CGB Palette State
Located in `pkg/gb/palettes.go`

- **bgPalette**: Background palette (*cgbPalette) - `gameboy.go:60`
- **spritePalette**: Sprite palette (*cgbPalette) - `gameboy.go:61`

Each cgbPalette contains:
- **Palette**: Color data `[]byte` (0x40 size) - `palettes.go:66`
- **Index**: Current palette index (byte) - `palettes.go:68`
- **Inc**: Auto-increment flag (bool) - `palettes.go:70`

### Global Palette State
- **CurrentPalette**: Global DMG palette selection (byte) - `palettes.go:14`
  - Note: This is a package-level global variable, ideally should be per-Gameboy instance

## APU/Sound State

Located in `pkg/apu/`

### APU Core State (apu.go)
- **memory**: Sound registers `[52]byte` - `apu.go:29`
- **waveformRam**: Channel 3 waveform data `[]byte` (0x20 size) - `apu.go:30`
- **tickCounter**: Sample timing counter (float64) - `apu.go:34`
- **lVol**: Left volume (float64) - `apu.go:35`
- **rVol**: Right volume (float64) - `apu.go:35`

### Channel State (channel.go)

Each of the 4 sound channels (chn1, chn2, chn3, chn4) contains:

- **frequency**: Channel frequency (float64) - `channel.go:12`
- **time**: Current waveform time (float64) - `channel.go:14`
- **amplitude**: Current amplitude (float64) - `channel.go:15`
- **duration**: Duration in samples (int, -1 for infinite) - `channel.go:18`
- **length**: Length counter (int) - `channel.go:19`

#### Envelope State
- **envelopeVolume**: Envelope volume (int) - `channel.go:21`
- **envelopeTime**: Envelope timer (int) - `channel.go:22`
- **envelopeSteps**: Current envelope steps (int) - `channel.go:23`
- **envelopeStepsInit**: Initial envelope steps (int) - `channel.go:24`
- **envelopeSamples**: Samples per envelope step (int) - `channel.go:25`
- **envelopeIncreasing**: Envelope direction (bool) - `channel.go:26`

#### Sweep State (Channel 1 only)
- **sweepTime**: Sweep timer (float64) - `channel.go:28`
- **sweepStepLen**: Sweep step length (byte) - `channel.go:29`
- **sweepSteps**: Total sweep steps (byte) - `channel.go:30`
- **sweepStep**: Current sweep step (byte) - `channel.go:31`
- **sweepIncrease**: Sweep direction (bool) - `channel.go:32`

#### Channel Output State
- **onL**: Output to left channel (bool) - `channel.go:34`
- **onR**: Output to right channel (bool) - `channel.go:35`

Note: The `generator` field (WaveGenerator) is a function pointer and cannot be serialized directly. It would need to be reconstructed based on the sound register state.

## Timer State

Located in `pkg/gb/gameboy.go`

- **timerCounter**: Timer cycle accumulator (int) - `gameboy.go:32`

## Interrupt/Execution State

Located in `pkg/gb/gameboy.go`

- **interruptsEnabling**: Interrupt enable pending (bool) - `gameboy.go:48`
- **interruptsOn**: Interrupts enabled (bool) - `gameboy.go:49`
- **halted**: CPU halted state (bool) - `gameboy.go:50`

## Input State

Located in `pkg/gb/gameboy.go`

- **inputMask**: Currently pressed buttons bitmask (byte) - `gameboy.go:55`

## CGB Mode State

Located in `pkg/gb/gameboy.go`

- **cgbMode**: Whether CGB features are enabled (bool) - `gameboy.go:59`
- **currentSpeed**: CPU speed multiplier (byte, 0=normal, 1=double) - `gameboy.go:63`
- **prepareSpeed**: Speed switch pending (bool) - `gameboy.go:64`

## State Not Requiring Serialization

The following state does not need to be saved:

- **ROM data**: Loaded from file, never modified
- **PreparedData**: Completed frame buffer - can be reconstructed on load
- **paused**: Runtime state, not emulation state
- **Debug flags**: Runtime debugging state
- **keyHandlers**: Function pointers, reconstructed on init
- **cbInst**: CB instruction table, reconstructed on init
- **audioBuffer**: Transient audio data
- **player**: Audio output device, reconstructed on init
- **thisCpuTicks**: Temporary cycle counter

## Implementation Notes

### Current State
- No save state implementation exists
- Cartridge RAM is saved/loaded via `GetSaveData()` and `LoadSaveData()` methods, but this is for battery-backed saves only
- State is scattered across multiple structs with no unified serialization

### Challenges for Implementation
1. **Scattered State**: State is distributed across ~20 different structs
2. **Private Fields**: Most state fields are unexported, requiring serialization support to be added to each package
3. **Interface Types**: The `BankingController` interface means different cartridge types have different state shapes
4. **Function Pointers**: Sound channel generators and instruction tables cannot be serialized directly
5. **Global State**: `CurrentPalette` is a package-level global

### Suggested Approach
1. Add serialization methods to each major component (CPU, Memory, APU, etc.)
2. Create a top-level `SaveState` struct that aggregates all component state
3. Use a format like JSON or gob for serialization
4. Include a version number for forward compatibility
5. Store cartridge type information to reconstruct the correct BankingController on load
