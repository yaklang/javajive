// Package lower translates the strict DEX IR subset into Java 5 class files.
package lower

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

const (
	dexPublic               = uint32(0x0001)
	dexPrivate              = uint32(0x0002)
	dexProtected            = uint32(0x0004)
	dexStatic               = uint32(0x0008)
	dexFinal                = uint32(0x0010)
	dexSynchronized         = uint32(0x0020)
	dexVolatile             = uint32(0x0040)
	dexBridge               = uint32(0x0040)
	dexTransient            = uint32(0x0080)
	dexVarargs              = uint32(0x0080)
	dexNative               = uint32(0x0100)
	dexInterface            = uint32(0x0200)
	dexAbstract             = uint32(0x0400)
	dexStrict               = uint32(0x0800)
	dexSynthetic            = uint32(0x1000)
	dexAnnotation           = uint32(0x2000)
	dexEnum                 = uint32(0x4000)
	dexConstructor          = uint32(0x10000)
	dexDeclaredSynchronized = uint32(0x20000)
)

// ClassOptions controls optional work around a class translation.
type ClassOptions struct {
	// FileValidated reports that the caller already validated File with
	// ir.ValidateFile and will not mutate it. Method lowering then skips the
	// per-method whole-file metadata validation, which is quadratic on large
	// applications.
	FileValidated bool
	// Hierarchy is the application-wide class index across every DEX file.
	// Multidex applications split a superclass or interface from its subclass,
	// so assignability must be provable across files. When nil, only the file
	// being lowered is consulted.
	Hierarchy map[string]*model.Class
	// ForceClassShell skips normal lowering and emits a declaration-preserving
	// shell. Application recovery uses this when a superclass or interface was
	// reduced to a minimal shell.
	ForceClassShell bool
	// RecoveryReason is embedded in diagnostics when ForceClassShell is set.
	RecoveryReason string
}

// FailurePolicy controls whether a method-local unsupported capability aborts
// the class or becomes a verifier-safe throwing stub. Malformed input and
// internal failures are never eligible for stubbing.
type FailurePolicy uint8

const (
	Strict FailurePolicy = iota
	StubUnsupportedMethods
	Recover
)

// StubbedMethod records one concrete method whose body was replaced because
// the DEX construct is outside the translator's supported capability set.
type StubbedMethod struct {
	Method     string
	Descriptor string
	Feature    string
	Message    string
}

// ClassResult is the classfile and any explicit method degradation evidence.
type ClassResult struct {
	Bytes          []byte
	StubbedMethods []StubbedMethod
	Recoveries     []Recovery
	ClassStubbed   bool
	ClassMinimal   bool
}

// Class lowers one parser-neutral DEX class. It never returns partial bytes.
func Class(file *model.File, class *model.Class) ([]byte, error) {
	return ClassWithOptions(file, class, ClassOptions{})
}

// ClassWithOptions is Class with explicit caller-provided guarantees.
func ClassWithOptions(file *model.File, class *model.Class, options ClassOptions) ([]byte, error) {
	result, err := ClassWithPolicy(file, class, options, Strict)
	return result.Bytes, err
}

// ClassWithPolicy lowers one DEX class with an explicit unsupported-capability
// policy. It never returns partial class bytes.
func ClassWithPolicy(file *model.File, class *model.Class, options ClassOptions, policy FailurePolicy) (ClassResult, error) {
	if file == nil {
		return ClassResult{}, report.New("", "", "", -1, "class", fmt.Errorf("nil DEX file"))
	}
	if class == nil {
		return ClassResult{}, report.New(file.Entry, "", "", -1, "class", fmt.Errorf("nil class"))
	}
	if policy != Strict && policy != StubUnsupportedMethods && policy != Recover {
		return ClassResult{}, classDiagnostic(file, class, "class", fmt.Errorf("unknown failure policy %d", policy))
	}
	if policy == Recover {
		if options.ForceClassShell {
			reason := options.RecoveryReason
			if reason == "" {
				reason = "application dependency requires a class shell"
			}
			return emitClassShell(file, class, options, errors.New(reason), false)
		}
		result, err := classWithPolicyOnce(file, class, options, StubUnsupportedMethods)
		if err == nil {
			return result, nil
		}
		shell, shellErr := emitClassShell(file, class, options, err, false)
		if shellErr == nil {
			return shell, nil
		}
		shell, reducedErr := emitClassShell(file, class, options, err, true)
		if reducedErr == nil {
			return shell, nil
		}
		return emitMinimalClassShell(file, class, fmt.Errorf("normal=%v; declaration shell=%v; reduced shell=%v", err, shellErr, reducedErr))
	}
	return classWithPolicyOnce(file, class, options, policy)
}

