.PHONY: all collect fetch render

all: render

collect:
	TWITTER_BEARER_TOKEN="$$(cat .secrets/twitter_bearer_token)" \
		go run ./collect --config config.yml --state stored-state/state.json

fetch:
	go run ./fetch-media --config config.yml \
		--state stored-state/state.json \
		--output_dir generated

render:
	go run ./render --config config.yml \
		--state stored-state/state.json \
		--output_dir generated \
		--mappings generated/mappings.json
