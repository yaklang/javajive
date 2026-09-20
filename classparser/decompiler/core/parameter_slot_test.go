package core

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestParameterReferenceSlotReuse(t *testing.T) {
	for _, tc := range []struct {
		name, parameter, stored string
		reuse                   bool
	}{
		{"unrelated classes", "org.gradle.wrapper.WrapperExecutor", "org.gradle.wrapper.PathAssembler", false},
		{"known subtype", "java.lang.CharSequence", "java.lang.String", true},
		{"known superclass is not assignable", "java.lang.String", "java.lang.CharSequence", false},
		{"object accepts references", "java.lang.Object", "org.gradle.wrapper.PathAssembler", true},
		{"same type", "java.io.File", "java.io.File", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JDEC_PARAM_REASSIGN_SPLIT", "")
			param := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(tc.parameter))
			param.IsParam = true
			sim := NewStackSimulation(nil, map[int]*values.JavaRef{0: param}, utils.NewRootVariableId())
			value := values.NewJavaLiteral("value", types.NewJavaClass(tc.stored))
			stored, fresh := sim.AssignVarGuarded(0, value, false)
			require.Equal(t, !tc.reuse, fresh)
			if tc.reuse {
				require.Same(t, param, stored)
			} else {
				require.NotSame(t, param, stored)
				require.Equal(t, tc.parameter, param.Type().RawType().(*types.JavaClass).Name)
			}
		})
	}
}

func TestParameterReferenceCompatibility(t *testing.T) {
	require.True(t, parameterAcceptsReference(types.NewJavaClass("java.lang.CharSequence"), types.NewJavaPrimer(types.JavaString)))
	require.False(t, parameterAcceptsReference(types.NewJavaClass("java.lang.Object"), types.NewJavaPrimer(types.JavaInteger)))
}

func TestNullStoreSplitsPrimitiveParameter(t *testing.T) {
	param := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	param.IsParam = true
	sim := NewStackSimulation(nil, map[int]*values.JavaRef{0: param}, utils.NewRootVariableId())
	_, fresh := sim.AssignVarGuarded(0, values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")), false)
	require.True(t, fresh)
}
