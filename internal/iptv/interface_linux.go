//go:build linux

package iptv

import (
	"net"
	"syscall"
)

func interfaceDialer(name string, ip net.IP) (*net.Dialer, error) {
	control := func(_, _ string, conn syscall.RawConn) error {
		var bindErr error
		if err := conn.Control(func(fd uintptr) {
			bindErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, name)
		}); err != nil {
			return err
		}
		return bindErr
	}

	resolverDialer := &net.Dialer{Control: control}
	return &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: ip},
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial:     resolverDialer.DialContext,
		},
		Control: control,
	}, nil
}
