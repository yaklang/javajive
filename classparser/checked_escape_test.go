package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCheckedConstructorExceptionsAreNotInherited(t *testing.T) {
	provider := func(name string) (callbinding.Class, bool) {
		if name == "p/Child" {
			return callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true, Parents: []string{"p/Parent"}}, true
		}
		if name == "p/Parent" {
			return callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: "()V", ExceptionsKnown: true, Exceptions: []string{"java/io/IOException"}}, {Name: "read", Desc: "()I", ExceptionsKnown: true, Exceptions: []string{"java/io/IOException"}}}}, true
		}
		return callbinding.Class{}, false
	}
	if _, known := exactInvocationExceptions(provider, "p/Child", "<init>", "()V"); known {
		t.Fatal("constructor declaration inherited")
	}
	if got, known := exactInvocationExceptions(provider, "p/Parent", "<init>", "()V"); !known || len(got) != 1 || got[0] != "java/io/IOException" {
		t.Fatal("exact owner declaration lost")
	}
	if got, known := exactInvocationExceptions(provider, "p/Child", "read", "()I"); !known || len(got) != 1 || got[0] != "java/io/IOException" {
		t.Fatal("ordinary inherited declaration lost")
	}
}

func TestCheckedEscapeRejectsUncoveredConstructorDelegatePrefix(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "CheckedConstructorMetadata.java")
	source := `class MetadataParent {MetadataParent(int mode)throws java.io.IOException{}} class MetadataArgumentOps {static int value(int mode)throws java.io.IOException{return mode;}} public class CheckedConstructorMetadata extends MetadataParent {CheckedConstructorMetadata(int mode)throws java.io.IOException{super(MetadataArgumentOps.value(mode));}}`
	if err := os.WriteFile(file, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "CheckedConstructorMetadata.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, argumentChecked := range []bool{false, true} {
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		dumper := NewClassObjectDumper(object)
		dumper.FuncCtx = &class_context.ClassContext{InvocationMetadata: func(owner string) (callbinding.Class, bool) {
			x := callbinding.Method{ExceptionsKnown: true, Exceptions: []string{"java/io/IOException"}}
			switch owner {
			case "MetadataParent":
				x.Name = "<init>"
				x.Desc = "(I)V"
			case "MetadataArgumentOps":
				x.Name = "value"
				x.Desc = "(I)I"
				if !argumentChecked {
					x.Exceptions = nil
				}
			default:
				return callbinding.Class{}, false
			}
			return callbinding.Class{Name: owner, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{x}}, true
		}}
		method := object.Methods[0]
		var code *CodeAttribute
		var attrs []AttributeInfo
		for _, a := range method.Attributes {
			if c, ok := a.(*CodeAttribute); ok {
				code = c
			}
			if _, ok := a.(*ExceptionsAttribute); !ok {
				attrs = append(attrs, a)
			}
		}
		method.Attributes = attrs
		if code == nil {
			t.Fatal("missing constructor code")
		}
		decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(dumper.ConstantPool, index) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		delegate := -1
		for _, op := range decoder.Opcodes() {
			if op.Instr.OpCode == core.OP_INVOKESPECIAL {
				delegate = int(op.CurrentOffset)
			}
		}
		if delegate < 0 {
			t.Fatal("missing original constructor invoke PC")
		}
		body := []statements.Statement{&statements.ExpressionStatement{Expression: &values.FunctionCallExpression{Object: &values.JavaRef{IsThis: true}, Kind: values.InvokeSpecial, FunctionName: "<init>", HasOriginPC: true, OriginPC: delegate}}}
		needs, err := dumper.methodNeedsCheckedEscape(code, body, method)
		if needs || err == nil || !strings.Contains(err.Error(), "constructor delegation") {
			t.Fatalf("argument checked=%v: bridge=%v err=%v", argumentChecked, needs, err)
		}
	}
}

