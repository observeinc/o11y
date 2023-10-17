package o11y

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Create a new observability client. It runs until the context is canceled, or
// you call Close() on it. Close() will synchronously wait until it's actually
// closed, and is safe to call multiple times or if the context has already
// been cancelled.
func NewClient(ctx context.Context, opts ...ClientOpt) (Client, error) {
	buf := bytes.NewBuffer(nil)
	ret := &client{
		authtoken:    os.Getenv("OBSERVE_AUTHTOKEN"),
		url:          os.Getenv("OBSERVE_URL"),
		identifier:   os.Args[0],
		httpClient:   http.DefaultClient,
		verbosity:    5,
		interval:     5 * time.Second,
		maxSize:      1024 * 1024,
		clock:        NewRealClock(),
		checkConnect: true,
		nRetries:     5,
		buffer:       buf,
		queue:        json.NewEncoder(buf),
		warnedType:   map[string]struct{}{},
		boop:         make(chan any, 1024),
		done:         make(chan struct{}),
		wg:           &sync.WaitGroup{},
		allowWrite:   true,
		busy:         false,
	}
	for _, opt := range opts {
		opt(ret)
	}
	if err := ret.fixPath(); err != nil {
		return nil, err
	}
	if ret.checkConnect {
		if err := ret.checkConnection(ctx); err != nil {
			return nil, err
		}
	}
	ret.wg.Add(1)
	go ret.run(ctx, ret.done)
	return ret, nil
}

type Client interface {
	// You can Close() the client more than once if you want. It will wait for
	// the client to complete.
	io.Closer
	// Call SendJSON to send some data. The data must be JSON marshal-able.
	// The "v" value is checked against the verbosity; the default is 5; if the
	// "v" value is 5 or lower, the data will actually be forwarded. You can
	// change this with the WithVerbosity() option when creating the Client.
	// Just pass 0 if you don't know what to pass. If the client has been
	// closed, or the client is falling behind and is unable to keep up with
	// the rate of messages, an error is returned, else the data are enqueued
	// and nil is returned.
	SendJSON(v int, data any) error
}

// Various options, typically spelled "WithXxxx", can be passed to NewClient().
type ClientOpt func(*client)

// If you don't specify an authtoken, one will be read from OBSERVE_AUTHTOKEN
func WithAuthtoken(authtoken string) ClientOpt {
	return func(c *client) {
		c.authtoken = authtoken
	}
}

// If you don't specify a URL, one will be read from OBSERVE_URL.
// A typical value is https://123456789012.collect.observeinc.com/v1/http/some/tag
func WithUrl(url string) ClientOpt {
	return func(c *client) {
		c.url = url
	}
}

// The identifier is added to each observation sent from this client. Make it
// good!
func WithIdentifier(identifier string) ClientOpt {
	return func(c *client) {
		c.identifier = identifier
	}
}

// You can pass in your own http.Client. This can be helpful if you need to
// proxy, or if you want particular API timeouts, or if you want to mock the
// Transport for unit tests.
func WithHttpClient(hc *http.Client) ClientOpt {
	return func(c *client) {
		c.httpClient = hc
	}
}

func WithVerbosity(v int) ClientOpt {
	return func(c *client) {
		c.verbosity = v
	}
}

// You can call NewRealClock() (which is the default) or NewFakeClock() to make
// clocks for the client. This is mainly helpful for unit testing.
func WithClock(clock Clock) ClientOpt {
	return func(c *client) {
		c.clock = clock
	}
}

// How frequently are observations pushed into the collector? Default is every
// 5 seconds. Longer times may be more efficient, but risks more loss if your
// process dies, and adds a little observational latency.
func WithInterval(i time.Duration) ClientOpt {
	return func(c *client) {
		c.interval = i
	}
}

// How much data do we collect in RAM before flushing it out to the collector
// even if the timer hasn't yet expired? Default is a megabyte, which is plenty
// for most use cases. Note that there is a maximum single observation limit of
// four megabytes.
func WithMaxSize(sz int) ClientOpt {
	if sz < MinMaxSize {
		sz = MinMaxSize
	}
	if sz > MaxMaxSize {
		sz = MaxMaxSize
	}
	return func(c *client) {
		c.maxSize = sz
	}
}

// How many times do we re-try if the first attempt to send the observation
// fails for whatever reason? (Typically, network connectivity.) Re-tries use
// exponantial back-off sleeping, but there is back pressure against collecting
// too many observations while there's an outstanding HTTP request.
func WithNumRetries(n int) ClientOpt {
	if n > MaxNumRetries {
		n = MaxNumRetries
	}
	if n < 1 {
		n = 1
	}
	return func(c *client) {
		c.nRetries = n
	}
}

// Turn off the initial connectivity check. That check is helpful to detect bad
// configurations, and is also a helpful marker in the data stream, but you
// might not want it for whatever reason.
func WithoutCheckConnect() ClientOpt {
	return func(c *client) {
		c.checkConnect = false
	}
}

const SizeMargin = 128
const MinMaxSize = 4096
const MaxMaxSize = 4096 * 1000
const MaxNumRetries = 10
const MaxSleepRetries = 6
const SleepQuantaMs = 250

