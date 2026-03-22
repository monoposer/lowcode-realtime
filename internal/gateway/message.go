package gateway

import "encoding/json"

// ClientMessage is the structure of messages received from clients over WebSocket.
type ClientMessage struct {
	Type    string          `json:"type"`              // "subscribe", "broadcast", "presence_join", "presence_update", "presence_leave"
	Channel string          `json:"channel"`           // logical room/channel ID
	Event   string          `json:"event,omitempty"`   // event name for broadcast
	Payload json.RawMessage `json:"payload,omitempty"` // content for broadcast or presence meta
	Listen  []string        `json:"listen,omitempty"`  // for subscribe: ["broadcast","presence"]
}

// BusMessage is the internal message sent via MQ between gateway instances.
type BusMessage struct {
	Type      string          `json:"type"`                 // "broadcast", "presence_join", "presence_update", "presence_leave"
	ChannelID string          `json:"channel_id"`           // room/channel
	UserID    string          `json:"user_id,omitempty"`    // user who triggered the event
	Event     string          `json:"event,omitempty"`      // broadcast event name
	Payload   json.RawMessage `json:"payload,omitempty"`    // broadcast payload
	Meta      PresenceMeta    `json:"meta,omitempty"`       // presence metadata
	Timestamp int64           `json:"ts"`                   // Unix ms
}

// PresenceMeta is used in BusMessage for presence events.
type PresenceMeta struct {
	UserID    string                 `json:"user_id"`
	Meta      map[string]interface{} `json:"meta"`
	UpdatedAt int64                  `json:"updated_at"`
}

