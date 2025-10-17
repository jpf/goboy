# Game Boy/Color Emulator Save State Formats: Technical Analysis

**Game Boy and Game Boy Color save states remain fragmented across emulators**, with each using proprietary formats that rarely interoperate. Save states capture complete system snapshots including CPU registers, memory banks, video state, audio channels, and timing information. Only one universal standard exists: BESS (Best Effort Save State), pioneered by SameBoy. This analysis examines save state implementations across major GB/GBC emulators, documenting what data gets stored, file formats used, and compatibility limitations.

## Save State Component Analysis

The table below documents which components appear in save states across different emulators and how commonly each element is implemented:

| Component | Frequency | Details | Size Impact | Critical for Accuracy |
|-----------|-----------|---------|-------------|----------------------|
| **CPU Registers (A, F, B, C, D, E, H, L)** | Universal | All 8-bit registers of Sharp LR35902 | 8 bytes | Mandatory |
| **Program Counter (PC)** | Universal | 16-bit address of next instruction | 2 bytes | Mandatory |
| **Stack Pointer (SP)** | Universal | 16-bit stack location | 2 bytes | Mandatory |
| **Interrupt Master Enable (IME)** | Universal | Global interrupt enable flag | 1 bit | Mandatory |
| **Interrupt Enable (IE) Register** | Universal | Which interrupts are enabled (0xFFFF) | 1 byte | Mandatory |
| **Interrupt Flag (IF) Register** | Universal | Pending interrupts (0xFF0F) | 1 byte | Mandatory |
| **Working RAM (8KB)** | Universal | Main system RAM at 0xC000-0xDFFF | 8,192 bytes | Mandatory |
| **Video RAM (8KB DMG, 16KB CGB)** | Universal | Pattern/tile data at 0x8000-0x9FFF | 8,192-16,384 bytes | Mandatory |
| **OAM (Object Attribute Memory)** | Universal | Sprite data at 0xFE00-0xFE9F | 160 bytes | Mandatory |
| **High RAM (127 bytes)** | Universal | Fast RAM at 0xFF80-0xFFFE | 127 bytes | Mandatory |
| **LCD Control (LCDC) Register** | Universal | Display enable, window, sprites (0xFF40) | 1 byte | Mandatory |
| **LCD Status (STAT) Register** | Universal | PPU mode, LYC=LY, interrupts (0xFF41) | 1 byte | Mandatory |
| **Scroll Y (SCY)** | Universal | Vertical scroll position (0xFF42) | 1 byte | Mandatory |
| **Scroll X (SCX)** | Universal | Horizontal scroll position (0xFF43) | 1 byte | Mandatory |
| **Current Scanline (LY)** | Universal | Which line PPU is rendering (0xFF44) | 1 byte | Mandatory |
| **LY Compare (LYC)** | Universal | Scanline compare value (0xFF45) | 1 byte | Mandatory |
| **Window Y (WY)** | Universal | Window Y position (0xFF4A) | 1 byte | Mandatory |
| **Window X (WX)** | Universal | Window X position (0xFF4B) | 1 byte | Mandatory |
| **Background Palette (BGP)** | Universal (DMG) | DMG background colors (0xFF47) | 1 byte | Mandatory for DMG |
| **Object Palettes (OBP0, OBP1)** | Universal (DMG) | DMG sprite colors (0xFF48-0xFF49) | 2 bytes | Mandatory for DMG |
| **CGB Palette Indexes** | Universal (CGB) | BCPS/OCPS palette select (0xFF68/0xFF6A) | 2 bytes | Mandatory for CGB |
| **CGB Palette Data** | Universal (CGB) | 64 bytes background + 64 bytes sprite | 128 bytes | Mandatory for CGB |
| **Sound Channel 1 (Pulse + Sweep)** | Very Common | NR10-NR14 registers, envelope, frequency | 5+ bytes | High |
| **Sound Channel 2 (Pulse)** | Very Common | NR21-NR24 registers, envelope, frequency | 4+ bytes | High |
| **Sound Channel 3 (Wave)** | Very Common | NR30-NR34 + 16-byte wave RAM | 21+ bytes | High |
| **Sound Channel 4 (Noise)** | Very Common | NR41-NR44 registers, LFSR state | 4+ bytes | High |
| **Sound Control (NR50, NR51, NR52)** | Very Common | Master volume, panning, enable (0xFF24-0xFF26) | 3 bytes | High |
| **Timer Counter (TIMA)** | Universal | Timer value (0xFF05) | 1 byte | Mandatory |
| **Timer Modulo (TMA)** | Universal | Timer reload value (0xFF06) | 1 byte | Mandatory |
| **Timer Control (TAC)** | Universal | Timer enable and frequency (0xFF07) | 1 byte | Mandatory |
| **Divider Register (DIV)** | Universal | System counter (0xFF04) | 1-2 bytes | Mandatory |
| **Sub-scanline Position** | Common | PPU dot position within scanline | 1-2 bytes | High for accuracy |
| **PPU Mode** | Very Common | Mode 0/1/2/3 state | 1 byte | High |
| **DMA Source/Dest/Active** | Very Common | OAM DMA state (0xFF46) | 3+ bytes | High |
| **CGB VRAM Bank** | Universal (CGB) | Current VRAM bank 0/1 (0xFF4F) | 1 byte | Mandatory for CGB |
| **CGB WRAM Bank** | Universal (CGB) | Current WRAM bank 1-7 (0xFF70) | 1 byte | Mandatory for CGB |
| **CGB Speed Switch** | Universal (CGB) | Double speed mode state (0xFF4D) | 1 byte | Mandatory for CGB |
| **CGB HDMA State** | Common (CGB) | H-blank DMA registers and progress | 5+ bytes | High for CGB |
| **MBC Type** | Universal | Which memory bank controller | 1 byte | Mandatory |
| **Current ROM Bank** | Universal | Selected ROM bank number | 1-2 bytes | Mandatory |
| **Current RAM Bank** | Universal | Selected RAM bank number | 1 byte | Mandatory |
| **MBC Banking Mode** | Common | Mode select for MBC1 | 1 bit | High for MBC1 |
| **RAM Enable State** | Universal | Whether cart RAM is accessible | 1 bit | Mandatory |
| **RTC Registers (MBC3)** | Common | Seconds, minutes, hours, days, control | 5+ bytes | Mandatory for MBC3 RTC |
| **RTC Latch State (MBC3)** | Common | Latched vs live RTC values | 5 bytes | High for MBC3 RTC |
| **RTC Unix Timestamp** | Common | Real-world time reference | 8 bytes | High for MBC3 RTC |
| **Joypad State** | Very Common | Currently pressed buttons (0xFF00) | 1 byte | Medium |
| **Serial Transfer Data** | Common | SB, SC registers (0xFF01-0xFF02) | 2 bytes | Medium |
| **Extra OAM (0xFEA0-0xFEFF)** | Uncommon | "Prohibited" area behavior | 96 bytes | Low (accuracy) |
| **Bootrom Status** | Uncommon | Whether bootrom unmapped (0xFF50) | 1 bit | Low |
| **Wave RAM Position** | Common | Channel 3 current sample position | 1 byte | High for audio |
| **Audio Frame Sequencer** | Common | 512Hz sequencer position | 1 byte | High for audio |
| **Envelope Counters** | Common | Per-channel envelope timers | 3+ bytes | High for audio |
| **Sweep Counter (CH1)** | Common | Frequency sweep timer state | 1+ bytes | High for audio |
| **Length Counters** | Common | Per-channel length timers | 4 bytes | High for audio |
| **Screenshot/Thumbnail** | Rare | Preview image of game state | 23-46KB | None (UI only) |
| **ROM CRC32/Checksum** | Common | Verification of correct ROM | 4 bytes | Medium (validation) |
| **Save State Version** | Common | Format version identifier | 2-4 bytes | High (compatibility) |
| **Timestamp/Metadata** | Uncommon | When state was created | Variable | None (UI only) |

