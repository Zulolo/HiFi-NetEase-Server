//go:build !linux

package netease

func freeBytes(string) (uint64, bool) { return 0, false }
