package funk

import (
	"reflect"
	"testing"
)

func TestFilter(t *testing.T) {
	in := []int{1, 2, 3, 4, 5, 6}
	got := Filter(in, func(v int) bool { return v%2 == 0 }).([]int)
	want := []int{2, 4, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Filter = %v want %v", got, want)
	}
}

func TestMap(t *testing.T) {
	in := []int{1, 2, 3}
	got := Map(in, func(v int) string {
		return string(rune('a' + v - 1))
	}).([]string)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Map = %v want %v", got, want)
	}
}

func TestReverseSlice(t *testing.T) {
	got := Reverse([]int{1, 2, 3}).([]int)
	want := []int{3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reverse slice = %v want %v", got, want)
	}
}

func TestReverseString(t *testing.T) {
	if got := Reverse("abc").(string); got != "cba" {
		t.Fatalf("Reverse string = %q want %q", got, "cba")
	}
}

func TestContains(t *testing.T) {
	if !Contains([]string{"x", "y"}, "y") {
		t.Fatal("slice Contains should be true")
	}
	if Contains([]string{"x", "y"}, "z") {
		t.Fatal("slice Contains should be false")
	}
	if !Contains("hello", "ell") {
		t.Fatal("string Contains should be true")
	}
	if !Contains(map[string]int{"a": 1}, "a") {
		t.Fatal("map Contains should be true")
	}
}

func TestForEach(t *testing.T) {
	sum := 0
	ForEach([]int{1, 2, 3, 4}, func(v int) { sum += v })
	if sum != 10 {
		t.Fatalf("ForEach sum = %d want 10", sum)
	}
}
