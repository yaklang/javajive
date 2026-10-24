package javaclassparser

import (
	"context"
	"encoding/binary"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// The only Alpha->Beta source reference lives in Entry.get, not in its
// enclosing method. Beta.Value's field closes the reverse source dependency.
const nativeLocalOnlyDependencyCycleFixture = `class LocalOnlyAlpha{static class Mark{}static Object make(){class Entry{Object get(){return new LocalOnlyBeta.Value();}}return new Entry();}}
class LocalOnlyBeta{static class Value{LocalOnlyAlpha.Mark mark;}}
class LocalOnlyDriver{public static void main(String[]args)throws Exception{Object local=LocalOnlyAlpha.make();if(!local.getClass().isLocalClass()||local.getClass().getEnclosingMethod().getDeclaringClass()!=LocalOnlyAlpha.class||!local.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("original local owner:"+local.getClass()+":"+local.getClass().isLocalClass()+":"+local.getClass().getEnclosingMethod());java.lang.reflect.Method method=local.getClass().getDeclaredMethod("get");method.setAccessible(true);Object value=method.invoke(local);if(value.getClass()!=LocalOnlyBeta.Value.class||LocalOnlyAlpha.class.getDeclaredClasses().length!=1||LocalOnlyBeta.Value.class.getDeclaringClass()!=LocalOnlyBeta.class)throw new AssertionError("original declared dependencies");System.out.println("local:cycle:original:ownership");}}`

func nativeLocalOnlyDependencyFixture(scope string) string {
	f := nativeLocalOnlyDependencyCycleFixture
	switch scope {
	case "instance":
		f = strings.Replace(f, "static Object make()", "Object make()", 1)
		f = strings.Replace(f, "Object local=LocalOnlyAlpha.make()", "Object local=new LocalOnlyAlpha().make()", 1)
	case "member":
		f = strings.Replace(f, "static Object make(){", "static class Container{static Object make(){", 1)
		f = strings.Replace(f, "return new Entry();}}", "return new Entry();}}}", 1)
		f = strings.Replace(f, "Object local=LocalOnlyAlpha.make()", "Object local=LocalOnlyAlpha.Container.make()", 1)
		f = strings.Replace(f, "getDeclaringClass()!=LocalOnlyAlpha.class", "getDeclaringClass()!=LocalOnlyAlpha.Container.class", 1)
		f = strings.Replace(f, "getDeclaredClasses().length!=1", "getDeclaredClasses().length!=2", 1)
	}
	return f
}

// javac may copy a foreign member row and its otherwise unused CONSTANT_Class
// into the outer even though executable code refers to it only in the local.
// Remove those optional hints to exercise the actual emitted-body dependency.
// Every runtime test verifies these mutated authored originals with the JVM
// before using their output as an oracle. No instruction/descriptor is edited.
func nativeLocalOnlyDependencyRemoveOuterHints(t *testing.T, files map[string][]byte, owner, other string) {
	t.Helper()
	for name, raw := range files {
		if name != owner+".class" && name != owner+"$Container.class" {
			continue
		}
		root, e := Parse(append([]byte(nil), raw...))
		if e != nil {
			t.Fatal(e)
		}
		objectName := sourceBridgePoolString(t, root, "java/lang/Object")
		for _, a := range root.Attributes {
			if table, ok := a.(*InnerClassesAttribute); ok {
				rows := table.Classes[:0]
				for _, row := range table.Classes {
					n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
					if n != other+"$Value" {
						rows = append(rows, row)
					}
				}
				table.Classes = rows
				table.NumberOfClasses = uint16(len(rows))
				table.AttrLen = uint32(2 + 8*len(rows))
			}
		}
		for _, cp := range root.ConstantPool {
			if c, ok := cp.(*ConstantClassInfo); ok {
				n, _ := sourceBridgeUTF8(root, c.NameIndex)
				if n == other+"$Value" {
					c.NameIndex = objectName
				}
			}
		}
		files[name] = root.Bytes()
	}
}

func TestNativeMethodLocalDependencyGraphIncludesOriginalBodies(t *testing.T) {
	for _, scope := range []string{"static", "instance", "member"} {
		t.Run(scope, func(t *testing.T) {
			files := nativeCompileClasses(t, nativeLocalOnlyDependencyFixture(scope))
			nativeLocalOnlyDependencyRemoveOuterHints(t, files, "LocalOnlyAlpha", "LocalOnlyBeta")
			for _, root := range []string{"LocalOnlyAlpha", "LocalOnlyBeta"} {
				z := nativeArchive(t, files)
				graph, known := z.nativeMemberOriginalDependencyGraph(root, nil)
				if !known || !graph["LocalOnlyAlpha"]["LocalOnlyBeta"] || !graph["LocalOnlyBeta"]["LocalOnlyAlpha"] {
					t.Fatalf("missing rendered-local dependency from %s: %v/%v", root, graph, known)
				}
				components, known := nativeMemberSourceComponents(graph, nil)
				if !known || len(components[root]) != 2 {
					t.Fatalf("local dependency not in joint component: %v/%v", components, known)
				}
				if z.nativeMemberDependenciesAcyclic(root, nil) {
					t.Fatal("ordinary cache admitted a hidden source cycle")
				}
				z.Close()
			}
		})
	}
}

func TestNativeMethodLocalDependencyGraphKeepsOriginalSuperCallers(t *testing.T) {
	files := nativeCompileClasses(t, `class LocalParent {class Base {Base(){}}}class LocalSubclass extends LocalParent {Object make(){class Entry extends Base{}return new Entry();}}`)
	for _, root := range []string{"LocalParent", "LocalSubclass"} {
		t.Run(root, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			graph, known := z.nativeMemberOriginalDependencyGraph(root, nil)
			if !known || !graph["LocalParent"]["LocalSubclass"] || !graph["LocalSubclass"]["LocalParent"] {
				t.Fatalf("local SUPER caller lost its enclosing source role: %v/%v", graph, known)
			}
		})
	}
}

