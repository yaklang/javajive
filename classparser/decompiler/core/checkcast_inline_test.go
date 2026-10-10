package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestImmediateCheckcastThrowRequiresPrivateHandlerPreservingEdge(t *testing.T) {
	for _, change := range []string{"valid", "same handler", "other entry", "wrong source", "multiple targets", "nil target", "back edge", "handler boundary", "different handler", "duplicate", "store", "call", "custom check", "custom throw", "catch check", "catch throw", "try boundary", "invalid check"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			throw := &OpCode{Instr: &Instruction{OpCode: OP_ATHROW}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{throw}
			d := &Decompiler{}
			switch change {
			case "same handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 5, HandlerPc: 8}}
			case "other entry":
				throw.Source = append(throw.Source, &OpCode{})
			case "wrong source":
				throw.Source[0] = &OpCode{}
			case "multiple targets":
				check.Target = append(check.Target, throw)
			case "nil target":
				check.Target[0] = nil
			case "back edge":
				throw.CurrentOffset = 0
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "different handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}, {StartPc: 4, EndPc: 5, HandlerPc: 9}}
			case "duplicate":
				throw.Instr.OpCode = OP_DUP
			case "store":
				throw.Instr.OpCode = OP_ASTORE_0
			case "call":
				throw.Instr.OpCode = OP_INVOKESTATIC
			case "custom check":
				check.IsCustom = true
			case "custom throw":
				throw.IsCustom = true
			case "catch check":
				check.IsCatch = true
			case "catch throw":
				throw.IsCatch = true
			case "try boundary":
				throw.IsTryCatchParent = true
			case "invalid check":
				check.Instr.OpCode = OP_ALOAD_0
			}
			if got, want := d.canInlineImmediateCheckcastThrow(check), change == "valid" || change == "same handler"; got != want {
				t.Fatalf("private typed throw edge accepted=%v want=%v", got, want)
			}
		})
	}
}

func TestLinearCheckcastArgumentOrderProof(t *testing.T) {
	for _, kind := range []string{"later cast", "repeated cast", "wide argument", "primitive argument", "receiver", "primitive formal", "field read", "store", "duplicate", "branch", "extra entry", "handler boundary", "back edge", "nil target"} {
		t.Run(kind, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_1}, CurrentOffset: 4}
			cast := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 5}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 8, Data: []byte{0, 1}}
			path := []*OpCode{check, load, cast, invoke}
			for i := 0; i+1 < len(path); i++ {
				path[i].Target = []*OpCode{path[i+1]}
				path[i+1].Source = []*OpCode{path[i]}
			}
			descriptor, accepted := "([I[I)Z", false
			d := &Decompiler{}
			switch kind {
			case "later cast":
				accepted = true
			case "repeated cast":
				load.Instr.OpCode, descriptor, accepted = OP_CHECKCAST, "([I)Z", true
			case "wide argument":
				load.Instr.OpCode, cast.Instr.OpCode, descriptor, accepted = OP_LLOAD_1, OP_NOP, "([IJ)Z", true
			case "primitive argument":
				load.Instr.OpCode, cast.Instr.OpCode, descriptor, accepted = OP_ICONST_1, OP_NOP, "([II)Z", true
			case "receiver":
				invoke.Instr.OpCode, descriptor = OP_INVOKEVIRTUAL, "([I)Z"
			case "primitive formal":
				descriptor = "(I[I)Z"
			case "field read":
				load.Instr.OpCode = OP_GETSTATIC
			case "store":
				load.Instr.OpCode = OP_ASTORE_1
			case "duplicate":
				load.Instr.OpCode = OP_DUP
			case "branch":
				load.Instr.OpCode = OP_IFNULL
			case "extra entry":
				cast.Source = append(cast.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 5, HandlerPc: 20}}
			case "back edge":
				cast.CurrentOffset = 2
			case "nil target":
				load.Target[0] = nil
			}
			typ, err := types.ParseMethodDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			d.constantPoolGetter = func(int) values.JavaValue { return values.NewJavaClassMember("Probe", "consume", descriptor, typ) }
			if got := d.canInlineCheckcastArgument(check); got != accepted {
				t.Fatalf("accepted=%v want=%v", got, accepted)
			}
		})
	}
}

