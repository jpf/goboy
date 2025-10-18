// ABOUTME: Tests for APU state filesystem (binary format).
// ABOUTME: Validates reading and writing audio processing unit state.

package ninep

import (
	"encoding/binary"
	"io/fs"
	"math"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/apu"
	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAPUStateFS() (*apuStateFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	// Initialize APU
	sound := &apu.APU{}
	sound.Init(false) // Don't start audio playback
	gameboy.SetAPU(sound)

	fsys := newAPUStateFS(gameboy)
	return fsys, gameboy
}

func TestAPUStateFS_Open(t *testing.T) {
	fsys, _ := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()

	// Opening other names should fail
	_, err = fsys.Open("invalid")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestAPUStateFile_Read(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	// Set known APU state
	gameboy.Mu.Lock()
	playing, memory, lVol, rVol, tickCounter := gameboy.GetAPUState()
	_ = playing
	_ = memory
	_ = lVol
	_ = rVol
	_ = tickCounter
	// State should have default values from Init
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	// Read entire file
	buf := make([]byte, apuStateSize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, apuStateSize, n)

	// Verify binary format
	// Byte 0: playing flag (0 since we Init'd with false)
	assert.Equal(t, byte(0), buf[0], "playing should be false")

	// Bytes 1-52: memory (should be zeroed initially)
	// Bytes 53-60: lVol (float64)
	// Bytes 61-68: rVol (float64)
	// Bytes 69-76: tickCounter (float64)
}

func TestAPUStateFile_ReadWithState(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	// Set specific APU state using Write to registers
	gameboy.Mu.Lock()
	sound := gameboy.GetAPU()
	// Write to volume control register (0xFF24)
	sound.Write(0xFF24, 0x77) // Max volume both channels
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, apuStateSize)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, apuStateSize, n)

	// Verify memory contains the write
	// APU memory is indexed from 0xFF00, and serialization starts at byte 1
	memoryOffset := 1
	regOffset := 0x24 // Register 0xFF24 is at memory[0x24]
	assert.Equal(t, byte(0x77), buf[memoryOffset+regOffset], "Register 0xFF24 should contain 0x77")
}

func TestAPUStateFile_ReadAt(t *testing.T) {
	fsys, _ := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	reader, ok := f.(interface{ ReadAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement ReadAt")

	// Read playing flag at offset 0
	buf := make([]byte, 1)
	n, err := reader.ReadAt(buf, 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, n)

	// Read memory section at offset 1
	memBuf := make([]byte, 52)
	n, err = reader.ReadAt(memBuf, 1)
	assert.NoError(t, err)
	assert.Equal(t, 52, n)

	// Read volumes at offset 53
	volBuf := make([]byte, 16) // 2 float64s
	n, err = reader.ReadAt(volBuf, 53)
	assert.NoError(t, err)
	assert.Equal(t, 16, n)
}

func TestAPUStateFile_ReadAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestAPUStateFS()

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

func TestAPUStateFile_Write(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	// Construct binary state data
	data := make([]byte, apuStateSize)
	data[0] = 1 // playing = true

	// Set memory bytes
	for i := 0; i < 52; i++ {
		data[1+i] = byte(i)
	}

	// Set volumes and tickCounter
	binary.LittleEndian.PutUint64(data[53:61], math.Float64bits(0.5))   // lVol
	binary.LittleEndian.PutUint64(data[61:69], math.Float64bits(0.75))  // rVol
	binary.LittleEndian.PutUint64(data[69:77], math.Float64bits(123.45)) // tickCounter

	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, apuStateSize, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "apu-state-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestAPUStateFile_WriteAt(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok, "File should implement WriteAt")

	// Write just the playing flag at offset 0
	data := []byte{1}
	n, err := writer.WriteAt(data, 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "apu-state-write", cmd.Name)
		assert.Equal(t, 0, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestAPUStateFile_WriteAt_MemorySection(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ WriteAt([]byte, int64) (int, error) })
	require.True(t, ok)

	// Write 4 bytes to memory section at offset 1
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	n, err := writer.WriteAt(data, 1)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "apu-state-write", cmd.Name)
		assert.Equal(t, 1, cmd.Offset)
		assert.Equal(t, data, cmd.Data)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestAPUStateFile_WriteAt_OutOfBounds(t *testing.T) {
	fsys, _ := setupTestAPUStateFS()

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

func TestAPUStateFile_Stat(t *testing.T) {
	fsys, _ := setupTestAPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.Stat()
	require.NoError(t, err)

	assert.Equal(t, "state", info.Name())
	assert.Equal(t, int64(apuStateSize), info.Size())
	assert.Equal(t, fs.FileMode(0666), info.Mode())
	assert.False(t, info.IsDir())
}

func TestAPUStateFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestAPUStateFS()

	// Set some state
	gameboy.Mu.Lock()
	sound := gameboy.GetAPU()
	sound.Write(0xFF24, 0x77)
	gameboy.Mu.Unlock()

	// Read from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, apuStateSize)
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