**Frequency Legend:**
- **Universal**: Found in virtually all emulators (95-100%)
- **Very Common**: Found in most emulators (75-95%)
- **Common**: Found in many emulators (50-75%)
- **Uncommon**: Found in some emulators (25-50%)
- **Rare**: Found in few emulators (<25%)

**Typical Save State Sizes:**
- **Game Boy (DMG)**: 10-20KB uncompressed
- **Game Boy Color**: 20-50KB uncompressed
- **With Screenshots**: +23KB (160×144) or +46KB (256×224)
- **Compressed**: 5-15KB depending on game state and algorithm

## Modern Emulators Lead with Open Documentation

### mGBA (GB/GBC support)

**mGBA provides the best-documented format** through fully specified source code in `include/mgba/internal/gb/serialize.h`. Save states use `.ss1` through `.ss9` extensions containing:

- **Magic number**: `0x00400003` identifies GB format version
- **Version field**: Allows format evolution
- **ROM CRC32**: Verifies correct game loaded
- **Complete system state**: All registers, memory, and hardware state
- **Extdata system**: Extensible blocks for screenshots, cheats, metadata
- **Optional PNG compression**: State data stored in custom PNG chunks

The structure uses explicit sizing and C structs that directly map to binary layout. Every field is documented in header comments. The serialization code in `src/gb/serialize.c` shows exact save/load process.

