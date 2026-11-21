package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	dexAccInterface = 0x0200
	dexAccAbstract  = 0x0400
)

func resolveCPUTF8(pool []classparser.ConstantInfo, index uint16) string {
	if index == 0 || int(index) > len(pool) {
		return ""
	}
	if u, ok := pool[index-1].(*classparser.ConstantUtf8Info); ok {
		return u.Value
	}
	return ""
}

func resolveCPClass(pool []classparser.ConstantInfo) func(uint16) string {
	return func(idx uint16) string {
		if idx == 0 || int(idx) > len(pool) {
			return ""
		}
		if c, ok := pool[idx-1].(*classparser.ConstantClassInfo); ok {
			return resolveCPUTF8(pool, c.NameIndex)
		}
		return ""
	}
}

func verifyClassWithFrameTransfer(t *testing.T, raw []byte) *classparser.ClassObject {
	t.Helper()
	parsed, err := classparser.Parse(raw)
	if err != nil {
		t.Fatalf("classparser.Parse: %v", err)
	}
	resolveClass := resolveCPClass(parsed.ConstantPool)

	for _, method := range parsed.Methods {
		name := resolveCPUTF8(parsed.ConstantPool, method.NameIndex)
		desc := resolveCPUTF8(parsed.ConstantPool, method.DescriptorIndex)
		var codeAttr *classparser.CodeAttribute
		for _, attr := range method.Attributes {
			if ca, ok := attr.(*classparser.CodeAttribute); ok {
				codeAttr = ca
				break
			}
		}
		if codeAttr == nil {
			continue
		}

		d := core.NewDecompiler(codeAttr.Code, func(i int) values.JavaValue {
			return classparser.GetValueFromCP(parsed.ConstantPool, i)
		})
		d.ConstantPoolLiteralGetter = func(i int) values.JavaValue {
			return classparser.GetLiteralFromCP(parsed.ConstantPool, i)
		}
		d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
			if index <= 0 || index > len(parsed.ConstantPool) {
				return 0, "", ""
			}
			dynamic, ok := parsed.ConstantPool[index-1].(*classparser.ConstantInvokeDynamicInfo)
			if !ok || dynamic == nil || dynamic.NameAndTypeIndex == 0 || int(dynamic.NameAndTypeIndex) > len(parsed.ConstantPool) {
				return 0, "", ""
			}
			nat, ok := parsed.ConstantPool[dynamic.NameAndTypeIndex-1].(*classparser.ConstantNameAndTypeInfo)
			if !ok || nat == nil {
				return 0, "", ""
			}
			return dynamic.BootstrapMethodAttrIndex, resolveCPUTF8(parsed.ConstantPool, nat.NameIndex), resolveCPUTF8(parsed.ConstantPool, nat.DescriptorIndex)
		}
		limits := core.CodeLimits{
			Present:          true,
			MaxLocals:        int(codeAttr.MaxLocals),
			MaxStack:         int(codeAttr.MaxStack),
			DirectSuperClass: parsed.GetSupperClassName(),
		}
		d.CodeLimits = limits
		for _, h := range codeAttr.ExceptionTable {
			d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{
				StartPc: h.StartPc, EndPc: h.EndPc, HandlerPc: h.HandlerPc, CatchType: h.CatchType,
			})
		}

		mir, err := methodir.BuildFromBytes(codeAttr.Code, d.ExceptionTable, methodir.MethodMeta{
			Limits:     limits,
			ClassName:  parsed.GetClassName(),
			Name:       name,
			Descriptor: desc,
			Bytecode:   codeAttr.Code,
			IsStatic:   method.AccessFlags&0x0008 != 0,
		}, d)
		if err != nil {
			t.Fatalf("%s.%s%s: methodir.BuildFromBytes: %v", parsed.GetClassName(), name, desc, err)
		}

		// Provide reference type information for LDC MethodHandle / MethodType so frametransfer checks StackMapTable.
		for i := range mir.Instrs {
			op := mir.Instrs[i].Opcode
			if (op == core.OP_LDC || op == core.OP_LDC_W) && mir.Instrs[i].Const.Kind == methodir.ConstNone {
				cpIdx := int(mir.Instrs[i].CPIndex)
				if cpIdx > 0 && cpIdx <= len(parsed.ConstantPool) {
					switch parsed.ConstantPool[cpIdx-1].(type) {
					case *classparser.ConstantMethodHandleInfo:
						mir.Instrs[i].Const = methodir.Const{Kind: methodir.ConstClass, Class: "java/lang/invoke/MethodHandle"}
					case *classparser.ConstantMethodTypeInfo:
						mir.Instrs[i].Const = methodir.Const{Kind: methodir.ConstClass, Class: "java/lang/invoke/MethodType"}
					}
				}
			}
		}

		fn, err := ssabuild.Build(mir, ssabuild.Options{})
		if err != nil {
			t.Fatalf("%s.%s%s: ssabuild.Build (frametransfer verification): %v", parsed.GetClassName(), name, desc, err)
		}

		var smtBytes []byte
		for _, sub := range codeAttr.Attributes {
			if u, ok := sub.(*classparser.UnparsedAttribute); ok && u.Name == "StackMapTable" {
				smtBytes = u.Info
				break
			}
		}

		computed := make(map[uint16]frametransfer.Frame, len(fn.Blocks))
		for _, bf := range fn.Blocks {
			if bf.Reachable {
				computed[bf.First] = bf.In
			}
		}
		if err := frametransfer.CheckStackMapTable(parsed.MajorVersion, smtBytes, computed, resolveClass); err != nil {
			t.Fatalf("%s.%s%s (major=%d): CheckStackMapTable: %v", parsed.GetClassName(), name, desc, parsed.MajorVersion, err)
		}

		if parsed.MajorVersion >= 51 {
			smtSet := make(map[uint16]bool)
			if len(smtBytes) > 0 {
				frames, parseErr := frametransfer.ParseStackMapTable(smtBytes, resolveClass)
				if parseErr != nil {
					t.Fatalf("%s.%s%s: ParseStackMapTable: %v", parsed.GetClassName(), name, desc, parseErr)
				}
				for _, f := range frames {
					smtSet[f.Offset] = true
				}
			}
			inCount := make(map[uint16]int)
			hasJumpOrHandler := len(codeAttr.ExceptionTable) > 0
			for _, e := range mir.Edges {
				inCount[uint16(e.To)]++
				if e.Kind == core.EdgeTaken || e.Kind == core.EdgeCase || e.Kind == core.EdgeDefault || e.Kind == core.EdgeException {
					hasJumpOrHandler = true
				}
			}
			for _, h := range codeAttr.ExceptionTable {
				if !smtSet[h.HandlerPc] {
					t.Fatalf("%s.%s%s: missing StackMapTable frame at exception handler pc=%d", parsed.GetClassName(), name, desc, h.HandlerPc)
				}
			}
			if hasJumpOrHandler {
				for _, bf := range fn.Blocks {
					if !bf.Reachable || bf.First == 0 {
						continue
					}
					if inCount[bf.First] > 1 && !smtSet[bf.First] {
						t.Fatalf("%s.%s%s: missing StackMapTable frame at join block pc=%d", parsed.GetClassName(), name, desc, bf.First)
					}
				}
			}
		}
	}
	return parsed
}

