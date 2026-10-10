package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeForeignAnonymousMemberSuperRequiresOriginalDeclarationAndAccess(t *testing.T) {
	original := nativeCompileClasses(t, inheritedAnonymousMemberSuperFixture)
	variants := []string{"original", "nil object", "wrong owner", "failed family", "missing parent", "missing declaring owner", "private member", "private constructor", "missing self", "duplicate self", "wrong self owner", "missing reciprocal", "duplicate reciprocal", "wrong reciprocal flags", "wrong superclass", "interface outer", "superclass cycle", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for name, raw := range original {
				object, err := Parse(append([]byte(nil), raw...))
				if err != nil {
					t.Fatal(err)
				}
				objects[strings.TrimSuffix(name, ".class")] = object
			}
			parent, declaring, current := objects["DeclaringNamespace$Entry"], objects["DeclaringNamespace"], objects["CurrentNamespace"]
			row := func(object *ClassObject) (*InnerClassesAttribute, *InnerClassInfo) {
				for _, attribute := range object.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok && table != nil {
						for _, entry := range table.Classes {
							name, _ := sourceBridgeClassName(object, entry.InnerClassInfoIndex)
							if name == "DeclaringNamespace$Entry" {
								return table, entry
							}
						}
					}
				}
				t.Fatal("missing original named edge")
				return nil, nil
			}
			selfTable, self := row(parent)
			ownerTable, reciprocal := row(declaring)
			switch variant {
			case "missing parent":
				delete(objects, parent.GetClassName())
			case "missing declaring owner":
				delete(objects, declaring.GetClassName())
			case "private member":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 2, 2
			case "private constructor":
				for _, method := range parent.Methods {
					name, _ := sourceBridgeUTF8(parent, method.NameIndex)
					if name == "<init>" {
						method.AccessFlags = 2
					}
				}
			case "missing self":
				selfTable.Classes = nil
			case "duplicate self":
				selfTable.Classes = append(selfTable.Classes, self)
			case "wrong self owner":
				self.OuterClassInfoIndex = parent.ThisClass
			case "missing reciprocal":
				ownerTable.Classes = nil
			case "duplicate reciprocal":
				ownerTable.Classes = append(ownerTable.Classes, reciprocal)
			case "wrong reciprocal flags":
				reciprocal.InnerClassAccessFlags ^= 1
			case "wrong superclass":
				constant := current.ConstantPool[current.SuperClass-1].(*ConstantClassInfo)
				current.ConstantPool[constant.NameIndex-1].(*ConstantUtf8Info).SetString("java/lang/Object")
			case "interface outer":
				declaring.AccessFlags |= 0x200
			case "superclass cycle":
				current.SuperClass = current.ThisClass
			}
			files := map[string][]byte{}
			for name, object := range objects {
				files[name+".class"] = object.Bytes()
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CurrentNamespace.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				if variant == "original" {
					t.Fatal("original named family")
				}
				return
			}
			object, _ := Parse(files["CurrentNamespace$1.class"])
			owner, method, known := originalAnonymousOwner(object)
			if !known {
				t.Fatal("original anonymous scope")
			}
			switch variant {
			case "nil object":
				object = nil
			case "wrong owner":
				owner = "Other"
			case "failed family":
				p.failed = true
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			packet := d.nativeAnonymousConstructorForCompiler(object, owner, method, p.owner, p, nil, d.buildInvocationMetadata(), nil)
			if (packet != nil) != (variant == "original") {
				t.Fatalf("original foreign SUPER packet=%v", packet != nil)
			}
			if p.children["DeclaringNamespace$Entry"] != nil || p.lexicalObjects["DeclaringNamespace$Entry"] != nil || len(p.constructorBridges("DeclaringNamespace$Entry")) != 0 {
				t.Fatal("foreign binding imported private ownership")
			}
		})
	}
}

func TestNativeForeignAnonymousMemberSuperPhysicalCallRequiresExactOriginalSite(t *testing.T) {
	files := nativeCompileClasses(t, inheritedAnonymousMemberSuperFixture)
	for _, variant := range []string{"original", "wrong pc", "wrong descriptor", "same bytes foreign method", "nonconstructor", "wrong parent packet", "private parent constructor", "foreign bridge", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CurrentNamespace.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			object, _ := Parse(files["CurrentNamespace$1.class"])
			owner, method, _ := originalAnonymousOwner(object)
			packet := d.nativeAnonymousConstructorForCompiler(object, owner, method, p.owner, p, nil, d.buildInvocationMetadata(), nil)
			if packet == nil || packet.memberSuper == nil {
				t.Fatal("original packet")
			}
			var ctor *MemberInfo
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "<init>" {
					ctor = method
				}
			}
			reader := z.nativeMemberReader(object)
			pc, descriptor, parent := packet.superPC, packet.superDescriptor, packet.memberSuper
			switch variant {
			case "wrong pc":
				pc++
			case "wrong descriptor":
				descriptor = "()V"
			case "same bytes foreign method":
				copy := *ctor
				ctor = &copy
			case "nonconstructor":
				for _, method := range object.Methods {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					if name != "<init>" {
						ctor = method
						break
					}
				}
			case "wrong parent packet":
				copy := *parent
				copy.owner = owner
				parent = &copy
			case "private parent constructor":
				for _, method := range parent.object.Methods {
					name, _ := sourceBridgeUTF8(parent.object, method.NameIndex)
					if name == "<init>" {
						method.AccessFlags = 2
					}
				}
			case "foreign bridge":
				copy := *parent
				copy.accessBridges = map[string]*nativeConstructorAccessBridge{"unproved": {}}
				parent = &copy
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := reader.nativeAnonymousForeignOriginalSuper(parent, ctor, descriptor, pc); got != (variant == "original") {
				t.Fatalf("physical SUPER proof=%v", got)
			}
		})
	}
}

