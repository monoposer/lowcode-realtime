package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"github.com/example/lowcode-realtime/internal/broker"
	"github.com/example/lowcode-realtime/internal/presence"
)

// Server bundles WebSocket gateway + HTTP broadcast API.
type Server struct {
	hub      *Hub
	upgrader websocket.Upgrader

	broker  broker.MessageBroker
	store   presence.Store
	ttl     time.Duration
	redis   *redis.Client
	baseCtx context.Context
	jwtAuth *JWTAuth // optional; when set, WebSocket requires a valid JWT and user id comes from claims
}

// NewServer creates a new realtime server.
// jwtAuth may be nil: then WebSocket accepts user_id via query (dev / playground).
// When jwtAuth is non-nil with JWT_SECRET set in main, connections must send a valid HS256 JWT.
func NewServer(b broker.MessageBroker, rdb *redis.Client, ttl time.Duration, jwtAuth *JWTAuth) *Server {
	hub := NewHub()
	s := &Server{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		broker:  b,
		store:   presence.NewRedisStore(rdb, "presence:"),
		ttl:     ttl,
		redis:   rdb,
		baseCtx: context.Background(),
		jwtAuth: jwtAuth,
	}
	go s.runBroadcastConsumer()
	go s.runPresenceConsumer()
	return s
}

// RegisterRoutes registers HTTP and WebSocket routes on the router.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/ws", s.handleWebSocket)
	r.POST("/api/event", s.handleHTTPEvent)
}

// handleWebSocket handles client WebSocket connections.
func (s *Server) handleWebSocket(c *gin.Context) {
	userID, err := s.resolveWebSocketUserID(c)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrMissingToken) || errors.Is(err, ErrInvalidToken) {
			status = http.StatusUnauthorized
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	serveWs(s.hub, s.upgrader, c.Writer, c.Request, userID, s.handleClientMessage)
}

// resolveWebSocketUserID returns the authenticated user id: from JWT when jwtAuth is configured,
// otherwise from the user_id query parameter (legacy / local dev).
func (s *Server) resolveWebSocketUserID(c *gin.Context) (string, error) {
	jwtOn := s.jwtAuth != nil && len(s.jwtAuth.Secret) > 0
	hasTok := HasTokenInRequest(c.Request)
	qUser := c.Query("user_id")

	log.Printf("ws/auth: jwt_enabled=%v has_token=%v has_user_id_query=%v remote=%s",
		jwtOn, hasTok, qUser != "", c.ClientIP())

	if jwtOn {
		uid, err := s.jwtAuth.ParseUserIDFromRequest(c.Request)
		if err != nil {
			log.Printf("ws/auth: jwt path failed: %v", err)
		} else {
			log.Printf("ws/auth: jwt ok user_id=%q", uid)
		}
		return uid, err
	}

	// JWT off: ?token= is ignored unless JWT_SECRET is set — explain instead of generic missing user_id.
	if hasTok {
		err := fmt.Errorf("JWT is not enabled (JWT_SECRET unset): pass user_id=... in the WebSocket URL, or set JWT_SECRET so ?token= is verified")
		log.Printf("ws/auth: %v", err)
		return "", err
	}
	if qUser == "" {
		err := fmt.Errorf("missing user_id")
		log.Printf("ws/auth: %v", err)
		return "", err
	}
	log.Printf("ws/auth: legacy user_id=%q", qUser)
	return qUser, nil
}

// handleClientMessage processes messages from a client.
func (s *Server) handleClientMessage(ctx context.Context, c *Client, raw []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Printf("invalid client message: %v", err)
		return
	}

	switch msg.Type {
	case "subscribe":
		s.hub.Subscribe(c, msg.Channel, msg.Listen)
		// If presence is requested, sync current state.
		for _, t := range msg.Listen {
			if t == SubPresence {
				s.sendPresenceSync(c, msg.Channel)
				break
			}
		}

	case "broadcast":
		s.handleClientBroadcast(ctx, c, msg)

	case "presence_join", "presence_update":
		s.handlePresenceUpsert(ctx, c, msg)

	case "presence_leave":
		s.handlePresenceLeave(ctx, c, msg)
	}
}

func (s *Server) handleClientBroadcast(ctx context.Context, c *Client, msg ClientMessage) {
	bus := BusMessage{
		Type:      "broadcast",
		ChannelID: msg.Channel,
		UserID:    c.UserID,
		Event:     msg.Event,
		Payload:   msg.Payload,
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(bus)
	if err != nil {
		log.Printf("marshal broadcast failed: %v", err)
		return
	}
	if err := s.broker.PublishBroadcast(ctx, msg.Channel, data); err != nil {
		log.Printf("publish broadcast failed: %v", err)
	}
}

func (s *Server) handlePresenceUpsert(ctx context.Context, c *Client, msg ClientMessage) {
	var metaMap map[string]interface{}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &metaMap); err != nil {
			log.Printf("invalid presence meta: %v", err)
			return
		}
	}

	stored, err := s.store.Upsert(ctx, msg.Channel, c.UserID, metaMap, s.ttl)
	if err != nil {
		log.Printf("presence upsert failed: %v", err)
		return
	}
	bus := BusMessage{
		Type:      msg.Type,
		ChannelID: msg.Channel,
		UserID:    c.UserID,
		Meta: PresenceMeta{
			UserID:    stored.UserID,
			Meta:      stored.Meta,
			UpdatedAt: stored.UpdatedAt,
		},
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(bus)
	if err != nil {
		log.Printf("marshal presence failed: %v", err)
		return
	}
	if err := s.broker.PublishPresenceEvent(ctx, msg.Channel, data); err != nil {
		log.Printf("publish presence failed: %v", err)
	}
}

