package class_context

// AllocationRenderView observes the actual NEW renderer. Preview/debug renders
// may occur repeatedly; only the method's final source publisher can consume
// their observations. The callback can add removable boundary annotations, not
// replace an invocation or any of its operands.
type AllocationRenderView struct {
	Value  any
	End    bool
	Source string
}

// ConstructedInvocationView is emitted by the actual NEW source writer only
// after every selection/gate has succeeded. SourcePlan is the existing native
// allocation plan identity, never a new grant of lexical/access authority.
// Parameterized describes the writer's actual class-argument/diamond branch;
// argument IR types and an enclosing type alone cannot manufacture it.
type ConstructedInvocationView struct {
	Allocation                any
	Invocation                any
	Context                   *ClassContext
	SourcePlan                any
	Owner, PhysicalDescriptor string
	NewPC, InitPC             int
	Parameterized             bool
}
