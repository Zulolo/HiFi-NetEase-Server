//go:build !linux

package remote

import "context"

func (m *Manager) runInputs(ctx context.Context) { <-ctx.Done() }
