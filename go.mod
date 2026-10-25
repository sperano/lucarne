module github.com/sperano/lucarne

go 1.27.1

// npm packages can ship Go files (flatted does); keep them out of ./...
ignore ./web/node_modules

require github.com/santhosh-tekuri/jsonschema/v6 v6.0.2

require golang.org/x/text v0.14.0 // indirect
