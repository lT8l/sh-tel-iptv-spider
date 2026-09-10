//go:build !linux

package iptv

import (
	"fmt"
	"net"
)

func interfaceDialer(name string, _ net.IP) (*net.Dialer, error) {
	return nil, fmt.Errorf("binding all network traffic to interface %q requires Linux", name)
}
