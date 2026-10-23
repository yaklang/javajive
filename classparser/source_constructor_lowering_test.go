package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func loweringAuthorityTestInputs(t *testing.T) ([]byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	compiler, _ := t04Tools(t)
	classes := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, constructorLoweringFixture, "LoweringChild", "none", false)
	original := constructorLoweringOriginal(t, classes)
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"LoweringChild.class": original["LoweringChild.class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(binary string) ([]byte, bool) { raw, ok := original[binary+".class"]; return raw, ok })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	source, err := a.ReadFile("LoweringChild.class")
	if err != nil || legacySourceHasFailureMarker(string(source)) || a.source.jar.sourceConstructorLowering.active["LoweringChild"] == nil {
		t.Fatalf("complete real source publication: %v\n%s", err, source)
	}
	return original["LoweringChild.class"], classes["LoweringChild.class"], a
}

func TestSourceConstructorLoweringRejectsFreshPacketAndPCMutation(t *testing.T) {
	original, compiled, a := loweringAuthorityTestInputs(t)
	for _, kind := range []string{"control", "parameter slot", "wide category", "field identity", "field descriptor", "delegation", "extra instruction", "handler", "frame", "unknown Code annotation", "line operand PC", "local interior", "max stack", "max locals", "method flags", "missing constructor", "class identity"} {
		t.Run(kind, func(t *testing.T) {
			obj := mustParseLowering(t, compiled)
			var ctor *MemberInfo
			for _, method := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if n == "<init>" {
					ctor = method
				}
			}
			code := constructorLoweringCode(ctor)
			switch kind {
			case "parameter slot":
				code.Code[7] = 0x2b // token's aload_3 becomes input's aload_1
			case "wide category":
				code.Code[12] = 0x18 // lload becomes dload at the same word slot
			case "field identity":
				obj.Fields[0].NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("unrelated"))
			case "field descriptor":
				obj.Fields[0].DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("Ljava/lang/String;"))
			case "delegation":
				code.Code[2] = 0x1b // mode becomes an invalid reference argument slot
			case "extra instruction":
				code.Code = append(bytes.Clone(code.Code[:35]), 0x00, 0xb1)
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 6, HandlerPc: 35}}
			case "frame":
				obj.ConstantPoolManager.AddUtf8Info("StackMapTable")
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "StackMapTable", Length: 3, Info: []byte{0, 1, 0}})
			case "unknown Code annotation":
				obj.ConstantPoolManager.AddUtf8Info("RuntimeVisibleTypeAnnotations")
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "line operand PC":
				obj.ConstantPoolManager.AddUtf8Info("LineNumberTable")
				code.Attributes = append(code.Attributes, &LineNumberTableAttribute{AttrLen: 6, LineNumberTable: []*LineNumberTableEntry{{StartPc: 4, LineNumber: 17}}})
			case "local interior":
				obj.ConstantPoolManager.AddUtf8Info("LocalVariableTable")
				data := make([]byte, 12)
				for i, v := range []uint16{1, 1, 35, uint16(obj.ConstantPoolManager.AddUtf8Info("mode")), uint16(obj.ConstantPoolManager.AddUtf8Info("I")), 2} {
					binary.BigEndian.PutUint16(data[2*i:], v)
				}
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "LocalVariableTable", Length: uint32(len(data)), Info: data})
			case "max stack":
				code.MaxStack = 0
			case "max locals":
				code.MaxLocals = 4
			case "method flags":
				ctor.AccessFlags = 4
			case "missing constructor":
				ctor.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("ordinary"))
			case "class identity":
				obj.ThisClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("Unrelated"))
			}
			input := obj.Bytes()
			hash, originalHash := sha256.Sum256(input), sha256.Sum256(original)
			got, err := a.source.jar.sourceConstructorLowering.lower(original, input, nil)
			if kind == "control" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("positive lowering control", err)
				}
			} else if err == nil || got != nil {
				t.Fatalf("unproved fresh packet accepted: %v", err)
			}
			if sha256.Sum256(input) != hash || sha256.Sum256(original) != originalHash {
				t.Fatal("input mutated, including on refusal")
			}
		})
	}
}

