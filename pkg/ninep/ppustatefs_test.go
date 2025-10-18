// ABOUTME: Tests for PPU state filesystem (text format).
// ABOUTME: Validates reading and writing picture processing unit state.

package ninep

import (
	"io"
	"sync"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestPPUStateFS() (*ppuStateFS, *gb.Gameboy) {
	gameboy := &gb.Gameboy{}
	gameboy.CommandChan = make(chan gb.Command, 32)
	gameboy.Mu = sync.RWMutex{}

	fsys := newPPUStateFS(gameboy)
	return fsys, gameboy
}

func TestPPUStateFS_Open(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	require.NotNil(t, f)
	defer f.Close()
}

func TestPPUStateFile_Read(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	// Set known PPU state
	gameboy.Mu.Lock()
	gameboy.SetPPUState(456, true, false)
	gameboy.Mu.Unlock()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	assert.Greater(t, n, 0)

	content := string(buf[:n])

	// Verify text format
	assert.Contains(t, content, "scanlineCounter=0x1c8", "Should contain scanlineCounter=456 (0x1c8)")
	assert.Contains(t, content, "screenCleared=0x01", "Should contain screenCleared=true")
	assert.Contains(t, content, "cgbMode=0x00", "Should contain cgbMode=false")
}

func TestPPUStateFile_ReadDefaultState(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(buf[:n])

	// Default state should have scanlineCounter=0
	assert.Contains(t, content, "scanlineCounter=")
	assert.Contains(t, content, "screenCleared=")
	assert.Contains(t, content, "cgbMode=")
}

func TestPPUStateFile_Write_Valid(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "File should implement Write")

	// Write valid state update
	data := []byte("scanlineCounter=0x100\n")
	n, err := writer.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-state-write", cmd.Name)
		assert.NotNil(t, cmd.State)
		if val, ok := cmd.State["scanlineCounter"]; ok {
			assert.Equal(t, byte(0x00), val, "High byte should be 0")
		}
		if val, ok := cmd.State["scanlineCounter_high"]; ok {
			assert.Equal(t, byte(0x01), val, "High byte should be 1 (0x100)")
		}
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUStateFile_Write_MultipleFields(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write multiple fields
	data := []byte("scanlineCounter=0x1c8\nscreenCleared=0x01\ncgbMode=0x01\n")
	_, err = writer.Write(data)
	assert.NoError(t, err)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "ppu-state-write", cmd.Name)
		assert.Equal(t, 4, len(cmd.State), "Should have 4 entries (scanlineCounter split into 2 + screenCleared + cgbMode)")
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUStateFile_Write_InvalidScanlineCounter(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write invalid scanlineCounter (>456)
	data := []byte("scanlineCounter=0x200\n")
	_, err = writer.Write(data)
	assert.Error(t, err, "Should reject scanlineCounter > 456")
	assert.Contains(t, err.Error(), "scanlineCounter")
}

func TestPPUStateFile_Write_InvalidScreenCleared(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write invalid screenCleared (>1)
	data := []byte("screenCleared=0x02\n")
	_, err = writer.Write(data)
	assert.Error(t, err, "Should reject screenCleared > 1")
}

func TestPPUStateFile_Write_InvalidCgbMode(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write invalid cgbMode (>1)
	data := []byte("cgbMode=0x02\n")
	_, err = writer.Write(data)
	assert.Error(t, err, "Should reject cgbMode > 1")
}

func TestPPUStateFile_Write_UnknownKey(t *testing.T) {
	fsys, _ := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write unknown key
	data := []byte("unknownField=0x42\n")
	_, err = writer.Write(data)
	assert.Error(t, err, "Should reject unknown keys")
}

func TestPPUStateFile_Write_WithComments(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write with comments and blank lines
	data := []byte(`# PPU State
scanlineCounter=0x100

# Flags
screenCleared=0x01
`)
	_, err = writer.Write(data)
	assert.NoError(t, err)

	// Verify command
	select {
	case cmd := <-gameboy.CommandChan:
		assert.NotNil(t, cmd.State)
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUStateFile_Write_DecimalValues(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	f, err := fsys.Open(".")
	require.NoError(t, err)
	defer f.Close()

	writer, ok := f.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write decimal value
	data := []byte("scanlineCounter=456\n")
	_, err = writer.Write(data)
	assert.NoError(t, err)

	// Verify command
	select {
	case <-gameboy.CommandChan:
		// Success
	default:
		t.Fatal("Expected command in channel")
	}
}

func TestPPUStateFile_ConcurrentRead(t *testing.T) {
	fsys, gameboy := setupTestPPUStateFS()

	gameboy.Mu.Lock()
	gameboy.SetPPUState(100, true, false)
	gameboy.Mu.Unlock()

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			f, err := fsys.Open(".")
			require.NoError(t, err)
			defer f.Close()

			buf := make([]byte, 512)
			_, err = f.Read(buf)
			if err != nil && err != io.EOF {
				t.Errorf("Read failed: %v", err)
			}

			done <- true
		}()
	}

	// Wait for all reads to complete
	for i := 0; i < 5; i++ {
		<-done
	}
}
