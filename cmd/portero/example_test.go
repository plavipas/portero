package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func TestExampleRulesIdenticalAndValid(t *testing.T) {
	onDisk, err := os.ReadFile("../../examples/rules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, exampleRules) {
		t.Error("examples/rules.yaml and cmd/portero/rules.example.yaml differ")
	}
	r, err := rules.Parse(exampleRules)
	if err != nil {
		t.Fatal(err)
	}
	groups := 0
	for _, rule := range r.Rules {
		if rule.IsGroup() {
			groups++
		}
	}
	if groups != 1 || len(r.Rules) < 3 {
		t.Errorf("want one group and plain rules, got %d groups in %d elements", groups, len(r.Rules))
	}
}

func TestRuleCounts(t *testing.T) {
	r, err := rules.Parse(exampleRules)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ruleCounts(r), "7 rule(s): 3 plain, 4 in 1 group(s)"; got != want {
		t.Errorf("ruleCounts = %q, want %q", got, want)
	}
	if got := ruleCounts(&rules.Rules{Rules: []rules.Rule{{}, {}}}); got != "2 rule(s)" {
		t.Errorf("flat = %q", got)
	}
}
