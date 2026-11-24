package types

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

type JavaPrimer struct {
	Name string
}

type javaPrimerAlloc struct {
	wrap   JavaTypeWrap
	ref    javaTypeRef
	primer JavaPrimer
}

func NewJavaPrimer(name string) JavaType {
	a := &javaPrimerAlloc{
		primer: JavaPrimer{Name: name},
	}
	a.ref.javaType = &a.primer
	a.wrap.javaTypeRef = &a.ref
	return &a.wrap
}

var (
	JavaChar    = "char"
	JavaInteger = "int"
	JavaLong    = "long"
	JavaDouble  = "double"
	JavaFloat   = "float"
	JavaBoolean = "boolean"
	JavaByte    = "byte"
	JavaShort   = "short"
	JavaString  = "String"
	JavaVoid    = "void"
)

func (j *JavaPrimer) String(funcCtx *class_context.ClassContext) string {
	return j.Name
}

func (j *JavaPrimer) IsJavaType() {}
