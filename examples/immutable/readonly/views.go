// Package readonly supplies the borrowed views used by the immutable example.
// Views share storage; the owner is responsible for preventing later mutation.
package readonly

// Slice exposes elements without a mutation API. Its zero value is empty.
type Slice[T any] struct{ values []T }

// Borrow views values without copying them.
func Borrow[T any](values []T) Slice[T] { return Slice[T]{values: values} }

// Len returns the number of elements.
func (s Slice[T]) Len() int { return len(s.values) }

// At reads one element, panicking if i is out of range.
func (s Slice[T]) At(i int) T { return s.values[i] }

// Map exposes entries without a mutation API. Its zero value is empty.
type Map[K comparable, V any] struct{ values map[K]V }

// BorrowMap views values without copying them.
func BorrowMap[K comparable, V any](values map[K]V) Map[K, V] { return Map[K, V]{values: values} }

// Len returns the number of entries.
func (m Map[K, V]) Len() int { return len(m.values) }

// Get returns the value and whether key is present.
func (m Map[K, V]) Get(key K) (V, bool) { v, ok := m.values[key]; return v, ok }
