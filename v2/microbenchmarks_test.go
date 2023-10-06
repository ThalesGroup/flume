package flume

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func BenchmarkAtomicInt(b *testing.B) {
	var ai atomic.Int64
	ai.Store(100)

	var k int
	defer func() {
		fmt.Print(k)
	}()

	for i := 0; i < b.N; i++ {
		j := ai.Load()
		k = int(j) + i
	}
}

func BenchmarkAtomicPtr(b *testing.B) {
	var ai atomic.Pointer[int64]
	var s int64 = 100
	ai.Store(&s)

	var k int
	defer func() {
		fmt.Print(k)
	}()

	for i := 0; i < b.N; i++ {
		j := ai.Load()
		k = int(*j) + i
	}
}
