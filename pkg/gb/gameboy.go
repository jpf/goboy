package gb

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/Humpheh/goboy/pkg/apu"
	"github.com/Humpheh/goboy/pkg/cart"
)

const (
	// ClockSpeed is the number of cycles the GameBoy CPU performs each second.
	ClockSpeed = 4194304
	// FramesSecond is the target number of frames for each frame of GameBoy output.
	FramesSecond = 60
	// CyclesFrame is the number of CPU cycles in each frame.
	CyclesFrame = ClockSpeed / FramesSecond
)

// Command represents a control command from the 9P interface.
type Command struct {
	Name     string
	Offset   int                // For binary writes (vram, wram, oam, highram)
	Data     []byte             // For binary writes
	State    map[string]byte    // For text state writes (memorystate)
	CPUState map[string]uint16  // For CPU state writes
}

// Gameboy is the master struct which contains all of the sub components
// for running the Gameboy emulator.
type Gameboy struct {
	options gameboyOptions

	// Core components of the Gameboy.
	memory *Memory
	cpu    *CPU
	sound  *apu.APU

	Debug  DebugFlags
	paused bool

	// 9P server support (exported for testing)
	Mu          sync.RWMutex
	CommandChan chan Command

	timerCounter int

	// Matrix of pixel data which is used while the screen is rendering. When a
	// frame has been completed, this data is copied into the PreparedData matrix.
	screenData [ScreenWidth][ScreenHeight][3]uint8
	bgPriority [ScreenWidth][ScreenHeight]bool

	// Track colour of tiles in scanline for priority management.
	tileScanline    [ScreenWidth]uint8
	scanlineCounter int
	screenCleared   bool

	// PreparedData is a matrix of screen pixel data for a single frame which has
	// been fully rendered.
	PreparedData [ScreenWidth][ScreenHeight][3]uint8

	interruptsEnabling bool
	interruptsOn       bool
	halted             bool

	cbInst [0x100]func()

	// Mask of the currently pressed buttons.
	inputMask byte

	// Flag if the game is running in cgb mode. For this to be true the game
	// rom must support cgb mode and the option be true.
	cgbMode       bool
	bgPalette     *cgbPalette
	spritePalette *cgbPalette

	currentSpeed byte
	prepareSpeed bool

	thisCpuTicks int

	keyHandlers map[Button]func()
}

// Update update the state of the gameboy by a single frame.
func (gb *Gameboy) Update() int {
	if gb.paused {
		return 0
	}

	cycles := 0
	for cycles < CyclesFrame*gb.getSpeed() {
		cyclesOp := 4
		if !gb.halted {
			if gb.Debug.OutputOpcodes {
				LogOpcode(gb, false)
			}
			cyclesOp = gb.ExecuteNextOpcode()
		} else {
			// TODO: This is incorrect
		}
		cycles += cyclesOp
		gb.updateGraphics(cyclesOp)
		gb.updateTimers(cyclesOp)
		cycles += gb.doInterrupts()

		gb.sound.Buffer(cyclesOp, gb.getSpeed())
	}
	return cycles
}

// togglePaused switches the paused state of the execution.
func (gb *Gameboy) togglePaused() {
	gb.Mu.Lock()
	defer gb.Mu.Unlock()
	gb.paused = !gb.paused
}

// IsPaused returns whether the emulator is currently paused.
// This method is safe to call from other goroutines.
func (gb *Gameboy) IsPaused() bool {
	gb.Mu.RLock()
	defer gb.Mu.RUnlock()
	return gb.paused
}

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

// GetVRAM returns a pointer to the VRAM array for 9P interface access.
// Thread safety: Callers must acquire Mu.RLock() before reading.
func (gb *Gameboy) GetVRAM() *[0x4000]byte {
	return &gb.memory.VRAM
}

// GetWRAM returns pointer to WRAM array for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetWRAM() *[0x9000]byte {
	return &gb.memory.WRAM
}

// GetOAM returns pointer to OAM array for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetOAM() *[0x100]byte {
	return &gb.memory.OAM
}

