package omap

import "sync"

type OrderedMap[T comparable, V any] struct {
	initOnce sync.Once
	lock     *sync.RWMutex
	m        map[T]V
	keyChain []T
}

func (o *OrderedMap[T, V]) init() {
	if o == nil {
		return
	}
	o.initOnce.Do(func() {
		if o.lock == nil {
			o.lock = new(sync.RWMutex)
		}
		if o.m == nil {
			o.m = make(map[T]V)
		}
		if o.keyChain == nil {
			o.keyChain = make([]T, 0)
		}
	})
}

func NewEmptyOrderedMap[T comparable, V any]() *OrderedMap[T, V] {
	m := &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        make(map[T]V),
		keyChain: make([]T, 0),
	}
	m.initOnce.Do(func() {})
	return m
}

func (o *OrderedMap[T, V]) Set(key T, v V) {
	if o == nil {
		return
	}
	o.init()
	o.lock.Lock()
	if _, ok := o.m[key]; !ok {
		o.keyChain = append(o.keyChain, key)
	}
	o.m[key] = v
	o.lock.Unlock()
}

func (o *OrderedMap[T, V]) Get(key T) (V, bool) {
	if o == nil {
		var zero V
		return zero, false
	}
	o.init()
	o.lock.RLock()
	v, ok := o.m[key]
	o.lock.RUnlock()
	return v, ok
}

func (o *OrderedMap[T, V]) GetMust(key T) V {
	if o == nil {
		var zero V
		return zero
	}
	o.init()
	o.lock.RLock()
	v := o.m[key]
	o.lock.RUnlock()
	return v
}

func (o *OrderedMap[T, V]) Len() int {
	if o == nil {
		return 0
	}
	o.init()
	o.lock.RLock()
	n := len(o.keyChain)
	o.lock.RUnlock()
	return n
}

func (o *OrderedMap[T, V]) Delete(key T) {
	if o == nil {
		return
	}
	o.init()
	o.lock.Lock()
	delete(o.m, key)
	index := -1
	for i, k := range o.keyChain {
		if k == key {
			index = i
			break
		}
	}
	if index != -1 {
		after := make([]T, len(o.keyChain)-1)
		copy(after, o.keyChain[:index])
		copy(after[index:], o.keyChain[index+1:])
		o.keyChain = after
	}
	o.lock.Unlock()
}

func (o *OrderedMap[T, V]) Keys() []T {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	ks := o.keyChain
	o.lock.RUnlock()
	return ks
}

func (o *OrderedMap[T, V]) Values() []V {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	values := make([]V, len(o.keyChain))
	for i, k := range o.keyChain {
		values[i] = o.m[k]
	}
	o.lock.RUnlock()
	return values
}

// ForEachReadOnly iterates in insertion order without copying the key/value
// snapshot. The handler must not mutate o or call a method that takes its
// write lock. Read-only analysis loops use this to avoid a per-call allocation.
func (o *OrderedMap[T, V]) ForEachReadOnly(handler func(i T, v V) bool) {
	if o == nil {
		return
	}
	o.init()
	o.lock.RLock()
	defer o.lock.RUnlock()
	for _, key := range o.keyChain {
		if !handler(key, o.m[key]) {
			return
		}
	}
}

func (o *OrderedMap[T, V]) ForEach(handler func(i T, v V) bool) {
	if o == nil {
		return
	}
	o.init()
	o.lock.RLock()
	keys := append([]T(nil), o.keyChain...)
	values := make([]V, len(keys))
	for i, key := range keys {
		values[i] = o.m[key]
	}
	o.lock.RUnlock()

	for i, key := range keys {
		if !handler(key, values[i]) {
			break
		}
	}
}

func (s *OrderedMap[T, V]) Copy() *OrderedMap[T, V] {
	if s == nil {
		return nil
	}
	s.init()
	s.lock.RLock()
	m := make(map[T]V, len(s.m))
	for k, v := range s.m {
		m[k] = v
	}
	ks := make([]T, len(s.keyChain))
	copy(ks, s.keyChain)
	s.lock.RUnlock()

	newMap := &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: ks,
	}
	newMap.initOnce.Do(func() {})
	return newMap
}
