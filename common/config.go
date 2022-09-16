package common

import (
	"fmt"
	"path"
	"sort"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Root     Subdir
	MediaDir string `yaml:"media_dir"`
}

type SubdirMapEntry struct {
	Name   string
	Subdir Subdir
}
type YamlSubdirMap []SubdirMapEntry

func (m *YamlSubdirMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected MappingNode value kind, got %d", value.Kind)
	}
	for i := 0; i < len(value.Content); i += 2 {
		entry := SubdirMapEntry{}
		if err := value.Content[i].Decode(&entry.Name); err != nil {
			return fmt.Errorf("decoding key at index %d: %w", i, err)
		}
		if err := value.Content[i+1].Decode(&entry.Subdir); err != nil {
			return fmt.Errorf("decoding value at index %d: %w", i+1, err)
		}
		*m = append(*m, entry)
	}
	return nil
}

type PageMapEntry struct {
	Name string
	Page YamlThread
}
type YamlPageMap struct {
	Entries []PageMapEntry
}

func (m *YamlPageMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected MappingNode value kind, got %d", value.Kind)
	}
	knownField := map[string]bool{
		"subdirs":     true,
		"title":       true,
		"description": true,
		"config":      true,
	}
	for i := 0; i < len(value.Content); i += 2 {
		entry := PageMapEntry{}
		if err := value.Content[i].Decode(&entry.Name); err != nil {
			return fmt.Errorf("decoding key at index %d: %w", i, err)
		}
		if knownField[entry.Name] {
			// Not sure how Unmarshaler on an inline field was intended to work,
			// but it seems we need to filter out all known fields of Subdir
			// struct ourselves.
			continue
		}
		if err := value.Content[i+1].Decode(&entry.Page); err != nil {
			return fmt.Errorf("decoding value at index %d: %w", i+1, err)
		}
		m.Entries = append(m.Entries, entry)
	}
	return nil
}

type Subdir struct {
	Title       string
	Description string
	Config      SubdirConfig
	Subdirs     YamlSubdirMap `yaml:",omitempty"`
	Pages       YamlPageMap   `yaml:",inline,omitempty"`
}

type SubdirConfig struct {
	CollapseInParent bool `yaml:"collapse_in_parent"`
}

type Thread struct {
	ThreadID string `yaml:"thread_id"`
	Title    string
	Config   RenderConfig
}

type YamlThread struct {
	Thread `yaml:",inline"`
}

type RenderConfig struct {
	MergeUpperCaseAfterEllipsis bool `yaml:"merge_upper_case_after_ellipsis"`
	MergeUnlessEndsWithPeriod   bool `yaml:"merge_unless_ends_with_period"`
}

func (t *YamlThread) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&t.ThreadID)
	}
	return value.Decode(&t.Thread)
}

func (t YamlThread) MarshalYAML() (interface{}, error) {
	if true {
		return t.ThreadID, nil
	}
	return t.Thread, nil
}

func (s *Subdir) ThreadIDs() []string {
	r := []string{}
	for _, e := range s.Pages.Entries {
		r = append(r, e.Page.ThreadID)
	}
	for _, e := range s.Subdirs {
		r = append(r, e.Subdir.ThreadIDs()...)
	}
	return r
}

func (cfg *Config) ThreadIDs() []string {
	id_list := cfg.Root.ThreadIDs()
	present := map[string]bool{}
	r := []string{}
	for _, id := range id_list {
		if present[id] {
			continue
		}
		present[id] = true
		r = append(r, id)
	}
	sort.Strings(r)
	return r
}

func (s *Subdir) ThreadPages() map[string]Thread {
	r := map[string]Thread{}
	for _, e := range s.Pages.Entries {
		r[e.Name] = e.Page.Thread
	}
	for _, e := range s.Subdirs {
		for sname, t := range e.Subdir.ThreadPages() {
			r[path.Join(e.Name, sname)] = t
		}
	}
	return r
}

func (cfg *Config) ThreadPages() map[string]Thread {
	return cfg.Root.ThreadPages()
}
