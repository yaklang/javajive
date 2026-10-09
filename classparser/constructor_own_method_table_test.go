package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func ownMethodTableDomain(t *testing.T, pairs int) (*ClassObjectDumper, *ClassObject) {
	t.Helper()
	var methods strings.Builder
	for i := 0; i < pairs; i++ {
		fmt.Fprintf(&methods, "private int spare%d(int v){return v;}private long spare%d(long v){return v;}", i, i)
	}
	fixture := strings.Replace(closedMethodEvidenceFixture, "class ClosedMethodEvidence {", "class ClosedMethodEvidence {"+methods.String(), 1)
	files := nativeCompileClasses(t, fixture+"class ClosedBodyMemoChild extends ClosedMethodEvidence{}")
	return closedBodyMemoDomain(t, files)
}

func TestAdversarialConstructorOwnMethodTableBindsEveryCompleteDescriptor(t *testing.T) {
	for _, pairs := range []int{16, 32, 48} {
		t.Run(fmt.Sprint(pairs), func(t *testing.T) {
			c, object := ownMethodTableDomain(t, pairs)
			remaining := 512
			if _, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining); !known {
				t.Fatal("original method table")
			}
			if 512-remaining != len(object.Methods) || len(c.constructorProfileEvidence.ownMethods[object]) != len(object.Methods) {
				t.Fatal("first lookup did not charge and index the complete original table")
			}
			seen := map[*MemberInfo]bool{}
			for i := 0; i < pairs; i++ {
				for _, descriptor := range []string{"(I)I", "(J)J"} {
					name := fmt.Sprintf("spare%d", i)
					remaining = 1
					member, known := c.constructorReceiverOwnMethod(object, name, descriptor, &remaining)
					if !known || member == nil || remaining != 0 || seen[member] {
						t.Fatalf("distinct complete descriptor lost: %s%s", name, descriptor)
					}
					actualName, nameErr := object.getUtf8(member.NameIndex)
					actualDescriptor, descriptorErr := object.getUtf8(member.DescriptorIndex)
					if nameErr != nil || descriptorErr != nil || actualName != name || actualDescriptor != descriptor || member.AccessFlags&2 == 0 {
						t.Fatal("original declaration identity differs from source-defined signature")
					}
					seen[member] = true
				}
			}
			for _, descriptor := range []string{"(I)J", "(J)I", "(Ljava/lang/Object;)I", "()I"} {
				remaining = 1
				if member, known := c.constructorReceiverOwnMethod(object, "spare0", descriptor, &remaining); !known || member != nil {
					t.Fatal("absent exact signature acquired a same-name declaration")
				}
			}
		})
	}
}

func TestAdversarialConstructorOwnMethodTableNeverCachesCallerOrBorrowedTrees(t *testing.T) {
	for _, variant := range []string{"caller", "unregistered", "outside body", "ineligible", "inconsistent"} {
		t.Run(variant, func(t *testing.T) {
			c, object := ownMethodTableDomain(t, 1)
			e := c.constructorProfileEvidence
			switch variant {
			case "caller":
				c.obj = object
			case "unregistered":
				delete(e.parsedOriginals, object)
			case "outside body":
				e.inBody = false
			case "ineligible":
				e.eligible = false
			case "inconsistent":
				e.inconsistent = true
			}
			for i := 0; i < 2; i++ {
				remaining := 512
				member, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining)
				if !known || member == nil || 512-remaining != len(object.Methods) || len(e.ownMethods) != 0 {
					t.Fatal("borrowed tree was answered by an immutable-original index")
				}
				if i == 0 {
					duplicate := *member
					object.Methods = append(object.Methods, &duplicate)
					remaining = 512
					if _, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining); known {
						t.Fatal("fresh caller-table ambiguity was hidden")
					}
					object.Methods = object.Methods[:len(object.Methods)-1]
				}
			}
		})
	}
}

func TestAdversarialConstructorOwnMethodTableRejectsMalformedOrAmbiguousOriginals(t *testing.T) {
	for _, variant := range []string{"nil member", "bad name", "bad descriptor", "duplicate signature", "wide descriptor"} {
		t.Run(variant, func(t *testing.T) {
			c, object := ownMethodTableDomain(t, 1)
			e := c.constructorProfileEvidence
			before := e.bodyRetention
			member := object.Methods[len(object.Methods)-1]
			switch variant {
			case "nil member":
				object.Methods = append(object.Methods, nil)
			case "bad name":
				member.NameIndex = 0
			case "bad descriptor":
				member.DescriptorIndex = sourceBridgePoolString(t, object, "(I")
			case "duplicate signature":
				duplicate := *member
				object.Methods = append(object.Methods, &duplicate)
			case "wide descriptor":
				member.AccessFlags = 2
				member.DescriptorIndex = sourceBridgePoolString(t, object, "("+strings.Repeat("J", 128)+")V")
			}
			remaining := 512
			if _, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining); known || len(e.ownMethods) != 0 || e.bodyRetention != before {
				t.Fatal("partial or malformed original table was published")
			}
		})
	}
}

func TestAdversarialConstructorOwnMethodTableRetainsCumulativeResourceBounds(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, variant := range []string{"cancel", "work", "memory", "parsed overflow", "body overflow", "remaining"} {
			t.Run(fmt.Sprintf("%v/%s", cached, variant), func(t *testing.T) {
				c, object := ownMethodTableDomain(t, 1)
				e := c.constructorProfileEvidence
				remaining := 512
				if cached {
					if _, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining); !known {
						t.Fatal("first original table")
					}
				}
				switch variant {
				case "cancel":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					c.Work = workbudget.New(ctx, workbudget.Limits{})
				case "work":
					c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					_ = c.Work.Charge(workbudget.CounterGraphScans, 1)
				case "memory":
					c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "parsed overflow":
					e.parsedRetention = int64(^uint64(0) >> 1)
				case "body overflow":
					e.bodyRetention = int64(^uint64(0) >> 1)
				}
				remaining = 512
				if variant == "remaining" {
					remaining = 0
				}
				if _, known := c.constructorReceiverOwnMethod(object, "spare0", "(I)I", &remaining); known {
					t.Fatal("index bypassed the request's resource refusal")
				}
			})
		}
	}
}
