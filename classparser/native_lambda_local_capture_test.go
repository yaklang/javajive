package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaLocalCapturePhiMustRetainOneOriginalStore(t *testing.T) {
	files := nativeCompileClasses(t, `class TypedPhiCaptureWords {
 static int integer(boolean choose,int first,int second){int local=choose?first:second;return local;}
 static long wide(boolean choose,long first,long second){long local=choose?first:second;return local;}
 static float fractional(boolean choose,float first,float second){float local=choose?first:second;return local;}
 static double precise(boolean choose,double first,double second){double local=choose?first:second;return local;}
 static Object reference(boolean choose,Object first,Object second){Object local=choose?first:second;return local;}
}`)
	object, err := Parse(files["TypedPhiCaptureWords.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "<init>" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
			arguments, result, err := callbinding.Descriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			params := map[int]string{}
			slot := 0
			for _, argument := range arguments {
				params[slot] = argument
				slot++
				if argument == "J" || argument == "D" {
					slot++
				}
			}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				decoder := core.NewDecompiler(code.Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				flow := nativeEnumParameterOriginalFlow(decoder, code, nil)
				reader := NewClassObjectDumper(object)
				reads, known := reader.nativeTypedLocalReads(method, code, flow, params, true)
				if !known || len(reads) != 1 {
					t.Fatalf("one original STORE for the selected word: known=%v reads=%v", known, reads)
				}
				for _, read := range reads {
					if read.descriptor != result || !nativeLambdaLocalSingleStore(constructorMotionOps(decoder), read, nil) {
						t.Fatal("lost original typed STORE/LOAD")
					}
				}
				// The older enum admission is intentionally unchanged: phi
				// source publication there requires a different certificate.
				older, known := reader.nativeEnumLocalSelectorReads(method, code, flow, params)
				if !known || len(older) != 0 {
					t.Fatal("lambda proof broadened enum selector admission")
				}
			}
		})
	}
}

// Instruction-only cases isolate word interval writes; they do not claim JVM
// verifier validity. Actual verifier frames and source bindings are separate.
func TestNativeLambdaLocalCaptureSingleStoreRejectsSlotReuse(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		want bool
	}{
		{"original", []byte{core.OP_ASTORE_2, core.OP_ALOAD_2, core.OP_RETURN}, true},
		{"disjoint store", []byte{core.OP_ASTORE_2, core.OP_ISTORE_3, core.OP_ALOAD_2, core.OP_RETURN}, true},
		{"disjoint wide store", []byte{core.OP_ASTORE_2, core.OP_WIDE, core.OP_ISTORE, 1, 0, core.OP_ALOAD_2, core.OP_RETURN}, true},
		{"second reference assignment", []byte{core.OP_ASTORE_2, core.OP_ASTORE_2, core.OP_ALOAD_2, core.OP_RETURN}, false},
		{"later reuse", []byte{core.OP_ASTORE_2, core.OP_ALOAD_2, core.OP_ISTORE_2, core.OP_RETURN}, false},
		{"wide lower overlap", []byte{core.OP_ASTORE_2, core.OP_LSTORE_1, core.OP_ALOAD_2, core.OP_RETURN}, false},
		{"wide upper overlap", []byte{core.OP_ASTORE_2, core.OP_DSTORE_2, core.OP_ALOAD_2, core.OP_RETURN}, false},
		{"increment", []byte{core.OP_ASTORE_2, core.OP_IINC, 2, 1, core.OP_ALOAD_2, core.OP_RETURN}, false},
		{"wide increment", []byte{core.OP_ASTORE_2, core.OP_WIDE, core.OP_IINC, 0, 2, 0, 1, core.OP_ALOAD_2, core.OP_RETURN}, false},
		{"no store", []byte{core.OP_ALOAD_2, core.OP_RETURN}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := core.NewDecompiler(tc.code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			read := &nativeEnumLocalRead{slot: 2, storePC: 0}
			if got := nativeLambdaLocalSingleStore(constructorMotionOps(decoder), read, nil); got != tc.want {
				t.Fatalf("single store=%v want %v", got, tc.want)
			}
		})
	}
}

