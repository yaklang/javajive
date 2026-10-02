package decompiler

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Snapshot the original graph before Rewrite mutates it. The oracle follows
// actual original edges, independently of labels, loop nesting, and source text.
type nestedLoopCFGNode struct {
	name        string
	conditional bool
	next        []int
}
type nestedLoopCFG struct {
	entry int
	nodes map[int]nestedLoopCFGNode
}

func snapshotNestedLoopCFG(t *testing.T, entry *core.Node) nestedLoopCFG {
	t.Helper()
	result := nestedLoopCFG{entry: entry.Id, nodes: map[int]nestedLoopCFGNode{}}
	var visit func(*core.Node)
	visit = func(n *core.Node) {
		if _, ok := result.nodes[n.Id]; ok {
			return
		}
		row := nestedLoopCFGNode{name: n.Statement.String(&class_context.ClassContext{})}
		if c, ok := n.Statement.(*statements.ConditionStatement); ok {
			literal, ok := c.Condition.(*values.JavaLiteral)
			if !ok {
				t.Fatalf("unsupported original condition %T", c.Condition)
			}
			row.name = fmt.Sprint(literal.Data)
			row.conditional = true
			if len(n.Next) != 2 {
				t.Fatalf("condition %d must retain two ordered edges", n.Id)
			}
		} else if len(n.Next) > 1 {
			t.Fatalf("effect %d has ambiguous outgoing edges", n.Id)
		}
		for _, next := range n.Next {
			row.next = append(row.next, next.Id)
		}
		result.nodes[n.Id] = row
		for _, next := range n.Next {
			visit(next)
		}
	}
	visit(entry)
	return result
}

type nestedLoopDecisions struct {
	bits  uint
	used  int
	trace []string
	fuel  int
}

func (d *nestedLoopDecisions) condition(name string) bool {
	value := d.used < 10 && d.bits&(1<<uint(d.used)) != 0
	d.used++
	d.trace = append(d.trace, fmt.Sprintf("%s=%t", name, value))
	return value
}
func (d *nestedLoopDecisions) tick() bool { d.fuel--; return d.fuel >= 0 }
func nestedLoopOriginalTrace(g nestedLoopCFG, bits uint) ([]string, error) {
	d := nestedLoopDecisions{bits: bits, fuel: 200}
	id := g.entry
	for d.tick() {
		n, ok := g.nodes[id]
		if !ok {
			return nil, fmt.Errorf("missing original node %d", id)
		}
		if n.conditional {
			edge := 1
			if d.condition(n.name) {
				edge = 0
			}
			id = n.next[edge]
			continue
		}
		d.trace = append(d.trace, n.name)
		if len(n.next) == 0 {
			return d.trace, nil
		}
		id = n.next[0]
	}
	return nil, fmt.Errorf("original graph did not terminate")
}

type nestedLoopTransfer struct{ kind, label string }

func nestedLoopStructuredTrace(body []statements.Statement, bits uint) ([]string, error) {
	d := nestedLoopDecisions{bits: bits, fuel: 400}
	var evalList func([]statements.Statement) (nestedLoopTransfer, error)
	evalCondition := func(v values.JavaValue) (bool, error) {
		lit, ok := v.(*values.JavaLiteral)
		if !ok {
			return false, fmt.Errorf("unsupported structured condition %T", v)
		}
		switch x := lit.Data.(type) {
		case bool:
			return x, nil
		case string:
			return d.condition(x), nil
		}
		return false, fmt.Errorf("unsupported literal %v", lit.Data)
	}
	evalList = func(list []statements.Statement) (nestedLoopTransfer, error) {
		for _, st := range list {
			if !d.tick() {
				return nestedLoopTransfer{}, fmt.Errorf("structured graph did not terminate")
			}
			switch x := st.(type) {
			case *statements.IfStatement:
				b, err := evalCondition(x.Condition)
				if err != nil {
					return nestedLoopTransfer{}, err
				}
				arm := x.ElseBody
				if b {
					arm = x.IfBody
				}
				tr, err := evalList(arm)
				if err != nil || tr.kind != "" {
					return tr, err
				}
			case *statements.DoWhileStatement:
				for {
					if !d.tick() {
						return nestedLoopTransfer{}, fmt.Errorf("structured loop did not terminate")
					}
					tr, err := evalList(x.Body)
					if err != nil {
						return tr, err
					}
					own := tr.label == "" || tr.label == x.Label
					if tr.kind != "" && !own {
						return tr, nil
					}
					if tr.kind == "break" {
						break
					}
					if tr.kind != "" && tr.kind != "continue" {
						return tr, fmt.Errorf("unexpected loop transfer %s", tr.kind)
					}
					b, err := evalCondition(x.ConditionValue)
					if err != nil {
						return tr, err
					}
					if !b {
						break
					}
				}
			case *statements.CustomStatement:
				text := strings.TrimSpace(x.String(&class_context.ClassContext{}))
				parts := strings.Fields(text)
				if len(parts) > 0 && (parts[0] == "continue" || parts[0] == "break") {
					if len(parts) > 2 {
						return nestedLoopTransfer{}, fmt.Errorf("invalid transfer %q", text)
					}
					tr := nestedLoopTransfer{kind: parts[0]}
					if len(parts) == 2 {
						tr.label = parts[1]
					}
					return tr, nil
				}
				switch text {
				case "start", "loop2 body", "loop1 end":
					d.trace = append(d.trace, text)
				default:
					return nestedLoopTransfer{}, fmt.Errorf("unexpected reachable leaf %q", text)
				}
			default:
				return nestedLoopTransfer{}, fmt.Errorf("unsupported structured statement %T", st)
			}
		}
		return nestedLoopTransfer{}, nil
	}
	tr, err := evalList(body)
	if err != nil {
		return nil, err
	}
	if tr.kind != "" {
		return nil, fmt.Errorf("escaped transfer %+v", tr)
	}
	return d.trace, nil
}
func assertNestedLoopCFGPaths(t *testing.T, original nestedLoopCFG, body []statements.Statement) {
	t.Helper()
	// All 1024 branch tapes cover zero and repeated inner iterations, multiple
	// outer entries, both exits, and termination after the tape's last decision.
	for bits := uint(0); bits < 1024; bits++ {
		want, err := nestedLoopOriginalTrace(original, bits)
		if err != nil {
			t.Fatal(err)
		}
		got, err := nestedLoopStructuredTrace(body, bits)
		if err != nil {
			t.Fatalf("tape %010b: %v", bits, err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("tape %010b: original path %v; structured path %v", bits, want, got)
		}
	}
}

func TestNestedLoopPathOracleRejectsEscapingTransfer(t *testing.T) {
	transfer := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "continue UNKNOWN" }, nil)
	loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, nil), []statements.Statement{transfer})
	loop.Label = "OWNER"
	if _, err := nestedLoopStructuredTrace([]statements.Statement{loop}, 0); err == nil {
		t.Fatal("an unresolved continue target must not be accepted as a valid loop path")
	}
}

func TestNestedLoopPathOracleBoundsEmptyInfiniteLoop(t *testing.T) {
	loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, nil), nil)
	if _, err := nestedLoopStructuredTrace([]statements.Statement{loop}, 0); err == nil {
		t.Fatal("an emptied infinite loop must fail the bounded path oracle")
	}
}