func TestAdversarialP0WebSplitNarrowOverWideU0Pattern(t *testing.T) {
	// Reproduces Google Play Core b/a/a/b/a/a/u0.a bug:
	// v0 first holds a 64-bit long (`const-wide v0, 0x1020304050607L`).
	// Later v0 is overwritten with a narrow 32-bit int (`const/4 v0, 7`), and
	// before v0 is returned, v1 (the upper half of the stale wide register) is
	// overwritten (`const/4 v1, 2`). Before the fix, instructionDefWidth consulted
	// pre-instruction CategoryAt and treated the narrow def on v0 as width=2, so
	// writing v1 falsely killed v0's narrow definition and leaked an empty local slot.
	const owner = "Ladv/WebSplitU0;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{{Descriptor: owner}, {Descriptor: "Ljava/lang/Object;"}},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      model.MethodID{Class: owner, Name: "u0Like", ProtoIndex: 0},
				Code: &model.Code{
					RegistersSize: 4, // v0..v2 locals, v3 = arg0
					InsSize:       1,
					Instructions: []uint16{
						// 0: if-eqz v3, +9 -> offset 9
						0x0338, 0x0009,
						// 2: const-wide/32 v0, 0x00010005
						0x0017, 0x0005, 0x0001,
						// 5: add-long/2addr v0, v0
						0x00bb,
						// 6: const/4 v0, 7 (narrow overwrite on v0 after wide lifetime)
						0x7012,
						// 7: const/4 v1, 2 (write to v1 = reg+1 MUST NOT kill narrow v0!)
						0x2112,
						// 8: goto +3 -> offset 11
						0x0328,
						// 9: const/4 v0, 3
						0x3012,
						// 10: const/4 v1, 4
						0x4112,
						// 11: add-int/2addr v0, v1
						0x10b0,
						// 12: return v0
						0x000f,
					},
				},
			}},
		}},
	}
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	verifyClassWithFrameTransfer(t, raw)
	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "empty stack slot") || strings.Contains(src, "undecompilable") {
		t.Fatalf("decompilation failed on u0 pattern:\n%s", src)
	}
}

