// Package adversarial evaluates JavaJive against adversarial JVM bytecode
// shapes that are not part of the checked-in corpus. It is opt-in:
//
//	JAVAJIVE_ADVERSARIAL_EVAL=1 go test ./test/adversarial -run TestAdversarialEval -v
//
// The suite builds its own classfiles with the reviewed assembler, compiles
// only repository-authored Java sources, and never executes third-party code.
package adversarial

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
	cf "github.com/yaklang/javajive/classparser/classfilebuilder"
)

const evalTimeout = 10 * time.Second

type expectation int

const (
	expectRecovered expectation = iota
	expectFailClosed
)

type record struct {
	name     string
	expect   expectation
	outcome  string
	detail   string
	elapsed  time.Duration
	lines    int
	bytes    int
	states   int
	verified bool
	behavior bool
}

type evalCase struct {
	name   string
	expect expectation
	build  func(*cf.Class) error
}

func envEnabled() bool { return os.Getenv("JAVAJIVE_ADVERSARIAL_EVAL") != "" }

// ---------------------------------------------------------------------------
// Classfile construction helpers (major 49, no StackMapTable)
// ---------------------------------------------------------------------------

func addObjectCtor(class *cf.Class) error {
	initRef, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		return err
	}
	class.AddMethod(cf.ACC_PUBLIC, "<init>", "()V").Code().
		Load(cf.RefLocal, 0).CP(cf.INVOKESPECIAL, initRef).Emit(cf.RETURN)
	return nil
}

func pushInt(class *cf.Class, code *cf.Code, value int) error {
	switch value {
	case -1:
		code.Emit(cf.ICONST_M1)
	case 0:
		code.Emit(cf.ICONST_0)
	case 1:
		code.Emit(cf.ICONST_1)
	case 2:
		code.Emit(cf.ICONST_2)
	case 3:
		code.Emit(cf.ICONST_3)
	case 4:
		code.Emit(cf.ICONST_4)
	case 5:
		code.Emit(cf.ICONST_5)
	default:
		if value >= -128 && value <= 127 {
			code.Byte(cf.BIPUSH, value)
		} else if value >= -32768 && value <= 32767 {
			code.Short(cf.SIPUSH, value)
		} else {
			index, err := class.Pool().Integer(int32(value))
			if err != nil {
				return err
			}
			code.LDC(index)
		}
	}
	return nil
}

func addConst(class *cf.Class, code *cf.Code, local uint16, value int) error {
	code.Load(cf.IntLocal, local)
	if err := pushInt(class, code, value); err != nil {
		return err
	}
	code.Emit(cf.IADD).Store(cf.IntLocal, local)
	return nil
}

func initAccum(class *cf.Class, code *cf.Code) error {
	code.Load(cf.IntLocal, 1).Store(cf.IntLocal, 2)
	if err := pushInt(class, code, 0); err != nil {
		return err
	}
	code.Store(cf.IntLocal, 3)
	return nil
}

func returnAcc(code *cf.Code) {
	code.Load(cf.IntLocal, 3).Emit(cf.IRETURN)
}

func addRun(class *cf.Class, build func(*cf.Code) error) error {
	return build(class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code())
}

// ---------------------------------------------------------------------------
// Supported shapes
// ---------------------------------------------------------------------------