func TestImmediateCheckcastArgumentProof(t *testing.T) {
	for _, change := range []string{"valid", "other source", "handler boundary", "dup", "constructor", "constructor invoke", "constructor return", "no arguments", "primitive", "invalid source"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, Data: []byte{0, 1}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{invoke}
			descriptor, name := "(ILjava/lang/CharSequence;)Ljava/lang/String;", "join"
			d := &Decompiler{}
			switch change {
			case "other source":
				invoke.Source = append(invoke.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "dup":
				invoke.Instr = &Instruction{OpCode: OP_DUP}
			case "constructor":
				name = "<init>"
			case "constructor invoke", "constructor return":
				name = "<init>"
				invoke.Instr.OpCode = OP_INVOKESPECIAL
				if change == "constructor invoke" {
					descriptor = "(ILjava/lang/CharSequence;)V"
				}
			case "no arguments":
				descriptor = "()V"
			case "primitive":
				descriptor = "(I)V"
			case "invalid source":
				check.Instr = &Instruction{OpCode: OP_ALOAD}
			}
			typ, err := types.ParseMethodDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			d.constantPoolGetter = func(int) values.JavaValue { return values.NewJavaClassMember("Probe", name, descriptor, typ) }
			if got := d.canInlineCheckcastArgument(check); got != (change == "valid" || change == "constructor invoke") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestCheckcastArgumentAcrossLaterCalls(t *testing.T) {
	for _, kind := range []string{"instance", "static argument", "static no arguments", "wide return", "wide input", "void call", "constructor call", "checked receiver", "primitive formal", "discarded result", "duplicate", "local write", "extra entry", "handler boundary", "unknown call", "unknown return"} {
		t.Run(kind, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_1}, CurrentOffset: 4}
			later := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 5, Data: []byte{0, 1}}
			cast := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 8}
			consume := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 11, Data: []byte{0, 2}}
			path := []*OpCode{check, load, later, cast, consume}
			for i := 0; i+1 < len(path); i++ {
				path[i].Target, path[i+1].Source = []*OpCode{path[i+1]}, []*OpCode{path[i]}
			}
			nestedDesc, finalDesc, name := "()Ljava/lang/Object;", "(Ljava/lang/Object;Ljava/lang/Object;)Z", "next"
			d := &Decompiler{}
			switch kind {
			case "static argument":
				later.Instr.OpCode, nestedDesc = OP_INVOKESTATIC, "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "static no arguments":
				load.Instr.OpCode, later.Instr.OpCode = OP_NOP, OP_INVOKESTATIC
			case "wide return":
				nestedDesc, finalDesc, cast.Instr.OpCode = "()J", "(Ljava/lang/Object;J)Z", OP_NOP
			case "wide input":
				load.Instr.OpCode, later.Instr.OpCode, nestedDesc = OP_LLOAD_1, OP_INVOKESTATIC, "(J)Ljava/lang/Object;"
			case "void call":
				nestedDesc = "()V"
			case "constructor call":
				later.Instr.OpCode, name = OP_INVOKESPECIAL, "<init>"
			case "checked receiver":
				nestedDesc = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "primitive formal":
				finalDesc = "(ILjava/lang/Object;)Z"
			case "discarded result":
				cast.Instr.OpCode = OP_POP
			case "duplicate":
				cast.Instr.OpCode = OP_DUP
			case "local write":
				cast.Instr.OpCode = OP_ASTORE_1
			case "extra entry":
				later.Source = append(later.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 5, HandlerPc: 20}}
			}
			d.constantPoolGetter = func(index int) values.JavaValue {
				desc, member := nestedDesc, name
				if index == 2 {
					desc, member = finalDesc, "consume"
				}
				if index == 1 && kind == "unknown call" {
					return nil
				}
				typ, err := types.ParseMethodDescriptor(desc)
				if err != nil {
					t.Fatal(err)
				}
				if index == 1 && kind == "unknown return" {
					typ.FunctionType().ReturnType = nil
				}
				return values.NewJavaClassMember("Probe", member, desc, typ)
			}
			want := kind == "instance" || kind == "static argument" || kind == "static no arguments" || kind == "wide return" || kind == "wide input"
			if got := d.canInlineCheckcastArgument(check); got != want {
				t.Fatalf("proof accepted=%t, want=%t", got, want)
			}
		})
	}
}

