//go:build e2e

package pubsub

// SetTestBeforeAck instala una barrera exclusiva del harness e2e.
func SetTestBeforeAck(hook func() error) func() {
	previous := testBeforeAck
	testBeforeAck = hook
	return func() { testBeforeAck = previous }
}
