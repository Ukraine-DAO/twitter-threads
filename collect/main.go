package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Ukraine-DAO/twitter-threads/common"
	"github.com/Ukraine-DAO/twitter-threads/state"
)

var (
	configPath = flag.String("config", "", "Path to the config file")
	statePath  = flag.String("state", "", "Path to the state file")
)

func main() {
	flag.Parse()

	b, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err)
	}

	cfg := &common.Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %s", err)
	}

	state := state.New()
	f, err := os.Open(*statePath)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to read state file: %s", err)
	}

	if err == nil {
		if err := json.NewDecoder(f).Decode(state); err != nil {
			log.Fatalf("Failed to unmarshal state: %s", err)
		}
		f.Close()
	}

	if err := state.Update(cfg); err != nil {
		log.Fatalf("Failed to update state: %s", err)
	}

	f, err = os.Create(*statePath)
	if err != nil {
		log.Fatalf("Failed to open state file for writing: %s", err)
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(state); err != nil {
		log.Fatalf("Failed to marshal state: %s", err)
	}
	f.Close()
}
