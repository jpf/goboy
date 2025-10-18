# GoBoy 9P Interface Work Plan

## Project Goal

Implement a 9P filesystem interface for the GoBoy Game Boy emulator, exposing internal emulator state as a virtual filesystem accessible over TCP. This enables:
- Save states using standard Unix tools (tar)
- Live debugging with grep, xxd, watch
- State manipulation with echo, dd, hex editors
- TAS (Tool-Assisted Speedrun) tool integration

## Implementation Status

### ✅ Phase 1: VRAM Access (COMPLETE)

**Commit:** 9f95be5 "Add VRAM access via 9P filesystem interface"

**What Was Implemented:**
- `/README` file with self-documenting interface help
- `/ctl` control file (pause/resume commands)
- `/state/memory/vram` (16KB binary, both VRAM banks)
- Command queue architecture for frame-boundary writes
- Thread-safe reads using RWMutex
- Generic `binaryMemoryFS` pattern for binary files

**Architecture Decisions:**
- Hybrid approach: fs.FS for state logic + native p9.File for protocol
- Command queue with 32-buffer size for tar extraction support
- Accept brief read races (document, users pause for consistency)
- All writes validated before queueing, applied at frame boundaries

**Test Coverage:**
- 54 unit tests passing
- 1,200+ lines of test code
- Race detector clean (`go test -race`)

### ✅ Phase 2: Complete state/memory (COMPLETE)

**Commit:** e408309 "Complete state/memory filesystem implementation via 9P"

**What Was Implemented:**
- `/state/memory/wram` (36KB binary, 8 banks including gap)
- `/state/memory/oam` (256B binary, sprite data)
- `/state/memory/highram` (256B binary, I/O registers)
- `/state/memory/state` (text format, banking state with validation)

**Key Improvements:**
- Generic `binaryMemoryFS` eliminated ~700 lines of code duplication
- Text-based validation for memory state (VRAMBank ≤1, WRAMBank ≤7, hdmaActive ≤1)
- Helper methods on Gameboy (GetWRAM, GetOAM, GetHighRAM, GetMemoryState)
- Command handlers for all 4 memory write commands

**What Works:**
- Full save/restore of memory state via `tar -czf state.tar.gz state/memory/`
- Surgical edits with dd/xxd for debugging
- Validation returns errors synchronously to 9P client
- All binary files support offset-based read/write

### ✅ Phase 3 Part 1: CPU State (COMPLETE)

**Commit:** (completed in previous session)

**What Was Implemented:**
- `/state/cpu` (text format, CPU registers and timer state)
- Helper method `GetCPUState()` on Gameboy
- `cpu-write` command handler for partial register updates
- Text-based key=value format following memoryStateFS pattern
- Support for hex (0x prefix) and decimal values

**Test Coverage:**
- Comprehensive unit tests for all register combinations
- Validation tests for invalid formats and values
- Race detector clean

### ✅ Phase 3 Part 2: Cartridge Info and RAM (COMPLETE)

**Commits:**
- 9134e69 "Add GetCartridge() helper method for 9P access"
- 2a355fe "Add cartridge info file to 9P interface"
- bc36b35 "Fix cartridge info ReadAt to use Read() pattern"
- 4f5d301 "Add GetRAM() method to BankingController interface"
- 13f714f "Add MBC state getter methods for 9P access"
- 2d60a07 "Add cartridge RAM filesystem for save data access"
- 76ada51 "Handle nil RAM gracefully in binaryMemoryFS"
- 5a0a17f "Fix dynamic size handling in p9BinaryFile GetAttr"

**What Was Implemented:**
- `/state/cartridge/info` (read-only ROM header metadata)
  - Parses title, MBC type, ROM/RAM sizes, CGB/SGB flags, checksums
  - Complete MBC type detection (ROM, MBC1, MBC2, MBC3, MBC5)
  - Handles all ROM/RAM size encodings
- `/state/cartridge/ram` (binary save data access)
  - Dynamic size based on MBC type (8KB-128KB)
  - Gracefully handles ROM-only cartridges (returns empty file)
  - Offset-based read/write for save backup/restore
- `GetRAM()` interface method on all MBC types
- `GetBankingState()` methods on all MBC types
- `GetRTCState()` method on MBC3
- `cartridge-ram-write` command handler

**Key Design Decisions:**
- Cartridge info is read-only (ROM header is immutable)
- RAM file returns empty (size 0) for ROM-only cartridges instead of error
- Dynamic size (-1) properly handled in both fs.FS and p9.File layers
- Thread-safe RAM access via GetRAM() interface method

