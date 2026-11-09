// Package schema embeds the protocol JSON Schemas so Go tests and tools can load them
// without depending on the working directory.
package schema

import "embed"

//go:embed *.schema.json
var FS embed.FS
