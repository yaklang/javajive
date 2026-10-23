package values

import (
	"sync"

	"github.com/yaklang/javajive/internal/jdecenv"
)

// maxReplaceVisits is the number of nodes one top-level ReplaceVar may touch.
// A binary tree of distinct ternaries is exponential in depth; a depth cap
// still visits 2^depth nodes. The counter is not decremented until the
// outermost call returns, so the whole walk shares one budget.
const maxReplaceVisits = 10000

var (
	replaceWalkMu    sync.Mutex
	replaceWalkCount = map[uint64]int{}
)

func beginReplaceWalk() (proceed bool, root bool) {
	id := jdecenv.Goid()
	replaceWalkMu.Lock()
	defer replaceWalkMu.Unlock()
	n := replaceWalkCount[id]
	if n >= maxReplaceVisits {
		return false, false
	}
	replaceWalkCount[id] = n + 1
	return true, n == 0
}

func endReplaceWalk(root bool) {
	if !root {
		return
	}
	id := jdecenv.Goid()
	replaceWalkMu.Lock()
	delete(replaceWalkCount, id)
	replaceWalkMu.Unlock()
}
