package setup

import (
	"os"
	"os/exec"
	"runtime"
	"time"
)

// openBrowser attempts to open url in the user's default browser.
// It respects the $BROWSER environment variable so CI pipelines can
// set BROWSER=echo to suppress real browser launches.
// Returns true if a browser command was successfully started.
func openBrowser(url string) bool {
	for _, args := range browserCandidates() {
		cmd := exec.Command(args[0], append(args[1:], url)...)
		if cmd.Start() == nil && commandCompletedWithin(cmd, 3*time.Second) {
			return true
		}
	}
	return false
}

// browserCandidates returns candidate commands in priority order.
func browserCandidates() [][]string {
	var cmds [][]string

	// Allow the caller to override with a custom browser (useful in tests).
	if exe := os.Getenv("BROWSER"); exe != "" {
		cmds = append(cmds, []string{exe})
	}

	switch runtime.GOOS {
	case "darwin":
		cmds = append(cmds, []string{"/usr/bin/open"})
	case "windows":
		cmds = append(cmds, []string{"cmd", "/c", "start"})
	default:
		// Linux / FreeBSD — only attempt if a display is available.
		if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
			cmds = append(cmds, []string{"xdg-open"})
		}
	}

	return cmds
}

// commandCompletedWithin waits up to d for cmd to exit.
// Some launchers (like macOS `open`) exit immediately after handing off to
// the browser; others block. We treat "exited before timeout" as success.
func commandCompletedWithin(cmd *exec.Cmd, d time.Duration) bool {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err == nil
	case <-time.After(d):
		// The command is still running (e.g. it opened a terminal browser).
		// Treat this as success — the browser is likely open.
		return true
	}
}
