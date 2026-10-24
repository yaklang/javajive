package javaclassparser

// Rendering contexts must remain distinct, but alpha-renaming is a property
// of the original implementation declaration. A shared original body can be
// rendered at several factories. Giving each render different local names
// defeats modern javac's structural lambda deduplication and invents observable
// implementation methods. Distinct originals still get disjoint namespaces,
// including when one body is inlined inside another.
func (c *ClassObjectDumper) lambdaLocalNamespaceID(method *MemberInfo) (int, bool) {
	if c == nil || c.obj == nil || method == nil || len(c.obj.Methods) > methodSourceLambdaRequestLimit || !nativeProofWork(c.Work, int64(len(c.obj.Methods))+1) {
		return 0, false
	}
	owned := 0
	for _, original := range c.obj.Methods {
		if original == method {
			owned++
		}
	}
	if owned != 1 {
		return 0, false
	}
	if id := c.lambdaLocalNamespaces[method]; id != 0 {
		return id, true
	}
	if len(c.lambdaLocalNamespaces) >= methodSourceLambdaRequestLimit || c.Work != nil && c.Work.CheckAlloc(int64(len(c.lambdaLocalNamespaces)+1)*32) != nil {
		return 0, false
	}
	if c.lambdaLocalNamespaces == nil {
		c.lambdaLocalNamespaces = map[*MemberInfo]int{}
	}
	c.lambdaLocalSeq++
	c.lambdaLocalNamespaces[method] = c.lambdaLocalSeq
	return c.lambdaLocalSeq, true
}
