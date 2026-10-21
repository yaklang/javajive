package javaclassparser

import "sort"

type nativeLambdaFactoryScope struct {
	nativeLambdaBlockScope
	reference bool
}

// Direct block intervals hide their nested factories from their parent's
// event list. A compiler reference is a leaf at its real :: token; it has no
// body interval. Plain references cannot borrow this role: callers must close
// both the original factory inventory and each typed reference receipt.
func (c *ClassObjectDumper) nativeLambdaDirectFactoryScopes(source string, references bool, charge func(int64) bool) ([]nativeLambdaFactoryScope, bool) {
	blocks, known := c.nativeLambdaDirectBlockScopes(source, charge)
	if !known {
		return nil, false
	}
	var tokens []int
	if references {
		tokens, known = nativeLambdaSourceImplementationTokens(source, c.Work, true)
		if !known {
			return nil, false
		}
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(blocks)+len(tokens))*64) != nil {
		return nil, false
	}
	out := make([]nativeLambdaFactoryScope, 0, len(blocks)+len(tokens))
	for _, block := range blocks {
		out = append(out, nativeLambdaFactoryScope{nativeLambdaBlockScope: block})
	}
	block := 0
	for _, token := range tokens {
		if source[token:token+2] != "::" {
			continue
		}
		for block < len(blocks) && token > blocks[block].end {
			block++
		}
		if block < len(blocks) && token >= blocks[block].open && token < blocks[block].end {
			continue
		}
		out = append(out, nativeLambdaFactoryScope{nativeLambdaBlockScope: nativeLambdaBlockScope{arrow: token, open: -1, end: -1}, reference: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].arrow < out[j].arrow })
	return out, true
}
