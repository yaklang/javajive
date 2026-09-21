package types

import (
	"testing"
	"time"
)

func TestT16_C04_MissingCyclicCancelUnknown(t *testing.T) {
	t.Run("T16-C04", func(t *testing.T) { testT16C04(t) })
}

func testT16C04(t *testing.T) {
	cyclic := map[string]ClassHierarchyIdentity{
		"p/A": {BinaryName: "p/A", SuperClass: "p/B", ContentHash: "ha"},
		"p/B": {BinaryName: "p/B", SuperClass: "p/A", ContentHash: "hb"},
	}
	resolver := func(name string) (ClassHierarchyIdentity, bool) {
		v, ok := cyclic[name]
		return v, ok
	}
	p := NewMetadataHierarchyProvider(resolver, "cp-cyclic", "8", "opt", nil)
	wa := p.Walk("p/A")
	if !wa.Unknown || !wa.Cyclic {
		t.Fatalf("cyclic parents should be unknown/cyclic: %+v", wa)
	}
	join := p.JoinNames("p/A", "p/B")
	if !join.Unknown || !IsUnknownType(join.Source) {
		t.Fatalf("cyclic join must be unknown, got %+v", join)
	}

	miss := NewMetadataHierarchyProvider(func(string) (ClassHierarchyIdentity, bool) {
		return ClassHierarchyIdentity{}, false
	}, "cp-miss", "8", "", nil)
	if ident, ok, reason := miss.Lookup("missing/X"); ok || reason == "" {
		t.Fatalf("miss should be explicit: ident=%+v ok=%v reason=%s", ident, ok, reason)
	}
	wj := miss.JoinNames("missing/X", "missing/Y")
	if !wj.Unknown {
		t.Fatal("resolver miss must not yield a concrete class")
	}

	cancel := make(chan struct{})
	close(cancel)
	cp := NewMetadataHierarchyProvider(resolver, "cp-cancel", "8", "", cancel)
	wc := cp.Walk("p/A")
	if !wc.Unknown || !wc.Cancelled {
		t.Fatalf("cancel should be unknown: %+v", wc)
	}

	// Bounded walk: a long chain must terminate.
	long := map[string]ClassHierarchyIdentity{}
	for i := 0; i < 32; i++ {
		name := "c/N" + itoa(i)
		next := "java/lang/Object"
		if i+1 < 32 {
			next = "c/N" + itoa(i+1)
		}
		long[name] = ClassHierarchyIdentity{BinaryName: name, SuperClass: next, ContentHash: name}
	}
	lp := NewMetadataHierarchyProvider(func(n string) (ClassHierarchyIdentity, bool) {
		v, ok := long[n]
		return v, ok
	}, "cp-long", "8", "", nil)
	lp.walkCap = 8
	wl := lp.Walk("c/N0")
	if !wl.Unknown {
		t.Fatalf("capped walk should be unknown: %+v", wl)
	}

	// Negative cache is per provider: a hit on p must not appear in miss.
	if _, ok, _ := miss.Lookup("p/A"); ok {
		t.Fatal("negative cache leaked across providers")
	}

	timeout := time.After(2 * time.Second)
	done := make(chan struct{})
	go func() {
		_ = p.Walk("p/A")
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
		t.Fatal("cyclic walk did not terminate")
	}
}

func TestT16_C05_ArrayNullJoin(t *testing.T) {
	t.Run("T16-C05", func(t *testing.T) { testT16C05(t) })
}