func classWithPolicyOnce(file *model.File, class *model.Class, options ClassOptions, policy FailurePolicy) (ClassResult, error) {
	className, err := objectInternalName(class.Descriptor)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class", err)
	}
	superName, err := superclassName(class)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class", err)
	}
	access, err := classAccess(class.AccessFlags)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class", err)
	}
	if class.AccessFlags&dexInterface != 0 && class.Superclass != "Ljava/lang/Object;" {
		return ClassResult{}, classDiagnostic(file, class, "class", fmt.Errorf("interface superclass must be java/lang/Object"))
	}
	if interfaceNeedsJava8(class) {
		// Static and default interface methods require a Java 8 class file. The
		// translator does not emit StackMapTable, so ordinary translated methods
		// may not have control flow or handlers. Throwing stubs are straight-line.
		if policy == Strict {
			if err := validateNoControlFlow(file, class); err != nil {
				return ClassResult{}, err
			}
		}
		builder := classfilebuilder.NewClass(className, superName, access)
		builder.SetVersion(classfilebuilder.VersionJava8)
		return lowerClassBody(file, class, builder, options, policy)
	}
	builder := classfilebuilder.NewClass(className, superName, access)
	return lowerClassBody(file, class, builder, options, policy)
}

// interfaceNeedsJava8 reports whether an interface carries a static or default
// method, which a Java 5 class file cannot express.
func interfaceNeedsJava8(class *model.Class) bool {
	if class.AccessFlags&dexInterface == 0 {
		return false
	}
	for _, method := range appendMethods(class.DirectMethods, class.VirtualMethods) {
		if method.Method.Name == "<clinit>" {
			continue
		}
		if method.Code != nil || method.AccessFlags&dexStatic != 0 || method.AccessFlags&dexAbstract == 0 {
			return true
		}
	}
	return false
}

// validateNoControlFlow rejects interface methods whose code would need a
// StackMapTable, which the translator does not emit.
func validateNoControlFlow(file *model.File, class *model.Class) error {
	for _, method := range appendMethods(class.DirectMethods, class.VirtualMethods) {
		if method.Code == nil {
			continue
		}
		if len(method.Code.Tries) > 0 {
			return classDiagnostic(file, class, "interface", fmt.Errorf("method %s has exception handlers, which need Java 8 stack maps", method.Method.Name))
		}
		instructions, err := ir.Decode(method.Code, methodContext(file, class, &method))
		if err != nil {
			return err
		}
		for _, instruction := range instructions {
			if isBranchOpcode(instruction.Opcode) || instruction.Opcode == 0x2b || instruction.Opcode == 0x2c {
				return classDiagnostic(file, class, "interface", fmt.Errorf("method %s has control flow, which needs Java 8 stack maps", method.Method.Name))
			}
		}
	}
	return nil
}

