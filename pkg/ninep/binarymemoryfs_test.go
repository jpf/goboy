// ABOUTME: Tests for generic binary memory filesystem implementation.
// ABOUTME: Covers read/write operations, bounds checking, and command queueing.

package ninep

import (
	"io"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
)

func setupTestBinaryFS(size int64, cmdName string) (*binaryMemoryFS, *gb.Gameboy, []byte) {
	memory := make([]byte, size)
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fsys := &binaryMemoryFS{
		gb:          gameboy,
		getMemory:   func(*gb.Gameboy) []byte { return memory },
		size:        size,
		commandName: cmdName,
	}

	return fsys, gameboy, memory
}

func TestBinaryMemoryFS_Open(t *testing.T) {
	fsys, _, _ := setupTestBinaryFS(256, "test-write")

	// Open "." should succeed
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open(.) failed: %v", err)
	}
	defer f.Close()

	// Open "" should succeed (equivalent to ".")
	f2, err := fsys.Open("")
	if err != nil {
		t.Fatalf("Open(\"\") failed: %v", err)
	}
	defer f2.Close()

	// Open other names should fail
	_, err = fsys.Open("invalid")
	if err == nil {
		t.Fatal("Expected error for Open(\"invalid\"), got nil")
	}
}

func TestBinaryMemoryFile_Read(t *testing.T) {
	fsys, gameboy, memory := setupTestBinaryFS(256, "test-write")

	// Write known pattern to memory
	gameboy.Mu.Lock()
	for i := 0; i < 256; i++ {
		memory[i] = byte(i)
	}
	gameboy.Mu.Unlock()

	// Open and read
	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read first 128 bytes
	buf := make([]byte, 128)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 128 {
		t.Fatalf("Read returned %d bytes, want 128", n)
	}

	// Verify pattern
	for i := 0; i < 128; i++ {
		if buf[i] != byte(i) {
			t.Fatalf("buf[%d] = %d, want %d", i, buf[i], i)
		}
	}
}

func TestBinaryMemoryFile_ReadEOF(t *testing.T) {
	fsys, _, _ := setupTestBinaryFS(256, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read entire file
	buf := make([]byte, 256)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 256 {
		t.Fatalf("Read returned %d bytes, want 256", n)
	}

	// Next read should return EOF
	n, err = f.Read(buf)
	if err != io.EOF {
		t.Fatalf("Expected EOF, got err=%v, n=%d", err, n)
	}
}

func TestBinaryMemoryFile_ReadAt(t *testing.T) {
	fsys, gameboy, memory := setupTestBinaryFS(1024, "test-write")

	// Write known pattern
	gameboy.Mu.Lock()
	for i := 0; i < 1024; i++ {
		memory[i] = byte(i & 0xFF)
	}
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read at offset 0x100
	buf := make([]byte, 16)
	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement ReadAt")
	}

	n, err := reader.ReadAt(buf, 0x100)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("ReadAt returned %d bytes, want 16", n)
	}

	// Verify values (should be 0x00-0x0F, as 0x100 & 0xFF = 0x00)
	for i := 0; i < 16; i++ {
		expected := byte((0x100 + i) & 0xFF)
		if buf[i] != expected {
			t.Fatalf("buf[%d] = 0x%02x, want 0x%02x", i, buf[i], expected)
		}
	}
}

func TestBinaryMemoryFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _, _ := setupTestBinaryFS(256, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement ReadAt")
	}

	// Read at exact size should return EOF
	buf := make([]byte, 16)
	n, err := reader.ReadAt(buf, 256)
	if err != io.EOF {
		t.Fatalf("ReadAt at size should return EOF, got err=%v, n=%d", err, n)
	}

	// Read past end should return EOF
	n, err = reader.ReadAt(buf, 300)
	if err != io.EOF {
		t.Fatalf("ReadAt past end should return EOF, got err=%v, n=%d", err, n)
	}
}

