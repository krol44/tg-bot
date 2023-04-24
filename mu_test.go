package main

import (
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestMu(t *testing.T) {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	cw := ChatsWork{m: sync.Map{}}
	var sw sync.WaitGroup
	for c := 1; c <= 1000; c++ {
		go func(c int) {
			sw.Add(1)

			time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)))
			cw.IncPlus(c, int64(c))

			time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)))
			cw.IncMinus(c, int64(c))

			sw.Done()
		}(c)
	}

	cw.IncMinus(7, 7)

	sw.Wait()

	cw.m.Range(func(key, value any) bool {
		log.Println(key, value)
		return true
	})

	if cw.Len() != 0 {
		t.Errorf("error mu - %d", cw.Len())
	}
}
