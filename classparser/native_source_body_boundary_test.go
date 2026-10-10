package javaclassparser

import "testing"

func TestNativeAnonymousBodyDistinguishesEmptyFromUnknown(t *testing.T) {
	for _, row := range []struct {
		source, body string
		known        bool
	}{
		{"class Empty {}", "", true}, {"class With {String s=\"{}\";/*}*/}", "String s=\"{}\";/*}*/", true},
		{"class Missing", "", false}, {"class Broken {", "", false}, {"class Extra {} illegal", "", false}, {"class Quoted {String s=\"}\";}", "String s=\"}\";", true},
	} {
		body, known := javaClassBodyContentKnown(row.source)
		if known != row.known || body != row.body {
			t.Errorf("%q => %q,%v", row.source, body, known)
		}
	}
}