### SameBoy + BESS Standard

**SameBoy revolutionized compatibility with BESS** (Best Effort Save State), the only successful cross-emulator standard. BESS works by appending implementation-agnostic blocks to native save states, allowing files to function both as full-featured native states and portable cross-emulator states.

**BESS block structure** (defined at github.com/LIJI32/SameBoy/blob/master/BESS.md):

- **CORE block** (0xD0 bytes): Contains version, model ID, all CPU registers (PC, AF, BC, DE, HL, SP), IME flag, IE register, execution mode, and memory pointers with offsets/sizes for RAM, VRAM, MBC RAM, OAM, HRAM, and palettes
- **NAME block**: Optional ROM name for validation
- **INFO block**: Optional emulator name and extra information
- **XOAM block** (0x60 bytes): Extra OAM data at 0xFEA0-0xFEFF
- **MBC block**: Variable-length (address, value) pairs representing MBC register writes
- **RTC block**: Real-time clock state using VBA/BGB-compatible format
- **HUC3 block** (0x11 bytes): HuC3 mapper RTC state
- **TPP1 block** (0x11 bytes): TPP1 mapper RTC state (modern homebrew)
- **MBC7 block** (0x0A bytes): EEPROM and motion sensor state
- **END block**: Marks end of BESS data

**Memory management through offsets** avoids duplication—BESS stores absolute file offsets pointing to data in native format rather than duplicating RAM dumps. Size mismatches (DMG 8KB vs CGB 16KB VRAM) handle gracefully.

**Model identification** uses two-letter codes: first letter indicates family (G=GB, S=SGB, C=CGB), second letter indicates specific model (D=DMG, M=MGB, L=CGB/GBC).

**Adoption status**: SameBoy, BGB, and Emulicious implement BESS. Other major emulators stick with native formats. Users can't transfer states between mGBA and Gambatte despite both supporting GB/GBC.

### BGB

**BGB remains the developer standard** despite proprietary format. Uses `.sn1` through `.sn9` extensions with preview screenshots built into files. The format is entirely undocumented and closed-source, but BGB 1.0 introduced versioning after 0.x versions lacked it and would crash loading newer states.

BGB supports BESS as fallback—can export/import BESS format while using native `.sn#` for full-featured states. This provides best-of-both-worlds: full feature preservation natively, portability via BESS export when needed.

## What Game Boy Save States Must Store

### CPU State (Sharp LR35902)

The CPU has minimal registers compared to modern processors:

- **Seven 8-bit registers**: A (accumulator), F (flags), B, C, D, E, H, L
- **Can pair as 16-bit**: AF, BC, DE, HL for 16-bit operations
- **16-bit Stack Pointer (SP)**: Points to current stack position
- **16-bit Program Counter (PC)**: Address of next instruction
- **Interrupt Master Enable (IME)**: Global interrupt enable flag

The F (flags) register contains critical condition code bits: Zero, Subtract, Half-carry, Carry. These affect conditional jumps and must be preserved exactly.

**Interrupt state** requires IE register (which interrupts enabled), IF register (which pending), and IME flag. The timing of when interrupts get checked matters for accuracy—some games rely on precise interrupt handling.

### Memory Dumps

