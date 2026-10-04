package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
	"github.com/yaklang/javajive/internal/utils"
)

type rewriterFunc func(statementManager *RewriteManager, node *core.Node) error

func ifBranchNodes(ifNode *core.Node) (trueNode, falseNode *core.Node) {
	if ifNode == nil {
		return nil, nil
	}
	if ifNode.TrueNode != nil {
		trueNode = ifNode.TrueNode()
	}
	if ifNode.FalseNode != nil {
		falseNode = ifNode.FalseNode()
	}
	if trueNode == nil && len(ifNode.Next) > 0 {
		trueNode = ifNode.Next[0]
	}
	if falseNode == nil && len(ifNode.Next) > 1 {
		falseNode = ifNode.Next[1]
	}
	return trueNode, falseNode
}

func IfRewriter(manager *RewriteManager, ifNode *core.Node) error {
	splitSharedFallthroughExpression(manager, ifNode)
	splitSharedLiteralPhiStores(manager, ifNode)
	splitSharedTerminalLeaves(manager, ifNode)
	err := CalcEnd(manager.DominatorMap, ifNode)
	if err != nil {
		return err
	}
	trueNode, falseNode := ifBranchNodes(ifNode)
	//ifNode.RemoveAllNext()
	if trueNode == falseNode {
		trueNode = nil
		trueNode = nil
	}
	domNodes := utils2.NodeFilter(ifNode.Next, func(node *core.Node) bool {
		return slices.Contains(manager.DominatorMap[ifNode], node)
	})
	for _, node := range domNodes {
		ifNode.RemoveNext(node)
	}
	ifStatement := statements.NewIfStatement(nil, nil, nil)
	originNodeStatement := ifNode.Statement

	ifStatementNode := manager.NewNode(ifStatement)
	ifNode.Replace(ifStatementNode)
	checkIsEndNode := func(node1, node2 *core.Node) bool {
		if node1 == nil || node2 == nil {
			return false
		}
		endNodes := []*core.Node{}
		core.WalkGraph[*core.Node](node1, func(node *core.Node) ([]*core.Node, error) {
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				}
			}
			if len(next) == 0 {
				endNodes = append(endNodes, node)
			}
			return next, nil
		})
		endNodes = NodeDeduplication(endNodes)
		hasNext := false
		for _, node := range endNodes {
			for _, n := range node.Next {
				if encodedJumpTo(node, n) {
					continue
				}
				hasNext = true
				if n != node2 {
					return false
				}
			}
		}
		if hasNext {
			return true
		}
		return false
	}
	if checkIsEndNode(trueNode, falseNode) {
		falseNode = nil
	}
	if checkIsEndNode(falseNode, trueNode) {
		trueNode = nil
	}
	endNodes := []*core.Node{}
	getBody := func(bodyStartNode *core.Node) ([](*core.Node), error) {
		sts := []*core.Node{}
		if !slices.Contains(manager.DominatorMap[ifNode], bodyStartNode) {
			return sts, nil
		}
		err := core.WalkGraph[*core.Node](bodyStartNode, func(node *core.Node) ([]*core.Node, error) {
			err := manager.CheckVisitedNode(node)
			if err != nil {
				// Node was already visited (shared merge point). Skip it instead
				// of failing the entire method.
				return nil, nil
			}
			sts = append(sts, node)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				} else {
					endNodes = append(endNodes, n)
				}
			}
			return next, nil
		})
		if err != nil {
			return nil, err
		}
		return sts, nil
	}
	condition := originNodeStatement.(*statements.ConditionStatement).Condition
	ifStatement.Condition = condition
	ifBodyNodes := []*core.Node{}
	copyIfBody := false
	// Normal-join extraction above can remove either arm: that path is now a
	// shared continuation, not a second body eligible for shared-body copying.
	if trueNode != nil && falseNode != nil && IsEndNode(ifNode.MergeNode) && len(trueNode.Source) > 1 && len(falseNode.Source) > 1 {
		copyIfBody = true
		trueNode.RemoveSource(ifStatementNode)
	}
	if trueNode != nil {
		if copyIfBody {
			sts := WalkNodeToList(trueNode)
			ifStatement.IfBody = core.NodesToStatements(sts)
			ifBodyNodes = append(ifBodyNodes, sts...)
		} else {
			ifBody, err := getBody(trueNode)
			if err != nil {
				return err
			}
			ifStatement.IfBody = core.NodesToStatements(ifBody)
			ifBodyNodes = append(ifBodyNodes, ifBody...)
		}
	}
	if falseNode != nil {
		elseBody, err := getBody(falseNode)
		if err != nil {
			return err
		}
		ifStatement.ElseBody = core.NodesToStatements(elseBody)
		ifBodyNodes = append(ifBodyNodes, elseBody...)
	}
	endNodes = utils2.NodeFilter(endNodes, func(node *core.Node) bool {
		if slices.Contains(ifBodyNodes, node) {
			return false
		}
		return !IsEndNode(node)
	})
	// An if whose arm can `return`/`throw` (e.g. Jackson's deserializeAndSet, where the true arm
	// has `if (skipNulls) return;`) ends up with the early-exit terminator AND the real fall-through
	// continuation both wired as Next of the structured if node. That gives the if two Next edges,
	// and the linear statement collector aborts with "multiple next", degrading the whole method to
	// a stub. The early-exit terminator is not a real continuation (it ends its path), so drop it -
	// but only when at least one genuine fall-through remains, so an if that fully terminates the
	// method (every exit is a return) is left with its terminal Next rather than no successor at all.
	hasFallThrough := false
	for _, n := range endNodes {
		if !isMethodExitTerminator(n) {
			hasFallThrough = true
			break
		}
	}
	if hasFallThrough {
		endNodes = utils2.NodeFilter(endNodes, func(node *core.Node) bool {
			return !isMethodExitTerminator(node)
		})
	}
	for _, node := range NodeDeduplication(endNodes) {
		ifStatementNode.AddNext(node)
	}
	markEncodedJumps(ifStatementNode, ifBodyNodes)
	retargetProtectedIfBoundary(manager.TryNodes, ifNode, ifStatementNode)

	return nil
}

