package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const memberBindingSnapshotFixture = `interface MemberBindingRead{Object read();}
class MemberBindingOuter{final Object token;MemberBindingOuter(Object token){this.token=token;}class Inner{final MemberBindingRead view;<T>Inner(MemberBindingOuter alternate,final T captured){class Scope implements MemberBindingRead{public Object read(){return captured;}}view=new Scope();}MemberBindingOuter outer(){return MemberBindingOuter.this;}}}
class MemberBindingDriver{public static void main(String[]args){Object token=new Object();MemberBindingOuter first=new MemberBindingOuter(new Object()),alternate=new MemberBindingOuter(new Object());MemberBindingOuter.Inner value=first.new Inner(alternate,token);if(value.view.read()!=token||value.outer()!=(args.length==0?first:alternate))throw new AssertionError("original enclosing port identity");System.out.println(args.length==0?"outer:first":"outer:alternate");}}`

func memberBindingWriteClasses(t *testing.T, directory string, files map[string][]byte) {
	t.Helper()
	for name, raw := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func memberBindingOriginalOracle(t *testing.T, files map[string][]byte) {
	t.Helper()
	original := t.TempDir()
	memberBindingWriteClasses(t, original, files)
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, original, "MemberBindingDriver"); got != "outer:first\n" {
		t.Fatalf("original enclosing instance oracle=%q", got)
	}
}

