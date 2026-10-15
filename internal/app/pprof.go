//go:build pprof

package app

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // development builds only, loopback only
	"os"
	"time"
)

// A profiling build (-tags pprof) serves Go's profiler on a free loopback
// port and prints where, so the running launcher can be profiled:
//
//	go tool pprof http://127.0.0.1:<port>/debug/pprof/profile
//
// Release builds never carry this file (tools/release verify refuses it).
func init() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "pprof:", err)
		return
	}
	fmt.Fprintf(os.Stderr, "pprof: http://%s/debug/pprof/\n", l.Addr())
	srv := &http.Server{Handler: http.DefaultServeMux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(l) }()
}
