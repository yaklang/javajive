package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorAnonymousCapturePublishesLegalSource(t *testing.T) {
	const fixture = `
interface CaptureRead {boolean read();}
public class CapturedAdapter {
 final CaptureRead action;
 CapturedAdapter(final boolean flag){action=new CaptureRead(){public boolean read(){return flag;}};}
 boolean read(){return action.read();}
}
class CaptureDriver {public static void main(String[]args){
 CapturedAdapter a=new CapturedAdapter(false),b=new CapturedAdapter(true);
 if(a.read()||!b.read()||a.action==b.action)throw new AssertionError("captured values/identity");
 if(a.read()||!b.read())throw new AssertionError("retained capture");
 System.out.println("4:constructor-capture:values:identity");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			source, owner := fixture, "CapturedAdapter"
			if spelling == "renamed" {
				source = strings.NewReplacer("CapturedAdapter", "DeferredCheck", "action", "callback", "flag", "selected").Replace(source)
				owner = "DeferredCheck"
			}
			testAuthoredOwnerAcrossSourceModes(t, source, owner, "CaptureDriver", "4:constructor-capture:values:identity\n", owner+"$1")
		})
	}
}