Game Boy memory totals **65,536 bytes addressable** but not all physically present:

- **Working RAM (8KB)**: 0xC000-0xDFFF, main system RAM
- **Echo RAM (8KB)**: 0xE000-0xFDFF, mirrors WRAM (usually ignored)
- **Video RAM (8KB)**: 0x8000-0x9FFF, pattern/tile data
- **OAM (160 bytes)**: 0xFE00-0xFE9F, sprite attributes (40 sprites × 4 bytes)
- **Unusable (96 bytes)**: 0xFEA0-0xFEFF, hardware bug makes this unreliable
- **High RAM (127 bytes)**: 0xFF80-0xFFFE, fast internal RAM
- **I/O Registers**: 0xFF00-0xFF7F, hardware control

**Game Boy Color expands memory**:
- **VRAM banks**: 16KB total (8KB × 2 banks selectable)
- **WRAM banks**: 32KB total (8KB × 4 banks, plus 4KB fixed)
- **Palette memory**: 64 bytes background palettes, 64 bytes sprite palettes

All I/O registers must be saved since they control every subsystem. Missing even obscure registers breaks games with unusual hardware timing.

### Video State (PPU)

The Picture Processing Unit renders 160×144 screen at 59.7Hz. Save states need:

- **LCD Control (LCDC)**: Display on/off, window, sprites, tile maps
- **LCD Status (STAT)**: Current PPU mode, LYC=LY interrupt
- **Scroll positions**: SCY, SCX for background scrolling
- **Current scanline (LY)**: Which line (0-153) currently rendering
- **LY Compare (LYC)**: Triggers STAT interrupt when LY=LYC
- **Window positions**: WY, WX for window layer
- **Current PPU mode**: Mode 0 (H-blank), 1 (V-blank), 2 (OAM search), 3 (pixel transfer)
- **Dot position within scanline**: Sub-scanline timing (0-455 dots)

**Palettes differ by model**:
- **DMG**: BGP (background), OBP0/OBP1 (sprites), 2-bit per pixel
- **CGB**: BCPS/BCPD and OCPS/OCPD for 64 palettes, 15-bit color per entry

**Timing accuracy matters enormously**. Games that change palettes mid-frame or rely on STAT interrupts need precise sub-scanline state. Simple implementations store only scanline number and mode, causing graphical glitches. Accurate implementations store dot counter and mode transition timing.

### Audio State (APU)

The Audio Processing Unit mixes four channels:

**Channel 1 (Pulse with Sweep)**:
- NR10: Sweep control (time, direction, shift)
- NR11: Duty cycle and length
- NR12: Volume envelope
- NR13/NR14: Frequency (11-bit)
- Internal state: Envelope counter, sweep counter, length counter

**Channel 2 (Pulse)**:
- NR21-NR24: Same structure as Channel 1 but no sweep
- Internal state: Envelope counter, length counter

**Channel 3 (Wave)**:
- NR30-NR34: Control registers
- Wave RAM: 16 bytes (32 4-bit samples)
- Internal state: Current sample position, length counter

**Channel 4 (Noise)**:
- NR41-NR44: Control registers
- Internal state: LFSR (Linear Feedback Shift Register) value, envelope, length

**Master control**:
- NR50: Master volume
- NR51: Panning (which channel to which speaker)
- NR52: Sound on/off, channel enable status

**Critical internal state** includes envelope counters (volume changes), length counters (auto-stop), sweep counter (frequency changes), and frame sequencer position (512Hz for length/envelope/sweep). Missing these causes wrong volume, wrong frequency, or channels cutting off early.

**Sample position for Channel 3** determines which of 32 wave samples currently plays. Getting this wrong causes phase shifts in audio. The LFSR value for Channel 4 determines noise pattern—restore incorrectly and you get different noise.

### Timer State

The timer system uses four registers:

- **DIV (0xFF04)**: Divider register, increments at 16384Hz, write resets to 0
- **TIMA (0xFF05)**: Timer counter, increments at rate set by TAC
- **TMA (0xFF06)**: Timer modulo, loads into TIMA on overflow
- **TAC (0xFF07)**: Timer control (enable + frequency select)

