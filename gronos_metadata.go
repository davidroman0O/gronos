package gronos

import (
	"fmt"
	"reflect"
	"sync"
)

type Metadata[K comparable] struct {
	data sync.Map
}

func NewMetadata[K comparable]() *Metadata[K] {
	return &Metadata[K]{}
}

func (m *Metadata[K]) Get(key string) interface{} {
	value, _ := m.data.Load(key)
	return value
}

func (m *Metadata[K]) Set(key string, value interface{}) {
	m.data.Store(key, value)
}

func (m *Metadata[K]) Delete(key string) {
	m.data.Delete(key)
}

func (m *Metadata[K]) Clear() {
	m.data = sync.Map{}
}

func (m *Metadata[K]) Copy() *Metadata[K] {
	newMetadata := NewMetadata[K]()
	m.data.Range(func(key, value interface{}) bool {
		newMetadata.Set(key.(string), value)
		return true
	})
	return newMetadata
}

func (m *Metadata[K]) Merge(metadata *Metadata[K]) {
	metadata.data.Range(func(key, value interface{}) bool {
		m.Set(key.(string), value)
		return true
	})
}

func (m *Metadata[K]) GetKey() K {
	value, _ := m.data.Load("$key")
	return value.(K)
}

func (m *Metadata[K]) GetKeyString() string {
	value, _ := m.data.Load("$key")
	switch value.(type) {
	case string:
		return value.(string)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func (m *Metadata[K]) GetID() int {
	value, _ := m.data.Load("$id")
	return value.(int)
}

func (m *Metadata[K]) SetKey(key K) {
	m.data.Store("$key", fmt.Sprintf("%v", key))
}

func (m *Metadata[K]) SetID(id int) {
	m.data.Store("$id", id)
}

func (m *Metadata[K]) HasKey() bool {
	_, ok := m.data.Load("$key")
	return ok
}

func (m *Metadata[K]) HasID() bool {
	_, ok := m.data.Load("$id")
	return ok
}

func (m *Metadata[K]) GetType() reflect.Type {
	value, _ := m.data.Load("$type")
	return value.(reflect.Type)
}

func (m *Metadata[K]) SetType(t reflect.Type) {
	m.data.Store("$type", t)
}

func (m *Metadata[K]) GetName() string {
	value, _ := m.data.Load("$name")
	return value.(string)
}

func (m *Metadata[K]) SetName(n string) {
	m.data.Store("$name", n)
}

func (m *Metadata[K]) GetError() error {
	value, _ := m.data.Load("$error")
	return value.(error)
}

func (m *Metadata[K]) SetError(e error) {
	m.data.Store("$error", e)
}

func (m *Metadata[K]) HasType() bool {
	_, ok := m.data.Load("$type")
	return ok
}

func (m *Metadata[K]) HasName() bool {
	_, ok := m.data.Load("$name")
	return ok
}

func (m *Metadata[K]) HasError() bool {
	_, ok := m.data.Load("$error")
	return ok
}