func TestCheckedEscapeProofUsesOriginalDeclarationAndHandlerCoverage(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "CheckedBoundaryMetadata.java")
	source := `class MetadataDeclaredOps {static void effect()throws java.io.IOException{}}
public class CheckedBoundaryMetadata {static void gate()throws java.io.IOException{MetadataDeclaredOps.effect();}static void absorbed(){try{MetadataDeclaredOps.effect();}catch(java.io.IOException error){}}}`
	if err := os.WriteFile(file, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "CheckedBoundaryMetadata.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		method string
		remove bool
		decl   callbinding.Method
		change func(*CodeAttribute, []statements.Statement)
		want   bool
	}{
		{"caller declares original exception", "gate", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, nil, false},
		{"caller declaration absent", "gate", true, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, nil, true},
		{"unknown declaration is not evidence", "gate", true, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}}, nil, false},
		{"different descriptor is not evidence", "gate", true, callbinding.Method{Name: "effect", Desc: "(I)V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, nil, false},
		{"original declaration has no exceptions", "gate", true, callbinding.Method{Name: "effect", Desc: "()V", ExceptionsKnown: true}, nil, false},
		{"unchecked runtime declaration", "gate", true, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/lang/RuntimeException"}, ExceptionsKnown: true}, nil, false},
		{"unchecked error declaration", "gate", true, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/lang/Error"}, ExceptionsKnown: true}, nil, false},
		{"typed handler absorbs original exception", "absorbed", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, nil, false},
		{"handler ends before invoke", "absorbed", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, func(code *CodeAttribute, body []statements.Statement) {
			code.ExceptionTable[0].EndPc = code.ExceptionTable[0].StartPc
		}, true},
		{"raw catchall is not suppression", "absorbed", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, func(code *CodeAttribute, body []statements.Statement) { code.ExceptionTable[0].CatchType = 0 }, true},
		{"wrong handler entry has no witness", "absorbed", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, func(code *CodeAttribute, body []statements.Statement) {
			body[0].(*statements.TryCatchStatement).Handlers[0].EntryPC++
		}, true},
		{"handler explicit rethrow escapes", "absorbed", false, callbinding.Method{Name: "effect", Desc: "()V", Exceptions: []string{"java/io/IOException"}, ExceptionsKnown: true}, func(code *CodeAttribute, body []statements.Statement) {
			body[0].(*statements.TryCatchStatement).CatchBodies[0] = []statements.Statement{&statements.CustomStatement{ThrownValue: values.JavaNull}}
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			object, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			dumper := NewClassObjectDumper(object)
			provider := func(owner string) (callbinding.Class, bool) {
				if owner == "MetadataDeclaredOps" {
					return callbinding.Class{Name: owner, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{tt.decl}}, true
				}
				return callbinding.Class{}, false
			}
			dumper.FuncCtx = &class_context.ClassContext{InvocationMetadata: provider}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range object.Methods {
				name, _ := object.getUtf8(m.NameIndex)
				if name == tt.method {
					method = m
					for _, attr := range m.Attributes {
						if ca, ok := attr.(*CodeAttribute); ok {
							code = ca
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("missing witness")
			}
			if tt.remove {
				var attrs []AttributeInfo
				for _, a := range method.Attributes {
					if _, ok := a.(*ExceptionsAttribute); !ok {
						attrs = append(attrs, a)
					}
				}
				method.Attributes = attrs
			}
			var body []statements.Statement
			if len(code.ExceptionTable) > 0 {
				entry := code.ExceptionTable[0]
				body = []statements.Statement{&statements.TryCatchStatement{Handlers: []statements.CatchHandler{{EntryPC: int(entry.HandlerPc)}}, CatchBodies: [][]statements.Statement{{statements.NewReturnStatement(nil)}}}}
			}
			if tt.change != nil {
				tt.change(code, body)
			}
			got, err := dumper.methodNeedsCheckedEscape(code, body, method)
			if err != nil || got != tt.want {
				t.Fatalf("escape=%v want%v err%v", got, tt.want, err)
			}
		})
	}
}

func TestCheckedEscapeDeclarationLookupRequiresCompleteUniqueMetadata(t *testing.T) {
	base := callbinding.Method{Name: "call", Desc: "()V", ExceptionsKnown: true, Exceptions: []string{"java/io/IOException"}}
	for _, tt := range []struct {
		name  string
		table map[string]callbinding.Class
		want  bool
	}{
		{"exact inherited declaration", map[string]callbinding.Class{"Child": {Name: "Child", MembersComplete: true, ParentsComplete: true, Parents: []string{"Parent"}}, "Parent": {Name: "Parent", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{base}}}, true},
		{"incomplete own members", map[string]callbinding.Class{"Child": {Name: "Child", ParentsComplete: true, Methods: []callbinding.Method{base}}}, false},
		{"unknown parent table", map[string]callbinding.Class{"Child": {Name: "Child", MembersComplete: true, ParentsComplete: true, Parents: []string{"Missing"}}}, false},
		{"incomplete parent names", map[string]callbinding.Class{"Child": {Name: "Child", MembersComplete: true, Parents: []string{"Parent"}}, "Parent": {Name: "Parent", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{base}}}, false},
		{"duplicate declaration", map[string]callbinding.Class{"Child": {Name: "Child", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{base, base}}}, false},
		{"cyclic unknown hierarchy", map[string]callbinding.Class{"Child": {Name: "Child", MembersComplete: true, ParentsComplete: true, Parents: []string{"Parent"}}, "Parent": {Name: "Parent", MembersComplete: true, ParentsComplete: true, Parents: []string{"Child"}}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			exceptions, known := exactInvocationExceptions(func(name string) (callbinding.Class, bool) { c, ok := tt.table[name]; return c, ok }, "Child", "call", "()V")
			if known != tt.want || (known && (len(exceptions) != 1 || exceptions[0] != "java/io/IOException")) {
				t.Fatalf("exceptions=%v known%v", exceptions, known)
			}
		})
	}
}

func TestCheckedEscapeSyntheticNamesReserveOriginalAndScopedIdentifiers(t *testing.T) {
	object := &ClassObject{ConstantPool: []ConstantInfo{&ConstantUtf8Info{Value: "jdec$escape$0"}, &ConstantUtf8Info{Value: "jdec$rethrow$0"}}, Methods: []*MemberInfo{{NameIndex: 2}}}
	dumper := NewClassObjectDumper(object)
	dumper.FuncCtx = &class_context.ClassContext{Arguments: []string{"jdec$escape$1"}}
	if name := dumper.checkedEscapeCatchName(); name != "jdec$escape$2" {
		t.Fatalf("fresh catch name %q", name)
	}
	if name := dumper.checkedEscapeHelperName(); name != "jdec$rethrow$1" {
		t.Fatalf("fresh helper name %q", name)
	}
}

func TestCheckedEscapeImplicitSuperclassPrefixRequiresCompleteCoveredDeclaration(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "ImplicitSuperclassMetadata.java")
	const source = `class ImplicitMetadataParent {ImplicitMetadataParent()throws java.io.IOException{}} class ImplicitMetadataOps {static void effect()throws java.io.IOException{}} public class ImplicitSuperclassMetadata extends ImplicitMetadataParent {ImplicitSuperclassMetadata()throws java.io.IOException{ImplicitMetadataOps.effect();}}`
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ImplicitSuperclassMetadata.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name            string
		known, declared bool
		superExceptions []string
		wantBridge      bool
		wantError       bool
	}{
		{"no checked superclass declaration", true, false, nil, true, false},
		{"runtime superclass declaration", true, false, []string{"java/lang/RuntimeException"}, true, false},
		{"covered checked superclass declaration", true, true, []string{"java/io/IOException"}, true, false},
		{"uncovered checked superclass declaration", true, false, []string{"java/io/IOException"}, false, true},
		{"unknown superclass declaration", false, false, nil, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			object, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			method := object.Methods[0]
			var code *CodeAttribute
			var attrs []AttributeInfo
			for _, a := range method.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
				if _, checked := a.(*ExceptionsAttribute); !checked || tt.declared {
					attrs = append(attrs, a)
				}
			}
			method.Attributes = attrs
			dumper := NewClassObjectDumper(object)
			dumper.FuncCtx = &class_context.ClassContext{InvocationMetadata: func(owner string) (callbinding.Class, bool) {
				var m callbinding.Method
				switch owner {
				case "ImplicitMetadataParent":
					m = callbinding.Method{Name: "<init>", Desc: "()V", ExceptionsKnown: tt.known, Exceptions: tt.superExceptions}
				case "ImplicitMetadataOps":
					m = callbinding.Method{Name: "effect", Desc: "()V", ExceptionsKnown: true, Exceptions: []string{"java/lang/Throwable"}}
				default:
					return callbinding.Class{}, false
				}
				return callbinding.Class{Name: owner, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{m}}, true
			}}
			got, err := dumper.methodNeedsCheckedEscape(code, nil, method)
			if got != tt.wantBridge || (err != nil) != tt.wantError {
				t.Fatalf("bridge=%v err=%v", got, err)
			}
		})
	}
}

func TestCheckedEscapeInheritedNamesRequiresCompleteAcyclicFamily(t *testing.T) {
	object := &ClassObject{ConstantPool: []ConstantInfo{&ConstantUtf8Info{Value: "Child"}, &ConstantClassInfo{NameIndex: 1}, &ConstantUtf8Info{Value: "Parent"}, &ConstantClassInfo{NameIndex: 3}}, ThisClass: 2, SuperClass: 4}
	for _, variant := range []string{"complete parent", "unknown parent", "incomplete members", "incomplete parents", "cycle", "unknown ancestor", "wrong owner"} {
		t.Run(variant, func(t *testing.T) {
			table := map[string]callbinding.Class{"Parent": {Name: "Parent", MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: "jdec$rethrow$0", Desc: "(Ljava/lang/Throwable;)Ljava/lang/RuntimeException;", Public: true, Static: true}}}}
			parent := table["Parent"]
			switch variant {
			case "unknown parent":
				delete(table, "Parent")
			case "incomplete members":
				parent.MembersComplete = false
			case "incomplete parents":
				parent.ParentsComplete = false
			case "cycle":
				parent.Parents = []string{"Parent"}
			case "unknown ancestor":
				parent.Parents = []string{"Missing"}
			case "wrong owner":
				parent.Name = "Other"
			}
			if variant != "unknown parent" {
				table["Parent"] = parent
			}
			dumper := NewClassObjectDumper(object)
			dumper.FuncCtx = &class_context.ClassContext{InvocationMetadata: func(owner string) (callbinding.Class, bool) { c, ok := table[owner]; return c, ok }}
			names, known := dumper.checkedEscapeInheritedNames()
			if known != (variant == "complete parent") {
				t.Fatalf("complete=%v", known)
			}
			if known && (!names["jdec$rethrow$0"] || dumper.checkedEscapeHelperName() != "jdec$rethrow$1") {
				t.Fatalf("inherited reservations=%v", names)
			}
		})
	}
}

