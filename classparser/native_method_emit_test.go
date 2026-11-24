package javaclassparser

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func TestNativeMethodsEmitNativeModifierAndKeepVoidAndThrows(t *testing.T) {
	t.Run("seed_preserved_with_native_modifier", func(t *testing.T) {
		data, err := os.ReadFile("testdata/regression/StubGenericSigSeed.class")
		if err != nil {
			t.Fatalf("read StubGenericSigSeed.class: %v", err)
		}
		res, err := DecompileWithOptions(data, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if res.Status != "complete" || len(res.StubMethods) != 0 {
			t.Fatalf("expected complete decompilation with 0 stubs, got status=%s stubs=%v\n%s",
				res.Status, res.StubMethods, res.Source)
		}
		want := "public final native StringBuilder consume(StringBuilder var0, Iterator<?> var1);"
		if !strings.Contains(res.Source, want) {
			t.Fatalf("expected %q in source, got:\n%s", want, res.Source)
		}
		foundConsume := false
		for _, m := range res.Members {
			if m.Kind == "method" && m.Name == "consume" {
				foundConsume = true
				if m.State != "preserved" {
					t.Fatalf("expected consume member State=preserved, got %+v", m)
				}
			}
		}
		if !foundConsume {
			t.Fatalf("consume member missing from ledger: %+v", res.Members)
		}
	})

	t.Run("void_throws_varargs_synchronized_native", func(t *testing.T) {
		cls := classfilebuilder.NewClass("demo/NativeApi", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL)
		cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_NATIVE, "free", "(J)V")
		conv := cls.AddMethod(classfilebuilder.ACC_PRIVATE|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_NATIVE, "convert", "([FI)I")
		cls.SetMethodExceptions(conv, []string{"Ljava/io/IOException;"})
		cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_SYNCHRONIZED|classfilebuilder.ACC_VARARGS|classfilebuilder.ACC_NATIVE, "format", "([Ljava/lang/String;)Ljava/lang/String;")

		raw, err := cls.Bytes()
		if err != nil {
			t.Fatalf("build class bytes: %v", err)
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if res.Status != "complete" || len(res.StubMethods) != 0 {
			t.Fatalf("expected complete decompilation with 0 stubs, got status=%s stubs=%v\n%s",
				res.Status, res.StubMethods, res.Source)
		}
		for _, want := range []string{
			"public static native void free(long var0);",
			"private final native int convert(float[] var0, int var1) throws IOException;",
			"public static synchronized native String format(String... var0);",
		} {
			if !strings.Contains(res.Source, want) {
				t.Fatalf("missing %q in source:\n%s", want, res.Source)
			}
		}
		if strings.Contains(res.Source, DecompileStubMarker) {
			t.Fatalf("unexpected stub marker in source:\n%s", res.Source)
		}
		methodCount := 0
		for _, m := range res.Members {
			if m.Kind == "method" {
				methodCount++
				if m.State != "preserved" {
					t.Fatalf("expected method member %+v State=preserved", m)
				}
			}
		}
		if methodCount != 3 {
			t.Fatalf("expected 3 preserved method members, got %d (%+v)", methodCount, res.Members)
		}
	})

	t.Run("access_flag_combinations", func(t *testing.T) {
		cases := []struct {
			name  string
			flags uint16
			want  string
		}{
			{"final_native", 0x0001 | 0x0010 | 0x0100, "public final native"},
			{"final_abstract_strips_final", 0x0001 | 0x0010 | 0x0400, "public abstract"},
			{"native_strictfp_strips_strictfp", 0x0001 | 0x0100 | 0x0800, "public native"},
			{"native_abstract_strips_native", 0x0001 | 0x0100 | 0x0400, "public abstract"},
		}
		for _, tc := range cases {
			_, got := getMethodAccessFlagsVerbose(tc.flags)
			if got != tc.want {
				t.Fatalf("%s: getMethodAccessFlagsVerbose(0x%x) = %q, want %q", tc.name, tc.flags, got, tc.want)
			}
		}
	})
}