**Internal timing state** includes the divider's full 16-bit value (only upper 8 bits visible at 0xFF04). TIMA can be mid-increment, requiring precise timing state. Games use timer interrupts for music tempo, so wrong timing breaks music.

### DMA State

**OAM DMA** (0xFF46) copies 160 bytes from memory to OAM in 160 microseconds. During DMA, CPU can only access HRAM. Save states need:

- Source address (which 256-byte page)
- Bytes remaining to transfer
- Whether DMA currently active

Missing DMA state causes sprite glitches if state saved mid-DMA.

**CGB adds H-Blank DMA** (HDMA1-HDMA5 registers) for transferring data during H-blank. This needs source, destination, length remaining, and whether general-purpose or H-blank mode.

### Cartridge State

**Every MBC type requires different state**:

**MBC1** (most common):
- Current ROM bank (0-127)
- Current RAM bank (0-3)
- Banking mode (ROM banking vs RAM banking)
- RAM enable flag

**MBC2**:
- Current ROM bank (0-15)
- RAM enable flag
- 512×4-bit built-in RAM state

**MBC3**:
- Current ROM bank (0-127)
- Current RAM bank (0-3) or RTC register select (8-12)
- RAM/RTC enable flag
- RTC registers: Seconds, Minutes, Hours, Days (9-bit), Control
- Latched RTC values (separate copy)
- Unix timestamp for real-time tracking

**MBC5**:
- Current ROM bank (9-bit: 0-511)
- Current RAM bank (0-15)
- RAM enable flag

**MBC7** (tilt sensor):
- Current ROM bank
- EEPROM data (256 bytes)
- Accelerometer X/Y values

**No MBC** (32KB games):
- Only RAM enable state if 8KB RAM present

**Modern homebrew** uses TPP1 mapper with different RTC format than MBC3, requiring separate handling in BESS TPP1 block.

## RetroArch Cores Show Format Diversity

RetroArch's libretro API standardizes the interface but not the format. Cores implement three functions:

- `retro_serialize_size()`: Returns buffer size needed
- `retro_serialize()`: Writes state to buffer
- `retro_unserialize()`: Restores state from buffer

Save states use `.state` extension with numbered slots (.state1, .state2, etc.) containing core-provided data wrapped minimally by RetroArch.

### Gambatte

**Gambatte prioritizes speed and determinism** over format documentation. Uses custom StateSaver class with explicit struct serialization. The 2018.04.20 update broke compatibility—earlier versions omitted `duty.high` boolean variables for sound channels, causing random crashes on 3DS and Vita until fixed in PR #117.

The format stores CPU registers, all memory regions, PPU state with sub-scanline position, complete APU state with internal counters, timer values, and MBC state. No compression used for netplay/rewind performance.

### SameBoy core

The **RetroArch SameBoy core** differs from standalone SameBoy—it uses libretro `.state` format rather than native `.sbs` or BESS. The core can theoretically export BESS through RetroArch's save converter API, but few users know this exists.

SameBoy's extreme accuracy includes Game Boy Printer emulation, storing printer buffer and status in save states. This adds ~100KB for the print buffer.

### Gearboy core

**Gearboy** passes Blargg's accuracy tests with deterministic states. The format stores Z80-style CPU state (modified for LR35902 differences), 8KB RAM, PPU/APU state, MBC state including RTC data.

It supports bootrom (dmg_boot.bin, cgb_boot.bin) and custom DMG palettes, storing bootrom execution state and selected palette. This causes compatibility issues—states with bootrom enabled won't load if bootrom file missing.

### TGB Dual

**TGB Dual uniquely serializes two complete Game Boy instances** for link cable emulation. It stores dual states sequentially with proper offset handling. Screen layout (horizontal/vertical/top-only/bottom-only) gets stored since it affects internal buffer layout.

The libretro TGB Dual states are completely incompatible with standalone TGB Dual which uses `.sv0` through `.sv9` extensions. Conversion is impossible without reverse-engineering both formats.

## Legacy Emulators (1997-2004)

### VisualBoyAdvance (GB/GBC support)

**Original VBA** (last updated 2004) supported GB/GBC before focusing on GBA. Used `.sgm` files with uncompressed binary dumps. The format lacked versioning initially, causing crashes when loading states from different versions.

