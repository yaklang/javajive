package class_context

import "testing"

// An archive-only sibling lookup is authoritative for the flat spelling. A
// Kotlin stdlib class in kotlin.collections referring to kotlin.RequiresOptIn's
// nested Level type must use the emitted flat unit, not the stdlib dotted
// shortcut. External JDK nested types keep the dotted spelling.
func TestNestedTypeShouldDotPrefersArchiveSiblingLookup(t *testing.T) {
	f := &ClassContext{PackageName: "kotlin.collections"}
	f.SiblingSuperTypes = func(name string) ([]string, bool) {
		return nil, name == "kotlin/RequiresOptIn"
	}
	f.SiblingClassAccessible = func(name string) (bool, bool) {
		return true, name == "kotlin/RequiresOptIn"
	}
	if f.nestedTypeShouldDot("kotlin", "RequiresOptIn$Level") {
		t.Fatal("archive sibling was dotted instead of using its flat source unit")
	}
	if !f.nestedTypeShouldDot("java.util", "Map$Entry") {
		t.Fatal("external stdlib nested type was flattened")
	}

	// Without an archive callback the resolver-only path still keeps the
	// established behavior: a resolved sibling stays flat.
	f.SiblingClassAccessible = nil
	if f.nestedTypeShouldDot("kotlin", "RequiresOptIn$Level") {
		t.Fatal("resolved sibling was dotted without an archive callback")
	}
	if !f.nestedTypeShouldDot("java.util", "Map$Entry") {
		t.Fatal("external stdlib nested type was flattened without an archive callback")
	}
}
