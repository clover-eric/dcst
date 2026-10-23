package task

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadHandlerUsesIndependentParallelStreams(t *testing.T) {
	const streamCount = 4
	payload := bytes.Repeat([]byte{0x5a}, 8<<20)
	var active atomic.Int32
	var maxActive atomic.Int32
	var releaseOnce sync.Once
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maxActive.Load()
			if current <= observed || maxActive.CompareAndSwap(observed, current) {
				break
			}
		}
		if current == streamCount {
			releaseOnce.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	oldURL, oldTimeout, oldWarmup := URL, Timeout, Warmup
	oldStreams, oldPort := Streams, TCPPort
	t.Cleanup(func() {
		URL, Timeout, Warmup = oldURL, oldTimeout, oldWarmup
		Streams, TCPPort = oldStreams, oldPort
	})
	URL = server.URL
	Timeout = 3 * time.Second
	Warmup = 0
	Streams = streamCount
	TCPPort = port

	speed, _ := downloadHandler(&net.IPAddr{IP: net.ParseIP("127.0.0.1")})
	if speed <= 0 {
		t.Fatalf("expected positive throughput, got %f", speed)
	}
	t.Logf("loopback aggregate throughput: %.2f MiB/s", speed/(1<<20))
	if got := maxActive.Load(); got < 2 {
		t.Fatalf("expected parallel requests, max active streams = %d", got)
	}
}

func TestCheckDownloadDefaultClampsStreams(t *testing.T) {
	oldStreams, oldWarmup, oldTimeout := Streams, Warmup, Timeout
	t.Cleanup(func() {
		Streams, Warmup, Timeout = oldStreams, oldWarmup, oldTimeout
	})

	Streams = maxStreams + 1
	Timeout = 2 * time.Second
	Warmup = 3 * time.Second
	checkDownloadDefault()
	if Streams != maxStreams {
		t.Fatalf("Streams = %d, want %d", Streams, maxStreams)
	}
	if Warmup >= Timeout {
		t.Fatalf("Warmup = %v must be shorter than Timeout = %v", Warmup, Timeout)
	}
}
