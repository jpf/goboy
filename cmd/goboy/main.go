package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/Humpheh/goboy/pkg/ninep"
	"github.com/Humpheh/goboy/pkg/pixelbinding"
)

// The version of GoBoy
var version = "develop"

var (
	mute    = flag.Bool("mute", false, "mute sound output")
	dmgMode = flag.Bool("dmg", false, "set to force dmg mode")

	ninepPort = flag.Int("9p-port", 0, "enable 9P server on port (0 = disabled)")

	cpuprofile  = flag.String("cpuprofile", "", "write cpu profile to file (debugging)")
	vsyncOff    = flag.Bool("disableVsync", false, "set to disable vsync (debugging)")
	stepThrough = flag.Bool("stepthrough", false, "step through opcodes (debugging)")
	unlocked    = flag.Bool("unlocked", false, "if to unlock the cpu speed (debugging)")
)

func main() {
	flag.Parse()
	pixelbinding.Run(start)
}

func start(binding gb.IOBinding) {
	rom := flag.Arg(0)
	if rom == "" {
		log.Fatal("No ROM file specified. Please provide a ROM file as an argument.")
	}

	// If the CPU profile flag is set, then setup the profiling
	if *cpuprofile != "" {
		startCPUProfiling()
		defer pprof.StopCPUProfile()
	}

	if *unlocked {
		*mute = true
	}

	// Print the logo and the run settings to the console
	fmt.Printf("[\u001B[1;32mGoBoy\u001B[0m] %v :: apu=%v cgb=%v\n", version, !*mute, !*dmgMode)

	var opts []gb.GameboyOption
	if !*dmgMode {
		opts = append(opts, gb.WithCGBEnabled())
	}
	if !*mute {
		opts = append(opts, gb.WithSound())
	}

	// Initialise the GameBoy with the flag options
	gameboy, err := gb.New(rom, opts...)
	if err != nil {
		log.Fatal(err)
	}
	if *stepThrough {
		gameboy.Debug.OutputOpcodes = true
	}

	// Start 9P server if requested
	if *ninepPort != 0 {
		if err := ninep.Start(gameboy, *ninepPort); err != nil {
			log.Printf("Failed to start 9P server: %v", err)
		} else {
			printNinePInstructions(*ninepPort)
		}
	}

	// Create the monitor for pixels
	enableVSync := !(*vsyncOff || *unlocked)
	binding.SetEnableVSync(enableVSync)
	startGBLoop(gameboy, binding)
}

func startGBLoop(gameboy *gb.Gameboy, monitor gb.IOBinding) {
	frameTime := time.Second / gb.FramesSecond
	if *unlocked {
		frameTime = 1
	}

	ticker := time.NewTicker(frameTime)
	start := time.Now()
	frames := 0

	var cartName string
	if gameboy.IsCartLoaded() {
		cartName = gameboy.GetLoadedCart().GetName()
	}

	for range ticker.C {
		if !monitor.IsRunning() {
			return
		}

		frames++

		buttons := monitor.ProcessButtonInput()
		gameboy.ProcessInput(buttons)

		_ = gameboy.Update()
		monitor.Render(&gameboy.PreparedData)

		// Process any pending commands from 9P interface
		gameboy.ProcessCommands()

		since := time.Since(start)
		if since > time.Second {
			start = time.Now()

			title := fmt.Sprintf("GoBoy - %s (FPS: %2v)", cartName, frames)
			monitor.SetTitle(title)
			frames = 0
		}
	}
}

// Start the CPU profile to a the file passed in from the flag.
func startCPUProfiling() {
	log.Print("Starting CPU profile...")
	f, err := os.Create(*cpuprofile)
	if err != nil {
		log.Fatalf("Failed to create CPU profile: %v", err)
	}
	err = pprof.StartCPUProfile(f)
	if err != nil {
		log.Fatalf("Failed to start CPU profile: %v", err)
	}
}

func printNinePInstructions(port int) {
	fmt.Printf("\n=== 9P Server Started on port %d ===\n\n", port)
	fmt.Println("To test from Linux VM:")
	fmt.Println()
	fmt.Println("1. Mount the filesystem:")
	fmt.Println("   mkdir -p /tmp/goboy")
	fmt.Printf("   sudo mount -t 9p -o trans=tcp,port=%d <HOST_IP> /tmp/goboy\n", port)
	fmt.Println()
	fmt.Println("2. Test reading files:")
	fmt.Println("   cat /tmp/goboy/README")
	fmt.Println("   cat /tmp/goboy/ctl")
	fmt.Println()
	fmt.Println("3. Test pause command:")
	fmt.Println("   echo pause > /tmp/goboy/ctl")
	fmt.Println("   cat /tmp/goboy/ctl    # should show 'paused'")
	fmt.Println()
	fmt.Println("4. Test resume command:")
	fmt.Println("   echo resume > /tmp/goboy/ctl")
	fmt.Println("   cat /tmp/goboy/ctl    # should show 'running'")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the emulator when done testing.")
	fmt.Println()
}
