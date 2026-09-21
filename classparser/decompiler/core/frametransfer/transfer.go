package frametransfer

import (
	"math"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

func Transfer(in Frame, instr Instr) (out Frame, exceptionFrame *Frame, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = invalidf("panic: %v", r)
			out = Frame{}
			exceptionFrame = nil
		}
	}()
	out = in.Clone()
	op := instr.Op
	if op == core.OP_JSR || op == core.OP_JSR_W || op == core.OP_RET {
		return Frame{}, nil, unsupportedf("jsr/ret not inlined")
	}

	mayThrow := core.MayThrowOpcode(op)
	mkEx := func() *Frame {
		ef := Frame{Locals: append([]Type(nil), in.Locals...), Stack: []Type{RefOf("java/lang/Throwable")}}
		return &ef
	}

	bin := func(want Kind, res Kind) error {
		if _, err := out.popKind(want); err != nil {
			return err
		}
		if _, err := out.popKind(want); err != nil {
			return err
		}
		return out.push(T(res))
	}
	un := func(want Kind, res Kind) error {
		if _, err := out.popKind(want); err != nil {
			return err
		}
		return out.push(T(res))
	}
	shift := func(val Kind) error {
		if _, err := out.popKind(Int); err != nil {
			return err
		}
		v, err := out.popKind(val)
		if err != nil {
			return err
		}
		v = v.DropValue()
		return out.push(v)
	}

	var terr error
	switch op {
	case core.OP_NOP:
	case core.OP_ACONST_NULL:
		terr = out.push(T(Null))
	case core.OP_ICONST_M1, core.OP_ICONST_0, core.OP_ICONST_1, core.OP_ICONST_2, core.OP_ICONST_3, core.OP_ICONST_4, core.OP_ICONST_5:
		terr = out.push(IntConst(int32(op - core.OP_ICONST_0)))
	case core.OP_LCONST_0:
		terr = out.push(LongConst(0))
	case core.OP_LCONST_1:
		terr = out.push(LongConst(1))
	case core.OP_FCONST_0:
		terr = out.push(FloatBits(math.Float32bits(0)))
	case core.OP_FCONST_1:
		terr = out.push(FloatBits(math.Float32bits(1)))
	case core.OP_FCONST_2:
		terr = out.push(FloatBits(math.Float32bits(2)))
	case core.OP_DCONST_0:
		terr = out.push(DoubleBits(math.Float64bits(0)))
	case core.OP_DCONST_1:
		terr = out.push(DoubleBits(math.Float64bits(1)))
	case core.OP_BIPUSH, core.OP_SIPUSH:
		terr = out.push(T(Int))
	case core.OP_LDC, core.OP_LDC_W:
		terr = pushLDC(&out, instr, false)
	case core.OP_LDC2_W:
		terr = pushLDC(&out, instr, true)
	case core.OP_ILOAD, core.OP_ILOAD_0, core.OP_ILOAD_1, core.OP_ILOAD_2, core.OP_ILOAD_3:
		terr = load(&out, instr, Int)
	case core.OP_LLOAD, core.OP_LLOAD_0, core.OP_LLOAD_1, core.OP_LLOAD_2, core.OP_LLOAD_3:
		terr = load(&out, instr, Long)
	case core.OP_FLOAD, core.OP_FLOAD_0, core.OP_FLOAD_1, core.OP_FLOAD_2, core.OP_FLOAD_3:
		terr = load(&out, instr, Float)
	case core.OP_DLOAD, core.OP_DLOAD_0, core.OP_DLOAD_1, core.OP_DLOAD_2, core.OP_DLOAD_3:
		terr = load(&out, instr, Double)
	case core.OP_ALOAD, core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ALOAD_2, core.OP_ALOAD_3:
		terr = load(&out, instr, Ref)
	case core.OP_ISTORE, core.OP_ISTORE_0, core.OP_ISTORE_1, core.OP_ISTORE_2, core.OP_ISTORE_3:
		terr = store(&out, instr, Int)
	case core.OP_LSTORE, core.OP_LSTORE_0, core.OP_LSTORE_1, core.OP_LSTORE_2, core.OP_LSTORE_3:
		terr = store(&out, instr, Long)
	case core.OP_FSTORE, core.OP_FSTORE_0, core.OP_FSTORE_1, core.OP_FSTORE_2, core.OP_FSTORE_3:
		terr = store(&out, instr, Float)
	case core.OP_DSTORE, core.OP_DSTORE_0, core.OP_DSTORE_1, core.OP_DSTORE_2, core.OP_DSTORE_3:
		terr = store(&out, instr, Double)
	case core.OP_ASTORE, core.OP_ASTORE_0, core.OP_ASTORE_1, core.OP_ASTORE_2, core.OP_ASTORE_3:
		terr = store(&out, instr, Ref)
	case core.OP_IINC:
		terr = doIinc(&out, instr)
	case core.OP_POP, core.OP_POP2, core.OP_DUP, core.OP_DUP_X1, core.OP_DUP_X2, core.OP_DUP2, core.OP_DUP2_X1, core.OP_DUP2_X2, core.OP_SWAP:
		terr = out.dupFamily(op)
	case core.OP_IADD, core.OP_ISUB, core.OP_IMUL, core.OP_IDIV, core.OP_IREM, core.OP_IAND, core.OP_IOR, core.OP_IXOR:
		terr = bin(Int, Int)
	case core.OP_LADD, core.OP_LSUB, core.OP_LMUL, core.OP_LDIV, core.OP_LREM, core.OP_LAND, core.OP_LOR, core.OP_LXOR:
		terr = bin(Long, Long)
	case core.OP_FADD, core.OP_FSUB, core.OP_FMUL, core.OP_FDIV, core.OP_FREM:
		terr = floatBin(&out, op)
	case core.OP_DADD, core.OP_DSUB, core.OP_DMUL, core.OP_DDIV, core.OP_DREM:
		terr = doubleBin(&out, op)
	case core.OP_INEG:
		terr = un(Int, Int)
	case core.OP_LNEG:
		terr = un(Long, Long)
	case core.OP_FNEG:
		terr = floatUn(&out)
	case core.OP_DNEG:
		terr = doubleUn(&out)
	case core.OP_ISHL, core.OP_ISHR, core.OP_IUSHR:
		terr = shift(Int)
	case core.OP_LSHL, core.OP_LSHR, core.OP_LUSHR:
		terr = shift(Long)
	case core.OP_I2L:
		terr = conv(&out, Int, Long)
	case core.OP_I2F:
		terr = conv(&out, Int, Float)
	case core.OP_I2D:
		terr = conv(&out, Int, Double)
	case core.OP_L2I:
		terr = conv(&out, Long, Int)
	case core.OP_L2F:
		terr = conv(&out, Long, Float)
	case core.OP_L2D:
		terr = conv(&out, Long, Double)
	case core.OP_F2I:
		terr = conv(&out, Float, Int)
	case core.OP_F2L:
		terr = conv(&out, Float, Long)
	case core.OP_F2D:
		terr = conv(&out, Float, Double)
	case core.OP_D2I:
		terr = conv(&out, Double, Int)
	case core.OP_D2L:
		terr = conv(&out, Double, Long)
	case core.OP_D2F:
		terr = conv(&out, Double, Float)
	case core.OP_I2B, core.OP_I2C, core.OP_I2S:
		terr = un(Int, Int)
	case core.OP_LCMP:
		if _, err := out.popKind(Long); err != nil {
			terr = err
			break
		}
		if _, err := out.popKind(Long); err != nil {
			terr = err
			break
		}
		terr = out.push(T(Int))
	case core.OP_FCMPL, core.OP_FCMPG:
		if _, err := out.popKind(Float); err != nil {
			terr = err
			break
		}
		if _, err := out.popKind(Float); err != nil {
			terr = err
			break
		}
		terr = out.push(T(Int))
	case core.OP_DCMPL, core.OP_DCMPG:
		if _, err := out.popKind(Double); err != nil {
			terr = err
			break
		}
		if _, err := out.popKind(Double); err != nil {
			terr = err
			break
		}
		terr = out.push(T(Int))
	case core.OP_IFEQ, core.OP_IFNE, core.OP_IFLT, core.OP_IFGE, core.OP_IFGT, core.OP_IFLE:
		_, terr = out.popKind(Int)
	case core.OP_IF_ICMPEQ, core.OP_IF_ICMPNE, core.OP_IF_ICMPLT, core.OP_IF_ICMPGE, core.OP_IF_ICMPGT, core.OP_IF_ICMPLE:
		if _, err := out.popKind(Int); err != nil {
			terr = err
			break
		}
		_, terr = out.popKind(Int)
	case core.OP_IF_ACMPEQ, core.OP_IF_ACMPNE:
		if _, err := out.popKind(Ref); err != nil {
			terr = err
			break
		}
		_, terr = out.popKind(Ref)
	case core.OP_IFNULL, core.OP_IFNONNULL:
		_, terr = out.popKind(Ref)
	case core.OP_GOTO, core.OP_GOTO_W:
	case core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH:
		_, terr = out.popKind(Int)
	case core.OP_IRETURN:
		_, terr = out.popKind(Int)
	case core.OP_LRETURN:
		_, terr = out.popKind(Long)
	case core.OP_FRETURN:
		_, terr = out.popKind(Float)
	case core.OP_DRETURN:
		_, terr = out.popKind(Double)
	case core.OP_ARETURN:
		_, terr = out.popKind(Ref)
	case core.OP_RETURN:
	case core.OP_GETSTATIC:
		terr = fieldOp(&out, instr, true, false)
	case core.OP_PUTSTATIC:
		terr = fieldOp(&out, instr, true, true)
	case core.OP_GETFIELD:
		terr = fieldOp(&out, instr, false, false)
	case core.OP_PUTFIELD:
		terr = fieldOp(&out, instr, false, true)
	case core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE, core.OP_INVOKEDYNAMIC:
		terr = invoke(&out, instr)
	case core.OP_NEW:
		class := instr.Class
		if class == "" && instr.Const.Class != "" {
			class = instr.Const.Class
		}
		terr = out.push(UninitAt(instr.PC))
		_ = class
	case core.OP_NEWARRAY:
		if _, err := out.popKind(Int); err != nil {
			terr = err
			break
		}
		terr = out.push(RefOf(newArrayClass(instr.AType)))
	case core.OP_ANEWARRAY:
		if _, err := out.popKind(Int); err != nil {
			terr = err
			break
		}
		cls := instr.Class
		if cls == "" {
			cls = "java/lang/Object"
		}
		if len(cls) > 0 && cls[0] != '[' {
			cls = "[L" + cls + ";"
		}
		terr = out.push(RefOf(cls))
	case core.OP_ARRAYLENGTH:
		if _, err := out.popKind(Ref); err != nil {
			terr = err
			break
		}
		terr = out.push(T(Int))
	case core.OP_ATHROW:
		v, err := out.popKind(Ref)
		if err != nil {
			terr = err
			break
		}
		out.Stack = []Type{v}
	case core.OP_CHECKCAST:
		v, err := out.popKind(Ref)
		if err != nil {
			terr = err
			break
		}
		if instr.Class != "" {
			v.Class = instr.Class
			if v.Kind == Null {
				// null stays null
			} else if v.Kind == Ref {
				v.Class = instr.Class
			}
		}
		if v.Kind == Null {
			terr = out.push(v)
		} else {
			if instr.Class != "" {
				terr = out.push(RefOf(instr.Class))
			} else {
				terr = out.push(v)
			}
		}
	case core.OP_INSTANCEOF:
		if _, err := out.popKind(Ref); err != nil {
			terr = err
			break
		}
		terr = out.push(T(Int))
	case core.OP_MONITORENTER, core.OP_MONITOREXIT:
		_, terr = out.popKind(Ref)
	case core.OP_MULTIANEWARRAY:
		dims := instr.Dims
		if dims <= 0 {
			dims = 1
		}
		for i := 0; i < dims; i++ {
			if _, err := out.popKind(Int); err != nil {
				terr = err
				break
			}
		}
		if terr == nil {
			cls := instr.Class
			if cls == "" {
				cls = "java/lang/Object"
			}
			terr = out.push(RefOf(cls))
		}
	case core.OP_IALOAD, core.OP_BALOAD, core.OP_CALOAD, core.OP_SALOAD:
		terr = arrayLoad(&out, Int)
	case core.OP_LALOAD:
		terr = arrayLoad(&out, Long)
	case core.OP_FALOAD:
		terr = arrayLoad(&out, Float)
	case core.OP_DALOAD:
		terr = arrayLoad(&out, Double)
	case core.OP_AALOAD:
		terr = arrayLoad(&out, Ref)
	case core.OP_IASTORE, core.OP_BASTORE, core.OP_CASTORE, core.OP_SASTORE:
		terr = arrayStore(&out, Int)
	case core.OP_LASTORE:
		terr = arrayStore(&out, Long)
	case core.OP_FASTORE:
		terr = arrayStore(&out, Float)
	case core.OP_DASTORE:
		terr = arrayStore(&out, Double)
	case core.OP_AASTORE:
		terr = arrayStore(&out, Ref)
	default:
		return Frame{}, nil, unsupportedf("opcode %#x", op)
	}
	if terr != nil {
		if e, ok := terr.(*Error); ok {
			e.PC = instr.PC
			e.Op = op
		}
		return Frame{}, nil, terr
	}
	if mayThrow {
		return out, mkEx(), nil
	}
	return out, nil, nil
}

