// ABOUTME: Tests for cartridge info filesystem (read-only ROM header metadata).
// ABOUTME: Validates ROM header parsing, text output format, and read-only enforcement.

package ninep

import (
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/cart"
	"github.com/Humpheh/goboy/pkg/gb"
)

// createTestROM creates a minimal valid Game Boy ROM with specified header values
func createTestROM(title string, cartType byte, romSize byte, ramSize byte, cgbFlag byte, sgbFlag byte) []byte {
	// Create minimal 32KB ROM (smallest valid size)
	rom := make([]byte, 32*1024)

	// Copy title to ROM header (0x0134-0x0143)
	titleBytes := []byte(title)
	for i := 0; i < len(titleBytes) && i < 15; i++ {
		rom[0x0134+i] = titleBytes[i]
	}

	// Set header fields
	rom[0x0143] = cgbFlag     // CGB flag
	rom[0x0146] = sgbFlag     // SGB flag
	rom[0x0147] = cartType    // Cartridge type
	rom[0x0148] = romSize     // ROM size
	rom[0x0149] = ramSize     // RAM size
	rom[0x014D] = 0x3C        // Header checksum (fake)
	rom[0x014E] = 0xB0        // Global checksum high byte
	rom[0x014F] = 0xA2        // Global checksum low byte

	return rom
}

func setupTestCartridgeInfoFS() (*cartridgeInfoFS, *gb.Gameboy) {
	// Create test ROM with known values
	rom := createTestROM("POKEMON BLUE", 0x13, 0x05, 0x03, 0x00, 0x00)
	cartridge := cart.NewCart(rom, "test.gb")

	memory := &gb.Memory{
		Cart: cartridge,
	}

	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)

	fsys := newCartridgeInfoFS(gameboy)
	return fsys, gameboy
}

func TestCartridgeInfoFS_Open(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
}

func TestCartridgeInfoFS_Open_NonRoot(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	_, err := fsys.Open("nonexistent")
	if err != fs.ErrNotExist {
		t.Fatalf("Expected ErrNotExist for non-root path, got: %v", err)
	}
}

func TestCartridgeInfoFile_Read(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read entire file
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// Verify format matches spec
	if !strings.Contains(content, "# ROM Information") {
		t.Error("Missing '# ROM Information' section header")
	}
	if !strings.Contains(content, "title=POKEMON BLUE") {
		t.Errorf("Missing or incorrect title, got: %s", content)
	}
	if !strings.Contains(content, "type=MBC3") {
		t.Errorf("Missing or incorrect type (expected MBC3), got: %s", content)
	}
	if !strings.Contains(content, "romSize=1048576") { // 32KB * 2^5 = 1MB
		t.Errorf("Missing or incorrect romSize, got: %s", content)
	}
	if !strings.Contains(content, "ramSize=32768") { // 0x03 = 32KB
		t.Errorf("Missing or incorrect ramSize, got: %s", content)
	}

	if !strings.Contains(content, "# Features") {
		t.Error("Missing '# Features' section header")
	}
	if !strings.Contains(content, "cgbSupport=0x00") {
		t.Errorf("Missing or incorrect cgbSupport, got: %s", content)
	}
	if !strings.Contains(content, "sgbSupport=0x00") {
		t.Errorf("Missing or incorrect sgbSupport, got: %s", content)
	}
	if !strings.Contains(content, "hasBattery=0x01") { // MBC3+RAM+BATTERY (0x13) has battery
		t.Errorf("Missing or incorrect hasBattery, got: %s", content)
	}

	if !strings.Contains(content, "# Checksums") {
		t.Error("Missing '# Checksums' section header")
	}
	if !strings.Contains(content, "headerChecksum=0x3C") {
		t.Errorf("Missing or incorrect headerChecksum, got: %s", content)
	}
	if !strings.Contains(content, "globalChecksum=0xB0A2") {
		t.Errorf("Missing or incorrect globalChecksum, got: %s", content)
	}
}