func TestCheckedEscapeDirectThrowRequiresActualPCAndKnownOperand(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "CheckedThrowMetadata.java")
	if err := os.WriteFile(path, []byte(`public class CheckedThrowMetadata {static void direct(java.io.IOException value)throws java.io.IOException{throw value;}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("compile %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "CheckedThrowMetadata.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"original checked operand", "caller declares checked operand", "missing origin", "wrong opcode PC", "unthrowable operand", "canonical runtime operand", "canonical error operand", "unknown type variable", "null literal", "conflicting same PC", "typed nil node"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range object.Methods {
				n, _ := object.getUtf8(m.NameIndex)
				if n == "direct" {
					method = m
				}
			}
			var attrs []AttributeInfo
			for _, a := range method.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
				if _, checked := a.(*ExceptionsAttribute); !checked || variant == "caller declares checked operand" {
					attrs = append(attrs, a)
				}
			}
			method.Attributes = attrs
			value := values.NewJavaRef(nil, nil, types.NewJavaClass("java.io.IOException"))
			thrown := &statements.CustomStatement{ThrownValue: value, OriginPC: 1, HasOriginPC: true}
			body := []statements.Statement{thrown}
			switch variant {
			case "missing origin":
				thrown.HasOriginPC = false
			case "wrong opcode PC":
				thrown.OriginPC = 0
			case "unthrowable operand":
				thrown.ThrownValue = values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.String"))
			case "canonical runtime operand":
				thrown.ThrownValue = values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.IllegalStateException"))
			case "canonical error operand":
				thrown.ThrownValue = values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.AssertionError"))
			case "unknown type variable":
				thrown.ThrownValue = values.NewJavaRef(nil, nil, types.NewJavaClass("E"))
			case "null literal":
				thrown.ThrownValue = values.JavaNull
			case "conflicting same PC":
				body = append(body, &statements.CustomStatement{ThrownValue: values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.Throwable")), OriginPC: 1, HasOriginPC: true})
			case "typed nil node":
				var branch *statements.IfStatement
				body = append(body, branch)
			}
			dumper := NewClassObjectDumper(object)
			dumper.FuncCtx = &class_context.ClassContext{InvocationMetadata: func(string) (callbinding.Class, bool) { return callbinding.Class{}, false }}
			got, err := dumper.methodNeedsCheckedEscape(code, body, method)
			want := variant == "original checked operand" || variant == "typed nil node"
			if err != nil || got != want {
				t.Fatalf("bridge=%v want%v err=%v", got, want, err)
			}
		})
	}
}
