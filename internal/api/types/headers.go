package types

// The X-PinchTab-* wire headers that form the CLI/MCP-to-server contract. They
// live here, the leaf both clients and the server import, so a one-character
// drift on one side is a compile error rather than a silent no-op. Request-side
// tags travel in the body instead, because the strip middleware drops inbound
// X-PinchTab-* headers from public clients; these are the response headers
// (Vocab, Tab-Id) and the source tag the trusted hops set.
const (
	HeaderVocab  = "X-PinchTab-Vocab"
	HeaderTabID  = "X-PinchTab-Tab-Id"
	HeaderSource = "X-PinchTab-Source"
)

// The IDPI response headers warning a client that the content or domain it
// asked for matched a prompt-injection pattern without being blocked.
const (
	HeaderIDPIWarning = "X-IDPI-Warning"
	HeaderIDPIPattern = "X-IDPI-Pattern"
)
