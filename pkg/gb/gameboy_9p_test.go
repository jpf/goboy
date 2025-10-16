package gb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPaused(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	// Initially not paused
	assert.False(t, gb.IsPaused())

	// Pause and verify
	gb.SetPaused(true)
	assert.True(t, gb.IsPaused())

	// Resume and verify
	gb.SetPaused(false)
	assert.False(t, gb.IsPaused())
}

func TestProcessCommandsPause(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	// Send pause command
	gb.CommandChan <- Command{Name: "pause"}
	gb.ProcessCommands()

	assert.True(t, gb.IsPaused())
}

func TestProcessCommandsResume(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()
	gb.SetPaused(true)

	// Send resume command
	gb.CommandChan <- Command{Name: "resume"}
	gb.ProcessCommands()

	assert.False(t, gb.IsPaused())
}

func TestProcessCommandsMultiple(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	// Queue multiple commands
	gb.CommandChan <- Command{Name: "pause"}
	gb.CommandChan <- Command{Name: "resume"}
	gb.CommandChan <- Command{Name: "pause"}

	gb.ProcessCommands()

	assert.True(t, gb.IsPaused())
}

func TestProcessCommandsUnknown(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	gb.CommandChan <- Command{Name: "invalid"}
	gb.ProcessCommands()

	// Should not crash, state unchanged
	assert.False(t, gb.IsPaused())
}

func TestPausedThreadSafety(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			gb.SetPaused(i%2 == 0)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			_ = gb.IsPaused()
		}
		done <- true
	}()

	// Toggle goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			gb.togglePaused()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