// lowerClassBody emits members and attributes into an already configured
// builder.
func lowerClassBody(file *model.File, class *model.Class, builder *classfilebuilder.Class, options ClassOptions, policy FailurePolicy) (ClassResult, error) {
	if class.SourceFile != "" {
		builder.SetSourceFile(class.SourceFile)
	}
	if class.Signature != "" {
		builder.SetClassSignature(class.Signature)
	}
	if class.SourceDebugExtension != "" {
		builder.SetSourceDebugExtension(class.SourceDebugExtension)
	}
	if len(class.InnerClasses) > 0 {
		specs := make([]classfilebuilder.InnerClassSpec, 0, len(class.InnerClasses))
		for _, inner := range class.InnerClasses {
			specs = append(specs, classfilebuilder.InnerClassSpec{
				Inner: inner.Inner, Outer: inner.Outer, Name: inner.Name, Flags: inner.Flags,
			})
		}
		builder.SetInnerClasses(specs)
	}
	if class.Enclosing != nil {
		builder.SetEnclosingMethod(classfilebuilder.EnclosingMethodSpec{
			Class: class.Enclosing.Class, Method: class.Enclosing.Method, Proto: class.Enclosing.Proto,
		})
	}
	if len(class.Annotations) > 0 {
		translated, err := translateAnnotations(file, class.Annotations)
		if err != nil {
			return ClassResult{}, classDiagnostic(file, class, "annotation", err)
		}
		if translated.signature != "" {
			builder.SetClassSignature(translated.signature)
		}
		for index, annotation := range translated.annotations {
			builder.AddClassAnnotation(annotation, translated.visible[index])
		}
	}
	for _, descriptor := range class.Interfaces {
		name, err := objectInternalName(descriptor)
		if err != nil {
			return ClassResult{}, classDiagnostic(file, class, "interface", err)
		}
		builder.AddInterface(name)
	}
	if err := addFields(file, class, builder); err != nil {
		return ClassResult{}, err
	}
	seenMethods := make(map[string]bool)
	var stubbed []StubbedMethod
	for _, method := range appendMethods(class.DirectMethods, class.VirtualMethods) {
		if int(method.Method.ProtoIndex) >= len(file.Protos) {
			return ClassResult{}, methodDiagnostic(methodContext(file, class, &method), nil, "method", fmt.Errorf("proto index %d is out of range", method.Method.ProtoIndex))
		}
		descriptor, err := methodDescriptor(file.Protos[method.Method.ProtoIndex])
		if err != nil {
			return ClassResult{}, methodDiagnostic(methodContext(file, class, &method), nil, "method", err)
		}
		key := method.Method.Name + "\x00" + descriptor
		if seenMethods[key] {
			return ClassResult{}, methodDiagnostic(methodContext(file, class, &method), nil, "method", fmt.Errorf("duplicate method %s%s", method.Method.Name, descriptor))
		}
		seenMethods[key] = true
		stub, err := addMethod(file, class, &method, builder, options, policy)
		if err != nil {
			return ClassResult{}, err
		}
		if stub != nil {
			stubbed = append(stubbed, *stub)
		}
	}
	raw, err := builder.Bytes()
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "classfile", err)
	}
	return ClassResult{Bytes: raw, StubbedMethods: stubbed}, nil
}

// LowerClass is an explicit-name alias for Class.
func LowerClass(file *model.File, class *model.Class) ([]byte, error) {
	return Class(file, class)
}

func appendMethods(first, second []model.EncodedMethod) []model.EncodedMethod {
	methods := make([]model.EncodedMethod, 0, len(first)+len(second))
	methods = append(methods, first...)
	methods = append(methods, second...)
	return methods
}

