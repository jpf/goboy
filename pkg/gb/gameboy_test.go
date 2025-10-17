package gb

import (
	"sync"
	"testing"
)

// setupTestGameboy creates a Gameboy instance for testing.
func setupTestGameboy(t *testing.T) *Gameboy {
	gb := &Gameboy{}
	gb.memory = &Memory{}
	gb.CommandChan = make(chan Command, 32)
	gb.Mu = sync.RWMutex{}
	return gb
}

func TestCommandChan_BufferSize(t *testing.T) {
	// Test the production setup() method creates correct buffer size
	gb := &Gameboy{}
	gb.setup()

	// Verify buffer size is 32 by sending 32 commands without blocking
	for i := 0; i < 32; i++ {
		select {
		case gb.CommandChan <- Command{Name: "test"}:
			// Success
		default:
			t.Fatalf("CommandChan blocked at %d commands, expected buffer of 32", i)
		}
	}

	// 33rd command should block (channel full)
	select {
	case gb.CommandChan <- Command{Name: "test"}:
		t.Fatal("Expected channel to block at 33 commands")
	default:
		// Expected - channel is full
	}
}

func TestGetWRAM(t *testing.T) {
	gb := setupTestGameboy(t)
	wram := gb.GetWRAM()

	if wram == nil {
		t.Fatal("GetWRAM returned nil")
	}
	if len(wram) != 0x9000 {
		t.Fatalf("GetWRAM size = %d, want 0x9000", len(wram))
	}

	// Verify it's the actual memory (not a copy)
	gb.Mu.Lock()
	gb.memory.WRAM[0x100] = 0xAB
	gb.Mu.Unlock()

	if wram[0x100] != 0xAB {
		t.Fatal("GetWRAM returned copy instead of pointer")
	}
}

func TestGetOAM(t *testing.T) {
	gb := setupTestGameboy(t)
	oam := gb.GetOAM()

	if oam == nil {
		t.Fatal("GetOAM returned nil")
	}
	if len(oam) != 0x100 {
		t.Fatalf("GetOAM size = %d, want 0x100", len(oam))
	}
}

func TestGetHighRAM(t *testing.T) {
	gb := setupTestGameboy(t)
	highram := gb.GetHighRAM()

	if highram == nil {
		t.Fatal("GetHighRAM returned nil")
	}
	if len(highram) != 0x100 {
		t.Fatalf("GetHighRAM size = %d, want 0x100", len(highram))
	}
}

func TestGetMemoryState(t *testing.T) {
	gb := setupTestGameboy(t)

	// Set known values
	gb.Mu.Lock()
	gb.memory.VRAMBank = 1
	gb.memory.WRAMBank = 3
	gb.memory.hdmaLength = 0x42
	gb.memory.hdmaActive = true
	gb.Mu.Unlock()

	// Read under lock (caller responsibility)
	gb.Mu.RLock()
	vram, wram, hdma, active := gb.GetMemoryState()
	gb.Mu.RUnlock()

	if vram != 1 || wram != 3 || hdma != 0x42 || active != true {
		t.Fatalf("GetMemoryState = (%d, %d, 0x%02x, %v), want (1, 3, 0x42, true)",
			vram, wram, hdma, active)
	}
}

func TestProcessCommands_WramWrite(t *testing.T) {
	gb := setupTestGameboy(t)

	// Set initial values
	gb.memory.WRAM[0x100] = 0x00
	gb.memory.WRAM[0x101] = 0x00

	// Queue write command
	gb.CommandChan <- Command{
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

func TestProcessCommands_OamWrite(t *testing.T) {
	gb := setupTestGameboy(t)

	// Set initial values
	gb.memory.OAM[0x10] = 0x00
	gb.memory.OAM[0x11] = 0x00

	// Queue write command
	gb.CommandChan <- Command{
		Name:   "oam-write",
		Offset: 0x10,
		Data:   []byte{0x12, 0x34},
	}

	gb.ProcessCommands()

	if gb.memory.OAM[0x10] != 0x12 || gb.memory.OAM[0x11] != 0x34 {
		t.Fatalf("OAM[0x10:0x12] = %02x %02x, want 12 34",
			gb.memory.OAM[0x10], gb.memory.OAM[0x11])
	}
}

func TestProcessCommands_HighramWrite(t *testing.T) {
	gb := setupTestGameboy(t)

	// Set initial values
	gb.memory.HighRAM[0xFF] = 0x00

	// Queue write command
	gb.CommandChan <- Command{
		Name:   "highram-write",
		Offset: 0xFF,
		Data:   []byte{0xEE},
	}

	gb.ProcessCommands()

	if gb.memory.HighRAM[0xFF] != 0xEE {
		t.Fatalf("HighRAM[0xFF] = %02x, want EE", gb.memory.HighRAM[0xFF])
	}
}

func TestProcessCommands_MemoryStateWrite(t *testing.T) {
	gb := setupTestGameboy(t)

	// Initial state
	gb.memory.VRAMBank = 0
	gb.memory.WRAMBank = 1
	gb.memory.hdmaLength = 0
	gb.memory.hdmaActive = false

	// Queue memorystate write
	gb.CommandChan <- Command{
		Name: "memorystate-write",
		State: map[string]byte{
			"VRAMBank":   1,
			"WRAMBank":   3,
			"hdmaLength": 0x42,
			"hdmaActive": 1,
		},
	}

	gb.ProcessCommands()

	// Verify state updated
	if gb.memory.VRAMBank != 1 || gb.memory.WRAMBank != 3 ||
		gb.memory.hdmaLength != 0x42 || gb.memory.hdmaActive != true {
		t.Fatalf("Memory state = (%d, %d, 0x%02x, %v), want (1, 3, 0x42, true)",
			gb.memory.VRAMBank, gb.memory.WRAMBank,
			gb.memory.hdmaLength, gb.memory.hdmaActive)
	}
}

func TestProcessCommands_MemoryStateWrite_PartialUpdate(t *testing.T) {
	gb := setupTestGameboy(t)

	// Initial state
	gb.memory.VRAMBank = 0
	gb.memory.WRAMBank = 1

	// Queue partial update (only change WRAMBank)
	gb.CommandChan <- Command{
		Name: "memorystate-write",
		State: map[string]byte{
			"WRAMBank": 5,
		},
	}

	gb.ProcessCommands()

	// Verify only WRAMBank changed
	if gb.memory.VRAMBank != 0 || gb.memory.WRAMBank != 5 {
		t.Fatalf("After partial update: VRAMBank=%d, WRAMBank=%d, want 0 and 5",
			gb.memory.VRAMBank, gb.memory.WRAMBank)
	}
}