func TestNativeLambdaLocalCaptureCategoryTwoOwnsBothSlots(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		want bool
	}{
		{"original", []byte{core.OP_LSTORE_2, core.OP_LLOAD_2, core.OP_RETURN}, true},
		{"disjoint before", []byte{core.OP_LSTORE_2, core.OP_ISTORE_1, core.OP_LLOAD_2, core.OP_RETURN}, true},
		{"disjoint after", []byte{core.OP_LSTORE_2, core.OP_ISTORE, 4, core.OP_LLOAD_2, core.OP_RETURN}, true},
		{"head overwrite", []byte{core.OP_LSTORE_2, core.OP_ISTORE_2, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"tail overwrite", []byte{core.OP_LSTORE_2, core.OP_ISTORE_3, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"tail increment", []byte{core.OP_LSTORE_2, core.OP_IINC, 3, 1, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"wide tail overwrite", []byte{core.OP_LSTORE_2, core.OP_WIDE, core.OP_ISTORE, 0, 3, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"lower wide overlap", []byte{core.OP_LSTORE_2, core.OP_DSTORE_1, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"upper wide overlap", []byte{core.OP_LSTORE_2, core.OP_DSTORE_3, core.OP_LLOAD_2, core.OP_RETURN}, false},
		{"different computational word", []byte{core.OP_DSTORE_2, core.OP_LLOAD_2, core.OP_RETURN}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := core.NewDecompiler(tc.code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			// This helper proves write intervals only. Typed frame evidence
			// independently refuses the different computational word above.
			read := &nativeEnumLocalRead{slot: 2, storePC: 0, descriptor: "J"}
			if got := nativeLambdaLocalSingleStore(constructorMotionOps(decoder), read, nil); got != tc.want {
				t.Fatalf("category-two single store=%v want %v", got, tc.want)
			}
		})
	}
}

func TestNativeLambdaLocalCaptureSourceRetainsEachOriginalDeclaration(t *testing.T) {
	variants := []string{"original", "renamed", "swapped equal types", "missing capture", "extra capture", "nil value", "replacement seed", "missing declaration", "wrong store", "wrong slot", "custom text", "stack alias", "wrong type", "foreign method", "foreign code", "nil site", "nil source", "nil method", "nil code", "invalid factory pc", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("CaptureBox")
			method, code := &MemberInfo{}, &CodeAttribute{Code: make([]byte, 24)}
			site := &nativeLambdaLocalCaptureSite{method: method, code: code, pc: 20}
			source := &nativeLambdaLocalCaptureSource{method: method, code: code}
			refs := []*values.JavaRef{}
			for i := 0; i < 2; i++ {
				seed := values.NewJavaLiteral(nil, typ)
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
				pc, slot := 3+i*3, 2+i
				if variant == "wrong store" && i == 0 {
					pc++
				}
				if variant == "wrong slot" && i == 0 {
					slot++
				}
				if variant != "missing declaration" || i != 0 {
					ref.MarkOriginalLocalDeclaration(pc, slot, seed)
				}
				refs = append(refs, ref)
				source.values = append(source.values, ref)
				site.operands = append(site.operands, &nativeEnumSelectorProducer{opcode: core.OP_ALOAD, pc: 18 + i, slot: 2 + i, result: "LCaptureBox;", local: &nativeEnumLocalRead{pc: 18 + i, opcode: core.OP_ALOAD, slot: 2 + i, storePC: 3 + i*3, descriptor: "LCaptureBox;"}})
			}
			var work *workbudget.Budget
			switch variant {
			case "renamed":
				refs[0].Id = coreutils.NewRootVariableId()
			case "swapped equal types":
				source.values[0], source.values[1] = source.values[1], source.values[0]
			case "missing capture":
				source.values = source.values[:1]
			case "extra capture":
				source.values = append(source.values, refs[0])
			case "nil value":
				source.values[0] = nil
			case "replacement seed":
				refs[0].Val = values.NewJavaLiteral(nil, typ)
			case "custom text":
				refs[0].CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "same" }, func() types.JavaType { return typ })
			case "stack alias":
				refs[0].StackVar = refs[0].Val
			case "wrong type":
				source.values[0] = values.NewJavaRef(coreutils.NewRootVariableId(), refs[0].Val, types.NewJavaClass("Other"))
			case "foreign method":
				copy := *method
				source.method = &copy
			case "foreign code":
				copy := *code
				source.code = &copy
			case "nil site":
				site = nil
			case "nil source":
				source = nil
			case "nil method":
				site.method = nil
				source.method = nil
			case "nil code":
				site.code = nil
				source.code = nil
			case "invalid factory pc":
				site.pc = 24
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "renamed"
			if got := nativeLambdaLocalCaptureSourceClosed(site, source, work); got != want {
				t.Fatalf("source binding=%v want %v", got, want)
			}
		})
	}
}

func TestNativeLambdaLocalCaptureRecordDefersDeclarationUntilFullRender(t *testing.T) {
	typ := types.NewJavaClass("CaptureBox")
	seed := values.NewJavaLiteral(nil, typ)
	ref := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
	method := &MemberInfo{}
	code := &CodeAttribute{Code: make([]byte, 24)}
	object := &ClassObject{}
	site := &nativeLambdaLocalCaptureSite{method: method, code: code, pc: 20, operands: []*nativeEnumSelectorProducer{{opcode: core.OP_ALOAD, pc: 19, slot: 3, result: "LCaptureBox;", local: &nativeEnumLocalRead{pc: 19, opcode: core.OP_ALOAD, slot: 3, storePC: 8, descriptor: "LCaptureBox;"}}}}
	child := &nativeMemberClass{object: object, lambdaContext: nativeLambdaImplementationContext{localCaptures: map[string]*nativeLambdaLocalCaptureSite{"implementation()V": site}}}
	dumper := NewClassObjectDumper(object)
	dumper.FuncCtx = &class_context.ClassContext{}
	dumper.nativeMemberCurrent, dumper.CurrentMethod = child, method
	if !dumper.recordNativeLambdaLocalCaptureSource("implementation", "()V", code, []values.JavaValue{ref}) {
		t.Fatal("record original operands during stack simulation")
	}
	record := dumper.nativeLambdaLocalSources["implementation()V"]
	if nativeLambdaLocalCaptureSourceClosed(site, record, nil) {
		t.Fatal("no STORE declaration has been rendered")
	}
	ref.MarkOriginalLocalDeclaration(8, 3, seed)
	if !nativeLambdaLocalCaptureSourceClosed(site, record, nil) {
		t.Fatal("full render retains the original declaration")
	}
	if dumper.recordNativeLambdaLocalCaptureSource("implementation", "()V", code, []values.JavaValue{ref}) {
		t.Fatal("second body use cannot borrow the exclusive site")
	}
	ref.Val = values.NewJavaLiteral(nil, typ)
	if nativeLambdaLocalCaptureSourceClosed(site, record, nil) {
		t.Fatal("post-render value replacement must invalidate the certificate")
	}
}

func TestNativeLambdaLocalCaptureSnapshotRetainsFactoryPositionAndOriginalSource(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "no witness", "no snapshot declaration", "wrong factory pc", "wrong operand position", "replaced snapshot seed", "rebound source local", "changed local seed", "source custom", "snapshot custom", "snapshot stack", "foreign uid", "wrong snapshot type"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("CaptureBox")
			value := values.NewJavaLiteral(nil, typ)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), value, typ)
			local.MarkOriginalLocalDeclaration(8, 3, value)
			seed := values.NewSlotValue(local, typ)
			snapshot := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
			pc, index := 20, 0
			if variant == "wrong factory pc" {
				pc++
			}
			if variant == "wrong operand position" {
				index++
			}
			if variant != "no witness" {
				snapshot.MarkOriginalDynamicOperand(pc, index, seed)
			}
			if variant != "no snapshot declaration" {
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, seed)
			}
			switch variant {
			case "renamed":
				snapshot.Id, local.Id = coreutils.NewRootVariableId(), coreutils.NewRootVariableId()
			case "replaced snapshot seed":
				snapshot.Val = values.NewSlotValue(local, typ)
			case "rebound source local":
				other := values.NewJavaRef(coreutils.NewRootVariableId(), value, typ)
				other.MarkOriginalLocalDeclaration(12, 4, value)
				seed.ResetValue(other)
			case "changed local seed":
				local.Val = values.NewJavaLiteral(nil, typ)
			case "source custom":
				local.CustomValue = &values.CustomValue{}
			case "snapshot custom":
				snapshot.CustomValue = &values.CustomValue{}
			case "snapshot stack":
				snapshot.StackVar = seed
			case "foreign uid":
				snapshot.VarUid += "other"
			case "wrong snapshot type":
				snapshot.ResetVarType(types.NewJavaClass("Other"))
			}
			original := &nativeEnumSelectorProducer{opcode: core.OP_ALOAD, pc: 19, slot: 3, result: "LCaptureBox;", local: &nativeEnumLocalRead{pc: 19, opcode: core.OP_ALOAD, slot: 3, storePC: 8, descriptor: "LCaptureBox;"}}
			if got := nativeLambdaLocalCaptureValue(original, snapshot, 20, 0, &class_context.ClassContext{}, nil); got != (variant == "original" || variant == "renamed") {
				t.Fatalf("original snapshot=%v", got)
			}
		})
	}
}

