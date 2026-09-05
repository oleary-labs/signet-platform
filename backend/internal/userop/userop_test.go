package userop

import (
	"encoding/hex"
	"strings"
	"testing"
)

const (
	wallet  = "0x1111111111111111111111111111111111111111"
	factory = "0x2222222222222222222222222222222222222222"
	other   = "0x3333333333333333333333333333333333333333"
	// createGroup(address[],uint256,uint256,(string,string[])[],bytes[])
	createGroupSel = "5a0b4a8f"
	// A method the route does not allow.
	transferSel = "a9059cbb"
)

func word(hexBody string) string { return strings.Repeat("0", 64-len(hexBody)) + hexBody }

// buildExecute encodes execute(dest, value, inner) the way the smart account
// expects it, so the guard is exercised against real calldata rather than a
// hand-waved stand-in.
func buildExecute(dest string, valueHex string, innerSelector string, innerTail string) string {
	inner := innerSelector + innerTail
	innerBytes := len(inner) / 2

	var sb strings.Builder
	sb.WriteString(executeSelector)
	sb.WriteString(word(strings.TrimPrefix(strings.ToLower(dest), "0x")))
	sb.WriteString(word(valueHex))
	sb.WriteString(word("60")) // offset to the bytes argument: three head words
	sb.WriteString(word(hexLen(innerBytes)))
	padded := inner
	for len(padded)%64 != 0 {
		padded += "0"
	}
	sb.WriteString(padded)
	return "0x" + sb.String()
}

func hexLen(n int) string {
	return strings.TrimPrefix(hex.EncodeToString([]byte{byte(n >> 8), byte(n)}), "00")
}

func validOp() *Packed {
	return &Packed{
		Sender:    wallet,
		CallData:  buildExecute(factory, "0", createGroupSel, strings.Repeat("11", 32)),
		Signature: "0xdeadbeef",
	}
}

func intent() Intent {
	return Intent{
		Action:    "group creation",
		Dest:      factory,
		Selectors: map[string]string{"createGroup": createGroupSel},
	}
}

func TestValidateAcceptsTheIntendedCall(t *testing.T) {
	got, err := Validate(validOp(), intent(), wallet)
	if err != nil {
		t.Fatalf("a well-formed operation was rejected: %v", err)
	}
	if got.Method != "createGroup" {
		t.Fatalf("method = %q, want createGroup", got.Method)
	}
	if !strings.EqualFold(got.Dest, factory) {
		t.Fatalf("dest = %s, want %s", got.Dest, factory)
	}
}

// Each of these is a way a caller could try to get the paymaster to pay for
// something the route did not authorize.
func TestValidateRefusesEverythingElse(t *testing.T) {
	cases := map[string]func(*Packed){
		"another user's wallet": func(op *Packed) { op.Sender = other },
		"a different contract":  func(op *Packed) { op.CallData = buildExecute(other, "0", createGroupSel, "") },
		"a method the route does not allow": func(op *Packed) {
			op.CallData = buildExecute(factory, "0", transferSel, strings.Repeat("22", 64))
		},
		"a call that moves value": func(op *Packed) {
			op.CallData = buildExecute(factory, "de0b6b3a7640000", createGroupSel, "")
		},
		"a raw call not wrapped in execute": func(op *Packed) {
			op.CallData = "0x" + createGroupSel + strings.Repeat("00", 64)
		},
		"an unsigned operation":    func(op *Packed) { op.Signature = "0x" },
		"truncated calldata":       func(op *Packed) { op.CallData = "0x" + executeSelector },
		"calldata that is not hex": func(op *Packed) { op.CallData = "0xzzzz" },
		"an offset past the end": func(op *Packed) {
			op.CallData = "0x" + executeSelector + word(strings.TrimPrefix(factory, "0x")) + word("0") + word("ffff")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			op := validOp()
			mutate(op)
			if _, err := Validate(op, intent(), wallet); err == nil {
				t.Fatalf("the guard sponsored %s", name)
			}
		})
	}
}

func TestValidateRefusesANilOperation(t *testing.T) {
	if _, err := Validate(nil, intent(), wallet); err == nil {
		t.Fatal("a missing operation was accepted")
	}
}

// A refusal has to say which part was wrong, or a developer cannot fix it and
// an operator cannot read the logs.
func TestRefusalsNameTheProblem(t *testing.T) {
	op := validOp()
	op.Sender = other
	_, err := Validate(op, intent(), wallet)
	if err == nil || !strings.Contains(err.Error(), other) {
		t.Fatalf("the refusal should name the offending sender, got %v", err)
	}

	op = validOp()
	op.CallData = buildExecute(factory, "0", transferSel, "")
	_, err = Validate(op, intent(), wallet)
	if err == nil || !strings.Contains(err.Error(), "createGroup") {
		t.Fatalf("the refusal should name what was allowed, got %v", err)
	}
}

// Sponsorship has to be something the route grants. An operation that arrives
// carrying paymaster data on an unsponsored route is a developer getting the
// platform to pay for a production group, and must be refused rather than
// forwarded.
func TestValidateRefusesUnaskedSponsorship(t *testing.T) {
	unsponsored := intent()
	unsponsored.Action = "creating a production group"
	unsponsored.RefusePaymaster = true

	cases := map[string]func(*Packed){
		"paymaster address": func(op *Packed) {
			op.Paymaster = "0x4444444444444444444444444444444444444444"
		},
		"paymaster data only": func(op *Packed) { op.PaymasterData = "0xabcdef" },
		"both": func(op *Packed) {
			op.Paymaster = "0x4444444444444444444444444444444444444444"
			op.PaymasterData = "0xabcdef"
		},
	}
	for name, attach := range cases {
		t.Run(name, func(t *testing.T) {
			op := validOp()
			attach(op)
			_, err := Validate(op, unsponsored, wallet)
			if err == nil {
				t.Fatal("a sponsored operation was accepted on an unsponsored route")
			}
			if !strings.Contains(err.Error(), "paymaster") {
				t.Fatalf("the refusal does not name the problem: %v", err)
			}
		})
	}
}

// Empty and "0x" are how the console writes "no paymaster", and neither should
// read as sponsorship — otherwise every unsponsored deploy would be refused.
func TestValidateAllowsAbsentPaymasterFields(t *testing.T) {
	unsponsored := intent()
	unsponsored.RefusePaymaster = true

	for _, empty := range []string{"", "0x", "0X"} {
		op := validOp()
		op.Paymaster = empty
		op.PaymasterData = empty
		if _, err := Validate(op, unsponsored, wallet); err != nil {
			t.Fatalf("paymaster field %q was treated as sponsorship: %v", empty, err)
		}
	}
}

// The same operation is fine where the route does sponsor.
func TestValidateAllowsSponsorshipWhereOffered(t *testing.T) {
	op := validOp()
	op.Paymaster = "0x4444444444444444444444444444444444444444"
	op.PaymasterData = "0xabcdef"
	if _, err := Validate(op, intent(), wallet); err != nil {
		t.Fatalf("a sponsored operation was rejected on a sponsored route: %v", err)
	}
}
