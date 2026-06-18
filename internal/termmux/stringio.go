package termmux

// StringIO is an interface compatible with string-based agent handles
// (e.g., aimux.AgentHandle). Use [NewStringIOSession] to adapt it
// to [InteractiveSession] for use with [SessionManager.Register].
type StringIO interface {
	Send(input string) error
	Receive() (string, error)
	Close() error
}
