//go:build !windows

package rclone

import "os"

func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }
