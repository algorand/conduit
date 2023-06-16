# Third Party Plugins

Conduit supports Third Party Plugins, but not in the way you may be used to
in other pluggable systems. In order to limit adding dependencies, third party
plugins are enabaled with a custom build that imports exactly the plugins you
would like to deploy.

Over time this process can be automated, but for now it is manual and requres
writing a little bit of code.

# Third Party Plugins

A listing of third party plugins. If you create a plugin, please make a PR
and add it to the list.

* [conduit-plugin-template](https://github.com/algorand/conduit-plugin-template): A collection of templates.

# Configuring Your Deployment

As an example, we'll create a custom deployment that combines a subset of the
built-in Conduit plugins and the plugins from
[conduit-plugin-template](https://github.com/algorand/conduit-plugin-template).

## Initial Setup

To get started, initialize a new directory and setup the project. We'll add two
dependencies. To include other third party plugins, they would be added in a
similar way.
```sh
go mod init mywebsite.com/my_custom_conduit
go get github.com/algorand/conduit@latest
go get github.com/algorand/conduit-plugin-template@latest
go mod tidy
go mod download
```

## Main File

To install a plugin, it must be imported. This is done using the following
pattern:

```go
package main

import (
  "fmt"
  "os"

  // Import some of the built-in plugins.
  _ "github.com/algorand/conduit/conduit/plugins/exporters/noop"
  _ "github.com/algorand/conduit/conduit/plugins/exporters/postgresql"

  // Import from the conduit-plugin-template repo
  _ "github.com/algorand/conduit-plugin-template/plugin/exporter"
  _ "github.com/algorand/conduit-plugin-template/plugin/importer"
  _ "github.com/algorand/conduit-plugin-template/plugin/processor"

  "github.com/algorand/conduit/pkg/cli"
)

func main() {
  conduitCmd := cli.MakeConduitCmdWithUtilities()
  if err := conduitCmd.Execute(); err != nil {
    fmt.Fprintf(os.Stderr, "%v\n", err)
    os.Exit(1)
  }
  os.Exit(0)
}
```

## Build and Test

For a simple test, run `main.go` directly to list the plugins. The
algod importer and file_writer exporter are imported by default, which is why
you see them included in the list:
```sh
$ go run main.go list
importers:
  algod             - Importer for fetching blocks from an algod REST API.
  importer_template - Example importer.

processors:
  processor_template - Example processor.

exporters:
  exporter_template - Example exporter.
  file_writer       - Exporter for writing data to a file.
  noop              - noop exporter
  postgresql        - Exporter for writing data to a postgresql instance.
```

Build a binary:
```go
go build .
```

Cross-compile to different operating systems or architectures:
```go
GOOS=linux GOARCH=amd64 go build -o my_custom_conduit-linux-amd64 main.go
GOOS=linux GOARCH=arm64 go build -o my_custom_conduit-linux-arm64 main.go
GOOS=darwin GOARCH=amd64 go build -o my_custom_conduit-darwin-amd64 main.go
GOOS=darwin GOARCH=arm64 go build -o my_custom_conduit-darwin-arm64 main.go
GOOS=windows GOARCH=amd64 go build -o my_custom_conduit-windows-amd64.exe main.go
GOOS=windows GOARCH=arm64 go build -o my_custom_conduit-windows-arm64.exe main.go
```
