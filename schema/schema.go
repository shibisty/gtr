// Package schema holds the JSON Schema of gtr.json, for tools that serve
// it (gtr mcp's gtr://schema resource).
package schema

import _ "embed"

// JSON is gtr.schema.json.
//
//go:embed gtr.schema.json
var JSON []byte
