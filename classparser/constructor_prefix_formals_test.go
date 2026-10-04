package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorPrefixFormalScopeRejectsPartialEvidence(t *testing.T) {
	valid := "<T:Ljava/lang/Number;:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;"
	for _, sig := range []string{valid, "<T::Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;", "<A:Ljava/lang/Object;B:Ljava/util/List<TA;>;>Ljava/lang/Object;"} {
		names, scope, ok := constructorFormalScope(sig)
		if !ok || len(names) == 0 || len(scope) != len(names) {
			t.Fatalf("valid scope rejected %s", sig)
		}
	}
	for _, sig := range []string{"", "<T:>Ljava/lang/Object;", "<T:TT;>Ljava/lang/Object;", "<T:TU;U:TT;>Ljava/lang/Object;", "<T:Ljava/lang/Object;T:Ljava/lang/Object;>Ljava/lang/Object;", "<class:Ljava/lang/Object;>Ljava/lang/Object;", "<T:Ljava/lang/Comparable<TU;>;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>Ljava/lang/Object;garbage", "<T:Ljava/lang/Object;>Ljava/lang/Object<TU;>;", "<T:[I>Ljava/lang/Object;", "<T:Ljava/util/List<I>;>Ljava/lang/Object;", "<T:Ljava/util/List<>;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>I", strings.Repeat("<", 33) + strings.Repeat(">", 33), strings.Repeat("x", 4097)} {
		if _, _, ok := constructorFormalScope(sig); ok {
			t.Fatalf("partial/foreign/budget scope certified: %q", sig)
		}
	}
	scope := map[string]string{"T": "java.lang.Number"}
	for _, sig := range []string{"(TT;[[TT;Ljava/util/List<+TT;>;I)V", "(TT;)V^Ljava/io/IOException;"} {
		if !constructorClosedMethodSignature(sig, scope) {
			t.Fatalf("valid method signature %s", sig)
		}
	}
	for _, sig := range []string{"(TT;)Vgarbage", "(TU;)V", "(TT;)V^TU;", "(Ljava/util/List<TU;>;)V", "(V)V", "(TT;)I", "(TT;)V^[TT;", "(TT;)V^", "(TT;", "<T:Ljava/lang/Object;>(TT;)V"} {
		if constructorClosedMethodSignature(sig, scope) {
			t.Fatalf("partial/foreign method certified: %s", sig)
		}
	}
	for dim := 1; dim < 5; dim++ {
		tpe := types.NewJavaClass("T")
		for i := 0; i < dim; i++ {
			tpe = types.NewJavaArrayType(tpe)
		}
		if got, ok := constructorFormalTypeErasure(tpe, scope, 128); !ok || got != strings.Repeat("[", dim)+"Ljava/lang/Number;" {
			t.Fatalf("array dimension %d: %s %v", dim, got, ok)
		}
	}
	cycle := types.NewJavaArrayType(types.NewJavaClass("T"))
	cycle.RawType().(*types.JavaArrayType).JavaType = cycle
	if _, ok := constructorFormalTypeErasure(cycle, scope, 128); ok {
		t.Fatal("cyclic type certified")
	}
}