// A shared expression followed by the opposite successor is a conditional
// edge block, not that condition's unconditional continuation. Its other
// incoming edge prevents dominance-based body collection from owning it.
// Split only this edge, retaining one execution on each original path. No
// allocation, invocation or exception is moved across the condition.
func splitSharedFallthroughExpression(manager *RewriteManager, condition *core.Node) {
	left, right := ifBranchNodes(condition)
	if left == nil || right == nil || left == right {
		return
	}
	for _, pair := range [][2]*core.Node{{left, right}, {right, left}} {
		target, continuation := pair[0], pair[1]
		st, ok := target.Statement.(*statements.ExpressionStatement)
		if !ok || st.Expression == nil || len(target.Source) < 2 || len(target.Next) != 1 ||
			target.Next[0] != continuation || target.HideNext != nil || target.IsTryCatch ||
			target.IsCatchStart || target.IsCircle || target.IsInCircle || target.LoopBreak ||
			len(target.EncodedJumps) != 0 || encodedJumpTo(condition, target) ||
			utils2.IsDominate(manager.DominatorMap, condition, target) ||
			!sameProtectedMembership(manager.RootNode, condition, target) {
			continue
		}
		copy := *st
		edge := manager.NewNode(&copy)
		edge.OriginPC, edge.HasOriginPC = target.OriginPC, target.HasOriginPC
		edge.AddNext(continuation)
		replaceNextInPlace(condition, target, edge)
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
		return
	}
}