// GetHighRAM returns pointer to HighRAM array for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetHighRAM() *[0x100]byte {
	return &gb.memory.HighRAM
}

// GetMemoryState returns memory banking and DMA state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetMemoryState() (vramBank, wramBank, hdmaLength byte, hdmaActive bool) {
	return gb.memory.VRAMBank, gb.memory.WRAMBank, gb.memory.hdmaLength, gb.memory.hdmaActive
}

// GetCPUState returns CPU register and timer state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetCPUState() (af, bc, de, hl, sp, pc uint16, divider int) {
	return gb.cpu.AF.HiLo(), gb.cpu.BC.HiLo(), gb.cpu.DE.HiLo(),
		gb.cpu.HL.HiLo(), gb.cpu.SP.HiLo(), gb.cpu.PC, gb.cpu.Divider
}

// GetCartridge returns the loaded cartridge for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetCartridge() *cart.Cart {
	if gb.memory == nil {
		return nil
	}
	return gb.memory.Cart
}

// SetMemory sets the memory pointer for testing purposes.
func (gb *Gameboy) SetMemory(mem *Memory) {
	gb.memory = mem
}

// SetCPU sets the CPU pointer for testing purposes.
func (gb *Gameboy) SetCPU(cpu *CPU) {
	gb.cpu = cpu
}

// GetAPU returns the APU for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetAPU() *apu.APU {
	return gb.sound
}

// SetAPU sets the APU pointer for testing purposes.
func (gb *Gameboy) SetAPU(sound *apu.APU) {
	gb.sound = sound
}

// GetAPUState returns APU state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetAPUState() (playing byte, memory [52]byte, lVol, rVol, tickCounter float64) {
	return gb.sound.GetAPUState()
}

// GetPPUState returns PPU internal state for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetPPUState() (scanlineCounter int, screenCleared, cgbMode bool) {
	return gb.scanlineCounter, gb.screenCleared, gb.cgbMode
}

// SetPPUState updates PPU internal state from 9P writes.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) SetPPUState(scanlineCounter int, screenCleared, cgbMode bool) {
	gb.scanlineCounter = scanlineCounter
	gb.screenCleared = screenCleared
	gb.cgbMode = cgbMode
}

// GetTileScanline returns pointer to tileScanline array for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetTileScanline() *[160]uint8 {
	return &gb.tileScanline
}

// GetBGPalette returns pointer to serialized bgPalette data for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetBGPalette() *[66]byte {
	var data [66]byte
	copy(data[0:64], gb.bgPalette.Palette)
	data[64] = gb.bgPalette.Index
	if gb.bgPalette.Inc {
		data[65] = 0x01
	} else {
		data[65] = 0x00
	}
	return &data
}

// SetBGPalette sets the background palette for testing purposes.
func (gb *Gameboy) SetBGPalette(pal *cgbPalette) {
	gb.bgPalette = pal
}

// GetSpritePalette returns pointer to serialized spritePalette data for 9P access.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) GetSpritePalette() *[66]byte {
	var data [66]byte
	copy(data[0:64], gb.spritePalette.Palette)
	data[64] = gb.spritePalette.Index
	if gb.spritePalette.Inc {
		data[65] = 0x01
	} else {
		data[65] = 0x00
	}
	return &data
}

// SetSpritePalette sets the sprite palette for testing purposes.
func (gb *Gameboy) SetSpritePalette(pal *cgbPalette) {
	gb.spritePalette = pal
}

// GetBGPriority returns bit-packed bgPriority array for 9P access.
// Caller must hold Gameboy.Mu lock.
// Packs 160x144 bools into 2880 bytes (8 bools per byte).
func (gb *Gameboy) GetBGPriority() *[2880]byte {
	var data [2880]byte
	for x := 0; x < ScreenWidth; x++ {
		for y := 0; y < ScreenHeight; y++ {
			if gb.bgPriority[x][y] {
				byteIndex := x*18 + y/8 // 144/8 = 18 bytes per column
				bitIndex := uint(y % 8)
				data[byteIndex] |= 1 << bitIndex
			}
		}
	}
	return &data
}

