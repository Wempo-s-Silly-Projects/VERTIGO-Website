// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-FileCopyrightText: 2026 ItsWempo

package main

import (
	"net"
	"testing"
)

func TestHasServerIDArg(t *testing.T) {
	cmdline := []byte("/opt/vertexia/bin/RaveEngineServer\x00--port\x005100\x00--server-id\x00server-abc\x00")
	if !hasServerIDArg(cmdline, "server-abc") {
		t.Fatal("expected exact server ID argument to match")
	}
	if hasServerIDArg(cmdline, "server-ab") {
		t.Fatal("server ID matching must not accept a prefix")
	}
	if hasServerIDArg([]byte("--server-id server-abc"), "server-abc") {
		t.Fatal("arguments must be NUL-delimited as they are in /proc/PID/cmdline")
	}
}

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