// Eight external switch cases enter one eight-node cycle. Exercises an
// eight-way dispatcher rather than the corpus's two-to-three entries.
func buildSwitchEightEntry(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		labels := make([]*cf.Label, 8)
		for i := range labels {
			labels[i] = code.NewLabel()
		}
		exit := code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).TableSwitch(0, exit, labels...)
		for i, label := range labels {
			code.Mark(label)
			if err := addConst(class, code, 3, i+1); err != nil {
				return err
			}
			code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
			code.Branch(cf.GOTO, labels[(i+1)%len(labels)])
		}
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Eight external entries reached through a conditional chain instead of a
// switch. If this shape recovers while the tableswitch twin does not, the
// defect is in switch-arm emission rather than dispatch lowering.
func buildIfChainEightEntry(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		labels := make([]*cf.Label, 8)
		for i := range labels {
			labels[i] = code.NewLabel()
		}
		exit := code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		for i, label := range labels {
			code.Load(cf.IntLocal, 0)
			if err := pushInt(class, code, i); err != nil {
				return err
			}
			code.Branch(cf.IF_ICMPEQ, label)
		}
		code.Branch(cf.GOTO, exit)
		for i, label := range labels {
			code.Mark(label)
			if err := addConst(class, code, 3, i+1); err != nil {
				return err
			}
			code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
			code.Branch(cf.GOTO, labels[(i+1)%len(labels)])
		}
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Two irreducible kernels in sequence. The first lowering must re-analyze the
// statement CFG before it can find the second region.
func buildTwoSequentialKernels(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, mid, c, d, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, mid).Branch(cf.GOTO, b)
		code.Mark(b)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, mid).Branch(cf.GOTO, a)
		code.Mark(mid)
		// Second kernel budget: budget-2, computed without a second source local.
		code.Load(cf.IntLocal, 2)
		if err := pushInt(class, code, 2); err != nil {
			return err
		}
		code.Emit(cf.IADD).Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 4).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, c).Branch(cf.GOTO, d)
		code.Mark(c)
		if err := addConst(class, code, 3, 4); err != nil {
			return err
		}
		code.IInc(4, -1).Load(cf.IntLocal, 4).Branch(cf.IFLE, exit).Branch(cf.GOTO, d)
		code.Mark(d)
		if err := addConst(class, code, 3, 8); err != nil {
			return err
		}
		code.IInc(4, -1).Load(cf.IntLocal, 4).Branch(cf.IFLE, exit).Branch(cf.GOTO, c)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Regional irreducibility inside two nested natural loops.
func buildDeepRegional(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		outer, innerHeader, innerAdvance, outerBottom, a, b, exit :=
			code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Mark(outer)
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		if err := pushInt(class, code, 2); err != nil {
			return err
		}
		code.Store(cf.IntLocal, 4)
		code.Mark(innerHeader)
		code.Load(cf.IntLocal, 4).Branch(cf.IFLE, outerBottom)
		code.Load(cf.IntLocal, 3).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 4).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, b).Branch(cf.GOTO, innerAdvance)
		code.Mark(b)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 4).Emit(cf.ICONST_2).Emit(cf.IAND).Branch(cf.IFEQ, a).Branch(cf.GOTO, innerAdvance)
		code.Mark(innerAdvance)
		code.IInc(4, -1).Branch(cf.GOTO, innerHeader)
		code.Mark(outerBottom)
		code.IInc(2, -1).Branch(cf.GOTO, outer)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Four distinct exit states, each with a distinct post-loop effect.
func buildFourExitEffects(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, c, d, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		if err := pushInt(class, code, 0); err != nil {
			return err
		}
		code.Store(cf.IntLocal, 5)
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).TableSwitch(0, exit, a, b, c, d)
		code.Mark(a)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFGT, b)
		code.Emit(cf.ICONST_0).Store(cf.IntLocal, 5).Branch(cf.GOTO, exit)
		code.Mark(b)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFGT, c)
		code.Emit(cf.ICONST_1).Store(cf.IntLocal, 5).Branch(cf.GOTO, exit)
		code.Mark(c)
		if err := addConst(class, code, 3, 4); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFGT, d)
		code.Emit(cf.ICONST_2).Store(cf.IntLocal, 5).Branch(cf.GOTO, exit)
		code.Mark(d)
		if err := addConst(class, code, 3, 8); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFGT, a)
		code.Emit(cf.ICONST_3).Store(cf.IntLocal, 5)
		code.Mark(exit)
		// Post-loop exit dispatch: distinct additive effects.
		e0, e1, e2, e3, done := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		code.Load(cf.IntLocal, 5).Branch(cf.IFEQ, e0)
		code.Load(cf.IntLocal, 5).Emit(cf.ICONST_1).Branch(cf.IF_ICMPEQ, e1)
		code.Load(cf.IntLocal, 5).Emit(cf.ICONST_2).Branch(cf.IF_ICMPEQ, e2)
		code.Branch(cf.GOTO, e3)
		code.Mark(e0)
		if err := addConst(class, code, 3, 0); err != nil {
			return err
		}
		code.Branch(cf.GOTO, done)
		code.Mark(e1)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Branch(cf.GOTO, done)
		code.Mark(e2)
		if err := addConst(class, code, 3, 200); err != nil {
			return err
		}
		code.Branch(cf.GOTO, done)
		code.Mark(e3)
		if err := addConst(class, code, 3, 300); err != nil {
			return err
		}
		code.Mark(done)
		returnAcc(code)
		return nil
	})
}

