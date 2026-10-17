package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialUnicodeFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\u53d8\\u91cf")
}
func TestAdversarialGreekFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\u03B1")
}
func TestAdversarialSupplementaryFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\uD801\\uDC00")
}
func TestAdversarialCombiningFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "e\\u0301")
}
func TestAdversarialCurrencyFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\u20AC")
}
func TestAdversarialConnectorFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\u203F")
}
func TestAdversarialLetterNumberFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "\\u216B")
}
func TestAdversarialUnicodeDigitFieldSelectionPreservesOriginalIdentifier(t *testing.T) {
	unicodeFieldOwnerRoundTrip(t, "x\\u0661")
}
func unicodeFieldOwnerRoundTrip(t *testing.T, spelling string) {
	t.Helper()
	fixture := `class BindingBase{public int IDENTIFIER;}class BindingChild extends BindingBase{public int IDENTIFIER;}
class BindingAccess{static int read(BindingChild c){return ((BindingBase)c).IDENTIFIER;}static void write(BindingChild c,int n){((BindingBase)c).IDENTIFIER=n;}}
class BindingDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){BindingChild c=new BindingChild();((BindingBase)c).IDENTIFIER=n;c.IDENTIFIER=43;if(BindingAccess.read(c)!=n)throw new AssertionError("unicode read owner");BindingAccess.write(c,n+1);if(((BindingBase)c).IDENTIFIER!=n+1||c.IDENTIFIER!=43)throw new AssertionError("unicode store owner");rows++;}System.out.println(rows+":unicode-field:owner:identity");}}
`

	fixture = strings.ReplaceAll(fixture, "IDENTIFIER", spelling)
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "5:unicode-field:owner:identity\n")
}