func load(f *Frame, instr Instr, want Kind) error {
	idx := instr.Local
	t, err := f.loadLocal(idx, want)
	if err != nil {
		return err
	}
	return f.push(t)
}

func store(f *Frame, instr Instr, want Kind) error {
	t, err := f.popKind(want)
	if err != nil {
		return err
	}
	return f.storeLocal(instr.Local, t)
}

func doIinc(f *Frame, instr Instr) error {
	idx := instr.Local
	if err := f.ensureLocal(idx); err != nil {
		return err
	}
	cur := f.Locals[idx]
	if cur.Kind.IsCat2Head() || cur.Kind.IsTail() || (idx-1 >= 0 && idx-1 < len(f.Locals) && f.Locals[idx-1].Kind.IsCat2Head()) {
		f.invalidatePairAt(idx)
		if idx-1 >= 0 && idx-1 < len(f.Locals) && f.Locals[idx-1].Kind.IsCat2Head() {
			f.invalidatePairAt(idx - 1)
		}
		return f.storeLocal(idx, T(Int))
	}
	if cur.Kind != Int && cur.Kind != Top {
		return invalidf("iinc of %s", cur.Kind)
	}
	out := T(Int)
	if cur.HasInt {
		out = IntConst(cur.Int + instr.IincConst)
	}
	return f.storeLocal(idx, out)
}

