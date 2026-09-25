//go:build !linux

package sysinfo

import "time"

type Stats struct {
	SampledAt time.Time `json:"sampled_at"`
}

func Sample(iface string) Stats { return Stats{SampledAt: time.Now()} }
