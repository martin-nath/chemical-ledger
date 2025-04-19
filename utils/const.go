package utils

import "time"

const (
	MaxRetries   = 3
	RetryDelay   = 100 * time.Millisecond
	ScaleMg      = "mg"
	ScaleMl      = "ml"
	TypeIncoming = "incoming"
	TypeOutgoing = "outgoing"
	TypeBoth     = "both"
)
