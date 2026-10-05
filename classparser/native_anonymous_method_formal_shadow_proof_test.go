package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousMethodFormalShadowSignatureClosure(t *testing.T) {
	for _, row := range []struct {
		name, signature string
		outer           []string
		shadow, closed  bool
	}{
		{"same declaration spelling", "<T:Ljava/lang/CharSequence;>(TT;)TT;", []string{"T"}, true, true},
		{"different method declaration", "<U:Ljava/lang/CharSequence;>(TU;)TU;", []string{"T"}, false, true},
		{"outer reference is not method declaration", "<U:TT;>(TU;)TT;", []string{"T"}, false, true},
		{"non generic method", "(TT;)TT;", []string{"T"}, false, true},
		{"literal dollar", "<T$Scope:Ljava/lang/Object;>()TT$Scope;", []string{"T$Scope"}, true, true},
		{"missing signature", "", []string{"T"}, false, false},
		{"malformed", "<T:Ljava/lang/Object;>(", []string{"T"}, false, false},
		{"duplicate declaration", "<T:Ljava/lang/Object;T:Ljava/lang/Object;>()V", []string{"T"}, false, false},
		{"class signature cannot license method", "<T:Ljava/lang/Object;>Ljava/lang/Object;", []string{"T"}, false, false},
		{"depth bound", "(" + strings.Repeat("[", 129) + "TT;)V", []string{"T"}, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c := nativeAnonymousSignatureLexicalShadow(row.signature, &class_context.ClassContext{TypeParams: row.outer}, nil)
			if s != row.shadow || c != row.closed {
				t.Fatalf("shadow=%v closed=%v", s, c)
			}
		})
	}
	for _, mode := range []string{"budget", "canceled", "nil lexical"} {
		t.Run(mode, func(t *testing.T) {
			lexical := &class_context.ClassContext{TypeParams: []string{"T"}}
			var b *workbudget.Budget
			switch mode {
			case "budget":
				b = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				b = workbudget.New(ctx, workbudget.Limits{})
			case "nil lexical":
				lexical = nil
			}
			if s, c := nativeAnonymousSignatureLexicalShadow("<T:Ljava/lang/Object;>()V", lexical, b); s || c {
				t.Fatalf("unsafe closure shadow=%v closed=%v", s, c)
			}
		})
	}
}

func TestNativeAnonymousMethodFormalShadowUsesOriginalMethodIdentity(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousMethodFormalShadow)
	for _, mode := range []string{"original", "optional hint empty", "optional hint wrong", "missing signature", "duplicate signature", "nil signature", "bad pool", "wrong method name", "wrong method descriptor", "missing method", "missing outer", "budget", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			obj, e := Parse(files["FormalShadowOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			var method *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "echo" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original echo absent")
			}
			c := &ClassObjectDumper{obj: obj, CurrentMethod: method, FuncCtx: &class_context.ClassContext{FunctionName: "echo", CurrentMethodDesc: "(Ljava/lang/CharSequence;)Ljava/lang/CharSequence;"}, nativeOuterContext: &class_context.ClassContext{TypeParams: []string{"T"}}}
			switch mode {
			case "optional hint wrong":
				c.FuncCtx.CurrentMethodSig = "<Other:Ljava/lang/Object;>()V"
			case "missing signature":
				method.Attributes = nil
			case "duplicate signature":
				for _, a := range method.Attributes {
					if s, ok := a.(*SignatureAttribute); ok {
						method.Attributes = append(method.Attributes, s)
						break
					}
				}
			case "nil signature":
				method.Attributes = []AttributeInfo{(*SignatureAttribute)(nil)}
			case "bad pool":
				method.Attributes = []AttributeInfo{&SignatureAttribute{SignatureIndex: 65535}}
			case "wrong method name":
				c.FuncCtx.FunctionName = "unchecked"
			case "wrong method descriptor":
				c.FuncCtx.CurrentMethodDesc = "()Ljava/lang/Object;"
			case "missing method":
				c.CurrentMethod = nil
			case "missing outer":
				c.nativeOuterContext = nil
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			shadow, closed := nativeAnonymousMethodLexicalShadow(c)
			wantShadow := mode == "original" || mode == "optional hint empty" || mode == "optional hint wrong"
			wantClosed := wantShadow || mode == "missing signature"
			if shadow != wantShadow || closed != wantClosed {
				t.Fatalf("shadow=%v closed=%v", shadow, closed)
			}
		})
	}
}
