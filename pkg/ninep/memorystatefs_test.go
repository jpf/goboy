// ABOUTME: Tests for memory banking state filesystem (text format).
// ABOUTME: Validates key=value parsing, validation, and command generation.

package ninep

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
)

func setupTestMemoryStateFS() (*memoryStateFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(&gb.Memory{
		VRAMBank: 0,
		WRAMBank: 1,
	})

	fsys := newMemoryStateFS(gameboy)
	return fsys, gameboy
}

func TestMemoryStateFS_Open(t *testing.T) {
	fsys, _ := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
}

func TestMemoryStateFile_Read(t *testing.T) {
	fsys, gameboy := setupTestMemoryStateFS()

	// Set known state
	gameboy.Mu.Lock()
	vram, wram, hdma, active := gameboy.GetMemoryState()
	_ = vram
	_ = wram
	_ = hdma
	_ = active
	// Directly set via memory since we can't use GetMemoryState without lock
	mem := &gb.Memory{
		VRAMBank:   1,
		WRAMBank:   3,
		WRAM:       [0x9000]byte{},
		VRAM:       [0x4000]byte{},
		HighRAM:    [0x100]byte{},
		OAM:        [0x100]byte{},
	}
	// Use reflection or direct field access
	gameboy.SetMemory(mem)
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read entire file
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// Verify format matches spec
	if !strings.Contains(content, "# Banking") {
		t.Error("Missing '# Banking' section header")
	}
	if !strings.Contains(content, "VRAMBank=0x01") {
		t.Error("Missing or incorrect VRAMBank")
	}
	if !strings.Contains(content, "WRAMBank=0x03") {
		t.Error("Missing or incorrect WRAMBank")
	}
	if !strings.Contains(content, "# DMA") {
		t.Error("Missing '# DMA' section header")
	}
	if !strings.Contains(content, "hdmaLength=0x00") {
		t.Error("Missing or incorrect hdmaLength")
	}
	if !strings.Contains(content, "hdmaActive=0x00") {
		t.Error("Missing or incorrect hdmaActive")
	}
}

func TestMemoryStateFile_Write_Valid(t *testing.T) {
	fsys, gameboy := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write valid state update
	data := []byte("WRAMBank=0x05\n")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d, want %d", n, len(data))
	}

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		if cmd.Name != "memorystate-write" {
			t.Fatalf("Command name = %q, want \"memorystate-write\"", cmd.Name)
		}
		if cmd.State == nil {
			t.Fatal("Command.State is nil")
		}
		if val, ok := cmd.State["WRAMBank"]; !ok || val != 5 {
			t.Fatalf("State[WRAMBank] = %d, want 5", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestMemoryStateFile_Write_PartialUpdate(t *testing.T) {
	fsys, gameboy := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write multiple fields
	data := []byte("VRAMBank=0x01\nhdmaActive=0x01\n")
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify command has both fields
	select {
	case cmd := <-gameboy.CommandChan:
		if len(cmd.State) != 2 {
			t.Fatalf("Command.State has %d entries, want 2", len(cmd.State))
		}
		if val, ok := cmd.State["VRAMBank"]; !ok || val != 1 {
			t.Fatal("VRAMBank not set correctly")
		}
		if val, ok := cmd.State["hdmaActive"]; !ok || val != 1 {
			t.Fatal("hdmaActive not set correctly")
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestMemoryStateFile_Write_InvalidVRAMBank(t *testing.T) {
	fsys, _ := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write invalid VRAMBank (> 1)
	data := []byte("VRAMBank=0x02\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for VRAMBank > 1, got nil")
	}
	if !strings.Contains(err.Error(), "VRAMBank") {
		t.Fatalf("Error should mention VRAMBank, got: %v", err)
	}
}

func TestMemoryStateFile_Write_InvalidWRAMBank(t *testing.T) {
	fsys, _ := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write invalid WRAMBank (> 7)
	data := []byte("WRAMBank=0x08\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for WRAMBank > 7, got nil")
	}
	if !strings.Contains(err.Error(), "WRAMBank") {
		t.Fatalf("Error should mention WRAMBank, got: %v", err)
	}
}

func TestMemoryStateFile_Write_InvalidHdmaActive(t *testing.T) {
	fsys, _ := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write invalid hdmaActive (> 1)
	data := []byte("hdmaActive=0x02\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for hdmaActive > 1, got nil")
	}
	if !strings.Contains(err.Error(), "hdmaActive") {
		t.Fatalf("Error should mention hdmaActive, got: %v", err)
	}
}

func TestMemoryStateFile_Write_UnknownKey(t *testing.T) {
	fsys, _ := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write unknown key
	data := []byte("UnknownField=0x42\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for unknown key, got nil")
	}
}

func TestMemoryStateFile_Write_WithComments(t *testing.T) {
	fsys, gameboy := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write with comments and blank lines (should be ignored)
	data := []byte(`# Banking
VRAMBank=0x01

# Comment line
WRAMBank=0x05
`)
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify command has both fields (comments ignored)
	select {
	case cmd := <-gameboy.CommandChan:
		if len(cmd.State) != 2 {
			t.Fatalf("Command.State has %d entries, want 2", len(cmd.State))
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestMemoryStateFile_Write_DecimalValues(t *testing.T) {
	fsys, gameboy := setupTestMemoryStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write decimal values (should be accepted)
	data := []byte("WRAMBank=5\n")
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case cmd := <-gameboy.CommandChan:
		if val, ok := cmd.State["WRAMBank"]; !ok || val != 5 {
			t.Fatalf("State[WRAMBank] = %d, want 5", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}
