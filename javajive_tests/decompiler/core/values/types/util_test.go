package types_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	types "github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// TestSlashToDot verifies the fast '/'->'.' conversion against a table of edge cases.
func TestSlashToDot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"X", "X"},
		{"/", "."},
		{"a/b", "a.b"},
		{"java/lang/String", "java.lang.String"},
		{"no_slash_here", "no_slash_here"},
		{"/leading", ".leading"},
		{"trailing/", "trailing."},
		{"a//b", "a..b"},
		{"com/hazelcast/Foo$Bar", "com.hazelcast.Foo$Bar"},
	}
	for _, c := range cases {
		if got := types.SlashToDot(c.in); got != c.want {
			t.Errorf("types.SlashToDot(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSlashToDotEquivalence is a property test: types.SlashToDot must be byte-identical to the
// strings.Replace(s, "/", ".", -1) it replaces, for randomized inputs.
func TestSlashToDotEquivalence(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	alphabet := []byte("ab/.$_/0/")
	for i := 0; i < 5000; i++ {
		n := r.Intn(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		s := string(b)
		want := strings.Replace(s, "/", ".", -1)
		if got := types.SlashToDot(s); got != want {
			t.Fatalf("mismatch for %q: types.SlashToDot=%q strings.Replace=%q", s, got, want)
		}
	}
}

// TestSlashToDotNoAllocWhenClean documents the fast path: an input without '/' must be
// returned without copying (same backing string header), so the common case is free.
func TestSlashToDotNoAllocWhenClean(t *testing.T) {
	s := "already.dotted.name"
	got := types.SlashToDot(s)
	if got != s {
		t.Fatalf("types.SlashToDot(%q) = %q", s, got)
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = types.SlashToDot("java.lang.String")
	})
	if allocs != 0 {
		t.Errorf("types.SlashToDot on a slash-free string allocated %.0f times, want 0", allocs)
	}
}

func TestParseJavaDescriptionPrimitives(t *testing.T) {
	cases := map[string]string{
		"B": types.JavaByte, "C": types.JavaChar, "D": types.JavaDouble, "F": types.JavaFloat,
		"I": types.JavaInteger, "J": types.JavaLong, "S": types.JavaShort, "Z": types.JavaBoolean, "V": types.JavaVoid,
	}
	for desc, wantName := range cases {
		typ, rest, err := types.ParseJavaDescription(desc + "REST")
		if err != nil {
			t.Fatalf("types.ParseJavaDescription(%q) err: %v", desc, err)
		}
		if rest != "REST" {
			t.Errorf("desc %q rest = %q, want REST", desc, rest)
		}
		p, ok := typ.RawType().(*types.JavaPrimer)
		if !ok {
			t.Fatalf("desc %q: RawType %T, want *JavaPrimer", desc, typ.RawType())
		}
		if p.Name != wantName {
			t.Errorf("desc %q: name %q, want %q", desc, p.Name, wantName)
		}
	}
}

// TestParseJavaDescriptionClass verifies L...; parsing produces a dotted class name and
// consumes exactly through the ';'.
func TestParseJavaDescriptionClass(t *testing.T) {
	typ, rest, err := types.ParseJavaDescription("Ljava/lang/String;Lnext/Type;")
	if err != nil {
		t.Fatal(err)
	}
	if rest != "Lnext/Type;" {
		t.Errorf("rest = %q, want Lnext/Type;", rest)
	}
	jc, ok := typ.RawType().(*types.JavaClass)
	if !ok {
		t.Fatalf("RawType %T, want *JavaClass", typ.RawType())
	}
	if jc.Name != "java.lang.String" {
		t.Errorf("name %q, want java.lang.String", jc.Name)
	}
}

// TestParseJavaDescriptionArray checks single and multi-dimensional arrays.
func TestParseJavaDescriptionArray(t *testing.T) {
	t1, _, err := types.ParseJavaDescription("[I")
	if err != nil {
		t.Fatal(err)
	}
	if !t1.IsArray() || t1.ArrayDim() != 1 {
		t.Errorf("[I: IsArray=%v dim=%d, want true/1", t1.IsArray(), t1.ArrayDim())
	}
	t2, _, err := types.ParseJavaDescription("[[Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	if !t2.IsArray() || t2.ArrayDim() != 2 {
		t.Errorf("[[L...: IsArray=%v dim=%d, want true/2", t2.IsArray(), t2.ArrayDim())
	}
	t3, _, err := types.ParseJavaDescription("[[[J")
	if err != nil {
		t.Fatal(err)
	}
	if !t3.IsArray() || t3.ArrayDim() != 3 {
		t.Errorf("[[[J: IsArray=%v dim=%d, want true/3", t3.IsArray(), t3.ArrayDim())
	}
	if got := t3.String(&class_context.ClassContext{}); got != "long[][][]" {
		t.Errorf("[[[J string = %q, want long[][][]", got)
	}
	if got := t3.ElementType().String(&class_context.ClassContext{}); got != "long[][]" {
		t.Errorf("[[[J element string = %q, want long[][]", got)
	}
}

// TestParseMethodDescriptor checks param counts and return types for representative method
// descriptors -- the hot path that previously re-ran strings.Replace per occurrence.
func TestParseMethodDescriptor(t *testing.T) {
	cases := []struct {
		desc       string
		paramCount int
		retName    string // JavaPrimer name when applicable
	}{
		{"()V", 0, types.JavaVoid},
		{"(II)I", 2, types.JavaInteger},
		{"(Ljava/lang/String;)Z", 1, types.JavaBoolean},
		{"(Ljava/lang/String;[IJ)V", 3, types.JavaVoid},
	}
	for _, c := range cases {
		typ, err := types.ParseMethodDescriptor(c.desc)
		if err != nil {
			t.Fatalf("types.ParseMethodDescriptor(%q) err: %v", c.desc, err)
		}
		ft := typ.FunctionType()
		if ft == nil {
			t.Fatalf("desc %q: FunctionType nil", c.desc)
		}
		if len(ft.ParamTypes) != c.paramCount {
			t.Errorf("desc %q: params=%d, want %d", c.desc, len(ft.ParamTypes), c.paramCount)
		}
		if p, ok := ft.ReturnType.RawType().(*types.JavaPrimer); ok {
			if p.Name != c.retName {
				t.Errorf("desc %q: ret %q, want %q", c.desc, p.Name, c.retName)
			}
		}
	}
}

// TestParseMethodDescriptorInternsClasses proves the optimization end-to-end: parsing two
// method descriptors that both reference java/lang/String reuses the same interned leaf.
func TestParseMethodDescriptorInternsClasses(t *testing.T) {
	a, err := types.ParseMethodDescriptor("(Ljava/lang/String;)V")
	if err != nil {
		t.Fatal(err)
	}
	b, err := types.ParseMethodDescriptor("(ILjava/lang/String;)I")
	if err != nil {
		t.Fatal(err)
	}
	pa := a.FunctionType().ParamTypes[0].RawType()
	pb := b.FunctionType().ParamTypes[1].RawType()
	if pa != pb {
		t.Fatalf("expected the java/lang/String leaf to be interned and shared across descriptors")
	}
}

// BenchmarkSlashToDot vs BenchmarkStringsReplaceSlash is the algorithm comparison for the
// '/'->'.' conversion on a typical class name.
func BenchmarkSlashToDot(b *testing.B) {
	s := "com/hazelcast/client/impl/protocol/DefaultMessageTaskFactoryProvider"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = types.SlashToDot(s)
	}
}

func BenchmarkStringsReplaceSlash(b *testing.B) {
	s := "com/hazelcast/client/impl/protocol/DefaultMessageTaskFactoryProvider"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = strings.Replace(s, "/", ".", -1)
	}
}

// BenchmarkParseMethodDescriptorCached vs Uncached shows the flyweight effect on the hot
// descriptor path (same descriptor parsed repeatedly, as in a real constant pool).
func BenchmarkParseMethodDescriptor(b *testing.B) {
	desc := "(Ljava/lang/String;Ljava/util/Map;I[Ljava/lang/Object;)Ljava/lang/String;"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = types.ParseMethodDescriptor(desc)
	}
}