**VBA-M fork** inherited GB/GBC support but **broke all compatibility** due to audio core changes. Memory alignment and pointer size differences between 32-bit and 64-bit builds further fragment states. VBA-M's own versions often can't load each other's states.

The format stores LR35902 registers, all memory regions, sound state (sometimes incorrectly), video state, and MBC state. Quality varies—some versions save incomplete audio state causing wrong sound on restore.

**Recommendation**: Don't rely on VBA/VBA-M for GB/GBC save states. Use in-game battery saves instead.

### KiGB

**KiGB version 2.05** (2008) supports Windows/Linux/MS-DOS with portable compressed states in STATE folder. Shift+F1-F10 saves, F1-F10 loads across 10 slots.

The compressed format works identically across all three operating systems, impressive for 2008. File sizes typically 5-10KB compressed. States include CPU registers, memory, sound channels, RTC data, video controller state, and input state.

Documentation in readme.txt explains features but format remains proprietary. The compression and OS portability suggest careful engineering, though source code was never released.

### TGB Dual (standalone)

**TGB Dual and forks** (TGB Dual L, TGB Dual Kai) use `.sv0` through `.sv9` for states, `.sav` for GB1 battery saves, `.sa2` for GB2 battery saves in dual mode.

The format works only within TGB family. States sometimes appear grayed out when filename doesn't exactly match ROM name—the format stores ROM name for validation and rejects mismatches.

Japanese documentation with fan translations provides usage instructions but no format specification. Link cable state includes both Game Boys' complete states plus cable transmission state.

### GnuBoy

**GnuBoy** provides ports across multiple platforms (Linux, Windows, DOS, Wii, GameCube, Dreamcast, Dingoo). **GnuBoy GX** on Wii uses `.gbz` extension with LZSS compression.

VMU save states on Dreamcast compress to 16-20 blocks typically (up to 360 worst case). Command interface uses `savestate` and `loadstate` commands with 10 slots numbered 0-9.

Source at rofl0r/gnuboy on GitHub shows implementation but no formal specification. Compression algorithm selection (LZSS) optimizes for limited storage on portable platforms.

### BGB Evolution

**BGB started as Gameboybouken** before renaming. Version 1.0 introduced state versioning after 0.x versions crashed loading newer states. Modern BGB (1.6.6+) shows screenshot previews in state selection UI.

The format stores complete system state with screenshot as PNG data embedded in file. User manual at bgb.bircd.org/manual.html documents features but not binary format.

**BESS support** added later provides export/import capability. Users can export native BGB state as BESS for use in SameBoy/Emulicious, then reimport modified BESS back into BGB.

## Specialized and Niche Emulators

### GBE+ (GB Enhanced Plus)

**GBE+** focuses on obscure accessories: GB Camera, Mobile Adapter, Barcode Boy, Singer IZEK. Save states added July 2016 using binary format serializing entire system state.

F5 saves, F6 loads across numbered slots. Extensive PDF/ODT manuals document features but not binary format. Source code (GPL v2) shows serialization in core.cpp—each component implements SaveState()/LoadState() methods.

GB Camera state includes camera interface registers and captured image buffers. Mobile Adapter state includes network buffers and connection state. This adds kilobytes depending on accessory state.

### GiiBiiAdvance (GB/GBC)

**GiiBiiAdvance** by AntonioND pioneered full GB Camera emulation using webcam. SDL2-based, cross-platform via CMake. Optional Lua scripting support.

Documentation emphasizes "not actively recommended for playing" since better alternatives exist. Focus was accuracy for GB Camera hardware emulation and development. Source available under GPL v2 but state format not explicitly documented.

### GEST and hhugboy

**GEST** (Game Boy Emulation SysTem) v1.1.1 bases on VBA architecture, making save states VBA-compatible. **hhugboy** forks GEST adding unlicensed mapper support (Sachen, Sintax, etc.).

Both use GPL v2, full source on GitHub. Format compatibility with VBA means same fragmentation issues—different versions break states. The unlicensed mapper support in hhugboy adds new MBC state fields not in original GEST.

## Best Practices for Implementation

### Version from Day One

BGB's 0.x version crisis shows the cost of missing versioning. Add version magic number as first 4 bytes. Use format like:

```
Bytes 0-3:  Magic number (e.g., 0x47425353 = "GBSS")
Bytes 4-7:  Version (e.g., 0x00010000 = version 1.0)
Bytes 8+:   State data
```

This allows rejecting incompatible states immediately and enables format evolution.

### Handle Endianness

If cross-platform, document whether format uses little-endian or big-endian. Game Boy itself uses little-endian for multi-byte values. Modern x86/ARM processors use little-endian, so most emulators match hardware.

Use explicit byte ordering rather than raw struct writes. This prevents 32-bit ARM mobile states breaking on 64-bit x86 desktop.

### Serialize Timing State

Don't just save register values—save timing state:

- **PPU dot counter**: Where in 456-dot scanline
- **Frame sequencer position**: 512Hz audio timing
- **Envelope/length/sweep counters**: Per-channel timers
- **DIV full 16-bit value**: Not just visible upper 8 bits
- **Interrupt timing**: When interrupt just occurred

Games with raster effects, STAT interrupts, or precise audio timing break without this.

### Store RTC Real-Time Reference

For MBC3 with RTC, store both hardware registers and Unix timestamp. On restore, calculate elapsed real time and update registers. This maintains clock accuracy across sessions.

Format example:
```
struct RTC_State {
    uint8_t seconds;     // RTC seconds register
    uint8_t minutes;     // RTC minutes register
    uint8_t hours;       // RTC hours register
    uint16_t days;       // RTC days (9-bit)
    uint8_t control;     // RTC control
    uint8_t latch_seconds; // Latched values
    uint8_t latch_minutes;
    uint8_t latch_hours;
    uint16_t latch_days;
    uint8_t latch_control;
    uint64_t unix_time;  // Real-world reference
};
```

### Consider BESS for Portability

Implementing BESS alongside native format costs minimal effort and provides huge user benefit. Store native state first, append BESS blocks at end. Single file works both ways.

BESS memory offsets point to native format data, avoiding duplication. Just append CORE and END blocks at minimum for basic portability.

### Compress at Storage Layer

Don't compress during serialization—compress saved file after. This preserves netplay/rewind performance while reducing disk space. Most states compress 50-70% with zlib/gzip.

RetroArch explicitly requires uncompressed data from cores for this reason. Compression should be transparent to serialization code.

## Cross-Emulator Compatibility Reality

**Only BESS-compliant emulators share states**: SameBoy, BGB, Emulicious. Every other emulator uses incompatible format.

**Battery saves (.sav files) have much better compatibility** than save states. Most emulators agree on .sav format since it's just raw SRAM/EEPROM/Flash dumps matching physical cartridge formats.

**Conversion tools don't exist for save states** because the problem is intractable. Each format embeds complete emulator architecture. Converting requires deep understanding of both source and target internals, and some information may not map at all.

**The recommended workflow**:
1. Use save states for quick saves during play sessions
2. Use in-game battery saves for long-term progress
3. Back up both formats separately
4. If switching emulators, rely on battery saves not states

**For developers**: Document your format or implement BESS. Source code availability helps but doesn't guarantee stability. Explicit specifications reduce community reverse-engineering effort. BESS provides interoperability with minimal implementation cost.

## Summary

Game Boy/Color save states remain fragmented 25+ years after the platform launched. **Only BESS provides genuine cross-emulator compatibility**, but adoption remains limited to three emulators (SameBoy, BGB, Emulicious).

**What's stored**: CPU registers, 8-16KB RAM, 8-16KB VRAM, PPU state with sub-scanline timing, complete APU state with counters, timer state, MBC registers including RTC, interrupt state, and optionally screenshots for UI. Total size 10-50KB uncompressed, 5-15KB compressed.

**Compatibility**: Battery saves work everywhere. Save states lock you to specific emulator versions. Switching emulators means losing states but keeping battery saves.

**Recommendation for users**: Prefer mGBA (best all-around), SameBoy (most accurate), or BGB (best debugger). All three actively maintained with documented or BESS-compatible formats.

**Recommendation for developers**: Implement versioning from day one, document your format, serialize timing state not just registers, and strongly consider BESS support for user benefit. The fragmentation comes from proprietary formats—BESS proves the technical problem is solved if emulators adopt it.