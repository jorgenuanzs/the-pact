//go:build windows

package lifecycle

import "os"

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func interruptProcess(process *os.Process) error {
	return process.Kill()
}