func conv(f *Frame, from, to Kind) error {
	if _, err := f.popKind(from); err != nil {
		return err
	}
	return f.push(T(to))
}

func arrayLoad(f *Frame, elem Kind) error {
	if _, err := f.popKind(Int); err != nil {
		return err
	}
	if _, err := f.popKind(Ref); err != nil {
		return err
	}
	if elem == Ref {
		return f.push(RefOf("java/lang/Object"))
	}
	return f.push(T(elem))
}

func arrayStore(f *Frame, elem Kind) error {
	if _, err := f.popKind(elem); err != nil {
		return err
	}
	if _, err := f.popKind(Int); err != nil {
		return err
	}
	_, err := f.popKind(Ref)
	return err
}

func pushLDC(f *Frame, instr Instr, wide bool) error {
	c := instr.Const
	switch c.Kind {
	case methodir.ConstInt:
		return f.push(IntConst(c.Int))
	case methodir.ConstFloat:
		return f.push(FloatBits(c.FloatBits))
	case methodir.ConstString:
		return f.push(RefOf("java/lang/String"))
	case methodir.ConstClass:
		return f.push(RefOf("java/lang/Class"))
	case methodir.ConstLong:
		if !wide {
			return invalidf("long in ldc")
		}
		return f.push(LongConst(c.Long))
	case methodir.ConstDouble:
		if !wide {
			return invalidf("double in ldc")
		}
		return f.push(DoubleBits(c.DoubleBits))
	}
	if wide {
		return unsupportedf("ldc2_w without constant")
	}
	return f.push(RefOf("java/lang/Object"))
}

