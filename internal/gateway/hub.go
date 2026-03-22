package gateway

import (
	"sync"
)

// SubscriptionType constants.
const (
	SubBroadcast = "broadcast"
	SubPresence  = "presence"
)

// Subscription describes what a client listens to on a channel.
type Subscription struct {
	ClientID  string
	ChannelID string
	Types     map[string]bool // "broadcast": true, "presence": true
}

// Hub manages all clients and subscriptions in a single gateway instance.
type Hub struct {
	mu sync.RWMutex

	clients       map[*Client]bool
	subsByChannel map[string]map[*Client]*Subscription
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		subsByChannel: make(map[string]map[*Client]*Subscription),
	}
}

// AddClient registers a client with the hub.
func (h *Hub) AddClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

// RemoveClient removes a client and all of its subscriptions.
func (h *Hub) RemoveClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, c)
	for ch, m := range h.subsByChannel {
		delete(m, c)
		if len(m) == 0 {
			delete(h.subsByChannel, ch)
		}
	}
}

// Subscribe registers a client's subscription to a channel with given types.
func (h *Hub) Subscribe(c *Client, channelID string, types []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subsByChannel[channelID]; !ok {
		h.subsByChannel[channelID] = make(map[*Client]*Subscription)
	}
	sub := &Subscription{
		ClientID:  c.ID,
		ChannelID: channelID,
		Types:     make(map[string]bool),
	}
	for _, t := range types {
		sub.Types[t] = true
	}
	h.subsByChannel[channelID][c] = sub
}

// GetSubscribers returns clients subscribed to a given channel and type.
func (h *Hub) GetSubscribers(channelID, subType string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs, ok := h.subsByChannel[channelID]
	if !ok {
		return nil
	}
	out := make([]*Client, 0, len(subs))
	for c, sub := range subs {
		if sub.Types[subType] {
			out = append(out, c)
		}
	}
	return out
}

