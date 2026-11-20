//go:build !javajive_full_jdk

package javaclassparser

const embeddedJDKConstructorBundleEnabled = false

// In default builds (without `-tags javajive_full_jdk`), the 7.7 MiB embedded
// JDK classfile bytecode bundle is omitted to keep downstream binaries slim.
// Callers still have the compact (~132 KiB) JDK invocation/hierarchy catalog
// (jdk_metadata_catalog.json.zst) and any explicit declaration resolvers.
func jdkConstructorClassBytes(name string, release int) ([]byte, bool) {
	return nil, false
}
