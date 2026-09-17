package core

import "testing"

func TestStackDepthFork(t *testing.T) {
	root := newStackItem(nil, nil)
	a := NewStackSimulation(root, nil, nil)
	for i := 1; i <= 10000; i++ {
		a.Push(nil)
		if a.Size() != i {
			t.Fatal(i, a.Size())
		}
	}
	b := NewStackSimulation(a.stackEntry, nil, nil)
	for i := 0; i < 10000; i++ {
		b.Pop()
	}
	if b.Size() != 0 || a.Size() != 10000 {
		t.Fatal("fork changed shared stack depth")
	}
	b.Pop()
	if b.Size() != 0 {
		t.Fatal("underflow changed depth")
	}
}
func BenchmarkStackSizeDeep(b *testing.B) {
	s := NewStackSimulation(newStackItem(nil, nil), nil, nil)
	for i := 0; i < 65535; i++ {
		s.Push(nil)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.Size() != 65535 {
			b.Fatal(s.Size())
		}
	}
}
