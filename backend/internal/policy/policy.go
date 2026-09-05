// Package policy holds the platform's own rules about environments, operator
// composition, and plans.
//
// These are *platform* rules, not protocol guarantees, and the distinction is
// load-bearing everywhere it surfaces. The chain has no notion of an
// environment or a plan: `SignetFactory.createGroup` will happily deploy a
// group with any operators, to anyone who calls it. What the rules below
// govern is what this platform will do on a developer's behalf — deploy,
// attach, sponsor, and promote. A developer who transacts with the contracts
// directly is outside all of it, by design.
//
// Everything user-facing that derives from this package says so.
package policy

import "fmt"

// FirstPartyCategory marks an operator run by O'Leary Labs. The operator
// directory is staff-curated, so this is a claim the platform makes about
// itself rather than something derived from the chain.
const FirstPartyCategory = "signet"

// FreePlan is the plan a new organization starts on.
const FreePlan = "developer"

// MinExternalOperatorsForProduction is how many operators outside O'Leary Labs
// a group must have before the platform will run production traffic on it.
//
// Two, not one. The product's whole claim is that no single organization can
// authorize on your behalf; one external operator alongside O'Leary Labs still
// leaves a two-party group where we are half of it. Two makes O'Leary Labs
// removable without the group falling below a sane threshold.
const MinExternalOperatorsForProduction = 2

// IsFirstParty reports whether an operator category denotes an O'Leary Labs
// node.
func IsFirstParty(category string) bool { return category == FirstPartyCategory }

// PlanIsPaid reports whether a plan is something other than the free tier.
func PlanIsPaid(plan string) bool { return plan != "" && plan != FreePlan }

// Requirement is one condition on an environment change, and whether it is met.
type Requirement struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Met    bool   `json:"met"`
	// Blocking distinguishes a hard gate from advice. Everything here is
	// blocking today; the field exists so a future soft recommendation does not
	// have to look identical to a refusal.
	Blocking bool `json:"blocking"`
}

// EnvironmentCheck is the full answer to "can this app move to that
// environment, and if not, what is missing".
type EnvironmentCheck struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	Allowed      bool          `json:"allowed"`
	Requirements []Requirement `json:"requirements"`
	// Note explains the platform-versus-protocol boundary in the place a
	// developer is most likely to mistake one for the other.
	Note string `json:"note"`
}

// GroupComposition is what the caller learned about an app's operators.
type GroupComposition struct {
	ActiveTotal      int
	ActiveFirstParty int
	ActiveExternal   int
}

// CheckEnvironment evaluates a proposed environment change.
//
// Only the move to production is gated. Development is where a group starts:
// the platform deploys it on O'Leary Labs operators and pays the gas. Bringing
// other operators in happens inside development — that is the path to
// production, not a separate tier.
func CheckEnvironment(from, to, plan string, group GroupComposition, hasGroup bool) EnvironmentCheck {
	check := EnvironmentCheck{From: from, To: to, Allowed: true}

	if to != "production" {
		check.Note = "Only the move to production is gated. Invite operators into your " +
			"development group whenever you are ready; nothing about that is charged."
		return check
	}

	check.Note = "These are platform rules, not protocol ones. The contracts do not know what " +
		"an environment or a plan is — this governs what the platform will run production " +
		"traffic on, and what it will sponsor."

	paid := PlanIsPaid(plan)
	check.Requirements = append(check.Requirements, Requirement{
		ID:    "paid_plan",
		Label: "A paid plan",
		Detail: "Production usage is billed per active wallet. The free plan covers " +
			"development, where we deploy the group and pay its gas.",
		Met:      paid,
		Blocking: true,
	})

	check.Requirements = append(check.Requirements, Requirement{
		ID: "external_operators",
		Label: fmt.Sprintf("At least %d operators outside O'Leary Labs",
			MinExternalOperatorsForProduction),
		Detail: fmt.Sprintf(
			"Your group has %d active operator(s), %d of them ours. A group we run alone is "+
				"single-operator — exactly the failure mode this network exists to remove — "+
				"which is why it cannot hold production traffic.",
			group.ActiveTotal, group.ActiveFirstParty),
		Met:      group.ActiveExternal >= MinExternalOperatorsForProduction,
		Blocking: true,
	})

	check.Requirements = append(check.Requirements, Requirement{
		ID:       "group_deployed",
		Label:    "A deployed signing group",
		Detail:   "An app with no group on-chain cannot sign anything at all.",
		Met:      hasGroup,
		Blocking: true,
	})

	for _, r := range check.Requirements {
		if r.Blocking && !r.Met {
			check.Allowed = false
		}
	}
	return check
}

// DeployableOperatorError describes why the platform will not *deploy* a group
// on the given operators. It returns "" when the set is acceptable.
//
// The platform deploys and pays for development groups, so it deploys them on
// its own operators — nobody else should be asked to carry someone's throwaway
// test traffic, or to be enrolled by a party that is not their customer.
//
// This constrains creation, not the group's life afterwards. A developer
// invites whoever they like into their own group; that is exactly the path to
// production, and it is signed by them rather than by us.
func DeployableOperatorError(externalOperators []string) string {
	if len(externalOperators) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"the platform deploys a group on O'Leary Labs operators only, and this request names %d "+
			"other operator(s): %v. Deploy with ours, then invite the operators you want from "+
			"the marketplace — that is the route to production",
		len(externalOperators), externalOperators)
}
