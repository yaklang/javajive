package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeEnumConstructorSignatureRequiresOriginalErasureAndHiddenSlots(t *testing.T) {
	files := nativeCompileClasses(t, `enum SignatureGuard{LEFT(java.util.Arrays.asList("a"));SignatureGuard(java.util.List<String> input){}static void other(java.util.List<String> input){}}`)
	for _, v := range []string{"original", "nested wildcard", "own formal", "same named class", "wrong erasure", "missing formal", "malformed generic", "trailing input", "bad source return", "missing source parameter", "extra source parameter", "wrong hidden name type", "wrong ordinal type", "not enum", "not Enum superclass", "other method", "synthetic constructor", "public constructor", "wrong requested descriptor", "canceled", "budget", "oversized"} {
		t.Run(v, func(t *testing.T) {
			o, e := Parse(append([]byte(nil), files["SignatureGuard.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(o)
			var method *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				if n == "<init>" && m.AccessFlags&0x1000 == 0 {
					method = m
				}
			}
			if method == nil {
				t.Fatal("constructor")
			}
			desc, _ := sourceBridgeUTF8(o, method.DescriptorIndex)
			sig := "(Ljava/util/List<Ljava/lang/String;>;)V"
			pool := NewConstantPoolWithConstant(&o.ConstantPool)
			setDesc := func(s string) { desc = s; method.DescriptorIndex = uint16(pool.AddUtf8Info(s)) }
			switch v {
			case "nested wildcard":
				sig = "(Ljava/util/List<Ljava/util/Map<Ljava/lang/String;+Ljava/lang/Number;>;>;)V"
			case "own formal":
				sig = "<T:Ljava/lang/Object;>(Ljava/util/List<TT;>;)V"
			case "same named class":
				sig = "(Ljava/util/List<LT;>;)V"
			case "wrong erasure":
				sig = "(Ljava/util/Collection<Ljava/lang/String;>;)V"
			case "missing formal":
				sig = "(Ljava/util/List<TT;>;)V"
			case "malformed generic":
				sig = "(Ljava/util/List<I>;)V"
			case "trailing input":
				sig += "x"
			case "bad source return":
				sig = "(Ljava/util/List<Ljava/lang/String;>;)I"
			case "missing source parameter":
				sig = "()V"
			case "extra source parameter":
				sig = "(Ljava/util/List<Ljava/lang/String;>;J)V"
			case "wrong hidden name type":
				setDesc("(Ljava/lang/Object;ILjava/util/List;)V")
			case "wrong ordinal type":
				setDesc("(Ljava/lang/String;JLjava/util/List;)V")
			case "not enum":
				o.AccessFlags &^= 0x4000
			case "not Enum superclass":
				o.SuperClass = uint16(pool.AddNewClassInfo("java/lang/Object"))
			case "other method":
				method.NameIndex = uint16(pool.AddUtf8Info("other"))
			case "synthetic constructor":
				method.AccessFlags |= 0x1000
			case "public constructor":
				method.AccessFlags = 1
			case "wrong requested descriptor":
				desc = "(Ljava/lang/String;I)V"
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "oversized":
				sig = strings.Repeat("[", 4097) + "I"
			}
			want := v == "original" || v == "nested wildcard" || v == "own formal" || v == "same named class"
			if got := d.nativeEnumConstructorSignatureMatches(method, sig, desc); got != want {
				t.Fatalf("admitted=%v want=%v descriptor=%s signature=%s", got, want, desc, sig)
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource error")
			}
		})
	}
}
