//go:build !linux

package api

func diskUsage(path string) (total, free uint64, err error) { return 0, 0, nil }
