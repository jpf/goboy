// ABOUTME: Tests for PPU screen filesystem (binary format).
// ABOUTME: Validates reading and writing 69,120-byte RGB screen buffer.

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

const screenSize = 69120 // 160 * 144 * 3 = 69,120 bytes

func setupTestPPUScreenFS() (*binaryMemoryFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fsys := &binaryMemoryFS{
		gb: gameboy,
		getMemory: func(gb *gb.Gameboy) []byte {
			return gb.GetScreen()[:]
		},
		size:        screenSize,
		commandName: "ppu-screen-write",
		name:        "screen",
	}
	return fsys, gameboy
}

func TestPPUScreenFS_Open(t *testing.T) {
	fsys, _ := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Opening other names should fail
	_, err = fsys.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPPUScreenFile_Read(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	// Set known pattern in screen buffer (RGB stripes)
	gameboy.Mu.Lock()
	gameboy.SetScreen(0, 0, 255, 0, 0)     // Red pixel at (0,0)
	gameboy.SetScreen(1, 0, 0, 255, 0)     // Green pixel at (1,0)
	gameboy.SetScreen(2, 0, 0, 0, 255)     // Blue pixel at (2,0)
	gameboy.SetScreen(159, 143, 128, 64, 32) // Last pixel
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read entire file
	buf := make([]byte, screenSize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, screenSize, n)

	// Verify pattern - pixels are stored as [x][y][rgb]
	// Byte layout: column-major, 3 bytes per pixel (RGB)
	// Pixel (x,y) starts at byte: x*144*3 + y*3
	assert.Equal(t, byte(255), buf[0], "Pixel (0,0) R should be 255")
	assert.Equal(t, byte(0), buf[1], "Pixel (0,0) G should be 0")
	assert.Equal(t, byte(0), buf[2], "Pixel (0,0) B should be 0")

	assert.Equal(t, byte(0), buf[432], "Pixel (1,0) R should be 0")   // 1*144*3 = 432
	assert.Equal(t, byte(255), buf[433], "Pixel (1,0) G should be 255")
	assert.Equal(t, byte(0), buf[434], "Pixel (1,0) B should be 0")

	assert.Equal(t, byte(0), buf[864], "Pixel (2,0) R should be 0")   // 2*144*3 = 864
	assert.Equal(t, byte(0), buf[865], "Pixel (2,0) G should be 0")
	assert.Equal(t, byte(255), buf[866], "Pixel (2,0) B should be 255")

	// Last pixel at (159,143): 159*144*3 + 143*3 = 68688 + 429 = 69117
	assert.Equal(t, byte(128), buf[69117], "Last pixel R should be 128")
	assert.Equal(t, byte(64), buf[69118], "Last pixel G should be 64")
	assert.Equal(t, byte(32), buf[69119], "Last pixel B should be 32")
}

func TestPPUScreenFile_ReadAt(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	// Set pattern in middle of screen
	gameboy.Mu.Lock()
	gameboy.SetScreen(80, 72, 111, 222, 133) // Middle of screen
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read at offset 34560 (middle of file)
	buf := make([]byte, 12)
	n, err := reader.ReadAt(buf, 34560)
	assert.NoError(t, err)
	assert.Equal(t, 12, n)
}

func TestPPUScreenFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to read beyond bounds
	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, 70000)
	assert.Error(t, err)
}

func TestPPUScreenFile_Write(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	// Write some RGB data
	data := []byte{255, 128, 64, 32, 16, 8}
	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, 6, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-screen-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUScreenFile_WriteAt(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write at offset 34560 (middle)
	data := []byte{111, 222, 133}
	n, err := writer.WriteAt(data, 34560)
	assert.NoError(t, err)
	assert.Equal(t, 3, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-screen-write", cmd.Name)
		assert.Equal(t, 34560, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUScreenFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to write beyond bounds
	data := []byte{0xAA, 0xBB}
	_, err = writer.WriteAt(data, 70000)
	assert.Error(t, err)
}

func TestPPUScreenFile_Stat(t *testing.T) {
	fsys, _ := setupTestPPUScreenFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "screen", info.Name())
	assert.Equal(t, int64(screenSize), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestPPUScreenFile_ReadEOF(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	gameboy.Mu = sync.RWMutex{}
	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read to end
	buf := make([]byte, screenSize)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, screenSize, n)

	// Next read should return EOF
	n, err = f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestPPUScreenFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestPPUScreenFS()

	// Set pattern
	gameboy.Mu.Lock()
	gameboy.SetScreen(0, 0, 255, 0, 0)
	gameboy.SetScreen(1, 1, 0, 255, 0)
	gameboy.Mu.Unlock()

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, screenSize)
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
