package javaclassparser

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialOrdinaryConstructorThrowsRequiresIndependentFormals(t *testing.T) {
	const loose = "<E:Ljava/lang/Throwable;>(ZLjava/lang/Throwable;)V^TE;"
	for _, test := range []struct {
		name, signature, descriptor string
		erased, want, scopes        []string
		mutate                      func(map[string]callbinding.Class, string)
	}{
		{name: "throws only Throwable", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "throws only Exception", signature: "<Witness:Ljava/lang/Exception;>()V^TWitness;", descriptor: "()V", erased: []string{"java/lang/Exception"}, want: []string{"java/lang/RuntimeException"}},
		{name: "concrete checked throw remains", signature: "<E:Ljava/lang/Exception;>()V^TE;^Ljava/io/IOException;", descriptor: "()V", erased: []string{"java/lang/Exception", "java/io/IOException"}, want: []string{"java/lang/RuntimeException", "java/io/IOException"}},
		{name: "two independent throws variables", signature: "<E:Ljava/lang/Throwable;F:Ljava/lang/Exception;>()V^TE;^TF;", descriptor: "()V", erased: []string{"java/lang/Throwable", "java/lang/Exception"}, want: []string{"java/lang/RuntimeException", "java/lang/RuntimeException"}},
		{name: "method variable shadows class variable", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", scopes: []string{"<E:Ljava/io/IOException;>Ljava/lang/Object;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "argument lower bound cannot become unchecked", signature: "<E:Ljava/lang/Throwable;>(TE;)V^TE;", descriptor: "(Ljava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}},
		{name: "array argument constrains the variable", signature: "<E:Ljava/lang/Throwable;>([TE;)V^TE;", descriptor: "([Ljava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}},
		{name: "nested parameter constrains the variable", signature: "<E:Ljava/lang/Throwable;>(Ljava/util/List<TE;>;)V^TE;", descriptor: "(Ljava/util/List;)V", erased: []string{"java/lang/Throwable"}},
		{name: "narrow checked bound cannot become unchecked", signature: "<E:Ljava/io/IOException;>()V^TE;", descriptor: "()V", erased: []string{"java/io/IOException"}},
		{name: "intersection needs a separate proof", signature: "<E:Ljava/lang/Exception;:Lp/Marker;>()V^TE;", descriptor: "()V", erased: []string{"java/lang/Exception"}},
		{name: "dependent bound needs a separate proof", signature: "<E:Ljava/lang/Throwable;F:TE;>()V^TF;", descriptor: "()V", erased: []string{"java/lang/Throwable"}},
		{name: "unused method formal is not this domain", signature: "<E:Ljava/lang/Exception;>()V^Ljava/io/IOException;", descriptor: "()V", erased: []string{"java/io/IOException"}},
		{name: "class formal is not method inference", signature: "<X:Ljava/lang/Throwable;>()V^TE;", descriptor: "()V", scopes: []string{"<E:Ljava/lang/Exception;>Ljava/lang/Object;"}, erased: []string{"java/lang/Exception"}},
		{name: "Exceptions and Signature must agree", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Exception"}},
		{name: "descriptor must agree", signature: loose, descriptor: "(ILjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}},
		{name: "truncated Signature", signature: "<E:Ljava/lang/Throwable;>(Z", descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}},
		{name: "Signature bound", signature: strings.Repeat("x", 8193), descriptor: "()V"},
		{name: "unknown Exceptions", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) {
			c := m[o]
			c.Methods[0].ExceptionsKnown = false
			m[o] = c
		}},
		{name: "incomplete member table", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.MembersComplete = false; m[o] = c }},
		{name: "incomplete parent table", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.ParentsComplete = false; m[o] = c }},
		{name: "wrong owner identity", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.Name = "p/Other"; m[o] = c }},
		{name: "interface cannot be constructed", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.IsInterface = true; m[o] = c }},
		{name: "duplicate constructor", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) {
			c := m[o]
			c.Methods = append(c.Methods, c.Methods[0])
			m[o] = c
		}},
		{name: "overload has no independent selection proof", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) {
			c := m[o]
			c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "(I)V", ExceptionsKnown: true})
			m[o] = c
		}},
		{name: "varargs source selection unknown", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.Methods[0].Varargs = true; m[o] = c }},
		{name: "static constructor invalid", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, o string) { c := m[o]; c.Methods[0].Static = true; m[o] = c }},
		{name: "unknown hierarchy", signature: loose, descriptor: "(ZLjava/lang/Throwable;)V", erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class, _ string) { delete(m, "java/lang/RuntimeException") }},
	} {
		for _, owner := range []string{"p/Allocate", "q/AnotherAllocation"} {
			t.Run(test.name+"/"+owner, func(t *testing.T) {
				metadata := constructorThrowsBindingMetadata()
				metadata[owner] = callbinding.Class{Name: owner, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: "<init>", Desc: test.descriptor, Signature: test.signature, Generic: true, ExceptionsKnown: true, Exceptions: append([]string(nil), test.erased...)}}}
				if test.mutate != nil {
					test.mutate(metadata, owner)
				}
				before := append([]string(nil), metadata[owner].Methods[0].Exceptions...)
				got, known := ordinaryConstructorThrowsOnly(func(name string) (callbinding.Class, bool) { c, ok := metadata[name]; return c, ok }, owner, test.descriptor, test.scopes, nil)
				if known != (test.want != nil) || known && !reflect.DeepEqual(got, test.want) {
					t.Fatalf("throws %v known=%v want=%v", got, known, test.want)
				}
				if !reflect.DeepEqual(before, metadata[owner].Methods[0].Exceptions) {
					t.Fatal("inference changed original Exceptions metadata")
				}
			})
		}
	}
}

func TestAdversarialOrdinaryConstructorThrowsInferenceRespectsSharedBudget(t *testing.T) {
	metadata := constructorThrowsBindingMetadata()
	metadata["p/New"] = callbinding.Class{Name: "p/New", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: "()V", Signature: "<E:Ljava/lang/Throwable;>()V^TE;", Generic: true, ExceptionsKnown: true, Exceptions: []string{"java/lang/Throwable"}}}}
	provider := func(name string) (callbinding.Class, bool) { c, ok := metadata[name]; return c, ok }
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if canceled {
			cancel()
		}
		work := workbudget.New(ctx, workbudget.Limits{MaxRequestWork: 1})
		if _, known := ordinaryConstructorThrowsOnly(provider, "p/New", "()V", nil, work); known || work.Err() == nil {
			t.Fatal("unavailable work granted inference")
		}
		cancel()
	}
}
