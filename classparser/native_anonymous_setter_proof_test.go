package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func sourceScopeSetterFiles(t *testing.T) map[string][]byte {
	return nativeCompileSourceReleaseClasses(t, map[string]string{"p/Base.java": `package p;public abstract class Base{protected Object payload;public Object get(){return payload;}public abstract Object put(Object next);}`, "q/ScopeWriteOwner.java": `package q;public class ScopeWriteOwner{public static p.Base build(){return new p.Base(){class Writer{Object put(Object next){return payload=next;}Object read(){return payload;}}public Object put(Object next){return new Writer().put(next);}};}}`}, "none", "8")
}

func sourceScopeSetterPlan(t *testing.T, files map[string][]byte) (*nativeAnonymousForest, *nativeMemberPrivateGetter, *ClassObject) {
	t.Helper()
	z := nativeArchive(t, files)
	t.Cleanup(func() { z.Close() })
	root, e := Parse(files["q/ScopeWriteOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := z.nativeMemberReader(root)
	p := d.planNativeMemberFamily()
	if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
		t.Fatal("original complete source forest")
	}
	f := p.anonymousForest
	var g *nativeMemberPrivateGetter
	for _, packet := range p.getters {
		if packet.setter {
			if g != nil {
				t.Fatal("ambiguous fixture")
			}
			g = packet
		}
	}
	if g == nil {
		t.Fatal("original setter packet")
	}
	caller := f.objects["q/ScopeWriteOwner$1$Writer"]
	if caller == nil {
		t.Fatal("original named caller")
	}
	return f, g, caller
}
func TestNativeAnonymousSetterSourceScopeRequiresOriginalNamedChain(t *testing.T) {
	files := sourceScopeSetterFiles(t)
	for _, variant := range []string{"original", "foreign caller", "missing object", "foreign lexical object", "missing child", "changed child identity", "changed owner", "changed name", "changed flags", "static child", "missing original ownership", "missing anonymous unit", "foreign anonymous unit", "foreign anonymous object", "missing group", "failed group", "foreign group forest", "foreign source forest", "failed source family", "nil forest", "nil member family", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			f, g, caller := sourceScopeSetterPlan(t, files)
			p := f.members
			child := p.children[caller.GetClassName()]
			anchor := g.owner
			var work *workbudget.Budget
			switch variant {
			case "foreign caller":
				x := *caller
				caller = &x
			case "missing object":
				delete(f.objects, caller.GetClassName())
			case "foreign lexical object":
				x := *caller
				p.lexicalObjects[caller.GetClassName()] = &x
			case "missing child":
				delete(p.children, caller.GetClassName())
			case "changed child identity":
				x := *caller
				child.object = &x
			case "changed owner":
				child.owner = "ScopeWriteOwner"
			case "changed name":
				child.name = "Foreign"
			case "changed flags":
				child.flags ^= 1
			case "static child":
				child.static = true
			case "missing original ownership":
				caller.Attributes = nil
			case "missing anonymous unit":
				delete(f.units, anchor)
			case "foreign anonymous unit":
				x := *f.units[anchor]
				f.units[anchor] = &x
			case "foreign anonymous object":
				x := *f.units[anchor].object
				f.units[anchor].object = &x
			case "missing group":
				delete(f.groups, p.anonymousUnits[anchor].owner)
			case "failed group":
				p.anonymousUnits[anchor].failed = true
			case "foreign group forest":
				p.anonymousUnits[anchor].forest = &nativeAnonymousForest{}
			case "foreign source forest":
				p.anonymousForest = &nativeAnonymousForest{}
			case "failed source family":
				p.failed = true
			case "nil forest":
				f = nil
			case "nil member family":
				f.members = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousConsumerSourceScope(f, caller, work); got != (variant == "original") {
				t.Fatalf("scope=%v", got)
			}
		})
	}
}
func TestNativeAnonymousSetterDeclarationReopensTypedWritePacket(t *testing.T) {
	files := sourceScopeSetterFiles(t)
	for _, variant := range []string{"original", "copied method", "copied owner", "wrong field", "wrong descriptor", "wrong owner", "wrong ordinal", "wrong packet kind", "static field", "generic field", "changed flags", "changed code", "wrong CP tag", "missing original method", "duplicate original method", "copied original table method", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			f, g, _ := sourceScopeSetterPlan(t, files)
			o := f.objects[g.owner]
			m := g.method
			var code *CodeAttribute
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
			if code == nil || len(code.Code) != 7 {
				t.Fatal("original instance write packet")
			}
			var work *workbudget.Budget
			switch variant {
			case "copied method":
				x := *m
				m = &x
			case "copied owner":
				x := *o
				o = &x
			case "wrong field":
				g.field = "other"
			case "wrong descriptor":
				g.fieldDescriptor = "Ljava/lang/String;"
			case "wrong owner":
				g.owner = "Foreign"
			case "wrong ordinal":
				g.ordinal++
			case "wrong packet kind":
				g.setter = false
			case "static field":
				g.staticField = true
			case "generic field":
				g.genericField = true
			case "changed flags":
				m.AccessFlags &^= 0x1000
			case "changed code":
				code.Code[2] = core.OP_DUP
			case "wrong CP tag":
				i := int(code.Code[4])<<8 | int(code.Code[5])
				ref := o.ConstantPool[i-1].(*ConstantFieldrefInfo)
				o.ConstantPool[i-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "missing original method":
				o.Methods = nil
			case "duplicate original method":
				o.Methods = append(o.Methods, m)
			case "copied original table method":
				for i, v := range o.Methods {
					if v == m {
						x := *v
						o.Methods[i] = &x
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousFieldAccessorDeclaration(f, o, m, work); got != (variant == "original") {
				t.Fatalf("declaration=%v", got)
			}
		})
	}
}

func TestNativeAnonymousFieldReadDeclarationRequiresPhysicalMethod(t *testing.T) {
	files := sourceScopeSetterFiles(t)
	for _, variant := range []string{"original", "removed method", "copied table method", "duplicate table method", "copied method", "changed code"} {
		t.Run(variant, func(t *testing.T) {
			f, setter, _ := sourceScopeSetterPlan(t, files)
			o := f.objects[setter.owner]
			var g *nativeMemberPrivateGetter
			for _, candidate := range f.members.getters {
				if candidate.owner == setter.owner && !candidate.setter && candidate.call == nil && candidate.update == nil {
					if g != nil {
						t.Fatal("ambiguous original getter")
					}
					g = candidate
				}
			}
			if g == nil {
				t.Fatal("original read packet")
			}
			m := g.method
			switch variant {
			case "removed method":
				for i, v := range o.Methods {
					if v == m {
						o.Methods = append(o.Methods[:i], o.Methods[i+1:]...)
						break
					}
				}
			case "copied table method":
				for i, v := range o.Methods {
					if v == m {
						x := *v
						o.Methods[i] = &x
					}
				}
			case "duplicate table method":
				o.Methods = append(o.Methods, m)
			case "copied method":
				x := *m
				m = &x
			case "changed code":
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						c.Code[0] = core.OP_ACONST_NULL
					}
				}
			}
			if got := nativeAnonymousFieldAccessorDeclaration(f, o, m, nil); got != (variant == "original") {
				t.Fatalf("original getter declaration=%v", got)
			}
		})
	}
}
