// ABOUTME: Unit tests for button input filesystem.
// ABOUTME: Tests command parsing and queuing for button input operations.

package ninep

import (
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestButtonsFS_WritePress(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok, "buttonsFile should implement Write")

	// Write press command
	n, err := writer.Write([]byte("press a"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "button-press", cmd.Name)
		assert.Equal(t, []byte{byte(gb.ButtonA)}, cmd.Data)
	default:
		t.Fatal("Expected button-press command in channel")
	}
}

func TestButtonsFS_WriteRelease(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write release command
	n, err := writer.Write([]byte("release b"))
	require.NoError(t, err)
	assert.Equal(t, 9, n)

	// Verify command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "button-release", cmd.Name)
		assert.Equal(t, []byte{byte(gb.ButtonB)}, cmd.Data)
	default:
		t.Fatal("Expected button-release command in channel")
	}
}

func TestButtonsFS_WriteMomentary(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write momentary command (just "start")
	n, err := writer.Write([]byte("start"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	// Verify press command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "button-press", cmd.Name)
		assert.Equal(t, []byte{byte(gb.ButtonStart)}, cmd.Data)
	default:
		t.Fatal("Expected button-press command in channel")
	}

	// Verify release command was queued
	select {
	case cmd := <-gameboy.CommandChan:
		assert.Equal(t, "button-release", cmd.Name)
		assert.Equal(t, []byte{byte(gb.ButtonStart)}, cmd.Data)
	default:
		t.Fatal("Expected button-release command in channel")
	}
}

func TestButtonsFS_WriteInvalidButton(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write with invalid button name
	_, err = writer.Write([]byte("press invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown button")
}

func TestButtonsFS_WriteInvalidAction(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write with invalid action
	_, err = writer.Write([]byte("hold a"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid action")
}

func TestButtonsFS_WriteInvalidFormat(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write with too many arguments
	_, err = writer.Write([]byte("press a b c"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid button command format")
}

func TestButtonsFS_WriteEmpty(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write empty string (should be ignored)
	n, err := writer.Write([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// Channel should be empty
	select {
	case <-gameboy.CommandChan:
		t.Fatal("Expected no commands in channel")
	default:
		// Pass
	}
}

func TestButtonsFS_AllButtons(t *testing.T) {
	tests := []struct {
		input    string
		expected gb.Button
	}{
		{"a", gb.ButtonA},
		{"b", gb.ButtonB},
		{"select", gb.ButtonSelect},
		{"start", gb.ButtonStart},
		{"up", gb.ButtonUp},
		{"down", gb.ButtonDown},
		{"left", gb.ButtonLeft},
		{"right", gb.ButtonRight},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gameboy := testGameboy()

			bfs := newButtonsFS(gameboy)
			file, err := bfs.Open(".")
			require.NoError(t, err)
			defer file.Close()

			writer, ok := file.(interface{ Write([]byte) (int, error) })
			require.True(t, ok)

			// Write momentary command
			_, err = writer.Write([]byte(tt.input))
			require.NoError(t, err)

			// Verify press command
			cmd := <-gameboy.CommandChan
			assert.Equal(t, "button-press", cmd.Name)
			assert.Equal(t, []byte{byte(tt.expected)}, cmd.Data)

			// Verify release command
			cmd = <-gameboy.CommandChan
			assert.Equal(t, "button-release", cmd.Name)
			assert.Equal(t, []byte{byte(tt.expected)}, cmd.Data)
		})
	}
}

func TestButtonsFS_CaseInsensitive(t *testing.T) {
	gameboy := testGameboy()

	bfs := newButtonsFS(gameboy)
	file, err := bfs.Open(".")
	require.NoError(t, err)
	defer file.Close()

	writer, ok := file.(interface{ Write([]byte) (int, error) })
	require.True(t, ok)

	// Write with uppercase (should work - parseButtonName uses ToLower)
	_, err = writer.Write([]byte("A"))
	require.NoError(t, err)

	// Verify press command was queued
	cmd := <-gameboy.CommandChan
	assert.Equal(t, "button-press", cmd.Name)

	// Also verify release command (momentary)
	cmd = <-gameboy.CommandChan
	assert.Equal(t, "button-release", cmd.Name)
}

func TestParseButtonName(t *testing.T) {
	tests := []struct {
		input    string
		expected gb.Button
		valid    bool
	}{
		{"a", gb.ButtonA, true},
		{"A", gb.ButtonA, true},
		{"b", gb.ButtonB, true},
		{"select", gb.ButtonSelect, true},
		{"SELECT", gb.ButtonSelect, true},
		{"start", gb.ButtonStart, true},
		{"up", gb.ButtonUp, true},
		{"down", gb.ButtonDown, true},
		{"left", gb.ButtonLeft, true},
		{"right", gb.ButtonRight, true},
		{"invalid", 0, false},
		{"", 0, false},
		{"x", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			button, ok := parseButtonName(tt.input)
			assert.Equal(t, tt.valid, ok)
			if tt.valid {
				assert.Equal(t, tt.expected, button)
			}
		})
	}
}
