module github.com/transpara-ai/agent

go 1.24.2

require github.com/transpara-ai/eventgraph/go v0.0.0-20260309152918-5602caa542f2

require (
	github.com/anthropics/anthropic-sdk-go v1.26.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sync v0.17.0 // indirect
)

replace github.com/transpara-ai/eventgraph/go => ../eventgraph/go
