module github.com/dotcommander/agent

go 1.24.0

// Uncomment for local development with sibling SDK checkout:
// replace github.com/dotcommander/agent-sdk-go => ../agent-sdk-go

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/dotcommander/agent-sdk-go v0.1.4
	github.com/fsnotify/fsnotify v1.9.0
	github.com/sony/gobreaker v1.0.0
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	golang.org/x/sync v0.19.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.13.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
