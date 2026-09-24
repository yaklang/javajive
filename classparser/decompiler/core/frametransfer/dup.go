package frametransfer

import "github.com/yaklang/javajive/classparser/decompiler/core"

type stackVal struct {
	v     Type
	width int
}

func (f *Frame) popNComputational(n int) ([]stackVal, error) {
	out := make([]stackVal, n)
	for i := n - 1; i >= 0; i-- {
		if len(f.Stack) == 0 {
			return nil, invalidf("stack underflow")
		}
		top := f.Stack[len(f.Stack)-1]
		if top.Kind.IsTail() {
			v, err := f.popValue()
			if err != nil {
				return nil, err
			}
			out[i] = stackVal{v: v, width: 2}
			continue
		}
		if top.Kind.IsCat2Head() {
			return nil, invalidf("category-2 head on top")
		}
		if top.Kind == Top {
			return nil, invalidf("top on stack")
		}
		v, err := f.popValue()
		if err != nil {
			return nil, err
		}
		out[i] = stackVal{v: v, width: 1}
	}
	return out, nil
}

func (f *Frame) pushVals(vs ...stackVal) error {
	for _, v := range vs {
		if err := f.push(v.v); err != nil {
			return err
		}
	}
	return nil
}

func (f *Frame) dupFamily(op int) error {
	switch op {
	case core.OP_DUP:
		vs, err := f.popNComputational(1)
		if err != nil {
			return err
		}
		if vs[0].width != 1 {
			return invalidf("dup requires category-1")
		}
		return f.pushVals(vs[0], vs[0])
	case core.OP_DUP_X1:
		vs, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if vs[0].width != 1 || vs[1].width != 1 {
			return invalidf("dup_x1 requires two category-1 values")
		}
		return f.pushVals(vs[1], vs[0], vs[1])
	case core.OP_DUP_X2:
		vs, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if vs[1].width != 1 {
			return invalidf("dup_x2 requires category-1 top")
		}
		if vs[0].width == 2 {
			return f.pushVals(vs[1], vs[0], vs[1])
		}
		vs3, err := f.popNComputational(1)
		if err != nil {
			return err
		}
		if vs3[0].width != 1 {
			return invalidf("dup_x2 form1 requires three category-1 values")
		}
		return f.pushVals(vs[1], vs3[0], vs[0], vs[1])
	case core.OP_DUP2:
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		top := f.Stack[len(f.Stack)-1]
		if top.Kind.IsTail() {
			vs, err := f.popNComputational(1)
			if err != nil {
				return err
			}
			if vs[0].width != 2 {
				return invalidf("dup2 form2 requires category-2")
			}
			return f.pushVals(vs[0], vs[0])
		}
		vs, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if vs[0].width != 1 || vs[1].width != 1 {
			return invalidf("dup2 form1 requires two category-1 values")
		}
		return f.pushVals(vs[0], vs[1], vs[0], vs[1])
	case core.OP_DUP2_X1:
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		top := f.Stack[len(f.Stack)-1]
		if top.Kind.IsTail() {
			vs, err := f.popNComputational(2)
			if err != nil {
				return err
			}
			if vs[1].width != 2 || vs[0].width != 1 {
				return invalidf("dup2_x1 form2 requires cat2 under cat1")
			}
			return f.pushVals(vs[1], vs[0], vs[1])
		}
		vs, err := f.popNComputational(3)
		if err != nil {
			return err
		}
		if vs[0].width != 1 || vs[1].width != 1 || vs[2].width != 1 {
			return invalidf("dup2_x1 form1 requires three category-1 values")
		}
		return f.pushVals(vs[1], vs[2], vs[0], vs[1], vs[2])
	case core.OP_DUP2_X2:
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		top := f.Stack[len(f.Stack)-1]
		if top.Kind.IsTail() {
			v1, err := f.popNComputational(1)
			if err != nil {
				return err
			}
			if v1[0].width != 2 {
				return invalidf("dup2_x2 top must be category-2")
			}
			if len(f.Stack) == 0 {
				return invalidf("stack underflow")
			}
			under := f.Stack[len(f.Stack)-1]
			if under.Kind.IsTail() {
				v2, err := f.popNComputational(1)
				if err != nil {
					return err
				}
				if v2[0].width != 2 {
					return invalidf("dup2_x2 form4 requires two category-2 values")
				}
				return f.pushVals(v1[0], v2[0], v1[0])
			}
			v2, err := f.popNComputational(2)
			if err != nil {
				return err
			}
			if v2[0].width != 1 || v2[1].width != 1 {
				return invalidf("dup2_x2 form2 requires cat2 then two cat1")
			}
			return f.pushVals(v1[0], v2[0], v2[1], v1[0])
		}
		v1, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if v1[0].width != 1 || v1[1].width != 1 {
			return invalidf("dup2_x2 form1/3 top pair must be category-1")
		}
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		under := f.Stack[len(f.Stack)-1]
		if under.Kind.IsTail() {
			v2, err := f.popNComputational(1)
			if err != nil {
				return err
			}
			if v2[0].width != 2 {
				return invalidf("dup2_x2 form3 requires category-2 under two cat1")
			}
			return f.pushVals(v1[0], v1[1], v2[0], v1[0], v1[1])
		}
		v2, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if v2[0].width != 1 || v2[1].width != 1 {
			return invalidf("dup2_x2 form1 requires four category-1 values")
		}
		return f.pushVals(v1[0], v1[1], v2[0], v2[1], v1[0], v1[1])
	case core.OP_POP:
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		if f.Stack[len(f.Stack)-1].Kind.IsTail() || f.Stack[len(f.Stack)-1].Kind.IsCat2Head() {
			return invalidf("pop requires category-1")
		}
		_, err := f.popValue()
		return err
	case core.OP_POP2:
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		if f.Stack[len(f.Stack)-1].Kind.IsTail() {
			_, err := f.popValue()
			return err
		}
		if f.Stack[len(f.Stack)-1].Kind.IsCat2Head() {
			return invalidf("category-2 head on top")
		}
		if _, err := f.popValue(); err != nil {
			return err
		}
		if len(f.Stack) == 0 {
			return invalidf("stack underflow")
		}
		if f.Stack[len(f.Stack)-1].Kind.IsTail() || f.Stack[len(f.Stack)-1].Kind.IsCat2Head() {
			return invalidf("pop2 form1 requires two category-1 values")
		}
		_, err := f.popValue()
		return err
	case core.OP_SWAP:
		vs, err := f.popNComputational(2)
		if err != nil {
			return err
		}
		if vs[0].width != 1 || vs[1].width != 1 {
			return invalidf("swap requires two category-1 values")
		}
		return f.pushVals(vs[1], vs[0])
	}
	return unsupportedf("not a stack-move opcode")
}