**What Works:**
```bash
# View ROM metadata
cat /mnt/gb/state/cartridge/info

# Backup save data
cat /mnt/gb/state/cartridge/ram > backup.sav

# Restore save data
cat backup.sav > /mnt/gb/state/cartridge/ram
```

### Current State of 9P Specification

From the 9p-spec.md, the complete interface includes:

```
/
  README          ✅ Done
  ctl             ✅ Done (pause/resume working, reset deferred to Phase 5)
  rom             ⏸️ Deferred to Phase 5 (write-only ROM loading)
  state/
    cpu           ✅ Done (Phase 3 Part 1)
    memory/
      vram        ✅ Done
      wram        ✅ Done
      oam         ✅ Done
      highram     ✅ Done
      state       ✅ Done
    cartridge/
      info        ✅ Done (Phase 3 Part 2)
      state       🔄 In Progress (Phase 3 Part 3)
      ram         ✅ Done (Phase 3 Part 2)
    apu/
      state       ⏸️ Phase 4 (optional)
      waveform    ⏸️ Phase 4 (optional)
      channel1-4  ⏸️ Phase 4 (optional)
    ppu/
      state       ⏸️ Phase 4 (optional)
      screen      ⏸️ Phase 4 (optional)
      bgpriority  ⏸️ Phase 4 (optional)
      tilescanline ⏸️ Phase 4 (optional)
      bgpalette   ⏸️ Phase 4 (optional)
      spritepalette ⏸️ Phase 4 (optional)
      dmgpalette  ⏸️ Phase 4 (optional)
```

**Progress:** 9 of 25 files complete (36%)
**Phase 3 Remaining:** 1 file (cartridge/state)

## Phase 3 Analysis: CPU and Cartridge State

### What Phase 3 Aims to Accomplish

Phase 3 adds the remaining components needed for **functional save states**:
1. CPU registers and program counter
2. Cartridge metadata (title, type, sizes)
3. MBC banking state (varies by cartridge type)
4. Cartridge RAM (game save data)

**Note:** ROM loading has been moved to Phase 5 due to implementation complexity unknowns. Users can start the emulator with their desired ROM from the command line and use the 9P interface for save/restore.

After Phase 3, users will have complete gameplay save states. Phase 4 (APU/PPU) is only needed for perfect audio/video reproduction and advanced debugging.

### Honest Assessment of the Phase 3 Plan

#### What I Like About the Plan

1. **Clear prioritization** - Correctly identifies that CPU + cartridge state = functional save states
2. **Reuses existing patterns** - CPU state follows memoryStateFS pattern, RAM uses binaryMemoryFS
3. **Realistic about ROM loading** - Acknowledges it requires investigation of existing code
4. **MBC type detection is sound** - Runtime type switch is the right approach
5. **Test-driven** - Emphasizes TDD discipline throughout

#### What Concerns Me (Being Direct)

**1. ROM Loading is Severely Underspecified** ⚠️ **RESOLVED - Moved to Phase 5**

ROM loading has been deferred to Phase 5 because:
- Unknown if GoBoy supports hot-swapping ROMs without restart
- .sav file loading from memory buffer is unclear
- Reset command doesn't exist yet
- GUI coordination unknown
- Could be 2 hours or 16 hours depending on findings

**Decision:** Get CPU and cartridge state working first. Users can start emulator with desired ROM from CLI and use 9P for save/restore.

**2. Cartridge State Complexity is Underestimated**

The plan needs to:
- Add `GetRAM()` to BankingController interface (affects 5 files)
- Add state getter methods to each MBC type (4 implementations)
- Add state setter methods to each MBC type (4 implementations)
- Handle MBC3 RTC state (13 fields!)
- Validate banking ranges per ROM size (not just type)

Each MBC type is slightly different. The plan groups them together but reality is:
- MBC1: 4 fields (romBank, ramBank, ramEnabled, romBanking)
- MBC2: 3 fields (no romBanking mode)
- MBC3: 16 fields (banking + 11 RTC fields + rtcTimestamp)
- MBC5: 3 fields (but romBank is 9-bit)

**Estimated effort breakdown:**
- Interface changes: 1 hour
- MBC1 state: 1 hour (straightforward)
- MBC2 state: 0.5 hours (simpler than MBC1)
- MBC3 state: 3-4 hours (RTC is complex, 11 fields)
- MBC5 state: 1 hour (9-bit romBank needs care)
- Testing all MBC types: 2-3 hours

