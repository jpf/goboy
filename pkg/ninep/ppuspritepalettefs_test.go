// ABOUTME: Tests for PPU sprite palette filesystem (binary format).
// ABOUTME: Validates reading and writing 66-byte CGB sprite palette data.

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

const spritePaletteSize = 66

func setupTestPPUSpritePaletteFS() (*binaryMemoryFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}
	gameboy.SetSpritePalette(gb.NewPalette())

	fsys := &binaryMemoryFS{
		gb: gameboy,
		getMemory: func(gb *gb.Gameboy) []byte {
			return gb.GetSpritePalette()[:]
		},
		size:        spritePaletteSize,
		commandName: "ppu-spritepalette-write",
		name:        "spritepalette",
	}
	return fsys, gameboy
}

func TestPPUSpritePaletteFS_Open(t *testing.T) {
	fsys, _ := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Opening other names should fail
	_, err = fsys.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPPUSpritePaletteFile_Read(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	// Set known pattern in spritepalette by directly modifying the internal structure
	gameboy.Mu.Lock()
	pal := gb.NewPalette()
	for i := 0; i < 64; i++ {
		pal.Palette[i] = byte(i)
	}
	pal.Index = 64
	pal.Inc = true
	gameboy.SetSpritePalette(pal)
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read entire file
	buf := make([]byte, spritePaletteSize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, spritePaletteSize, n)

	// Verify pattern (0-63 for Palette, 64 for Index, 1 for Inc)
	for i := 0; i < 64; i++ {
		assert.Equal(t, byte(i), buf[i], "Mismatch at palette index %d", i)
	}
	assert.Equal(t, byte(64), buf[64], "Mismatch at Index field")
	assert.Equal(t, byte(0x01), buf[65], "Mismatch at Inc field")
}

func TestPPUSpritePaletteFile_ReadAt(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	// Set known pattern
	gameboy.Mu.Lock()
	pal := gb.NewPalette()
	for i := 0; i < 64; i++ {
		pal.Palette[i] = byte(i & 0xFF)
	}
	pal.Index = 64
	pal.Inc = false
	gameboy.SetSpritePalette(pal)
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read at offset 32 (middle of palette data)
	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 32)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)

	// Verify values
	for i := 0; i < 10; i++ {
		assert.Equal(t, byte(32+i), buf[i])
	}
}

func TestPPUSpritePaletteFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to read beyond bounds
	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, 100)
	assert.Error(t, err)
}

func TestPPUSpritePaletteFile_Write(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	// Write some data
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-spritepalette-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUSpritePaletteFile_WriteAt(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write at offset 64 (Index and Inc fields)
	data := []byte{0x1F, 0x01}
	n, err := writer.WriteAt(data, 64)
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-spritepalette-write", cmd.Name)
		assert.Equal(t, 64, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUSpritePaletteFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to write beyond bounds
	data := []byte{0xAA, 0xBB}
	_, err = writer.WriteAt(data, 100)
	assert.Error(t, err)
}

func TestPPUSpritePaletteFile_Stat(t *testing.T) {
	fsys, _ := setupTestPPUSpritePaletteFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "spritepalette", info.Name())
	assert.Equal(t, int64(spritePaletteSize), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestPPUSpritePaletteFile_ReadEOF(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	gameboy.Mu = sync.RWMutex{}
	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read to end
	buf := make([]byte, spritePaletteSize)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, spritePaletteSize, n)

	// Next read should return EOF
	n, err = f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestPPUSpritePaletteFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestPPUSpritePaletteFS()

	// Set pattern
	gameboy.Mu.Lock()
	pal := gb.NewPalette()
	for i := 0; i < 64; i++ {
		pal.Palette[i] = byte(i)
	}
	pal.Index = 64
	pal.Inc = true
	gameboy.SetSpritePalette(pal)
	gameboy.Mu.Unlock()

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, spritePaletteSize)
			_, err = f.Read(buf)
			assert.NoError(t, err)

			done <- true
		}()
	}

	// Wait for all reads to complete
	for i := 0; i < 5; i++ {
		<-done
	}
}
