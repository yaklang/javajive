package statements

// SwitchDefault is independent of the signed int32 case domain, including -1.
// Index refers to the switch successor list; Offset preserves bytecode layout.
type SwitchDefault struct {
	Index  int
	Offset int32
}
