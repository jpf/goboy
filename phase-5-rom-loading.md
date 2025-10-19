# Phase 5: ROM Loading and Reset Command

## Overview

Phase 5 adds the ability to load new ROMs via the 9P filesystem interface and implements the reset command. This is a "nice to have" feature that isn't critical for save state functionality but enables hot-swapping ROMs without restarting the emulator.

**Status:** Not started - deferred from Phase 3 due to complexity unknowns.

## Goals

1. **ROM Loading**: Write ROM data to `/rom` file to load a new game
2. **Reset Command**: Add `reset` command to `/ctl` for power-on state
3. **Save File Handling**: Automatically load .sav file for battery-backed carts

## Investigation Required Before Implementation

### 1. ROM Loading Flow

**Questions to answer:**
- Does `pkg/gb/memory.go:LoadCart()` work with an already-initialized emulator?
- What happens to existing cartridge when new ROM loads?
- Are there destructors or cleanup needed on old cartridge?
- Does GUI need notification of ROM change?
- What happens to emulator state (CPU, PPU, APU) when ROM changes?

**Action:**
```bash
# Trace through existing ROM loading
grep -r "LoadCart" pkg/
grep -r "Cart" pkg/gb/memory.go

# Check for cleanup/destructor patterns
grep -r "Close\|Cleanup\|Destroy" pkg/cart/
```

### 2. Reset Command Implementation

**Questions to answer:**
- Does a reset function already exist?
- What state needs to be reset? (CPU, memory, PPU, APU, cartridge)
- Should reset preserve cartridge RAM (battery-backed)?
- Should reset reload .sav file?

**Action:**
```bash
grep -r "reset\|Reset\|Init" pkg/gb/gameboy.go
grep -r "powerOn\|PowerOn" pkg/
```

### 3. Save File Loading

**Questions to answer:**
- Where does `LoadSaveData()` get called currently?
- How is .sav file path derived from ROM file path?
- Can we load .sav when ROM comes from memory buffer (no file path)?
- What if .sav file doesn't exist for battery-backed cart?

**Action:**
```bash
grep -r "LoadSaveData\|SaveData" pkg/
grep -r "\.sav" pkg/
```

## File Specifications

### `/rom` (Write-Only)

Write-only file for loading new ROMs:
```bash
cat ~/roms/tetris.gb > /mnt/goboy/rom
```

**Behavior:**
- Accepts ROM data in sequential writes
- Validates ROM size and format
- On successful Close():
  - Pauses emulator
  - Loads ROM into cartridge
  - Resets emulator to power-on state
  - Loads .sav file if it exists (how? TBD)
  - Resumes emulator
- Returns error immediately if ROM is invalid

**Format:**
- Minimum size: 32KB
- Maximum size: 8MB
- Must have valid header at 0x0100-0x014F

**Error Cases:**
- ROM too small/large → error on Close()
- Invalid header checksum → error on Close()
- Unknown MBC type → error (or warning?)
- Write called after data written → error (no append)

### `/ctl` - Add `reset` Command

Update control file to support:
```bash
echo reset > /mnt/goboy/ctl
```

**Behavior:**
- Resets emulator to power-on state
- Preserves loaded ROM
- Preserves cartridge RAM if battery-backed (matches real hardware)
- Resets CPU, memory, PPU, APU state
- Does NOT reload ROM or .sav file

**vs. ROM Loading:**
- `reset`: Same ROM, power-on state, keep cartridge RAM
- Load new ROM: Different ROM, power-on state, load new .sav

## Implementation Approach

### Step 1: Investigate Existing Code (2-4 hours)

Before writing any code:

1. **Trace ROM loading flow**
   - Read through `memory.LoadCart()`
   - Check if it works on initialized emulator
   - Document dependencies and side effects

2. **Find or implement reset**
   - Search for existing reset function
   - If none exists, identify what needs reset
   - Document reset requirements

3. **Understand .sav loading**
   - Trace `LoadSaveData()` calls
   - Understand file path derivation
   - Determine if we can load without file path

4. **Write findings document**
   - Create `rom-loading-investigation.md`
   - Document what works, what doesn't
   - Propose implementation approach

### Step 2: Implement Reset Command (2-3 hours)

**If reset function exists:**
- Add `reset` case to ProcessCommands()
- Call existing reset function
- Test with various games

**If reset function doesn't exist:**
- Implement reset method on Gameboy
- Reset: CPU state, memory banks, PPU state, APU state
- Preserve: Loaded ROM, cartridge RAM (if battery-backed)
- Add tests

### Step 3: Implement ROM Loading (6-12 hours, depending on findings)

**File:** `pkg/ninep/romfs.go`

```go
type romFS struct {
    gb *gb.Gameboy
}

type romFile struct {
    parent *romFS
    buf    *bytes.Buffer
}

func (f *romFile) Write(data []byte) (int, error) {
    return f.buf.Write(data)
}

func (f *romFile) Close() error {
    if f.buf.Len() == 0 {
        return nil
    }

    romData := f.buf.Bytes()

    // Validate ROM
    if err := validateROM(romData); err != nil {
        return err
    }

    // Queue rom-load command
    dataCopy := make([]byte, len(romData))
    copy(dataCopy, romData)

    f.parent.gb.CommandChan <- gb.Command{
        Name: "rom-load",
        Data: dataCopy,
    }

    return nil
}
```

