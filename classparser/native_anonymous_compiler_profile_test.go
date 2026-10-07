package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// These are metadata refusal models. The adjacent round-trip bank uses actual
// native javac8 originals and an actual selected compiler; models do not count
// as executed Java positives or as additional toolchain coverage.
func TestNativeAnonymousCompilerProfileRequiresExactOriginalMethodAndMetadata(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"CompilerModel.java": `class CompilerModel{static CompilerModelParent make(final long word){return new CompilerModelParent(word){long get(){return word;}};}}class CompilerModelParent{CompilerModelParent(long word){}long get(){return 0;}}`}, "none", "8")
	for _, variant := range []string{"legacy model", "default compiler", "modern compiler", "modern flags with legacy self row", "unknown compiler", "wrong source", "wrong major", "preview", "wrong class flags", "wrong self flags", "duplicate self", "wrong owner identity", "own method not static", "instance without enclosing capture", "missing method", "duplicate method", "missing enclosing", "constructor exceptions", "work budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			owner, _ := Parse(append([]byte(nil), files["CompilerModel.class"]...))
			obj, _ := Parse(append([]byte(nil), files["CompilerModel$1.class"]...))
			obj.AccessFlags = 0x0030
			var self *InnerClassesAttribute
			for _, attribute := range obj.Attributes {
				if table, ok := attribute.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						if name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex); known && name == obj.GetClassName() {
							row.InnerClassAccessFlags = 8
							self = table
						}
					}
				}
			}
			reader := NewClassObjectDumper(owner)
			reader.options.TargetSourceVersion, reader.options.SourceCompiler = 8, NativeJavac8
			reader.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			method := "make(J)LCompilerModelParent;"
			switch variant {
			case "default compiler":
				reader.options.SourceCompiler = ""
			case "modern compiler":
				reader.options.SourceCompiler = ModernJavac
			case "modern flags with legacy self row":
				reader.options.SourceCompiler = ModernJavac
				obj.AccessFlags = 0x0020
			case "unknown compiler":
				reader.options.SourceCompiler = "future"
			case "wrong source":
				reader.options.TargetSourceVersion = 11
			case "wrong major":
				obj.MajorVersion = 51
			case "preview":
				obj.MinorVersion = 65535
			case "wrong class flags":
				obj.AccessFlags = 0x0020
			case "wrong self flags":
				self.Classes[0].InnerClassAccessFlags = 0
			case "duplicate self":
				self.Classes = append(self.Classes, self.Classes[0])
			case "wrong owner identity":
				method = "other(J)LCompilerModelParent;"
			case "own method not static", "instance without enclosing capture", "missing method", "duplicate method":
				for i, declaration := range owner.Methods {
					name, _ := sourceBridgeUTF8(owner, declaration.NameIndex)
					if name == "make" {
						if variant == "own method not static" || variant == "instance without enclosing capture" {
							declaration.AccessFlags &^= 8
							if variant == "instance without enclosing capture" {
								obj.AccessFlags, self.Classes[0].InnerClassAccessFlags = 0x0020, 0
							}
						} else if variant == "missing method" {
							owner.Methods = append(owner.Methods[:i], owner.Methods[i+1:]...)
						} else {
							owner.Methods = append(owner.Methods, declaration)
						}
						break
					}
				}
			case "missing enclosing":
				for i, attribute := range obj.Attributes {
					if raw, ok := attribute.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						obj.Attributes = append(obj.Attributes[:i], obj.Attributes[i+1:]...)
						break
					}
				}
			case "constructor exceptions":
				for _, declaration := range obj.Methods {
					if name, _ := sourceBridgeUTF8(obj, declaration.NameIndex); name == "<init>" {
						declaration.Attributes = append(declaration.Attributes, &ExceptionsAttribute{})
					}
				}
			case "work budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			packet := reader.nativeAnonymousConstructorForCompiler(obj, "CompilerModel", method, "", nil, nil, reader.buildInvocationMetadata(), nil)
			if (packet != nil) != (variant == "legacy model") {
				t.Fatal("compiler choice cannot replace original ownership/staticness/metadata evidence")
			}
		})
	}
}

func TestSourceCompilerProfileAdmissionAndImmutableArchiveViews(t *testing.T) {
	for _, row := range []struct {
		profile SourceCompilerProfile
		source  int
		valid   bool
	}{{"", 0, true}, {ModernJavac, 8, true}, {ModernJavac, 21, true}, {NativeJavac8, 8, true}, {NativeJavac8, 0, false}, {NativeJavac8, 11, false}, {"future", 8, false}} {
		if got := row.profile.validate(row.source) == nil; got != row.valid {
			t.Fatalf("profile=%s source=%d valid=%t", row.profile, row.source, got)
		}
		if !row.valid {
			result, err := DecompileWithOptions(nil, DecompileOptions{SourceCompiler: row.profile, TargetSourceVersion: row.source})
			if err == nil || result.Source != "" {
				t.Fatal("invalid profile entered decompilation")
			}
		}
	}
	archive := nativeArchive(t, map[string][]byte{"note.txt": []byte("immutable")})
	defer archive.Close()
	archive.targetSourceVersion, archive.sourceCompiler = 8, NativeJavac8
	view := archive.sourceReleaseView(11)
	if view.sourceCompiler != NativeJavac8 || view.targetSourceVersion != 8 || view.sourceOwnership != archive.sourceOwnership {
		t.Fatal("Multi-Release selection changed source compiler identity")
	}
	reader := view.nativeMemberReader(nil)
	if reader.options.SourceCompiler != NativeJavac8 || reader.options.TargetSourceVersion != 8 {
		t.Fatal("archive reader lost immutable compiler profile")
	}
	if config := effectiveConfigOf(copyDecompileOptions(reader.options)); config.SourceCompiler != NativeJavac8 {
		t.Fatal("effective request omitted compiler identity")
	}
}

// Enumerate every class access-flag word. This independent compiler truth table
// covers reserved bits and invalid flag combinations; it is not a Java-program
// generator or a behavioral acceptance count.
func TestNativeAnonymousCompilerProfileEnumeratesEveryClassFlagWord(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"FlagModel.java": `class FlagModel{static Object make(final long word){return new Object(){long get(){return word;}};}}`}, "none", "8")
	for _, profile := range []SourceCompilerProfile{"", ModernJavac, NativeJavac8} {
		owner, _ := Parse(append([]byte(nil), files["FlagModel.class"]...))
		obj, _ := Parse(append([]byte(nil), files["FlagModel$1.class"]...))
		reader := NewClassObjectDumper(owner)
		reader.options.TargetSourceVersion, reader.options.SourceCompiler = 8, profile
		reader.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
		metadata := reader.buildInvocationMetadata()
		expected := uint16(0x0020)
		for _, attribute := range obj.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				for _, row := range table.Classes {
					if name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex); known && name == obj.GetClassName() && profile == NativeJavac8 {
						row.InnerClassAccessFlags = 8
						expected = 0x0030
					}
				}
			}
		}
		for flags := 0; flags < 1<<16; flags++ {
			obj.AccessFlags = uint16(flags)
			packet := reader.nativeAnonymousConstructorForCompiler(obj, "FlagModel", "make(J)Ljava/lang/Object;", "", nil, nil, metadata, nil)
			if (packet != nil) != (obj.AccessFlags == expected) {
				t.Fatalf("profile=%s flags=%04x expected-only=%04x", profile, flags, expected)
			}
		}
	}
}
