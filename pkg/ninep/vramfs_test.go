// ABOUTME: Unit tests for VRAM fs.FS implementation.
// ABOUTME: Tests read/write operations and offset-based access.

package ninep

import (
	"io"
	"io/fs"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVramFS_Open(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 10)
	vfs := newVramFS(gameboy)

	// Open "." should succeed
	f, err := vfs.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Open other names should fail
	_, err = vfs.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestVramFile_Read(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	// Create minimal gameboy structure for testing
	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	// Write known pattern to VRAM
	gameboy.Mu.Lock()
	vram := gameboy.GetVRAM()
	for i := 0; i < 0x4000; i++ {
		vram[i] = byte(i & 0xFF)
	}
	gameboy.Mu.Unlock()

	// Open and read
	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read first 256 bytes
	buf := make([]byte, 256)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 256, n)

	// Verify pattern
	for i := 0; i < 256; i++ {
		assert.Equal(t, byte(i), buf[i], "Mismatch at index %d", i)
	}
}

func TestVramFile_ReadEOF(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	vf := f.(*vramFile)

	// Read to end
	buf := make([]byte, 0x4000)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 0x4000, n)

	// Next read should return EOF
	n, err = f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	// Verify readPos
	assert.Equal(t, 0x4000, vf.readPos)
}

func TestVramFile_Write(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Write some data
	data := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "vram-write", cmd.Name)
		assert.Equal(t, int64(0), cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestVramFile_ReadAt(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	// Write known pattern
	gameboy.Mu.Lock()
	vram := gameboy.GetVRAM()
	for i := 0; i < 0x4000; i++ {
		vram[i] = byte(i & 0xFF)
	}
	gameboy.Mu.Unlock()

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read at offset 0x1000
	buf := make([]byte, 16)
	n, err := reader.ReadAt(buf, 0x1000)
	assert.NoError(t, err)
	assert.Equal(t, 16, n)

	// Verify values
	for i := 0; i < 16; i++ {
		expected := byte((0x1000 + i) & 0xFF)
		assert.Equal(t, expected, buf[i], "Mismatch at offset 0x%x", 0x1000+i)
	}
}

func TestVramFile_ReadAt_Bank1(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	// Write pattern to bank 1 (offset 0x2000)
	gameboy.Mu.Lock()
	vram := gameboy.GetVRAM()
	for i := 0x2000; i < 0x4000; i++ {
		vram[i] = byte((i >> 8) & 0xFF)
	}
	gameboy.Mu.Unlock()

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Read from bank 1
	buf := make([]byte, 16)
	n, err := reader.ReadAt(buf, 0x2000)
	assert.NoError(t, err)
	assert.Equal(t, 16, n)

	// All should be 0x20
	for i := 0; i < 16; i++ {
		assert.Equal(t, byte(0x20), buf[i])
	}
}

func TestVramFile_ReadAt_OutOfBounds(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to read beyond bounds
	buf := make([]byte, 16)
	_, err = reader.ReadAt(buf, 0x5000)
	assert.Error(t, err)
	assert.IsType(t, &fs.PathError{}, err)
}

func TestVramFile_WriteAt(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write at offset 0x800
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	n, err := writer.WriteAt(data, 0x800)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "vram-write", cmd.Name)
		assert.Equal(t, int64(0x800), cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestVramFile_WriteAt_OutOfBounds(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to write beyond bounds
	data := []byte{0xAA, 0xBB}
	_, err = writer.WriteAt(data, 0x5000)
	assert.Error(t, err)
	assert.IsType(t, &fs.PathError{}, err)
}

func TestVramFile_Stat(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "vram", info.Name())
	assert.Equal(t, int64(0x4000), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestVramFile_MultipleReads(t *testing.T) {
	gameboy := &gb.Gameboy{}
	gameboy.Mu = sync.RWMutex{}
	gameboy.CommandChan = make(chan gb.Command, 10)

	mem := &gb.Memory{}
	gameboy.SetMemory(mem)

	// Write pattern
	gameboy.Mu.Lock()
	vram := gameboy.GetVRAM()
	for i := 0; i < 0x4000; i++ {
		vram[i] = byte(i & 0xFF)
	}
	gameboy.Mu.Unlock()

	vfs := newVramFS(gameboy)
	f, err := vfs.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read in chunks
	buf1 := make([]byte, 100)
	n1, err := f.Read(buf1)
	assert.NoError(t, err)
	assert.Equal(t, 100, n1)

	buf2 := make([]byte, 100)
	n2, err := f.Read(buf2)
	assert.NoError(t, err)
	assert.Equal(t, 100, n2)

	// Verify data continuity
	for i := 0; i < 100; i++ {
		assert.Equal(t, byte(i), buf1[i])
		assert.Equal(t, byte(100+i), buf2[i])
	}
}
