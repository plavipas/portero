package main

import (
	"fmt"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
)

// containerLines renders one "containers of <profile>" line per Firefox profile with a readable containers.json.
func containerLines(b browsers.Browser) []string {
	if b.Kind != browsers.Firefox {
		return nil
	}
	profiles, err := b.Profiles()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range profiles {
		list, err := b.Containers(p)
		if err != nil {
			continue
		}
		var names []string
		for _, c := range list {
			names = append(names, c.Name)
		}
		line := fmt.Sprintf("    containers of %s: %s", p.Name, strings.Join(names, ", "))
		if active, err := b.ContainerExtension(p); err == nil {
			if active {
				line += " (extension: active)"
			} else {
				line += " (extension: missing)"
			}
		}
		out = append(out, line)
	}
	return out
}
