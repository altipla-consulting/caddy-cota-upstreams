
FILES = $(shell find . -type f -name '*.go' -not -path './vendor/*')

gofmt:
	@gofmt -s -w $(FILES)
	@gofmt -r '&α{} -> new(α)' -w $(FILES)
	@impsort . -p github.com/altipla-consulting/caddy-cota-upstreams

lint:
	go install ./...
	go vet ./...
	linter ./...

test:
	go test -v -race ./...