// One external entry is a fallthrough edge into the kernel; the other is a
// taken branch through a separate block.
func buildFallthroughEntry(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, gate, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, gate)
		// fallthrough into the kernel at A
		code.Mark(a)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
		code.Mark(b)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
		code.Mark(gate)
		code.Branch(cf.GOTO, b)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// A discarded static field read is not a legal Java expression statement. The
// sink comparison must preserve the read (and any class initialization) while
// keeping the rebuilt source compilable.
func buildDiscardedStaticRead(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "TOKEN", "Ljava/lang/Object;")
	return addRun(class, func(code *cf.Code) error {
		field, err := class.Pool().Fieldref("DiscardedStaticRead", "TOKEN", "Ljava/lang/Object;")
		if err != nil {
			return err
		}
		code.CP(cf.GETSTATIC, field).Emit(cf.POP)
		code.Load(cf.IntLocal, 1).Emit(cf.IRETURN)
		return nil
	})
}

// ---------------------------------------------------------------------------
// Fail-closed shapes
// ---------------------------------------------------------------------------

func buildConstructorIrreducible(class *cf.Class) error {
	initRef, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		return err
	}
	code := class.AddMethod(cf.ACC_PUBLIC, "<init>", "(II)V").Code()
	a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
	code.Load(cf.RefLocal, 0).CP(cf.INVOKESPECIAL, initRef)
	code.Load(cf.IntLocal, 2).Store(cf.IntLocal, 3)
	code.Load(cf.IntLocal, 3).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 1).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
	code.Mark(a)
	code.IInc(3, -1).Load(cf.IntLocal, 3).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
	code.Mark(b)
	code.IInc(3, -1).Load(cf.IntLocal, 3).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
	code.Mark(exit)
	code.Emit(cf.RETURN)
	return nil
}

func buildTryCatchIrreducible(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit, endTry, handler := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
		code.Mark(b)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
		code.Mark(endTry)
		code.Mark(exit)
		returnAcc(code)
		code.Mark(handler)
		code.Emit(cf.POP).Emit(cf.ICONST_M1).Emit(cf.IRETURN)
		code.Catch(a, endTry, handler, "java/lang/Exception")
		return nil
	})
}

func buildMonitorIrreducible(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Emit(cf.ACONST_NULL).Emit(cf.MONITORENTER)
		code.Emit(cf.ACONST_NULL).Emit(cf.MONITOREXIT)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Emit(cf.ACONST_NULL).Emit(cf.MONITORENTER)
		code.Emit(cf.ACONST_NULL).Emit(cf.MONITOREXIT)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

func buildNewIrreducible(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		objectRef, err := class.Pool().Class("java/lang/Object")
		if err != nil {
			return err
		}
		initRef, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
		if err != nil {
			return err
		}
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.CP(cf.NEW, objectRef).Emit(cf.DUP).CP(cf.INVOKESPECIAL, initRef).Emit(cf.POP)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
		code.Mark(b)
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Every edge into the kernel carries one int on the operand stack.
func buildStackCarryEntry(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, pushA, pushB, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, pushA)
		code.Load(cf.IntLocal, 0).Emit(cf.ICONST_1).Branch(cf.IF_ICMPEQ, pushB).Branch(cf.GOTO, exit)
		code.Mark(pushA)
		code.Emit(cf.ICONST_1).Branch(cf.GOTO, a)
		code.Mark(pushB)
		code.Emit(cf.ICONST_1).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.IntLocal, 4)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.ICONST_1).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.IntLocal, 4)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.ICONST_1).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Every edge into the kernel carries an immediate static-field read. The read