func TestNativeForeignAnonymousMemberSuperAtomicGraphIncludesAnonymousCaller(t *testing.T) {
	files := nativeCompileClasses(t, inheritedAnonymousMemberNestedFixture)
	for _, variant := range []string{"original", "missing enclosing root", "missing enclosing anonymous", "owner cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for name, raw := range files {
				input[name] = append([]byte(nil), raw...)
			}
			var work *workbudget.Budget
			switch variant {
			case "missing enclosing root":
				delete(input, "CurrentNamespace.class")
			case "missing enclosing anonymous":
				delete(input, "CurrentNamespace$1.class")
			case "owner cycle":
				object, _ := Parse(input["CurrentNamespace$1.class"])
				for _, attr := range object.Attributes {
					if raw, ok := attr.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						raw.Info[0], raw.Info[1] = byte(object.ThisClass>>8), byte(object.ThisClass)
					}
				}
				input["CurrentNamespace$1.class"] = object.Bytes()
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			z := nativeArchive(t, input)
			defer z.Close()
			graph, known := z.nativeMemberOriginalDependencyGraph("DeclaringNamespace", work)
			if known != (variant == "original") {
				t.Fatalf("original closure=%v", known)
			}
			if known && (!graph["DeclaringNamespace"]["CurrentNamespace"] || !graph["CurrentNamespace"]["DeclaringNamespace"]) {
				t.Fatal("anonymous foreign SUPER source families did not join atomically")
			}
		})
	}
}

func TestNativeForeignAnonymousMemberSuperProtectedScopeRequiresWholePreparedForest(t *testing.T) {
	files := nativeCompileClasses(t, inheritedAnonymousMemberNestedFixture)
	for _, variant := range []string{"original", "failed family", "no ancestor", "missing unit object", "same bytes replacement object", "missing enclosing unit", "wrong anonymous method", "wrong group owner", "missing forest", "missing root", "wrong root lexical identity", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CurrentNamespace.class"])
			peer := z.prepareNativeMemberFamilyUnpublished(root, nil)
			if peer == nil {
				t.Fatal("original prepared lexical forest")
			}
			user := "CurrentNamespace$1$1"
			p := peer.family
			var work *workbudget.Budget
			subclass := func(name string) bool { return name == "CurrentNamespace$1" }
			switch variant {
			case "failed family":
				p.failed = true
			case "no ancestor":
				subclass = func(string) bool { return false }
			case "missing unit object":
				delete(peer.objects, user)
			case "same bytes replacement object":
				peer.objects[user], _ = Parse(files[user+".class"])
			case "missing enclosing unit":
				delete(p.anonymousUnits, "CurrentNamespace$1")
			case "wrong anonymous method":
				p.anonymousUnits[user].children[user].method = "different"
			case "wrong group owner":
				p.anonymousUnits[user].owner = p.owner
			case "missing forest":
				p.anonymousForest = nil
			case "missing root":
				delete(peer.objects, p.owner)
			case "wrong root lexical identity":
				p.lexicalObjects[p.owner], _ = Parse(files[p.owner+".class"])
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberJointAnonymousProtectedTypeAccess(peer, user, work, subclass); got != (variant == "original") {
				t.Fatalf("joint protected forest proof=%v", got)
			}
		})
	}
}