func TestAdversarialP0WebSplitHandlerPropagationAndDualEdge(t *testing.T) {
	const owner = "Ladv/HandlerDualEdge;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/Throwable;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "mayThrow", ProtoIndex: 0},
		},
	}
	// Block 0: v0 = 1, goto B2 (normal edge into handler block!).
	// Block 1 (try): invokes mayThrow(v1), returns v0 on fallthrough.
	// Block 2 (shared by normal edge from B0 and catch-all handler from B1!): adds 5 to v0 and returns.
	file.Classes = []model.Class{{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexAccessPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      file.Methods[0],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000f}},
			},
			{
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      model.MethodID{Class: owner, Name: "dualEdge", ProtoIndex: 0},
				Code: &model.Code{
					RegistersSize: 2, // v0 local, v1 param
					InsSize:       1,
					OutsSize:      1,
					Instructions: []uint16{
						// 0: const/4 v0, 1
						0x1012,
						// 1: if-eqz v1, +5 -> offset 6 (normal edge directly into handler!)
						0x0138, 0x0005,
						// 3: invoke-static {v1}, mayThrow (protected by try)
						0x1071, 0x0000, 0x0001,
						// 6 (handler target without move-exception + normal fallthrough/branch): add-int/lit8 v0, v0, 5
						0x00d8, 0x0500,
						// 8: return v0
						0x000f,
					},
					Tries: []model.Try{{
						StartAddress:     3,
						InstructionCount: 3,
						Handler:          model.CatchHandler{CatchAllAddress: ptrU32(6)},
					}},
				},
			},
		},
	}}
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	verifyClassWithFrameTransfer(t, raw)
	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "empty stack slot") || strings.Contains(src, "undecompilable") {
		t.Fatalf("decompilation failed on dual-edge handler:\n%s", src)
	}
}

func ptrU32(v uint32) *uint32 { return &v }

func fillProtoParamTypes(file *model.File) {
	for i := range file.Protos {
		if len(file.Protos[i].ParameterTypes) != len(file.Protos[i].Parameters) {
			file.Protos[i].ParameterTypes = make([]uint32, len(file.Protos[i].Parameters))
		}
	}
}

func TestAdversarialInvokeCustomStringConcatAndLambda(t *testing.T) {
	const owner = "Ladv/CustomCalls;"
	file := &model.File{
		Entry: "classes.dex",
		Strings: []string{
			"makeConcatWithConstants",
			"(\x01)",
			"metafactory",
			"get",
			"lambda$0",
		},
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "Ljava/lang/invoke/StringConcatFactory;"},
			{Descriptor: "Ljava/lang/invoke/LambdaMetafactory;"},
			{Descriptor: "Ljava/lang/invoke/MethodHandles$Lookup;"},
			{Descriptor: "Ljava/lang/invoke/MethodType;"},
			{Descriptor: "Ljava/lang/invoke/MethodHandle;"},
			{Descriptor: "Ljava/lang/invoke/CallSite;"},
			{Descriptor: "Ljava/util/function/Supplier;"},
		},
		Protos: []model.Proto{
			// 0: (Lookup, String, MethodType, String, [Object;)CallSite;
			{
				ReturnType: "Ljava/lang/invoke/CallSite;",
				Parameters: []string{"Ljava/lang/invoke/MethodHandles$Lookup;", "Ljava/lang/String;", "Ljava/lang/invoke/MethodType;", "Ljava/lang/String;", "[Ljava/lang/Object;"},
			},
			// 1: (I)String;
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"I"}},
			// 2: (Lookup, String, MethodType, MethodType, MethodHandle, MethodType)CallSite;
			{
				ReturnType: "Ljava/lang/invoke/CallSite;",
				Parameters: []string{"Ljava/lang/invoke/MethodHandles$Lookup;", "Ljava/lang/String;", "Ljava/lang/invoke/MethodType;", "Ljava/lang/invoke/MethodType;", "Ljava/lang/invoke/MethodHandle;", "Ljava/lang/invoke/MethodType;"},
			},
			// 3: ()Object;
			{ReturnType: "Ljava/lang/Object;"},
			// 4: ()Supplier;
			{ReturnType: "Ljava/util/function/Supplier;"},
			// 5: ()String;
			{ReturnType: "Ljava/lang/String;"},
			// 6: (I)Supplier;
			{ReturnType: "Ljava/util/function/Supplier;", Parameters: []string{"I"}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/invoke/StringConcatFactory;", Name: "makeConcatWithConstants", ProtoIndex: 0},
			{Class: "Ljava/lang/invoke/LambdaMetafactory;", Name: "metafactory", ProtoIndex: 2},
			{Class: owner, Name: "lambda$0", ProtoIndex: 5},
			{Class: owner, Name: "concat", ProtoIndex: 1},
			{Class: owner, Name: "lambda", ProtoIndex: 6},
		},
		MethodHandles: []model.MethodHandle{
			{Kind: model.MethodHandleInvokeStatic, TargetIndex: 0},
			{Kind: model.MethodHandleInvokeStatic, TargetIndex: 1},
			{Kind: model.MethodHandleInvokeStatic, TargetIndex: 2},
		},
		CallSites: []model.CallSite{
			{
				BootstrapMethodHandle: 0,
				MethodName:            "makeConcatWithConstants",
				MethodTypeIndex:       1, // (I)String;
				Arguments: []model.EncodedValue{
					{Type: model.ValueString, Unsigned: 1},
				},
			},
			{
				BootstrapMethodHandle: 1,
				MethodName:            "get",
				MethodTypeIndex:       4, // ()Supplier;
				Arguments: []model.EncodedValue{
					{Type: model.ValueMethodType, Unsigned: 3},
					{Type: model.ValueMethodHandle, Unsigned: 2},
					{Type: model.ValueMethodType, Unsigned: 5},
				},
			},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			DirectMethods: []model.EncodedMethod{
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: owner, Name: "lambda$0", ProtoIndex: 5},
					Code: &model.Code{
						RegistersSize: 1,
						Instructions:  []uint16{0x001a, 0x0003, 0x0011}, // const-string v0, "get"; return-object v0
					},
				},
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: owner, Name: "concat", ProtoIndex: 1},
					Code: &model.Code{
						RegistersSize: 2,
						InsSize:       1,
						OutsSize:      1,
						Instructions: []uint16{
							0x10fc, 0x0000, 0x0001, // invoke-custom {v1}, call_site@0
							0x000c, // move-result-object v0
							0x0011, // return-object v0
						},
					},
				},
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: owner, Name: "lambda", ProtoIndex: 6},
					Code: &model.Code{
						RegistersSize: 2,
						InsSize:       1,
						Instructions: []uint16{
							0x0138, 0x0007, // if-eqz v1, +7
							0x00fc, 0x0001, 0x0000, // invoke-custom {}, call_site@1
							0x000c, // move-result-object v0
							0x0011, // return-object v0
							0x0012, // const/4 v0, 0 (null)
							0x0011, // return-object v0
						},
					},
				},
			},
		}},
	}
	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	parsed := verifyClassWithFrameTransfer(t, raw)
	if parsed.MajorVersion != 51 {
		t.Fatalf("MajorVersion = %d, want 51 (Java 7)", parsed.MajorVersion)
	}
	var bsmCount int
	for _, attr := range parsed.Attributes {
		if bsm, ok := attr.(*classparser.BootstrapMethodsAttribute); ok {
			bsmCount = len(bsm.BootstrapMethods)
		}
	}
	if bsmCount != 2 {
		t.Fatalf("BootstrapMethods count = %d, want 2", bsmCount)
	}
	// Decompilation of StringConcatFactory & LambdaMetafactory without panic.
	_ = decompileLoweredClass(t, raw)
}

