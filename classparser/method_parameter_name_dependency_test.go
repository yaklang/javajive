package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialMethodParameterNameCacheCPOnlyMutationCannotReuseSource(t *testing.T) {
	source := `class GuardOwner{static int read(int originalNamed){return originalNamed+1000000;}}class GuardDriver{public static void main(String[]args)throws Exception{int expected=args.length==0?1000007:Integer.parseInt(args[0]);if(GuardOwner.read(7)!=expected||!GuardOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getName().equals("originalNamed"))throw new AssertionError("original CP operand");System.out.println("boundary:original");}}`
	files := methodParameterBoundaryCompile(t, source, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	packet, err := d.DumpMethod("read", "(I)I")
	if err != nil || packet == nil {
		t.Fatal("original named packet", err)
	}
	beforeCode := append([]byte(nil), methodCodeBytes(packet.member)...)
	beforeNames := methodParameterNamesIndependent(t, d.obj.Bytes())
	changed := 0
	for _, constant := range d.obj.ConstantPool {
		if literal, ok := constant.(*ConstantIntegerInfo); ok && literal.Value == 1000000 {
			literal.Value = 2000000
			changed++
		}
	}
	if changed != 1 || !bytes.Equal(beforeCode, methodCodeBytes(packet.member)) {
		t.Fatal("CP-only authored mutation changed original Code")
	}
	after := d.obj.Bytes()
	afterNames := methodParameterNamesIndependent(t, after)
	if len(beforeNames) != len(afterNames) {
		t.Fatal("CP-only mutation changed MP table")
	}
	for key, rows := range beforeNames {
		got := afterNames[key]
		if len(got) != len(rows) {
			t.Fatal("CP-only mutation changed MP arity")
		}
		for i, row := range rows {
			if row != got[i] {
				t.Fatal("CP-only mutation changed MP payload")
			}
		}
	}
	dir := methodParameterNameDirectory(t, "cp-mutated-original")
	for name, raw := range files {
		if name == "GuardOwner.class" {
			raw = after
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, java := t04Tools(t)
	out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "GuardDriver", "2000007").CombinedOutput()
	t.Logf("CP-only mutated original JVM: %q", out)
	if err != nil || string(out) != "boundary:original\n" {
		t.Fatalf("valid original CP semantics: %v\n%s", err, out)
	}
	if got, err := d.DumpMethod("read", "(I)I"); err == nil || got != nil {
		t.Fatalf("CP-only changed input reused stale named source: err=%v packet=%+v", err, got)
	}
	if d.dumpedMethodsSet["name:read,desc:(I)I"] != packet {
		t.Fatal("stale rejection overwrote sealed original packet")
	}
}

func TestAdversarialMethodSourceRequestRetryCannotRefundIdentity(t *testing.T) {
	t.Run("strictly increasing", func(t *testing.T) {
		d := &ClassObjectDumper{}
		first, ok := d.newLambdaMethodSourceRequest(1)
		if !ok {
			t.Fatal("first request")
		}
		rollback := d.snapshotRetryState()
		middle, ok := d.newLambdaMethodSourceRequest(1)
		if !ok {
			t.Fatal("middle request")
		}
		rollback()
		last, ok := d.newLambdaMethodSourceRequest(1)
		if !ok || first.serial >= middle.serial || middle.serial >= last.serial {
			t.Fatalf("retry reused spent request identity: %d %d %d", first.serial, middle.serial, last.serial)
		}
	})
	t.Run("finite request capacity is spent", func(t *testing.T) {
		d := &ClassObjectDumper{methodSourceRequestSerial: methodSourceLambdaRequestLimit - 1}
		rollback := d.snapshotRetryState()
		last, ok := d.newLambdaMethodSourceRequest(0)
		if !ok || last.serial != methodSourceLambdaRequestLimit {
			t.Fatal("last bounded request")
		}
		rollback()
		if _, ok := d.newLambdaMethodSourceRequest(0); ok || d.methodSourceRequestSerial != methodSourceLambdaRequestLimit {
			t.Fatal("failed retry refunded callback capacity")
		}
	})
}

func TestAdversarialMethodParameterNameCacheHeaderInputsStayFresh(t *testing.T) {
	source := `class GuardOwner{static<T>T read(T originalNamed)throws java.io.IOException{return originalNamed;}static<U>U other(U otherNamed)throws java.lang.ReflectiveOperationException{return otherNamed;}}class GuardDriver{public static void main(String[]args)throws Exception{Object token=new Object();if(GuardOwner.read(token)!=token||!GuardOwner.class.getDeclaredMethod("read",Object.class).getParameters()[0].getName().equals("originalNamed")||!GuardOwner.class.getDeclaredMethod("read",Object.class).getTypeParameters()[0].getName().equals("T")||GuardOwner.class.getDeclaredMethod("read",Object.class).getExceptionTypes()[0]!=java.io.IOException.class)throw new AssertionError("original header");System.out.println("boundary:original");}}`
	files := methodParameterBoundaryCompile(t, source, "GuardOwner")
	for _, variant := range []string{"Signature index", "Signature length", "Exceptions index", "Exceptions length", "duplicate Signature", "duplicate Exceptions"} {
		t.Run(variant, func(t *testing.T) {
			d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
			cached, err := d.DumpMethod("read", "(Ljava/lang/Object;)Ljava/lang/Object;")
			if err != nil || cached == nil {
				t.Fatal("original header", err)
			}
			var signature, otherSignature *SignatureAttribute
			var throws, otherThrows *ExceptionsAttribute
			for _, method := range d.obj.Methods {
				name, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
				for _, a := range method.Attributes {
					switch a := a.(type) {
					case *SignatureAttribute:
						if name == "read" {
							signature = a
						} else if name == "other" {
							otherSignature = a
						}
					case *ExceptionsAttribute:
						if name == "read" {
							throws = a
						} else if name == "other" {
							otherThrows = a
						}
					}
				}
			}
			if signature == nil || otherSignature == nil || signature.SignatureIndex == otherSignature.SignatureIndex || throws == nil || otherThrows == nil || len(throws.ExceptionIndexTable) != 1 || len(otherThrows.ExceptionIndexTable) != 1 || throws.ExceptionIndexTable[0] == otherThrows.ExceptionIndexTable[0] {
				t.Fatal("independent original header inputs")
			}
			beforeCode := append([]byte(nil), methodCodeBytes(cached.member)...)
			beforePool, ok := d.methodParameterNamePoolStamp()
			if !ok {
				t.Fatal("original pool seal")
			}
			switch variant {
			case "Signature index":
				signature.SignatureIndex = otherSignature.SignatureIndex
			case "Signature length":
				signature.AttrLen++
			case "Exceptions index":
				throws.ExceptionIndexTable[0] = otherThrows.ExceptionIndexTable[0]
			case "Exceptions length":
				throws.AttrLen++
			case "duplicate Signature":
				cached.member.Attributes = append(cached.member.Attributes, signature)
			case "duplicate Exceptions":
				cached.member.Attributes = append(cached.member.Attributes, throws)
			}
			afterPool, ok := d.methodParameterNamePoolStamp()
			if !ok || beforePool != afterPool || !bytes.Equal(beforeCode, methodCodeBytes(cached.member)) {
				t.Fatal("header-only mutation changed CP or Code")
			}
			if result, err := d.DumpMethod("read", "(Ljava/lang/Object;)Ljava/lang/Object;"); err == nil || result != nil {
				t.Fatal("changed actual header input reused named packet")
			}
		})
	}
}

func TestAdversarialMethodParameterNamePoolSealCoversTypedPayload(t *testing.T) {
	for _, kind := range []string{"utf8 value", "utf8 units", "integer", "float bits", "long", "double bits", "class", "string", "field", "method", "interface method", "name and type", "method handle", "method type", "dynamic", "invoke dynamic", "module", "package"} {
		t.Run(kind, func(t *testing.T) {
			var constant ConstantInfo
			var mutate func()
			switch kind {
			case "utf8 value":
				v := NewUtf8FromString("original")
				constant = v
				mutate = func() { v.Value = "changed" }
			case "utf8 units":
				v := NewUtf8FromString("original")
				constant = v
				mutate = func() { v.Units[0]++ }
			case "integer":
				v := &ConstantIntegerInfo{Value: 1}
				constant = v
				mutate = func() { v.Value++ }
			case "float bits":
				v := &ConstantFloatInfo{Value: 0}
				constant = v
				mutate = func() { v.Value = math.Float32frombits(0x80000000) }
			case "long":
				v := &ConstantLongInfo{Value: 1}
				constant = v
				mutate = func() { v.Value++ }
			case "double bits":
				v := &ConstantDoubleInfo{Value: 0}
				constant = v
				mutate = func() { v.Value = math.Float64frombits(0x8000000000000000) }
			case "class":
				v := &ConstantClassInfo{NameIndex: 1}
				constant = v
				mutate = func() { v.NameIndex++ }
			case "string":
				v := &ConstantStringInfo{StringIndex: 1}
				constant = v
				mutate = func() { v.StringIndex++ }
			case "field":
				v := &ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: 1, NameAndTypeIndex: 1}}
				constant = v
				mutate = func() { v.ClassIndex++ }
			case "method":
				v := &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: 1, NameAndTypeIndex: 1}}
				constant = v
				mutate = func() { v.NameAndTypeIndex++ }
			case "interface method":
				v := &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: 1, NameAndTypeIndex: 1}}
				constant = v
				mutate = func() { v.ClassIndex++ }
			case "name and type":
				v := &ConstantNameAndTypeInfo{NameIndex: 1, DescriptorIndex: 1}
				constant = v
				mutate = func() { v.DescriptorIndex++ }
			case "method handle":
				v := &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: 1}
				constant = v
				mutate = func() { v.ReferenceKind++ }
			case "method type":
				v := &ConstantMethodTypeInfo{DescriptorIndex: 1}
				constant = v
				mutate = func() { v.DescriptorIndex++ }
			case "dynamic":
				v := &ConstantDynamicInfo{BootstrapMethodAttrIndex: 1, NameAndTypeIndex: 1}
				constant = v
				mutate = func() { v.BootstrapMethodAttrIndex++ }
			case "invoke dynamic":
				v := &ConstantInvokeDynamicInfo{BootstrapMethodAttrIndex: 1, NameAndTypeIndex: 1}
				constant = v
				mutate = func() { v.NameAndTypeIndex++ }
			case "module":
				v := &ConstantModuleInfo{NameIndex: 1}
				constant = v
				mutate = func() { v.NameIndex++ }
			case "package":
				v := &ConstantPackageInfo{NameIndex: 1}
				constant = v
				mutate = func() { v.NameIndex++ }
			}
			pool := []ConstantInfo{constant}
			if kind == "long" || kind == "double bits" {
				pool = append(pool, nil)
			}
			d := &ClassObjectDumper{obj: &ClassObject{ConstantPool: pool}, ConstantPool: pool}
			before, ok := d.methodParameterNamePoolStamp()
			if !ok {
				t.Fatal("known typed original payload")
			}
			mutate()
			after, ok := d.methodParameterNamePoolStamp()
			if !ok || before == after {
				t.Fatal("typed payload mutation not sealed")
			}
		})
	}
	for _, which := range []string{"object pool", "renderer pool"} {
		t.Run(which, func(t *testing.T) {
			original := &ConstantIntegerInfo{Value: 1}
			renderer := &ConstantIntegerInfo{Value: 1}
			d := &ClassObjectDumper{obj: &ClassObject{ConstantPool: []ConstantInfo{original}}, ConstantPool: []ConstantInfo{renderer}}
			before, ok := d.methodParameterNamePoolStamp()
			if !ok {
				t.Fatal("dual pool original")
			}
			if which == "object pool" {
				original.Value++
			} else {
				renderer.Value++
			}
			after, ok := d.methodParameterNamePoolStamp()
			if !ok || before == after {
				t.Fatal("one actual CP input was omitted")
			}
		})
	}
}

