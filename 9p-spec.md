# 9P Filesystem Interface Specification for GoBoy

## Overview

This specification describes a 9P filesystem interface for the GoBoy Game Boy emulator. The interface exposes the emulator's internal state as a virtual filesystem accessible over TCP, enabling save states, live debugging, and state manipulation using standard Unix tools.

### Design Philosophy

The core principle is to provide a live window into emulator state through a filesystem interface. Users can:
- Save state by running `tar` on the mounted filesystem
- Restore state by extracting a tarball while paused
- Monitor state in real-time with tools like `grep`, `watch`, `xxd`
- Modify state surgically with `echo`, `dd`, or hex editors
- Script complex operations using standard shell tools

The filesystem reflects live emulator state. Reads return current values instantly. Writes are queued and applied at frame boundaries to maintain consistency.

## High-Level Architecture

The emulator runs with its normal GUI window, executing the game loop at 60fps. A 9P server runs in a separate goroutine, serving a virtual filesystem over TCP. Multiple emulator instances can run simultaneously on different ports.

### Threading Model

1. **Main goroutine**: Existing game loop (CPU execution, PPU rendering, APU audio, GUI updates)
2. **9P server goroutine**: Handles network connections and file operations
3. **Shared state**: The existing emulator state structs (Gameboy, CPU, Memory, APU, etc.)

### Synchronization Strategy

**Reads:**
- A single mutex protects all emulator state
- 9P read operations acquire the mutex, copy/format the data, release the mutex
- Read latency is negligible (microseconds)
- The emulator continues running during reads

**Writes:**
- 9P write operations parse the data and enqueue commands in a thread-safe queue
- Every frame (at the end of the game loop iteration), the emulator processes all queued commands
- Commands are applied atomically - the entire batch is applied in one go
- Write latency is ~16ms (one frame at 60fps)

**Control commands:**
- `pause`: Sets a flag that halts the game loop
- `resume`: Clears the pause flag
- `reset`: Calls initialization logic to reset emulator state
- Control commands are processed immediately via atomic flags or channels

### Command-Line Interface

```
--9p-port PORT    Enable 9P server on specified TCP port (e.g., 5640)
```

When `--9p-port` is specified:
- The GUI window still opens and renders normally (9P is a side-channel interface)
- The 9P server starts listening on the specified port
- Multiple emulator instances can run simultaneously on different ports

## Filesystem Structure

```
/
  README          (read-only, describes the interface)
  ctl             (read/write, control commands)
  rom             (write-only, load new ROM)
  state/          (directory containing all emulator state)
    cpu
    memory/
      vram
      wram
      oam
      highram
      state
    cartridge/
      info        (read-only)
      state
      ram
    apu/
      state
      waveform
      channel1
      channel2
      channel3
      channel4
    ppu/
      state
      screen
      bgpriority
      tilescanline
      bgpalette
      spritepalette
      dmgpalette
```

## File Formats

### Text Files (Key=Value Format)

All text-based state files use:
- Hex values with `0x` prefix (e.g., `AF=0x01B0`)
- Grouped by logical component with comment section headers (e.g., `# Registers`)
- One key=value pair per line
- Blank lines separate sections for readability

When writing:
- Parser accepts hex (0x1234) or decimal (4660)
- Ignores comments and blank lines
- Order doesn't matter
- No validation of semantic correctness

### Binary Files

Binary files are raw byte dumps:
- Can be manipulated with `dd`, `xxd`, hex editors
- Support offset-based writes for surgical edits
- Must be exactly the expected size (enforced on write)

## Control Interface

### Reading `ctl`

Returns current status and available commands:
```
running

pause - pause emulation
resume - resume emulation
reset - reset to power-on state
```

First line is state (`running` or `paused`), followed by blank line, then command list.

### Writing `ctl`

Send commands:
```bash
echo pause > ctl
echo resume > ctl
echo reset > ctl
```

Commands are processed immediately (not queued like state writes).

### `reset` Command

Returns the emulator to power-on state (as if you just loaded the ROM). Cartridge RAM is cleared unless battery-backed (retains .sav file contents).

## ROM Loading

### `/rom` File

Write-only file for loading ROMs:
```bash
cat ~/roms/tetris.gb > rom
```