That's **8.5-10.5 hours just for cartridge state**, not including CPU or ROM loading.

**3. Missing Validation Strategy**

The plan says "no validation per spec philosophy" for CPU state, but:
- Setting PC to invalid address will crash emulator
- Setting SP outside valid range will corrupt memory
- Invalid Divider values could break timing

I agree with trusting the user, but the plan should explicitly document:
- Which values can crash the emulator (PC, SP)
- Which values are nonsensical but safe (register values)
- How to recover from bad writes (reload save state)

**4. Command Struct is Growing Unwieldy**

Current Command struct:
```go
type Command struct {
    Name           string
    Offset         int
    Data           []byte
    State          map[string]byte
    CPUState       map[string]uint16      // Phase 3
    CartridgeState map[string]interface{} // Phase 3
}
```

This is starting to smell. By Phase 4 it'll have:
- APUState, ChannelState, PPUState, PaletteState (all map[string]interface{})

**Recommendation:** Consider refactoring Command to use an interface or separate command types:
```go
type Command interface {
    Name() string
    Apply(*Gameboy) error
}

type VRAMWriteCommand struct { ... }
type CPUStateCommand struct { ... }
```

This isn't critical for Phase 3, but you should plan for it before Phase 4.

**5. Time Estimate Updated After Removing ROM Loading**

Breaking down the realistic timeline (ROM loading removed):

| Component | Plan Estimate | Updated Estimate | Notes |
|-----------|---------------|------------------|-------|
| CPU state | 2 hours | 2-3 hours | ✅ Straightforward |
| ~~ROM loading~~ | ~~4-6 hours~~ | **Moved to Phase 5** | Deferred |
| Cartridge info | 2 hours | 2-3 hours | ✅ Just parsing |
| Cartridge state | 3-4 hours | 8-10 hours | 🔴 MBC3 RTC complexity |
| Cartridge RAM | 1-2 hours | 2-3 hours | ⚠️ Interface changes |
| Testing | 3-4 hours | 4-6 hours | ⚠️ MBC variations |
| **TOTAL** | **11-16 hours** | **18-25 hours** | Much more manageable |

**Original estimate:** 15-22 hours (with ROM loading)
**Updated estimate:** 18-25 hours (without ROM loading)

The main remaining risks:
- MBC3 RTC state (11 fields is a lot to get right)
- Testing across all MBC types takes longer than expected
- Interface changes to BankingController affect 5 files

### What I'd Do Differently

**✅ DECISION MADE: ROM Loading Moved to Phase 5**

This addresses the biggest risk in Phase 3. With ROM loading deferred:

**Updated Phase 3 Plan (CPU + Cartridge State):**

1. **CPU state first** (2-3 hours)
   - Low risk, high value
   - Get working and committed before cartridge work
   - Builds confidence

2. **Cartridge info** (2-3 hours)
   - Read-only, just parsing
   - No write complexity
   - Quick win

3. **Implement MBC types sequentially** (10-15 hours)
   - Start with ROM-only (no banking) - simplest
   - Then MBC1 (4 fields, well understood)
   - Then MBC2 (3 fields, similar to MBC1)
   - Then MBC5 (3 fields, 9-bit romBank)
   - Finally MBC3 (16 fields including RTC) - save hardest for last

4. **Test each MBC type independently** (4-6 hours)
   - Don't batch testing at end
   - Verify each type works before moving to next
   - Use race detector throughout

**Total: 18-25 hours** - Much more manageable than 24-33 hours with ROM loading.

### Phase 3 Plan: Specific Concerns (Updated)

#### ROM Loading ✅ Moved to Phase 5

ROM loading investigation and implementation has been deferred to Phase 5 (see `phase-5-rom-loading.md`). This removes the biggest unknown from Phase 3.

Users can start the emulator with their desired ROM from command line:
```bash
./goboy --9p-port 5640 pokemon.gb
```

Then use 9P interface for save/restore of game state. ROM loading via filesystem is a "nice to have" feature that can be tackled after core save state functionality works.

#### Cartridge State Validation

The plan includes validation for memory state but says "no validation" for cartridge state. But:

```go
// What happens if user writes:
romBank=0xFFFF  // When ROM only has 32 banks?
ramBank=0x10    // When RAM only has 4 banks?
```

**Recommendation:** Validate against ROM/RAM size from cartridge header:
```go
func (r *MBC1) SetBankingState(romBank, ramBank uint32, ...) error {
    maxROMBank := len(r.rom) / 16384
    if romBank >= uint32(maxROMBank) {
        return fmt.Errorf("romBank=%d exceeds ROM size (max %d)", romBank, maxROMBank-1)
    }
    // ... set state
}
```