func memberBindingMutantOracle(t *testing.T, files map[string][]byte, object *ClassObject) {
	t.Helper()
	output := t.TempDir()
	memberBindingWriteClasses(t, output, files)
	path := filepath.Join(output, "MemberBindingOuter$Inner.class")
	if err := os.WriteFile(path, object.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	_, java := t04Tools(t)
	actual, err := exec.Command(java, "-Xverify:all", "-Xmx128m", "-cp", output, "MemberBindingDriver", "mutated").CombinedOutput()
	if err != nil || string(actual) != "outer:alternate\n" {
		t.Fatalf("valid authored capture overwrite: %v\n%s", err, actual)
	}
}

func memberBindingMethod(t *testing.T, object *ClassObject, descriptor string) (*MemberInfo, *CodeAttribute) {
	t.Helper()
	var method *MemberInfo
	var code *CodeAttribute
	for _, candidate := range object.Methods {
		name, _ := sourceBridgeUTF8(object, candidate.NameIndex)
		desc, _ := sourceBridgeUTF8(object, candidate.DescriptorIndex)
		if name != "<init>" || desc != descriptor {
			continue
		}
		if method != nil {
			t.Fatal("authored constructor must be unique")
		}
		method = candidate
		for _, attribute := range method.Attributes {
			if body, ok := attribute.(*CodeAttribute); ok {
				if code != nil || body == nil {
					t.Fatal("authored constructor Code must be unique")
				}
				code = body
			}
		}
	}
	if method == nil || code == nil {
		t.Fatal("missing authored constructor/Code")
	}
	return method, code
}

func memberBindingPrepared(t *testing.T, files map[string][]byte) (*JarFS, *nativeMemberPrepared, *nativeMemberClass) {
	t.Helper()
	archive := nativeArchive(t, files)
	t.Cleanup(func() { archive.Close() })
	root, err := Parse(files["MemberBindingOuter.class"])
	if err != nil {
		t.Fatal(err)
	}
	prepared := archive.prepareNativeMemberFamilyUnpublished(root, snapshotJDECEnv())
	if prepared == nil || prepared.family.children["MemberBindingOuter$Inner"] == nil {
		t.Fatal("authored original member family must be proved before mutation")
	}
	return archive, prepared, prepared.family.children["MemberBindingOuter$Inner"]
}

func memberBindingAppendOverwrite(code *CodeAttribute, packet *nativeMemberConstructor) {
	index := uint16(code.Code[packet.capturePC+1])<<8 | uint16(code.Code[packet.capturePC+2])
	insert := packet.delegatePC + 3
	write := []byte{core.OP_ALOAD_0, core.OP_ALOAD_2, core.OP_PUTFIELD, byte(index >> 8), byte(index)}
	code.Code = append(append(append([]byte(nil), code.Code[:insert]...), write...), code.Code[insert:]...)
	code.AttrLen += uint32(len(write))
}

func TestNativeMethodLocalMemberBindingSnapshotRechecksOriginalCapture(t *testing.T) {
	files := nativeCompileClasses(t, memberBindingSnapshotFixture)
	memberBindingOriginalOracle(t, files)
	for _, variant := range []string{
		"original", "capture overwritten after delegate", "capture final removed", "capture synthetic removed", "duplicate capture field",
		"duplicate method pointer", "duplicate method copy", "duplicate Code", "nil Code", "stale capture PC", "stale delegate PC",
		"stale delegate owner", "stale delegate descriptor", "stale source descriptor", "capture wrong CP kind", "delegate wrong CP kind",
		"delegate interface CP kind", "capture uses explicit alternate", "unreachable delegate witness", "multiple reachable delegates",
		"work budget", "allocation budget", "canceled",
	} {
		t.Run(variant, func(t *testing.T) {
			archive, prepared, child := memberBindingPrepared(t, files)
			object := child.object
			const descriptor = "(LMemberBindingOuter;LMemberBindingOuter;Ljava/lang/Object;)V"
			method, code := memberBindingMethod(t, object, descriptor)
			packet := child.constructors[descriptor]
			if packet == nil || packet.capturePC < 0 {
				t.Fatal("missing original direct-capture packet")
			}
			owner := &nativeMethodLocalOwner{owner: object.GetClassName(), method: "<init>", descriptor: descriptor, declaration: method}
			reader := NewClassObjectDumper(object)
			reader.nativeMemberRoot, reader.nativeMemberCurrent = prepared.family, child
			oldLength := len(code.Code)
			captureIndex := uint16(code.Code[packet.capturePC+1])<<8 | uint16(code.Code[packet.capturePC+2])
			invokeIndex := uint16(code.Code[packet.delegatePC+1])<<8 | uint16(code.Code[packet.delegatePC+2])
			var work *workbudget.Budget
			switch variant {
			case "capture overwritten after delegate":
				memberBindingAppendOverwrite(code, packet)
				// The physical constructor and capture/parent call PCs stay valid;
				// the JVM observes the later alternate instance, not hidden port 1.
				memberBindingMutantOracle(t, files, object)
			case "capture final removed", "capture synthetic removed", "duplicate capture field":
				for _, field := range object.Fields {
					name, _ := sourceBridgeUTF8(object, field.NameIndex)
					if name != child.field {
						continue
					}
					if variant == "capture final removed" {
						field.AccessFlags &^= 0x10
					} else if variant == "capture synthetic removed" {
						field.AccessFlags &^= 0x1000
					} else {
						object.Fields = append(object.Fields, field)
					}
					break
				}
			case "duplicate method pointer":
				object.Methods = append(object.Methods, method)
			case "duplicate method copy":
				duplicate := *method
				object.Methods = append(object.Methods, &duplicate)
			case "duplicate Code":
				method.Attributes = append(method.Attributes, code)
			case "nil Code":
				method.Attributes = append(method.Attributes, (*CodeAttribute)(nil))
			case "stale capture PC":
				packet.capturePC++
			case "stale delegate PC":
				packet.delegatePC++
			case "stale delegate owner":
				packet.delegateOwner = object.GetClassName()
			case "stale delegate descriptor":
				packet.delegateDescriptor = "(Ljava/lang/Object;)V"
			case "stale source descriptor":
				packet.sourceDescriptor = "()V"
			case "capture wrong CP kind":
				ref := object.ConstantPool[captureIndex-1].(*ConstantFieldrefInfo)
				object.ConstantPool[captureIndex-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "delegate wrong CP kind":
				ref := object.ConstantPool[invokeIndex-1].(*ConstantMethodrefInfo)
				object.ConstantPool[invokeIndex-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "delegate interface CP kind":
				ref := object.ConstantPool[invokeIndex-1].(*ConstantMethodrefInfo)
				object.ConstantPool[invokeIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "capture uses explicit alternate":
				code.Code[packet.capturePC-1] = core.OP_ALOAD_2
			case "unreachable delegate witness":
				packet.delegatePC = len(code.Code) + 1
				code.Code = append(code.Code, core.OP_ALOAD_0, core.OP_INVOKESPECIAL, byte(invokeIndex>>8), byte(invokeIndex), core.OP_RETURN)
			case "multiple reachable delegates":
				code.Code = []byte{core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_PUTFIELD, byte(captureIndex >> 8), byte(captureIndex), core.OP_ALOAD_2, core.OP_IFNULL, 0, 10, core.OP_ALOAD_0, core.OP_INVOKESPECIAL, byte(invokeIndex >> 8), byte(invokeIndex), core.OP_GOTO, 0, 7, core.OP_ALOAD_0, core.OP_INVOKESPECIAL, byte(invokeIndex >> 8), byte(invokeIndex), core.OP_RETURN}
				packet.delegatePC = 10
			case "work budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant != "capture overwritten after delegate" {
				code.AttrLen = uint32(int(code.AttrLen) + len(code.Code) - oldLength)
			}
			reader.Work = work
			children, objects, getters := len(prepared.family.children), len(prepared.family.lexicalObjects), len(prepared.family.getters)
			got := nativeMethodLocalMemberBindingSnapshot(reader, owner, child, packet, work)
			wantSnapshot := variant == "original"
			if variant != "stale source descriptor" && got != wantSnapshot {
				t.Errorf("fresh original capture proof=%v want=%v", got, wantSnapshot)
			}
			// sourceDescriptor belongs to the committed caller packet. The
			// prefix caller must reject its mismatch even if physical roles match.
			prefix, known := nativeMethodLocalConstructorBindingPrefix(object, owner, prepared.family, work)
			if known != (variant == "original") || known && (len(prefix) != 1 || prefix[0] != "LMemberBindingOuter;") {
				t.Errorf("binding prefix=%v known=%v", prefix, known)
			}
			if children != len(prepared.family.children) || objects != len(prepared.family.lexicalObjects) || getters != len(prepared.family.getters) || archive.nativeMembersBytes != 0 || archive.sourceOwnership.bytes != 0 {
				t.Fatal("binding-only proof changed source/private ownership")
			}
		})
	}
}

func TestNativeMethodLocalMemberBindingSnapshotClosesFreshThisTargets(t *testing.T) {
	source := strings.Replace(memberBindingSnapshotFixture, "final MemberBindingRead view;", "MemberBindingRead view;", 1)
	source = strings.Replace(source, "<T>Inner(MemberBindingOuter alternate,final T captured){", "<T>Inner(MemberBindingOuter alternate,final T captured){this(alternate,captured,true);", 1)
	source = strings.Replace(source, "view=new Scope();}MemberBindingOuter outer()", "view=new Scope();}private Inner(MemberBindingOuter alternate,Object value,boolean ignored){}MemberBindingOuter outer()", 1)
	files := nativeCompileClasses(t, source)
	memberBindingOriginalOracle(t, files)
	for _, variant := range []string{"original", "target capture overwrite", "duplicate target method pointer", "duplicate target method copy", "duplicate target Code", "nil target Code"} {
		t.Run(variant, func(t *testing.T) {
			archive, prepared, child := memberBindingPrepared(t, files)
			object := child.object
			const descriptor = "(LMemberBindingOuter;LMemberBindingOuter;Ljava/lang/Object;)V"
			const targetDescriptor = "(LMemberBindingOuter;LMemberBindingOuter;Ljava/lang/Object;Z)V"
			method, code := memberBindingMethod(t, object, descriptor)
			targetMethod, targetCode := memberBindingMethod(t, object, targetDescriptor)
			packet, targetPacket := child.constructors[descriptor], child.constructors[targetDescriptor]
			if packet == nil || targetPacket == nil || packet.capturePC >= 0 {
				t.Fatal("missing original THIS-delegation packets")
			}
			originalCode := append([]byte(nil), code.Code...)
			owner := &nativeMethodLocalOwner{owner: object.GetClassName(), method: "<init>", descriptor: descriptor, declaration: method}
			reader := NewClassObjectDumper(object)
			reader.nativeMemberRoot, reader.nativeMemberCurrent = prepared.family, child
			switch variant {
			case "target capture overwrite":
				memberBindingAppendOverwrite(targetCode, targetPacket)
				memberBindingMutantOracle(t, files, object)
			case "duplicate target method pointer":
				object.Methods = append(object.Methods, targetMethod)
			case "duplicate target method copy":
				duplicate := *targetMethod
				object.Methods = append(object.Methods, &duplicate)
			case "duplicate target Code":
				targetMethod.Attributes = append(targetMethod.Attributes, targetCode)
			case "nil target Code":
				targetMethod.Attributes = append(targetMethod.Attributes, (*CodeAttribute)(nil))
			}
			if !bytes.Equal(code.Code, originalCode) {
				t.Fatal("focused constructor must remain unchanged in target mutations")
			}
			got := nativeMethodLocalMemberBindingSnapshot(reader, owner, child, packet, nil)
			if got != (variant == "original") {
				t.Errorf("fresh THIS target proof=%v", got)
			}
			prefix, known := nativeMethodLocalConstructorBindingPrefix(object, owner, prepared.family, nil)
			if known != (variant == "original") || known && (len(prefix) != 1 || prefix[0] != "LMemberBindingOuter;") {
				t.Errorf("THIS binding prefix=%v known=%v", prefix, known)
			}
			if archive.nativeMembersBytes != 0 || archive.sourceOwnership.bytes != 0 {
				t.Fatal("binding-only THIS proof published source")
			}
		})
	}
}