func TestImmediateCheckcastFieldProof(t *testing.T) {
	for _, change := range []string{"valid", "other source", "handler boundary", "dup", "store", "static field", "different owner", "missing constant", "short operand", "wrong cast"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			read := &OpCode{Instr: &Instruction{OpCode: OP_GETFIELD}, Data: []byte{0, 1}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{read}
			d := &Decompiler{}
			member := values.NewJavaClassMember("example/Record", "value", "I", types.NewJavaPrimer(types.JavaInteger))
			d.constantPoolGetter = func(int) values.JavaValue { return member }
			switch change {
			case "other source":
				read.Source = append(read.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "dup":
				read.Instr.OpCode = OP_DUP
			case "store":
				read.Instr.OpCode = OP_ASTORE
			case "static field":
				read.Instr.OpCode = OP_GETSTATIC
			case "different owner":
				member.Name = "example/Other"
			case "missing constant":
				d.constantPoolGetter = func(int) values.JavaValue { return nil }
			case "short operand":
				read.Data = nil
			case "wrong cast":
				check.Instr.OpCode = OP_ALOAD
			}
			if got := d.canInlineImmediateCheckcastField(check, types.NewJavaClass("example.Record")); got != (change == "valid") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestCheckcastArrayStoreRequiresPrivateOrderedStackPath(t *testing.T) {
	for _, change := range []string{"valid", "repeated cast", "NOP", "invalid check opcode", "missing stack input", "primitive cast", "nil target", "multiple targets", "other entry", "back edge", "handler boundary", "custom consumer", "catch entry", "primitive store", "intervening store", "intervening call", "duplicate", "branch", "path budget"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1, stackConsumed: []values.JavaValue{values.JavaNull}}
			store := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{store}
			d := &Decompiler{}
			typ := types.NewJavaClass("java.lang.String")
			add := func(opcode int, count int) {
				previous := check
				for i := 0; i < count; i++ {
					next := &OpCode{Instr: &Instruction{OpCode: opcode}, CurrentOffset: uint16(i + 2), Source: []*OpCode{previous}}
					previous.Target, previous = []*OpCode{next}, next
				}
				store.CurrentOffset = uint16(count + 3)
				previous.Target, store.Source = []*OpCode{store}, []*OpCode{previous}
			}
			switch change {
			case "repeated cast":
				add(OP_CHECKCAST, 2)
			case "NOP":
				add(OP_NOP, 1)
			case "invalid check opcode":
				check.Instr.OpCode = OP_ALOAD_0
			case "missing stack input":
				check.stackConsumed = nil
			case "primitive cast":
				typ = types.NewJavaPrimer(types.JavaInteger)
			case "nil target":
				check.Target[0] = nil
			case "multiple targets":
				check.Target = append(check.Target, store)
			case "other entry":
				store.Source = append(store.Source, &OpCode{})
			case "back edge":
				store.CurrentOffset = 0
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "custom consumer":
				store.IsCustom = true
			case "catch entry":
				store.IsCatch = true
			case "primitive store":
				store.Instr.OpCode = OP_IASTORE
			case "intervening store":
				add(OP_ASTORE_0, 1)
			case "intervening call":
				add(OP_INVOKESTATIC, 1)
			case "duplicate":
				add(OP_DUP, 1)
			case "branch":
				add(OP_IFNULL, 1)
			case "path budget":
				add(OP_NOP, 8)
			}
			if got := d.canInlineCheckcastArrayStore(check, typ); got != (change == "valid" || change == "repeated cast" || change == "NOP") {
				t.Fatalf("cast/store path accepted=%v", got)
			}
		})
	}
}
