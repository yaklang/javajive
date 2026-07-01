package javaclassparser

import (
	"fmt"
	"os"

	"github.com/samber/lo"
	"github.com/yaklang/javajive/classparser/decompiler"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/log"
)

func getNameAndType(pool []ConstantInfo, index uint16) (string, string) {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	nameAndTypeInfo := pool[index-1].(*ConstantNameAndTypeInfo)
	name := indexFromPool(int(nameAndTypeInfo.NameIndex)).(*ConstantUtf8Info).Value
	desc := indexFromPool(int(nameAndTypeInfo.DescriptorIndex)).(*ConstantUtf8Info).Value
	return name, desc
}
func showOpcodes(codes []*core.OpCode) {
	for i, opCode := range codes {
		if opCode.Instr.Name == "if_icmpge" || opCode.Instr.Name == "goto" {
			fmt.Printf("%d %s jmpto:%d\n", i, opCode.Instr.Name, opCode.Jmp)
		} else {
			fmt.Printf("%d %s %v\n", i, opCode.Instr.Name, opCode.Data)
		}
	}
}

func GetValueFromCP(pool []ConstantInfo, index int) values.JavaValue {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	constant := pool[index-1]
	getClassName := func(index uint16) string {
		classInfo := indexFromPool(int(index)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)
		return nameInfo.Value
	}
	convertMemberInfo := func(classMemberInfo *ConstantMemberrefInfo) values.JavaValue {
		className := getClassName(classMemberInfo.ClassIndex)
		className = types.SlashToDot(className)
		name, desc := getNameAndType(pool, classMemberInfo.NameAndTypeIndex)
		typ, err := types.ParseMethodDescriptor(desc)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", desc)
		}
		val := values.NewJavaClassMember(className, name, desc, typ)
		return val
	}
	switch ret := constant.(type) {
	case *ConstantMethodHandleInfo:
		return GetValueFromCP(pool, int(ret.ReferenceIndex))
	case *ConstantMemberrefInfo:
		return convertMemberInfo(ret)
	case *ConstantInterfaceMethodrefInfo:
		memberInfo := ret.ConstantMemberrefInfo
		return convertMemberInfo(&memberInfo)
	case *ConstantFieldrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)
		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = types.SlashToDot(typeName)
		typ, err := types.ParseDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value, typ)
		return classIns
	case *ConstantMethodrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = types.SlashToDot(typeName)
		typ, err := types.ParseMethodDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value, typ)
		return classIns
	case *ConstantClassInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = types.SlashToDot(typeName)
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	case *ConstantModuleInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = types.SlashToDot(typeName)
		log.Warn("TODO: the java module should be a new java type")
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	case *ConstantPackageInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = types.SlashToDot(typeName)
		log.Warn("TODO: the java module should be a new java type")
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	case *ConstantMethodTypeInfo:
		descInfo := indexFromPool(int(ret.DescriptorIndex)).(*ConstantUtf8Info)
		typ, err := types.ParseMethodDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		_ = typ
		return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			// Return the descriptor itself so lambda type inference can read it.
			// Other consumers (non-lambda bootstraps) ignore the string value.
			return descInfo.Value
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.invoke.MethodType")
		})
	default:
		panic("failed")
	}
}
func GetLiteralFromCP(pool []ConstantInfo, index int) values.JavaValue {
	constant := pool[index-1]
	switch ret := constant.(type) {
	case *ConstantStringInfo:
		return values.NewJavaLiteral(pool[ret.StringIndex-1].(*ConstantUtf8Info).Value, types.NewJavaPrimer(types.JavaString))
	case *ConstantLongInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaLong))
	case *ConstantIntegerInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaInteger))
	case *ConstantDoubleInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaDouble))
	case *ConstantFloatInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaFloat))
	case *ConstantClassInfo:
		return GetValueFromCP(pool, index)
	case *ConstantModuleInfo:
		return GetValueFromCP(pool, index)
	case *ConstantPackageInfo:
		return GetValueFromCP(pool, index)
	default:
		return GetValueFromCP(pool, index)
	}
}

type VarMap struct {
	id  int
	val values.JavaValue
}

