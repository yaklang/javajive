package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/callbinding"

// jdkObjectCatalogMetadata mirrors java/lang/Object from the pinned platform
// catalog. Object is the language root every class references, so describing it
// must not force a cold decode of the entire 11 MB catalog.
var jdkObjectCatalogMetadata = map[int]callbinding.Class{
	8: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "registerNatives", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Static: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
	9: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "registerNatives", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Static: true},
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
	11: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "registerNatives", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Static: true},
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
	16: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
	17: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
	21: {
		Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true,
		Parents: []string{},
		Methods: []callbinding.Method{
			{Name: "<init>", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "getClass", Desc: "()Ljava/lang/Class;", Signature: "()Ljava/lang/Class<*>;", Exceptions: []string{}, ExceptionsKnown: true, Public: true, Generic: true},
			{Name: "hashCode", Desc: "()I", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "equals", Desc: "(Ljava/lang/Object;)Z", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "clone", Desc: "()Ljava/lang/Object;", Exceptions: []string{"java/lang/CloneNotSupportedException"}, ExceptionsKnown: true},
			{Name: "toString", Desc: "()Ljava/lang/String;", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notify", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "notifyAll", Desc: "()V", Exceptions: []string{}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "()V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "wait0", Desc: "(J)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true},
			{Name: "wait", Desc: "(JI)V", Exceptions: []string{"java/lang/InterruptedException"}, ExceptionsKnown: true, Public: true},
			{Name: "finalize", Desc: "()V", Exceptions: []string{"java/lang/Throwable"}, ExceptionsKnown: true},
		},
	},
}

// jdkObjectMetadata returns a request-local copy. Every class references
// java/lang/Object, so this lookup must not decode the full platform catalog.
func jdkObjectMetadata(release int) (callbinding.Class, bool) {
	class, ok := jdkObjectCatalogMetadata[release]
	if !ok {
		return callbinding.Class{}, false
	}
	class.Parents = append([]string{}, class.Parents...)
	class.Methods = append([]callbinding.Method(nil), class.Methods...)
	for i := range class.Methods {
		class.Methods[i].Exceptions = append([]string{}, class.Methods[i].Exceptions...)
	}
	return class, true
}
