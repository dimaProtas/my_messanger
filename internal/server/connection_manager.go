package server

import (
	"sync"

	"github.com/gorilla/websocket"
)

type ConnectionManager struct {
	mu    sync.RWMutex
	conns map[string]map[*websocket.Conn]bool
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		conns: make(map[string]map[*websocket.Conn]bool),
	}
}

func (cm *ConnectionManager) Register(userID string, conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.conns[userID] == nil {
		cm.conns[userID] = make(map[*websocket.Conn]bool)
	}
	cm.conns[userID][conn] = true
}

func (cm *ConnectionManager) Unregister(userID string, conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if conns, ok := cm.conns[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(cm.conns, userID)
		}
	}
}

func (cm *ConnectionManager) GetConnections(userID string) []*websocket.Conn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	conns := cm.conns[userID]
	result := make([]*websocket.Conn, 0, len(conns))
	for conn := range conns {
		result = append(result, conn)
	}
	return result
}

func (cm *ConnectionManager) IsOnline(userID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.conns[userID]) > 0
}

func (cm *ConnectionManager) OnlineCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	count := 0
	for _, conns := range cm.conns {
		count += len(conns)
	}
	return count
}
