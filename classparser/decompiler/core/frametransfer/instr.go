package frametransfer

import "github.com/yaklang/javajive/classparser/decompiler/core/methodir"

type Instr struct {
	Op        int
	PC        uint16
	Wide      bool
	Local     int
	IincConst int32
	Class     string
	Name      string
	Desc      string
	Const     methodir.Const
	AType     uint8
	Dims      int
}

func FromIR(in methodir.Instr) Instr {
	out := Instr{
		Op:        in.Opcode,
		PC:        in.PC,
		Wide:      in.Wide,
		Local:     in.Local,
		IincConst: in.IincConst,
		Class:     in.Class,
		Name:      in.Member,
		Desc:      in.Desc,
		Const:     in.Const,
	}
	if in.Opcode == 0xbc && len(in.Data) > 0 { // newarray
		out.AType = in.Data[0]
	}
	if in.Opcode == 0xc5 && len(in.Data) >= 3 { // multianewarray
		out.Dims = int(in.Data[2])
	}
	if out.Class == "" {
		out.Class = in.Class
	}
	if in.Const.Kind == methodir.ConstClass && out.Class == "" {
		out.Class = in.Const.Class
	}
	return out
}
