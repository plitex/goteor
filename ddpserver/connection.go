package ddpserver

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Protocol version
	protocolVersion = "1"

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Connection is a middleman between the websocket connection and the server.
type Connection struct {
	server *Server

	// The websocket connection.
	conn *websocket.Conn

	// Connected when handshake is successful
	connected bool

	// Session ID
	sessionID string

	// Buffered channel of outbound messages.
	send chan []byte

	// Client subscriptions
	subscriptions map[string]*PublicationContext

	// UserID of logged user
	userID string
}

func (c *Connection) run() {
	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go c.writePump()
	go c.readPump()

	// Send server ID message
	c.sendServerID()
}

// readPump pumps messages from the websocket connection to the server.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Connection) readPump() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	for {
		_, p, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("error: %v", err)
			}
			break
		}
		fmt.Printf("recv: %s", p)

		// Decode message
		var m Message
		if err := json.Unmarshal(p, &m); err != nil {
			fmt.Printf("error: %v", err)
			break
		}
		switch m.Msg {
		case "connect":
			c.handleConnect(m)
		case "ping":
			c.handlePing(m)
		case "pong":
			c.handlePong(m)
		case "method":
			c.handleMethod(m)
		case "sub":
			c.handleSubscribe(m)
		case "unsub":
			c.handleUnsubscribe(m)
		default:
			fmt.Println("error: Unknown message received from client")
		}
	}
	fmt.Println("Client disconnected")
}

// writePump pumps messages from the server to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The server closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			fmt.Printf("sent: %s\n", message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.sendPing()
		}
	}
}

func (c *Connection) writeMessage(msg interface{}) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.send <- b
	return nil
}

func (c *Connection) handleConnect(msg Message) {
	if msg.Version != protocolVersion {
		c.sendFailed()
		c.conn.Close()
		return
	}
	c.sessionID = generateID(17)
	c.sendConnected()
}

func (c *Connection) handlePing(msg Message) {
	c.sendPong(msg.ID)
}

func (c *Connection) handlePong(msg Message) {
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
}

func (c *Connection) handleMethod(msg Message) {
	ctx := NewMethodContext(msg, *c)

	fn, ok := c.server.methods[msg.Method]
	if !ok {
		c.sendMethodError(&ctx, &Error{
			Type:    "Method.Error",
			Error:   "unknown-method",
			Reason:  fmt.Sprintf("Method '%s' not found", msg.Method),
			Message: fmt.Sprintf("Method '%s' not found [unknown]", msg.Method),
		})
		return
	}

	r, err := fn(ctx)
	if err != nil {
		c.sendMethodError(&ctx, err)
		return
	}
	c.sendMethodResult(&ctx, r)

	// TODO: Update publications
	c.sendMethodUpdated(&ctx)
}

func (c *Connection) handleSubscribe(msg Message) {
	ctx := NewPublicationContext(msg, *c)

	id := generateID(17)

	c.subscriptions[id] = ctx

	handler, ok := c.server.publications[msg.Name]
	if !ok {
		c.sendSubscriptionError(ctx, &Error{
			Type:    "Server.Error",
			Error:   "unknown-subscription",
			Reason:  fmt.Sprintf("Subscription '%s' not found", msg.Name),
			Message: fmt.Sprintf("Subscription '%s' not found [unknown-subscription]", msg.Name),
		})
		return
	}

	handler(*ctx)
}

func (c *Connection) handleUnsubscribe(msg Message) {
	_, ok := c.subscriptions[msg.ID]
	if !ok {
		msg := Message{
			Msg: "nosub",
			ID:  msg.ID,
			Error: Error{
				Type:    "Server.Error",
				Error:   "unknown-subscription",
				Reason:  fmt.Sprintf("Subscription ID '%s' not found", msg.ID),
				Message: fmt.Sprintf("Subscription ID '%s' not found [unknown-subscription]", msg.ID),
			},
		}
		c.writeMessage(msg)
		return
	}

	delete(c.subscriptions, msg.ID)

	// Update subscriptions
}

func (c *Connection) sendServerID() {
	msg := map[string]string{
		"server_id": c.server.id,
	}
	c.writeMessage(msg)
}

func (c *Connection) sendConnected() {
	msg := map[string]string{
		"msg":     "connected",
		"session": c.sessionID,
	}
	c.writeMessage(msg)
}

func (c *Connection) sendFailed() {
	msg := map[string]string{
		"msg":     "failed",
		"version": protocolVersion,
	}
	c.writeMessage(msg)
}

func (c *Connection) sendPing() {
	msg := map[string]string{
		"msg": "ping",
	}
	c.writeMessage(msg)
}

func (c *Connection) sendPong(id string) {
	msg := map[string]string{
		"msg": "pong",
	}
	if id != "" {
		msg["id"] = id
	}
	c.writeMessage(msg)
}

func (c *Connection) sendMethodResult(ctx *MethodContext, result interface{}) error {
	ctx.done = true
	msg := map[string]interface{}{
		"msg":    "result",
		"id":     ctx.ID,
		"result": result,
	}
	return c.writeMessage(msg)
}

func (c *Connection) sendMethodError(ctx *MethodContext, e *Error) error {
	ctx.done = true
	msg := map[string]interface{}{
		"msg":   "result",
		"id":    ctx.ID,
		"error": *e,
	}
	return c.writeMessage(msg)
}

func (c *Connection) sendMethodUpdated(ctx *MethodContext) error {
	ctx.updated = true
	msg := map[string]interface{}{
		"msg":     "updated",
		"methods": []string{ctx.ID},
	}
	return c.writeMessage(msg)
}

func (c *Connection) sendSubscriptionError(ctx *PublicationContext, e *Error) error {
	msg := map[string]interface{}{
		"msg":   "nosub",
		"id":    ctx.ID,
		"error": *e,
	}
	return c.writeMessage(msg)
}

func (c *Connection) sendSubscriptionReady(ctx *PublicationContext) error {
	ctx.ready = true

	msg := map[string]interface{}{
		"msg":  "ready",
		"subs": []string{ctx.ID},
	}
	return c.writeMessage(msg)
}
