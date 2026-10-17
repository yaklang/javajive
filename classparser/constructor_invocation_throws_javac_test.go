package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These independent compiler controls also reject tempting blanket unchecked
// rules; they never call the production inference helper to predict an answer.
func TestAdversarialConstructorGenericThrowsJavacApplicabilityControls(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, compiler := range []struct {
		name, javac string
		native      bool
	}{{"native8", os.Getenv("JAVA8_JAVAC"), true}, {"modern-release8", modern, false}} {
		t.Run(compiler.name, func(t *testing.T) {
			if compiler.javac == "" {
				t.Skip("JAVA8_JAVAC required")
			}
			for _, test := range []struct {
				name, source string
				accept       bool
			}{
				{"explicit Throwable cast needs Throwable declaration", `class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value)throws java.io.IOException{super((Throwable)value);}}`, false},
				{"explicit Throwable cast with broad declaration", `class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value)throws Throwable{super((Throwable)value);}}`, true},
				{"same-type nonnull conditional retains IOException", `class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(boolean pick,java.io.IOException left,java.io.IOException right)throws java.io.IOException{super(pick?left:right);}}`, true},
				{"throws_only", "class Base{<E extends Throwable>Base()throws E{}}class Child extends Base{Child(){super();}}\n", true},
				{"constrained_io_requires_io", "class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value){super(value);}}\n", false},
				{"constrained_io_declared", "class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value)throws java.io.IOException{super(value);}}\n", true},
				{"constrained_io_not_file", "class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value)throws java.io.FileNotFoundException{super(value);}}\n", false},
				{"checked_upper_requires_io", "class Base{<E extends java.io.IOException>Base()throws E{}}class Child extends Base{Child(){super();}}\n", false},
				{"intersection_requires_exception", "interface Marker{}class Base{<E extends Exception&Marker>Base()throws E{}}class Child extends Base{Child(){super();}}\n", false},
				{"intersection_declared_exception", "interface Marker{}class Base{<E extends Exception&Marker>Base()throws E{}}class Child extends Base{Child()throws Exception{super();}}\n", true},
				{"intersection_missing_marker", "interface Marker{}class Base{<E extends Exception&Marker>Base(E value)throws E{}}class Child extends Base{Child(java.io.IOException value)throws java.io.IOException{super(value);}}\n", false},
				{"intersection_marked_argument", "interface Marker{}class Marked extends java.io.IOException implements Marker{}class Base{<E extends Exception&Marker>Base(E value)throws E{}}class Child extends Base{Child(Marked value)throws java.io.IOException{super(value);}}\n", true},
				{"class_method_shadow", "class Base<E extends java.io.IOException>{<E extends Throwable>Base()throws E{}}class Child extends Base<java.io.IOException>{Child(){super();}}\n", true},
				{"class_owned_is_not_method_owned", "class Base<E extends Throwable>{Base()throws E{}}class Child extends Base<java.io.IOException>{Child(){super();}}\n", false},
				{"null_has_no_proper_lower_bound", "class Base{<E extends Throwable>Base(E value)throws E{}}class Child extends Base{Child(){super(null);}}\n", true},
				{"numeric_boolean_overload_exclusion", "class Base{<E extends Throwable>Base(boolean x,Throwable value)throws E{}Base(int x,Throwable value)throws java.io.IOException{}}class Child extends Base{Child(boolean x,Throwable value){super(x,value);}}\n", true},
			} {
				t.Run(test.name, func(t *testing.T) {
					dir := t.TempDir()
					file := filepath.Join(dir, "Oracle.java")
					if err := os.WriteFile(file, []byte(test.source), 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"-proc:none", "--release", "8"}
					if compiler.native {
						args = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					args = append(args, "-g:none", "-d", dir, file)
					out, err := exec.Command(compiler.javac, args...).CombinedOutput()
					if (err == nil) != test.accept {
						t.Fatalf("independent javac acceptance=%v want=%v: %v\n%s", err == nil, test.accept, err, out)
					}
				})
			}
		})
	}
}