// SetBGPriority sets a single bgPriority value for testing purposes.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) SetBGPriority(x, y int, value bool) {
	gb.bgPriority[x][y] = value
}

// GetScreen returns flattened PreparedData array for 9P access.
// Caller must hold Gameboy.Mu lock.
// Flattens 160x144x3 RGB array into 69,120 bytes (column-major layout).
func (gb *Gameboy) GetScreen() *[69120]byte {
	var data [69120]byte
	for x := 0; x < ScreenWidth; x++ {
		for y := 0; y < ScreenHeight; y++ {
			offset := x*ScreenHeight*3 + y*3
			data[offset] = gb.PreparedData[x][y][0]     // R
			data[offset+1] = gb.PreparedData[x][y][1]   // G
			data[offset+2] = gb.PreparedData[x][y][2]   // B
		}
	}
	return &data
}

// SetScreen sets a single pixel in PreparedData for testing purposes.
// Caller must hold Gameboy.Mu lock.
func (gb *Gameboy) SetScreen(x, y int, r, g, b uint8) {
	gb.PreparedData[x][y][0] = r
	gb.PreparedData[x][y][1] = g
	gb.PreparedData[x][y][2] = b
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
			case "vram-write":
				// Lock for write safety
				gb.Mu.Lock()
				copy(gb.memory.VRAM[cmd.Offset:], cmd.Data)
				gb.Mu.Unlock()
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
				gb.Mu.Lock()
				// Apply state updates (partial updates supported)
				if val, ok := cmd.State["VRAMBank"]; ok {
					gb.memory.VRAMBank = val
				}
				if val, ok := cmd.State["WRAMBank"]; ok {
					gb.memory.WRAMBank = val
				}
				if val, ok := cmd.State["hdmaLength"]; ok {
					gb.memory.hdmaLength = val
				}
				if val, ok := cmd.State["hdmaActive"]; ok {
					gb.memory.hdmaActive = val != 0
				}
				gb.Mu.Unlock()
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
			case "cartridge-ram-write":
				gb.Mu.Lock()
				cart := gb.GetCartridge()
				if cart != nil {
					ram := cart.GetRAM()
					if ram != nil {
						copy(ram[cmd.Offset:], cmd.Data)
					}
				}
				gb.Mu.Unlock()
			case "ppu-state-write":
				gb.Mu.Lock()
				// Get current state
				scanlineCounter, screenCleared, cgbMode := gb.GetPPUState()

				// Apply updates from cmd.State
				if lowByte, ok := cmd.State["scanlineCounter"]; ok {
					highByte := cmd.State["scanlineCounter_high"] // May be 0 if not provided
					scanlineCounter = int(lowByte) | (int(highByte) << 8)
				}
				if val, ok := cmd.State["screenCleared"]; ok {
					screenCleared = val != 0
				}
				if val, ok := cmd.State["cgbMode"]; ok {
					cgbMode = val != 0
				}

				gb.SetPPUState(scanlineCounter, screenCleared, cgbMode)
				gb.Mu.Unlock()
			case "ppu-tilescanline-write":
				gb.Mu.Lock()
				copy(gb.tileScanline[cmd.Offset:], cmd.Data)
				gb.Mu.Unlock()
			case "ppu-bgpalette-write":
				gb.Mu.Lock()
				// Serialize current state to 66-byte array
				stateData := make([]byte, 66)
				copy(stateData[0:64], gb.bgPalette.Palette)
				stateData[64] = gb.bgPalette.Index
				if gb.bgPalette.Inc {
					stateData[65] = 0x01
				} else {
					stateData[65] = 0x00
				}

				// Apply write at offset
				copy(stateData[cmd.Offset:], cmd.Data)

				// Deserialize and update bgPalette
				copy(gb.bgPalette.Palette, stateData[0:64])
				gb.bgPalette.Index = stateData[64]
				gb.bgPalette.Inc = stateData[65] != 0
				gb.Mu.Unlock()
			case "ppu-spritepalette-write":
				gb.Mu.Lock()
				// Serialize current state to 66-byte array
				stateData := make([]byte, 66)
				copy(stateData[0:64], gb.spritePalette.Palette)
				stateData[64] = gb.spritePalette.Index
				if gb.spritePalette.Inc {
					stateData[65] = 0x01
				} else {
					stateData[65] = 0x00
				}

				// Apply write at offset
				copy(stateData[cmd.Offset:], cmd.Data)

				// Deserialize and update spritePalette
				copy(gb.spritePalette.Palette, stateData[0:64])
				gb.spritePalette.Index = stateData[64]
				gb.spritePalette.Inc = stateData[65] != 0
				gb.Mu.Unlock()
			case "ppu-bgpriority-write":
				gb.Mu.Lock()
				// Get current packed state
				packedData := gb.GetBGPriority()

				// Apply write at offset
				copy(packedData[cmd.Offset:], cmd.Data)

				// Unpack into bgPriority array
				for x := 0; x < ScreenWidth; x++ {
					for y := 0; y < ScreenHeight; y++ {
						byteIndex := x*18 + y/8
						bitIndex := uint(y % 8)
						gb.bgPriority[x][y] = (packedData[byteIndex] & (1 << bitIndex)) != 0
					}
				}
				gb.Mu.Unlock()
			case "ppu-screen-write":
				gb.Mu.Lock()
				// Get current flattened screen state
				screenData := gb.GetScreen()

				// Apply write at offset
				copy(screenData[cmd.Offset:], cmd.Data)

				// Unflatten back into PreparedData array
				for x := 0; x < ScreenWidth; x++ {
					for y := 0; y < ScreenHeight; y++ {
						offset := x*ScreenHeight*3 + y*3
						gb.PreparedData[x][y][0] = screenData[offset]     // R
						gb.PreparedData[x][y][1] = screenData[offset+1]   // G
						gb.PreparedData[x][y][2] = screenData[offset+2]   // B
					}
				}
				gb.Mu.Unlock()
			case "apu-state-write":
				gb.Mu.Lock()
				// Get current state
				playing, memory, lVol, rVol, tickCounter := gb.sound.GetAPUState()

				// Serialize to buffer
				stateData := make([]byte, 77) // 1 + 52 + 8 + 8 + 8
				stateData[0] = playing
				copy(stateData[1:53], memory[:])
				binary.LittleEndian.PutUint64(stateData[53:61], math.Float64bits(lVol))
				binary.LittleEndian.PutUint64(stateData[61:69], math.Float64bits(rVol))
				binary.LittleEndian.PutUint64(stateData[69:77], math.Float64bits(tickCounter))

				// Apply write at offset
				copy(stateData[cmd.Offset:], cmd.Data)

				// Deserialize and update
				playing = stateData[0]
				copy(memory[:], stateData[1:53])
				lVol = math.Float64frombits(binary.LittleEndian.Uint64(stateData[53:61]))
				rVol = math.Float64frombits(binary.LittleEndian.Uint64(stateData[61:69]))
				tickCounter = math.Float64frombits(binary.LittleEndian.Uint64(stateData[69:77]))

				gb.sound.SetAPUState(playing, memory, lVol, rVol, tickCounter)
				gb.Mu.Unlock()
			case "cartridge-state-write":
				gb.Mu.Lock()
				cartridge := gb.GetCartridge()
				if cartridge != nil {
					// Type-switch on MBC to apply appropriate state fields
					switch controller := cartridge.BankingController.(type) {
					case *cart.MBC1:
						// Extract current state
						romBank, ramBank, ramEnabled, romBanking := controller.GetBankingState()
						// Apply updates from cmd.State
						if val, ok := cmd.State["romBank"]; ok {
							romBank = uint32(val)
						}
						if val, ok := cmd.State["ramBank"]; ok {
							ramBank = uint32(val)
						}
						if val, ok := cmd.State["ramEnabled"]; ok {
							ramEnabled = val != 0
						}
						if val, ok := cmd.State["romBanking"]; ok {
							romBanking = val != 0
						}
						controller.SetBankingState(romBank, ramBank, ramEnabled, romBanking)

					case *cart.MBC2:
						romBank, ramBank, ramEnabled := controller.GetBankingState()
						if val, ok := cmd.State["romBank"]; ok {
							romBank = uint32(val)
						}
						if val, ok := cmd.State["ramEnabled"]; ok {
							ramEnabled = val != 0
						}
						controller.SetBankingState(romBank, ramBank, ramEnabled)

					case *cart.MBC3:
						// Banking state
						romBank, ramBank, ramEnabled := controller.GetBankingState()
						if val, ok := cmd.State["romBank"]; ok {
							romBank = uint32(val)
						}
						if val, ok := cmd.State["ramBank"]; ok {
							ramBank = uint32(val)
						}
						if val, ok := cmd.State["ramEnabled"]; ok {
							ramEnabled = val != 0
						}
						controller.SetBankingState(romBank, ramBank, ramEnabled)

						// RTC state
						rtc, latchedRtc, latched := controller.GetRTCState()
						if val, ok := cmd.State["rtcSeconds"]; ok {
							rtc[0x08] = val
						}
						if val, ok := cmd.State["rtcMinutes"]; ok {
							rtc[0x09] = val
						}
						if val, ok := cmd.State["rtcHours"]; ok {
							rtc[0x0A] = val
						}
						if val, ok := cmd.State["rtcDaysLow"]; ok {
							rtc[0x0B] = val
						}
						if val, ok := cmd.State["rtcDaysHigh"]; ok {
							rtc[0x0C] = val
						}
						if val, ok := cmd.State["latchedSeconds"]; ok {
							latchedRtc[0x08] = val
						}
						if val, ok := cmd.State["latchedMinutes"]; ok {
							latchedRtc[0x09] = val
						}
						if val, ok := cmd.State["latchedHours"]; ok {
							latchedRtc[0x0A] = val
						}
						if val, ok := cmd.State["latchedDaysLow"]; ok {
							latchedRtc[0x0B] = val
						}
						if val, ok := cmd.State["latchedDaysHigh"]; ok {
							latchedRtc[0x0C] = val
						}
						if val, ok := cmd.State["rtcLatched"]; ok {
							latched = val != 0
						}
						controller.SetRTCState(rtc[0x08], rtc[0x09], rtc[0x0A], rtc[0x0B], rtc[0x0C],
							latchedRtc[0x08], latchedRtc[0x09], latchedRtc[0x0A], latchedRtc[0x0B], latchedRtc[0x0C],
							latched)

					case *cart.MBC5:
						romBank, ramBank, ramEnabled := controller.GetBankingState()
						if val, ok := cmd.State["romBank"]; ok {
							romBank = uint32(val)
						}
						if val, ok := cmd.State["ramBank"]; ok {
							ramBank = uint32(val)
						}
						if val, ok := cmd.State["ramEnabled"]; ok {
							ramEnabled = val != 0
						}
						controller.SetBankingState(romBank, ramBank, ramEnabled)

					case *cart.ROM:
						// ROM-only has no banking state, ignore writes
					}
				}
				gb.Mu.Unlock()
			default:
				// Unknown command, ignore
			}
		default:
			// No more commands
			return
		}
	}
}