// is effectful but its producer is adjacent to the control boundary, so the
// dispatcher may evaluate it in the target case without crossing another
// observable operation.
func buildStackCarryField(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "CARRY", "I")
	field, err := class.Pool().Fieldref("StackCarryField", "CARRY", "I")
	if err != nil {
		return err
	}
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Every boundary carries the result of a helper invocation. The call is the
// last evaluation before the branch; delaying it into the target case is
// observationally equivalent because no other user instruction intervenes.
func buildStackCarryCall(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "COUNTER", "I")
	counter, err := class.Pool().Fieldref("StackCarryCall", "COUNTER", "I")
	if err != nil {
		return err
	}
	bump, err := class.Pool().Methodref("StackCarryCall", "bump", "()I")
	if err != nil {
		return err
	}
	bumpCode := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "bump", "()I").Code()
	bumpCode.CP(cf.GETSTATIC, counter).Emit(cf.ICONST_1).Emit(cf.IADD).CP(cf.PUTSTATIC, counter)
	bumpCode.CP(cf.GETSTATIC, counter).Emit(cf.IRETURN)
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.INVOKESTATIC, bump)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.INVOKESTATIC, bump).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.INVOKESTATIC, bump).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// The entry field read is carried across an invocation which mutates the same
// field. Re-evaluating the read in the dispatcher target would observe the new
// value, so this shape must fail closed instead of silently changing behavior.
func buildStackCarryReordered(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "CARRY", "I")
	field, err := class.Pool().Fieldref("StackCarryReordered", "CARRY", "I")
	if err != nil {
		return err
	}
	touch, err := class.Pool().Methodref("StackCarryReordered", "touch", "()V")
	if err != nil {
		return err
	}
	touchCode := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "touch", "()V").Code()
	touchCode.CP(cf.GETSTATIC, field).Emit(cf.ICONST_1).Emit(cf.IADD).CP(cf.PUTSTATIC, field).Emit(cf.RETURN)
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).CP(cf.INVOKESTATIC, touch)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Reference words use the same carrier proof. The static field is null for the
// whole run, so both branches exercise a real null check on the carried word.
func buildStackCarryReference(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "OBJ", "Ljava/lang/Object;")
	field, err := class.Pool().Fieldref("StackCarryReference", "OBJ", "Ljava/lang/Object;")
	if err != nil {
		return err
	}
	return addRun(class, func(code *cf.Code) error {
		a, b, skipA, skipB, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.RefLocal, 4)
		code.Load(cf.RefLocal, 4).Branch(cf.IFNULL, skipA)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipA)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.RefLocal, 4)
		code.Load(cf.RefLocal, 4).Branch(cf.IFNULL, skipB)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipB)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Every edge carries the same local identity (the initialized null local).
// The merge certificate may reuse one JavaRef because all incoming paths name
// the same variable.
func buildStackCarryLocalRef(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, skipA, skipB, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Emit(cf.ACONST_NULL).Store(cf.RefLocal, 4)
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.RefLocal, 5)
		code.Load(cf.RefLocal, 5).Branch(cf.IFNULL, skipA)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipA)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.RefLocal, 5)
		code.Load(cf.RefLocal, 5).Branch(cf.IFNULL, skipB)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipB)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Equal category-2 constants on every boundary edge need no stack phi.
func buildStackCarryLongSame(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_1)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.LongLocal, 4)
		code.Load(cf.LongLocal, 4).Emit(cf.L2I)
		code.Load(cf.IntLocal, 3).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_1).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.LongLocal, 4)
		code.Load(cf.LongLocal, 4).Emit(cf.L2I)
		code.Load(cf.IntLocal, 3).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_1).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// The entry read of OBJ is carried across setObj(), which replaces the field.