func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute, id *utils.VariableId) ([]values.JavaValue, []statements.Statement, error) {
	pool := dumper.ConstantPool
	parser := core.NewDecompiler(codeAttr.Code, func(id int) values.JavaValue {
		return GetValueFromCP(dumper.ConstantPool, id)
	})
	parser.DumpClassLambdaMethod = func(name, desc string, id *utils.VariableId, capturedCount int) (string, error) {
		dumper.lambdaMethods[name] = append(dumper.lambdaMethods[name], desc)
		dumper.lambdaCaptureCount[name+desc] = capturedCount
		// A lambda body is dumped LAZILY, mid-parse of the ENCLOSING method: the bootstrap closure
		// runs while the enclosing method's invokedynamic value is built during its stack
		// simulation. The recursive DumpMethodWithInitialId overwrites the SHARED dumper.FuncCtx
		// (FunctionName / FunctionType / IsStatic) and dumper.MethodType / CurrentMethod with the
		// lambda's own (e.g. a `void lambda$...`). Because parser.FunctionContext IS this same
		// dumper.FuncCtx pointer, the enclosing method's REMAINING opcodes — notably a trailing
		// `return false/true` that sits AFTER the invokedynamic (javac: `iconst_0/1; ireturn`) —
		// would then reset their return type against the lambda's `void` context instead of the
		// enclosing method's `boolean`, so resetReturnValueTypeSafe no-ops and the literal renders
		// as `return 1`/`return 0` instead of `return true`/`return false` (fastjson2
		// BeanUtils.isWriteEnumAsJavaBean, JSONPathTypedMultiIndexes, etc.). Save the enclosing
		// method's per-method identity and restore it after the re-entrant dump so the enclosing
		// parse resumes with its own context. (TypeParams is also restored by DumpMethod's own
		// defer; we save/restore it defensively in case that path is skipped.)
		// Kill-switch JDEC_LAMBDA_CTX_RESTORE_OFF=1 disables the restore to reproduce the defect.
		restore := os.Getenv("JDEC_LAMBDA_CTX_RESTORE_OFF") != "1"
		savedFunctionName := dumper.FuncCtx.FunctionName
		savedFunctionType := dumper.FuncCtx.FunctionType
		savedIsStatic := dumper.FuncCtx.IsStatic
		savedTypeParams := dumper.FuncCtx.TypeParams
		savedMethodType := dumper.MethodType
		savedCurrentMethod := dumper.CurrentMethod
		dumped, err := dumper.DumpMethodWithInitialId(name, desc, id)
		if restore {
			dumper.FuncCtx.FunctionName = savedFunctionName
			dumper.FuncCtx.FunctionType = savedFunctionType
			dumper.FuncCtx.IsStatic = savedIsStatic
			dumper.FuncCtx.TypeParams = savedTypeParams
			dumper.MethodType = savedMethodType
			dumper.CurrentMethod = savedCurrentMethod
		}
		if err != nil {
			return "", err
		}
		return dumped.code, nil
	}
	parser.BaseVarId = id
	parser.Aggressive = dumper.aggressive
	parser.FunctionContext = dumper.FuncCtx
	parser.FunctionType = dumper.MethodType
	//parser.FunctionContext.FunctionName
	parser.ConstantPoolLiteralGetter = func(id int) values.JavaValue {
		return GetLiteralFromCP(dumper.ConstantPool, id)
	}
	for _, entry := range codeAttr.ExceptionTable {
		parser.ExceptionTable = append(parser.ExceptionTable, &core.ExceptionTableEntry{
			StartPc:   entry.StartPc,
			EndPc:     entry.EndPc,
			HandlerPc: entry.HandlerPc,
			CatchType: entry.CatchType,
		})
	}
	attrInterfaces := lo.Filter(dumper.obj.Attributes, func(item AttributeInfo, index int) bool {
		_, ok := item.(*BootstrapMethodsAttribute)
		return ok
	})
	attrs := lo.Map(attrInterfaces, func(item AttributeInfo, index int) *BootstrapMethodsAttribute {
		return item.(*BootstrapMethodsAttribute)
	})
	var bootstrapMethod []*BootstrapMethod
	if len(attrs) > 0 {
		bootstrapMethod = attrs[0].BootstrapMethods
	}
	for _, method := range bootstrapMethod {
		val := GetValueFromCP(pool, int(method.BootstrapMethodRef))
		arguments := make([]values.JavaValue, len(method.BootstrapArguments))
		for i, arg := range method.BootstrapArguments {
			arguments[i] = GetLiteralFromCP(pool, int(arg))
		}
		parser.BootstrapMethods = append(parser.BootstrapMethods, &core.BootstrapMethod{
			Ref:       val,
			Arguments: arguments,
		})
	}

	parser.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		constant := pool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			name, desc := getNameAndType(dumper.ConstantPool, ret.NameAndTypeIndex)
			return ret.BootstrapMethodAttrIndex, name, desc
		default:
			panic("error")
		}
	}
	st, err := decompiler.ParseBytesCode(parser)
	return parser.Params, st, err
}
