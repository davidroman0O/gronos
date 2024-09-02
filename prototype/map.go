package gronos

import "sync"

// GMap is a wrapper for sync.Map
// I needed a bit of convinience
type GMap[K any, V any] struct {
	m sync.Map
}

func NewGMap[K any, V any]() *GMap[K, V] {
	return &GMap[K, V]{}
}

// Store sets the value for a key.
func (g *GMap[K, V]) Store(key K, value V) {
	g.m.Store(key, value)
}

// Load returns the value stored in the map for a key, or nil if no value is present.
// The ok result indicates whether value was found in the map.
func (g *GMap[K, V]) Load(key K) (value V, ok bool) {
	v, ok := g.m.Load(key)
	if !ok {
		return value, false
	}
	return v.(V), true
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (g *GMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	a, loaded := g.m.LoadOrStore(key, value)
	return a.(V), loaded
}

// Delete deletes the value for a key.
func (g *GMap[K, V]) Delete(key K) {
	g.m.Delete(key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (g *GMap[K, V]) Range(f func(key K, value V) bool) {
	g.m.Range(func(key, value interface{}) bool {
		return f(key.(K), value.(V))
	})
}