After writing a new ROM:
- The emulator loads the new ROM data
- State is reset to power-on
- Battery-backed RAM is loaded from corresponding .sav file if it exists

## State Files

### `state/cpu`

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

All register values are full 16-bit values.

### `state/memory/state`

```
# Banking
VRAMBank=0x00
WRAMBank=0x01

# DMA
hdmaLength=0x00
hdmaActive=0x00
```

### `state/memory/vram`

Binary file: 16KB (0x4000 bytes)
Both banks concatenated (bank 0: 0x0000-0x1FFF, bank 1: 0x2000-0x3FFF)

### `state/memory/wram`

Binary file: 36KB (0x9000 bytes)
All 8 banks (bank 0: 0x0000-0x0FFF, banks 1-7: 0x1000-0x8FFF)

### `state/memory/oam`

Binary file: 256 bytes (0x100 bytes)
Object Attribute Memory (sprite data)

### `state/memory/highram`

Binary file: 256 bytes (0x100 bytes)
High RAM including hardware registers (0xFF00-0xFFFF)

### `state/cartridge/info` (read-only)

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

Reflects the loaded ROM's header data.

### `state/cartridge/state`

Contents depend on MBC type. Example for MBC3:

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

For plain ROM carts, contains only a comment indicating no banking state.

### `state/cartridge/ram`

Binary file: Size varies by MBC type
- MBC1: 32KB (0x8000 bytes)
- MBC2: 8KB (0x2000 bytes)
- MBC3: 32KB (0x8000 bytes)
- MBC5: 128KB (0x20000 bytes)

### `state/apu/state`

```
# Volume
lVol=0x07
rVol=0x07

# Timing
tickCounter=0x0000
```

Sound registers (52 bytes at 0xFF10-0xFF3F) are exposed via `state/memory/highram`.

### `state/apu/waveform`

Binary file: 32 bytes (0x20 bytes)
Channel 3 waveform RAM

### `state/apu/channel1`

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

### `state/apu/channel2`, `channel3`, `channel4`

Similar structure to channel1, but without the sweep section. All channels include:
- Frequency tracking: `frequency` and `time` (current wave position)
- Amplitude control: `amplitude`, `duration`, `length`
- Envelope state: `envelopeVolume`, `envelopeTime`, `envelopeSteps`, `envelopeStepsInit`, `envelopeSamples`, `envelopeIncreasing`
- Output routing: `onL`, `onR`

Note: The `generator` function pointer cannot be serialized - it's reconstructed from sound register state when restoring.

### `state/ppu/state`

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

Includes some global gameboy state (interrupts, input, CGB mode) that doesn't fit elsewhere.

### `state/ppu/screen`

Binary file: 69,120 bytes (160 × 144 × 3)
Raw RGB data. Format: Row-major, top-to-bottom, left-to-right. Each pixel is 3 bytes: R, G, B (0-255 each).

### `state/ppu/bgpriority`

Binary file: 23,040 bytes (160 × 144)
Each byte is 0x00 (false) or 0x01 (true), indicating if that pixel has background priority.

### `state/ppu/tilescanline`

Binary file: 160 bytes
Current scanline's tile color indices.

### `state/ppu/bgpalette` and `state/ppu/spritepalette`

```
# Palette Data
index=0x00
autoIncrement=0x00

# Colors (64 bytes as hex pairs)
data=0x00010203...3E3F
```

The data field contains all 64 palette bytes as a single hex string (128 hex digits).

### `state/ppu/dmgpalette`

```
currentPalette=0x00
```

Global DMG mode palette selection.

## README File

The `/README` file is read-only and provides self-documenting interface help:

