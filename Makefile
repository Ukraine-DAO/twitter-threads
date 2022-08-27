.PHONY: all collect render

all: render

collect:
	TWITTER_BEARER_TOKEN="$$(cat .secrets/twitter_bearer_token)" \
		go run ./collect --config config.yml --state stored-state/state.json

render:
	go run ./render --config config.yml --state stored-state/state.json --output_dir generated
