package o11y

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestClientHappyPath(t *testing.T) {
	ctx, can := context.WithCancel(context.Background())
	defer can()
	svr := newTestServer(t)
	fc, _ := NewFakeClock("2023-04-20T23:20:00Z")
	cli, err := NewClient(ctx, WithUrl(svr.url()+"/v1/http/test-path"), WithAuthtoken("ds1deepspace:onetokengoeshere"), WithClock(fc), WithIdentifier("test-client:3"))
	if err != nil {
		t.Fatal(err)
	}
	if len(svr.data()) != 1 {
		t.Fatal("expected 1 request for connection test", len(svr.data()))
	}
	if svr.data()[0].URL.Path != "/v1/http/test-path" {
		t.Fatal("unexpected path", svr.data()[0].URL.Path)
	}
	err = cli.SendJSON(1, map[string]any{"key": "value"})
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	for len(svr.data()) < 2 {
		fc.Sleep(time.Second)
	}
	data, err := io.ReadAll(svr.data()[1].Body)
	if err != nil {
		t.Fatal(err)
	}
	var dat map[string]any
	err = json.Unmarshal(data, &dat)
	if err != nil {
		t.Fatal(err)
	}
	if dat["identifier"].(string) != "test-client:3" {
		t.Fatal("bad identifier", dat)
	}
}

type testSvr struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func (t *testSvr) url() string {
	return t.server.URL
}

func (t *testSvr) data() []*http.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := make([]*http.Request, len(t.requests))
	copy(r, t.requests)
	return r
}

func (t *testSvr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stderr, "%s %s %s\n", r.Method, r.Host, r.URL.String())
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = append(t.requests, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

func newTestServer(t *testing.T) *testSvr {
	ret := &testSvr{}
	ret.server = httptest.NewServer(ret)
	t.Cleanup(func() { ret.server.Close() })
	return ret
}
