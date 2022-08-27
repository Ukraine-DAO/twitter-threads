package common

import (
	"path"
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
	Title    string
	Config   RenderConfig
}

type YamlThread struct {
	Thread `yaml:",inline"`
}

type RenderConfig struct {
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

func (s *Subdir) ThreadPages() map[string]Thread {
	r := map[string]Thread{}
	for name, t := range s.Pages {
		r[name] = t.Thread
	}
	for name, subdir := range s.Subdirs {
		for sname, t := range subdir.ThreadPages() {
			r[path.Join(name, sname)] = t
		}
	}
	return r
}

func (cfg *Config) ThreadPages() map[string]Thread {
	return cfg.Root.ThreadPages()
}