func TestNativeLambdaLocalCaptureTypedReadsKeepCompletePrimitiveWords(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class TypedCaptureWords {
 static int integer(int input){int local=input^0x76543210;return local;}
 static long wide(long input){long local=input^0x123456789abcdefL;return local;}
 static float fractional(int input){float local=Float.intBitsToFloat(input);return local;}
 static double precise(long input){double local=Double.longBitsToDouble(input);return local;}
}`, "none")
	object, err := Parse(files["TypedCaptureWords.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "<init>" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			params, want := map[int]string{}, ""
			switch name {
			case "integer":
				params[0], want = "I", "I"
			case "wide":
				params[0], want = "J", "J"
			case "fractional":
				params[0], want = "I", "F"
			case "precise":
				params[0], want = "J", "D"
			}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				decoder := core.NewDecompiler(code.Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				flow := nativeEnumParameterOriginalFlow(decoder, code, nil)
				reader := NewClassObjectDumper(object)
				reads, known := reader.nativeTypedLocalReads(method, code, flow, params, true)
				if !known || len(reads) != 1 {
					t.Fatalf("original typed local reads=%v known=%v", reads, known)
				}
				for pc, read := range reads {
					if pc != read.pc || read.descriptor != want || !nativeLambdaLocalSingleStore(constructorMotionOps(decoder), read, nil) {
						t.Fatalf("lost complete word %+v", read)
					}
				}
				refs, known := reader.nativeEnumLocalSelectorReads(method, code, flow, params)
				if !known || len(refs) != 0 {
					t.Fatal("reference selector admitted primitive words")
				}
			}
		})
	}
}
