package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

func Env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func Duration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	result, err := time.ParseDuration(value)
	if err != nil || result <= 0 || result > 10*time.Second {
		return 0, fmt.Errorf("%s must be a duration in (0,10s]", key)
	}
	return result, nil
}

type State struct {
	draining  atomic.Bool
	DrainFile string
}

// EmptyDir survives container restarts; a prior preStop marker must not poison readiness.
func NewState(path string) (*State, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return &State{DrainFile: path}, nil
}

func (s *State) Draining() bool {
	if s.draining.Load() {
		return true
	}
	_, err := os.Stat(s.DrainFile)
	return err == nil
}
func (s *State) Drain() { s.draining.Store(true) }
func (s *State) Live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"alive"}`)
}
func (s *State) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.Draining() {
		w.WriteHeader(503)
		fmt.Fprintln(w, `{"status":"draining"}`)
		return
	}
	fmt.Fprintln(w, `{"status":"ready"}`)
}

func HTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}

// Kubernetes executes the same static binary for preStop, requiring no shell/Python.
func Command(defaultHTTPAddr, defaultDrainFile string) (bool, error) {
	if len(os.Args) < 2 {
		return false, nil
	}
	switch os.Args[1] {
	case "drain":
		if err := os.WriteFile(Env("DRAIN_FILE", defaultDrainFile), []byte("draining"), 0600); err != nil {
			return true, err
		}
		time.Sleep(10 * time.Second)
		return true, nil
	case "check":
		_, port, err := net.SplitHostPort(Env("HTTP_ADDR", defaultHTTPAddr))
		if err != nil {
			return true, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/readyz", nil)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return true, err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return true, fmt.Errorf("not ready: %d", response.StatusCode)
		}
		return true, nil
	default:
		return true, fmt.Errorf("unknown command: %s", os.Args[1])
	}
}