// ToggleSoundChannel toggles a sound channel for debugging.
func (gb *Gameboy) ToggleSoundChannel(channel int) {
	gb.sound.ToggleSoundChannel(channel)
}

func (gb *Gameboy) SoundString() {
	gb.sound.LogSoundState()
}

// BGMapString returns a string of the values in the background map.
func (gb *Gameboy) BGMapString() string {
	out := ""
	for y := uint16(0); y < 0x20; y++ {
		out += fmt.Sprintf("%2x: ", y)
		for x := uint16(0); x < 0x20; x++ {
			out += fmt.Sprintf("%2x ", gb.memory.Read(0x9800+(y*0x20)+x))
		}
		out += "\n"
	}
	return out
}

func (gb *Gameboy) printBGMap() {
	fmt.Printf("BG Map:\n%s", gb.BGMapString())
}

// Get the current CPU speed multiplier (either 1 or 2).
func (gb *Gameboy) getSpeed() int {
	return int(gb.currentSpeed + 1)
}

// Check if the speed needs to be switched for CGB mode.
func (gb *Gameboy) checkSpeedSwitch() {
	if gb.prepareSpeed {
		// Switch speed
		gb.prepareSpeed = false
		if gb.currentSpeed == 0 {
			gb.currentSpeed = 1
		} else {
			gb.currentSpeed = 0
		}
		gb.halted = false
	}
}

