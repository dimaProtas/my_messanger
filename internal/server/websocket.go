package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"my_messanger/internal/config"
	jwtpkg "my_messanger/internal/pkg/jwt"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type WsServer struct {
	redisClient *redis.Client
	cfg         *config.Config
	connManager *ConnectionManager
}

func NewWsServer(redisClient *redis.Client, cfg *config.Config, connManager *ConnectionManager) *WsServer {
	return &WsServer{
		redisClient: redisClient,
		cfg:         cfg,
		connManager: connManager,
	}
}

func (ws *WsServer) ServeWs(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     ws.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to websocket: %v", err)
		return
	}

	userID, err := ws.extractUserID(r)
	if err != nil {
		log.Printf("WebSocket auth failed: %v", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication failed"))
		conn.Close()
		return
	}
	log.Printf("User %s connected via WebSocket", userID)

	ws.connManager.Register(userID, conn)
	go ws.readPump(conn, userID)
	go ws.writePump(conn, userID)
}

func (ws *WsServer) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, allowed := range ws.cfg.AllowedOrigins {
		if allowed == "*" || strings.HasPrefix(origin, strings.TrimSuffix(allowed, "*")) {
			return true
		}
	}
	return false
}

func (ws *WsServer) extractUserID(r *http.Request) (string, error) {
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		return "", fmt.Errorf("token query parameter required")
	}
	claims, err := jwtpkg.ValidateToken(tokenString, ws.cfg)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	return claims.UserID, nil
}

func (ws *WsServer) readPump(conn *websocket.Conn, userID string) {
	defer func() {
		log.Printf("User %s disconnected (readPump)", userID)
		ws.connManager.Unregister(userID, conn)
		conn.Close()
	}()
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error for user %s: %v", userID, err)
			}
			break
		}
		log.Printf("Received message from user %s: %s", userID, message)
	}
}

func (ws *WsServer) writePump(conn *websocket.Conn, userID string) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		ws.connManager.Unregister(userID, conn)
		conn.Close()
		log.Printf("User %s disconnected (writePump)", userID)
	}()

	pubsub := ws.redisClient.Subscribe(context.Background(), fmt.Sprintf("messages:%s", userID))
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case message := <-ch:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := conn.WriteMessage(websocket.TextMessage, []byte(message.Payload))
			if err != nil {
				log.Printf("WebSocket write error for user %s: %v", userID, err)
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