func fieldOp(f *Frame, instr Instr, static, put bool) error {
	desc := instr.Desc
	if desc == "" {
		return unsupportedf("field without descriptor")
	}
	t, _, err := parseField(desc)
	if err != nil {
		return err
	}
	if put {
		want := t.Kind
		if want == Ref {
			if _, err := f.popKind(Ref); err != nil {
				return err
			}
		} else {
			if _, err := f.popKind(want); err != nil {
				return err
			}
		}
		if !static {
			if _, err := f.popKind(Ref); err != nil {
				return err
			}
		}
		return nil
	}
	if !static {
		if _, err := f.popKind(Ref); err != nil {
			return err
		}
	}
	return f.push(t)
}

func invoke(f *Frame, instr Instr) error {
	desc := instr.Desc
	if desc == "" {
		desc = "()V"
	}
	args, ret, hasRet, err := ParseDescriptor(desc)
	if err != nil {
		return err
	}
	for i := len(args) - 1; i >= 0; i-- {
		want := args[i].Kind
		if want == Ref {
			if _, err := f.popKind(Ref); err != nil {
				return err
			}
			continue
		}
		if _, err := f.popKind(want); err != nil {
			return err
		}
	}
	isStatic := instr.Op == core.OP_INVOKESTATIC || instr.Op == core.OP_INVOKEDYNAMIC
	var recv Type
	if !isStatic {
		var err error
		recv, err = f.popKind(Ref)
		if err != nil {
			return err
		}
	}
	if instr.Name == "<init>" && instr.Op == core.OP_INVOKESPECIAL {
		cls := instr.Class
		if cls == "" {
			cls = "java/lang/Object"
		}
		f.initialize(recv, RefOf(cls))
	}
	if hasRet {
		return f.push(ret)
	}
	return nil
}

