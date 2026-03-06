.PHONY: all solai tools token-price clean

# Build everything
all: solai tools

# Build the solai CLI binary → .out/solai
solai:
	CGO_ENABLED=0 go build -o .out/solai ./solai-agent

# Build all tools
tools: token-price

# Build the token-price subagent → tools/token-price/bin/token-price
# (must match the executable path declared in its manifest.json)
token-price:
	go build -o tools/token-price/bin/token-price ./tools/token-price

clean:
	rm -rf .out tools/token-price/bin