func sameProtectedMembership(root, a, b *core.Node) bool {
	if !a.HasOriginPC || !b.HasOriginPC {
		return false
	}
	valid := true
	core.WalkGraph[*core.Node](root, func(node *core.Node) ([]*core.Node, error) {
		// IsTryCatch marks the pre-region anchor used during parsing. The
		// actual synthetic protected owner is MiddleTryStart and carries the
		// half-open range; reading the anchor as that owner loses the witness.
		middle, isMiddle := node.Statement.(*statements.MiddleStatement)
		if node.HasProtectedRange || (isMiddle && middle.Flag == statements.MiddleTryStart) {
			if !node.HasProtectedRange {
				valid = false
				return nil, nil
			}
			contains := func(pc int) bool {
				if len(node.SharedProtectedRanges) == 0 {
					return pc >= node.ProtectedStartPC && pc < node.ProtectedEndPC
				}
				for _, row := range node.SharedProtectedRanges {
					if pc >= int(row.StartPc) && pc < int(row.EndPc) {
						return true
					}
				}
				return false
			}
			valid = valid && contains(a.OriginPC) == contains(b.OriginPC)
		}
		return node.Next, nil
	})
	return valid
}

// Shared terminal leaves are not necessarily dominated by an inner condition.
// Split its selected edge before collecting dominated regions, or an abrupt
// arm can disappear. Preserve the ATHROW operand and original PC; its expression
// is still evaluated once on the selected path. Opaque custom statements and
// nonliteral value returns remain outside this narrowly proved transformation.
func splitSharedTerminalLeaves(manager *RewriteManager, condition *core.Node) {
	changed := false
	for _, target := range slices.Clone(condition.Next) {
		var terminalCopy statements.Statement
		switch st := target.Statement.(type) {
		case *statements.ReturnStatement:
			value := values.UnpackSoltValue(st.JavaValue)
			_, literal := value.(*values.JavaLiteral)
			literal = literal || value == values.JavaNull
			if st.JavaValue == nil || (literal && st.HasOriginPC && target.HasOriginPC &&
				st.OriginPC == target.OriginPC && sameProtectedMembership(manager.RootNode, condition, target)) {
				copy := *st
				terminalCopy = &copy
			}
		case *statements.CustomStatement:
			if st.ThrownValue != nil && st.HasOriginPC {
				copy := *st
				terminalCopy = &copy
			}
		}
		if terminalCopy == nil || len(target.Source) < 2 || target.HideNext != nil ||
			target.IsTryCatch || target.IsCatchStart || target.IsCircle || target.IsInCircle ||
			len(target.EncodedJumps) != 0 || encodedJumpTo(condition, target) ||
			utils2.IsDominate(manager.DominatorMap, condition, target) {
			continue
		}
		terminal := true
		for _, next := range target.Next {
			terminal = terminal && IsEndNode(next)
		}
		if !terminal {
			continue
		}
		leaf := manager.NewNode(terminalCopy)
		leaf.OriginPC, leaf.HasOriginPC = target.OriginPC, target.HasOriginPC
		for _, next := range target.Next {
			leaf.AddNext(next)
		}
		replaceNextInPlace(condition, target, leaf)
		changed = true
	}
	if changed {
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	}
}

