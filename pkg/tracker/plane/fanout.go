package plane

import "sync"

// pullWorkers bounds the independent requests a status pull has in flight at once.
const pullWorkers = 8

// fanOut calls fn(i) for every i in [0, n) on at most workers goroutines and returns when all are
// done. Each call must write only to its own slot of any shared result.
func fanOut(n, workers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if workers > n {
		workers = n
	}
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
}
