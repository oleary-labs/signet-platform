package policy

import "testing"

func TestProductionRequiresPaidPlanAndExternalOperators(t *testing.T) {
	full := GroupComposition{ActiveTotal: 3, ActiveFirstParty: 1, ActiveExternal: 2}

	t.Run("everything satisfied", func(t *testing.T) {
		c := CheckEnvironment("development", "production", "team", full, true)
		if !c.Allowed {
			t.Fatalf("expected allowed, got %+v", c.Requirements)
		}
	})

	t.Run("free plan blocks", func(t *testing.T) {
		c := CheckEnvironment("development", "production", FreePlan, full, true)
		if c.Allowed {
			t.Fatal("the free plan should not reach production")
		}
		if !requirementMet(c, "external_operators") {
			t.Error("operator requirement should still read as met")
		}
		if requirementMet(c, "paid_plan") {
			t.Error("plan requirement should read as unmet")
		}
	})

	t.Run("one external operator is not enough", func(t *testing.T) {
		// A group of us plus one other is still a two-party group we are half
		// of — the requirement exists precisely to rule this out.
		one := GroupComposition{ActiveTotal: 2, ActiveFirstParty: 1, ActiveExternal: 1}
		c := CheckEnvironment("development", "production", "team", one, true)
		if c.Allowed {
			t.Fatal("one external operator should not reach production")
		}
	})

	t.Run("first-party-only group blocks", func(t *testing.T) {
		ours := GroupComposition{ActiveTotal: 3, ActiveFirstParty: 3, ActiveExternal: 0}
		c := CheckEnvironment("development", "production", "enterprise", ours, true)
		if c.Allowed {
			t.Fatal("a group we run alone should not reach production")
		}
	})

	t.Run("no group blocks", func(t *testing.T) {
		c := CheckEnvironment("development", "production", "team", full, false)
		if c.Allowed {
			t.Fatal("an app with no group should not reach production")
		}
	})
}

func TestNonProductionMovesAreUngated(t *testing.T) {
	empty := GroupComposition{}
	for _, to := range []string{"development"} {
		c := CheckEnvironment("development", to, FreePlan, empty, false)
		if !c.Allowed {
			t.Errorf("move to %s should be ungated, got %+v", to, c.Requirements)
		}
		if len(c.Requirements) != 0 {
			t.Errorf("move to %s should carry no requirements", to)
		}
	}
}

func TestDeployableOperatorError(t *testing.T) {
	if msg := DeployableOperatorError(nil); msg != "" {
		t.Fatalf("a first-party-only group should be accepted, got %q", msg)
	}
	msg := DeployableOperatorError([]string{"0xabc", "0xdef"})
	if msg == "" {
		t.Fatal("external operators in a development group should be rejected")
	}
	if !contains(msg, "marketplace") {
		t.Errorf("the refusal should point at the way forward, got %q", msg)
	}
}

func TestPlanAndCategoryHelpers(t *testing.T) {
	if PlanIsPaid(FreePlan) || PlanIsPaid("") {
		t.Error("the developer plan is the free tier")
	}
	if !PlanIsPaid("team") || !PlanIsPaid("enterprise") {
		t.Error("team and enterprise are paid")
	}
	if !IsFirstParty(FirstPartyCategory) || IsFirstParty("independent") {
		t.Error("first-party detection is wrong")
	}
}

func requirementMet(c EnvironmentCheck, id string) bool {
	for _, r := range c.Requirements {
		if r.ID == id {
			return r.Met
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