func floatBin(f *Frame, op int) error {
	b, err := f.popKind(Float)
	if err != nil {
		return err
	}
	a, err := f.popKind(Float)
	if err != nil {
		return err
	}
	if a.HasBits && b.HasBits {
		x := math.Float32frombits(uint32(a.Bits))
		y := math.Float32frombits(uint32(b.Bits))
		var z float32
		switch op {
		case core.OP_FADD:
			z = x + y
		case core.OP_FSUB:
			z = x - y
		case core.OP_FMUL:
			z = x * y
		case core.OP_FDIV:
			z = x / y
		case core.OP_FREM:
			z = float32(math.Mod(float64(x), float64(y)))
		}
		return f.push(FloatBits(math.Float32bits(z)))
	}
	return f.push(T(Float))
}

func doubleBin(f *Frame, op int) error {
	b, err := f.popKind(Double)
	if err != nil {
		return err
	}
	a, err := f.popKind(Double)
	if err != nil {
		return err
	}
	if a.HasBits && b.HasBits {
		x := math.Float64frombits(a.Bits)
		y := math.Float64frombits(b.Bits)
		var z float64
		switch op {
		case core.OP_DADD:
			z = x + y
		case core.OP_DSUB:
			z = x - y
		case core.OP_DMUL:
			z = x * y
		case core.OP_DDIV:
			z = x / y
		case core.OP_DREM:
			z = math.Mod(x, y)
		}
		return f.push(DoubleBits(math.Float64bits(z)))
	}
	return f.push(T(Double))
}

func floatUn(f *Frame) error {
	a, err := f.popKind(Float)
	if err != nil {
		return err
	}
	if a.HasBits {
		x := math.Float32frombits(uint32(a.Bits))
		return f.push(FloatBits(math.Float32bits(-x)))
	}
	return f.push(T(Float))
}

func doubleUn(f *Frame) error {
	a, err := f.popKind(Double)
	if err != nil {
		return err
	}
	if a.HasBits {
		x := math.Float64frombits(a.Bits)
		return f.push(DoubleBits(math.Float64bits(-x)))
	}
	return f.push(T(Double))
}