func TestNativeMethodLocalDependencyGraphDoesNotBorrowForeignOwnership(t *testing.T) {
	original := nativeCompileClasses(t, nativeLocalOnlyDependencyCycleFixture)
	nativeLocalOnlyDependencyRemoveOuterHints(t, original, "LocalOnlyAlpha", "LocalOnlyBeta")
	for _, variant := range []string{"original", "missing local", "wrong identity", "missing exact method", "duplicate enclosing", "initializer scope", "wrong enclosing class", "wrong self name", "wrong row flags", "duplicate row", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range original {
				files[n] = append([]byte(nil), b...)
			}
			local, e := Parse(files["LocalOnlyAlpha$1Entry.class"])
			if e != nil {
				t.Fatal(e)
			}
			root, e := Parse(files["LocalOnlyAlpha.class"])
			if e != nil {
				t.Fatal(e)
			}
			var work *workbudget.Budget
			switch variant {
			case "missing exact method":
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "make" {
						m.NameIndex = sourceBridgePoolString(t, root, "differentMethod")
					}
				}
			case "duplicate enclosing", "initializer scope", "wrong enclosing class":
				for _, a := range local.Attributes {
					if attr, ok := a.(*UnparsedAttribute); ok && attr.Name == "EnclosingMethod" {
						switch variant {
						case "duplicate enclosing":
							local.Attributes = append(local.Attributes, attr)
						case "initializer scope":
							attr.Info[2], attr.Info[3] = 0, 0
						case "wrong enclosing class":
							binary.BigEndian.PutUint16(attr.Info[:2], local.ThisClass)
						}
						break
					}
				}
			case "wrong self name":
				for _, a := range local.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(local, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								row.InnerNameIndex = sourceBridgePoolString(t, local, "Other")
							}
						}
					}
				}
			case "wrong row flags", "duplicate row":
				for _, a := range root.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								if variant == "wrong row flags" {
									row.InnerClassAccessFlags ^= 16
								} else {
									copy := *row
									table.Classes = append(table.Classes, &copy)
								}
								break
							}
						}
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			for _, object := range []*ClassObject{root, local} {
				for _, a := range object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						table.NumberOfClasses = uint16(len(table.Classes))
						table.AttrLen = uint32(2 + 8*len(table.Classes))
					}
				}
			}
			files["LocalOnlyAlpha.class"] = root.Bytes()
			files["LocalOnlyAlpha$1Entry.class"] = local.Bytes()
			if variant == "missing local" {
				delete(files, "LocalOnlyAlpha$1Entry.class")
			}
			if variant == "wrong identity" {
				files["LocalOnlyAlpha$1Entry.class"] = files["LocalOnlyBeta.class"]
			}
			z := nativeArchive(t, files)
			defer z.Close()
			graph, known := z.nativeMemberOriginalDependencyGraph("LocalOnlyAlpha", work)
			if got := known && graph["LocalOnlyAlpha"]["LocalOnlyBeta"]; got != (variant == "original") {
				t.Fatalf("uncorroborated body imported: %v/%v", graph, known)
			}
		})
	}
}