type client struct {
	authtoken    string
	url          string
	identifier   string
	httpClient   *http.Client
	verbosity    int
	interval     time.Duration
	maxSize      int
	checkConnect bool
	nRetries     int
	clock        Clock
	mu           sync.Mutex
	buffer       *bytes.Buffer
	queue        *json.Encoder
	warnedType   map[string]struct{}
	boop         chan any
	done         chan struct{}
	wg           *sync.WaitGroup
	allowWrite   bool
	busy         bool
}

func (c *client) checkConnection(ctx context.Context) error {
	data, err := json.Marshal(ObservationHeader{Timestamp: c.clock.Now().UnixNano(), Identifier: c.identifier, Action: "connect"})
	if err != nil {
		return err
	}
	return c.sendPayload(data)
}

func (c *client) SendJSON(v int, data any) error {
	if v > c.verbosity {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.allowWrite {
		return ErrClientClosed
	}
	select {
	case c.boop <- data:
		// all OK
		c.busy = false
	default:
		// chan is full -- we're not keeping up!
		if !c.busy {
			c.busy = true
			fmt.Fprintf(os.Stderr, "%s: o11y client is backlogged; dropping observations\n", c.clock.Now())
		}
		return ErrClientBacklogged
	}
	return nil
}

func (c *client) Close() error {
	c.mu.Lock()
	if c.done != nil {
		cl := c.done
		c.done = nil
		close(cl)
	}
	c.mu.Unlock()
	c.wg.Wait()
	return nil
}

func (c *client) fixPath() error {
	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(u.Path, "/v1/http/") {
		if u.Path == "/" {
			u.Path = "/v1/http/go-o11y"
		} else {
			u.Path = "/v1/http" + u.Path
		}
		c.url = u.String()
	}
	return nil
}

func (c *client) run(ctx context.Context, done chan struct{}) {
	defer c.wg.Done()
	tkr := c.clock.NewTicker(c.interval)
	defer tkr.Stop()
	running := true
	for running {
		select {
		case <-ctx.Done():
			c.preClose()
		case <-done:
			c.preClose()
			done = nil
		case arg, ok := <-c.boop:
			if ok {
				c.writeOne(arg)
			} else {
				c.queue.Encode(&ObservationHeader{Timestamp: c.clock.Now().UnixNano(), Identifier: c.identifier, Action: "closing"})
				c.flushBuffer()
				running = false
			}
		case <-tkr.Chan():
			c.flushBuffer()
		}
	}
}

func (c *client) preClose() {
	c.mu.Lock()
	if c.allowWrite {
		c.allowWrite = false
		close(c.boop)
	}
	c.mu.Unlock()
}

func (c *client) writeOne(arg any) {
	oh := ObservationHeader{
		Timestamp:  c.clock.Now().UnixNano(),
		Identifier: c.identifier,
		Data:       arg,
		Action:     "data",
	}
	if err := c.queue.Encode(&oh); err != nil {
		c.warnError(&oh, err)
	}
	if c.buffer.Len() >= c.maxSize-SizeMargin {
		c.flushBuffer()
	}
}

func (c *client) warnError(oh *ObservationHeader, err error) {
	// that's too bad
	t := fmt.Sprintf("%T", oh.Data)
	if _, has := c.warnedType[t]; !has {
		c.warnedType[t] = struct{}{}
		fmt.Fprintf(os.Stderr, "%s: o11y client: type %s is not JSON marshalable: %s\n", c.clock.Now(), t, err)
		oh.Data = BadType{Typename: t}
		// this won't fail, I'm sure...
		c.queue.Encode(&oh)
	}
}

func (c *client) flushBuffer() {
	data := c.buffer.Bytes()
	defer c.buffer.Reset()
	for len(data) > 0 {
		var rest []byte
		if len(data) > c.maxSize {
			data, rest = cutPoint(data, c.maxSize)
		}
		// If we end up backing up here, eventually the SendJSON() call
		// will start returning "client busy" errors.
		if err := c.sendPayload(data); err != nil {
			fmt.Fprintf(os.Stderr, "%s: o11y client failed to send: %s\n", c.clock.Now(), err)
		}
		data = rest
	}
}

func (c *client) sendPayload(data []byte) error {
	var err error
	var resp *http.Response
	// I re-try some number of times, with exponential back-off sleep in
	// between iterations. However, I don't sleep for too long, because
	// if it backs up too much, I will need to start shedding observations,
	// and I don't want to buffer old observations forever in RAM.
	for i := 0; i < c.nRetries; i++ {
		if i > 0 {
			w := i
			// limit how much I sleep per iteration if doing many retries
			if w > MaxSleepRetries {
				w = MaxSleepRetries
			}
			c.clock.Sleep(SleepQuantaMs * time.Millisecond * time.Duration(1<<w))
		}
		req, err := http.NewRequest("POST", c.url, bytes.NewBuffer(data))
		if err != nil {
			// this should essentially never happen
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authtoken))
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.Header.Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
		resp, err = c.httpClient.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		var ok CheckResponse
		if err = jsonDecode(resp.Body, &ok); err != nil {
			// maybe we got HTML from nginx or something
			continue
		}
		if !ok.Ok {
			// If it decoded OK, but say "false" then I shouldn't re-try,
			// because it's likely credentials are bad.
			return ErrSendFailed
		}
		return nil
	}
	// return the error we ended up with
	return err
}