func TestBinaryMemoryFile_Write(t *testing.T) {
	fsys, gameboy, _ := setupTestBinaryFS(256, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement Write")
	}

	// Write some data
	data := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write returned %d, want 4", n)
	}

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		if cmd.Name != "test-write" {
			t.Fatalf("Command name = %q, want \"test-write\"", cmd.Name)
		}
		if cmd.Offset != 0 {
			t.Fatalf("Command offset = %d, want 0", cmd.Offset)
		}
		if len(cmd.Data) != 4 {
			t.Fatalf("Command data len = %d, want 4", len(cmd.Data))
		}
		for i, b := range data {
			if cmd.Data[i] != b {
				t.Fatalf("Command data[%d] = 0x%02x, want 0x%02x", i, cmd.Data[i], b)
			}
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestBinaryMemoryFile_WriteAt(t *testing.T) {
	fsys, gameboy, _ := setupTestBinaryFS(1024, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement WriteAt")
	}

	// Write at offset 0x200
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	n, err := writer.WriteAt(data, 0x200)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("WriteAt returned %d, want 4", n)
	}

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		if cmd.Name != "test-write" {
			t.Fatalf("Command name = %q, want \"test-write\"", cmd.Name)
		}
		if cmd.Offset != 0x200 {
			t.Fatalf("Command offset = %d, want 0x200", cmd.Offset)
		}
		if len(cmd.Data) != 4 {
			t.Fatalf("Command data len = %d, want 4", len(cmd.Data))
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestBinaryMemoryFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _, _ := setupTestBinaryFS(256, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	if !ok {
		t.Fatal("File doesn't implement WriteAt")
	}

	// Write at exact size should fail
	data := []byte{0xAA}
	_, err = writer.WriteAt(data, 256)
	if err == nil {
		t.Fatal("Expected error for write at size, got nil")
	}

	// Write past end should fail
	_, err = writer.WriteAt(data, 300)
	if err == nil {
		t.Fatal("Expected error for write past end, got nil")
	}

	// Write that extends past end should fail
	data = []byte{0xAA, 0xBB, 0xCC, 0xDD}
	_, err = writer.WriteAt(data, 254)
	if err == nil {
		t.Fatal("Expected error for write extending past end, got nil")
	}
}

func TestBinaryMemoryFile_Stat(t *testing.T) {
	fsys, _, _ := setupTestBinaryFS(16384, "test-write")

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Name() != "memory" {
		t.Fatalf("Name = %q, want \"memory\"", info.Name())
	}
	if info.Size() != 16384 {
		t.Fatalf("Size = %d, want 16384", info.Size())
	}
	if info.Mode() != 0666 {
		t.Fatalf("Mode = %o, want 0666", info.Mode())
	}
	if info.IsDir() {
		t.Fatal("IsDir = true, want false")
	}
}

func TestBinaryMemoryFile_MultipleReads(t *testing.T) {
	fsys, gameboy, memory := setupTestBinaryFS(256, "test-write")

	// Write pattern
	gameboy.Mu.Lock()
	for i := 0; i < 256; i++ {
		memory[i] = byte(i)
	}
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	// Read 64 bytes at a time
	for chunk := 0; chunk < 4; chunk++ {
		buf := make([]byte, 64)
		n, err := f.Read(buf)
		if err != nil {
			t.Fatalf("Read chunk %d failed: %v", chunk, err)
		}
		if n != 64 {
			t.Fatalf("Read chunk %d returned %d bytes, want 64", chunk, n)
		}

		// Verify pattern
		for i := 0; i < 64; i++ {
			expected := byte(chunk*64 + i)
			if buf[i] != expected {
				t.Fatalf("chunk %d, buf[%d] = %d, want %d", chunk, i, buf[i], expected)
			}
		}
	}

	// Next read should EOF
	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != io.EOF {
		t.Fatalf("Expected EOF after 256 bytes, got err=%v, n=%d", err, n)
	}
}
