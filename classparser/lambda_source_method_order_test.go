package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// These controls query completed original packets. Mutated metadata and Code
// are denial controls, not claims that the resulting bytecode is executable.
func TestAdversarialLexicalLambdaMethodOrderRequiresCompletedOriginalPackets(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for the original compiler counter")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, lexicalInitializerLambdaFixture, "InitPacketDriver", "none", true)
	for _, variant := range []string{"original", "missing family", "failed family", "foreign owner", "lookup only", "no members", "interface", "foreign dumper", "foreign object", "detached cache", "lambda role", "synthetic parent", "duplicate parent", "missing parent", "missing implementation", "changed factory seal", "changed body packet", "comment packet", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, methods := lexicalOrderTestPackets(t, files)
			var captured, initializer *dumpedMethods
			for _, body := range methods {
				switch body.methodName {
				case "captured":
					captured = body
				case "<clinit>":
					initializer = body
				}
			}
			if captured == nil || initializer == nil || len(captured.lambdaEmission.children) != 1 {
				t.Fatal("original initializer/captured factory inventory")
			}
			// Reconstruct the unscheduled declaration assembly. This is a
			// proposal input, before member insertion and final certification.
			var packets []string
			for _, body := range methods {
				packets = append(packets, body.code)
			}
			source := "class InitPacketContract{\n" + strings.Join(packets, "\n") + "\n}"
			members := []string{"original member packets not inserted yet"}
			originalOrder := slices.Clone(methods)
			child := captured.lambdaEmission.children[0].body
			switch variant {
			case "missing family":
				d.nativeMemberRoot = nil
			case "failed family":
				d.nativeMemberRoot.failed = true
			case "foreign owner":
				d.nativeMemberRoot.owner = "AnotherOwner"
			case "lookup only":
				d.nativeMemberRoot.declarationLookupOnly = true
			case "no members":
				members = nil
			case "interface":
				d.obj.AccessFlags |= 0x200
			case "foreign dumper":
				captured.sourceDumper = NewClassObjectDumper(d.obj)
			case "foreign object":
				captured.sourceObject = &ClassObject{}
			case "detached cache":
				delete(d.dumpedMethodsSet, captured.sourceCacheKey)
			case "lambda role":
				captured.sourceRequest.role = methodSourceLambdaBody
			case "synthetic parent":
				captured.member.AccessFlags |= 0x1000
			case "duplicate parent":
				methods = append(methods, captured)
			case "missing parent":
				methods = slices.DeleteFunc(slices.Clone(methods), func(body *dumpedMethods) bool { return body == captured })
			case "missing implementation":
				delete(d.lambdaMethodSourceBodies, child.member)
			case "changed factory seal":
				child.sourceRequest.factory.sealed = false
			case "changed body packet":
				child.code += " "
			case "comment packet":
				source = strings.Replace(source, captured.code, "/*"+captured.code+"*/", 1)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			proposal, known := d.proposeLexicalLambdaMethodOrder(source, members, methods)
			if known != (variant == "original") {
				t.Fatalf("original declaration proposal=%v", known)
			}
			if !bytes.Equal(before, d.obj.Bytes()) || !slices.Equal(methods, originalOrder) && variant != "duplicate parent" && variant != "missing parent" {
				t.Fatal("query mutated the original class or method pointer slice")
			}
			if !known {
				return
			}
			// The initializer and all nonmoving declarations retain their
			// relative order; completed bodies are conserved exactly once.
			var fixedBefore, fixedAfter []*dumpedMethods
			seen := map[*dumpedMethods]bool{}
			for _, body := range methods {
				if body != captured {
					fixedBefore = append(fixedBefore, body)
				}
			}
			for _, body := range proposal {
				if seen[body] || !slices.Contains(methods, body) {
					t.Fatal("proposal duplicated or replaced a declaration")
				}
				seen[body] = true
				if body != captured {
					fixedAfter = append(fixedAfter, body)
				}
			}
			if len(seen) != len(methods) || !slices.Equal(fixedBefore, fixedAfter) || slices.Index(proposal, initializer) >= slices.Index(proposal, captured) {
				t.Fatal("proposal changed fixed order or original factory order")
			}
		})
	}
}

