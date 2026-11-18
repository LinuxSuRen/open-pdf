// Package listenport resolves a TCP listener for a configured address,
// optionally drifting to subsequent ports when the address is occupied.
package listenport

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// MaxAttempts is the number of consecutive ports tried when drifting.
const MaxAttempts = 20

// Result describes a successful listener acquisition.
type Result struct {
	// Listener is the acquired TCP listener (owned by the caller).
	Listener net.Listener
	// Addr is the address that was finally bound (host:port form).
	Addr string
	// Drifted is true when the configured address was occupied and a
	// different port was chosen.
	Drifted bool
	// Attempts counts the occupied ports skipped (0 when no drift).
	Attempts int
}

// Listen acquires a listener for addr. When drift is true and the port is
// occupied, up to MaxAttempts-1 subsequent ports are tried; the host part
// stays unchanged. Port :0 and non-occupancy errors never drift.
func Listen(addr string, drift bool) (Result, error) {
	return ListenMax(addr, drift, MaxAttempts)
}

// ListenMax is Listen with a custom drift window (ports tried = max).
func ListenMax(addr string, drift bool, max int) (Result, error) {
	host, port, err := splitAddr(addr)
	if err != nil || port == 0 {
		// Unparsable address or kernel-assigned port: single plain attempt.
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			return Result{}, lerr
		}
		return Result{Listener: ln, Addr: ln.Addr().String()}, nil
	}

	for i := 0; i < max; i++ {
		candidate := joinAddr(host, port+i)
		ln, lerr := net.Listen("tcp", candidate)
		if lerr == nil {
			return Result{
				Listener: ln,
				Addr:     candidate,
				Drifted:  i > 0,
				Attempts: i,
			}, nil
		}
		if !drift || !isAddrInUse(lerr) {
			return Result{}, fmt.Errorf("listen %s: %w", candidate, lerr)
		}
	}
	return Result{}, fmt.Errorf("端口 %d 起连续 %d 个端口均被占用，无法监听（可用 OPEN_PDF_PORT_DRIFT=false 关闭漂移以查看具体错误）", port, max)
}

// splitAddr parses "host:port" (host may be empty for all interfaces).
// Bare IPv6 hosts are normalized to bracketed form.
func splitAddr(addr string) (host string, port int, err error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", 0, fmt.Errorf("地址缺少端口: %q", addr)
	}
	host = addr[:idx]
	port, err = strconv.Atoi(addr[idx+1:])
	if err != nil {
		return "", 0, fmt.Errorf("端口不是数字: %q", addr)
	}
	if host == ":" {
		// Input was "::port": the IPv6 any-address without brackets.
		host = "[::]"
	}
	if host != "" && strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return host, port, nil
}

func joinAddr(host string, port int) string {
	if host != "" && strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		// Bare IPv6 without brackets: net.Listen wants [::1]:8080 form.
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// isAddrInUse reports whether the error means the port is already taken.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	// String fallback covers platform differences (e.g. Windows WSA wording).
	return strings.Contains(err.Error(), "address already in use")
}