func (s *Server) handlePresenceLeave(ctx context.Context, c *Client, msg ClientMessage) {
	if err := s.store.Remove(ctx, msg.Channel, c.UserID); err != nil {
		log.Printf("presence remove failed: %v", err)
	}
	bus := BusMessage{
		Type:      "presence_leave",
		ChannelID: msg.Channel,
		UserID:    c.UserID,
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(bus)
	if err != nil {
		log.Printf("marshal presence leave failed: %v", err)
		return
	}
	if err := s.broker.PublishPresenceEvent(ctx, msg.Channel, data); err != nil {
		log.Printf("publish presence leave failed: %v", err)
	}
}

func (s *Server) sendPresenceSync(c *Client, channelID string) {
	metas, err := s.store.List(s.baseCtx, channelID)
	if err != nil {
		log.Printf("presence list failed: %v", err)
		return
	}
	ev := struct {
		Type    string          `json:"type"`
		Channel string          `json:"channel"`
		Users   []presence.Meta `json:"users"`
	}{
		Type:    "presence_sync",
		Channel: channelID,
		Users:   metas,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// handleHTTPEvent receives external event (broadcast) requests and publishes them to MQ.
func (s *Server) handleHTTPEvent(c *gin.Context) {
	var req struct {
		Channel string          `json:"channel" binding:"required"`
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Event == "" {
		req.Event = "message"
	}

	bus := BusMessage{
		Type:      "broadcast",
		ChannelID: req.Channel,
		Event:     req.Event,
		Payload:   req.Payload,
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(bus)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal failed"})
		return
	}
	if err := s.broker.PublishBroadcast(c, req.Channel, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "publish failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"message_id": time.Now().UnixNano(),
	})
}

// runBroadcastConsumer consumes broadcast messages from MQ and forwards to local subscribers.
func (s *Server) runBroadcastConsumer() {
	ch, err := s.broker.SubscribeBroadcast(s.baseCtx, "*")
	if err != nil {
		log.Printf("subscribe broadcast failed: %v", err)
		return
	}
	for msg := range ch {
		var bus BusMessage
		if err := json.Unmarshal(msg.Data, &bus); err != nil {
			log.Printf("invalid bus broadcast: %v", err)
			continue
		}
		s.deliverBroadcast(bus)
	}
}

// runPresenceConsumer consumes presence events from MQ and forwards to local subscribers.
func (s *Server) runPresenceConsumer() {
	ch, err := s.broker.SubscribePresenceEvent(s.baseCtx, "*")
	if err != nil {
		log.Printf("subscribe presence failed: %v", err)
		return
	}
	for msg := range ch {
		var bus BusMessage
		if err := json.Unmarshal(msg.Data, &bus); err != nil {
			log.Printf("invalid bus presence: %v", err)
			continue
		}
		s.deliverPresence(bus)
	}
}

func (s *Server) deliverBroadcast(bus BusMessage) {
	ev := struct {
		Type      string          `json:"type"`
		Channel   string          `json:"channel"`
		Event     string          `json:"event"`
		Payload   json.RawMessage `json:"payload"`
		UserID    string          `json:"user_id,omitempty"`
		Timestamp int64           `json:"ts"`
	}{
		Type:      "broadcast",
		Channel:   bus.ChannelID,
		Event:     bus.Event,
		Payload:   bus.Payload,
		UserID:    bus.UserID,
		Timestamp: bus.Timestamp,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	clients := s.hub.GetSubscribers(bus.ChannelID, SubBroadcast)
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (s *Server) deliverPresence(bus BusMessage) {
	ev := struct {
		Type      string                 `json:"type"`
		Channel   string                 `json:"channel"`
		UserID    string                 `json:"user_id"`
		Meta      map[string]interface{} `json:"meta,omitempty"`
		UpdatedAt int64                  `json:"updated_at,omitempty"`
		Timestamp int64                  `json:"ts"`
	}{
		Type:      bus.Type,
		Channel:   bus.ChannelID,
		UserID:    bus.UserID,
		Meta:      bus.Meta.Meta,
		UpdatedAt: bus.Meta.UpdatedAt,
		Timestamp: bus.Timestamp,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	clients := s.hub.GetSubscribers(bus.ChannelID, SubPresence)
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
		}
	}
}
