// ABOUTME: Tests for CPU state filesystem (text format).
// ABOUTME: Validates register read/write, key=value parsing, and command generation.

package ninep

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
)

func setupTestCPUStateFS() (*cpuStateFS, *gb.Gameboy, *gb.CPU) {
	cpu := &gb.CPU{}
	cpu.Init(false) // Initialize with non-CGB values

	memory := &gb.Memory{}

	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetMemory(memory)
	gameboy.SetCPU(cpu)

	fsys := newCPUStateFS(gameboy)
	return fsys, gameboy, cpu
}

func TestCPUStateFS_Open(t *testing.T) {
	fsys, _, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
}

func TestCPUStateFile_Read(t *testing.T) {
	fsys, gameboy, cpu := setupTestCPUStateFS()

	// Set known CPU state
	gameboy.Mu.Lock()
	cpu.AF.Set(0x01B0)
	cpu.BC.Set(0x0013)
	cpu.DE.Set(0xD8A0)
	cpu.HL.Set(0x014D)
	cpu.SP.Set(0xFFFE)
	cpu.PC = 0x0100
	cpu.Divider = 0x00AB
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
	if !strings.Contains(content, "# Registers") {
		t.Error("Missing '# Registers' section header")
	}
	if !strings.Contains(content, "AF=0x01B0") {
		t.Errorf("Missing or incorrect AF, got: %s", content)
	}
	if !strings.Contains(content, "BC=0x0013") {
		t.Errorf("Missing or incorrect BC, got: %s", content)
	}
	if !strings.Contains(content, "DE=0xD8A0") {
		t.Errorf("Missing or incorrect DE, got: %s", content)
	}
	if !strings.Contains(content, "HL=0x014D") {
		t.Errorf("Missing or incorrect HL, got: %s", content)
	}

	if !strings.Contains(content, "# Program State") {
		t.Error("Missing '# Program State' section header")
	}
	if !strings.Contains(content, "SP=0xFFFE") {
		t.Errorf("Missing or incorrect SP, got: %s", content)
	}
	if !strings.Contains(content, "PC=0x0100") {
		t.Errorf("Missing or incorrect PC, got: %s", content)
	}

	if !strings.Contains(content, "# Timer") {
		t.Error("Missing '# Timer' section header")
	}
	if !strings.Contains(content, "Divider=0x00AB") {
		t.Errorf("Missing or incorrect Divider, got: %s", content)
	}
}

func TestCPUStateFile_Write_Valid(t *testing.T) {
	fsys, gameboy, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write valid CPU register update
	data := []byte("PC=0x0150\n")
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
		if cmd.Name != "cpu-write" {
			t.Fatalf("Command name = %q, want \"cpu-write\"", cmd.Name)
		}
		if cmd.CPUState == nil {
			t.Fatal("Command.CPUState is nil")
		}
		if val, ok := cmd.CPUState["PC"]; !ok || val != 0x0150 {
			t.Fatalf("CPUState[PC] = 0x%04X, want 0x0150", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCPUStateFile_Write_PartialUpdate(t *testing.T) {
	fsys, gameboy, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write multiple registers
	data := []byte("AF=0xFFFF\nPC=0x0200\nDivider=0x1234\n")
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify command has all three fields
	select {
	case cmd := <-gameboy.CommandChan:
		if len(cmd.CPUState) != 3 {
			t.Fatalf("Command.CPUState has %d entries, want 3", len(cmd.CPUState))
		}
		if val, ok := cmd.CPUState["AF"]; !ok || val != 0xFFFF {
			t.Fatal("AF not set correctly")
		}
		if val, ok := cmd.CPUState["PC"]; !ok || val != 0x0200 {
			t.Fatal("PC not set correctly")
		}
		if val, ok := cmd.CPUState["Divider"]; !ok || val != 0x1234 {
			t.Fatal("Divider not set correctly")
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCPUStateFile_Write_AllRegisters(t *testing.T) {
	fsys, gameboy, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write all registers
	data := []byte(`AF=0x1111
BC=0x2222
DE=0x3333
HL=0x4444
SP=0x5555
PC=0x6666
Divider=0x7777
`)
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify command has all fields
	select {
	case cmd := <-gameboy.CommandChan:
		if len(cmd.CPUState) != 7 {
			t.Fatalf("Command.CPUState has %d entries, want 7", len(cmd.CPUState))
		}
		expectedVals := map[string]uint16{
			"AF":      0x1111,
			"BC":      0x2222,
			"DE":      0x3333,
			"HL":      0x4444,
			"SP":      0x5555,
			"PC":      0x6666,
			"Divider": 0x7777,
		}
		for key, expected := range expectedVals {
			if val, ok := cmd.CPUState[key]; !ok || val != expected {
				t.Fatalf("CPUState[%s] = 0x%04X, want 0x%04X", key, val, expected)
			}
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCPUStateFile_Write_UnknownKey(t *testing.T) {
	fsys, _, _ := setupTestCPUStateFS()

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
	data := []byte("UnknownField=0x1234\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Error should mention unknown field, got: %v", err)
	}
}

func TestCPUStateFile_Write_WithComments(t *testing.T) {
	fsys, gameboy, _ := setupTestCPUStateFS()

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
	data := []byte(`# Registers
AF=0xABCD

# Program State
PC=0x0150
`)
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify command has both fields (comments ignored)
	select {
	case cmd := <-gameboy.CommandChan:
		if len(cmd.CPUState) != 2 {
			t.Fatalf("Command.CPUState has %d entries, want 2", len(cmd.CPUState))
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCPUStateFile_Write_DecimalValues(t *testing.T) {
	fsys, gameboy, _ := setupTestCPUStateFS()

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
	data := []byte("PC=256\nDivider=65535\n")
	_, err = writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case cmd := <-gameboy.CommandChan:
		if val, ok := cmd.CPUState["PC"]; !ok || val != 256 {
			t.Fatalf("CPUState[PC] = %d, want 256", val)
		}
		if val, ok := cmd.CPUState["Divider"]; !ok || val != 65535 {
			t.Fatalf("CPUState[Divider] = %d, want 65535", val)
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestCPUStateFile_Write_InvalidValue(t *testing.T) {
	fsys, _, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write value that's too large for uint16
	data := []byte("PC=0x10000\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for value > 0xFFFF, got nil")
	}
}

func TestCPUStateFile_Write_InvalidFormat(t *testing.T) {
	fsys, _, _ := setupTestCPUStateFS()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write invalid format (no =)
	data := []byte("PC 0x0100\n")
	_, err = writer.Write(data)
	if err == nil {
		t.Fatal("Expected error for invalid format, got nil")
	}
}

func TestCPUStateFile_MultipleReads(t *testing.T) {
	fsys, gameboy, cpu := setupTestCPUStateFS()

	// Set known state
	gameboy.Mu.Lock()
	cpu.PC = 0x1234
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// First read
	buf1 := make([]byte, 512)
	n1, err1 := f.Read(buf1)
	if err1 != nil && err1 != io.EOF {
		t.Fatalf("First read failed: %v", err1)
	}

	// Second read should return EOF
	buf2 := make([]byte, 512)
	n2, err2 := f.Read(buf2)

	if err2 != io.EOF {
		t.Fatalf("Second read should return EOF, got: %v", err2)
	}
	if n2 != 0 {
		t.Fatalf("Second read returned %d bytes, want 0", n2)
	}

	// Verify first read got the content
	content := string(buf1[:n1])
	if !strings.Contains(content, "PC=0x1234") {
		t.Error("First read missing PC value")
	}
}