// Delaying the read would make the null test see a non-null object.
func buildStackCarryReferenceReordered(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "OBJ", "Ljava/lang/Object;")
	field, err := class.Pool().Fieldref("StackCarryReferenceReordered", "OBJ", "Ljava/lang/Object;")
	if err != nil {
		return err
	}
	objectRef, err := class.Pool().Class("java/lang/Object")
	if err != nil {
		return err
	}
	initRef, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		return err
	}
	setObj, err := class.Pool().Methodref("StackCarryReferenceReordered", "setObj", "()V")
	if err != nil {
		return err
	}
	setCode := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "setObj", "()V").Code()
	setCode.CP(cf.NEW, objectRef).Emit(cf.DUP).CP(cf.INVOKESPECIAL, initRef).CP(cf.PUTSTATIC, field).Emit(cf.RETURN)
	return addRun(class, func(code *cf.Code) error {
		a, b, skipA, skipB, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).CP(cf.INVOKESTATIC, setObj)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.RefLocal, 4)
		code.Load(cf.RefLocal, 4).Branch(cf.IFNULL, skipA)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipA)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.RefLocal, 4)
		code.Load(cf.RefLocal, 4).Branch(cf.IFNULL, skipB)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipB)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.GETSTATIC, field).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// The entry path falls through into the loop header with a call result already
// on the stack. The producer is the boundary source itself, so the lowering must
// materialize the result instead of invoking the helper again in the target
// case.
func buildStackCarryCallFallthrough(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "COUNTER", "I")
	counter, err := class.Pool().Fieldref("StackCarryCallFallthrough", "COUNTER", "I")
	if err != nil {
		return err
	}
	bump, err := class.Pool().Methodref("StackCarryCallFallthrough", "bump", "()I")
	if err != nil {
		return err
	}
	bumpCode := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "bump", "()I").Code()
	bumpCode.CP(cf.GETSTATIC, counter).Emit(cf.ICONST_1).Emit(cf.IADD).CP(cf.PUTSTATIC, counter)
	bumpCode.CP(cf.GETSTATIC, counter).Emit(cf.IRETURN)
	return addRun(class, func(code *cf.Code) error {
		h, m, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, m)
		code.CP(cf.INVOKESTATIC, bump)
		// fallthrough into H with one int on the stack
		code.Mark(h)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, m)
		code.Mark(m)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.CP(cf.INVOKESTATIC, bump).Branch(cf.GOTO, h)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// Different constants arrive at the same entry target from the external edge
// and the backedge. A correct lowering must preserve a per-path value instead
// of folding the first source into every case.
func buildStackCarryDifferingInt(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.ICONST_0)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.ICONST_1).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.IntLocal, 4)
		code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.ICONST_2).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// A category-2 word crosses the same boundaries. The carried long must remain
// one value in the dispatcher, not be split into two independent int slots.
func buildStackCarryLong(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_0)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.LongLocal, 4)
		code.Load(cf.LongLocal, 4).Emit(cf.L2I)
		code.Load(cf.IntLocal, 3).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_1).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.LongLocal, 4)
		code.Load(cf.LongLocal, 4).Emit(cf.L2I)
		code.Load(cf.IntLocal, 3).Emit(cf.IADD).Store(cf.IntLocal, 3)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Emit(cf.LCONST_1).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// The entry loads a non-null local, overwrites the same slot with null, then
// carries the old reference across the boundary. Re-reading the local by name
// in the target case would observe null and change the branch.
func buildStackCarryStaleRef(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		a, b, skipA, skipB, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
		text, err := class.Pool().String("x")
		if err != nil {
			return err
		}
		if err := initAccum(class, code); err != nil {
			return err
		}
		code.LDC(text).Store(cf.RefLocal, 4)
		code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4)
		code.Emit(cf.ACONST_NULL).Store(cf.RefLocal, 4)
		code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
		code.Mark(a)
		code.Store(cf.RefLocal, 5)
		code.Load(cf.RefLocal, 5).Branch(cf.IFNULL, skipA)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipA)
		if err := addConst(class, code, 3, 1); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4).Branch(cf.GOTO, b)
		code.Mark(b)
		code.Store(cf.RefLocal, 5)
		code.Load(cf.RefLocal, 5).Branch(cf.IFNULL, skipB)
		if err := addConst(class, code, 3, 100); err != nil {
			return err
		}
		code.Mark(skipB)
		if err := addConst(class, code, 3, 2); err != nil {
			return err
		}
		code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
		code.Load(cf.RefLocal, 4).Branch(cf.GOTO, a)
		code.Mark(exit)
		returnAcc(code)
		return nil
	})
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

