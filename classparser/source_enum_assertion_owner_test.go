package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const enumAssertionOwnerFixture = `class PortOwner {
 enum Mode { ONE { public long apply(long x) { assert x != 9 : "original"; return x+1; } }; public abstract long apply(long x); }
}
class AssertionOwnerDriver {
 public static void main(String[]args) {
  // Status is cached by the constant subclass at its initialization, even
  // when the loader changes the outer class's default afterwards.
  PortOwner.Mode m=PortOwner.Mode.ONE;
  boolean enabled=PortOwner.class.desiredAssertionStatus();
  PortOwner.class.getClassLoader().setClassAssertionStatus("PortOwner",!enabled);
  boolean thrown=false;
  try { if(m.apply(9)!=10)throw new IllegalStateException("value"); }
  catch(AssertionError e) { thrown=true;if(!"original".equals(e.getMessage()))throw e; }
  if(thrown!=enabled)throw new IllegalStateException("lexical cached status");
  if(m.apply(Long.MAX_VALUE)!=Long.MIN_VALUE)throw new IllegalStateException("overflow");
  System.out.println("cached:lexical:overflow");
 }
}`

func enumAssertionInitializer(t *testing.T, object *ClassObject) *MemberInfo {
	t.Helper()
	for _, m := range object.Methods {
		if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "<clinit>" {
			return m
		}
	}
	t.Fatal("fixture lacks initializer")
	return nil
}

func TestAdversarialFlatEnumAssertionLiteralWidthsKeepFreshCodeAndCachedStatus(t *testing.T) {
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			originals, fresh, a := enumFlatTestInputs(t, debug, enumAssertionOwnerFixture)
			const name = "PortOwner$Mode$1.class"
			// Compile and run the independent original driver before any completion.
			originalFiles := map[string][]byte{}
			for n, b := range originals {
				originalFiles[n] = b
			}
			for _, n := range []string{"PortOwner", "AssertionOwnerDriver"} {
				b, ok := a.source.jar.declarationResolver(n)
				if !ok {
					t.Fatal("external fixture", n)
				}
				originalFiles[n+".class"] = b
			}
			run := func(t *testing.T, dir string) {
				t.Helper()
				for _, mode := range []string{"-ea", "-da", "-ea:PortOwner", "-ea:PortOwner$Mode"} {
					out, err := enumFreshOracleRun(t, java, mode, dir, "AssertionOwnerDriver")
					if err != nil || string(out) != "cached:lexical:overflow\n" {
						t.Fatal("cached lexical oracle", mode, err, string(out))
					}
				}
			}
			run(t, sourceArtifactWriteFiles(t, originalFiles))
			for _, width := range []string{"narrow", "widen", "already-wide"} {
				t.Run(width, func(t *testing.T) {
					obj := mustParseLowering(t, bytes.Clone(fresh[name]))
					init := enumAssertionInitializer(t, obj)
					code := constructorLoweringCode(init)
					if code.Code[0] != core.OP_LDC {
						t.Fatal("narrow fixture required")
					}
					if width != "narrow" {
						for len(obj.ConstantPool) < 270 {
							obj.ConstantPoolManager.AddUtf8Info(fmt.Sprintf("unused-padding-%d", len(obj.ConstantPool)))
						}
					}
					if width == "already-wide" {
						// Independently widen the fixture, retaining its verifier frames.
						// The first frame moves one byte; later frame deltas are unchanged.
						old := code.Code[1]
						code.Code = append([]byte{core.OP_LDC_W, 0, old}, code.Code[2:]...)
						var frames *UnparsedAttribute
						for _, attr := range code.Attributes {
							if u, ok := attr.(*UnparsedAttribute); ok && u.Name == "StackMapTable" {
								frames = u
							}
						}
						if frames == nil || len(frames.Info) < 3 || frames.Info[2] >= 63 {
							t.Fatal("fixture requires a compact initial same_frame")
						}
						frames.Info[2]++
						code.Attributes = []AttributeInfo{frames}
						enumPortFixCodeLength(t, obj, code)
					}
					input := obj.Bytes()
					before := bytes.Clone(input)
					done, err := a.source.jar.sourceConstructorLowering.restoreEnumAssertionOwner(originals[name], input, nil)
					if err != nil || bytes.Equal(done, input) {
						t.Fatal("literal restoration", err)
					}
					if !bytes.Equal(input, before) {
						t.Fatal("input mutated")
					}
					repaired := mustParseLowering(t, done)
					afterCode := constructorLoweringCode(enumAssertionInitializer(t, repaired))
					wantWidth := 2
					if width != "narrow" {
						wantWidth = 3
					}
					beforeWidth := 2
					if width == "already-wide" {
						beforeWidth = 3
					}
					if len(afterCode.Code) != len(code.Code)+wantWidth-beforeWidth || !bytes.Equal(afterCode.Code[wantWidth:], code.Code[beforeWidth:]) {
						t.Fatal("fresh branch/cache suffix changed")
					}
					if wantWidth == 3 && afterCode.Code[0] != core.OP_LDC_W || wantWidth == 2 && afterCode.Code[0] != core.OP_LDC {
						t.Fatal("wrong literal width")
					}
					for i, m := range obj.Methods {
						if m != init {
							left, right := constructorLoweringCode(m), constructorLoweringCode(repaired.Methods[i])
							if left != nil && !bytes.Equal(left.Code, right.Code) {
								t.Fatal("unrelated fresh executable changed")
							}
						}
					}
					// Existing constants are immutable: consumers elsewhere may share them.
					oldReader, _ := NewClassReaderWithBudget(input, nil)
					newReader, _ := NewClassReaderWithBudget(done, nil)
					oldLayout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
					newLayout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
					if _, err := parseWithMetadataLayout(oldReader, nil, oldLayout); err != nil {
						t.Fatal(err)
					}
					if _, err := parseWithMetadataLayout(newReader, nil, newLayout); err != nil {
						t.Fatal(err)
					}
					for i, p := range oldLayout.constants {
						q := newLayout.constants[i]
						if !bytes.Equal(input[p.start:p.end], done[q.start:q.end]) {
							t.Fatal("shared constant changed")
						}
					}
					again, err := a.source.jar.sourceConstructorLowering.restoreEnumAssertionOwner(originals[name], done, nil)
					if err != nil || !bytes.Equal(done, again) {
						t.Fatal("non-idempotent", err)
					}
					files := map[string][]byte{"PortOwner.class": originalFiles["PortOwner.class"], "AssertionOwnerDriver.class": originalFiles["AssertionOwnerDriver.class"]}
					for n, b := range fresh {
						if n == name {
							b = input
						}
						complete, err := a.FinishClass(n, b)
						if err != nil {
							t.Fatal("public completion", n, err)
						}
						files[n] = complete
					}
					run(t, sourceArtifactWriteFiles(t, files))
				})
			}
		})
	}
}

