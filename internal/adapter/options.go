package adapter

// StartOpts controls interactive vs non-interactive start.
type StartOpts struct {
	// Yes skips prompts and uses saved config / defaults (like npm -y).
	Yes bool
}