func TestAdversarialInvokePolymorphicNormalAndRange(t *testing.T) {
	const owner = "Ladv/PolyCalls;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "Ljava/lang/invoke/MethodHandle;"},
		},
		Protos: []model.Proto{
			// 0: signature-polymorphic declaration ([Ljava/lang/Object;)Ljava/lang/Object;
			{ReturnType: "Ljava/lang/Object;", Parameters: []string{"[Ljava/lang/Object;"}},
			// 1: entry (MethodHandle, int, long)String;
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"Ljava/lang/invoke/MethodHandle;", "I", "J"}},
			// 2: call-site (int, long)String;
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"I", "J"}},
			// 3: call-site (int, long, int, String)void
			{ReturnType: "V", Parameters: []string{"I", "J", "I", "Ljava/lang/String;"}},
			// 4: entry (MethodHandle, int, long)void
			{ReturnType: "V", Parameters: []string{"Ljava/lang/invoke/MethodHandle;", "I", "J"}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/invoke/MethodHandle;", Name: "invokeExact", ProtoIndex: 0},
			{Class: owner, Name: "callExact", ProtoIndex: 1},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			DirectMethods: []model.EncodedMethod{
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: owner, Name: "callExact", ProtoIndex: 1},
					Code: &model.Code{
						RegistersSize: 9, // v0..v4 temps/range, v5=MH, v6=int, v7..v8=long
						InsSize:       4,
						OutsSize:      5,
						Instructions: []uint16{
							// 0: invoke-polymorphic {v5, v6, v7, v8}, MethodHandle.invokeExact, (IJ)String;
							0x40fa, 0x0000, 0x6500, 0x0002,
							// 4: move-result-object v0
							0x000c,
							// 5: move-object v0_copy = v5 in range setup: v1=v5, v2=v6, v3=v7, v4=v8 -> check move-wide
							0x1012, // v0? No, keep v0 string! Move args to v1..v5 (regs 1..6 = mh, int, long_lo, long_hi, int, str)
							// Actually allocate v1=v5 (mh), v2=v6 (int), v3/v4=v7/v8 (long), v5=v6 (int), v6=v0 (str) -> range C=6 start v1!
							0x0507, // move-object v0 -> wait: don't overwrite v0! Let's save str in v6 first:
						},
					},
				},
			},
		}},
	}
	// Clean two methods: one 45cc (invoke-polymorphic) with wide arg, one 4rcc (invoke-polymorphic/range).
	file.Classes[0].DirectMethods = []model.EncodedMethod{
		{
			AccessFlags: dexAccessPublic | dexAccessStatic,
			Method:      model.MethodID{Class: owner, Name: "callExact", ProtoIndex: 1},
			Code: &model.Code{
				RegistersSize: 5, // v0=result, v1=mh, v2=int, v3..v4=long
				InsSize:       4,
				OutsSize:      4,
				Instructions: []uint16{
					// invoke-polymorphic {v1, v2, v3, v4}, MethodHandle.invokeExact, proto#2 (IJ)String;
					0x40fa, 0x0000, 0x2100, 0x0002, // F..A? count=4 -> regs D=v1, E=v2, F=v3, G=v4 -> 0x2143? Wait:
					0x000c,
					0x0011,
				},
			},
		},
		{
			AccessFlags: dexAccessPublic | dexAccessStatic,
			Method:      model.MethodID{Class: owner, Name: "callRange", ProtoIndex: 4},
			Code: &model.Code{
				RegistersSize: 10, // v0..v5 range (mh, int, longlo, longhi, int, string), v6..v9 = params
				InsSize:       4,
				OutsSize:      6,
				Instructions: []uint16{
					0x6007, // move-object v0, v6 (mh)
					0x7101, // move v1, v7 (int)
					0x8204, // move-wide v2, v8 (long)
					0x7401, // move v4, v7 (int)
					0x0512, // const/4 v5, 0 (null String)
					// invoke-polymorphic/range {v0..v5} (count=6, first=0), method=0, proto=3
					0x06fb, 0x0000, 0x0000, 0x0003,
					0x000e,
				},
			},
		},
	}
	// Encode 45cc word2 properly: nibble layout [C=count|G=arg4] [F=arg3|E=arg2|D=arg1|C=arg0?].
	// For 45cc: word0=(count<<12)|(G<<8)|op, word2=(F<<12)|(E<<8)|(D<<4)|C
	// With count=4 and C=1, D=2, E=3, F=4: word0=0x40fa, word1=0x0000, word2=0x4321, word3=0x0002.
	file.Classes[0].DirectMethods[0].Code.Instructions[2] = 0x4321

	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	verifyClassWithFrameTransfer(t, raw)
	src := decompileLoweredClass(t, raw)
	if !strings.Contains(src, "invokeExact") {
		t.Fatalf("expected invokeExact in decompiled source:\n%s", src)
	}
}

