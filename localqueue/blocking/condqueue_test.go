package blocking

import (
	"fmt"
	"sync"
	"testing"
)

func TestCondQueue(t *testing.T) {
	var queue = NewCondQueue[int](5)
	var wg sync.WaitGroup
	wg.Add(20)
	//time.Sleep(time.Second * 3)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			r := queue.Consume()
			fmt.Println(r)
		}()
	}

	for i := 0; i < 10; i++ {
		go func(a int) {
			defer wg.Done()
			queue.Produce(a)
		}(i)
	}

	wg.Wait()
	//time.Sleep(time.Second * 5)
}
