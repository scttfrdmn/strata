// Command mkregistry writes the testregistry fixture into a directory, so that
// an offline resolve can be reproduced from a shell rather than only from a Go
// test:
//
//	go run ./internal/testregistry/mkregistry /tmp/strata-fixture
//	STRATA_REGISTRY_URL=file:///tmp/strata-fixture strata resolve profile.yaml
//
// It also writes the fixture profiles next to the registry, so that the command
// above has something to resolve. It prints the STRATA_REGISTRY_URL value on
// stdout.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scttfrdmn/strata/internal/testregistry"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: mkregistry <dir>\n") //nolint:errcheck
		os.Exit(2)
	}
	dir := os.Args[1]

	root, err := testregistry.Materialize(context.Background(), filepath.Join(dir, "registry"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkregistry: %v\n", err) //nolint:errcheck
		os.Exit(1)
	}

	profileDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkregistry: %v\n", err) //nolint:errcheck
		os.Exit(1)
	}
	for _, name := range testregistry.ProfileNames() {
		data, err := testregistry.ProfileBytes(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mkregistry: %v\n", err) //nolint:errcheck
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(profileDir, name), data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "mkregistry: %v\n", err) //nolint:errcheck
			os.Exit(1)
		}
	}

	fmt.Println(testregistry.URI(root))
}
