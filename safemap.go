package gronos

import "sync"

type safeMapPtr[T comparable, V any] struct {
	m map[T]*V
	sync.Mutex
}

func newSafeMapPtr[T comparable, V any]() *safeMapPtr[T, V] {
	return &safeMapPtr[T, V]{
		m: make(map[T]*V),
	}
}

func (sm *safeMapPtr[T, V]) Set(k T, v *V) {
	sm.Lock()
	sm.m[k] = v
	sm.Unlock()
}

func (sm *safeMapPtr[T, V]) Get(k T) (*V, bool) {
	sm.Lock()
	v, ok := sm.m[k]
	sm.Unlock()
	return v, ok
}

func (sm *safeMapPtr[T, V]) Delete(k T) {
	sm.Lock()
	delete(sm.m, k)
	sm.Unlock()
}

func (sm *safeMapPtr[T, V]) Has(k T) bool {
	sm.Lock()
	_, ok := sm.m[k]
	sm.Unlock()
	return ok
}

func (sm *safeMapPtr[T, V]) Len() int {
	sm.Lock()
	l := len(sm.m)
	sm.Unlock()
	return l
}

func (sm *safeMapPtr[T, V]) ForEach(f func(T, *V) error) error {
	sm.Lock()
	defer sm.Unlock()
	for k, v := range sm.m {
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}

///////////

type safeMap[T comparable, V any] struct {
	m map[T]V
	sync.Mutex
}

func newSafeMap[T comparable, V any]() *safeMap[T, V] {
	return &safeMap[T, V]{
		m: make(map[T]V),
	}
}

func (sm *safeMap[T, V]) Set(k T, v V) {
	sm.Lock()
	sm.m[k] = v
	sm.Unlock()
}

func (sm *safeMap[T, V]) Get(k T) (V, bool) {
	sm.Lock()
	v, ok := sm.m[k]
	sm.Unlock()
	return v, ok
}

func (sm *safeMap[T, V]) Delete(k T) {
	sm.Lock()
	delete(sm.m, k)
	sm.Unlock()
}

func (sm *safeMap[T, V]) Has(k T) bool {
	sm.Lock()
	_, ok := sm.m[k]
	sm.Unlock()
	return ok
}

func (sm *safeMap[T, V]) Len() int {
	sm.Lock()
	l := len(sm.m)
	sm.Unlock()
	return l
}

func (sm *safeMap[T, V]) ForEach(f func(T, V)) {
	sm.Lock()
	for k, v := range sm.m {
		f(k, v)
	}
	sm.Unlock()
}
