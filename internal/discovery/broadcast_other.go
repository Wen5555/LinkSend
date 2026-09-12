//go:build !windows

package discovery

import (
	"errors"
	"net"
	"syscall"
)

func enableBroadcast(conn net.PacketConn) error {
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		return errors.New("LAN discovery requires UDP")
	}
	raw, err := udp.SyscallConn()
	if err != nil {
		return err
	}
	var optionErr error
	if err = raw.Control(func(fd uintptr) {
		optionErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		return err
	}
	return optionErr
}