func addFields(file *model.File, class *model.Class, builder *classfilebuilder.Class) error {
	seen := make(map[string]bool)
	fields := make([]model.EncodedField, 0, len(class.StaticFields)+len(class.InstanceFields))
	fields = append(fields, class.StaticFields...)
	fields = append(fields, class.InstanceFields...)
	for index := range fields {
		field := fields[index]
		if field.Field.Class != class.Descriptor {
			return classDiagnostic(file, class, "field", fmt.Errorf("field %q owner %q does not match class", field.Field.Name, field.Field.Class))
		}
		if field.Field.Name == "" {
			return classDiagnostic(file, class, "field", fmt.Errorf("field name is empty"))
		}
		if _, err := fieldKind(field.Field.Type); err != nil {
			return classDiagnostic(file, class, "field", fmt.Errorf("field %q: %w", field.Field.Name, err))
		}
		key := field.Field.Name + "\x00" + field.Field.Type
		if seen[key] {
			return classDiagnostic(file, class, "field", fmt.Errorf("duplicate field %s %s", field.Field.Name, field.Field.Type))
		}
		seen[key] = true
		access, err := fieldAccess(field.AccessFlags)
		if err != nil {
			return classDiagnostic(file, class, "field", fmt.Errorf("field %q: %w", field.Field.Name, err))
		}
		isStatic := index < len(class.StaticFields)
		if isStatic != (field.AccessFlags&dexStatic != 0) {
			return classDiagnostic(file, class, "field", fmt.Errorf("field %q static membership and access flag disagree", field.Field.Name))
		}
		if class.AccessFlags&dexInterface != 0 && field.AccessFlags&(dexPublic|dexStatic|dexFinal) != dexPublic|dexStatic|dexFinal {
			return classDiagnostic(file, class, "field", fmt.Errorf("Java 5 interface field must be public static final"))
		}
		member := builder.AddField(access, field.Field.Name, field.Field.Type)
		if isStatic && index < len(class.StaticValues) {
			if err := applyStaticValue(file, class, &fields[index], member, builder, class.StaticValues[index]); err != nil {
				return err
			}
		}
		if annotations := fieldAnnotationsOf(class, field.FieldIndex); len(annotations) > 0 {
			translated, err := translateAnnotations(file, annotations)
			if err != nil {
				return classDiagnostic(file, class, "field annotation", fmt.Errorf("field %q: %w", field.Field.Name, err))
			}
			if translated.signature != "" {
				builder.SetFieldSignature(member, translated.signature)
			}
			for i, annotation := range translated.annotations {
				builder.AddFieldAnnotation(member, annotation, translated.visible[i])
			}
		}
	}
	return nil
}

func fieldAnnotationsOf(class *model.Class, index uint32) []model.Annotation {
	for _, entry := range class.FieldAnnotations {
		if entry.FieldIndex == index {
			return entry.Annotations
		}
	}
	return nil
}

func methodAnnotationsOf(class *model.Class, index uint32) []model.Annotation {
	for _, entry := range class.MethodAnnotations {
		if entry.MethodIndex == index {
			return entry.Annotations
		}
	}
	return nil
}

func parameterAnnotationsOf(class *model.Class, index uint32) [][]model.Annotation {
	for _, entry := range class.ParameterAnnotations {
		if entry.MethodIndex == index {
			return entry.Parameters
		}
	}
	return nil
}

// applyStaticValue turns a representable encoded static initializer into a
// ConstantValue attribute. Values that have no compatible classfile encoding
// are rejected before mutating the builder.
func applyStaticValue(file *model.File, class *model.Class, field *model.EncodedField, member *classfilebuilder.Field, builder *classfilebuilder.Class, value model.EncodedValue) error {
	kind, err := fieldKind(field.Field.Type)
	if err != nil {
		return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q: %w", field.Field.Name, err))
	}
	if kind == ir.Reference {
		switch value.Type {
		case model.ValueNull:
			return nil
		case model.ValueString:
			if field.Field.Type != "Ljava/lang/String;" {
				return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q descriptor %q cannot use a String ConstantValue", field.Field.Name, field.Field.Type))
			}
			index := value.Unsigned
			if index >= uint64(len(file.Strings)) {
				return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q string index %d is out of range", field.Field.Name, index))
			}
			builder.SetConstantString(member, file.Strings[index])
			return nil
		default:
			return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has unsupported reference initializer", field.Field.Name))
		}
	}
	switch kind {
	case ir.Long:
		if value.Type != model.ValueLong && value.Type != model.ValueInt {
			return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has incompatible long initializer kind %#x", field.Field.Name, value.Type))
		}
		builder.SetConstantLong(member, value.Signed)
		return nil
	case ir.Float:
		if value.Type != model.ValueFloat {
			return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has incompatible float initializer kind %#x", field.Field.Name, value.Type))
		}
		builder.SetConstantFloat(member, uint32(value.Unsigned))
		return nil
	case ir.Double:
		if value.Type != model.ValueDouble {
			return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has incompatible double initializer kind %#x", field.Field.Name, value.Type))
		}
		builder.SetConstantDouble(member, value.Unsigned)
		return nil
	}
	switch value.Type {
	case model.ValueBoolean:
		if !value.Boolean {
			return nil
		}
		builder.SetConstantInt(member, 1)
	case model.ValueByte, model.ValueShort, model.ValueChar, model.ValueInt:
		if value.Signed == 0 && value.Unsigned == 0 {
			return nil
		}
		builder.SetConstantInt(member, shrinkSigned(field.Field.Type, value))
	case model.ValueNull:
		return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has a null initializer for a primitive type", field.Field.Name))
	default:
		return classDiagnostic(file, class, "static-value", fmt.Errorf("field %q has unsupported initializer kind %#x", field.Field.Name, value.Type))
	}
	return nil
}

