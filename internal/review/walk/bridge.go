// The embedded nvim bridge: review.lua frozen as the RPC contract the
// controller drives (ReviewGo, ReviewCommit, ReviewNote) plus
// ReviewCapabilities for negotiation.
package walk

import (
	_ "embed"
)

//go:embed bridge/review.lua
var BridgeLua string

// BridgeVersion is the contract version of the embedded bridge. Bump on a
// breaking change to ReviewGo/ReviewCommit/ReviewNote/ReviewCapabilities.
const BridgeVersion = 1
