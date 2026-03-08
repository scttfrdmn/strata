package main

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

func newSearchCmd() *cobra.Command {
	var arch, family string
	var formations bool

	cmd := &cobra.Command{
		Use:   "search [name]",
		Short: "Search the embedded layer/formation catalog",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			if formations {
				searchFormations(name)
				return nil
			}
			searchLayers(name, arch, family)
			return nil
		},
	}

	cmd.Flags().StringVar(&arch, "arch", "", "filter by architecture: x86_64 or arm64")
	cmd.Flags().StringVar(&family, "family", "", "filter by OS family: rhel or debian")
	cmd.Flags().BoolVar(&formations, "formation", false, "list formations instead of layers")
	return cmd
}

// searchLayers prints a table of layers matching the given filters.
func searchLayers(name, arch, family string) {
	store := buildCatalog()
	layers, err := store.ListLayers(context.Background(), name, arch, family)
	if err != nil {
		fmt.Printf("search: listing layers: %v\n", err)
		return
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
