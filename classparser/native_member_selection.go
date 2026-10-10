package javaclassparser

// Search states are bounded by the existing source-node profile and 4096
// attempts, independently of machine word size. Immutable bit strings provide
// exact memo keys without aliasing across sibling or nested continuations.
const nativeMemberLayoutNodeLimit = 1024

func nativeMemberSelectionEmpty(count int) string {
	return string(make([]byte, (count+7)/8))
}
func nativeMemberSelectionContains(bits string, index int) bool {
	return bits[index/8]&(1<<uint(index%8)) != 0
}
func nativeMemberSelectionAdd(bits string, index int) string {
	copy := []byte(bits)
	copy[index/8] |= 1 << uint(index%8)
	return string(copy)
}
