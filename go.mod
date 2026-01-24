module github.com/dotcommander/agent

go 1.24.0

// LOCAL DEV ONLY: Remove this replace directive before tagging releases.
// External users must comment out or delete this line to use published versions.
// See CLAUDE.md "Dependencies" section for details.
replace github.com/dotcommander/agent-sdk-go => ../agent-sdk-go

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/dotcommander/agent-sdk-go v0.0.0-00010101000000-000000000000
	github.com/fsnotify/fsnotify v1.9.0
	github.com/sony/gobreaker v1.0.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/sync v0.19.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