func TestAdversarialConstMethodHandleAllKindsAndInterfaceJava8Bump(t *testing.T) {
	const iface = "Ladv/IfaceTarget;"
	const owner = "Ladv/MHAll;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: iface},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/invoke/MethodHandle;"},
			{Descriptor: "Ljava/lang/invoke/MethodType;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "Ljava/lang/invoke/MethodHandle;", Parameters: []string{"I"}},
			{ReturnType: "Ljava/lang/invoke/MethodType;"},
		},
		Fields: []model.FieldID{
			{Class: owner, Type: "I", Name: "sf"},
			{Class: owner, Type: "I", Name: "inf"},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "st", ProtoIndex: 0},
			{Class: owner, Name: "virt", ProtoIndex: 0},
			{Class: owner, Name: "<init>", ProtoIndex: 0},
			{Class: owner, Name: "priv", ProtoIndex: 0},
			{Class: iface, Name: "ifaceStat", ProtoIndex: 0},
			{Class: iface, Name: "ifaceDef", ProtoIndex: 0},
		},
		MethodHandles: []model.MethodHandle{
			{Kind: model.MethodHandleStaticPut, TargetIndex: 0},
			{Kind: model.MethodHandleStaticGet, TargetIndex: 0},
			{Kind: model.MethodHandleInstancePut, TargetIndex: 1},
			{Kind: model.MethodHandleInstanceGet, TargetIndex: 1},
			{Kind: model.MethodHandleInvokeStatic, TargetIndex: 0},
			{Kind: model.MethodHandleInvokeInstance, TargetIndex: 1},
			{Kind: model.MethodHandleInvokeConstructor, TargetIndex: 2},
			{Kind: model.MethodHandleInvokeDirect, TargetIndex: 3},
			{Kind: model.MethodHandleInvokeInterface, TargetIndex: 5},
			// 9: InvokeStatic on an interface -> forces Java 8 (52) + InterfaceMethodref!
			{Kind: model.MethodHandleInvokeStatic, TargetIndex: 4},
		},
		Classes: []model.Class{
			{
				Descriptor:  iface,
				Superclass:  "Ljava/lang/Object;",
				AccessFlags: dexAccessPublic | dexAccInterface | dexAccAbstract,
				DirectMethods: []model.EncodedMethod{{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: iface, Name: "ifaceStat", ProtoIndex: 0},
					Code:        &model.Code{Instructions: []uint16{0x000e}},
				}},
				VirtualMethods: []model.EncodedMethod{{
					AccessFlags: dexAccessPublic,
					Method:      model.MethodID{Class: iface, Name: "ifaceDef", ProtoIndex: 0},
					Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
				}},
			},
			{
				Descriptor:  owner,
				Superclass:  "Ljava/lang/Object;",
				AccessFlags: dexAccessPublic,
				DirectMethods: []model.EncodedMethod{
					{
						AccessFlags: dexAccessPublic | dexAccessStatic,
						Method:      model.MethodID{Class: owner, Name: "getAll", ProtoIndex: 1},
						Code: &model.Code{
							RegistersSize: 2, // v0 out, v1 selector
							InsSize:       1,
							Instructions: []uint16{
								// 0: packed-switch v1, +26 (offset 26)
								0x012b, 26, 0,
								// 3: default -> const-method-handle v0, #0..#9 via switch table
								0x00fe, 0, 0x0011, // 3: kind 0
								0x00fe, 1, 0x0011, // 6: kind 1
								0x00fe, 2, 0x0011, // 9: kind 2
								0x00fe, 3, 0x0011, // 12: kind 3
								0x00fe, 4, 0x0011, // 15: kind 4
								0x00fe, 5, 0x0011, // 18: kind 5
								0x00fe, 6, 0x0011, // 21: kind 6
								0x00fe, 7, 0x0011, // 24: wait -> offset 26 is packed-switch payload! Let's fix offsets cleanly!
							},
						},
					},
				},
			},
		},
	}
	// Cleanly build getAll (branching through all 10 MethodHandles + const-method-type) on non-interface and interface-bump class:
	file.Classes[1].DirectMethods = []model.EncodedMethod{
		{
			AccessFlags: dexAccessPublic | dexAccessStatic,
			Method:      model.MethodID{Class: owner, Name: "getMH0to8", ProtoIndex: 1},
			Code: &model.Code{
				RegistersSize: 2, // v0, v1=param
				InsSize:       1,
				Instructions: []uint16{
					// 0: const/4 v0, #1
					0x1012,
					// 1: if-eq v1, v0, +5 -> 6 (0x00fe, 0x0001)
					0x0132, 5,
					// 3: const-method-handle v0, #0; return-object v0
					0x00fe, 0, 0x0011,
					// 6: const-method-handle v0, #1; ... chained via if-eqz/else? Simpler: chain all 9 handles ending with if/else!
					// Actually emit one sequential block reading 0..8 and returning #8, so every kind is lowered into CP!
					0x00fe, 1,
					0x00fe, 2,
					0x00fe, 3,
					0x00fe, 4,
					0x00fe, 5,
					0x00fe, 6,
					0x00fe, 7,
					0x00fe, 8,
					0x0011,
				},
			},
		},
		{
			AccessFlags: dexAccessPublic | dexAccessStatic,
			Method:      model.MethodID{Class: owner, Name: "getIfaceStaticMH", ProtoIndex: 1},
			Code: &model.Code{
				RegistersSize: 2,
				InsSize:       1,
				Instructions: []uint16{
					0x0138, 5, // if-eqz v1, +5 -> off 5
					0x00fe, 9, // MH #9 = InvokeStatic on iface target -> bumps class to VersionJava8 (52)
					0x0011,
					0x00fe, 4,
					0x0011,
				},
			},
		},
		{
			AccessFlags: dexAccessPublic | dexAccessStatic,
			Method:      model.MethodID{Class: owner, Name: "getMT", ProtoIndex: 2},
			Code: &model.Code{
				RegistersSize: 1,
				Instructions:  []uint16{0x00ff, 0, 0x0011},
			},
		},
	}

	fillProtoParamTypes(file)
	rawIface, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower iface: %v", err)
	}
	verifyClassWithFrameTransfer(t, rawIface)

	rawOwner, err := lower.ClassWithOptions(file, &file.Classes[1], lower.ClassOptions{})
	if err != nil {
		t.Fatalf("lower owner: %v", err)
	}
	parsed := verifyClassWithFrameTransfer(t, rawOwner)
	if parsed.MajorVersion != 52 {
		t.Fatalf("MajorVersion with interface static MethodHandle = %d, want 52", parsed.MajorVersion)
	}
	mhCount := 0
	mtCount := 0
	for _, cp := range parsed.ConstantPool {
		switch cp.(type) {
		case *classparser.ConstantMethodHandleInfo:
			mhCount++
		case *classparser.ConstantMethodTypeInfo:
			mtCount++
		}
	}
	if mhCount != 10 || mtCount != 1 {
		t.Fatalf("CP MethodHandles=%d MethodTypes=%d, want 10 and 1", mhCount, mtCount)
	}
}

