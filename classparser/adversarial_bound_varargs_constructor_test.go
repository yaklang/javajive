package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialBoundVarargsConstructorReferenceKeepsArrayAndCompilerRoles(t *testing.T) {
	source := strings.NewReplacer("void accept(Object value){", "Object[] accept(Object... values){if(values.length!=1)throw new AssertionError(\"varargs shape\");Object value=values[0];", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;}", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;return values;}").Replace(constructorBoundReferenceFamilyFixture)
	testConstructorBoundReferenceFamily(t, source)
}
