package lower

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/model"
)

const RecoveryAttributeName = "JavaJiveRecovery"

const (
	recoveryMaxPayload = 64 << 10
	recoveryMaxRecords = 128
)

// RecoveryRecord is embedded in a degraded class and mirrored in the JAR
// diagnostic report. Version 1 is intentionally small and JSON-compatible.
type RecoveryRecord struct {
	Version    int    `json:"version"`
	Code       string `json:"code"`
	Action     string `json:"action"`
	Entry      string `json:"entry,omitempty"`
	Class      string `json:"class,omitempty"`
	Member     string `json:"member,omitempty"`
	Descriptor string `json:"descriptor,omitempty"`
	Reason     string `json:"reason"`
}

// Recovery describes one degradation used to emit ClassResult.Bytes.
type Recovery struct {
	Code       string
	Action     string
	Scope      string
	Member     string
	Descriptor string
	Feature    string
	Message    string
}

func recoveryPayload(file *model.File, class *model.Class, records []Recovery) ([]byte, error) {
	out := make([]RecoveryRecord, 0, len(records))
	for _, record := range records {
		out = append(out, RecoveryRecord{
			Version: 1, Code: sanitizeStubReason(record.Code), Action: sanitizeStubReason(record.Action),
			Entry: sanitizeStubReason(file.Entry), Class: sanitizeStubReason(class.Descriptor), Member: sanitizeStubReason(record.Member),
			Descriptor: sanitizeStubReason(record.Descriptor), Reason: sanitizeStubReason(record.Message),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Member != out[j].Member {
			return out[i].Member < out[j].Member
		}
		if out[i].Descriptor != out[j].Descriptor {
			return out[i].Descriptor < out[j].Descriptor
		}
		return out[i].Code < out[j].Code
	})
	original := len(out)
	if len(out) > recoveryMaxRecords-1 {
		out = out[:recoveryMaxRecords-1]
	}
	if len(out) != original {
		out = append(out, RecoveryRecord{Version: 1, Code: "recovery.records_truncated", Action: "drop_attribute", Entry: sanitizeStubReason(file.Entry), Class: sanitizeStubReason(class.Descriptor), Reason: fmt.Sprintf("%d recovery records were omitted from the class attribute; see JAR diagnostics", original-len(out))})
	}
	for {
		payload, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		if len(payload) <= recoveryMaxPayload {
			return payload, nil
		}
		if len(out) == 1 {
			return nil, fmt.Errorf("recovery attribute exceeds %d bytes", recoveryMaxPayload)
		}
		out = append(out[:len(out)-2], RecoveryRecord{Version: 1, Code: "recovery.records_truncated", Action: "drop_attribute", Entry: sanitizeStubReason(file.Entry), Class: sanitizeStubReason(class.Descriptor), Reason: "recovery records were omitted to fit the class attribute; see JAR diagnostics"})
	}
}

func addRecoveryAttribute(builder *classfilebuilder.Class, file *model.File, class *model.Class, records []Recovery) error {
	if len(records) == 0 {
		return nil
	}
	payload, err := recoveryPayload(file, class, records)
	if err != nil {
		return err
	}
	builder.AddClassAttribute(RecoveryAttributeName, payload)
	return nil
}

// emitClassShell rebuilds a class with a fresh builder. It preserves the class
// identity, hierarchy, fields, declarations and representable metadata while
// replacing every concrete method body with a throwing stub.
func emitClassShell(file *model.File, class *model.Class, options ClassOptions, cause error, reducedMetadata bool) (ClassResult, error) {
	className, err := objectInternalName(class.Descriptor)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}
	superName, err := recoverySuperclassName(class, true)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}
	access, err := classAccess(class.AccessFlags)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}
	builder := classfilebuilder.NewClass(className, superName, access)
	if interfaceNeedsJava8(class) {
		builder.SetVersion(classfilebuilder.VersionJava8)
	}
	if err := addShellInterfaces(class, builder); err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}

	root := Recovery{
		Code: "class.body_recovery", Action: "stub_class", Feature: "class-shell",
		Message: fmt.Sprintf("normal class translation failed; emitted declaration-preserving shell: %v", cause),
	}
	recoveries := []Recovery{root}
	if reducedMetadata && len(class.ParameterAnnotations) > 0 {
		recoveries = append(recoveries, Recovery{Code: "metadata.parameter_annotation_dropped", Action: "drop_attribute", Feature: "parameter-annotation", Message: "reduced declaration shell omits parameter annotations"})
	}
	if !reducedMetadata {
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
				specs = append(specs, classfilebuilder.InnerClassSpec{Inner: inner.Inner, Outer: inner.Outer, Name: inner.Name, Flags: inner.Flags})
			}
			builder.SetInnerClasses(specs)
		}
		if class.Enclosing != nil {
			builder.SetEnclosingMethod(classfilebuilder.EnclosingMethodSpec{Class: class.Enclosing.Class, Method: class.Enclosing.Method, Proto: class.Enclosing.Proto})
		}
		if translated, translateErr := translateAnnotations(file, class.Annotations); translateErr != nil {
			recoveries = append(recoveries, Recovery{Code: "metadata.class_annotation_dropped", Action: "drop_attribute", Message: translateErr.Error()})
		} else {
			if translated.signature != "" {
				builder.SetClassSignature(translated.signature)
			}
			for i, annotation := range translated.annotations {
				builder.AddClassAnnotation(annotation, translated.visible[i])
			}
		}
	}
	stubInitializer, err := addShellFields(file, class, builder, !reducedMetadata, &recoveries)
	if err != nil {
		return ClassResult{}, err
	}
	stubbed, err := addShellMethods(file, class, builder, !reducedMetadata, root.Message, stubInitializer, &recoveries)
	if err != nil {
		return ClassResult{}, err
	}
	if err := addRecoveryAttribute(builder, file, class, recoveries); err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}
	raw, err := builder.Bytes()
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "class recovery", err)
	}
	return ClassResult{Bytes: raw, StubbedMethods: stubbed, Recoveries: recoveries, ClassStubbed: true}, nil
}