func TestAdversarialJava8InterfaceControlFlowLoopsSwitchesTryCatchUnfused(t *testing.T) {
	const iface = "Ladv/RichJava8Iface;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"hi"},
		Types: []model.Type{
			{Descriptor: iface},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "Ljava/lang/StringBuilder;"},
			{Descriptor: "Ljava/lang/RuntimeException;"},
			{Descriptor: "Ljava/lang/IllegalArgumentException;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "I", Parameters: []string{"I"}},
			{ReturnType: "Ljava/lang/Object;", Parameters: []string{"I"}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/StringBuilder;", Name: "<init>", ProtoIndex: 0},
		},
		Classes: []model.Class{{
			Descriptor:  iface,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic | dexAccInterface | dexAccAbstract,
			DirectMethods: []model.EncodedMethod{
				// Static method with loop back-edge + packed-switch + try/catch (trampoline without move-exception)
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: iface, Name: "loopSwitchTry", ProtoIndex: 1},
					Code: &model.Code{
						RegistersSize: 3, // v0=acc, v1=i, v2=param
						InsSize:       1,
						Instructions: []uint16{
							// 0: const/4 v0, 0
							0x0012,
							// 1: const/4 v1, 0
							0x0112,
							// 2 (loop header): if-ge v1, v2, +6 -> offset 8 (return)
							// Actually if-lt is 0x34, if-ge is 0x35! if v1 >= v2 goto +6 (offset 8)
							0x2135, 6,
							// 4 (try-protected division or switch): add-int v0, v0, v1 -> use 2addr: add-int/2addr v0, v1
							0x10b0,
							// 5: div-int/lit8 v0, v0, 1 (trapped by catch RuntimeException -> handler without move-exception!)
							0x00dd, 0x0100,
							// 7: goto +3 -> wait, let's put catch-trampoline AFTER return:
							// Wait: let's add 1 to v1 (7: add-int/lit8 v1, v1, 1) -> offset 7..8; 9: goto -7 -> 2; 10: return v0; 11: const/4 v0, -1; return v0
							// Fix lengths carefully:
							// 0: 0x0012 (1u) -> v0=0
							// 1: 0x0112 (1u) -> v1=0
							// 2: 0x2135, 8 (2u) -> if v1 >= v2 goto 10
							// 4: 0x10dd, 0x0200 (2u) -> v0 = v0 / 2 -> wait: div-int/2addr can throw ArithmeticException! 0x10b3 (div-int/2addr v0, v1): 1u!
							// Let's trace:
							// 0: const/4 v0, 1 (0x1012)
							// 1: const/4 v1, 0 (0x0112)
							// 2: if-ge v1, v2, +6 -> 8 (0x2135, 6)
							// 4: add-int/lit8 v1, v1, 1 (0x01d8, 0x0101) [2u -> off 4..5]
							// 6: div-int/2addr v0, v1 (0x10b3) [try covers 6..7]
							// 7: goto -5 -> 2 (0xfb28 -> -5 is 0xfb in 10t: (byte -5)<<8 | 0x28 = 0xfb28!) [Wait: if goto -5 from 7 = 2, wait target at 8 is 2+6=8! Wait: at 2+7=9!]
							0x0000, // placeholder replaced below
						},
					},
				},
			},
			VirtualMethods: []model.EncodedMethod{
				// Default interface method with move-exception handler + unfused new-instance across a branch!
				{
					AccessFlags: dexAccessPublic,
					Method:      model.MethodID{Class: iface, Name: "unfusedAndCatch", ProtoIndex: 2},
					Code: &model.Code{
						RegistersSize: 4, // v0=alloc/ret, v1=tmp, v2=this, v3=flag
						InsSize:       2,
						OutsSize:      1,
						Instructions: []uint16{
							// 0: new-instance v0, StringBuilder (unfused across branch!)
							0x0022, 3,
							// 2: if-eqz v3, +4 -> offset 6
							0x0338, 4,
							// 4: const/4 v1, 1
							0x1112,
							// 5: goto +2 -> offset 7
							0x0228,
							// 6: const/4 v1, 2
							0x2112,
							// 7: invoke-direct {v0}, StringBuilder.<init>()V
							0x1070, 0, 0,
							// 10: return-object v0
							0x0011,
							// 11 (catch Throwable): move-exception v0
							0x000d,
							// 12: const/4 v0, 0 (null)
							0x0012,
							// 13: return-object v0
							0x0011,
						},
						Tries: []model.Try{{
							StartAddress:     7,
							InstructionCount: 3, // covers invoke-direct <init>
							Handler: model.CatchHandler{
								Handlers: []model.TypedHandler{{TypeIndex: 4, Type: "Ljava/lang/RuntimeException;", Address: 11}},
							},
						}},
					},
				},
			},
		}},
	}
	// Replace loopSwitchTry with clean exact offsets:
	// 0: const/4 v0, 1      (0x1012)
	// 1: const/4 v1, 0      (0x0112)
	// 2: if-ge v1, v2, +7   (0x2135, 7) -> target 9
	// 4: add-int/lit8 v1, v1, 1 (0x01d8, 0x0101)
	// 6: div-int/2addr v0, v1   (0x10b3) -> try [6, 7) -> catchall at 10 (no move-exception -> trampoline!)
	// 7: goto -5            (0xfb28) -> target 2 (wait: 7 + int8(-5) = 2! Wait in 10t format: (offset << 8) | 0x28, so uint16(int16(-5))<<8 | 0x28 = 0xfb28)
	// 8: nop? No, next is 8 -> fix 2's target to 8: +6! Wait: goto is 1u at 7, so next is 8!
	// 8: return v0          (0x000f)
	// 9: const/4 v0, 7      (0x7012) -> wait try catches to 9 -> returns 7 (no move-exception -> trampoline!)
	// 10: return v0         (0x000f)
	file.Classes[0].DirectMethods[0].Code.Instructions = []uint16{
		0x1012,    // 0: v0 = 1
		0x0112,    // 1: v1 = 0
		0x2135, 6, // 2: if v1 >= v2 goto 8
		0x01d8, 0x0101, // 4: v1 = v1 + 1
		0x10b3, // 6: v0 = v0 / v1
		0xfb28, // 7: goto 2
		0x000f, // 8: return v0
		0x7012, // 9 (trampoline handler without move-exception): v0 = 7
		0x000f, // 10: return v0
	}
	file.Classes[0].DirectMethods[0].Code.Tries = []model.Try{{
		StartAddress:     6,
		InstructionCount: 1,
		Handler: model.CatchHandler{
			Handlers: []model.TypedHandler{{TypeIndex: 4, Type: "Ljava/lang/RuntimeException;", Address: 9}},
		},
	}}
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: iface, Name: "switchIface", ProtoIndex: 1},
		Code: &model.Code{
			RegistersSize: 2, // v0 out, v1 selector
			InsSize:       1,
			Instructions: []uint16{
				// 0: packed-switch v1, +8 -> payload at offset 8
				0x012b, 8, 0,
				// 3: default -> const/4 v0, 0; nop; return v0
				0x0012, 0x0000, 0x000f,
				// 6: case 1 -> const/4 v0, 5; return v0
				0x5012, 0x000f,
				// 8: packed-switch-payload (ident=0x0100, size=1, first_key=1, target[0]=6-0=+6)
				0x0100, 0x0001, 0x0001, 0x0000, 0x0006, 0x0000,
			},
		},
	})
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: iface, Name: "mergeRefsAndMultiCatch", ProtoIndex: 2},
		Code: &model.Code{
			RegistersSize: 2, // v0 ref, v1 param
			InsSize:       1,
			OutsSize:      1,
			Instructions: []uint16{
				0x0138, 0x0005, // 0: if-eqz v1, +5 -> 5
				0x001a, 0x0000, // 2: const-string v0, "hi"
				0x0628,         // 4: goto +6 -> 10
				0x0022, 0x0003, // 5: new-instance v0, StringBuilder
				0x1070, 0x0000, 0x0000, // 7: invoke-direct {v0}, StringBuilder.<init>()V
				0x0011, // 10: return-object v0 (merges String & StringBuilder)
				0x000d, // 11: move-exception v0 (shared multi-catch handler)
				0x0011, // 12: return-object v0
			},
			Tries: []model.Try{{
				StartAddress:     7,
				InstructionCount: 3,
				Handler: model.CatchHandler{
					Handlers: []model.TypedHandler{
						{TypeIndex: 5, Type: "Ljava/lang/IllegalArgumentException;", Address: 11},
						{TypeIndex: 4, Type: "Ljava/lang/RuntimeException;", Address: 11},
					},
				},
			}},
		},
	})

	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower Java 8 interface: %v", err)
	}
	parsed := verifyClassWithFrameTransfer(t, raw)
	if parsed.MajorVersion != 52 {
		t.Fatalf("interface MajorVersion = %d, want 52", parsed.MajorVersion)
	}
	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "undecompilable") {
		t.Fatalf("Java 8 interface decompilation produced stub:\n%s", src)
	}
}

