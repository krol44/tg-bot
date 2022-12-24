package main

import (
	"sync"
)

type ChatsWork struct {
	m           sync.Map
	chat        sync.Map
	LenMu       sync.Mutex
	LockIncPlus sync.Mutex
}

func (c *ChatsWork) IncPlus(messId int, chatId int64) {
	c.LockIncPlus.Lock()
	defer c.LockIncPlus.Unlock()
	c.m.LoadOrStore(messId, c.Len())

	c.chat.Store(chatId, true)
}

func (c *ChatsWork) IncMinus(messId int, chatId int64) {
	c.LockIncPlus.Lock()
	defer c.LockIncPlus.Unlock()

	c.chat.Delete(chatId)

	_, b := c.m.Load(messId)
	if !b {
		return
	}

	c.m.Delete(messId)

	c.m.Range(func(messId, qq any) bool {
		if qq != 0 {
			c.m.Store(messId, qq.(int)-1)
		}
		return true
	})
}

func (c *ChatsWork) Len() int {
	c.LenMu.Lock()
	defer c.LenMu.Unlock()
	var lq int
	c.m.Range(func(_, _ any) bool {
		lq++
		return true
	})
	return lq
}
