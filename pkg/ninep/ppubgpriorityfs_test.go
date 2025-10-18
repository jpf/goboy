// ABOUTME: Tests for PPU background priority filesystem (binary format).
// ABOUTME: Validates reading and writing 2,880-byte bit-packed priority map.

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

const bgPrioritySize = 2880 // 160 * 144 / 8 = 23040 / 8 = 2880

func setupTestPPUBGPriorityFS() (*binaryMemoryFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fsys := &binaryMemoryFS{
		gb: gameboy,
		getMemory: func(gb *gb.Gameboy) []byte {
			return gb.GetBGPriority()[:]
		},
		size:        bgPrioritySize,
		commandName: "ppu-bgpriority-write",
		name:        "bgpriority",
	}
	return fsys, gameboy
}

func TestPPUBGPriorityFS_Open(t *testing.T) {
	fsys, _ := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Opening other names should fail
	_, err = fsys.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPPUBGPriorityFile_Read(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	// Set known pattern - every 8th pixel has priority
	gameboy.Mu.Lock()
	gameboy.SetBGPriority(0, 0, true)   // Bit 0 of byte 0
	gameboy.SetBGPriority(0, 8, true)   // Bit 0 of byte 1
	gameboy.SetBGPriority(0, 16, true)  // Bit 0 of byte 2
	gameboy.SetBGPriority(1, 0, true)   // Bit 0 of byte 18 (144 bytes per column, 144/8=18)
	gameboy.SetBGPriority(159, 143, true) // Last pixel
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read entire file
	buf := make([]byte, bgPrioritySize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, bgPrioritySize, n)

	// Verify pattern (bit 0 should be set in specific bytes)
	assert.Equal(t, byte(0x01), buf[0], "Byte 0 should have bit 0 set")
	assert.Equal(t, byte(0x01), buf[1], "Byte 1 should have bit 0 set")
	assert.Equal(t, byte(0x01), buf[2], "Byte 2 should have bit 0 set")
	assert.Equal(t, byte(0x01), buf[18], "Byte 18 (column 1) should have bit 0 set")

	// Last pixel is at x=159, y=143
	// Byte index = (159 * 18) + (143 / 8) = 2862 + 17 = 2879
	// Bit index = 143 % 8 = 7
	assert.Equal(t, byte(0x80), buf[2879], "Last byte should have bit 7 set")
}

func TestPPUBGPriorityFile_ReadAt(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	// Set pattern in middle of data
	gameboy.Mu.Lock()
	gameboy.SetBGPriority(80, 72, true) // Middle of screen
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read at offset 1440 (middle of file)
	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 1440)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
}

func TestPPUBGPriorityFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to read beyond bounds
	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, 3000)
	assert.Error(t, err)
}

func TestPPUBGPriorityFile_Write(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	// Write some data (bit pattern 0b10101010)
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-bgpriority-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUBGPriorityFile_WriteAt(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write at offset 1440 (middle)
	data := []byte{0xFF, 0x00}
	n, err := writer.WriteAt(data, 1440)
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-bgpriority-write", cmd.Name)
		assert.Equal(t, 1440, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUBGPriorityFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Try to write beyond bounds
	data := []byte{0xAA, 0xBB}
	_, err = writer.WriteAt(data, 3000)
	assert.Error(t, err)
}

func TestPPUBGPriorityFile_Stat(t *testing.T) {
	fsys, _ := setupTestPPUBGPriorityFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "bgpriority", info.Name())
	assert.Equal(t, int64(bgPrioritySize), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestPPUBGPriorityFile_ReadEOF(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	gameboy.Mu = sync.RWMutex{}
	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read to end
	buf := make([]byte, bgPrioritySize)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, bgPrioritySize, n)

	// Next read should return EOF
	n, err = f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestPPUBGPriorityFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestPPUBGPriorityFS()

	// Set pattern
	gameboy.Mu.Lock()
	gameboy.SetBGPriority(0, 0, true)
	gameboy.SetBGPriority(1, 1, true)
	gameboy.Mu.Unlock()

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, bgPrioritySize)
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
