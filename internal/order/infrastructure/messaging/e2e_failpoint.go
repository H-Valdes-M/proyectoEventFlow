//go:build e2e

package messaging

import "eventflow/pkg/pubsub"

// SetTestBeforeProcess instala un fallo exclusivo del harness e2e.
func SetTestBeforeProcess(hook func(pubsub.Delivery) error) func() {
	previous := testBeforeProcess
	testBeforeProcess = hook
	return func() { testBeforeProcess = previous }
}
