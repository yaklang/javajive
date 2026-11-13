package utils

import (
	"sync"
)

type Set[T comparable] struct {
	m map[T]struct{}
	sync.RWMutex
}

func NewSet[T comparable](list ...[]T) *Set[T] {
	s := &Set[T]{
		m: make(map[T]struct{}),
	}
	if len(list) > 0 {
		s.AddList(list[0])
	}
	return s
}

// NewSetFromMap takes ownership of m. It lets a caller build a set with a
// plain map (no per-element locking) and publish it as a Set afterwards. The
// caller must not retain or mutate m after the call.
func NewSetFromMap[T comparable](m map[T]struct{}) *Set[T] {
	if m == nil {
		m = make(map[T]struct{})
	}
	return &Set[T]{m: m}
}

// NewSetSized allocates a set whose backing map is sized for n elements.
func NewSetSized[T comparable](n int) *Set[T] {
	if n < 0 {
		n = 0
	}
	return &Set[T]{m: make(map[T]struct{}, n)}
}

//func main() {
//	// Initialize our Set
//	s := New()
//
//	// Add example items
//	s.Add("item1")
//	s.Add("item1") // duplicate item
//	s.Add("item2")
//	fmt.Printf("%d items\n", s.Len())
//
//	// Clear all items
//	s.Clear()
//	if s.IsEmpty() {
//		fmt.Printf("0 items\n")
//	}
//
//	s.Add("item2")
//	s.Add("item3")
//	s.Add("item4")
//
//	// Check for existence
//	if s.Has("item2") {
//		fmt.Println("item2 does exist")
//	}
//
//	// Remove some of our items
//	s.Remove("item2")
//	s.Remove("item4")
//	fmt.Println("list of all items:", s.List())
//}

// Add add
func (s *Set[T]) Add(item T) {
	s.Lock()
	defer s.Unlock()
	s.m[item] = struct{}{}
}

func (s *Set[T]) AddList(items []T) {
	for _, item := range items {
		s.Add(item)
	}
}

// CloneWithList copies the set under a single read lock and returns the copied
// elements. It is cheaper than NewSetSized plus Add because it does not take a
// write lock per element.
func (s *Set[T]) CloneWithList() (*Set[T], []T) {
	s.RLock()
	defer s.RUnlock()
	dst := make(map[T]struct{}, len(s.m))
	list := make([]T, 0, len(s.m))
	for item := range s.m {
		dst[item] = struct{}{}
		list = append(list, item)
	}
	return &Set[T]{m: dst}, list
}

// ContainsAll reports whether every item is present, taking one read lock.
func (s *Set[T]) ContainsAll(items []T) bool {
	s.RLock()
	defer s.RUnlock()
	for _, item := range items {
		if _, ok := s.m[item]; !ok {
			return false
		}
	}
	return true
}

// Remove deletes the specified item from the map
func (s *Set[T]) Remove(item T) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, item)
}

// Has looks for the existence of an item
func (s *Set[T]) Has(item T) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.m[item]
	return ok
}

// Len returns the number of items in a set.
func (s *Set[T]) Len() int {
	return len(s.List())
}

// Size returns the number of items without materializing a list.
func (s *Set[T]) Size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Clear removes all items from the set
func (s *Set[T]) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[T]struct{})
}

// IsEmpty checks for emptiness
func (s *Set[T]) IsEmpty() bool {
	if s.Len() == 0 {
		return true
	}
	return false
}

// Set returns a slice of all items
func (s *Set[T]) List() []T {
	s.RLock()
	defer s.RUnlock()
	list := make([]T, 0)
	for item := range s.m {
		list = append(list, item)
	}
	return list
}

func (s *Set[T]) ForEach(h func(T)) {
	if s == nil {
		return
	}
	s.RLock()
	defer s.RUnlock()
	for item := range s.m {
		h(item)
	}
}

// ForEachWhile iterates until the handler returns false. It avoids the
// allocation of List for callers that only scan or stop at the first match.
func (s *Set[T]) ForEachWhile(h func(T) bool) {
	if s == nil {
		return
	}
	s.RLock()
	defer s.RUnlock()
	for item := range s.m {
		if !h(item) {
			return
		}
	}
}

func (s *Set[T]) Diff(other *Set[T]) *Set[T] {
	s.RLock()
	other.RLock()
	defer s.RUnlock()
	defer other.RUnlock()

	diff := NewSet[T]()
	for item := range s.m {
		if !other.Has(item) {
			diff.Add(item)
		}
	}
	for item := range other.m {
		if !s.Has(item) {
			diff.Add(item)
		}
	}
	return diff
}

func (s *Set[T]) And(other *Set[T]) *Set[T] {
	s.RLock()
	other.RLock()
	defer s.RUnlock()
	defer other.RUnlock()

	intersection := NewSet[T]()
	for item := range s.m {
		if other.Has(item) {
			intersection.Add(item)
		}
	}
	return intersection
}

func (s *Set[T]) Or(other *Set[T]) *Set[T] {
	s.RLock()
	other.RLock()
	defer s.RUnlock()
	defer other.RUnlock()

	union := NewSet[T]()
	for item := range s.m {
		union.Add(item)
	}
	for item := range other.m {
		union.Add(item)
	}
	return union
}
