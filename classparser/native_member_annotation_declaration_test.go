package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"testing"
)

func TestNativeMemberAnnotationRequiresOriginalElementDomain(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberAnnotationFixture)
	cases := []string{"original", "missing static", "missing annotation flag", "kind mismatch", "final declaration", "wrong superclass", "missing Annotation interface", "extra interface", "generic declaration", "constructor", "code on element", "static element", "default interface method", "element parameters", "void element", "ordinary reference element", "multidimensional element", "direct cycle", "indirect cycle", "Object public override", "Object protected override", "Annotation override", "keyword element", "duplicate element", "throws element", "generic element", "constrained Class signature", "duplicate signature", "duplicate default", "mismatched default tag", "invalid boolean", "invalid byte", "invalid short", "invalid char", "noncanonical float NaN", "noncanonical double NaN", "wrong enum type", "missing enum constant", "wrong nested type", "unknown nested element", "missing nested required element", "missing dependency", "wrong dependency identity", "budget", "canceled"}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			obj, e := Parse(files["NamedAnnOwner$Spec.class"])
			if e != nil {
				t.Fatal(e)
			}
			root, e := Parse(files["NamedAnnOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			reader := NewClassObjectDumper(root)
			reader.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			graph := map[string]*ClassObject{obj.GetClassName(): obj}
			metadata := reader.nativeAnnotationDeclarationResolver()
			resolve := func(n string) (*ClassObject, bool) {
				if n == "NamedAnnOwner$Meta" && scenario == "missing dependency" {
					return nil, false
				}
				if n == "NamedAnnOwner$Meta" && scenario == "wrong dependency identity" {
					return root, true
				}
				if o := graph[n]; o != nil {
					return o, true
				}
				o, ok := metadata(n)
				if ok {
					graph[n] = o
				}
				return o, ok
			}
			find := func(o *ClassObject, n string) *MemberInfo {
				for _, m := range o.Methods {
					v, _ := sourceBridgeUTF8(o, m.NameIndex)
					if v == n {
						return m
					}
				}
				t.Fatalf("missing witness %s", n)
				return nil
			}
			changeName := func(m *MemberInfo, n string) { m.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(n)) }
			changeType := func(m *MemberInfo, n string) { m.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(n)) }
			defaultOf := func(m *MemberInfo) *AnnotationDefaultAttribute {
				for _, a := range m.Attributes {
					if v, ok := a.(*AnnotationDefaultAttribute); ok {
						return v
					}
				}
				t.Fatal("missing default")
				return nil
			}
			count := find(obj, "count")
			flags := uint16(0x2609)
			var work *workbudget.Budget
			switch scenario {
			case "missing static":
				flags &^= 8
			case "missing annotation flag":
				flags &^= 0x2000
			case "kind mismatch":
				obj.AccessFlags &^= 0x2000
			case "final declaration":
				obj.AccessFlags |= 0x10
			case "wrong superclass":
				obj.SuperClass = obj.ThisClass
			case "missing Annotation interface":
				obj.Interfaces = nil
			case "extra interface":
				obj.Interfaces = append(obj.Interfaces, obj.SuperClass)
			case "generic declaration":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;Ljava/lang/annotation/Annotation;"))})
			case "constructor":
				changeName(count, "<init>")
			case "code on element":
				count.Attributes = append(count.Attributes, &CodeAttribute{})
			case "static element":
				count.AccessFlags |= 8
			case "default interface method":
				count.AccessFlags = 1
				count.Attributes = append(count.Attributes, &CodeAttribute{})
			case "element parameters":
				changeType(count, "(I)I")
			case "void element":
				changeType(count, "()V")
			case "ordinary reference element":
				changeType(count, "()Ljava/lang/Object;")
			case "multidimensional element":
				changeType(count, "()[[I")
			case "direct cycle":
				changeType(count, "()LNamedAnnOwner$Spec;")
				count.Attributes = nil
			case "indirect cycle":
				meta, _ := resolve("NamedAnnOwner$Meta")
				m := find(meta, "value")
				m.DescriptorIndex = uint16(meta.ConstantPoolManager.AddUtf8Info("()LNamedAnnOwner$Spec;"))
				m.Attributes = nil
			case "Object public override":
				changeName(count, "hashCode")
			case "Object protected override":
				changeName(count, "clone")
			case "Annotation override":
				changeName(count, "annotationType")
			case "keyword element":
				changeName(count, "switch")
			case "duplicate element":
				changeName(count, "enabled")
			case "throws element":
				count.Attributes = append(count.Attributes, &ExceptionsAttribute{})
			case "generic element":
				count.Attributes = append(count.Attributes, &SignatureAttribute{SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>()I"))})
			case "constrained Class signature":
				m := find(obj, "type")
				for _, a := range m.Attributes {
					if sig, ok := a.(*SignatureAttribute); ok {
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("()Ljava/lang/Class<+Ljava/lang/Number;>;"))
					}
				}
			case "duplicate signature":
				m := find(obj, "type")
				for _, a := range m.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						m.Attributes = append(m.Attributes, a)
						break
					}
				}
			case "duplicate default":
				count.Attributes = append(count.Attributes, defaultOf(count))
			case "mismatched default tag":
				defaultOf(count).DefaultValue.Tag = 'J'
				defaultOf(count).DefaultValue.Value = &ConstantLongInfo{Value: -7}
			case "invalid boolean":
				defaultOf(find(obj, "enabled")).DefaultValue.Value = &ConstantIntegerInfo{Value: 2}
			case "invalid byte":
				defaultOf(find(obj, "tiny")).DefaultValue.Value = &ConstantIntegerInfo{Value: 128}
			case "invalid short":
				defaultOf(find(obj, "shorty")).DefaultValue.Value = &ConstantIntegerInfo{Value: 32768}
			case "invalid char":
				defaultOf(find(obj, "character")).DefaultValue.Value = &ConstantIntegerInfo{Value: -1}
			case "noncanonical float NaN":
				defaultOf(find(obj, "nan32")).DefaultValue.Value = &ConstantFloatInfo{Value: math.Float32frombits(0x7fc00001)}
			case "noncanonical double NaN":
				defaultOf(find(obj, "nan64")).DefaultValue.Value = &ConstantDoubleInfo{Value: math.Float64frombits(0x7ff8000000000001)}
			case "wrong enum type":
				defaultOf(find(obj, "choice")).DefaultValue.Value.(*EnumConstValue).TypeName = "Ljava/lang/annotation/RetentionPolicy;"
			case "missing enum constant":
				defaultOf(find(obj, "choice")).DefaultValue.Value.(*EnumConstValue).ConstName = "MISSING"
			case "wrong nested type":
				defaultOf(find(obj, "nested")).DefaultValue.Value.(*AnnotationAttribute).TypeName = "LNamedAnnOwner$Spec;"
			case "unknown nested element":
				defaultOf(find(obj, "nested")).DefaultValue.Value.(*AnnotationAttribute).ElementValuePairs = []*ElementValuePairAttribute{{Name: "missing", Tag: 'I', Value: &ConstantIntegerInfo{Value: 1}}}
			case "missing nested required element":
				meta, _ := resolve("NamedAnnOwner$Meta")
				find(meta, "value").Attributes = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 3})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberDeclarationKindRepresentable(obj, flags, work, resolve); got != (scenario == "original") {
				t.Fatalf("original annotation declaration proof=%v", got)
			}
		})
	}
}