// shrinkSigned narrows the encoded constant to the field's storage width so the
// ConstantValue matches the descriptor instead of silently truncating later.
func shrinkSigned(descriptor string, value model.EncodedValue) int32 {
	switch descriptor {
	case "Z":
		if value.Signed != 0 || value.Unsigned != 0 {
			return 1
		}
		return 0
	case "B":
		return int32(int8(value.Signed))
	case "S":
		return int32(int16(value.Signed))
	case "C":
		return int32(uint16(value.Unsigned))
	default:
		return int32(value.Signed)
	}
}

func addMethod(file *model.File, class *model.Class, method *model.EncodedMethod, builder *classfilebuilder.Class, options ClassOptions, policy FailurePolicy) (*StubbedMethod, error) {
	ctx := methodContext(file, class, method)
	if method.Method.Class != class.Descriptor {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("method owner %q does not match class", method.Method.Class))
	}
	if method.Method.Name == "" {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("method name is empty"))
	}
	if int(method.Method.ProtoIndex) >= len(file.Protos) {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("proto index %d is out of range", method.Method.ProtoIndex))
	}
	proto := file.Protos[method.Method.ProtoIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return nil, methodDiagnostic(ctx, nil, "method", err)
	}
	access, err := methodAccess(method.AccessFlags, method.Method.Name)
	if err != nil {
		return nil, methodDiagnostic(ctx, nil, "method", err)
	}
	declarationOnly := method.AccessFlags&(dexAbstract|dexNative) != 0
	if declarationOnly && method.Code != nil {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("abstract/native method has code"))
	}
	if !declarationOnly && method.Code == nil {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("concrete method has no code"))
	}
	if method.Method.Name == "<init>" {
		if method.AccessFlags&dexStatic != 0 || proto.ReturnType != "V" {
			return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("constructor must be non-static and return void"))
		}
	} else if method.AccessFlags&dexConstructor != 0 && method.Method.Name != "<clinit>" {
		return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("constructor access flag is set on a non-initializer method"))
	}
	if method.Method.Name == "<clinit>" {
		if method.AccessFlags&dexStatic == 0 || len(proto.Parameters) != 0 || proto.ReturnType != "V" {
			return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("class initializer must be static with descriptor ()V"))
		}
	}
	if class.AccessFlags&dexInterface != 0 && method.Method.Name != "<clinit>" {
		if builder.Version() >= classfilebuilder.VersionJava8 {
			if method.AccessFlags&dexPublic == 0 {
				return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("Java 8 interface method must be public"))
			}
			if method.AccessFlags&dexAbstract == 0 && method.Code == nil {
				return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("interface method has neither code nor the abstract flag"))
			}
		} else if method.AccessFlags&(dexPublic|dexAbstract) != dexPublic|dexAbstract {
			return nil, methodDiagnostic(ctx, nil, "method", fmt.Errorf("Java 5 interface method must be public abstract"))
		}
	}

	var built *ir.Method
	var stub *StubbedMethod
	if !declarationOnly {
		built, err = ir.Build(ir.Input{File: file, Class: class, Method: method, SkipFileValidation: options.FileValidated})
		if err != nil {
			if policy != StubUnsupportedMethods || !report.IsUnsupported(err) {
				return nil, err
			}
			var capability *report.CapabilityError
			if !errors.As(err, &capability) {
				return nil, err
			}
			stub = &StubbedMethod{
				Method:     method.Method.Name,
				Descriptor: descriptor,
				Feature:    capability.Feature,
				Message:    capability.Error(),
			}
		} else if builder.Version() >= classfilebuilder.VersionJava8 && class.AccessFlags&dexInterface != 0 {
			if len(method.Code.Tries) > 0 || hasControlFlow(built.Instructions) {
				message := fmt.Sprintf("method %s has control flow, which needs Java 8 stack maps", method.Method.Name)
				if policy != StubUnsupportedMethods {
					return nil, classDiagnostic(file, class, "interface", errors.New(message))
				}
				stub = &StubbedMethod{
					Method:     method.Method.Name,
					Descriptor: descriptor,
					Feature:    "java8-interface-control-flow",
					Message:    message,
				}
			}
		}
	}

	member := builder.AddMethod(access, method.Method.Name, descriptor)
	if annotations := methodAnnotationsOf(class, method.MethodIndex); len(annotations) > 0 {
		translated, err := translateAnnotations(file, annotations)
		if err != nil {
			return nil, methodDiagnostic(ctx, nil, "method annotation", err)
		}
		if translated.signature != "" {
			builder.SetMethodSignature(member, translated.signature)
		}
		if len(translated.throws) > 0 {
			builder.SetMethodExceptions(member, translated.throws)
		}
		if translated.defaultVal != nil {
			builder.SetMethodAnnotationDefault(member, *translated.defaultVal)
		}
		for index, annotation := range translated.annotations {
			builder.AddMethodAnnotation(member, annotation, translated.visible[index])
		}
	}
	if parameters := parameterAnnotationsOf(class, method.MethodIndex); len(parameters) > 0 {
		if len(parameters) != len(proto.Parameters) {
			return nil, methodDiagnostic(ctx, nil, "parameter annotation", fmt.Errorf("parameter annotation count %d does not match descriptor parameter count %d", len(parameters), len(proto.Parameters)))
		}
		converted := make([][]classfilebuilder.Annotation, len(parameters))
		visible := make([][]bool, len(parameters))
		for index, set := range parameters {
			converted[index] = make([]classfilebuilder.Annotation, 0, len(set))
			visible[index] = make([]bool, 0, len(set))
			for _, annotation := range set {
				if isSystemAnnotation(annotation) {
					continue
				}
				value, err := builderAnnotation(file, annotation)
				if err != nil {
					return nil, methodDiagnostic(ctx, nil, "parameter annotation", err)
				}
				converted[index] = append(converted[index], value)
				visible[index] = append(visible[index], annotation.Visibility == model.AnnotationRuntime)
			}
		}
		builder.SetMethodParameterAnnotations(member, converted, visible)
	}
	if declarationOnly {
		return nil, nil
	}
	if stub != nil {
		if err := emitUnsupportedStub(builder, member, stub.Message); err != nil {
			return nil, methodDiagnostic(ctx, nil, "method stub", err)
		}
		return stub, nil
	}
	lowerer := methodLowerer{
		file:             file,
		class:            class,
		encoded:          method,
		method:           built,
		builder:          builder,
		code:             member.Code(),
		homeBase:         incomingWords(proto, method.AccessFlags),
		allocationByNew:  make(map[int]ir.Allocation),
		allocationByCtor: make(map[int]ir.Allocation),
		hierarchy:        options.Hierarchy,
	}
	if err := lowerer.lower(); err != nil {
		return nil, err
	}
	return nil, nil
}

