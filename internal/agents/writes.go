package agents

// WritesCode reports whether a run of role wrote the code under review: false
// for read-only roles and for spec authors (scope specs), true for the rest,
// for an empty role (`hoom run`) and for an unknown one — nothing says it did
// not write. It is the one rule behind the cross review and the card's
// writer logo.
func WritesCode(role string) bool {
	r, err := Lookup(role)
	if err != nil {
		return true
	}
	return !r.ReadOnly && r.Scope != ScopeSpecs
}

// AllowPorDefecto is the default write territory of a role of shape scope
// (ScopeEvidencia, ScopeTests, ScopeCodigo, ScopeSpecs): what
// agents.<rol>.write.allow is when hoom.yaml does not declare it, the list
// agentcmd.PolicyFor starts from. It lives here, and not in agentcmd, so that
// verify, the review and the board read the test-writer's territory
// (spec riesgo-efectivo-y-tamano: «prueba» is that territory) without
// importing agentcmd, which imports verifycmd. For ScopeTests it is, in this
// order: .hoom/**, tests/**, test/**, spec/**, src/test/**, **/*_test.go,
// **/*Test.php, **/*Test.kt, **/*Test.java, **/*Spec.kt, **/*.test.ts,
// **/*.test.js; ScopeCodigo **, ScopeSpecs .hoom/specs/**, and any other
// scope .hoom/**. Each call returns a new slice.
func AllowPorDefecto(scope string) []string {
	switch scope {
	case ScopeTests:
		return []string{".hoom/**", "tests/**", "test/**", "spec/**", "src/test/**",
			"**/*_test.go", "**/*Test.php", "**/*Test.kt", "**/*Test.java",
			"**/*Spec.kt", "**/*.test.ts", "**/*.test.js"}
	case ScopeCodigo:
		return []string{"**"}
	case ScopeSpecs:
		// the spec authors' territory is the spec, not the rest of .hoom/:
		// role contracts, intake documents and findings stay out
		return []string{".hoom/specs/**"}
	default: // evidencia
		return []string{".hoom/**"}
	}
}