func emitMinimalClassShell(file *model.File, class *model.Class, cause error) (ClassResult, error) {
	className, err := objectInternalName(class.Descriptor)
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "minimal class recovery", err)
	}
	access, normalized := minimalClassAccess(class.AccessFlags)
	superName, err := recoverySuperclassName(class, true)

	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "minimal class recovery", err)
	}
	builder := classfilebuilder.NewClass(className, superName, access)
	if class.AccessFlags&dexAnnotation != 0 {
		builder.AddInterface("java/lang/annotation/Annotation")
	}
	recoveries := []Recovery{{Code: "class.minimal_recovery", Action: "stub_class_minimal", Feature: "minimal-class-shell", Message: cause.Error()}}
	if normalized {
		recoveries = append(recoveries, Recovery{Code: "class.access_normalized", Action: "drop_attribute", Feature: "class-access", Message: fmt.Sprintf("minimal shell normalized unsupported class access flags 0x%x", class.AccessFlags)})
	}
	if len(class.ParameterAnnotations) > 0 {
		recoveries = append(recoveries, Recovery{Code: "metadata.parameter_annotation_dropped", Action: "drop_attribute", Feature: "parameter-annotation", Message: "minimal class shell omits parameter annotations"})
	}
	if err := addRecoveryAttribute(builder, file, class, recoveries); err != nil {
		return ClassResult{}, classDiagnostic(file, class, "minimal class recovery", err)
	}
	raw, err := builder.Bytes()
	if err != nil {
		return ClassResult{}, classDiagnostic(file, class, "minimal class recovery", err)
	}
	return ClassResult{Bytes: raw, Recoveries: recoveries, ClassStubbed: true, ClassMinimal: true}, nil
}

func minimalClassAccess(flags uint32) (uint16, bool) {
	if access, err := classAccess(flags); err == nil {
		return access, false
	}
	if flags&dexInterface != 0 {
		access := uint16(flags & (dexPublic | dexSynthetic | dexAnnotation))
		access |= classfilebuilder.ACC_INTERFACE | classfilebuilder.ACC_ABSTRACT
		if flags&dexAnnotation == 0 {
			access &^= classfilebuilder.ACC_ANNOTATION
		}
		return access, true
	}
	access := uint16(flags & (dexPublic | dexSynthetic))
	access |= classfilebuilder.ACC_ABSTRACT | classfilebuilder.ACC_SUPER
	return access, true
}

func recoverySuperclassName(class *model.Class, preserveKind bool) (string, error) {
	if preserveKind && class.AccessFlags&dexInterface != 0 {
		return "java/lang/Object", nil
	}
	return superclassName(class)
}