func cases() []evalCase {
	return []evalCase{
		{"SwitchEightEntry", expectRecovered, buildSwitchEightEntry},
		{"IfChainEightEntry", expectRecovered, buildIfChainEightEntry},
		{"TwoSequentialKernels", expectRecovered, buildTwoSequentialKernels},
		{"DeepRegional", expectRecovered, buildDeepRegional},
		{"FourExitEffects", expectRecovered, buildFourExitEffects},
		{"FallthroughEntry", expectRecovered, buildFallthroughEntry},
		{"DiscardedStaticRead", expectRecovered, buildDiscardedStaticRead},
		{"ConstructorIrreducible", expectFailClosed, buildConstructorIrreducible},
		{"TryCatchIrreducible", expectFailClosed, buildTryCatchIrreducible},
		{"MonitorIrreducible", expectFailClosed, buildMonitorIrreducible},
		{"NewIrreducible", expectFailClosed, buildNewIrreducible},
		{"StackCarryEntry", expectRecovered, buildStackCarryEntry},
		{"StackCarryField", expectRecovered, buildStackCarryField},
		{"StackCarryCall", expectRecovered, buildStackCarryCall},
		{"StackCarryCallFallthrough", expectRecovered, buildStackCarryCallFallthrough},
		{"StackCarryReference", expectRecovered, buildStackCarryReference},
		{"StackCarryLocalRef", expectRecovered, buildStackCarryLocalRef},
		{"StackCarryStaleRef", expectFailClosed, buildStackCarryStaleRef},
		{"StackCarryLongSame", expectRecovered, buildStackCarryLongSame},
		{"StackCarryDifferingInt", expectFailClosed, buildStackCarryDifferingInt},
		{"StackCarryLong", expectFailClosed, buildStackCarryLong},
		{"StackCarryReordered", expectFailClosed, buildStackCarryReordered},
		{"StackCarryReferenceReordered", expectFailClosed, buildStackCarryReferenceReordered},
	}
}

func decompile(raw []byte) (classparser.DecompileResult, error, any, time.Duration) {
	return decompileWith(raw, evalTimeout)
}

func decompileWith(raw []byte, timeout time.Duration) (classparser.DecompileResult, error, any, time.Duration) {
	start := time.Now()
	var result classparser.DecompileResult
	var err error
	var panicked any
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = r
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result, err = classparser.DecompileWithOptions(raw, classparser.DecompileOptions{
			Mode:                classparser.Precision,
			TargetSourceVersion: 8,
			EnvSnapshot:         map[string]string{},
			Context:             ctx,
			Limits: classparser.Limits{
				MaxInputBytes: 1 << 20, MaxReadBytes: 1 << 20, MaxParseItems: 1_000_000,
				MaxGraphScans: 1_000_000, MaxGraphEdges: 1_000_000, MaxSetElementWork: 10_000_000,
				MaxRequestWork: 100_000_000, MaxOutputBytes: 1 << 20, MaxASTDepth: 10_000,
				MaxDiagnostics: 64, MaxNodeCopies: 1_000_000,
			},
		})
	}()
	return result, err, panicked, time.Since(start)
}

func writeSource(t *testing.T, dir, class, source string) string {
	t.Helper()
	path := filepath.Join(dir, class+".java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func compileJavaFile(t *testing.T, javac, dir, path string) error {
	t.Helper()
	cmd := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-d", dir, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("javac: %v\n%s", err, out)
	}
	return nil
}