func testT16C05(t *testing.T) {
	str := NewJavaClass("java.lang.String")
	obj := NewJavaClass("java.lang.Object")
	strArr := NewJavaArrayType(str)
	objArr := NewJavaArrayType(obj)
	intArr := NewJavaArrayType(NewJavaPrimer(JavaInteger))
	str2 := NewJavaArrayType(strArr)
	obj2 := NewJavaArrayType(objArr)
	int2 := NewJavaArrayType(intArr)

	got := ArrayLUB(strArr, objArr, nil)
	if name, _ := classNameOf(got.Source.ElementType()); name != "java.lang.Object" || got.Source.ArrayDim() != 1 {
		t.Fatalf("String[]|Object[] => %s", nameOf(got.Source))
	}
	if IsArraySubtype(intArr, objArr, nil) {
		t.Fatal("int[] must not be covariant with Object[]")
	}
	intObj := ArrayLUB(intArr, objArr, nil)
	if intObj.Source != nil && intObj.Source.IsArray() {
		t.Fatalf("int[]|Object[] must not be an array, got %s", nameOf(intObj.Source))
	}
	if !IsArraySubtype(strArr, objArr, nil) {
		t.Fatal("String[] is a subtype of Object[]")
	}
	if IsArraySubtype(intArr, int2, nil) {
		t.Fatal("int[] is not int[][]")
	}
	dim := ArrayLUB(str2, obj2, nil)
	if dim.Source == nil || dim.Source.ArrayDim() != 2 {
		t.Fatalf("String[][]|Object[][] dim=%v", dim.Source)
	}
	nullJoin := JoinTypes(NullType(), strArr, nil)
	if !nullJoin.Source.IsArray() || nullJoin.Source.ArrayDim() != 1 {
		t.Fatalf("null|String[] => %+v", nullJoin)
	}
	samePrim := ArrayLUB(intArr, intArr, nil)
	if samePrim.Unknown || primerName(samePrim.Source.ElementType()) != JavaInteger {
		t.Fatalf("int[]|int[] => %+v", samePrim)
	}

	merged := MergeTypes(strArr, objArr)
	if merged == nil || !merged.IsArray() {
		t.Fatalf("MergeTypes(String[], Object[]) should be Object[], got %v", merged)
	}
	cs := CommonSuperType(strArr, objArr)
	if cs == nil || !cs.IsArray() {
		t.Fatalf("CommonSuperType array LUB got %v", cs)
	}
}

func TestT16_C06_MemberInternalConstraint(t *testing.T) {
	t.Run("T16-C06", func(t *testing.T) { testT16C06(t) })
}

func testT16C06(t *testing.T) {
	method := NewJavaClass("java.lang.reflect.Method")
	field := NewJavaClass("java.lang.reflect.Field")
	res := JoinTypes(method, field, nil)
	if n, _ := RawClassFQN(res.Source); n != "java.lang.reflect.Member" {
		t.Fatalf("source LUB=%s want Member", n)
	}
	label := res.ConstraintLabel()
	if !containsAll(label, "java.lang.reflect.Member", "java.lang.reflect.AccessibleObject") {
		t.Fatalf("internal constraint missing AccessibleObject&Member: %s", label)
	}
	if n, _ := RawClassFQN(res.Source); n == "java.lang.reflect.AccessibleObject" {
		t.Fatal("source type must not be the non-denotable AccessibleObject intersection")
	}

	hidden := NewJavaClass("hidden.HiddenMember")
	provider := func(name string) ([]string, bool) {
		if name == "hidden/HiddenMember" {
			return []string{"java/lang/Object", "java/lang/reflect/Member"}, true
		}
		return nil, false
	}
	access := func(name string) (bool, bool) {
		switch name {
		case "hidden/HiddenMember":
			return false, true
		case "java/lang/reflect/Member", "java/lang/reflect/Method":
			return true, true
		}
		return false, false
	}
	got := JoinTypesAccessible(hidden, method, provider, access)
	if got.Unknown {
		t.Fatalf("hidden|Method unknown: %+v", got)
	}
	src, _ := RawClassFQN(got.Source)
	if src == "hidden.HiddenMember" {
		t.Fatal("inaccessible impl must not become the source declaration")
	}
}

func TestMergeTypesViaUnknownNotFirstArm(t *testing.T) {
	foo := NewJavaClass("com.acme.Foo")
	bar := NewJavaClass("com.acme.Bar")
	got := MergeTypesVia(nil, foo, bar)
	if !IsUnknownType(got) {
		t.Fatalf("unknown must not become first-arm %s", nameOf(got))
	}
	legacy := MergeTypes(foo, bar)
	if n, _ := classNameOf(legacy); n != "com.acme.Foo" {
		t.Fatalf("legacy MergeTypes keeps first-arm, got %s", n)
	}
}

func TestCommonSuperTypeViaDiamond(t *testing.T) {
	hier := map[string][]string{
		"p/C": {"p/A", "p/I"},
		"p/D": {"p/B", "p/I"},
		"p/A": {"java/lang/Object"},
		"p/B": {"java/lang/Object"},
		"p/I": {"java/lang/Object"},
	}
	provider := func(name string) ([]string, bool) { v, ok := hier[name]; return v, ok }
	got := CommonSuperTypeVia(NewJavaClass("p.C"), NewJavaClass("p.D"), provider)
	if n, _ := classNameOf(got); n != "p.I" {
		t.Fatalf("diamond join=%s want p.I", n)
	}
	merged := MergeTypesVia(provider, NewJavaClass("p.C"), NewJavaClass("p.D"))
	if n, _ := classNameOf(merged); n != "p.I" {
		t.Fatalf("MergeTypesVia diamond=%s", n)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !stringContains(s, p) {
			return false
		}
	}
	return true
}

func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