func TestNativeMethodLocalOnlyCycleReadOrdersShareAtomicSourceReceipt(t *testing.T) {
	files := nativeCompileClasses(t, nativeLocalOnlyDependencyCycleFixture)
	nativeLocalOnlyDependencyRemoveOuterHints(t, files, "LocalOnlyAlpha", "LocalOnlyBeta")
	const local = "LocalOnlyAlpha$1Entry"
	for _, order := range [][]string{{"LocalOnlyAlpha", "LocalOnlyBeta", local}, {local, "LocalOnlyBeta", "LocalOnlyAlpha"}, {"LocalOnlyBeta", local, "LocalOnlyAlpha"}} {
		t.Run(strings.Join(order, ":"), func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			for _, n := range order {
				source, e := z.ReadFile(n + ".class")
				if e != nil || strings.Contains(string(source), DecompileStubMarker) {
					t.Fatalf("read %s:%v\n%s", n, e, source)
				}
			}
			object, e := Parse(append([]byte(nil), files["LocalOnlyAlpha.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			first := z.nativeMemberTransactionEntry(object)
			if first == nil || first.family == nil || first.family.methodLocals[local] == nil {
				t.Fatal("missing atomic local body receipt")
			}
			before := z.nativeMembersBytes
			var wg sync.WaitGroup
			errors := make(chan string, 12)
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					n := order[i%len(order)]
					source, e := z.ReadFile(n + ".class")
					if e != nil || strings.Contains(string(source), DecompileStubMarker) {
						errors <- n
					}
				}(i)
			}
			wg.Wait()
			close(errors)
			for n := range errors {
				t.Errorf("concurrent local read failed: %s", n)
			}
			if before != z.nativeMembersBytes {
				t.Fatal("repeat local read reserved a new source body")
			}
		})
	}
}

func TestNativeMethodLocalDependencyGraphIgnoresCorroboratedForeignLocalRow(t *testing.T) {
	files := nativeCompileClasses(t, nativeLocalOnlyDependencyCycleFixture+`class ForeignLocalOwner {Object make(){class ForeignEntry{Object get(){return new ForeignLocalDependency.Value();}}return new ForeignEntry();}}class ForeignLocalDependency{static class Value{}}`)
	nativeLocalOnlyDependencyRemoveOuterHints(t, files, "LocalOnlyAlpha", "LocalOnlyBeta")
	root, e := Parse(append([]byte(nil), files["LocalOnlyAlpha.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	foreign, e := Parse(append([]byte(nil), files["ForeignLocalOwner$1ForeignEntry.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	foreignParent, e := Parse(append([]byte(nil), files["ForeignLocalOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	if _, known := originalMethodLocalOwner(foreign, foreignParent, nil); !known {
		t.Fatal("original foreign owner must be valid")
	}
	for _, a := range root.Attributes {
		if table, ok := a.(*InnerClassesAttribute); ok {
			table.Classes = append(table.Classes, &InnerClassInfo{InnerClassInfoIndex: nativeWideningTestPoolClass(t, root, foreign.GetClassName()), InnerNameIndex: sourceBridgePoolString(t, root, "ForeignEntry")})
			table.NumberOfClasses = uint16(len(table.Classes))
			table.AttrLen = uint32(2 + 8*len(table.Classes))
			break
		}
	}
	files["LocalOnlyAlpha.class"] = root.Bytes()
	z := nativeArchive(t, files)
	defer z.Close()
	graph, known := z.nativeMemberOriginalDependencyGraph("LocalOnlyAlpha", nil)
	if !known || !graph["LocalOnlyAlpha"]["LocalOnlyBeta"] || graph["LocalOnlyAlpha"]["ForeignLocalOwner"] || graph["LocalOnlyAlpha"]["ForeignLocalDependency"] {
		t.Fatalf("foreign local metadata changed physical ownership: %v/%v", graph, known)
	}
}
