package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// runSearch implements "strata search [name] [--arch ...] [--family ...] [--formation]".
//
// Without --formation it prints a table of matching layers from the embedded
// Tier 0 catalog. With --formation it lists formation definitions instead.
func runSearch(args []string) {
	fset := flag.NewFlagSet("search", flag.ExitOnError)
	arch := fset.String("arch", "", "filter by architecture: x86_64 or arm64")
	family := fset.String("family", "", "filter by OS family: rhel or debian")
	formations := fset.Bool("formation", false, "list formations instead of layers")
	fset.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: strata search [name] [--arch x86_64|arm64] [--family rhel|debian] [--formation]\n")
		fset.PrintDefaults()
	}
	if err := fset.Parse(args); err != nil {
		fatal("search: %v", err)
	}
	name := fset.Arg(0)

	if *formations {
		searchFormations(name)
		return
	}
	searchLayers(name, *arch, *family)
}

// searchLayers prints a table of layers matching the given filters.
func searchLayers(name, arch, family string) {
	store := buildCatalog()
	layers, err := store.ListLayers(context.Background(), name, arch, family)
	if err != nil {
		fatal("search: listing layers: %v", err)
	}
	if len(layers) == 0 {
		fmt.Println("no layers found")
		return
	}

	fmt.Printf("%-16s %-12s %-8s %-8s %s\n", "NAME", "VERSION", "ARCH", "FAMILY", "PROVIDES")
	fmt.Printf("%-16s %-12s %-8s %-8s %s\n",
		"----------------", "------------", "--------", "--------", "--------")
	for _, m := range layers {
		provides := ""
		for i, p := range m.Provides {
			if i > 0 {
				provides += ", "
			}
			provides += p.String()
		}
		fmt.Printf("%-16s %-12s %-8s %-8s %s\n", m.Name, m.Version, m.Arch, m.Family, provides)
	}
}

// searchFormations prints a table of formations from the embedded catalog.
func searchFormations(name string) {
	entries, err := fs.Glob(catalogFS, "formations/*.yaml")
	if err != nil || len(entries) == 0 {
		fmt.Println("no formations found")
		return
	}

	fmt.Printf("%-28s %-10s %s\n", "NAME", "VERSION", "DESCRIPTION")
	fmt.Printf("%-28s %-10s %s\n", "----------------------------", "----------", "-----------")

	for _, path := range entries {
		data, readErr := catalogFS.ReadFile(path)
		if readErr != nil {
			continue
		}
		var f spec.Formation
		if yaml.Unmarshal(data, &f) != nil {
			continue
		}
		if name != "" && f.Name != name {
			continue
		}
		desc := f.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Printf("%-28s %-10s %s\n", f.Name, f.Version, desc)
	}
}
