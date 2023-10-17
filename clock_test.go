package o11y

import (
	"testing"
	"time"
)

func TestClockReal(t *testing.T) {
	rc := NewRealClock()
	start := rc.Now()
	tkr := rc.NewTicker(10 * time.Millisecond)
	rc.Sleep(20 * time.Millisecond)
	<-tkr.Chan()
	end := rc.Now()
	if end.Sub(start) < 20*time.Millisecond {
		t.Fatal("bad times", start, end)
	}
}

func TestClockFake(t *testing.T) {
	rc, _ := NewFakeClock("2023-04-20T23:20:00Z")
	start := rc.Now()
	tkr := rc.NewTicker(10 * time.Millisecond)
	rc.Sleep(20 * time.Millisecond)
	<-tkr.Chan()
	end := rc.Now()
	if end.Sub(start) < 20*time.Millisecond {
		t.Fatal("bad times", start, end)
	}
}