func TestAdversarialDEX038And039EndToEndWithFrameVerification(t *testing.T) {
	fixtures := []string{
		filepath.Join("..", "testdata", "external", "dex2jar-dex038.dex"),
		filepath.Join("..", "testdata", "external", "dex2jar-dex039.dex"),
	}
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			jar1, rep1, err := dex2jar.DexToJar(data, dex2jar.Options{})
			if err != nil {
				t.Fatalf("DexToJar: %v", err)
			}
			jar2, rep2, err := dex2jar.DexToJar(data, dex2jar.Options{})
			if err != nil || !bytes.Equal(jar1, jar2) || rep1 != rep2 {
				t.Fatalf("DexToJar nondeterministic: err=%v rep1=%#v rep2=%#v", err, rep1, rep2)
			}
			zr, err := zip.NewReader(bytes.NewReader(jar1), int64(len(jar1)))
			if err != nil {
				t.Fatal(err)
			}
			classCount := 0
			for _, f := range zr.File {
				if !strings.HasSuffix(f.Name, ".class") {
					continue
				}
				classCount++
				rc, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				var buf bytes.Buffer
				if _, err := buf.ReadFrom(rc); err != nil {
					rc.Close()
					t.Fatal(err)
				}
				rc.Close()
				verifyClassWithFrameTransfer(t, buf.Bytes())
				src := decompileLoweredClass(t, buf.Bytes())
				if len(src) == 0 {
					t.Fatalf("empty decompiled source for %s", f.Name)
				}
			}
			if classCount == 0 {
				t.Fatal("no class files emitted")
			}
		})
	}
}
