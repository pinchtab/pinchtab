package mcp

import "github.com/mark3labs/mcp-go/mcp"

func typedArgsOf(tool mcp.Tool) map[string]string {
	return typedOnly(propertiesOf(tool))
}
