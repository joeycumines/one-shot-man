package termmux

// DefaultRows is the default terminal row count.
const DefaultRows = 24

// DefaultCols is the default terminal column count.
const DefaultCols = 80

// DefaultChannelBuffer is the default buffer size for internal channels
// (request channel, merged output channel, capture output channel).
const DefaultChannelBuffer = 64

// EventBusBufferSize is the default channel buffer size for EventBus
// subscribers. Applied when Subscribe is called with bufSize < 1.
const EventBusBufferSize = 64

// PassthroughReadBufferSize is the read buffer size for passthrough
// stdin forwarding. Large enough to batch terminal input efficiently
// while keeping latency low.
const PassthroughReadBufferSize = 4096
