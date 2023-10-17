package o11y

import (
	"sync"
	"time"
)

type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
	NewTicker(time.Duration) Ticker
}

func NewRealClock() Clock {
	return &RealClock{}
}

type RealClock struct{}

func (r *RealClock) Now() time.Time {
	return time.Now()
}

func (r *RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

type RealTicker struct {
	tkr *time.Ticker
}

func (r *RealClock) NewTicker(d time.Duration) Ticker {
	return RealTicker{time.NewTicker(d)}
}

func (t RealTicker) Chan() <-chan time.Time {
	return t.tkr.C
}

func (t RealTicker) Stop() {
	t.tkr.Stop()
}

type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*fakeTicker
}

// returns the clock, and a function that advances the clock
func NewFakeClock(now string) (Clock, func(time.Duration)) {
	tm, err := time.Parse(time.RFC3339, now)
	if err != nil {
		panic(err)
	}
	ret := &FakeClock{
		now: tm,
	}
	return ret, ret.Sleep
}

func (f *FakeClock) Now() time.Time {
	// support polling for time to advance
	f.Sleep(time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *FakeClock) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sleepUntil := f.now.Add(d)
	for {
		var earliest *fakeTicker
		for _, t := range f.tickers {
			if earliest == nil || t.next.Before(earliest.next) {
				earliest = t
			}
		}
		if earliest == nil || earliest.next.After(sleepUntil) {
			f.now = sleepUntil
			return
		}
		f.now = earliest.next
		earliest.next = earliest.next.Add(earliest.d)
		n := f.now
		f.mu.Unlock()
		select {
		case earliest.c <- n:
			// might be blocking, so default out
		default:
		}
		f.mu.Lock()
	}
}

func (f *FakeClock) NewTicker(d time.Duration) Ticker {
	if 0 == d {
		panic("zero interval ticker")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ret := &fakeTicker{
		d:    d,
		c:    make(chan time.Time, 1),
		f:    f,
		next: f.now.Add(d),
	}
	f.tickers = append(f.tickers, ret)
	return ret
}

type fakeTicker struct {
	d    time.Duration
	c    chan time.Time
	f    *FakeClock
	next time.Time
}

func (t *fakeTicker) Chan() <-chan time.Time {
	return t.c
}

func (t *fakeTicker) Stop() {
	// you will crash if you stop it twice!
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	for i, z := range t.f.tickers {
		if z == t {
			copy(t.f.tickers[i:], t.f.tickers[i+1:])
			t.f.tickers = t.f.tickers[:len(t.f.tickers)-1]
			break
		}
	}
	t.f = nil
	// drain any tick
	select {
	case <-t.c:
	default:
	}
}
