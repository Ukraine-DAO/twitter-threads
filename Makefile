.PHONY: all collect render

all: collect

collect/collect: collect/*.go common/*.go state/*.go twitter/*.go
	go build -o "$@" ./collect

collect: collect/collect config.yml
	TWITTER_BEARER_TOKEN="$$(cat ../.secrets/twitter_bearer_token)" \
		$< --config config.yml --state state.json

render/render: render/*.go common/*.go state/*.go twitter/*.go
	go build -o "$@" ./render

render: render/render config.yml state.json
	$< --config config.yml --state state.json --output_dir generated
