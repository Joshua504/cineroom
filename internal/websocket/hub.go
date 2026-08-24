package websocket

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/Joshua504/cineroom/internal/database"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]struct{}),
	}
}

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.roomID] == nil {
		h.clients[client.roomID] = make(map[*Client]struct{})
	}
	h.clients[client.roomID][client] = struct{}{}
	go h.broadcastPresence(client.roomID)
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.clients[client.roomID]
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.clients, client.roomID)
	}
	go h.broadcastPresence(client.roomID)
}

func (h *Hub) broadcastPresence(roomID string) {
	h.broadcast(roomID, Message{Type: "presence.update", RoomID: roomID, Users: h.participants(roomID)})
}
func (h *Hub) participants(roomID string) []Participant {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := map[string]Participant{}
	for c := range h.clients[roomID] {
		ping := c.pingMS.Load()
		old, ok := users[c.userID]
		if !ok || (ping >= 0 && (old.PingMS < 0 || ping < old.PingMS)) {
			users[c.userID] = Participant{UserID: c.userID, Username: c.username, PingMS: ping}
		}
	}
	result := make([]Participant, 0, len(users))
	for _, user := range users {
		result = append(result, user)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Username < result[j].Username })
	return result
}

func (h *Hub) broadcast(roomID string, message Message) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients[roomID] {
		select {
		case client.send <- payload:
		default:
			go client.close()
		}
	}
}

func (h *Hub) DisconnectMember(roomID, userID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[roomID] {
		if client.userID == userID {
			client.sendError("kicked", "You were removed from this room.")
			go client.close()
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*Client, 0)
	for _, roomClients := range h.clients {
		for client := range roomClients {
			clients = append(clients, client)
		}
	}
	h.clients = make(map[string]map[*Client]struct{})
	h.mu.Unlock()
	for _, client := range clients {
		client.close()
	}
}

func (h *Hub) BroadcastRoomState(room database.Room) {
	h.broadcast(room.ID, stateMessage(room))
}
