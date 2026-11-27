// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-FileCopyrightText: 2026 ItsWempo

package gsm

import (
	"net"
	"path/filepath"
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
func TestLogPathRejectsInvalidServerIDs(t *testing.T) {
	dir := t.TempDir()
	launcher := NewProcessLauncherWithOptions(ProcessOptions{LogsDir: dir})
	if got := launcher.LogPath("../../outside"); got != "" {
		t.Fatalf("invalid server id produced path %q", got)
	}
	if _, _, err := launcher.logFile("../../outside"); err == nil {
		t.Fatal("invalid server id opened a log file")
	}
	want := filepath.Join(dir, "gs-abcdefgh.log")
	if got := launcher.LogPath("abcdefgh"); got != want {
		t.Fatalf("valid server id path = %q, want %q", got, want)
	}
}