func hasControlFlow(instructions []ir.Instruction) bool {
	for _, instruction := range instructions {
		if isBranchOpcode(instruction.Opcode) || instruction.Opcode == 0x2b || instruction.Opcode == 0x2c {
			return true
		}
	}
	return false
}

func emitUnsupportedStub(builder *classfilebuilder.Class, member *classfilebuilder.Method, reason string) error {
	const exception = "java/lang/UnsupportedOperationException"
	reason = sanitizeStubReason(reason)
	classIndex, err := builder.Pool().Class(exception)
	if err != nil {
		return err
	}
	messageIndex, err := builder.Pool().String("javajive dex2jar stub: " + reason)
	if err != nil {
		return err
	}
	constructor, err := builder.Pool().Methodref(exception, "<init>", "(Ljava/lang/String;)V")
	if err != nil {
		return err
	}
	member.Code().CP(classfilebuilder.NEW, classIndex).
		Emit(classfilebuilder.DUP).
		LDC(messageIndex).
		CP(classfilebuilder.INVOKESPECIAL, constructor).
		Emit(classfilebuilder.ATHROW)
	return nil
}

func sanitizeStubReason(reason string) string {
	reason = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, reason)
	runes := []rune(strings.TrimSpace(reason))
	if len(runes) > 240 {
		runes = runes[:240]
	}
	return string(runes)
}

