//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/lly8666/wobuzhidao/internal/windowsgui"
	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "WBD_WINDOWS_QUALIFY_FAIL", err)
		os.Exit(1)
	}
}

func run() (retErr error) {
	profilePath := flag.String("profile", "", "path to the WBD Windows client JSON profile")
	runFor := flag.Duration("run-for", 30*time.Second, "maximum connected qualification lifetime; 0 waits for interrupt or stop-file")
	stopFile := flag.String("stop-file", "", "optional file whose creation requests a clean Disconnect")
	readyFile := flag.String("ready-file", "", "optional file created only after Controller.Connect succeeds and removed after cleanup")
	flag.Parse()

	if *profilePath == "" {
		return errors.New("-profile is required")
	}
	if *runFor < 0 {
		return errors.New("-run-for must be >= 0")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve qualifier executable: %w", err)
	}
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return errors.New("ProgramData is not set")
	}
	stateDir := filepath.Join(programData, "WBD", "qualification")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create qualification state directory: %w", err)
	}
	profile, err := windowsgui.LoadRuntimeProfile(*profilePath, filepath.Dir(exe), stateDir)
	if err != nil {
		return err
	}

	for _, path := range []string{*stopFile, *readyFile} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear stale qualification signal %s: %w", path, err)
		}
	}

	controller := windowsruntime.NewController(nil, nil, nil)
	if err := controller.Connect(profile); err != nil {
		return err
	}
	connected := true
	defer func() {
		if !connected {
			return
		}
		if err := controller.Disconnect(); err != nil {
			if retErr == nil {
				retErr = fmt.Errorf("automatic qualification cleanup: %w", err)
			} else {
				retErr = errors.Join(retErr, fmt.Errorf("automatic qualification cleanup: %w", err))
			}
		}
	}()
	if *readyFile != "" {
		if err := os.WriteFile(*readyFile, []byte(fmt.Sprintf("pid=%d\n", os.Getpid())), 0o600); err != nil {
			return fmt.Errorf("write qualification ready-file: %w", err)
		}
		defer os.Remove(*readyFile)
	}

	fmt.Printf("WBD_WINDOWS_QUALIFY_CONNECTED pid=%d cleanup=automatic\n", os.Getpid())

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	var deadline <-chan time.Time
	var timer *time.Timer
	if *runFor > 0 {
		timer = time.NewTimer(*runFor)
		deadline = timer.C
		defer timer.Stop()
	}

	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-interrupt:
			fmt.Println("WBD_WINDOWS_QUALIFY_STOP reason=interrupt")
			goto disconnect
		case <-deadline:
			fmt.Println("WBD_WINDOWS_QUALIFY_STOP reason=deadline")
			goto disconnect
		case <-poll.C:
			if *stopFile == "" {
				continue
			}
			if _, err := os.Stat(*stopFile); err == nil {
				fmt.Println("WBD_WINDOWS_QUALIFY_STOP reason=stop-file")
				goto disconnect
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect stop-file: %w", err)
			}
		}
	}

disconnect:
	if err := controller.Disconnect(); err != nil {
		return err
	}
	connected = false
	if *readyFile != "" {
		if err := os.Remove(*readyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove qualification ready-file: %w", err)
		}
	}
	fmt.Println("WBD_WINDOWS_QUALIFY_CLEANUP_PASS routes=removed runtime=stopped")
	return nil
}
