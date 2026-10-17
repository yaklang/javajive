package javaclassparser

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func methodParameterNameGuardClass(t *testing.T) []byte {
	t.Helper()
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "GuardOwner.java")
	if err := os.WriteFile(path, []byte(`class GuardOwner{static long read(long leading,int trailing){return leading+trailing;}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("authored guard compile: %v\n%s", err, log)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "GuardOwner.class"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func methodParameterNameGuardInput(t *testing.T, raw []byte) (*ClassObjectDumper, *MemberInfo, *UnparsedAttribute, []values.JavaValue) {
	t.Helper()
	obj, err := Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatal(err)
	}
	var method *MemberInfo
	var table *UnparsedAttribute
	for _, m := range obj.Methods {
		if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "read" {
			method = m
			for _, a := range m.Attributes {
				if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
					table = p
				}
			}
		}
	}
	if method == nil || table == nil {
		t.Fatal("missing authored original table")
	}
	d := NewClassObjectDumper(obj)
	d.CurrentMethod = method
	d.FuncCtx = &class_context.ClassContext{IsStatic: true, FunctionName: "read", CurrentMethodDesc: "(JI)J", LocalNames: map[*coreutils.VariableId]string{}}
	var header []values.JavaValue
	for i, slot := range []int{0, 2} {
		typ := types.NewJavaPrimer(types.JavaInteger)
		if i == 0 {
			typ = types.NewJavaPrimer(types.JavaLong)
		}
		ref := values.NewJavaRef(coreutils.NewRootVariableId(), values.NewJavaLiteral(i, typ), typ)
		ref.Id.SetName([]string{"var0", "var1"}[i])
		ref.IsParam = true
		ref.MarkOriginalParameter(slot)
		header = append(header, ref)
	}
	return d, method, table, header
}

func methodParameterNameGuardSet(t *testing.T, object *ClassObject, table *UnparsedAttribute, position int, text string) {
	t.Helper()
	object.ConstantPool = append(object.ConstantPool, &ConstantUtf8Info{Value: text})
	binary.BigEndian.PutUint16(table.Info[1+4*position:3+4*position], uint16(len(object.ConstantPool)))
}

func TestAdversarialOriginalOrdinaryMethodParameterNamesRequireCompleteOriginalTable(t *testing.T) {
	raw := methodParameterNameGuardClass(t)
	for _, variant := range []string{"original", "unicode", "absent", "duplicate name", "keyword", "illegal character", "leading digit", "ignorable", "underscore", "empty name", "no name", "wrong CP kind", "short table", "extra table", "wrong attribute length", "wrong logical count", "synthetic parameter", "mandated parameter", "unknown flags", "duplicate attribute", "nil attribute", "illegal class version", "duplicate method pointer", "duplicate method copy", "detached method", "nil method", "wrong descriptor", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, table, _ := methodParameterNameGuardInput(t, raw)
			descriptor := "(JI)J"
			switch variant {
			case "unicode":
				methodParameterNameGuardSet(t, d.obj, table, 0, "π")
			case "absent":
				for i, a := range method.Attributes {
					if a == table {
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
						break
					}
				}
			case "duplicate name":
				copy(table.Info[5:7], table.Info[1:3])
			case "keyword", "illegal character", "leading digit", "ignorable", "underscore", "empty name":
				text := map[string]string{"keyword": "class", "illegal character": "a-b", "leading digit": "1entry", "ignorable": "en\x00try", "underscore": "_", "empty name": ""}[variant]
				methodParameterNameGuardSet(t, d.obj, table, 0, text)
			case "no name":
				binary.BigEndian.PutUint16(table.Info[1:3], 0)
			case "wrong CP kind":
				binary.BigEndian.PutUint16(table.Info[1:3], d.obj.ThisClass)
			case "short table":
				table.Info = table.Info[:len(table.Info)-1]
			case "extra table":
				table.Info = append(table.Info, 0)
			case "wrong attribute length":
				table.Length++
			case "wrong logical count":
				table.Info[0]--
			case "synthetic parameter":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x1000)
			case "mandated parameter":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x8000)
			case "unknown flags":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x4000)
			case "duplicate attribute":
				method.Attributes = append(method.Attributes, table)
			case "nil attribute":
				method.Attributes = append(method.Attributes, (*UnparsedAttribute)(nil))
			case "illegal class version":
				d.obj.MajorVersion = 0
			case "duplicate method pointer":
				d.obj.Methods = append(d.obj.Methods, method)
			case "duplicate method copy":
				copy := *method
				d.obj.Methods = append(d.obj.Methods, &copy)
			case "detached method":
				copy := *method
				method = &copy
			case "nil method":
				d.obj.Methods = append(d.obj.Methods, nil)
			case "wrong descriptor":
				descriptor = "(II)J"
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
			want := variant == "original" || variant == "unicode" || variant == "absent"
			if known != want || variant == "absent" && names != nil {
				t.Fatalf("names=%q known=%v want=%v", names, known, want)
			}
		})
	}
}

func TestAdversarialOrdinaryMethodParameterNamesBindEntryIdentitiesWithoutSlotGuessing(t *testing.T) {
	raw := methodParameterNameGuardClass(t)
	for _, variant := range []string{"original", "instance", "wrong wide offset", "reversed formals", "extra prefix", "missing formal", "shared identity", "unsealed", "replacement seed", "this", "not parameter", "nil identity", "different source binding", "wrong source method", "wrong source descriptor", "wrong source packet", "stale name list", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, _, header := methodParameterNameGuardInput(t, raw)
			names, known := d.originalOrdinaryMethodParameterNames(method, "(JI)J")
			if !known {
				t.Fatal("original table")
			}
			first, second := header[0].(*values.JavaRef), header[1].(*values.JavaRef)
			switch variant {
			case "instance", "wrong wide offset":
				method.AccessFlags &^= 8
				d.FuncCtx.IsStatic = false
				for i, old := range []*values.JavaRef{first, second} {
					fresh := values.NewJavaRef(old.Id, old.Val, old.Type())
					fresh.IsParam = true
					slot := []int{1, 3}[i]
					if variant == "wrong wide offset" && i == 1 {
						slot = 2
					}
					fresh.MarkOriginalParameter(slot)
					header[i] = fresh
				}
			case "reversed formals":
				header[0], header[1] = header[1], header[0]
			case "extra prefix":
				header = append([]values.JavaValue{first}, header...)
			case "missing formal":
				header = header[:1]
			case "shared identity":
				second.Id = first.Id
			case "unsealed":
				header[0] = values.NewJavaRef(first.Id, first.Val, first.Type())
			case "replacement seed":
				first.Val = values.NewJavaLiteral(0, first.Type())
			case "this":
				first.IsThis = true
			case "not parameter":
				first.IsParam = false
			case "nil identity":
				first.Id = nil
			case "different source binding":
				d.FuncCtx.LocalNames[first.Id] = "another"
			case "wrong source method":
				d.FuncCtx.FunctionName = "another"
			case "wrong source descriptor":
				d.FuncCtx.CurrentMethodDesc = "(II)J"
			case "wrong source packet":
				d.CurrentMethod = nil
			case "stale name list":
				names[0] = "another"
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := map[*coreutils.VariableId]string{}
			for _, v := range header {
				if ref := v.(*values.JavaRef); ref.Id != nil {
					before[ref.Id] = ref.Id.String()
				}
			}
			binding, bound := d.bindOrdinaryMethodParameterNames(method, "(JI)J", names, header)
			want := variant == "original" || variant == "instance"
			if bound != want || bound && !d.ordinaryMethodParameterNamesClosed(binding, header) {
				t.Fatalf("bound=%v want=%v", bound, want)
			}
			if !want {
				for id, name := range before {
					if id.String() != name {
						t.Fatal("failed proof partially changed a declaration name")
					}
				}
			}
			if d.Work != nil && d.Work.Used(workbudget.CounterOutputBytes) != 0 {
				t.Fatal("source name binding reserved source publication")
			}
		})
	}
}

func TestAdversarialOrdinaryMethodParameterNamesCloseFinalHeaderAgainstFreshOriginal(t *testing.T) {
	raw := methodParameterNameGuardClass(t)
	for _, variant := range []string{"original", "split final header", "same spelling different identity", "source override", "lost formal role", "stale name", "duplicate table", "duplicate method", "renamed method", "wrong current method", "wrong current descriptor", "different owner object"} {
		t.Run(variant, func(t *testing.T) {
			d, method, table, header := methodParameterNameGuardInput(t, raw)
			names, tableKnown := d.originalOrdinaryMethodParameterNames(method, "(JI)J")
			if !tableKnown || names == nil {
				t.Fatal("original name table")
			}
			binding, known := d.bindOrdinaryMethodParameterNames(method, "(JI)J", names, header)
			if !known {
				t.Fatal("original entry names")
			}
			first := header[0].(*values.JavaRef)
			switch variant {
			case "split final header":
				entry := values.NewJavaRef(first.Id, nil, first.Type().Copy())
				entry.IsParam = true
				header[0] = entry
				if _, sealed := entry.OriginalParameterSlot(); sealed {
					t.Fatal("source entry invented a physical parameter witness")
				}
			case "same spelling different identity":
				first.Id = coreutils.NewRootVariableId()
				first.Id.SetName(names[0])
			case "source override":
				d.FuncCtx.LocalNames[first.Id] = "another"
			case "lost formal role":
				first.IsParam = false
			case "stale name":
				methodParameterNameGuardSet(t, d.obj, table, 0, "another")
			case "duplicate table":
				method.Attributes = append(method.Attributes, table)
			case "duplicate method":
				d.obj.Methods = append(d.obj.Methods, method)
			case "renamed method":
				method.NameIndex = sourceBridgePoolString(t, d.obj, "another")
			case "wrong current method":
				d.CurrentMethod = nil
			case "wrong current descriptor":
				d.FuncCtx.CurrentMethodDesc = "(II)J"
			case "different owner object":
				copy := *d.obj
				d.obj = &copy
			}
			want := variant == "original" || variant == "split final header"
			if closed := d.ordinaryMethodParameterNamesClosed(binding, header); closed != want {
				t.Fatalf("closed=%v want=%v", closed, want)
			}
			if !reflect.DeepEqual(binding.names, []string{"leading", "trailing"}) {
				t.Fatal("original name snapshot followed mutable source metadata")
			}
		})
	}
}