const matrixDriver = `import java.lang.reflect.Method;
public final class AdvMatrixDriver {
  public static void main(String[] args) throws Exception {
    Class<?> owner = Class.forName(args[0]);
    Method run = owner.getMethod("run", int.class, int.class);
    for (int entry = -2; entry <= 6; entry++) {
      for (int budget = -1; budget <= 10; budget++) {
        Object value = run.invoke(null, entry, budget);
        System.out.println(entry + "," + budget + "=" + value);
      }
    }
  }
}`

const loadDriver = `public final class AdvLoadDriver {
  public static void main(String[] args) throws Exception {
    Class.forName(args[0], true, AdvLoadDriver.class.getClassLoader());
  }
}`

func buildDriver(t *testing.T, javac, dir string) {
	t.Helper()
	for name, source := range map[string]string{"AdvMatrixDriver": matrixDriver, "AdvLoadDriver": loadDriver} {
		path := filepath.Join(dir, name+".java")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(javac, "-proc:none", "-d", dir, path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("compile driver %s: %v\n%s", name, err, out)
		}
	}
}

func loadClass(t *testing.T, java, driverRoot, targetRoot, fqcn string) error {
	t.Helper()
	cmd := exec.Command(java, "-Xverify:all", "-cp", driverRoot+string(os.PathListSeparator)+targetRoot, "AdvLoadDriver", fqcn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("java -Xverify:all: %v\n%s", err, out)
	}
	return nil
}

func runMatrix(t *testing.T, java, driverRoot, targetRoot, fqcn string) (string, error) {
	t.Helper()
	cmd := exec.Command(java, "-Xverify:all", "-cp", driverRoot+string(os.PathListSeparator)+targetRoot, "AdvMatrixDriver", fqcn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("java: %v\n%s", err, out)
	}
	return string(out), nil
}

func buildClass(t *testing.T, name string, build func(*cf.Class) error) []byte {
	t.Helper()
	class := cf.NewClass(name, "java/lang/Object", cf.ACC_PUBLIC|cf.ACC_SUPER)
	if err := addObjectCtor(class); err != nil {
		t.Fatalf("%s constructor: %v", name, err)
	}
	if err := build(class); err != nil {
		t.Fatalf("%s build: %v", name, err)
	}
	raw, err := class.Bytes()
	if err != nil {
		t.Fatalf("%s assemble: %v", name, err)
	}
	return raw
}

// countDispatchTransfers counts state-dispatch control transfers. The
// synthetic variable names are cleaned up before source emission, so looking
// for jdecState/jdecExit would undercount; LOOP labels survive cleanup.
func countDispatchTransfers(source string) int {
	return strings.Count(source, "continue LOOP_") + strings.Count(source, "break LOOP_")
}

func diagnosticText(result classparser.DecompileResult) string {
	parts := make([]string, 0, len(result.Diagnostics))
	for _, d := range result.Diagnostics {
		parts = append(parts, d.Code+": "+d.Message)
	}
	return strings.Join(parts, " | ")
}

