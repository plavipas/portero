package browsers

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

const (
	// ContainerExtensionID is the add-on that opens ext+container: links.
	ContainerExtensionID = "{f069aec0-43c5-4bbf-b6b4-df95c4326b98}"
	// ContainerExtensionName is its display name.
	ContainerExtensionName = "Open external links in a container"
)

// Container is one Firefox container (contextual identity).
type Container struct {
	Name string
	ID   uint32
}

// Firefox labels the built-in containers by id; containers.json stores no name for them.
var builtinContainers = map[uint32]string{1: "Personal", 2: "Work", 3: "Banking", 4: "Shopping"}

var legacyContainerLabels = map[string]string{
	"userContextPersonal.label": "Personal",
	"userContextWork.label":     "Work",
	"userContextBanking.label":  "Banking",
	"userContextShopping.label": "Shopping",
}

// FirefoxContainers reads containers.json and returns the user-visible containers sorted by id.
func FirefoxContainers(path string) ([]Container, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Identities []struct {
			ID     uint32 `json:"userContextId"`
			Public bool   `json:"public"`
			Name   string `json:"name"`
			L10nID string `json:"l10nID"`
		} `json:"identities"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []Container
	for _, i := range file.Identities {
		if !i.Public {
			continue
		}
		name := i.Name
		if name == "" {
			if i.L10nID == "" {
				name = builtinContainers[i.ID]
			} else {
				name = legacyContainerLabels[i.L10nID]
			}
		}
		if name == "" {
			continue
		}
		out = append(out, Container{Name: name, ID: i.ID})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, nil
}

// FirefoxExtensionActive reports whether extensions.json lists the add-on as installed and enabled.
func FirefoxExtensionActive(extensionsJSON, id string) (bool, error) {
	raw, err := os.ReadFile(extensionsJSON)
	if err != nil {
		return false, err
	}
	var file struct {
		Addons []struct {
			ID           string `json:"id"`
			Active       bool   `json:"active"`
			UserDisabled bool   `json:"userDisabled"`
			AppDisabled  bool   `json:"appDisabled"`
		} `json:"addons"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return false, fmt.Errorf("%s: %w", extensionsJSON, err)
	}
	for _, a := range file.Addons {
		if a.ID == id && a.Active && !a.UserDisabled && !a.AppDisabled {
			return true, nil
		}
	}
	return false, nil
}
