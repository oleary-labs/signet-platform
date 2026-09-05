package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Every exported Config field must actually be assigned in Load(). A field that
// is declared but never populated compiles, passes vet, and silently disables
// whatever it controls — which is exactly how the bundler and the paymaster
// were dark for a while.
func TestEveryFieldIsAssignedInLoad(t *testing.T) {
	source, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "c := &Config{")
	if start == -1 {
		t.Fatal("could not find the Config literal in Load()")
	}
	literal := body[start:]

	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !strings.Contains(literal, name+":") {
			t.Errorf("Config.%s is declared but never assigned in Load()", name)
		}
	}
}
