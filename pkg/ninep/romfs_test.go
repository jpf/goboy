// ABOUTME: Tests for ROM filesystem and validation.
// ABOUTME: Validates ROM size, header checksum, and load functionality.

package ninep

import (
	"bytes"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
)

// createMinimalROM creates a minimal valid 32KB ROM with correct header checksum
func createMinimalROM() []byte {
	rom := make([]byte, 32*1024) // 32KB minimum

	// Set title
	copy(rom[0x0134:0x0143], "TEST ROM")

	// Calculate header checksum (0x0134-0x014C)
	checksum := byte(0)
	for addr := 0x0134; addr <= 0x014C; addr++ {
		checksum = checksum - rom[addr] - 1
	}
	rom[0x014D] = checksum

	return rom
}

func TestValidateROM_ValidROM(t *testing.T) {
	rom := createMinimalROM()

	err := validateROM(rom)
	if err != nil {
		t.Errorf("validateROM() failed for valid ROM: %v", err)
	}
}

func TestValidateROM_TooSmall(t *testing.T) {
	rom := make([]byte, 16*1024) // 16KB - too small

	err := validateROM(rom)
	if err == nil {
		t.Error("validateROM() should reject ROM < 32KB")
	}
	if err != nil && err.Error() != "ROM too small: 16384 bytes (minimum 32KB)" {
		t.Errorf("validateROM() wrong error message: %v", err)
	}
}

func TestValidateROM_TooLarge(t *testing.T) {
	rom := make([]byte, 9*1024*1024) // 9MB - too large

	err := validateROM(rom)
	if err == nil {
		t.Error("validateROM() should reject ROM > 8MB")
	}
	if err != nil && err.Error() != "ROM too large: 9437184 bytes (maximum 8MB)" {
		t.Errorf("validateROM() wrong error message: %v", err)
	}
}

func TestValidateROM_BadChecksum(t *testing.T) {
	rom := createMinimalROM()
	rom[0x014D] = 0xFF // Corrupt checksum

	err := validateROM(rom)
	if err == nil {
		t.Error("validateROM() should reject ROM with bad checksum")
	}
}

func TestROMFS_WriteAndClose(t *testing.T) {
	// Setup gameboy with command channel
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fs := newROMFS(gameboy)
	fsFile, err := fs.Open("")
	if err != nil {
		t.Fatalf("Failed to open ROM file: %v", err)
	}
	file := fsFile.(*romFile)

	// Write valid ROM data
	rom := createMinimalROM()
	n, err := file.Write(rom)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(rom) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(rom))
	}

	// Close should trigger ROM load command
	err = file.Close()
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Verify command was sent
	select {
	case cmd := <-gameboy.CommandChan:
		if cmd.Name != "rom-load" {
			t.Errorf("Command name = %s, want rom-load", cmd.Name)
		}
		if !bytes.Equal(cmd.Data, rom) {
			t.Error("Command data does not match written ROM")
		}
	default:
		t.Error("No rom-load command sent to CommandChan")
	}
}

func TestROMFS_WriteInvalidROM(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fs := newROMFS(gameboy)
	fsFile, err := fs.Open("")
	if err != nil {
		t.Fatalf("Failed to open ROM file: %v", err)
	}
	file := fsFile.(*romFile)

	// Write invalid ROM (too small)
	rom := make([]byte, 1024)
	file.Write(rom)

	// Close should return error
	err = file.Close()
	if err == nil {
		t.Error("Close() should return error for invalid ROM")
	}

	// Verify no command was sent
	select {
	case <-gameboy.CommandChan:
		t.Error("rom-load command should not be sent for invalid ROM")
	default:
		// Expected - no command sent
	}
}

func TestROMFS_EmptyWrite(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fs := newROMFS(gameboy)
	file, err := fs.Open("")
	if err != nil {
		t.Fatalf("Failed to open ROM file: %v", err)
	}

	// Close without writing anything
	err = file.Close()
	if err != nil {
		t.Errorf("Close() with no data should succeed: %v", err)
	}

	// Verify no command was sent
	select {
	case <-gameboy.CommandChan:
		t.Error("rom-load command should not be sent for empty file")
	default:
		// Expected - no command sent
	}
}