func addShellInterfaces(class *model.Class, builder *classfilebuilder.Class) error {
	seen := make(map[string]bool, len(class.Interfaces))
	annotationInterface := false
	for _, descriptor := range class.Interfaces {
		name, err := objectInternalName(descriptor)
		if err != nil {
			return err
		}
		if descriptor == class.Descriptor {
			return fmt.Errorf("class must not implement or extend itself")
		}
		if seen[descriptor] {
			return fmt.Errorf("duplicate interface %s", descriptor)
		}
		seen[descriptor] = true
		annotationInterface = annotationInterface || descriptor == "Ljava/lang/annotation/Annotation;"
		builder.AddInterface(name)
	}
	if class.AccessFlags&dexAnnotation != 0 && !annotationInterface {
		return fmt.Errorf("annotation interface must directly extend java/lang/annotation/Annotation")
	}
	return nil
}

func addShellFields(file *model.File, class *model.Class, builder *classfilebuilder.Class, metadata bool, recoveries *[]Recovery) (bool, error) {
	seen := make(map[string]bool)
	stubInitializer := false
	addFields := func(fields []model.EncodedField, isStatic bool) error {
		for index := range fields {
			field := &fields[index]
			if field.Field.Class != class.Descriptor {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q owner %q does not match class", field.Field.Name, field.Field.Class))
			}
			if _, err := fieldKind(field.Field.Type); err != nil {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q: %w", field.Field.Name, err))
			}
			if err := classfilebuilder.ValidateFieldSymbol(field.Field.Name, field.Field.Type); err != nil {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q: %w", field.Field.Name, err))
			}
			key := field.Field.Name + "\x00" + field.Field.Type
			if seen[key] {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("duplicate field %s %s", field.Field.Name, field.Field.Type))
			}
			seen[key] = true
			access, err := fieldAccess(field.AccessFlags)
			if err != nil {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q: %w", field.Field.Name, err))
			}
			if isStatic != (field.AccessFlags&dexStatic != 0) {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q static membership and access flag disagree", field.Field.Name))
			}
			if err := validateInterfaceFieldShape(class, field.AccessFlags); err != nil {
				return classDiagnostic(file, class, "class recovery field", fmt.Errorf("field %q: %w", field.Field.Name, err))
			}
			member := builder.AddField(access, field.Field.Name, field.Field.Type)
			if isStatic && index < len(class.StaticValues) {
				if valueErr := applyStaticValue(file, class, field, member, builder, class.StaticValues[index]); valueErr != nil {
					stubInitializer = true
					*recoveries = append(*recoveries, Recovery{Code: "field.initializer_stubbed", Action: "stub_initializer", Scope: "field", Member: field.Field.Name, Descriptor: field.Field.Type, Feature: "static-initializer", Message: valueErr.Error()})
				}
			}
			if !metadata {
				continue
			}
			if annotations := fieldAnnotationsOf(class, field.FieldIndex); len(annotations) > 0 {
				translated, translateErr := translateAnnotations(file, annotations)
				if translateErr != nil {
					*recoveries = append(*recoveries, Recovery{Code: "metadata.field_annotation_dropped", Action: "drop_attribute", Scope: "field", Member: field.Field.Name, Descriptor: field.Field.Type, Message: translateErr.Error()})
					continue
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
	if err := addFields(class.StaticFields, true); err != nil {
		return false, err
	}
	if err := addFields(class.InstanceFields, false); err != nil {
		return false, err
	}
	return stubInitializer, nil
}

func addShellMethods(file *model.File, class *model.Class, builder *classfilebuilder.Class, metadata bool, reason string, stubInitializer bool, recoveries *[]Recovery) ([]StubbedMethod, error) {
	seen := make(map[string]bool)
	var stubbed []StubbedMethod
	for _, method := range appendMethods(class.DirectMethods, class.VirtualMethods) {
		if method.Method.Class != class.Descriptor {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", fmt.Errorf("method owner %q does not match class", method.Method.Class))
		}
		if int(method.Method.ProtoIndex) >= len(file.Protos) {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", fmt.Errorf("proto index %d is out of range", method.Method.ProtoIndex))
		}
		proto := file.Protos[method.Method.ProtoIndex]
		descriptor, err := methodDescriptor(proto)
		if err != nil {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", err)
		}
		if err := classfilebuilder.ValidateMethodSymbol(method.Method.Name, descriptor); err != nil {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", err)
		}
		if incomingWords(proto, method.AccessFlags) > 255 {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", fmt.Errorf("parameter word count plus receiver exceeds 255"))
		}
		key := method.Method.Name + "\x00" + descriptor
		if seen[key] {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", fmt.Errorf("duplicate method %s%s", method.Method.Name, descriptor))
		}
		seen[key] = true
		access, err := methodAccess(method.AccessFlags, method.Method.Name)
		if err != nil {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", err)
		}
		if err := validateInterfaceMethodShape(class, &method, builder.Version()); err != nil {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery method", err)
		}
		declarationOnly := method.AccessFlags&(dexAbstract|dexNative) != 0
		if stubInitializer && method.Method.Name == "<clinit>" {
			declarationOnly = false
		}
		member := builder.AddMethod(access, method.Method.Name, descriptor)
		if metadata {
			if err := addShellMethodMetadata(file, class, &method, proto, descriptor, member, builder, recoveries); err != nil {
				return nil, err
			}
		}
		if declarationOnly {
			continue
		}
		stubReason := reason
		code := "method.body_stubbed"
		action := "stub_method"
		feature := "class-shell"
		if stubInitializer && method.Method.Name == "<clinit>" {
			stubReason = "javajive could not preserve one or more static field initializers"
			code = "class.initializer_stubbed"
			action = "stub_initializer"
			feature = "static-initializer"
		}
		if err := emitUnsupportedStub(builder, member, stubReason); err != nil {
			return nil, methodDiagnostic(methodContext(file, class, &method), nil, "class recovery stub", err)
		}
		stubbed = append(stubbed, StubbedMethod{Method: method.Method.Name, Descriptor: descriptor, Feature: feature, Message: stubReason})
		*recoveries = append(*recoveries, Recovery{Code: code, Action: action, Scope: "method", Member: method.Method.Name, Descriptor: descriptor, Feature: feature, Message: stubReason})
	}
	if stubInitializer && !seen["<clinit>\x00()V"] {
		member := builder.AddMethod(classfilebuilder.ACC_STATIC, "<clinit>", "()V")
		message := "javajive could not preserve one or more static field initializers"
		if err := emitUnsupportedStub(builder, member, message); err != nil {
			return nil, classDiagnostic(file, class, "class recovery initializer", err)
		}
		stubbed = append(stubbed, StubbedMethod{Method: "<clinit>", Descriptor: "()V", Feature: "static-initializer", Message: message})
		*recoveries = append(*recoveries, Recovery{Code: "class.initializer_stubbed", Action: "stub_initializer", Scope: "method", Member: "<clinit>", Descriptor: "()V", Feature: "static-initializer", Message: message})
	}
	return stubbed, nil
}

func addShellMethodMetadata(file *model.File, class *model.Class, method *model.EncodedMethod, proto model.Proto, descriptor string, member *classfilebuilder.Method, builder *classfilebuilder.Class, recoveries *[]Recovery) error {
	annotations := methodAnnotationsOf(class, method.MethodIndex)
	if len(annotations) > 0 {
		translated, err := translateAnnotations(file, annotations)
		if err != nil {
			*recoveries = append(*recoveries, Recovery{Code: "metadata.method_annotation_dropped", Action: "drop_attribute", Scope: "method", Member: method.Method.Name, Descriptor: descriptor, Message: err.Error()})
		} else {
			if translated.signature != "" {
				builder.SetMethodSignature(member, translated.signature)
			}
			if len(translated.throws) > 0 {
				for _, descriptor := range translated.throws {
					if err := classfilebuilder.ValidateObjectDescriptor(descriptor); err != nil {
						*recoveries = append(*recoveries, Recovery{Code: "metadata.method_throws_dropped", Action: "drop_attribute", Scope: "method", Member: method.Method.Name, Descriptor: descriptor, Message: err.Error()})
						translated.throws = nil
						break
					}
				}
				if len(translated.throws) > 0 {
					builder.SetMethodExceptions(member, translated.throws)
				}
			}
			if translated.defaultVal != nil {
				builder.SetMethodAnnotationDefault(member, *translated.defaultVal)
			}
			for i, annotation := range translated.annotations {
				builder.AddMethodAnnotation(member, annotation, translated.visible[i])
			}
		}
	}
	if parameters := parameterAnnotationsOf(class, method.MethodIndex); len(parameters) > 0 {
		converted, visible, err := translateParameterAnnotations(file, parameters, len(proto.Parameters))
		if err != nil {
			*recoveries = append(*recoveries, Recovery{Code: "metadata.parameter_annotation_dropped", Action: "drop_attribute", Scope: "method", Member: method.Method.Name, Descriptor: descriptor, Message: err.Error()})
		} else {
			builder.SetMethodParameterAnnotations(member, converted, visible)
		}
	}
	return nil
}
