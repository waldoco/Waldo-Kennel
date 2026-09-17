package domain

import (
	"testing"
	"time"
)

func policyFixture() AdmissionPolicy {
	p := AdmissionPolicy{ID: "p", Version: "v", Default: ExecutionBudget{WallTimeLimit: time.Hour, RetryLimit: 1, TokenAccounting: TokenAccountingUnsupported, Source: ExecutionBudgetPolicyDefault, PolicyID: "p", PolicyVersion: "v"}, MaxWallTime: 2 * time.Hour, MaxRetries: 2}
	p.Digest, _ = p.ComputedDigest()
	p.Default.PolicyDigest = p.Digest
	return p
}
func TestAdmissionPolicyDigestBindsEveryField(t *testing.T) {
	p := policyFixture()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.MaxRetries++
	if err := p.Validate(); err == nil {
		t.Fatal("mutation accepted")
	}
	p = policyFixture()
	p.Digest = "attacker"
	p.Default.PolicyDigest = "attacker"
	if err := p.Validate(); err == nil {
		t.Fatal("chosen digest accepted")
	}
}
func TestAdmissionPolicyDigestDeterministicAndDefaultBound(t *testing.T) {
	a, b := policyFixture(), policyFixture()
	da, _ := a.ComputedDigest()
	db, _ := b.ComputedDigest()
	if da != db {
		t.Fatal("nondeterministic")
	}
	b.Default.PolicyDigest = "other"
	if err := b.Validate(); err == nil {
		t.Fatal("default digest unbound")
	}
}
