//go:build windows

package harnesscommand

import (
	"errors"
	"net"
)

// ErrUnsupportedPlatform is returned by Listen on any platform without a
// protected local-transport implementation. There is no fallback TCP
// listener: the transport fails closed rather than exposing a network port.
var ErrUnsupportedPlatform = errors.New("harnesscommand: protected local transport is not implemented on this platform")

func Listen(runtimeDir string) (net.Listener, string, error) {
	return nil, "", ErrUnsupportedPlatform
}
