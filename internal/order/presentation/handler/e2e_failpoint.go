//go:build e2e

package handler

import "context"

// SetTestAfterCreateCommit instala una barrera exclusiva del harness e2e.
func (h *OrderHandler) SetTestAfterCreateCommit(hook func(context.Context)) {
	h.afterCreateCommit = hook
}
