package main

import (
	"fmt"
	"io"

	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// trustVerb mirrors versionRequested's shape: it inspects os.Args-style
// arguments and reports which trust-store verb (if any) was requested. Only
// a bare "trust"/"untrust" as args[1] matches -- no flags are supported, so
// "trust --force" does not match.
func trustVerb(args []string) (string, bool) {
	if len(args) != 2 {
		return "", false
	}
	switch args[1] {
	case "trust", "untrust":
		return args[1], true
	default:
		return "", false
	}
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
		fmt.Fprintf(out, "No local config found at %s -- nothing to %s.\n", localPath, verb)
		return 1
	}

	hash, err := config.HashLocalConfig(localPath)
	if err != nil {
		fmt.Fprintf(out, "Error reading local config %s: %v\n", localPath, err)
		return 1
	}

	store, err := config.LoadTrust(trustPath)
	if err != nil {
		// Fail closed: a malformed/unreadable trust store is never
		// rewritten -- surface the error and leave it untouched.
		fmt.Fprintf(out, "Error reading trust store %s: %v\n", trustPath, err)
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
		fmt.Fprintf(out, "Error writing trust store %s: %v\n", trustPath, err)
		return 1
	}
	return 0
}