func TestCartridgeInfoFile_Read_MBC1(t *testing.T) {
	// Test MBC1 cartridge
	rom := createTestROM("TETRIS", 0x03, 0x00, 0x02, 0x00, 0x00)
	cartridge := cart.NewCart(rom, "tetris.gb")

	memory := &gb.Memory{Cart: cartridge}
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)

	fsys := newCartridgeInfoFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	if !strings.Contains(content, "title=TETRIS") {
		t.Errorf("Incorrect title, got: %s", content)
	}
	if !strings.Contains(content, "type=MBC1") {
		t.Errorf("Expected MBC1 type, got: %s", content)
	}
	if !strings.Contains(content, "romSize=32768") { // 0x00 = 32KB
		t.Errorf("Incorrect romSize, got: %s", content)
	}
	if !strings.Contains(content, "ramSize=8192") { // 0x02 = 8KB
		t.Errorf("Incorrect ramSize, got: %s", content)
	}
	if !strings.Contains(content, "hasBattery=0x01") { // 0x03 has battery
		t.Errorf("Incorrect hasBattery, got: %s", content)
	}
}

func TestCartridgeInfoFile_Read_MBC5(t *testing.T) {
	// Test MBC5 cartridge without battery
	rom := createTestROM("MARIO", 0x19, 0x04, 0x04, 0x80, 0x00)
	cartridge := cart.NewCart(rom, "mario.gb")

	memory := &gb.Memory{Cart: cartridge}
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)

	fsys := newCartridgeInfoFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	if !strings.Contains(content, "type=MBC5") {
		t.Errorf("Expected MBC5 type, got: %s", content)
	}
	if !strings.Contains(content, "romSize=524288") { // 32KB * 2^4 = 512KB
		t.Errorf("Incorrect romSize, got: %s", content)
	}
	if !strings.Contains(content, "ramSize=131072") { // 0x04 = 128KB
		t.Errorf("Incorrect ramSize, got: %s", content)
	}
	if !strings.Contains(content, "hasBattery=0x00") { // 0x19 has no battery
		t.Errorf("Incorrect hasBattery, got: %s", content)
	}
	if !strings.Contains(content, "cgbSupport=0x80") { // CGB compatible
		t.Errorf("Incorrect cgbSupport, got: %s", content)
	}
}

func TestCartridgeInfoFile_Read_ROMOnly(t *testing.T) {
	// Test ROM-only cartridge
	rom := createTestROM("SIMPLEROM", 0x00, 0x00, 0x00, 0x00, 0x00)
	cartridge := cart.NewCart(rom, "simple.gb")

	memory := &gb.Memory{Cart: cartridge}
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)

	fsys := newCartridgeInfoFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	if !strings.Contains(content, "type=ROM") {
		t.Errorf("Expected ROM type, got: %s", content)
	}
	if !strings.Contains(content, "ramSize=0") {
		t.Errorf("Expected 0 RAM, got: %s", content)
	}
	if !strings.Contains(content, "hasBattery=0x00") {
		t.Errorf("ROM-only should have no battery, got: %s", content)
	}
}

func TestCartridgeInfoFile_Write_ReadOnly(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Attempt to write should fail
	data := []byte("title=HACKED\n")
	_, err = writer.Write(data)
	if err != fs.ErrPermission {
		t.Fatalf("Expected ErrPermission for write to read-only file, got: %v", err)
	}
}

func TestCartridgeInfoFile_Stat(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Name() != "info" {
		t.Errorf("Name() = %q, want %q", info.Name(), "info")
	}
	if info.IsDir() {
		t.Error("IsDir() = true, want false")
	}
	if info.Mode()&0200 != 0 { // Check not writable
		t.Errorf("Mode() = %o, should not be writable", info.Mode())
	}
}

func TestCartridgeInfoFile_NoCartridge(t *testing.T) {
	// Test with no cartridge loaded
	memory := &gb.Memory{
		Cart: nil, // No cartridge
	}

	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)

	fsys := newCartridgeInfoFS(gameboy)
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read should fail when no cartridge is loaded
	buf := make([]byte, 1024)
	_, err = f.Read(buf)
	if err == nil {
		t.Fatal("Expected error when reading with no cartridge, got nil")
	}
	if !strings.Contains(err.Error(), "no cartridge loaded") {
		t.Errorf("Error should mention no cartridge, got: %v", err)
	}
}

