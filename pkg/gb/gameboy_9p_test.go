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

func TestProcessCommands_VramWrite(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	// Initialize VRAM to zeros
	for i := range gb.memory.VRAM {
		gb.memory.VRAM[i] = 0
	}

	// Queue vram-write command
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	gb.CommandChan <- Command{
		Name:   "vram-write",
		Offset: 0x1000,
		Data:   data,
	}

	// Process commands
	gb.ProcessCommands()

	// Verify write
	assert.Equal(t, byte(0xAA), gb.memory.VRAM[0x1000])
	assert.Equal(t, byte(0xBB), gb.memory.VRAM[0x1001])
	assert.Equal(t, byte(0xCC), gb.memory.VRAM[0x1002])
	assert.Equal(t, byte(0xDD), gb.memory.VRAM[0x1003])
}

func TestProcessCommands_VramWriteMultiple(t *testing.T) {
	gb := &Gameboy{}
	gb.setup()

	// Initialize VRAM to zeros
	for i := range gb.memory.VRAM {
		gb.memory.VRAM[i] = 0
	}

	// Queue multiple vram-write commands
	gb.CommandChan <- Command{
		Name:   "vram-write",
		Offset: 0,
		Data:   []byte{0x01, 0x02},
	}
	gb.CommandChan <- Command{
		Name:   "vram-write",
		Offset: 0x2000,
		Data:   []byte{0x03, 0x04},
	}

	// Process commands
	gb.ProcessCommands()

	// Verify both writes
	assert.Equal(t, byte(0x01), gb.memory.VRAM[0])
	assert.Equal(t, byte(0x02), gb.memory.VRAM[1])
	assert.Equal(t, byte(0x03), gb.memory.VRAM[0x2000])
	assert.Equal(t, byte(0x04), gb.memory.VRAM[0x2001])
}
