package methodir

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// SnapshotVersion is the MethodIR schema version. It is constant for a built snapshot
// and does not change when the old printer mutates AST or opcode Target lists.
const SnapshotVersion uint64 = 1

type MethodID string
type BlockID uint32
type InstrID uint16
type ValueID uint32

type EdgeID struct {
	From         InstrID
	To           InstrID
	Kind         core.EdgeKind
	CaseValue    int32
	HandlerOrder int
}

func (e EdgeID) String() string {
	return fmt.Sprintf("%d>%d:k%d:c%d:h%d", e.From, e.To, e.Kind, e.CaseValue, e.HandlerOrder)
}

type MethodMeta struct {
	ClassName  string
	Name       string
	Descriptor string
	Bytecode   []byte
	IsStatic   bool
}

type ConstKind uint8

const (
	ConstNone ConstKind = iota
	ConstInt
	ConstLong
	ConstFloat
	ConstDouble
	ConstString
	ConstClass
)

type Const struct {
	Kind       ConstKind
	Int        int32
	Long       int64
	FloatBits  uint32
	DoubleBits uint64
	String     string
	Class      string
}

type SwitchCase struct {
	Key      int32
	TargetPC uint16
}

type Instr struct {
	ID        InstrID
	PC        uint16
	Opcode    int
	Name      string
	Wide      bool
	Data      []byte
	Local     int
	IincConst int32
	CPIndex   uint16
	Class     string
	Member    string
	Desc      string
	Const     Const
	Cases     []SwitchCase
	DefaultPC int32
	MayThrow  bool
	Result    ValueID
}

type Edge struct {
	ID           EdgeID
	From         InstrID
	To           InstrID
	Kind         core.EdgeKind
	CaseValue    int32
	HandlerOrder int
	CatchType    uint16
}

type Handler struct {
	Order     int
	StartPC   uint16
	EndPC     uint16
	HandlerPC uint16
	CatchType uint16
}

type Value struct {
	ID    ValueID
	Kind  string
	PC    uint16
	Slot  int
	Param int
}

type Block struct {
	ID       BlockID
	FirstPC  uint16
	InstrIDs []InstrID
}

// MethodIR is an immutable semantic snapshot copied from SemanticCFG.
type MethodIR struct {
	ID         MethodID
	Version    uint64
	ClassName  string
	Name       string
	Descriptor string
	IsStatic   bool
	Bytecode   []byte
	Blocks     []Block
	Instrs     []Instr
	Edges      []Edge
	Handlers   []Handler
	Values     []Value
	EntryPC    uint16
	instrIndex map[InstrID]int
}

func (m *MethodIR) InstrByID(id InstrID) (Instr, bool) {
	if m == nil || m.instrIndex == nil {
		return Instr{}, false
	}
	i, ok := m.instrIndex[id]
	if !ok || i < 0 || i >= len(m.Instrs) {
		return Instr{}, false
	}
	return m.Instrs[i], true
}

func (m *MethodIR) BlockOf(pc InstrID) (Block, bool) {
	if m == nil {
		return Block{}, false
	}
	for _, b := range m.Blocks {
		for _, id := range b.InstrIDs {
			if id == pc {
				return b, true
			}
		}
	}
	return Block{}, false
}

func KindName(k core.EdgeKind) string {
	switch k {
	case core.EdgeFallthrough:
		return "fallthrough"
	case core.EdgeTaken:
		return "taken"
	case core.EdgeCase:
		return "case"
	case core.EdgeDefault:
		return "default"
	case core.EdgeException:
		return "exception"
	default:
		return fmt.Sprintf("kind%d", k)
	}
}