func (gb *Gameboy) updateTimers(cycles int) {
	gb.dividerRegister(cycles)
	if gb.isClockEnabled() {
		gb.timerCounter += cycles

		freq := gb.getClockFreqCount()
		for gb.timerCounter >= freq {
			gb.timerCounter -= freq
			tima := gb.memory.HighRAM[0x05] /* TIMA */
			if tima == 0xFF {
				gb.memory.HighRAM[TIMA-0xFF00] = gb.memory.HighRAM[0x06] /* TMA */
				gb.requestInterrupt(2)
			} else {
				gb.memory.HighRAM[TIMA-0xFF00] = tima + 1
			}
		}
	}
}

func (gb *Gameboy) isClockEnabled() bool {
	return bitTest(gb.memory.HighRAM[0x07] /* TAC */, 2)
}

func (gb *Gameboy) getClockFreq() byte {
	return gb.memory.HighRAM[0x07] /* TAC */ & 0x3
}

func (gb *Gameboy) getClockFreqCount() int {
	switch gb.getClockFreq() {
	case 0:
		return 1024
	case 1:
		return 16
	case 2:
		return 64
	default:
		return 256
	}
}

func (gb *Gameboy) setClockFreq() {
	gb.timerCounter = 0
}

func (gb *Gameboy) dividerRegister(cycles int) {
	gb.cpu.Divider += cycles
	if gb.cpu.Divider >= 255 {
		gb.cpu.Divider -= 255
		gb.memory.HighRAM[DIV-0xFF00]++
	}
}