func TestAdversarialConstructorPrefixFormalBindingRequiresOriginalScopeAndTarget(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "FormalConsumer.java")
	source := `class FormalParent<T>{FormalParent(T first,int mode){}}
public class FormalConsumer<T extends Number & Comparable<T>> extends FormalParent<T>{public FormalConsumer(T first,Object marker,int mode){super(first,mode);}}`
	if e := os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); e != nil {
		t.Fatalf("original %v\n%s", e, out)
	}
	raw := readClassBytes(t, dir, "FormalConsumer")
	for _, variant := range []string{"complete", "missing class signature", "wrong class signature tag", "duplicate class signature", "foreign parent", "injected formal", "missing formal", "method formal shadow", "missing constructor signature", "duplicate constructor signature", "missing constructor signature tag", "wrong parameter erasure", "changed source parameter", "first operand call", "copied first ref", "missing ID", "missing owner", "wrong owner", "incomplete family", "missing exact target", "duplicate target", "competing constructor", "varargs family", "different arity overload"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			dumper := NewClassObjectDumper(obj)
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			var classAttr, methodAttr *SignatureAttribute
			var method *MemberInfo
			for _, a := range obj.Attributes {
				if sig, ok := a.(*SignatureAttribute); ok {
					classAttr = sig
				}
			}
			for _, m := range obj.Methods {
				if pool.GetUtf8(int(m.NameIndex)).Value == "<init>" {
					method = m
					for _, a := range m.Attributes {
						if sig, ok := a.(*SignatureAttribute); ok {
							methodAttr = sig
						}
					}
				}
			}
			if classAttr == nil || methodAttr == nil || method == nil {
				t.Fatal("original signature absent")
			}
			cs := pool.GetUtf8(int(classAttr.SignatureIndex)).Value
			ms := pool.GetUtf8(int(methodAttr.SignatureIndex)).Value
			_, paramTypes, _ := types.ParseMethodSignatureFull(ms, nil)
			ctx := &class_context.ClassContext{ClassName: "FormalConsumer", ClassSig: cs, ClassTypeParams: []string{"T"}, TypeParams: []string{"T"}}
			owner := callbinding.Class{Name: "FormalParent", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: "(Ljava/lang/Object;I)V"}}}
			ctx.InvocationMetadata = func(string) (callbinding.Class, bool) { return owner, variant != "missing owner" }
			dumper.FuncCtx = ctx
			p := &constructorSourceBoundary{delegate: &values.FunctionCallExpression{ClassName: "FormalParent", Descriptor: "(Ljava/lang/Object;I)V"}}
			for _, typ := range paramTypes {
				p.params = append(p.params, values.NewJavaRef(utils.NewRootVariableId(), nil, typ))
			}
			p.delegate.Arguments = []values.JavaValue{p.params[0], p.params[2]}
			switch variant {
			case "missing class signature":
				obj.Attributes = nil
			case "wrong class signature tag":
				classAttr.SignatureIndex = obj.ThisClass
			case "duplicate class signature":
				obj.Attributes = append(obj.Attributes, classAttr)
			case "foreign parent":
				pool.GetUtf8(int(classAttr.SignatureIndex)).Value = strings.Replace(cs, "FormalParent", "ForeignParent", 1)
				ctx.ClassSig = pool.GetUtf8(int(classAttr.SignatureIndex)).Value
			case "injected formal":
				ctx.TypeParams = append(ctx.TypeParams, "U")
			case "missing formal":
				ctx.ClassTypeParams = nil
			case "method formal shadow":
				pool.GetUtf8(int(methodAttr.SignatureIndex)).Value = "<T:Ljava/lang/Object;>" + ms
				ctx.TypeParams = append(ctx.TypeParams, "T")
			case "missing constructor signature":
				method.Attributes = nil
			case "duplicate constructor signature":
				method.Attributes = append(method.Attributes, methodAttr)
			case "missing constructor signature tag":
				methodAttr.SignatureIndex = obj.ThisClass
			case "wrong parameter erasure":
				pool.GetUtf8(int(method.DescriptorIndex)).Value = "(Ljava/lang/Object;Ljava/lang/Object;I)V"
			case "changed source parameter":
				p.params[0].(*values.JavaRef).ResetVarType(types.NewJavaClass("java.lang.String"))
			case "first operand call":
				p.delegate.Arguments[0] = &values.FunctionCallExpression{}
			case "copied first ref":
				copy := *p.params[0].(*values.JavaRef)
				p.delegate.Arguments[0] = &copy
			case "missing ID":
				p.params[0].(*values.JavaRef).Id = nil
			case "wrong owner":
				owner.Name = "OtherParent"
			case "incomplete family":
				owner.MembersComplete = false
			case "missing exact target":
				owner.Methods = nil
			case "duplicate target":
				owner.Methods = append(owner.Methods, owner.Methods[0])
			case "competing constructor":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "<init>", Desc: "(Ljava/lang/Number;I)V"})
			case "varargs family":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "<init>", Desc: "([Ljava/lang/Object;)V", Varargs: true})
			case "different arity overload":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "<init>", Desc: "()V"})
			}
			header, names, params, result, ok := dumper.constructorPrefixFormalBinding(p, method)
			want := variant == "complete" || variant == "different arity overload"
			if ok != want {
				t.Fatalf("binding=%v want%v", ok, want)
			}
			if ok && (header != "<T extends Number & Comparable<T>> " || len(names) != 1 || len(params) != 3 || result.String(ctx) != "T") {
				t.Fatalf("lost original scope %q %v %v", header, names, result)
			}
		})
	}
}
