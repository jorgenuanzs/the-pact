package lifecycle

import (
	"context"
	"os"
	"os/signal"
)

// NotifyContext returns a context cancelled by the termination signals
// supported by the current operating system.
func NotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, terminationSignals()...)
}

// InterruptProcess asks a child process to stop gracefully when the platform
// supports it. On Windows, where os.Interrupt cannot be sent reliably to an
// arbitrary child process, the platform implementation terminates the child.
func InterruptProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	return interruptProcess(process)
}
