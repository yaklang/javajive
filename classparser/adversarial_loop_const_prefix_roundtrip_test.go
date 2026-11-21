package javaclassparser

import "testing"

func TestAdversarialLoopConstPrefixPerIterationResetRoundTrip(t *testing.T) {
	const fixture = `class BindingAccess {
 static int condPostIncReset() {
  int i = 0;
  int rounds = 0;
  while (true) {
   int x = 0;
   if (!(x++ < 1 && i++ < 2)) {
    break;
   }
   rounds++;
  }
  return rounds;
 }
 static int bodyIncReset() {
  int i = 0;
  int total = 0;
  while (true) {
   int x = 0;
   if (i >= 3) {
    break;
   }
   x++;
   total += x;
   i++;
  }
  return total;
 }
 static int bodyDecReset() {
  int i = 0;
  int total = 0;
  while (true) {
   int x = 0;
   if (i >= 3) {
    break;
   }
   x--;
   total += x;
   i++;
  }
  return total;
 }
 static int condPreDecReset() {
  int i = 0;
  int rounds = 0;
  while (true) {
   int x = 0;
   if (!(--x == -1 && i++ < 2)) {
    break;
   }
   rounds++;
  }
  return rounds;
 }
 static int readOnlyConstPrefix() {
  int i = 0;
  int total = 0;
  while (true) {
   int limit = 3;
   if (i >= limit) {
    break;
   }
   total += limit;
   i++;
  }
  return total;
 }
}
class BindingDriver {
 public static void main(String[] args) {
  int r1 = BindingAccess.condPostIncReset();
  int r2 = BindingAccess.bodyIncReset();
  int r3 = BindingAccess.bodyDecReset();
  int r4 = BindingAccess.condPreDecReset();
  int r5 = BindingAccess.readOnlyConstPrefix();
  String out = r1 + ":" + r2 + ":" + r3 + ":" + r4 + ":" + r5 + ":loop-const-prefix";
  if (!"2:3:-3:2:9:loop-const-prefix".equals(out)) {
   throw new AssertionError("unexpected original oracle: " + out);
  }
  System.out.println(out);
 }
}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "2:3:-3:2:9:loop-const-prefix\n")
}
