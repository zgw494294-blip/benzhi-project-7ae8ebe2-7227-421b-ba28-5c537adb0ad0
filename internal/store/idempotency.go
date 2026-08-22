package store

import (
	"fmt"
	"sync"
)

type Idempotency struct {
	mu     sync.Mutex
	values map[string]string
}

func NewIdempotency() *Idempotency { return &Idempotency{values: map[string]string{}} }
func (i *Idempotency) Get(key string) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	v, ok := i.values[key]
	return v, ok
}
func (i *Idempotency) Put(key, val string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if key == "" {
		return fmt.Errorf("幂等键不能为空")
	}
	if old, ok := i.values[key]; ok && old != val {
		return fmt.Errorf("幂等键已用于其他请求")
	}
	i.values[key] = val
	return nil
}