```
GoBoy 9P Interface
==================

This filesystem exposes the Game Boy emulator's internal state for inspection
and manipulation using standard Unix tools.

STRUCTURE
---------
/README              This file
/ctl                 Control interface (read status, write commands)
/rom                 Write ROM data to load new game
/state/              Complete emulator state

CONTROL COMMANDS
----------------
Read 'ctl' to see current status and available commands.
Write commands to 'ctl':
  echo pause > ctl   - Pause emulation
  echo resume > ctl  - Resume emulation
  echo reset > ctl   - Reset to power-on state

STATE FILES
-----------
Text files use KEY=0xVALUE format with hex values.
Binary files are raw byte dumps matching internal memory layout.

state/cpu                 CPU registers and program counter
state/memory/vram         Video RAM (16KB binary)
state/memory/wram         Work RAM (36KB binary)
state/memory/oam          Object Attribute Memory (256B binary)
state/memory/highram      High RAM including hardware registers (256B binary)
state/memory/state        Memory banking state
state/cartridge/info      ROM information (read-only)
state/cartridge/state     Cartridge banking and RTC state
state/cartridge/ram       Cartridge RAM (size varies, binary)
state/apu/state           Audio processing unit state
state/apu/waveform        Channel 3 waveform RAM (32B binary)
state/apu/channel1-4      Individual sound channel state
state/ppu/state           Picture processing unit and execution state
state/ppu/screen          Current frame RGB data (160x144x3 binary)
state/ppu/bgpriority      Background priority map (160x144 binary)
state/ppu/tilescanline    Current scanline buffer (160B binary)
state/ppu/bgpalette       CGB background palette
state/ppu/spritepalette   CGB sprite palette
state/ppu/dmgpalette      DMG palette selection

SAVE STATES
-----------
Save:   tar -czf savestate.tar.gz state/
Restore: echo pause > ctl && tar -xzf savestate.tar.gz && echo resume > ctl

Individual state can be modified while running:
  grep PC state/cpu              # Read program counter
  echo 'PC=0x0150' > state/cpu   # Modify program counter

ROM LOADING
-----------
Load a new ROM:
  echo pause > ctl
  cat ~/roms/tetris.gb > rom
  echo reset > ctl
  echo resume > ctl

NOTES
-----
- Reads are instantaneous and reflect live state
- Writes are applied at next frame boundary (~16ms latency)
- No validation - invalid values will crash or corrupt the emulator
- Multiple emulator instances can run on different ports
```

## Error Handling

### Write Errors

**Format errors** (rejected immediately, write fails):
- Invalid syntax: `AF=notahexnumber`
- Unknown keys: `INVALID_REGISTER=0x1234`
- Malformed format: `AF:0x1234` (missing =)
- Binary file wrong size

**Semantic errors** (accepted, may crash emulator):
- Out-of-range values: `romBank=0xFFFF` when ROM only has 32 banks
- Invalid pointers: `PC=0xFFFF` pointing to invalid code
- Inconsistent state: Setting VRAM bank to 2 (only 0-1 exist)

Philosophy: Trust the user. They can always restore from a good save state.

### Multiple Client Connections

The 9P server supports multiple simultaneous connections:
- All clients see the same live state
- Writes from different clients are serialized in the command queue
- No client isolation - one client's writes affect all clients
- No locking between clients - race conditions are possible

### Disconnection and Cleanup

- Client disconnection doesn't affect the emulator
- The emulator continues running even with no connected clients
- Open file handles are cleaned up automatically by the 9P library

## Implementation Considerations

### Code Organization

New packages to add:
- `pkg/ninep/`: 9P server implementation
  - `server.go`: TCP listener and 9P protocol handler
  - `filesystem.go`: Virtual filesystem structure and routing
  - `readers.go`: State reading and formatting logic
  - `writers.go`: Command parsing and queue management
  - `control.go`: Control file implementation

Modifications to existing packages:
- `pkg/gb/gameboy.go`: Add mutex, command queue, and frame-boundary command processing
- `cmd/goboy/main.go`: Add `--9p-port` flag and server initialization
- Each state-containing package may need accessor methods if direct field access is insufficient

### Mutex Granularity

Single coarse-grained mutex protecting all emulator state:
- Simple to implement and reason about
- Minimal contention since reads are fast and writes are queued
- Can optimize later if profiling shows bottlenecks (unlikely)

### Binary File Writes

For binary files, writing partial data is supported:
- The 9P protocol supports offset-based writes
- Queue the offset + data as a command
- Apply at frame boundary: `copy(vram[offset:], data)`

Example: `echo -n '\xFF\xFF' | dd of=state/memory/vram bs=1 seek=1024 conv=notrunc`

### Future Extensibility

The architecture explicitly supports future additions:
- **Input injection**: Add `state/input` file for scripted button presses
- **Debug features**: Expose keyboard shortcuts (background toggle, sprite toggle) as files
- **Breakpoints**: Add `state/breakpoints/` directory for CPU/memory breakpoints
- **Frame stepping**: Add `step` command for single-frame execution
- **Screen recording**: Read frame-by-frame from screen buffer
- **Additional control commands**: Speed adjustment, frame advance, etc.

