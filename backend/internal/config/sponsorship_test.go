package config

import "testing"

// Sponsorship spends real ETH and sign-in is open to any Google account, so an
// unset list must sponsor nobody. A spending gate that defaults to permitting
// spending is not a gate — and this is the case a deploy would silently get
// wrong, because the variable is easy to forget and the failure is a bill.
func TestSponsorshipIsClosedByDefault(t *testing.T) {
	empty := &Config{}
	for _, subject := range []string{
		"signet:02b285a5aa8819d8a0cbdbf4f67f01bef320ea78369eae6dcdab06f6658ddf8f73",
		"eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10",
		"",
	} {
		if empty.IsSponsoredSubject(subject) {
			t.Errorf("an unset list sponsored %q", subject)
		}
	}
}

// Both sign-in routes produce subjects, and either may be sponsored: a Signet
// subject is "signet:" + the group public key, a wallet subject "eth:" + the
// address. The address form is normalised, since one pasted from a block
// explorer is checksummed and the stored form is not.
func TestSponsoredSubjectsAcceptBothRoutes(t *testing.T) {
	cfg := &Config{SponsoredSubjects: normalizeSubjects(splitList(
		"signet:02B285A5AA8819D8A0CBDBF4F67F01BEF320EA78369EAE6DCDAB06F6658DDF8F73, " +
			"0xDCB876Ac74297655a73F04DC7a5039175dD15e10"))}

	for _, subject := range []string{
		"signet:02b285a5aa8819d8a0cbdbf4f67f01bef320ea78369eae6dcdab06f6658ddf8f73",
		"eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10",
	} {
		if !cfg.IsSponsoredSubject(subject) {
			t.Errorf("listed subject %q was refused", subject)
		}
	}
	if cfg.IsSponsoredSubject("signet:03deadbeef") {
		t.Error("an unlisted Signet subject was sponsored")
	}
}

// Staff and sponsorship are separate grants. Curating the marketplace is not a
// licence to spend, and vice versa.
func TestStaffAndSponsorshipDoNotImplyEachOther(t *testing.T) {
	cfg := &Config{
		StaffSubjects:     normalizeSubjects(splitList("0xdcb876ac74297655a73f04dc7a5039175dd15e10")),
		SponsoredSubjects: normalizeSubjects(splitList("signet:02b285")),
	}
	if cfg.IsSponsoredSubject("eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10") {
		t.Error("a staff subject was sponsored without being listed")
	}
	if cfg.IsStaffSubject("signet:02b285") {
		t.Error("a sponsored subject was made staff without being listed")
	}
}

// The default ceiling exists so a listed developer cannot loop, and so a
// mistaken entry costs a bounded amount.
func TestDefaultSponsoredGroupCeilingIsFinite(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SESSION_SECRET", "x")
	cfg := Load()
	if cfg.MaxSponsoredGroupsPerOrg <= 0 {
		t.Fatalf("default ceiling is %d — unbounded by default", cfg.MaxSponsoredGroupsPerOrg)
	}
}
