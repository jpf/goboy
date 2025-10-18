// ABOUTME: Tests for PPU tilescanline filesystem (binary format).
// ABOUTME: Validates reading and writing 160-byte scanline tile data.

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

const tileScanlineSize = 160

func setupTestPPUTileScanlineFS() (*binaryMemoryFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fsys := &binaryMemoryFS{
		gb: gameboy,
		getMemory: func(gb *gb.Gameboy) []byte {
			return gb.GetTileScanline()[:]
		},
		size:        tileScanlineSize,
		commandName: "ppu-tilescanline-write",
		name:        "tilescanline",
	}
	return fsys, gameboy
}

func TestPPUTileScanlineFS_Open(t *testing.T) {
	fsys, _ := setupTestPPUTileScanlineFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Opening other names should fail
	_, err = fsys.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPPUTileScanlineFile_Read(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

	// Set known pattern in tileScanline
	gameboy.Mu.Lock()
	tiles := gameboy.GetTileScanline()
	for i := 0; i < 160; i++ {
		tiles[i] = byte(i)
	}
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read entire file
	buf := make([]byte, tileScanlineSize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, tileScanlineSize, n)

	// Verify pattern
	for i := 0; i < 160; i++ {
		assert.Equal(t, byte(i), buf[i], "Mismatch at index %d", i)
	}
}

func TestPPUTileScanlineFile_ReadAt(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

	// Set known pattern
	gameboy.Mu.Lock()
	tiles := gameboy.GetTileScanline()
	for i := 0; i < 160; i++ {
		tiles[i] = byte(i & 0xFF)
	}
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read at offset 50
	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 50)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)

	// Verify values
	for i := 0; i < 10; i++ {
		assert.Equal(t, byte(50+i), buf[i])
	}
}

func TestPPUTileScanlineFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUTileScanlineFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to read beyond bounds
	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, 200)
	assert.Error(t, err)
}

func TestPPUTileScanlineFile_Write(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

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
		assert.Equal(t, "ppu-tilescanline-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUTileScanlineFile_WriteAt(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write at offset 50
	data := []byte{0x11, 0x22, 0x33, 0x44}
	n, err := writer.WriteAt(data, 50)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-tilescanline-write", cmd.Name)
		assert.Equal(t, 50, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUTileScanlineFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUTileScanlineFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to write beyond bounds
	data := []byte{0xAA, 0xBB}
	_, err = writer.WriteAt(data, 200)
	assert.Error(t, err)
}

func TestPPUTileScanlineFile_Stat(t *testing.T) {
	fsys, _ := setupTestPPUTileScanlineFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "tilescanline", info.Name())
	assert.Equal(t, int64(tileScanlineSize), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestPPUTileScanlineFile_ReadEOF(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

	gameboy.Mu = sync.RWMutex{}
	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read to end
	buf := make([]byte, tileScanlineSize)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, tileScanlineSize, n)

	// Next read should return EOF
	n, err = f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestPPUTileScanlineFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestPPUTileScanlineFS()

	// Set pattern
	gameboy.Mu.Lock()
	tiles := gameboy.GetTileScanline()
	for i := 0; i < 160; i++ {
		tiles[i] = byte(i)
	}
	gameboy.Mu.Unlock()

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, tileScanlineSize)
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
