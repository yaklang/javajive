package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeIndependentLeafCannotBorrowForeignSyntheticAccess(t *testing.T) {
	for _, variant := range []string{"private method", "renamed private method", "private field"} {
		t.Run(variant, func(t *testing.T) {
			fixture := nativeIndependentLeafChainFixture
			declaration, argument := "private static Object identity(Object token){return token;}", "(T)LeafSupport.identity(token)"
			if variant == "renamed private method" {
				declaration, argument = "private static Object changed(Object token){return token;}", "(T)LeafSupport.changed(token)"
			}
			if variant == "private field" {
				declaration, argument = "private static Object token; static <T>T assign(T t){token=t;return t;}", "(T)(LeafSupport.assign(token)==null?LeafSupport.token:LeafSupport.token)"
			}
			fixture = strings.Replace(fixture, "class LeafSupport{static abstract class Base<T>", "class LeafSupport{"+declaration+"static abstract class Base<T>", 1)
			fixture = strings.Replace(fixture, "Base(T token){super(token);}", "Base(T token){super("+argument+");}", 1)
			files := nativeCompileClasses(t, fixture)
			nativeIndependentLeafChainInput(t, files)
			// Run the unchanged original driver before requiring source admission to
			// refuse the inaccessible invocation. Metadata-only refusal is not an
			// algorithm-correctness repair for the private bridge itself.
			_, java := t04Tools(t)
			dir := t.TempDir()
			for name, raw := range files {
				if e := os.WriteFile(filepath.Join(dir, name), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, dir, "LeafDriver"); got != "80:independent:static:leaf:early:receiver\n" {
				t.Fatal("original oracle", got)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			obj, e := Parse(files["LeafSupport$Base.class"])
			if e != nil {
				t.Fatal(e)
			}
			entry := z.nativeMemberIndependentEntry(obj)
			if entry != nil && entry.family != nil {
				t.Fatalf("independent source borrowed a synthetic accessor from its physical outer:\n%s", entry.source)
			}
			if z.sourceOwnership.bytes != 0 {
				t.Fatal("refused source committed ownership")
			}
		})
	}
}

func TestNativeIndependentLeafAccessUsesOriginalFlagsAndInstructions(t *testing.T) {
	plain := nativeCompileClasses(t, nativeIndependentLeafChainFixture)
	privateFixture := strings.Replace(nativeIndependentLeafChainFixture, "class LeafSupport{static abstract class Base<T>", "class LeafSupport{private static Object identity(Object token){return token;}static abstract class Base<T>", 1)
	privateFixture = strings.Replace(privateFixture, "Base(T token){super(token);}", "Base(T token){super((T)LeafSupport.identity(token));}", 1)
	private := nativeCompileClasses(t, privateFixture)
	for _, variant := range []string{"ordinary call", "synthetic invocation", "renamed synthetic invocation", "unused synthetic reference", "ordinary nonsynthetic binary method", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			input := plain
			if variant == "synthetic invocation" || variant == "renamed synthetic invocation" || variant == "ordinary nonsynthetic binary method" || variant == "unused synthetic reference" {
				input = private
			}
			for n, b := range input {
				files[n] = append([]byte(nil), b...)
			}
			if variant == "unused synthetic reference" {
				o, e := Parse(plain["LeafSupport$Base.class"])
				if e != nil {
					t.Fatal(e)
				}
				cp := NewConstantPoolWithConstant(&o.ConstantPool)
				cp.AddNewMethodInfo("LeafSupport", "access$000", "(Ljava/lang/Object;)Ljava/lang/Object;")
				files["LeafSupport$Base.class"] = o.Bytes()
			}
			if variant == "renamed synthetic invocation" {
				for _, name := range []string{"LeafSupport.class", "LeafSupport$Base.class"} {
					o, e := Parse(files[name])
					if e != nil {
						t.Fatal(e)
					}
					for _, c := range o.ConstantPool {
						if u, ok := c.(*ConstantUtf8Info); ok && u.Value == "access$000" {
							u.Value = "unconventionalBridge"
						}
					}
					files[name] = o.Bytes()
				}
			}
			if variant == "ordinary nonsynthetic binary method" {
				o, e := Parse(files["LeafSupport.class"])
				if e != nil {
					t.Fatal(e)
				}
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "access$000" {
						m.AccessFlags &^= 0x1000
					}
				}
				files["LeafSupport.class"] = o.Bytes()
			}
			nativeIndependentLeafChainInput(t, files)
			z := nativeArchive(t, files)
			defer z.Close()
			obj, e := Parse(files["LeafSupport$Base.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(obj)
			cert := d.originalNativeMemberIndependentRoot()
			if cert == nil {
				t.Fatal("original root proof")
			}
			prepared := z.prepareNativeMemberFamilyFromRoot(obj, nil, cert)
			if prepared == nil {
				t.Fatal("original local family")
			}
			switch variant {
			case "budget":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				prepared.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "ordinary call" || variant == "unused synthetic reference" || variant == "ordinary nonsynthetic binary method"
			if got := z.nativeMemberIndependentInvocationsClosed(prepared); got != want {
				t.Fatalf("actual foreign synthetic invocation closure %v want %v", got, want)
			}
			if z.sourceOwnership.bytes != 0 {
				t.Fatal("read-only access query committed source")
			}
		})
	}
}