This prevents crashes while still trusting user with semantic correctness.

#### MBC3 RTC Timestamp Field

The plan correctly notes that `rtcTimestamp` is exposed but not used. However:

```go
# RTC Real-Time Reference
rtcTimestamp=0x0000000000000000
```

This is a 64-bit Unix timestamp but GoBoy doesn't implement RTC advancement. Questions:

1. Should we allow writes to this field even though it's unused?
2. Should writes return an error?
3. Should we document "writes accepted but ignored"?

**Recommendation:** Accept writes to rtcTimestamp (for forward compatibility if RTC gets implemented), but document in README that it's currently unused.

### What Would Make Phase 3 Stronger

1. **ROM Loading Prototype** - Before committing to the design, write a 20-line prototype:
   ```go
   func TestROMLoading() {
       gb := NewGameboy("test1.gb")
       // Can we load a second ROM?
       gb.memory.LoadCart("test2.gb")
       // Does it work?
   }
   ```

2. **MBC State Getters First** - Implement all the getter methods before writing the filesystem code. This de-risks the "do we have access to the data?" question.

3. **One MBC Type End-to-End** - Fully implement MBC1 (info, state, RAM) before touching MBC2/3/5. Verify the pattern works.

4. **Integration Test Early** - After CPU state is done, do a manual test:
   ```bash
   # Start emulator, play a bit, then:
   cat state/cpu  # Verify we can read PC/registers
   echo "PC=0x0150" > state/cpu  # Verify we can modify
   ```
   Don't wait until everything is done to test the integration.

5. **Explicit Error Paths** - The plan focuses on happy path. Document error handling:
   - ROM write with invalid data → return error, don't load
   - Cartridge state write with out-of-range bank → return error
   - CPU state write with invalid hex → return error

## Recommendation for Joel

### ✅ Decision Made: ROM Loading → Phase 5

This de-risks Phase 3 significantly. You can now proceed with a clear, manageable scope.

### Short Term (Next Session)

**Ready to start Phase 3** with the following approach:

1. **Verify Phase 1/2 work correctly** (1 hour)
   - Manual mount test with diod or similar
   - Verify tar save/restore works
   - Test with actual game (Super Mario Land)
   - Document any bugs found

2. **Start with CPU state** (2-3 hours)
   - Low risk, straightforward implementation
   - Reuses memoryStateFS pattern
   - Get working and committed
   - Builds confidence

3. **Then cartridge info** (2-3 hours)
   - Read-only, just parsing ROM header
   - No write complexity
   - Quick win before tackling MBC state

### Medium Term (Phase 3 Execution: 18-25 hours)

**Implement MBC types sequentially:**
1. ROM-only (simplest, no banking)
2. MBC1 (well understood, 4 fields)
3. MBC2 (similar to MBC1, 3 fields)
4. MBC5 (like MBC1 but 9-bit romBank)
5. MBC3 (save for last, 16 fields with RTC)

**Test each independently:**
- Don't batch testing at end
- Use race detector throughout
- Verify working before next MBC type

### Long Term (After Phase 3)

**You'll have functional save states after Phase 3:**
- CPU state for program counter and registers
- Memory state (already done)
- Cartridge state for banking and save data
- All necessary for gameplay save/restore

**Optional future phases:**
- **Phase 4 (APU/PPU):** Audio/video state for TAS tools and frame-perfect replay
- **Phase 5 (ROM loading):** Hot-swap ROMs without restart (nice to have)

**Consider architecture refactoring before Phase 4:**
- Command struct is getting messy with map[string]interface{} fields
- Consider command interface or separate command types
- Plan this before Phase 4 adds APU/PPU complexity

## What's Good About the Current Implementation

Before being too critical, here's what you've done well:

1. **Generic binaryMemoryFS is excellent** - Eliminated massive code duplication
2. **Test coverage is solid** - 54 tests, race detector clean, 1200+ lines
3. **Thread safety is well thought out** - Command queue pattern is elegant
4. **Documentation is thorough** - README content is comprehensive
5. **YAGNI discipline** - No over-engineering, focused on what's needed
6. **TDD approach** - Tests written first, good habits

The foundation is **very solid**. Phase 3 is ambitious but achievable if you:
- Investigate ROM loading before coding
- Budget realistic time (25-30 hours)
- Implement incrementally (CPU → MBC1 → MBC2 → MBC5 → MBC3)
- Test each component independently
- Stop and ask when blocked

