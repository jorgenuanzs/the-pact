//go:build !windows

package lifecycle

import (
	"os"
	"syscall"
)

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func interruptProcess(process *os.Process) error {
	return process.Signal(os.Interrupt)
}