func TestAdversarialMethodParameterNamePoolSealAlwaysBounded(t *testing.T) {
	for _, variant := range []string{"nil work aggregate payload", "entry count", "work", "memory", "unknown kind", "typed nil", "reserved hole"} {
		t.Run(variant, func(t *testing.T) {
			pool := []ConstantInfo{&ConstantIntegerInfo{Value: 1}}
			d := &ClassObjectDumper{obj: &ClassObject{ConstantPool: pool}, ConstantPool: pool}
			switch variant {
			case "nil work aggregate payload":
				pool = nil
				for i := 0; i < 300; i++ {
					pool = append(pool, &ConstantUtf8Info{Value: strings.Repeat("x", 32768)})
				}
				d.obj.ConstantPool = pool
				d.ConstantPool = pool
			case "entry count":
				d.obj.ConstantPool = make([]ConstantInfo, 65535)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "unknown kind":
				d.obj.ConstantPool = []ConstantInfo{&methodParameterUnknownConstant{}}
			case "typed nil":
				d.obj.ConstantPool = []ConstantInfo{(*ConstantUtf8Info)(nil)}
			case "reserved hole":
				d.obj.ConstantPool = []ConstantInfo{nil}
			}
			if _, ok := d.methodParameterNamePoolStamp(); ok {
				t.Fatal("unbounded or unexpressible payload minted a named source seal")
			}
		})
	}
}

type methodParameterUnknownConstant struct{}

func (*methodParameterUnknownConstant) readInfo(*ClassParser) {}
