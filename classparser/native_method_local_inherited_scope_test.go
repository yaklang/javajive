package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeMethodLocalInheritedBinderScopeIdentity(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class PhysicalBinderOwner<T extends Number>{class Level<U extends CharSequence>{class Inner{Object make(final T seed,final U token){class Entry{T number(){return seed;}U text(){return token;}}return new Entry();}}}}`, "none")
	for _, variant := range []string{"original", "missing root identity", "missing ancestor", "cached ancestor owner", "cached ancestor flags", "cached static role", "original static boundary", "foreign method declaration", "wrong method name", "wrong method descriptor", "static declaring method", "missing root formal", "changed original bound", "free original method variable", "duplicate original signature", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, e := Parse(files["PhysicalBinderOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("actual family")
			}
			local := p.methodLocals["PhysicalBinderOwner$Level$Inner$1Entry"]
			if local == nil {
				t.Fatal("actual local")
			}
			enclosing := p.lexicalObjects[local.owner.owner]
			level := p.children["PhysicalBinderOwner$Level"]
			replaceSignature := func(obj *ClassObject, attrs []AttributeInfo, text string) {
				for _, a := range attrs {
					if s, ok := a.(*SignatureAttribute); ok {
						obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: text})
						s.SignatureIndex = uint16(len(obj.ConstantPool))
						return
					}
				}
				t.Fatal("actual Signature")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing root identity":
				delete(p.lexicalObjects, p.owner)
			case "missing ancestor":
				delete(p.lexicalObjects, level.object.GetClassName())
			case "cached ancestor owner":
				level.owner = "Other"
			case "cached ancestor flags":
				level.flags ^= 8
			case "cached static role":
				level.static = !level.static
			case "original static boundary":
				for _, a := range level.object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, known := sourceBridgeClassName(level.object, row.InnerClassInfoIndex)
							if known && n == level.object.GetClassName() {
								row.InnerClassAccessFlags |= 8
							}
						}
					}
				}
				level.flags |= 8
				level.static = true
			case "foreign method declaration":
				copy := *local.owner.declaration
				local.owner.declaration = &copy
			case "wrong method name":
				local.owner.method = "other"
			case "wrong method descriptor":
				local.owner.descriptor = "()Ljava/lang/Object;"
			case "static declaring method":
				local.owner.declaration.AccessFlags |= 8
			case "missing root formal":
				attrs := []AttributeInfo{}
				for _, a := range root.Attributes {
					if _, sig := a.(*SignatureAttribute); !sig {
						attrs = append(attrs, a)
					}
				}
				root.Attributes = attrs
			case "changed original bound":
				replaceSignature(root, root.Attributes, "<T:Ljava/lang/String;>Ljava/lang/Object;")
			case "free original method variable":
				replaceSignature(enclosing, local.owner.declaration.Attributes, "(TX;TU;)Ljava/lang/Object;")
			case "duplicate original signature":
				for _, a := range root.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						root.Attributes = append(root.Attributes, a)
						break
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, known := nativeMethodLocalScopeSignature(local.object, enclosing, local.owner, work, p)
			if known != (variant == "original") {
				t.Fatalf("scope admitted=%v", known)
			}
		})
	}
}

func TestNativeMethodLocalInheritedBinderEnvironmentBudget(t *testing.T) {
	classes := []string{}
	for frame := 0; frame < 32; frame++ {
		var b strings.Builder
		b.WriteByte('<')
		for index := 0; index < 16; index++ {
			fmt.Fprintf(&b, "V%d:Ljava/lang/Number;", frame*16+index)
		}
		b.WriteString(">Ljava/lang/Object;")
		classes = append(classes, b.String())
	}
	for _, variant := range []string{"unlimited", "adequate", "derived allocation", "derived work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			var work *workbudget.Budget
			switch variant {
			case "adequate":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1 << 20, MaxRequestWork: 1 << 20})
			case "derived allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 32 << 10})
			case "derived work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 100000})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			known := nativeMethodLocalBindingBudget(classes, "(TV0;TV511;)V", work)
			want := variant == "unlimited" || variant == "adequate"
			if known != want {
				t.Fatalf("binding budget allowed=%v err=%v", known, work)
			}
			if !want && work.Check() == nil {
				t.Fatal("canonical budget/cancel error missing")
			}
		})
	}
}
