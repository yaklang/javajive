package values

import (
	"sync"

	"github.com/yaklang/javajive/internal/jdecenv"
)

// maxReplaceWalkDepth stops a long chain of distinct ternaries or calls from
// growing the goroutine stack until the process stalls. Nodes already in the
// walk are also skipped via the replacing flag.
const maxReplaceWalkDepth = 128

var (
	replaceWalkMu    sync.Mutex
	replaceWalkDepth = map[uint64]int{}
)

func enterReplaceWalk() bool {
	id := jdecenv.Goid()
	replaceWalkMu.Lock()
	defer replaceWalkMu.Unlock()
	if replaceWalkDepth[id] >= maxReplaceWalkDepth {
		return false
	}
	replaceWalkDepth[id]++
	return true
}

func leaveReplaceWalk() {
	id := jdecenv.Goid()
	replaceWalkMu.Lock()
	defer replaceWalkMu.Unlock()
	n := replaceWalkDepth[id]
	if n <= 1 {
		delete(replaceWalkDepth, id)
		return
	}
	replaceWalkDepth[id] = n - 1
}