// Request the Gameboy to perform an interrupt.
func (gb *Gameboy) requestInterrupt(interrupt byte) {
	req := gb.memory.HighRAM[0x0F] | 0xE0
	req = bitSet(req, interrupt)
	gb.memory.Write(0xFF0F, req)
}

func (gb *Gameboy) doInterrupts() (cycles int) {
	if gb.interruptsEnabling {
		gb.interruptsOn = true
		gb.interruptsEnabling = false
		return 0
	}
	if !gb.interruptsOn && !gb.halted {
		return 0
	}

	req := gb.memory.HighRAM[0x0F] | 0xE0
	enabled := gb.memory.HighRAM[0xFF]

	if req > 0 {
		var i byte
		for i = 0; i < 5; i++ {
			if bitTest(req, i) && bitTest(enabled, i) {
				gb.serviceInterrupt(i)
				return 20
			}
		}
	}
	return 0
}

// Address that should be jumped to by interrupt.
var interruptAddresses = map[byte]uint16{
	0: 0x40, // V-Blank
	1: 0x48, // LCDC Status
	2: 0x50, // Timer Overflow
	3: 0x58, // Serial Transfer
	4: 0x60, // Hi-Lo P10-P13
}

// Called if an interrupt has been raised. Will check if interrupts are
// enabled and will jump to the interrupt address.
func (gb *Gameboy) serviceInterrupt(interrupt byte) {
	// If was halted without interrupts, do not jump or reset IF
	if !gb.interruptsOn && gb.halted {
		gb.halted = false
		return
	}
	gb.interruptsOn = false
	gb.halted = false

	req := gb.memory.ReadHighRam(0xFF0F)
	req = bitReset(req, interrupt)
	gb.memory.Write(0xFF0F, req)

	gb.pushStack(gb.cpu.PC)
	gb.cpu.PC = interruptAddresses[interrupt]
}

// Push a 16 bit value onto the stack and decrement SP.
func (gb *Gameboy) pushStack(address uint16) {
	sp := gb.cpu.SP.HiLo()
	gb.memory.Write(sp-1, byte(uint16(address&0xFF00)>>8))
	gb.memory.Write(sp-2, byte(address&0xFF))
	gb.cpu.SP.Set(gb.cpu.SP.HiLo() - 2)
}

