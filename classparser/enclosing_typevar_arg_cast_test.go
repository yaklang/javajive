package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A type witness can live on the argument or on the erased result view.
// The old text workaround switch must not disable the production binding plan.
func TestEnclosingTypeVarArgCastIsLoadBearing(t *testing.T) {
	view, err := os.ReadFile("testdata/regression/EnclosingTypeVarArgSeed$View.class")
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(name string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + name + ".class")
		return b, e == nil
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", setting)
		source, err := DecompileWithResolver(view, resolver)
		if err != nil {
			t.Fatal(err)
		}
		argumentWitness := strings.Contains(source, "(K)")
		resultWitness := strings.Contains(source, "(Map.Entry<K, Collection<V>>) (Map.Entry)")
		if !strings.Contains(source, "immutableEntry(") || (!argumentWitness && !resultWitness) || !strings.Contains(source, "wrapCollection(") {
			t.Fatalf("lost enclosing type witness or factory call:\n%s", source)
		}
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

	path := "testdata/regression/ImmediateFutureCastSeed.class"
	raw, _, _ := reviewedFixtureMethod(t, path, "load", "()Ljava/util/concurrent/Future;")
	assertReviewedTypeVarMethod(t, raw, "load", "()Ljava/util/concurrent/Future;", "()Ljava/util/concurrent/Future<TV;>;")
	assertReviewedGenericField(t, raw, "futureValue", "Ljava/util/concurrent/Future;", "Ljava/util/concurrent/Future<TV;>;")
	helper, err := os.ReadFile("testdata/regression/ImmediateFutureCastSeed$Futures.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, helper, "immediateFuture", "(Ljava/lang/Object;)Ljava/util/concurrent/Future;", "<V:Ljava/lang/Object;>(TV;)Ljava/util/concurrent/Future<TV;>;")
	assertReviewedTypeVarInvoke(t, path, "load", "()Ljava/util/concurrent/Future;", 18, 184, "ImmediateFutureCastSeed$Futures", "immediateFuture", "(Ljava/lang/Object;)Ljava/util/concurrent/Future;")
	reviewedSeedSources(t, path, "JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", true, func(source string) {
		body := reviewedSourceMethod(t, source, `load\(\)`)
		compact := compactReviewedGenericSource(body)
		if !strings.Contains(compact, "(Future<V>)(Future)") || (!strings.Contains(compact, "this.futureValue") || !strings.Contains(compact, ".immediateFuture(null)")) {
			t.Fatal("lost saved/fallback identity or original propagated null: " + body)
		}
	})
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
