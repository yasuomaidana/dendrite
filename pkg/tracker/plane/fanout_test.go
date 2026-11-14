package plane

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFanOutRunsEveryIndexOnceWithBoundedConcurrency(t *testing.T) {
	const n, workers = 40, 5
	var inFlight, peak int32
	seen := make([]int32, n)
	fanOut(n, workers, func(i int) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&seen[i], 1)
		atomic.AddInt32(&inFlight, -1)
	})
	for i, c := range seen {
		if c != 1 {
			t.Errorf("index %d ran %d times", i, c)
		}
	}
	if peak < 2 || peak > workers {
		t.Errorf("peak concurrency %d, want between 2 and %d", peak, workers)
	}
	fanOut(0, 4, func(int) { t.Error("ran for n=0") })
	var ran atomic.Int32
	fanOut(3, 100, func(int) { ran.Add(1) }) // more workers than work
	if ran.Load() != 3 {
		t.Errorf("ran %d times, want 3", ran.Load())
	}
}