func TestAdversarialLexicalLambdaScheduleRejectsAllocationAndPrivateRegistration(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for actual private accessor bytecode")
	}
	fixture := strings.Replace(lexicalInitializerLambdaFixture, "static Object echo", "static Object read(Cell cell){return cell.value;}\n static Object echo", 1)
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "InitPacketDriver", "none", true)
	for _, variant := range []string{"original", "object allocation", "private getter", "method local owner", "missing nested receipt", "changed nested packet", "missing original Code", "duplicate original Code", "recursion", "depth", "negative depth", "negative scan", "scan limit", "nil active", "nil scan", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, methods := lexicalOrderTestPackets(t, files)
			var body *dumpedMethods
			for _, candidate := range methods {
				if candidate.methodName == "captured" || variant == "object allocation" && candidate.methodName == "echo" || variant == "private getter" && candidate.methodName == "read" {
					if body == nil || candidate.methodName != "captured" {
						body = candidate
					}
				}
			}
			if body == nil {
				t.Fatal("missing original method")
			}
			active, scanned, depth := map[*MemberInfo]bool{}, 0, 0
			scan := &scanned
			switch variant {
			case "method local owner":
				name, _ := sourceBridgeUTF8(d.obj, body.member.NameIndex)
				desc, _ := sourceBridgeUTF8(d.obj, body.member.DescriptorIndex)
				d.nativeMemberRoot.methodLocals = map[string]*nativeMethodLocalClass{"local": {owner: &nativeMethodLocalOwner{owner: d.obj.GetClassName(), method: name, descriptor: desc}}}
			case "missing nested receipt":
				delete(d.lambdaMethodSourceBodies, body.lambdaEmission.children[0].body.member)
			case "changed nested packet":
				body.lambdaEmission.children[0].body.code += " "
			case "missing original Code":
				body.member.Attributes = slices.DeleteFunc(slices.Clone(body.member.Attributes), func(a AttributeInfo) bool { _, ok := a.(*CodeAttribute); return ok })
			case "duplicate original Code":
				for _, a := range body.member.Attributes {
					if _, ok := a.(*CodeAttribute); ok {
						body.member.Attributes = append(body.member.Attributes, a)
						break
					}
				}
			case "recursion":
				active[body.member] = true
			case "depth":
				depth = 64
			case "negative depth":
				depth = -1
			case "negative scan":
				scanned = -1
			case "scan limit":
				scanned = 256 << 10
			case "nil active":
				active = nil
			case "nil scan":
				scan = nil
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			if allowed := d.lambdaScheduleHasNoLexicalEvents(body, active, depth, scan); allowed != (variant == "original") {
				t.Fatalf("declaration has no lexical registration=%v", allowed)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("registration query mutated original bytecode")
			}
		})
	}
}

func lexicalOrderTestPackets(t *testing.T, files map[string][]byte) (*ClassObjectDumper, []*dumpedMethods) {
	t.Helper()
	archive := nativeArchive(t, files)
	t.Cleanup(func() { _ = archive.Close() })
	archive.sourceCompiler, archive.targetSourceVersion = NativeJavac8, 8
	object, err := Parse(bytes.Clone(files["InitPacketContract.class"]))
	if err != nil {
		t.Fatal(err)
	}
	prepared := archive.prepareNativeMemberFamilyUnpublished(object, map[string]string{})
	if prepared == nil {
		t.Fatal("original private member family preparation")
	}
	d := prepared.reader
	d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
	if _, err := d.DumpClass(); err != nil {
		t.Fatal("complete original declaration packets", err)
	}
	methods, err := d.DumpMethods()
	if err != nil {
		t.Fatal(err)
	}
	return d, methods
}
