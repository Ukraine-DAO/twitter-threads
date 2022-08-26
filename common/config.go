package common

import (
	"sort"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Root     Subdir
	MediaDir string `yaml:"media_dir"`
}

type Subdir struct {
	Subdirs map[string]Subdir     `yaml:",omitempty"`
	Pages   map[string]YamlThread `yaml:",inline,omitempty"`
}

type Thread struct {
	ThreadID string `yaml:"thread_id"`
}

type YamlThread struct {
	Thread `yaml:",inline"`
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
	for _, v := range s.Pages {
		r = append(r, v.ThreadID)
	}
	for _, sd := range s.Subdirs {
		r = append(r, sd.ThreadIDs()...)
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
