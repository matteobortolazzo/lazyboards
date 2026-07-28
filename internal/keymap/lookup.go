package keymap

import (
	"sort"
	"strings"
)

// Outcome is the result of a Lookup call.
type Outcome int

const (
	// OutcomeNoMatch is the zero value of Outcome: seq has no exact
	// binding and is not a prefix of any bound sequence.
	OutcomeNoMatch Outcome = iota
	// OutcomeMatch means seq is exactly bound; Result.Binding holds the
	// resolved binding.
	OutcomeMatch
	// OutcomePending means seq is a strict prefix of one or more bound
	// sequences; Result.Candidates holds every match.
	OutcomePending
)

// Candidate is one sequence a pending lookup could still resolve to. It
// carries no "is this a custom action" flag -- Binding.Kind is the only
// way to distinguish a command from an inline action, by design (see
// docs/... Outcome purity).
type Candidate struct {
	Sequence string
	Binding  Binding
}

// Result is the outcome of a Lookup call. Candidates is sorted by canonical
// sequence string for deterministic, reproducible which-key rendering.
type Result struct {
	Outcome    Outcome
	Binding    Binding
	Candidates []Candidate
}

// Lookup resolves seq against (mode, column)'s effective table.
//
// If the last key of seq is "ctrl+c", Lookup short-circuits to
// OutcomeMatch(CommandBinding(CommandQuit)) before consulting any table --
// this holds regardless of table contents, even if some table entry
// rebinds ctrl+c, and regardless of seq's earlier keys, so a pending
// prefix can never strand a user without a way to quit.
//
// Otherwise Lookup canonicalizes seq via seq.String() and resolves the
// effective table for (mode, column): the column-overlaid table when mode
// is ModeNormal or ModeDetail and column matches (case-insensitively) an
// overlay, the plain mode table otherwise. A canonical key present in that
// table with a binding other than BindingUnbound/BindingInvalid is an
// exact OutcomeMatch. Failing that, every table entry whose canonical key
// is seq extended by further keys (a whitespace-boundary prefix, not a
// raw substring test) and whose binding is not BindingUnbound/
// BindingInvalid becomes an OutcomePending candidate; with no such entry,
// the result is OutcomeNoMatch.
func (k *Keymap) Lookup(mode Mode, column string, seq Sequence) Result {
	if len(seq) > 0 && seq[len(seq)-1] == Key("ctrl+c") {
		return Result{Outcome: OutcomeMatch, Binding: CommandBinding(CommandQuit)}
	}

	table := k.effectiveTable(mode, column)
	query := seq.String()

	if binding, ok := table[query]; ok && isResolved(binding) {
		return Result{Outcome: OutcomeMatch, Binding: binding}
	}

	prefix := query + " "
	var candidates []Candidate
	for canonical, binding := range table {
		if !isResolved(binding) {
			continue
		}
		if strings.HasPrefix(canonical, prefix) {
			candidates = append(candidates, Candidate{Sequence: canonical, Binding: binding})
		}
	}
	if len(candidates) > 0 {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Sequence < candidates[j].Sequence })
		return Result{Outcome: OutcomePending, Candidates: candidates}
	}
	return Result{Outcome: OutcomeNoMatch}
}

// isResolved reports whether binding represents an actual bound target --
// false for BindingUnbound (explicitly unbound) and BindingInvalid (the
// zero value, "never specified").
func isResolved(binding Binding) bool {
	return binding.Kind != BindingUnbound && binding.Kind != BindingInvalid
}