func TestAdversarialEval(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	root := t.TempDir()
	driverRoot := filepath.Join(root, "driver")
	if err := os.MkdirAll(driverRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	buildDriver(t, javac, driverRoot)

	var records []record
	for _, eval := range cases() {
		eval := eval
		t.Run(eval.name, func(t *testing.T) {
			rec := record{name: eval.name, expect: eval.expect}
			raw := buildClass(t, eval.name, eval.build)

			originalRoot := filepath.Join(root, eval.name+"-original")
			if err := os.MkdirAll(originalRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(originalRoot, eval.name+".class"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			// Every adversarial class must be a verifier-valid JVM artifact;
			// otherwise the measurement says nothing about the decompiler.
			if err := loadClass(t, java, driverRoot, originalRoot, eval.name); err != nil {
				t.Fatalf("original class does not verify: %v", err)
			}
			var original string
			if eval.expect == expectRecovered {
				var runErr error
				original, runErr = runMatrix(t, java, driverRoot, originalRoot, eval.name)
				if runErr != nil {
					t.Fatalf("original class does not run: %v", runErr)
				}
			}

			result, err, panicked, elapsed := decompile(raw)
			rec.elapsed = elapsed
			rec.lines = strings.Count(result.Source, "\n") + 1
			rec.bytes = len(result.Source)
			rec.states = countDispatchTransfers(result.Source)
			switch {
			case panicked != nil:
				rec.outcome = "panic"
				rec.detail = fmt.Sprint(panicked)
				t.Errorf("decompiler panicked: %v", panicked)
			case eval.expect == expectFailClosed:
				diags := diagnosticText(result)
				stubbed := result.Status != "complete" || len(result.StubMethods) > 0
				switch {
				case !stubbed:
					rec.outcome = "silent-success"
					rec.detail = fmt.Sprintf("status=%s stubs=%v diagnostics=%s", result.Status, result.StubMethods, diags)
					t.Errorf("shape expected to fail closed decompiled silently")
				case err != nil && strings.Contains(err.Error(), "unsupported_irreducible_control_flow"):
					rec.outcome = "fail-closed"
					rec.detail = err.Error()
				case err != nil:
					rec.outcome = "other-error"
					rec.detail = err.Error()
					t.Errorf("shape failed with an undocumented error: %v", err)
				case strings.Contains(diags, "unsupported_irreducible_control_flow"):
					rec.outcome = "fail-closed-stub"
					rec.detail = diags
				default:
					rec.outcome = "unlabeled-stub"
					rec.detail = fmt.Sprintf("status=%s stubs=%v diagnostics=%s", result.Status, result.StubMethods, diags)
					t.Errorf("shape degraded to a stub without an irreducible diagnostic")
				}
			default:
				if err != nil {
					rec.outcome = "error"
					rec.detail = err.Error()
					t.Errorf("supported shape failed to decompile: %v", err)
					break
				}
				if result.Status != "complete" || len(result.StubMethods) != 0 {
					rec.outcome = "incomplete"
					rec.detail = fmt.Sprintf("status=%s stubs=%v diagnostics=%s", result.Status, result.StubMethods, diagnosticText(result))
					t.Errorf("decompile incomplete: %s\nsource:\n%s", rec.detail, result.Source)
					break
				}
				rebuiltRoot := filepath.Join(root, eval.name+"-rebuilt")
				if err := os.MkdirAll(rebuiltRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				sourcePath := writeSource(t, rebuiltRoot, eval.name, result.Source)
				if err := compileJavaFile(t, javac, rebuiltRoot, sourcePath); err != nil {
					rec.outcome = "compile-failed"
					rec.detail = err.Error()
					t.Errorf("rebuilt source does not compile: %v\nsource:\n%s", err, result.Source)
					break
				}
				rebuilt, runErr := runMatrix(t, java, driverRoot, rebuiltRoot, eval.name)
				if runErr != nil {
					rec.outcome = "verify-failed"
					rec.detail = runErr.Error()
					t.Errorf("rebuilt class does not verify/run: %v", runErr)
					break
				}
				rec.verified = true
				if rebuilt != original {
					rec.outcome = "behavior-mismatch"
					rec.detail = "rebuilt matrix differs from original"
					t.Errorf("behavior mismatch\noriginal:\n%s\nrebuilt:\n%s\nsource:\n%s", original, rebuilt, result.Source)
					break
				}
				rec.behavior = true
				rec.outcome = "recovered"
			}
			records = append(records, rec)
			t.Logf("%-26s outcome=%-16s states=%d lines=%d bytes=%d elapsed=%s",
				rec.name, rec.outcome, rec.states, rec.lines, rec.bytes, rec.elapsed.Round(time.Millisecond))
		})
	}

	t.Log("\n=== ADVERSARIAL SUMMARY (irreducible boundary matrix) ===")
	t.Log("| case | expect | outcome | states | lines | bytes | ms |")
	t.Log("|---|---|---|---:|---:|---:|---:|")
	for _, rec := range records {
		expect := "recover"
		if rec.expect == expectFailClosed {
			expect = "fail-closed"
		}
		t.Logf("| %s | %s | %s | %d | %d | %d | %d |", rec.name, expect, rec.outcome, rec.states, rec.lines, rec.bytes, rec.elapsed.Milliseconds())
	}
}