func methodDescriptor(proto model.Proto) (string, error) {
	var descriptor strings.Builder
	descriptor.WriteByte('(')
	for _, parameter := range proto.Parameters {
		if parameter == "V" {
			return "", fmt.Errorf("void method parameter")
		}
		if _, err := fieldKind(parameter); err != nil {
			return "", fmt.Errorf("parameter descriptor: %w", err)
		}
		descriptor.WriteString(parameter)
	}
	descriptor.WriteByte(')')
	if proto.ReturnType == "V" {
		descriptor.WriteByte('V')
	} else {
		if _, err := fieldKind(proto.ReturnType); err != nil {
			return "", fmt.Errorf("return descriptor: %w", err)
		}
		descriptor.WriteString(proto.ReturnType)
	}
	return descriptor.String(), nil
}

func incomingWords(proto model.Proto, flags uint32) uint16 {
	words := 0
	for _, parameter := range proto.Parameters {
		words += descriptorWords(parameter)
	}
	if flags&dexStatic == 0 {
		words++
	}
	return uint16(words)
}

// descriptorWords is the JVM local and DEX register width of a descriptor.
func descriptorWords(descriptor string) int {
	if len(descriptor) == 1 && (descriptor[0] == 'J' || descriptor[0] == 'D') {
		return 2
	}
	return 1
}

func superclassName(class *model.Class) (string, error) {
	if class.Descriptor == "Ljava/lang/Object;" {
		if class.Superclass != "" {
			return "", fmt.Errorf("java/lang/Object must not declare a superclass")
		}
		return "", nil
	}
	if class.Superclass == "" {
		return "", fmt.Errorf("non-Object class has no superclass")
	}
	return objectInternalName(class.Superclass)
}

func objectInternalName(descriptor string) (string, error) {
	if !strings.HasPrefix(descriptor, "L") || !strings.HasSuffix(descriptor, ";") {
		return "", fmt.Errorf("descriptor %q is not an object type", descriptor)
	}
	return ir.InternalName(descriptor)
}

// memberOwnerName converts a member owner. Array descriptors are legal owners
// for methods (javac emits invokevirtual on a class constant such as "[I" for
// array clone), while fields only ever belong to object types.
func memberOwnerName(descriptor string) (string, error) {
	if strings.HasPrefix(descriptor, "[") {
		return descriptor, nil
	}
	return objectInternalName(descriptor)
}

func classAccess(flags uint32) (uint16, error) {
	const allowed = dexPublic | dexFinal | dexInterface | dexAbstract | dexSynthetic | dexAnnotation | dexEnum
	if flags&^allowed != 0 {
		return 0, fmt.Errorf("unsupported class access flags 0x%x", flags&^allowed)
	}
	access := uint16(flags)
	if flags&dexInterface == 0 {
		access |= classfilebuilder.ACC_SUPER
	} else if flags&dexAbstract == 0 {
		return 0, fmt.Errorf("interface is not abstract")
	}
	if flags&dexAnnotation != 0 && flags&dexInterface == 0 {
		return 0, fmt.Errorf("annotation is not an interface")
	}
	if flags&dexFinal != 0 && flags&dexAbstract != 0 {
		return 0, fmt.Errorf("class is both final and abstract")
	}
	if flags&dexInterface != 0 && flags&(dexFinal|dexEnum) != 0 {
		return 0, fmt.Errorf("interface has incompatible final/enum flags")
	}
	return access, nil
}

