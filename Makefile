.PHONY: all collect

all: collect

collect/collect: collect/*.go
	go build -o "$@" ./collect

collect: collect/collect
	TWITTER_BEARER_TOKEN="$$(cat ../.secrets/twitter_bearer_token)" \
		$< --config config.yml --state state.json
