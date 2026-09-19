package types

import "testing"

func TestBridgedCommonSuperTypeEliminatesDominatedMarker(t *testing.T) {
	hierarchy := map[string][]string{
		"p/Left":   {"p/Middle", "java/io/Serializable"},
		"p/Middle": {"p/Base"},
		"p/Right":  {"p/Base", "java/io/Serializable"},
		"p/Base":   {"java/lang/Object", "java/io/Serializable"},
	}
	provider := func(name string) ([]string, bool) { v, ok := hierarchy[name]; return v, ok }
	for _, pair := range [][2]string{{"p.Left", "p.Right"}, {"p.Right", "p.Left"}} {
		got := BridgedCommonSuperType(NewJavaClass(pair[0]), NewJavaClass(pair[1]), provider)
		if name, _ := RawClassFQN(got); name != "p.Base" {
			t.Fatalf("join %v = %v; want p.Base", pair, name)
		}
	}
}