## Usage Examples

### Basic Monitoring

```bash
# Connect and explore
mount -t 9p tcp!localhost!5640 /mnt/goboy
cat /mnt/goboy/README
cat /mnt/goboy/ctl

# Watch program counter in real-time
watch -n 0.1 'grep PC /mnt/goboy/state/cpu'

# Monitor memory bank switching
watch 'cat /mnt/goboy/state/cartridge/state'
```

### Save and Restore

```bash
cd /mnt/goboy
echo pause > ctl
tar -czf ~/saves/pokemon-$(date +%s).tar.gz state/
echo resume > ctl

# Restore later
cd /mnt/goboy
echo pause > ctl
tar -xzf ~/saves/pokemon-1234567890.tar.gz
echo resume > ctl
```

### Live Modification

```bash
# Give yourself max HP by modifying RAM
echo 'pause' > /mnt/goboy/ctl
xxd /mnt/goboy/state/cartridge/ram | grep ...  # find HP address
echo -n '\xFF' | dd of=/mnt/goboy/state/cartridge/ram bs=1 seek=4096 conv=notrunc
echo 'resume' > /mnt/goboy/ctl

# Warp to different location by changing PC
echo 'PC=0x0150' > /mnt/goboy/state/cpu

# Modify VRAM to create graphical glitches
dd if=/dev/random of=/mnt/goboy/state/memory/vram bs=1024 count=4
```

### ROM Loading

```bash
# Load a different ROM
echo pause > ctl
cat ~/roms/tetris.gb > rom
echo reset > ctl
echo resume > ctl
```

### Scripting

```bash
# Auto-save every 60 seconds while playing
while true; do
    sleep 60
    cd /mnt/goboy
    echo pause > ctl
    tar -czf ~/autosave-$(date +%s).tar.gz state/
    echo resume > ctl
done
```

## Known Limitations

This specification includes fields for complete save state functionality, but some features are not currently implemented in GoBoy's core emulation:

### MBC3 Real-Time Clock Timestamp

The `rtcTimestamp` field in `state/cartridge/state` is included in the specification to support accurate RTC save/restore across sessions. However, **GoBoy does not currently implement real-time clock advancement based on wall-clock time**.

**Current behavior:**
- RTC registers (seconds, minutes, hours, days) are exposed and can be read/written
- The `rtcTimestamp` field can be saved and restored via the 9P interface
- The RTC does not automatically advance based on real-world time passage

**Impact:**
- Games with time-based events (e.g., Pokémon Generation II) will not experience time progression between save state sessions
- Save states will preserve the RTC register values at the moment of saving
- Users can manually modify RTC values via the 9P interface if needed

**For accurate RTC emulation**, GoBoy would need to:
1. Store the Unix timestamp when RTC registers are written
2. On RTC register read, calculate elapsed real-world time since the timestamp
3. Advance the RTC registers by the calculated elapsed time
4. Update the stored timestamp

This is a well-understood implementation pattern used by emulators like mGBA, SameBoy, and Gambatte, as documented in the BESS specification and industry save state formats.

### Audio Frame Sequencer

The APU state files capture most audio timing state (envelope counters, length counters, sweep state), but **GoBoy does not implement the 512Hz audio frame sequencer** that is present in accurate Game Boy emulators.

**Current behavior:**
- Sound channels have per-channel envelope, length, and sweep timing
- The `time` field tracks wave position for accurate phase when restoring save states
- Audio functions correctly for most games

**Impact:**
- Some games with precise audio timing dependencies may have minor audio inaccuracies
- The lack of frame sequencer does not affect save state compatibility, but audio may not be bit-perfect after restore

**For reference**: The frame sequencer is a 512Hz clock (every ~8192 CPU cycles) that coordinates:
- Step 0, 2, 4, 6: Length counter clock
- Step 2, 6: Sweep clock (channel 1 only)
- Step 7: Envelope clock
- Step 1, 3, 5: No operation

Accurate emulators (SameBoy, mGBA, Gambatte) track frame sequencer position in save states. This is documented in the Game Boy audio hardware analysis and BESS specification.
