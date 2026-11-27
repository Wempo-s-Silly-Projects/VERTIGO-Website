// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-FileCopyrightText: 2026 ItsWempo

package gsm

import (
	"net"
	"testing"
)

func TestUDPPortBound(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if !udpPortBound(port) {
		t.Fatal("expected occupied UDP port to be detected")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if udpPortBound(port) {
		t.Fatal("expected released UDP port to be available")
	}
}