// Pop the next 16 bit value off the stack and increment SP.
func (gb *Gameboy) popStack() uint16 {
	sp := gb.cpu.SP.HiLo()
	byte1 := uint16(gb.memory.Read(sp))
	byte2 := uint16(gb.memory.Read(sp+1)) << 8
	gb.cpu.SP.Set(gb.cpu.SP.HiLo() + 2)
	return byte1 | byte2
}

func (gb *Gameboy) joypadValue(current byte) byte {
	var in byte = 0xF
	if bitTest(current, 4) {
		in = gb.inputMask & 0xF
	} else if bitTest(current, 5) {
		in = (gb.inputMask >> 4) & 0xF
	}
	return current | 0xc0 | in
}

// GetLoadedCart returns the currently loaded cartridge, or nil if no cartridge is loaded.
func (gb *Gameboy) GetLoadedCart() *cart.Cart {
	if gb.memory == nil || gb.memory.Cart == nil {
		return nil
	}
	return gb.memory.Cart
}

// IsCartLoaded returns if there is a game loaded in the gameboy.
func (gb *Gameboy) IsCartLoaded() bool {
	return gb.memory != nil && gb.memory.Cart != nil
}

// IsCGB returns if we are using CGB features.
func (gb *Gameboy) IsCGB() bool {
	return gb.cgbMode
}

// Initialise the Gameboy using a path to a rom.
func (gb *Gameboy) init(romFile string) error {
	gb.setup()

	// Load the ROM file
	hasCGB, err := gb.memory.LoadCart(romFile)
	if err != nil {
		return fmt.Errorf("failed to open rom file: %s", err)
	}
	fmt.Printf("Loaded ROM: %s\n", gb.memory.Cart.GetName())
	gb.cgbMode = gb.options.cgbMode && hasCGB
	return nil
}

func (gb *Gameboy) initKeyHandlers() {
	gb.keyHandlers = map[Button]func(){
		ButtonPause:               gb.togglePaused,
		ButtonChangePallete:       changePalette,
		ButtonToggleBackground:    gb.Debug.toggleBackGround,
		ButtonToggleSprites:       gb.Debug.toggleSprites,
		ButtonToggleOutputOpCode:  gb.Debug.toggleOutputOpCode,
		ButtonPrintBGMap:          gb.printBGMap,
		ButtonToggleSoundChannel1: func() { gb.ToggleSoundChannel(1) },
		ButtonToggleSoundChannel2: func() { gb.ToggleSoundChannel(2) },
		ButtonToggleSoundChannel3: func() { gb.ToggleSoundChannel(3) },
		ButtonToggleSoundChannel4: func() { gb.ToggleSoundChannel(4) },
	}
}

// Setup and instantiate the GameBoys components.
func (gb *Gameboy) setup() {
	// Initialise the CPU
	gb.cpu = &CPU{}
	gb.cpu.Init(gb.options.cgbMode)

	// Initialise the memory
	gb.memory = &Memory{}
	gb.memory.Init(gb)

	gb.sound = &apu.APU{}
	gb.sound.Init(gb.options.sound)

	gb.Debug = DebugFlags{}
	gb.scanlineCounter = 456
	gb.inputMask = 0xFF

	gb.cbInst = gb.cbInstructions()

	gb.spritePalette = NewPalette()
	gb.bgPalette = NewPalette()

	gb.initKeyHandlers()

	// Buffer size 32 supports simultaneous writes from tar extraction of current
	// state files (5 in state/memory/) plus future state files (apu channels,
	// ppu state, cartridge state, etc.). Prevents blocking during full state restore.
	gb.CommandChan = make(chan Command, 32)
}

// New returns a new Gameboy instance.
func New(romFile string, opts ...GameboyOption) (*Gameboy, error) {
	// Build the gameboy
	gameboy := Gameboy{}
	for _, opt := range opts {
		opt(&gameboy.options)
	}
	err := gameboy.init(romFile)
	if err != nil {
		return nil, err
	}
	return &gameboy, nil
}
