package userop

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

const (
	txGroup   = "0x86fE28144034FDAf86D3c964296DD33E4b94aC59"
	txEOA     = "0x762F96819a7705448843E96D63D638Ec2f39403B"
	txSmart   = "0xc74c144F330970710d7EC928868Df02055f04e6E"
	txOther   = "0x000000000000000000000000000000000000dEaD"
	addKeySel = "d1a3d5d2"
)

func txIntent() Intent {
	return Intent{
		Action:    "adding an authorization key",
		Dest:      txGroup,
		Selectors: map[string]string{"addAuthKey": addKeySel},
	}
}

func addKeyInput() []byte {
	b, _ := hex.DecodeString(addKeySel + strings.Repeat("00", 32))
	return b
}

// The whole point of the transaction path: a group whose manager is an EOA
// cannot be managed by a user operation, because the contract compares
// msg.sender to the manager and a smart account is not it.
func TestValidateTransactionAcceptsEitherAddressTheCallerControls(t *testing.T) {
	controlled := []string{txSmart, txEOA}

	for _, from := range []string{txEOA, txSmart, strings.ToUpper(txEOA)} {
		got, err := ValidateTransaction(from, txGroup, addKeyInput(), big.NewInt(0), txIntent(), controlled)
		if err != nil {
			t.Fatalf("from %s: %v", from, err)
		}
		if got.Method != "addAuthKey" {
			t.Errorf("from %s: method %q", from, got.Method)
		}
	}
}

// A transaction hash is public. Without this check, quoting someone else's
// would have the platform record metadata against your app on the strength of
// their transaction.
func TestValidateTransactionRefusesAStrangersTransaction(t *testing.T) {
	_, err := ValidateTransaction(txOther, txGroup, addKeyInput(), big.NewInt(0), txIntent(),
		[]string{txSmart, txEOA})
	if err == nil {
		t.Fatal("accepted a transaction the caller did not send")
	}
	if !strings.Contains(err.Error(), "not an account you control") {
		t.Errorf("refusal should say why: %v", err)
	}
}

func TestValidateTransactionAppliesTheSameIntentRulesAsAUserOperation(t *testing.T) {
	controlled := []string{txEOA}
	wrongSelector, _ := hex.DecodeString("deadbeef" + strings.Repeat("00", 32))

	cases := []struct {
		name  string
		to    string
		input []byte
		value *big.Int
		want  string
	}{
		{"another contract", txOther, addKeyInput(), big.NewInt(0), "acts on"},
		{"a method the route does not allow", txGroup, wrongSelector, big.NewInt(0), "allows only"},
		{"a call that moves value", txGroup, addKeyInput(), big.NewInt(1), "never transfers"},
		{"no selector at all", txGroup, []byte{0x01}, big.NewInt(0), "no selector"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ValidateTransaction(txEOA, c.to, c.input, c.value, txIntent(), controlled)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("want %q in refusal, got: %v", c.want, err)
			}
		})
	}
}

// A user with no addresses at all cannot have sent anything.
func TestValidateTransactionRefusesWhenTheCallerControlsNothing(t *testing.T) {
	if _, err := ValidateTransaction(txEOA, txGroup, addKeyInput(), big.NewInt(0), txIntent(), nil); err == nil {
		t.Fatal("accepted with no controlled addresses")
	}
	if _, err := ValidateTransaction("", txGroup, addKeyInput(), big.NewInt(0), txIntent(), []string{""}); err == nil {
		t.Fatal("accepted a blank sender against a blank candidate")
	}
}