func fieldAccess(flags uint32) (uint16, error) {
	const allowed = dexPublic | dexPrivate | dexProtected | dexStatic | dexFinal | dexVolatile | dexTransient | dexSynthetic | dexEnum
	if flags&^allowed != 0 {
		return 0, fmt.Errorf("unsupported access flags 0x%x", flags&^allowed)
	}
	if err := validateVisibility(flags); err != nil {
		return 0, err
	}
	if flags&dexFinal != 0 && flags&dexVolatile != 0 {
		return 0, fmt.Errorf("field is both final and volatile")
	}
	return uint16(flags), nil
}

func methodAccess(flags uint32, name string) (uint16, error) {
	const allowed = dexPublic | dexPrivate | dexProtected | dexStatic | dexFinal | dexSynchronized | dexBridge | dexVarargs | dexNative | dexAbstract | dexStrict | dexSynthetic | dexConstructor | dexDeclaredSynchronized
	if flags&^allowed != 0 {
		return 0, fmt.Errorf("unsupported access flags 0x%x", flags&^allowed)
	}
	if err := validateVisibility(flags); err != nil {
		return 0, err
	}
	if flags&dexAbstract != 0 && flags&(dexPrivate|dexStatic|dexFinal|dexSynchronized|dexNative|dexStrict|dexDeclaredSynchronized) != 0 {
		return 0, fmt.Errorf("abstract method has incompatible access flags")
	}
	if name == "<clinit>" {
		// The JVM ignores class-initializer access flags except ACC_STATIC, and
		// DEX compilers routinely set public/private/synthetic/constructor bits
		// that carry no JVM meaning.
		return classfilebuilder.ACC_STATIC, nil
	}
	if flags&dexSynchronized != 0 && flags&dexNative == 0 {
		return 0, fmt.Errorf("DEX synchronized flag requires native method")
	}
	if flags&dexConstructor != 0 && name != "<init>" && name != "<clinit>" {
		return 0, fmt.Errorf("constructor access flag is set on %q", name)
	}
	if name == "<init>" && flags&(dexStatic|dexFinal|dexSynchronized|dexBridge|dexNative|dexAbstract|dexStrict|dexDeclaredSynchronized) != 0 {
		return 0, fmt.Errorf("constructor has incompatible access flags")
	}
	access := uint16(flags & 0xffff)
	if flags&dexDeclaredSynchronized != 0 {
		access |= classfilebuilder.ACC_SYNCHRONIZED
	}
	return access, nil
}

func validateVisibility(flags uint32) error {
	visibility := flags & (dexPublic | dexPrivate | dexProtected)
	if visibility != 0 && visibility&(visibility-1) != 0 {
		return fmt.Errorf("multiple visibility flags are set")
	}
	return nil
}

func fieldKind(descriptor string) (ir.CategoryKind, error) {
	kind, err := ir.DescriptorCategory(descriptor)
	if err != nil {
		return ir.Unknown, err
	}
	if kind == ir.Unknown {
		return ir.Unknown, fmt.Errorf("invalid field descriptor %q", descriptor)
	}
	return kind, nil
}

func classDiagnostic(file *model.File, class *model.Class, op string, err error) error {
	entry, descriptor := "", ""
	if file != nil {
		entry = file.Entry
	}
	if class != nil {
		descriptor = class.Descriptor
	}
	return report.New(entry, descriptor, "", -1, op, err)
}

func methodContext(file *model.File, class *model.Class, method *model.EncodedMethod) ir.Context {
	ctx := ir.Context{}
	if file != nil {
		ctx.Entry = file.Entry
	}
	if class != nil {
		ctx.Class = class.Descriptor
	}
	if method != nil {
		ctx.Method = method.Method.Name
	}
	return ctx
}

func methodDiagnostic(ctx ir.Context, instruction *ir.Instruction, op string, err error) error {
	offset := int64(-1)
	if instruction != nil {
		offset = int64(instruction.Offset)
		if op == "" {
			op = fmt.Sprintf("%s(0x%02x)", instruction.Name, instruction.Opcode)
		}
	}
	return report.New(ctx.Entry, ctx.Class, ctx.Method, offset, op, err)
}
