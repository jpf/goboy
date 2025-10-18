// ABOUTME: Tests for cartridge banking state filesystem (text format).
// ABOUTME: Validates MBC-specific state reading, parsing, and validation.

package ninep

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/cart"
	"github.com/Humpheh/goboy/pkg/gb"
)

func setupTestCartridgeStateFS_MBC1() (*cartridgeStateFS, *gb.Gameboy) {
	// Create MBC1 cartridge with minimal ROM
	romData := make([]byte, 0x8000) // 32KB
	romData[0x0147] = 0x01          // MBC1 type

	mbc1 := cart.NewMBC1(romData)
	testCart := &cart.Cart{
		BankingController: mbc1,
	}

	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	memory := &gb.Memory{
		Cart: testCart,
	}
	gameboy.SetMemory(memory)

	fsys := newCartridgeStateFS(gameboy)
	return fsys, gameboy
}

func TestCartridgeStateFS_MBC1_Open(t *testing.T) {
	fsys, _ := setupTestCartridgeStateFS_MBC1()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
}

func TestCartridgeStateFile_MBC1_Read(t *testing.T) {
	fsys, _ := setupTestCartridgeStateFS_MBC1()

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

	// Verify MBC1-specific fields
	if !strings.Contains(content, "# Banking") {
		t.Error("Missing '# Banking' section header")
	}
	if !strings.Contains(content, "romBank=") {
		t.Error("Missing romBank field")
	}
	if !strings.Contains(content, "ramBank=") {
		t.Error("Missing ramBank field")
	}
	if !strings.Contains(content, "ramEnabled=") {
		t.Error("Missing ramEnabled field")
	}
	if !strings.Contains(content, "romBanking=") {
		t.Error("Missing romBanking field (MBC1 specific)")
	}
}

func TestCartridgeStateFile_MBC1_Write_Valid(t *testing.T) {
	fsys, gameboy := setupTestCartridgeStateFS_MBC1()

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
	data := []byte("romBank=0x05\n")
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
		if cmd.Name != "cartridge-state-write" {
			t.Fatalf("Command name = %q, want \"cartridge-state-write\"", cmd.Name)
		}
		if cmd.State == nil {
			t.Fatal("Command.State is nil")
		}
		if val, ok := cmd.State["romBank"]; !ok || val != 5 {
			t.Fatalf("State[romBank] = %d, want 5", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCartridgeStateFile_NoCartridge_Read(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(&gb.Memory{}) // No cartridge

	fsys := newCartridgeStateFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// Should return empty or error message for no cartridge
	if !strings.Contains(content, "no cartridge") && n > 0 {
		t.Error("Expected 'no cartridge' message or empty content")
	}
}

func setupTestCartridgeStateFS_MBC3() (*cartridgeStateFS, *gb.Gameboy) {
	romData := make([]byte, 0x8000)
	romData[0x0147] = 0x10 // MBC3

	mbc3 := cart.NewMBC3(romData)
	testCart := &cart.Cart{
		BankingController: mbc3,
	}

	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	memory := &gb.Memory{
		Cart: testCart,
	}
	gameboy.SetMemory(memory)

	fsys := newCartridgeStateFS(gameboy)
	return fsys, gameboy
}

func TestCartridgeStateFile_MBC3_Read(t *testing.T) {
	fsys, _ := setupTestCartridgeStateFS_MBC3()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// Verify MBC3-specific fields (no romBanking like MBC1)
	if !strings.Contains(content, "romBank=") {
		t.Error("Missing romBank field")
	}
	if !strings.Contains(content, "ramBank=") {
		t.Error("Missing ramBank field")
	}
	if !strings.Contains(content, "ramEnabled=") {
		t.Error("Missing ramEnabled field")
	}
	// MBC3 should have RTC fields
	if !strings.Contains(content, "rtcSeconds=") {
		t.Error("Missing rtcSeconds field")
	}
	if !strings.Contains(content, "rtcLatched=") {
		t.Error("Missing rtcLatched field")
	}
	// MBC3 should NOT have romBanking (that's MBC1-specific)
	if strings.Contains(content, "romBanking=") {
		t.Error("MBC3 should not have romBanking field")
	}
}

func TestCartridgeStateFile_MBC3_Write_Valid(t *testing.T) {
	fsys, gameboy := setupTestCartridgeStateFS_MBC3()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write valid state update including RTC
	data := []byte("romBank=0x05\nrtcSeconds=0x30\n")
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
		if cmd.Name != "cartridge-state-write" {
			t.Fatalf("Command name = %q, want \"cartridge-state-write\"", cmd.Name)
		}
		if cmd.State == nil {
			t.Fatal("Command.State is nil")
		}
		if len(cmd.State) != 2 {
			t.Fatalf("Command.State has %d entries, want 2", len(cmd.State))
		}
		if val, ok := cmd.State["romBank"]; !ok || val != 5 {
			t.Fatalf("State[romBank] = %d, want 5", val)
		}
		if val, ok := cmd.State["rtcSeconds"]; !ok || val != 0x30 {
			t.Fatalf("State[rtcSeconds] = 0x%02x, want 0x30", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCartridgeStateFile_ROM_Read(t *testing.T) {
	// Create ROM-only cartridge
	romData := make([]byte, 0x8000)
	romData[0x0147] = 0x00 // ROM only

	romCtrl := cart.NewROM(romData)
	testCart := &cart.Cart{
		BankingController: romCtrl,
	}

	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	memory := &gb.Memory{
		Cart: testCart,
	}
	gameboy.SetMemory(memory)

	fsys := newCartridgeStateFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// ROM-only should indicate no banking
	if !strings.Contains(content, "ROM-only") {
		t.Error("Expected 'ROM-only' message for ROM cartridge")
	}
}