func TestNativeForeignAnonymousMemberSuperProtectedNamedScopeRequiresOriginalSpelling(t *testing.T) {
	files := nativeCompileClasses(t, inheritedAnonymousMemberNamedContextFixture())
	for _, variant := range []string{"original", "wrong source name", "wrong original owner", "wrong original flags", "missing lexical object", "same bytes foreign object"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CurrentNamespace.class"])
			peer := z.prepareNativeMemberFamilyUnpublished(root, nil)
			if peer == nil {
				t.Fatal("original mixed named/anonymous forest")
			}
			child := peer.family.children["CurrentNamespace$Container"]
			if child == nil {
				t.Fatal("original named scope")
			}
			switch variant {
			case "wrong source name":
				child.sourceName = "Decoy.Container"
			case "wrong original owner":
				child.owner = "Decoy"
			case "wrong original flags":
				child.flags ^= 8
			case "missing lexical object":
				delete(peer.family.lexicalObjects, "CurrentNamespace$Container")
			case "same bytes foreign object":
				peer.objects["CurrentNamespace$Container"], _ = Parse(files["CurrentNamespace$Container.class"])
			}
			if got := nativeMemberJointAnonymousProtectedTypeAccess(peer, "CurrentNamespace$Container$1$1", nil, func(name string) bool { return name == "CurrentNamespace$Container$1" }); got != (variant == "original") {
				t.Fatalf("mixed original source scope=%v", got)
			}
		})
	}
}

func TestNativeForeignAnonymousMemberSuperCommitRequiresSameOriginalPacket(t *testing.T) {
	files := nativeCompileClasses(t, inheritedAnonymousMemberSuperFixture)
	for _, variant := range []string{"original", "missing completed", "wrong bytes", "wrong owner", "wrong field", "wrong source descriptor", "wrong capture pc", "wrong delegate pc", "missing constructor", "foreign bridge", "missing source name", "wrong source name", "wrong unit key", "mixed valid/refused units", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CurrentNamespace.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			object, _ := Parse(files["CurrentNamespace$1.class"])
			owner, method, _ := originalAnonymousOwner(object)
			packet := d.nativeAnonymousConstructorForCompiler(object, owner, method, p.owner, p, nil, d.buildInvocationMetadata(), nil)
			if packet == nil || packet.memberSuper == nil {
				t.Fatal("original planning packet")
			}
			parent := packet.memberSuper
			completed := *parent
			completed.object, _ = Parse(append([]byte(nil), files["DeclaringNamespace$Entry.class"]...))
			completed.sourceName = "DeclaringNamespace.Entry"
			completed.constructors = map[string]*nativeMemberConstructor{}
			for key, ctor := range parent.constructors {
				copy := *ctor
				completed.constructors[key] = &copy
			}
			p.allocationDependencies = map[string]*nativeMemberClass{"DeclaringNamespace$Entry": &completed}
			p.sourceDependencies = map[string]string{"DeclaringNamespace$Entry": completed.sourceName}
			group := &nativeAnonymousFamily{owner: p.owner, children: map[string]*nativeAnonymousClass{object.GetClassName(): packet}}
			p.anonymousUnits = map[string]*nativeAnonymousFamily{object.GetClassName(): group}
			var work *workbudget.Budget
			switch variant {
			case "missing completed":
				p.allocationDependencies = nil
			case "wrong bytes":
				completed.object.AccessFlags ^= 0x10
			case "wrong owner":
				completed.owner = p.owner
			case "wrong field":
				completed.field = "sameTypeDecoy"
			case "wrong source descriptor", "wrong capture pc", "wrong delegate pc":
				for _, ctor := range completed.constructors {
					switch variant {
					case "wrong source descriptor":
						ctor.sourceDescriptor = "()V"
					case "wrong capture pc":
						ctor.capturePC++
					case "wrong delegate pc":
						ctor.delegatePC++
					}
				}
			case "missing constructor":
				completed.constructors = nil
			case "foreign bridge":
				completed.accessBridges = map[string]*nativeConstructorAccessBridge{"unproved": {}}
			case "missing source name":
				completed.sourceName = ""
			case "wrong source name":
				p.sourceDependencies["DeclaringNamespace$Entry"] = "Decoy.Entry"
			case "wrong unit key":
				p.anonymousUnits = map[string]*nativeAnonymousFamily{"Other": {children: map[string]*nativeAnonymousClass{"Other": packet}}}
			case "mixed valid/refused units":
				other := *packet
				other.object, _ = Parse(files["CurrentNamespace$1.class"])
				class := other.object.ConstantPool[other.object.ThisClass-1].(*ConstantClassInfo)
				other.object.ConstantPool[class.NameIndex-1].(*ConstantUtf8Info).SetString("CurrentNamespace$2")
				uncommitted := *parent
				uncommitted.owner = "UnprovedOwner"
				other.memberSuper = &uncommitted
				p.anonymousUnits["CurrentNamespace$2"] = &nativeAnonymousFamily{owner: p.owner, children: map[string]*nativeAnonymousClass{"CurrentNamespace$2": &other}}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousForeignSupersCommitted(p, work)
			if got != (variant == "original") {
				t.Fatalf("dependency commit=%v", got)
			}
			if !got && packet.memberSuper != parent {
				t.Fatal("failed validation partially published a completed binding")
			}
			if got && (packet.memberSuper != &completed || len(p.pendingAnonymousSuperDependencies) != 0) {
				t.Fatal("committed dependency did not replace temporary packet")
			}
		})
	}
}
