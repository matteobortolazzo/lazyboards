package main

import (
	"fmt"
	"io"

	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// trustVerb mirrors versionRequested's shape: it inspects os.Args-style
// arguments and reports which trust-store verb (if any) was requested. A
// bare "trust"/"untrust" as args[1] matches regardless of what (if anything)
// follows it -- extra positional args or flags (e.g. "trust --force",
// "untrust extra-arg") still match here so main() recognizes the invocation
// and can reject it with a clear usage error (see trustVerbExtraArgs) instead
// of silently falling through to a normal board launch (#568).
func trustVerb(args []string) (string, bool) {
	if len(args) < 2 {
		return "", false
	}
	switch args[1] {
	case "trust", "untrust":
		return args[1], true
	default:
		return "", false
	}
}

// trustVerbExtraArgs reports whether args -- already matched by trustVerb --
// carries anything beyond the bare verb (extra positional args and/or
// flags). main() uses this to reject such an invocation with a usage error
// rather than dispatching it to runTrustVerb.
func trustVerbExtraArgs(args []string) bool {
	return len(args) > 2
}

// runTrustVerb runs the "trust"/"untrust" CLI verb against the local config
// at localPath, hashing its content and adding/removing a matching entry in
// the trust store at trustPath. note is stored alongside a newly trusted
// entry for the user's own reference (the caller supplies os.Getwd()).
//
// It never touches any path other than localPath (read) and trustPath
// (read/write) -- no global config is read or written.
func runTrustVerb(verb, localPath, trustPath, note string, out io.Writer) int {
	if !config.LocalExists(localPath) {
		_, _ = fmt.Fprintf(out, "No local config found at %s -- nothing to %s.\n", localPath, verb)
		return 1
	}

	hash, err := config.HashLocalConfig(localPath)
	if err != nil {
		_, _ = fmt.Fprintf(out, "Error reading local config %s: %v\n", localPath, err)
		return 1
	}

	store, err := config.LoadTrust(trustPath)
	if err != nil {
		// Fail closed: a malformed/unreadable trust store is never
		// rewritten -- surface the error and leave it untouched.
		_, _ = fmt.Fprintf(out, "Error reading trust store %s: %v\n", trustPath, err)
		return 1
	}

	switch verb {
	case "trust":
		if !store.Trusts(hash) {
			store.Trusted = append(store.Trusted, config.TrustEntry{Hash: hash, Note: note})
		}
	case "untrust":
		filtered := make([]config.TrustEntry, 0, len(store.Trusted))
		for _, entry := range store.Trusted {
			if entry.Hash == hash {
				continue
			}
			filtered = append(filtered, entry)
		}
		store.Trusted = filtered
	}

	if err := config.SaveTrust(trustPath, store); err != nil {
		_, _ = fmt.Fprintf(out, "Error writing trust store %s: %v\n", trustPath, err)
		return 1
	}
	return 0
}
