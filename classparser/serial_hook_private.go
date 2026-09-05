package javaclassparser

import "os"

// isSerializationHookMethod reports methods the JVM serialization machinery
// invokes by name. nestDemotePrivate must not strip `private` from these:
// a package-private readResolve in a superclass becomes an override target
// for a still-private subclass hook ("cannot override; weaker access").
// Kill-switch: JDEC_SERIAL_HOOK_PRIVATE_OFF=1 restores the old demote.
func isSerializationHookMethod(name string) bool {
	if os.Getenv("JDEC_SERIAL_HOOK_PRIVATE_OFF") == "1" {
		return false
	}
	switch name {
	case "readResolve", "writeReplace", "readObject", "writeObject", "readObjectNoData":
		return true
	}
	return false
}
