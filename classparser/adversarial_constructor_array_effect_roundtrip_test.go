package javaclassparser

import (
	"strings"
	"testing"
)

// Every original primitive array category and reference-array access appears
// in the retained parent. The driver computes the checksum with independent
// scalar arithmetic; capture identity and all abrupt exits are also checked.
func TestAdversarialConstructorIndependentArrayAndDivisionCaptureRoundTrip(t *testing.T) {
	// Keep separate subjects within the unchanged shared 512-unit proof budget
	// across all catalogued runtime profiles; a larger method must still refuse.
	for _, tc := range []struct{ name, body, scalar string }{
		{"Narrow", `int[] a={n,3};a[0]+=a[1];byte[] b={(byte)n};short[] s={(short)n};char[] c={(char)n};checksum=(long)a[0]+b[0]+s[0]+c[0];`, `(long)(n+3)+(byte)n+(short)n+(char)n`},
		{"Wide", `long[] l={n,9};float[] f={(float)n/4f};double[] d={(double)n/8d};checksum=l[1]+(long)f[0]+(long)d[0];`, `9L+(long)((float)n/4f)+(long)((double)n/8d)`},
		{"Reference", `Object[] r=new String[2];r[0]="alpha";r[1]=r[0];checksum=r.length+((String)r[1]).length()+10/(n|1)+9L%((long)n|1L);`, `7L+10/(n|1)+9L%((long)n|1L)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := strings.Replace(constructorUnknownAbruptCaptureFixture, "final int n;", "final int n;final long checksum;", 1)
			f = strings.Replace(f, "if(n==-1)throw marker;", "", 1)
			f = strings.Replace(f, "if(n==-2){last=new java.io.IOException(\"fresh\");throw last;}", "", 1)
			f = strings.Replace(f, "if(n==-3)throw null;", "", 1)
			f = strings.Replace(f, "trace=trace*31+1;", "", 1)
			f = strings.Replace(f, "this.n=n;trace=trace*31+2;", tc.body+"this.n=n;trace=trace*31+2;", 1)
			f = strings.Replace(f, "c.value!=value||AbruptParent.trace!=33", "c.value!=value||c.checksum!="+tc.scalar+"||AbruptParent.trace!=33", 1)
			f = strings.Replace(f, "trace=trace*31+2;", "", 1)
			f = strings.Replace(f, "AbruptParent.trace!=33", "AbruptParent.trace!=0", 1)
			f = strings.Replace(f, "new int[]{-3,-2,-1,0,7,70000}", "new int[]{0,7,70000}", 1)
			for _, prefix := range []string{"Arrays", "Delta"} {
				t.Run(prefix, func(t *testing.T) {
					testConstructorUnknownInputAbruptCaptureRoundTrip(t, strings.ReplaceAll(f, "Abrupt", prefix), prefix, "0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n")
				})
			}
		})
	}
}

func TestAdversarialConstructorIndependentArrayFailureCaptureRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, body, label, exception string }{
		{"NullLength", `int[] a=null;checksum=a.length;`, "null", ""},
		{"Bounds", `int[] a=new int[0];checksum=a[n];`, "bounds", "ArrayIndexOutOfBoundsException"},
		{"Component", `Object[] a=new String[1];a[0]=new Object();checksum=0;`, "store", "ArrayStoreException"},
		{"ZeroDivisor", `int zero=n-n;checksum=7/zero;`, "divide", "ArithmeticException"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := strings.Replace(constructorUnknownAbruptCaptureFixture, "final int n;", "final int n;final long checksum;", 1)
			f = strings.Replace(f, "this.n=n;trace=trace*31+2;", tc.body+"this.n=n;trace=trace*31+2;", 1)
			if tc.exception != "" {
				clause := "}catch(" + tc.exception + " e){if(n<0||AbruptParent.trace!=1)throw new AssertionError(\"exception/order\");System.out.println(n+\":" + tc.label + ":\"+(value==x));}catch(NullPointerException e){"
				f = strings.Replace(f, "}catch(NullPointerException e){", clause, 1)
			} else {
				f = strings.Replace(f, "if(n!=-3||AbruptParent.trace!=1)", "if(n<0&&n!=-3||AbruptParent.trace!=1)", 1)
			}
			expected := "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n"
			for _, n := range []string{"0", "7", "70000"} {
				expected += n + ":" + tc.label + ":false\n" + n + ":" + tc.label + ":true\n"
			}
			for _, prefix := range []string{"Arrays", "Delta"} {
				t.Run(prefix, func(t *testing.T) {
					testConstructorUnknownInputAbruptCaptureRoundTrip(t, strings.ReplaceAll(f, "Abrupt", prefix), prefix, expected)
				})
			}
		})
	}
}