func TestSourceConstructorLoweringRequiresOriginalAndReturnedSourceAuthority(t *testing.T) {
	original, compiled, a := loweringAuthorityTestInputs(t)
	authority := a.source.jar.sourceConstructorLowering
	publication := authority.active["LoweringChild"]
	for _, kind := range []string{"no publication", "pending only", "stale original", "changed source", "different namespace", "canceled", "work budget", "output budget"} {
		t.Run(kind, func(t *testing.T) {
			issuer := &sourceConstructorLoweringAuthority{jar: authority.jar, active: map[string]*sourceConstructorLoweringPublication{"LoweringChild": publication}}
			input := bytes.Clone(original)
			var work *workbudget.Budget
			switch kind {
			case "no publication":
				issuer.active = nil
			case "pending only":
				issuer.active = nil
				issuer.pending = map[string]*sourceConstructorLoweringPublication{"LoweringChild": publication}
			case "stale original":
				obj := mustParseLowering(t, input)
				obj.AccessFlags ^= 0x0010
				input = obj.Bytes()
			case "changed source":
				issuer.pending = map[string]*sourceConstructorLoweringPublication{"LoweringChild": publication}
				if err := issuer.sourceReturned("LoweringChild", []byte("changed body")); err == nil {
					t.Fatal("changed source was published")
				}
				return
			case "different namespace":
				issuer.active = map[string]*sourceConstructorLoweringPublication{"Foreign": publication}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "work budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			before, fresh := sha256.Sum256(input), sha256.Sum256(compiled)
			got, err := issuer.lower(input, compiled, work)
			if err == nil || got != nil {
				t.Fatal("missing original/source/resource proof was accepted", err)
			}
			if sha256.Sum256(input) != before || sha256.Sum256(compiled) != fresh {
				t.Fatal("authority check mutated inputs")
			}
		})
	}
}

func TestSourceConstructorLoweringCannotTurnOrdinaryStorageIntoCaptures(t *testing.T) {
	compiler, _ := t04Tools(t)
	classes := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, constructorLoweringFixture, "LoweringChild", "none", false)
	obj := mustParseLowering(t, constructorLoweringOriginal(t, classes)["LoweringChild.class"])
	for _, kind := range []string{"ordinary", "mutable", "static", "volatile", "foreign owner", "computed argument", "extra side effect"} {
		t.Run(kind, func(t *testing.T) {
			candidate := mustParseLowering(t, obj.Bytes())
			var method *MemberInfo
			for _, m := range candidate.Methods {
				name, _ := sourceBridgeUTF8(candidate, m.NameIndex)
				if name == "<init>" {
					method = m
				}
			}
			code := constructorLoweringCode(method)
			switch kind {
			case "ordinary":
				candidate.Fields[0].AccessFlags &^= syntheticFieldFlag
			case "mutable":
				candidate.Fields[0].AccessFlags &^= 0x10
			case "static":
				candidate.Fields[0].AccessFlags |= 8
			case "volatile":
				candidate.Fields[0].AccessFlags |= 0x40
			case "foreign owner":
				for _, constant := range candidate.ConstantPool {
					if field, ok := constant.(*ConstantFieldrefInfo); ok {
						field.ClassIndex = candidate.SuperClass
					}
				}
			case "computed argument":
				code.Code[31] = 0x04 // mode parameter becomes iconst_1
			case "extra side effect":
				code.Code = append(bytes.Clone(code.Code[:35]), 0x00, 0xb1)
			}
			if constructorLoweringPacketFor(candidate, method, true, true, nil) != nil {
				t.Fatal("original identity obligation was relaxed", kind)
			}
		})
	}
}

func TestSourceConstructorLoweringBindsPhysicalReleaseSourceIndependently(t *testing.T) {
	original, compiled, base := loweringAuthorityTestInputs(t)
	// Identical executable semantics, distinct physical original identity. An
	// unused constant changes no instruction or ABI and is not copied back.
	versioned := mustParseLowering(t, bytes.Clone(original))
	versioned.ConstantPoolManager.AddUtf8Info("independent physical namespace")
	jar := filepath.Join(t.TempDir(), "namespaces.jar")
	entries := map[string][]byte{
		"META-INF/MANIFEST.MF":                     []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n"),
		"LoweringChild.class":                      original,
		"META-INF/versions/9/LoweringChild.class":  versioned.Bytes(),
		"META-INF/versions/11/LoweringChild.class": original,
	}
	if err := os.WriteFile(jar, t23Zip(t, entries), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, base.source.jar.declarationResolver)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.ReadFile("LoweringChild.class"); err != nil {
		t.Fatal(err)
	}
	const path = "META-INF/versions/9/LoweringChild.class"
	if got, err := a.FinishClass(path, compiled); err == nil || got != nil {
		t.Fatal("base source publication escaped into a physical namespace", err)
	}
	source, err := a.ReadFile(path)
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatalf("own physical source publication: %v\n%s", err, source)
	}
	got, err := a.FinishClass(path, compiled)
	if err != nil {
		t.Fatal("own namespace must close independently", err)
	}
	control, err := a.FinishClass("LoweringChild.class", compiled)
	if err != nil || !bytes.Equal(got, control) {
		t.Fatal("unused original pool constant affected fresh executable output", err)
	}
	view := a.source.jar.sourceReleaseView(9)
	if view.sourceConstructorLowering.active["LoweringChild"] == nil || view.sourceConstructorLowering.active["LoweringChild"] == a.source.jar.sourceConstructorLowering.active["LoweringChild"] {
		t.Fatal("physical source obligation was inherited instead of proved")
	}
	// The distinct implicit-source case can borrow the already returned base
	// receipt when original bytes and compiler/source tuple are all identical.
	// It must not enroll a fictitious publication in that physical namespace.
	const implicit = "META-INF/versions/11/LoweringChild.class"
	unchanged, err := a.FinishClass(implicit, compiled)
	if err != nil || !bytes.Equal(unchanged, control) {
		t.Fatal("identical published base source could not close its implicit output", err)
	}
	implicitView := a.source.jar.sourceReleaseView(11)
	if implicitView.sourceConstructorLowering.active["LoweringChild"] != nil {
		t.Fatal("implicit compilation fabricated an own-source publication")
	}
	for _, variant := range []string{"compiler", "source level"} {
		t.Run(variant, func(t *testing.T) {
			priorCompiler, priorSource := implicitView.sourceCompiler, implicitView.targetSourceVersion
			if variant == "compiler" {
				implicitView.sourceCompiler = NativeJavac8
			} else {
				implicitView.targetSourceVersion = 11
			}
			got, err := a.FinishClass(implicit, compiled)
			implicitView.sourceCompiler, implicitView.targetSourceVersion = priorCompiler, priorSource
			if err == nil || got != nil {
				t.Fatal("compiler/source tuple mismatch borrowed a base lowering", err)
			}
		})
	}
}
