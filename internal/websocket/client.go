package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Joshua504/cineroom/internal/database"
	gws "github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second

	pongWait = 30 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = 4096
)

func Handle(hub *Hub, store *database.Store, userID, username string, allowedOrigins map[string]struct{}, w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("roomId")
	if roomID == "" {
		http.Error(w, "roomId is required", http.StatusBadRequest)
		return
	}
	room, err := store.RoomForMember(r.Context(), roomID, userID)
	if err != nil {
		http.Error(w, "room membership required", http.StatusForbidden)
		return
	}

	localUpgrader := upgrader
	localUpgrader.CheckOrigin = func(r *http.Request) bool { _, ok := allowedOrigins[r.Header.Get("Origin")]; return ok }
	conn, err := localUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := NewClient(hub, store, conn, roomID, userID, username)

	hub.addClient(client)
	if state, err := json.Marshal(stateMessage(room)); err == nil {
		client.send <- state
	}
	if ping, err := json.Marshal(Message{Type: "presence.ping", RoomID: roomID, PingSent: time.Now().UnixMilli()}); err == nil {
		client.send <- ping
	}

	go client.writePump()
	go client.readPump()
}

var upgrader = gws.Upgrader{
	CheckOrigin: func(*http.Request) bool { return false },
}

type Client struct {
	hub      *Hub
	store    *database.Store
	conn     *gws.Conn
	send     chan []byte
	roomID   string
	userID   string
	username string
	pingMS   atomic.Int64
	done     chan struct{}
}

func NewClient(hub *Hub, store *database.Store, conn *gws.Conn, roomID, userID, username string) *Client {
	return &Client{
		hub:      hub,
		store:    store,
		conn:     conn,
		send:     make(chan []byte, 256),
		roomID:   roomID,
		userID:   userID,
		username: username,
		done:     make(chan struct{}),
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.removeClient(c)
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)

	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return
	}

	c.pingMS.Store(-1)
	c.conn.SetPongHandler(func(payload string) error {
		if sent, err := strconv.ParseInt(payload, 10, 64); err == nil {
			c.pingMS.Store(time.Since(time.Unix(0, sent)).Milliseconds())
			go c.hub.broadcastPresence(c.roomID)
		}
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != gws.TextMessage {
			c.sendError("unsupported_message", "only JSON text messages are accepted")
			continue
		}
		var message Message
		if err := json.Unmarshal(payload, &message); err != nil {
			c.sendError("invalid_message", "invalid JSON")
			continue
		}
		c.handleMessage(message)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}

			if !ok {
				_ = c.conn.WriteMessage(gws.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(gws.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}

			payload, err := json.Marshal(Message{Type: "presence.ping", RoomID: c.roomID, PingSent: time.Now().UnixMilli()})
			if err != nil {
				return
			}
			if err := c.conn.WriteMessage(gws.TextMessage, payload); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(message Message) {
	switch message.Type {
	case "presence.pong":
		if message.PingSent <= 0 {
			return
		}
		ping := time.Now().UnixMilli() - message.PingSent
		if ping < 0 || ping > 60000 {
			return
		}
		c.pingMS.Store(ping)
		go c.hub.broadcastPresence(c.roomID)
	case "playback.play", "playback.pause", "playback.seek":
		playing := message.Type != "playback.pause"
		if message.Type == "playback.seek" {
			room, err := c.store.RoomForMember(contextBackground(), c.roomID, c.userID)
			if err != nil {
				c.sendError("forbidden", "room membership required")
				return
			}
			playing = room.Playing
		}
		room, err := c.store.UpdatePlayback(contextBackground(), c.roomID, c.userID, playing, message.Position)
		if err != nil {
			c.sendError("invalid_playback", err.Error())
			return
		}
		c.hub.broadcast(c.roomID, stateMessage(room))
	case "chat.send":
		chat, err := c.store.AddChatMessage(contextBackground(), c.roomID, c.userID, message.Text)
		if err != nil {
			c.sendError("invalid_chat", err.Error())
			return
		}
		c.hub.broadcast(c.roomID, Message{Type: "chat.message", RoomID: c.roomID, Text: chat.Body, SenderID: chat.SenderID, SenderName: chat.SenderName, UpdatedAt: chat.CreatedAt})
	default:
		c.sendError("unsupported_type", "unsupported message type")
	}
}

func stateMessage(room database.Room) Message {
	playing := room.Playing
	return Message{Type: "room.state", RoomID: room.ID, HostID: room.HostID, Position: room.Position, Playing: &playing, Locked: &room.Locked, Version: room.Version, UpdatedAt: room.UpdatedAt}
}
func (c *Client) sendError(code, text string) {
	payload, err := json.Marshal(Message{Type: "error", Code: code, Text: text})
	if err != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
		go c.close()
	}
}
func (c *Client) close() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
		_ = c.conn.Close()
	}
}

// Kept small to make persistence calls independent from the WebSocket request lifetime.
func contextBackground() context.Context { return context.Background() }
