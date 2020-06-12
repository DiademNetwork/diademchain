package events

import (
	"github.com/diademnetwork/diademchain"
	log "github.com/diademnetwork/diademchain/log"
)

// LogEventDispatcher just logs events
type LogEventDispatcher struct {
}

var _ diademchain.EventDispatcher = &LogEventDispatcher{}

// NewLogEventDispatcher create a new redis dispatcher
func NewLogEventDispatcher() *LogEventDispatcher {
	return &LogEventDispatcher{}
}

// Send sends the event
func (ed *LogEventDispatcher) Send(index uint64, eventIdex int, msg []byte) error {
	log.Info("Event emitted", "index", index, "length", len(msg), "msg", string(msg))
	return nil
}

func (ed *LogEventDispatcher) Flush() {
}