## Key Questions to Answer Before Phase 3

**✅ Resolved:**
1. ~~**ROM Loading:**~~ Deferred to Phase 5 (see phase-5-rom-loading.md)
2. ~~**Reset Command:**~~ Deferred to Phase 5 with ROM loading
3. ~~**Save Files:**~~ Deferred to Phase 5
4. ~~**Time Budget:**~~ Now manageable at 18-25 hours without ROM loading

**Still Need to Answer:**
1. **MBC State:** Do we need SetBankingState() methods or just direct field writes?
   - Recommendation: Start with direct field writes, add setters if needed

2. **Validation:** Bank values - validate against ROM size or trust user completely?
   - Recommendation: Validate against ROM/RAM size from cartridge header

3. **Command Struct:** Refactor now or defer to post-Phase 4?
   - Recommendation: Defer to post-Phase 4, not critical yet

## Summary

**What's Done:**
- Phase 1: VRAM (✅ complete, excellent)
- Phase 2: Memory (✅ complete, excellent)
- Progress: 24% of total 9P spec (6 of 25 files)

**What's Next:**
- ✅ ROM loading moved to Phase 5 (de-risked)
- Phase 3 now focused: CPU + Cartridge state only
- Realistic estimate: 18-25 hours (was 24-33 with ROM loading)
- Clear sequential plan: CPU → info → MBC types (simplest to hardest)

**Bottom Line:**
Phase 3 is now **well-scoped and achievable**. Key improvements:
1. ✅ ROM loading deferred to Phase 5 (biggest risk removed)
2. ✅ Realistic time budget: 18-25 hours
3. ✅ Clear implementation order: CPU → ROM-only → MBC1 → MBC2 → MBC5 → MBC3
4. ⚠️ MBC3 RTC still complex (8-10 hours) - save for last

After Phase 3, you'll have **functional save states** for gameplay. Phase 4 (APU/PPU) and Phase 5 (ROM loading) are optional enhancements.

**Next Steps:**
1. ✅ Verify Phase 1/2 work correctly (DONE - tar extraction working)
2. ✅ Start Phase 3 with CPU state (DONE - 2-3 hours)
3. ✅ Then cartridge info (DONE - 2-3 hours, read-only working)
4. ✅ Add GetRAM() and banking state getters (DONE - 2 hours)
5. ✅ Implement cartridge RAM file (DONE - 2 hours with nil handling)
6. 🔄 **IN PROGRESS:** Cartridge state filesystem (MBC-specific banking state)
   - Estimated: 6-8 hours remaining
   - Read: Type switch on MBC, return appropriate fields
   - Write: Parse and validate, queue cartridge-state-write command
   - Test: Each MBC type separately (ROM, MBC1, MBC2, MBC3, MBC5)

## Phase 3 Part 3: Cartridge State (IN PROGRESS)

### What Remains

**File:** `/state/cartridge/state` (text format, MBC-dependent)

**Format varies by MBC type:**

- **ROM-only:** Empty or "# No banking state"
- **MBC1:** romBank, ramBank, ramEnabled, romBanking (4 fields)
- **MBC2:** romBank, ramBank, ramEnabled (3 fields, ramBank always 0)
- **MBC3:** romBank, ramBank, ramEnabled + 11 RTC fields + timestamp (16 fields total)
- **MBC5:** romBank (9-bit), ramBank, ramEnabled (3 fields)

**Implementation Tasks:**

1. **Create cartridgeStateFS** (similar to memoryStateFS pattern)
   - Read: Type switch to determine MBC, format output
   - Write: Parse key=value, validate fields
   - Handle MBC3 RTC complexity (11 additional fields)

2. **Add p9.File wrapper** (p9cartridgestatefile.go)
   - Standard delegation pattern

3. **Add to cartridgeDir** (statedirs.go)
   - Walk/Readdir/Create/UnlinkAt handling

4. **Add command handler** (gameboy.go)
   - cartridge-state-write command
   - Type switch to apply state per MBC type
   - Add SetBankingState() methods to MBC types (or direct field writes)

5. **Write comprehensive tests**
   - Each MBC type separately
   - Validation for out-of-range values
   - MBC3 RTC state handling
   - Race detector clean

**Estimated Effort:** 6-8 hours
- cartridgeStateFS implementation: 2-3 hours
- MBC3 RTC complexity: 2-3 hours
- Testing all MBC types: 2 hours

**After This:** Phase 3 complete! Functional save states achieved.