func TestCartridgeInfoFile_MultipleReads(t *testing.T) {
	fsys, _ := setupTestCartridgeInfoFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// First read
	buf1 := make([]byte, 1024)
	n1, err1 := f.Read(buf1)
	if err1 != nil && err1 != io.EOF {
		t.Fatalf("First read failed: %v", err1)
	}

	// Second read should return EOF
	buf2 := make([]byte, 1024)
	n2, err2 := f.Read(buf2)

	if err2 != io.EOF {
		t.Fatalf("Second read should return EOF, got: %v", err2)
	}
	if n2 != 0 {
		t.Fatalf("Second read returned %d bytes, want 0", n2)
	}

	// Verify first read got content
	content := string(buf1[:n1])
	if !strings.Contains(content, "title=POKEMON BLUE") {
		t.Error("First read missing title")
	}
}

func TestCartridgeInfoFile_RAMSizeDecoding(t *testing.T) {
	tests := []struct {
		ramSizeByte byte
		expectedKB  int
	}{
		{0x00, 0},
		{0x02, 8},
		{0x03, 32},
		{0x04, 128},
		{0x05, 64},
	}

	for _, tt := range tests {
		rom := createTestROM("TEST", 0x00, 0x00, tt.ramSizeByte, 0x00, 0x00)
		cartridge := cart.NewCart(rom, "test.gb")

		memory := &gb.Memory{Cart: cartridge}
		gameboy := &gb.Gameboy{}
		gameboy.Mu = sync.RWMutex{}
		gameboy.SetMemory(memory)

		fsys := newCartridgeInfoFS(gameboy)
		f, _ := fsys.Open(".")
		defer f.Close()

		buf := make([]byte, 1024)
		n, _ := f.Read(buf)
		content := string(buf[:n])

		expectedBytes := tt.expectedKB * 1024
		expectedStr := "ramSize=" + string(rune(expectedBytes/10000+'0')) + string(rune((expectedBytes/1000)%10+'0'))
		if expectedBytes == 0 {
			expectedStr = "ramSize=0"
		} else if expectedBytes == 8192 {
			expectedStr = "ramSize=8192"
		} else if expectedBytes == 32768 {
			expectedStr = "ramSize=32768"
		} else if expectedBytes == 65536 {
			expectedStr = "ramSize=65536"
		} else if expectedBytes == 131072 {
			expectedStr = "ramSize=131072"
		}

		if !strings.Contains(content, expectedStr) {
			t.Errorf("RAM size byte 0x%02X: expected %s in output, got: %s", tt.ramSizeByte, expectedStr, content)
		}
	}
}

func TestCartridgeInfoFile_ROMSizeDecoding(t *testing.T) {
	tests := []struct {
		romSizeByte  byte
		expectedKB   int
		expectedStr  string
	}{
		{0x00, 32, "romSize=32768"},        // 32KB
		{0x01, 64, "romSize=65536"},        // 64KB
		{0x02, 128, "romSize=131072"},      // 128KB
		{0x03, 256, "romSize=262144"},      // 256KB
		{0x04, 512, "romSize=524288"},      // 512KB
		{0x05, 1024, "romSize=1048576"},    // 1MB
		{0x06, 2048, "romSize=2097152"},    // 2MB
		{0x07, 4096, "romSize=4194304"},    // 4MB
	}

	for _, tt := range tests {
		rom := createTestROM("TEST", 0x00, tt.romSizeByte, 0x00, 0x00, 0x00)
		cartridge := cart.NewCart(rom, "test.gb")

		memory := &gb.Memory{Cart: cartridge}
		gameboy := &gb.Gameboy{}
		gameboy.Mu = sync.RWMutex{}
		gameboy.SetMemory(memory)

		fsys := newCartridgeInfoFS(gameboy)
		f, _ := fsys.Open(".")
		defer f.Close()

		buf := make([]byte, 1024)
		n, _ := f.Read(buf)
		content := string(buf[:n])

		if !strings.Contains(content, tt.expectedStr) {
			t.Errorf("ROM size byte 0x%02X: expected %s in output, got: %s", tt.romSizeByte, tt.expectedStr, content)
		}
	}
}
