package javaclassparser

import (
	"strings"
	"testing"
)

// This larger original parent formerly exhausted the shared 512-unit budget
// when its identical effect flow was traversed for every platform profile.
// Each platform's Object/finalizer facts still need an independent certificate.
func TestAdversarialConstructorProfileIndependentEffectsReuseRoundTrip(t *testing.T) {
	f := strings.Replace(constructorUnknownAbruptCaptureFixture, "final int n;", "final int n;final long checksum;", 1)
	f = strings.Replace(f, "this.n=n;trace=trace*31+2;", `
 int[] a={n,3};a[0]+=a[1];long[] l={n,9};
 float[] f={(float)n/4f};double[] d={(double)n/8d};
 byte[] b={(byte)n};short[] s={(short)n};char[] c={(char)n};
 Object[] r=new String[2];r[0]="alpha";r[1]=r[0];
 checksum=(long)a[0]+l[1]+(long)f[0]+(long)d[0]+b[0]+s[0]+c[0]+((String)r[1]).length()+10/(n|1);
 this.n=n;trace=trace*31+2;`, 1)
	f = strings.Replace(f, "c.value!=value||AbruptParent.trace!=33", "c.value!=value||c.checksum!=(long)(n+3)+9+(long)((float)n/4f)+(long)((double)n/8d)+(byte)n+(short)n+(char)n+5+10/(n|1)||AbruptParent.trace!=33", 1)
	for _, prefix := range []string{"Reuse", "Epsilon"} {
		t.Run(prefix, func(t *testing.T) {
			testConstructorUnknownInputAbruptCaptureRoundTrip(t, strings.ReplaceAll(f, "Abrupt", prefix), prefix)
		})
	}
}