func CalcEnd1(domTree map[*core.Node][]*core.Node, ifNode *core.Node) error {
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()

	// 获取从trueNode出发可以到达的所有节点和路径
	trueNodeSet := utils.NewSet[*core.Node]()
	trueNodePaths := make(map[*core.Node][][]*core.Node)
	trueNodePaths[trueNode] = [][]*core.Node{{trueNode, trueNode}}
	core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
		next := []*core.Node{}
		for _, n := range node.Next {
			if n != ifNode {
				next = append(next, n)
				// 记录到达n的所有路径
				if len(trueNodePaths[node]) == 0 {
					trueNodePaths[n] = append(trueNodePaths[n], []*core.Node{node, n})
				} else {
					for _, path := range trueNodePaths[node] {
						newPath := append(append([]*core.Node{}, path...), n)
						trueNodePaths[n] = append(trueNodePaths[n], newPath)
					}
				}
			}
		}
		trueNodeSet.Add(node)
		return next, nil
	})

	// 获取从falseNode出发可以到达的所有节点和路径
	falseNodeSet := utils.NewSet[*core.Node]()
	falseNodePaths := make(map[*core.Node][][]*core.Node)
	falseNodePaths[falseNode] = [][]*core.Node{{falseNode, falseNode}}
	core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
		next := []*core.Node{}
		for _, n := range node.Next {
			if n != ifNode {
				next = append(next, n)
				// 记录到达n的所有路径
				if len(falseNodePaths[node]) == 0 {
					falseNodePaths[n] = append(falseNodePaths[n], []*core.Node{node, n})
				} else {
					for _, path := range falseNodePaths[node] {
						newPath := append(append([]*core.Node{}, path...), n)
						falseNodePaths[n] = append(falseNodePaths[n], newPath)
					}
				}
			}
		}
		falseNodeSet.Add(node)
		return next, nil
	})

	// 找到所有true和false分支路径都经过的节点中最近的一个
	var mergeNode *core.Node
	minDepth := -1
	for node := range trueNodePaths {
		if falseNodePaths[node] != nil {
			depth := len(trueNodePaths[node][0])
			if minDepth == -1 || depth < minDepth {
				minDepth = depth
				mergeNode = node
			}
		}
	}
	ifNode.MergeNode = mergeNode
	return nil
}
func __CalcEnd(domTree map[*core.Node][]*core.Node, ifNode *core.Node) error {
	ifNode.MergeNode = nil
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()

	// 获取从trueNode和falseNode出发可以到达的所有节点
	trueNodeSet := utils.NewSet[*core.Node]()
	falseNodeSet := utils.NewSet[*core.Node]()

	// 遍历true分支可达的所有节点
	if trueNode != nil {
		err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode {
				return nil, nil
			}
			trueNodeSet.Add(node)
			return node.Next, nil
		})
		if err != nil {
			return err
		}
	}

	// 遍历false分支可达的所有节点
	if falseNode != nil {
		err := core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode {
				return nil, nil
			}
			falseNodeSet.Add(node)
			return node.Next, nil
		})
		if err != nil {
			return err
		}
	}

	// 找出两个分支都经过的节点中,最先被访问到的那个节点作为汇聚点
	var mergeNode *core.Node
	minDepth := -1

	// 遍历true分支节点 (sortNodesByID: Set.List() 的 map 顺序随机, 下面以最短深度选汇聚点, 深度相等时
	// 先访问者胜出, 不排序会让同一 if 的 merge 节点在不同 run 间漂移 -> 非确定性结构化)
	for _, node := range sortNodesByID(trueNodeSet.List()) {
		if falseNodeSet.Has(node) {
			// 计算该节点到ifNode的最短路径长度
			depth := 0
			current := node
			for current != nil && current != ifNode {
				depth++
				if len(current.Source) > 0 {
					current = current.Source[0]
				} else {
					current = nil
				}
			}

			// 更新最短路径的汇聚点
			if minDepth == -1 || depth < minDepth {
				minDepth = depth
				mergeNode = node
			}
		}
	}

	ifNode.MergeNode = mergeNode
	return nil
}
func CalcEnd(domTree map[*core.Node][]*core.Node, ifNode *core.Node) error {
	if ifNode == nil {
		return nil
	}
	ifNode.MergeNode = nil
	trueNode, falseNode := ifBranchNodes(ifNode)
	if trueNode == nil || falseNode == nil {
		return nil
	}

	domTree = GenerateDominatorTree(ifNode)
	doms := domTree[ifNode]
	switch len(doms) {
	case 1:
		ok1 := false
		if trueNode != nil {
			err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok1 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
		}
		ok2 := false
		if falseNode != nil {
			err := core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok2 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
		}

		if ok1 && ok2 {
			ifNode.MergeNode = doms[0]
		}
	case 2:
		for _, dom := range doms {
			ok1 := false
			err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == dom {
					ok1 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			ok2 := false
			err = core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == dom {
					ok2 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			if ok1 && ok2 {
				ifNode.MergeNode = dom
				break
			}
		}
	case 3:
		candidates := utils2.NodeFilter(doms, func(node *core.Node) bool {
			return node != trueNode && node != falseNode
		})
		if len(candidates) > 0 {
			ifNode.MergeNode = candidates[0]
		}
	}
	return nil
}