func TestAdversarialFlatEnumAssertionOwnerRejectsUnprovedCacheEdits(t *testing.T) {
	originals, fresh, a := enumFlatTestInputs(t, "source,lines,vars", enumAssertionOwnerFixture)
	const name = "PortOwner$Mode$1.class"
	for _, variant := range []string{"wrong status", "extra effect", "wrong cache flags", "wrong branch", "extra write", "field handle", "opaque PC table", "frame into operand", "stale original", "unreturned source", "pending source", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh[name]))
			init := enumAssertionInitializer(t, obj)
			code := constructorLoweringCode(init)
			authority := a.source.jar.sourceConstructorLowering
			original := originals[name]
			var work *workbudget.Budget
			switch variant {
			case "wrong status":
				index := obj.ConstantPoolManager.AddNewClassInfo("Unrelated")
				if index > 255 {
					t.Fatal("small fixture")
				}
				code.Code[1] = byte(index)
			case "extra effect":
				code.Code = append(code.Code[:len(code.Code)-1], core.OP_NOP, core.OP_RETURN)
			case "wrong cache flags":
				for _, f := range obj.Fields {
					if n, _ := sourceBridgeUTF8(obj, f.NameIndex); n == nativeAssertionField {
						f.AccessFlags &^= 0x10
					}
				}
			case "wrong branch":
				code.Code[6]++
			case "extra write":
				for _, m := range obj.Methods {
					if m != init {
						c := constructorLoweringCode(m)
						if c != nil && len(c.Code) > 3 && c.Code[0] == core.OP_GETSTATIC {
							c.Code[0] = core.OP_PUTSTATIC
							break
						}
					}
				}
			case "field handle":
				for i, c := range obj.ConstantPool {
					if f, ok := c.(*ConstantFieldrefInfo); ok {
						nt := obj.ConstantPool[f.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						n, _ := sourceBridgeUTF8(obj, nt.NameIndex)
						if n == nativeAssertionField {
							obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 2, ReferenceIndex: uint16(i + 1)})
							break
						}
					}
				}
			case "opaque PC table":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "OpaquePCs", Length: 2, Info: []byte{0, 0}})
			case "frame into operand":
				for _, attr := range code.Attributes {
					if u, ok := attr.(*UnparsedAttribute); ok && u.Name == "StackMapTable" {
						u.Info = []byte{0, 1, 1}
						u.Length = 3
					}
				}
			case "stale original":
				old := mustParseLowering(t, bytes.Clone(original))
				old.ConstantPoolManager.AddUtf8Info("changed original")
				original = old.Bytes()
			case "unreturned source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "pending source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, enumPending: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": {}}}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			enumPortFixCodeLength(t, obj, code)
			input := obj.Bytes()
			before := bytes.Clone(input)
			if _, err := Parse(input); err != nil {
				t.Fatal("structural fixture", err)
			}
			result, err := authority.restoreEnumAssertionOwner(original, input, work)
			bypass := variant == "pending source" || variant == "unreturned source"
			if bypass {
				if err != nil || !bytes.Equal(result, input) {
					t.Fatal("unowned source edited", err)
				}
			} else if err == nil || result != nil {
				t.Fatal("unproved edit accepted", variant, err)
			}
			if !bytes.Equal(before, input) {
				t.Fatal("input mutated")
			}
		})
	}
}
