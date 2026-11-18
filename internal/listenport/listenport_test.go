package listenport

import (
	"net"
	"strings"
	"testing"
)

// freePort opens ":0" and returns the assigned port with the listener kept
// open so the port stays occupied by the caller.
func freePort(t *testing.T) (int, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	return ln.Addr().(*net.TCPAddr).Port, ln
}

func TestListenNoDriftWhenFree(t *testing.T) {
	port, blocker := freePort(t)
	blocker.Close() // give it back, we just wanted a free number

	res, err := Listen(joinAddr("127.0.0.1", port), true)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer res.Listener.Close()
	if res.Drifted {
		t.Fatalf("free port should not drift, got %s", res.Addr)
	}
	if res.Addr != joinAddr("127.0.0.1", port) {
		t.Fatalf("addr = %s, want %s", res.Addr, joinAddr("127.0.0.1", port))
	}
}

func TestListenDriftsWhenOccupied(t *testing.T) {
	port, blocker := freePort(t)
	defer blocker.Close()

	res, err := Listen(joinAddr("127.0.0.1", port), true)
	if err != nil {
		t.Fatalf("listen with drift: %v", err)
	}
	defer res.Listener.Close()
	if !res.Drifted {
		t.Fatalf("expected drift from occupied port %d, bound %s", port, res.Addr)
	}
	if res.Attempts == 0 {
		t.Fatal("attempts should be > 0 after drift")
	}
	boundPort := res.Listener.Addr().(*net.TCPAddr).Port
	if boundPort == port {
		t.Fatalf("bound the occupied port %d", port)
	}
	// The drift stays on the same host.
	if !strings.HasPrefix(res.Addr, "127.0.0.1:") {
		t.Fatalf("host changed during drift: %s", res.Addr)
	}
}

func TestListenDisabledDriftFails(t *testing.T) {
	port, blocker := freePort(t)
	defer blocker.Close()

	_, err := Listen(joinAddr("127.0.0.1", port), false)
	if err == nil {
		t.Fatal("expected error with drift disabled")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListenZeroPortNeverDrifts(t *testing.T) {
	res, err := Listen("127.0.0.1:0", true)
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer res.Listener.Close()
	if res.Drifted {
		t.Fatal(":0 must not drift")
	}
	if res.Listener.Addr().(*net.TCPAddr).Port == 0 {
		t.Fatal("kernel did not assign a port")
	}
}

func TestListenExhaustedWindow(t *testing.T) {
	// Occupy a full drift window so every candidate is taken. Retry the
	// reservation when the base lands too close to the port ceiling.
	var blockers []net.Listener
	defer func() {
		for _, ln := range blockers {
			ln.Close()
		}
	}()
	var first int
	var blocker net.Listener
	for range 8 {
		p, ln := freePort(t)
		if p+MaxAttempts <= 65535 {
			first, blocker = p, ln
			blockers = append(blockers, ln)
			break
		}
		ln.Close()
	}
	if blocker == nil {
		t.Skip("无法预留远离端口上限的连续窗口")
	}
	for i := 1; i < MaxAttempts; i++ {
		ln, err := net.Listen("tcp", joinAddr("127.0.0.1", first+i))
		if err != nil {
			t.Fatalf("reserve port %d: %v", first+i, err)
		}
		blockers = append(blockers, ln)
	}

	_, err := Listen(joinAddr("127.0.0.1", first), true)
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if !strings.Contains(err.Error(), "均被占用") {
		t.Fatalf("unexpected exhaustion error: %v", err)
	}
}

func TestSplitAddrForms(t *testing.T) {
	cases := []struct {
		in        string
		host      string
		port      int
		expectErr bool
	}{
		{":8080", "", 8080, false},
		{"127.0.0.1:8080", "127.0.0.1", 8080, false},
		{"localhost:9000", "localhost", 9000, false},
		{"[::1]:8080", "[::1]", 8080, false},
		{"::8080", "[::]", 8080, false},
		{"nonsense", "", 0, true},
		{"host:abc", "", 0, true},
	}
	for _, tc := range cases {
		host, port, err := splitAddr(tc.in)
		if tc.expectErr {
			if err == nil {
				t.Fatalf("splitAddr(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("splitAddr(%q): %v", tc.in, err)
		}
		if host != tc.host || port != tc.port {
			t.Fatalf("splitAddr(%q) = %q,%d; want %q,%d", tc.in, host, port, tc.host, tc.port)
		}
	}
}