**Command Handler:**
```go
case "rom-load":
    gb.Mu.Lock()

    // Approach depends on investigation findings:
    // Option A: If LoadCart works on initialized emulator
    //   - Call memory.LoadCart() with temp file
    // Option B: If need manual loading
    //   - Create new cartridge from ROM data
    //   - Swap out old cartridge
    //   - Reset emulator state
    //   - Load .sav somehow (TBD)

    gb.Mu.Unlock()
```

### Step 4: Handle Save File Loading (TBD)

This is the trickiest part. Options:

**Option A: Require file path in write**
```bash
# Write ROM path instead of ROM data
echo "/Users/joel/roms/tetris.gb" > /mnt/goboy/rom
```
- Pro: Can derive .sav path easily
- Con: Not true filesystem write, feels hacky

**Option B: Accept ROM data, no .sav support**
```bash
cat tetris.gb > /mnt/goboy/rom
# Cart RAM starts empty, user must restore via state/cartridge/ram
```
- Pro: Simple, clean interface
- Con: User loses save data unless they backup/restore manually

**Option C: Two-file interface**
```bash
cat tetris.gb > /mnt/goboy/rom
cat tetris.sav > /mnt/goboy/cartridge/ram  # Load save separately
```
- Pro: Clean separation, flexible
- Con: Two-step process, easy to forget

**Recommendation:** Start with Option B (no auto .sav), document that user should save/restore RAM via `/state/cartridge/ram`.

## Testing Strategy

### Unit Tests

1. **ROM Validation**
   - Valid ROM (32KB, 1MB, 2MB)
   - Too small (< 32KB)
   - Too large (> 8MB)
   - Invalid header checksum

2. **Reset Command**
   - Reset preserves ROM
   - Reset clears CPU state
   - Reset preserves cartridge RAM (battery-backed)
   - Reset clears cartridge RAM (non-battery)

3. **ROM Loading**
   - Load valid ROM
   - Load different ROM after first
   - Load same ROM again
   - Error cases propagate correctly

### Integration Tests

1. **Manual ROM Loading**
   ```bash
   # Start with Mario
   ./goboy --9p-port 5640 mario.gb

   # Load Tetris via 9P
   cat tetris.gb > /mnt/goboy/rom

   # Verify Tetris is running
   cat /mnt/goboy/state/cartridge/info
   ```

2. **Reset Command**
   ```bash
   # Play game for a bit
   # Check PC is not 0x0100
   grep PC /mnt/goboy/state/cpu

   # Reset
   echo reset > /mnt/goboy/ctl

   # Verify PC is 0x0100
   grep PC /mnt/goboy/state/cpu
   ```

3. **Save Data Handling**
   ```bash
   # Load ROM
   cat pokemon.gb > /mnt/goboy/rom

   # Restore save data
   cat pokemon.sav > /mnt/goboy/state/cartridge/ram

   # Verify save loaded correctly
   ```

## Known Limitations

1. **No Automatic .sav Loading**
   - User must manually load save data via `/state/cartridge/ram`
   - Not as convenient as real file-based loading
   - Documented workaround: backup/restore cartridge RAM

2. **GUI State**
   - GUI might not update ROM title on load
   - Window title might still show old ROM name
   - Cosmetic issue only

3. **Pause During Load**
   - Emulator pauses during ROM load (necessary)
   - Could be jarring if not expected
   - Document in README

4. **No ROM Validation Against Real Hardware**
   - Only validates basic structure (size, header)
   - Doesn't check if ROM will actually run
   - User responsible for valid ROMs

## Success Criteria

1. ✅ Reset command works
   - Resets CPU, memory, PPU, APU state
   - Preserves ROM and battery-backed RAM
   - Tests pass with race detector

2. ✅ ROM loading works
   - Can load new ROM via `/rom` write
   - Emulator runs new ROM correctly
   - Invalid ROMs return errors

3. ✅ Save data handling documented
   - README explains how to backup/restore
   - Workaround for missing auto-.sav loading
   - Examples provided

4. ✅ Integration tests pass
   - Can load multiple ROMs in sequence
   - Reset command tested with real games
   - No data races detected

## Estimated Effort

**Investigation Phase:** 2-4 hours
- Trace existing code
- Write findings document
- Propose approach

**Implementation Phase:** 8-16 hours (depending on investigation findings)
- Reset command: 2-3 hours
- ROM loading: 6-12 hours
- Testing: 2-3 hours

**Total: 10-20 hours**

Wide range because investigation might reveal:
- Easy path: LoadCart works, reset exists → 10 hours
- Hard path: Need manual cartridge swap, build reset from scratch → 20 hours

## Dependencies

- Phases 1-4 should be complete before starting Phase 5
- Need working cartridge state (Phase 3) to properly reset
- Investigation findings might change approach significantly

## Next Steps (When Starting Phase 5)

1. Run investigation steps (2-4 hours)
2. Write `rom-loading-investigation.md` with findings
3. Decide on implementation approach based on findings
4. Implement reset command first (smaller, de-risked)
5. Implement ROM loading with chosen approach
6. Test integration thoroughly
7. Update README with examples and limitations
