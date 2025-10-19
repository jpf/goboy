// ABOUTME: Unit tests for control file implementation.
// ABOUTME: Tests read/write operations and command processing via ctl file.

package ninep

import (
	"io"
	"io/fs"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGameboy creates a minimal Gameboy instance for testing
func testGameboy() *gb.Gameboy {
	gameboy := &gb.Gameboy{}
	// Initialize only the fields needed for 9P testing
	gameboy.CommandChan = make(chan gb.Command, 10)
	return gameboy
}

func TestCtlFileRead_Running(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	require.NoError(t, err)

	content := string(buf[:n])
	assert.Contains(t, content, "running")
	assert.Contains(t, content, "pause - pause emulation")
	assert.Contains(t, content, "resume - resume emulation")
}

func TestCtlFileRead_Paused(t *testing.T) {
	gameboy := testGameboy()
	gameboy.SetPaused(true)

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	require.NoError(t, err)

	content := string(buf[:n])
	assert.Contains(t, content, "paused")
}

func TestCtlFileWrite_Pause(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	// Cast to writer interface
	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "ctlFile should implement Write")

	n, err := writer.Write([]byte("pause\n"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	// Process the command
	gameboy.ProcessCommands()

	assert.True(t, gameboy.IsPaused())
}

func TestCtlFileWrite_Resume(t *testing.T) {
	gameboy := testGameboy()
	gameboy.SetPaused(true)

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	n, err := writer.Write([]byte("resume"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	gameboy.ProcessCommands()
	assert.False(t, gameboy.IsPaused())
}

func TestCtlFileWrite_EmptyWrite(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Empty write (from truncate) should succeed
	n, err := writer.Write([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// State should be unchanged
	assert.False(t, gameboy.IsPaused())
}

func TestCtlFileWrite_UnknownCommand(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	_, err = writer.Write([]byte("invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestCtlFileMultipleReads(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	// First read
	buf1 := make([]byte, 10)
	n1, err := file.Read(buf1)
	require.NoError(t, err)
	assert.Greater(t, n1, 0)

	// Second read should continue from where first left off
	buf2 := make([]byte, 100)
	n2, err := file.Read(buf2)
	if err != io.EOF {
		require.NoError(t, err)
	}

	// Concatenate reads should equal full content
	fullContent := string(buf1[:n1]) + string(buf2[:n2])
	assert.Contains(t, fullContent, "running")
}

func TestCtlFileStat(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	info, err := file.Stat()
	require.NoError(t, err)

	assert.Equal(t, "ctl", info.Name())
	assert.False(t, info.IsDir())
	assert.Equal(t, fs.FileMode(0666), info.Mode().Perm())
	assert.Greater(t, info.Size(), int64(0))
}

func TestCtlFileWrite_StepDefault(t *testing.T) {
	gameboy := testGameboy()
	gameboy.SetPaused(true)

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write step command
	n, err := writer.Write([]byte("step"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	// Verify command was queued with count=1
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "step", cmd.Name)
		assert.Equal(t, 1, cmd.Count)
	default:
		t.Fatal("Expected step command in channel")
	}
}

func TestCtlFileWrite_StepWithCount(t *testing.T) {
	gameboy := testGameboy()
	gameboy.SetPaused(true)

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write step command with count
	n, err := writer.Write([]byte("step 10"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)

	// Verify command was queued with count=10
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "step", cmd.Name)
		assert.Equal(t, 10, cmd.Count)
	default:
		t.Fatal("Expected step command in channel")
	}
}

func TestCtlFileWrite_StepInvalidCount(t *testing.T) {
	gameboy := testGameboy()
	gameboy.SetPaused(true)

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write step command with invalid count
	_, err = writer.Write([]byte("step invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step count")

	// Write step command with negative count
	_, err = writer.Write([]byte("step -5"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step count")

	// Write step command with zero count
	_, err = writer.Write([]byte("step 0"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step count")
}

func TestCtlFileRead_IncludesStepCommand(t *testing.T) {
	gameboy := testGameboy()

	ctlfs := newCtlFS(gameboy)
	file, err := ctlfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	require.NoError(t, err)

	content := string(buf[:n])
	assert.Contains(t, content, "step [N] - advance N frames")
}
