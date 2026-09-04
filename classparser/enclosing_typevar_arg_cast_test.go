package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestEnclosingTypeVarArgCastIsLoadBearing pins enclosingTypeVarArgCast.
// Kill-switch: JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF. Real hit: guava
// AbstractMapBasedMultimap$AsMap.wrapEntry Maps.immutableEntry(Object key, ...).
func TestEnclosingTypeVarArgCastIsLoadBearing(t *testing.T) {
	view, err := os.ReadFile("testdata/regression/EnclosingTypeVarArgSeed$View.class")
	if err != nil {
		t.Fatalf("read View seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF")
	on, err := DecompileWithResolver(view, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "immutableEntry((K)") && !strings.Contains(on, "immutableEntry((K) (") {
		t.Errorf("fix ON: expected (K) cast on immutableEntry key, got:\n%s", on)
	}

	t.Setenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "1")
	off, err := DecompileWithResolver(view, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "immutableEntry((K)") || strings.Contains(off, "immutableEntry((K) (") {
		t.Errorf("fix OFF: expected no (K) cast, got:\n%s", off)
	}
}

func TestClosedTypeVarArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClosedTypeVarArgSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF")
	on, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "closed((C)") && !strings.Contains(on, "closed((C) (") {
		t.Errorf("fix ON: expected (C) cast on closed() args, got:\n%s", on)
	}

	t.Setenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "1")
	off, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "closed((C)") || strings.Contains(off, "closed((C) (") {
		t.Errorf("fix OFF: expected no (C) cast on closed(), got:\n%s", off)
	}

	// Static closedInts must not reference class type variable C.
	if strings.Contains(on, "closedInts") && strings.Contains(on, "closed((C)") {
		// The instance method intersection() legitimately has closed((C); the static
		// method must not. Split on closedInts and assert that fragment has no (C).
		idx := strings.Index(on, "closedInts")
		if idx >= 0 {
			frag := on[idx:]
			if end := strings.Index(frag, "\n\t"); end > 0 {
				frag = frag[:end+20]
			}
			if strings.Contains(frag, "(C)") {
				t.Errorf("fix ON: static closedInts must not emit (C), got:\n%s", on)
			}
		}
	}
}

func TestImmediateFutureCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ImmediateFutureCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF")
	on, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "immediateFuture((V)") && !strings.Contains(on, "immediateFuture((V) (") {
		t.Errorf("fix ON: expected (V) cast on immediateFuture arg, got:\n%s", on)
	}

	t.Setenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "1")
	off, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "immediateFuture((V)") || strings.Contains(off, "immediateFuture((V) (") {
		t.Errorf("fix OFF: expected no (V) cast, got:\n%s", off)
	}
}

func TestNewEntryTypeVarCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NewEntryTypeVarCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF")
	on, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "newEntry(") {
		t.Fatalf("expected newEntry call, got:\n%s", on)
	}
	if !strings.Contains(on, "newEntry(this,var1,0,(E)") && !strings.Contains(on, "newEntry(this, var1, 0, (E)") &&
		!strings.Contains(on, ",(E)(var2)") && !strings.Contains(on, ", (E)(var2)") &&
		!strings.Contains(on, ",(E) (var2)") {
		t.Errorf("fix ON: expected (E) cast on newEntry next arg, got:\n%s", on)
	}

	t.Setenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "1")
	off, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "newEntry(this,var1,0,(E)") || strings.Contains(off, "newEntry(this, var1, 0, (E)") {
		t.Errorf("fix OFF: expected no (E) cast on newEntry next arg, got:\n%s", off)
	}
}

func TestWrapFieldInitializerGetDeclaredFieldIsLoadBearing(t *testing.T) {
	in := "\tprivate static final long valueOffset = UNSAFE.objectFieldOffset(Class.class.getDeclaredField(\"value\"));\n"
	os.Unsetenv("JDEC_WRAP_FIELD_INIT_OFF")
	on := wrapFieldInitializerReflection(in)
	if !strings.Contains(on, "catch(NoSuchFieldException") {
		t.Errorf("fix ON: expected try/catch(NoSuchFieldException), got:\n%s", on)
	}
	if strings.Contains(on, "= UNSAFE.objectFieldOffset") && !strings.Contains(on, "valueOffset = ") {
		t.Errorf("fix ON: initializer must move into try, got:\n%s", on)
	}
	t.Setenv("JDEC_WRAP_FIELD_INIT_OFF", "1")
	off := wrapFieldInitializerReflection(in)
	if strings.Contains(off, "catch(NoSuchFieldException") {
		t.Errorf("fix OFF: expected no wrap, got:\n%s", off)
	}
}

func TestFixDrainUninterruptiblyPollInTryIsLoadBearing(t *testing.T) {
	in := "\tpublic static <E> int drainUninterruptibly(BlockingQueue<E> var0, Collection<? super E> var1, int var2, long var3, TimeUnit var4) {\n" +
		drainUnintEmptyTryBlock + "\n\t}\n"
	os.Unsetenv("JDEC_DRAIN_UNINTERRUPTIBLY_POLL_IN_TRY_OFF")
	on := fixDrainUninterruptiblyPollInTry(in)
	if !strings.Contains(on, "var8 = var0.poll(") {
		t.Errorf("fix ON: expected poll() inside try, got:\n%s", on)
	}
	if strings.Contains(on, "try{\n\t\t\t\t\t\t\t\tbreak;") {
		t.Errorf("fix ON: empty try{break} must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_DRAIN_UNINTERRUPTIBLY_POLL_IN_TRY_OFF", "1")
	off := fixDrainUninterruptiblyPollInTry(in)
	if strings.Contains(off, "try{\n\t\t\t\t\t\t\t\tvar8 = var0.poll(") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixGetUninterruptiblyGetInTryIsLoadBearing(t *testing.T) {
	in := "\tprivate static <V> V getUninterruptibly(Future<V> var0) throws ExecutionException {\n" +
		getUnintEmptyTryBlock + "\n\t}\n"
	os.Unsetenv("JDEC_GETUNINTERRUPTIBLY_GET_IN_TRY_OFF")
	on := fixGetUninterruptiblyGetInTry(in)
	if !strings.Contains(on, "return (V) (var0.get());") {
		t.Errorf("fix ON: expected get() inside try, got:\n%s", on)
	}
	if strings.Contains(on, "\t\t\t\tbreak;") {
		t.Errorf("fix ON: empty try{break} must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_GETUNINTERRUPTIBLY_GET_IN_TRY_OFF", "1")
	off := fixGetUninterruptiblyGetInTry(in)
	if strings.Contains(off, "return (V) (var0.get());") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}
