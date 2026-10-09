package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAnonymousForeignStoreRequiresWritableOriginalDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, foreignInitializerStoreFixture)
	for _, change := range []string{"original", "public cross-package", "inaccessible public-field owner", "volatile", "private", "static", "final", "synthetic", "constant", "protected cross-package", "package-private cross-package", "unknown owner", "wrong owner identity", "wrong declaration descriptor", "wrong target descriptor", "wrong source descriptor", "wrong member", "duplicate declaration", "unsafe member", "reference target", "array target", "wrong receiver type", "missing receiver type", "nil receiver", "nil target", "nil member", "nil child", "nil resolver", "budget", "memory", "canceled"} {
		t.Run(change, func(t *testing.T) {
			object, e := Parse(files["ForeignStoreOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			cell, e := Parse(files["ForeignStoreCell.class"])
			if e != nil {
				t.Fatal(e)
			}
			var declaration *MemberInfo
			for _, f := range cell.Fields {
				n, _ := sourceBridgeUTF8(cell, f.NameIndex)
				if n == "length" {
					declaration = f
				}
			}
			if declaration == nil {
				t.Fatal("original field declaration")
			}
			child := &nativeAnonymousClass{object: object}
			receiver := values.NewJavaRef(nil, nil, types.NewJavaClass(cell.GetClassName()))
			field := &values.JavaClassMember{Name: cell.GetClassName(), Member: "length", Description: "I"}
			target := values.NewRefMember(receiver, "length", types.NewJavaPrimer(types.JavaInteger))
			var work *workbudget.Budget
			resolve := func(n string) (*ClassObject, bool) { return cell, n == field.Name }
			switch change {
			case "public cross-package", "inaccessible public-field owner", "protected cross-package", "package-private cross-package":
				object.ConstantPool[object.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, object, "other/ForeignChild")
				if change == "public cross-package" || change == "inaccessible public-field owner" {
					declaration.AccessFlags = 1
					if change == "public cross-package" {
						cell.AccessFlags |= 1
					}
				} else if change == "protected cross-package" {
					declaration.AccessFlags = 4
				}
			case "volatile":
				declaration.AccessFlags |= 0x40
			case "private":
				declaration.AccessFlags = 2
			case "static":
				declaration.AccessFlags = 8
			case "final":
				declaration.AccessFlags = 0x10
			case "synthetic":
				declaration.AccessFlags = 0x1000
			case "constant":
				declaration.Attributes = append(declaration.Attributes, &ConstantValueAttribute{})
			case "unknown owner":
				resolve = func(string) (*ClassObject, bool) { return nil, false }
			case "wrong owner identity":
				cell.ThisClass = cell.SuperClass
			case "wrong declaration descriptor":
				declaration.DescriptorIndex = sourceBridgePoolString(t, cell, "J")
			case "wrong target descriptor":
				target.JavaType = types.NewJavaPrimer(types.JavaLong)
			case "wrong source descriptor":
				field.Description = "J"
			case "wrong member":
				target.Member = "other"
			case "duplicate declaration":
				cell.Fields = append(cell.Fields, declaration)
			case "unsafe member":
				field.Member = "a-b"
				target.Member = field.Member
			case "reference target":
				field.Description = "Ljava/lang/Object;"
				target.JavaType = types.NewJavaClass("java.lang.Object")
			case "array target":
				field.Description = "[I"
				target.JavaType = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
			case "wrong receiver type":
				receiver.ResetVarType(types.NewJavaClass("Other"))
			case "missing receiver type":
				receiver.ResetVarType(nil)
			case "nil receiver":
				target.Object = nil
			case "nil target":
				target = nil
			case "nil member":
				field = nil
			case "nil child":
				child = nil
			case "nil resolver":
				resolve = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousInitializerForeignStoreField(child, field, target, resolve, work); got != (change == "original" || change == "public cross-package" || change == "volatile") {
				t.Fatalf("write permission=%v", got)
			}
		})
	}
}

// PUTFIELD itself truncates a JVM computational int into B/C/S. Losing the
// original conversion can still pass the verifier, but must not license a Java
// source assignment without a representability/conversion proof.
func TestNativeAnonymousForeignStoreRequiresOriginalNarrowProducer(t *testing.T) {
	_, java := t04Tools(t)
	for _, kind := range []string{"byte", "short", "char"} {
		t.Run(kind, func(t *testing.T) {
			source := strings.ReplaceAll(foreignInitializerStoreFixture, "int get()", kind+" get()")
			source = strings.Replace(source, "int length=17", kind+" length=17", 1)
			source = strings.Replace(source, "target.length=ForeignStoreEffects.rhs(number)", "target.length=("+kind+")ForeignStoreEffects.rhs(number)", 1)
			source = strings.ReplaceAll(source, "value.get()!=n", "value.get()!=("+kind+")n")
			source = strings.ReplaceAll(source, "cell.length!=n", "cell.length!=("+kind+")n")
			files := nativeCompileClasses(t, source)
			obj, err := Parse(files["ForeignStoreOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			if child := nativeAnonymousConstructor(obj, "ForeignStoreOwner", "make", nil); child == nil || child.expressionInitializer == nil {
				t.Fatal("original narrow producer not admitted")
			}
			changed := 0
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "<init>" {
					continue
				}
				for _, attr := range method.Attributes {
					code, ok := attr.(*CodeAttribute)
					if !ok {
						continue
					}
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if err := d.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					for _, op := range constructorMotionOps(d) {
						if op.Instr.OpCode == core.OP_I2B || op.Instr.OpCode == core.OP_I2S || op.Instr.OpCode == core.OP_I2C {
							code.Code[op.CurrentOffset] = byte(core.OP_NOP)
							changed++
						}
					}
				}
			}
			if changed != 1 {
				t.Fatalf("narrow producer replacements=%d", changed)
			}
			files["ForeignStoreOwner$1.class"] = obj.Bytes()
			out := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(out, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, out, "ForeignStoreDriver"); got != "24:foreign:initializer:store\n" {
				t.Fatalf("valid original JVM truncation=%q", got)
			}
			if nativeAnonymousConstructor(obj, "ForeignStoreOwner", "make", nil) != nil {
				t.Fatal("unknown computational int borrowed source narrow conversion")
			}
		})
	}
}
