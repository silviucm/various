package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

/*
websocket.go: Websocket connectivity functionality
*/

// Websocket constants section
const (

	// Control messages constants
	MessageGetTestStatus = "getStatus"
	MessageCancelTest    = "cancelTest"

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size: we should only allow 4 Kilobytes
	maxMessageSize int64 = 4096
)

// Use default options for the Websocket upgrader
var upgrader = websocket.Upgrader{}

// GetAttackStatus handles to request to get the attack status.
// This request gets upgraded to websocket.
func GetAttackStatus(w http.ResponseWriter, r *http.Request) {

	// upgrade to websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		return
	}

	// Initialize a new connection wrapper struct that will hold this websocket
	// and a channel for outgoing messages
	c := &WsConnection{Send: make(chan []byte, 256), WS: ws}

	// Register the connection with the vegeta attack singleton structure
	log.Println("Registering the connection with the vegeta instance connection registry...")
	instance.Register <- c

	// The write pump for this websocket connection goes in a separate go routine
	log.Println("Initializing the connection write pump...")
	go c.WritePump()

	// Start the ReadPump right here, and keep this upgrade connection open
	// for any messages from the client
	log.Println("Entering the read pump loop...")
	c.ReadPump()
}

// connection is a middleman between the websocket connection and the hub.
type WsConnection struct {
	// The websocket connection.
	WS *websocket.Conn

	// Buffered channel of outbound messages.
	Send chan []byte
}

// ReadPump pumps messages from the websocket connection to the hub.
func (c *WsConnection) ReadPump() {

	// Ensure that we signal to the Vegeta instance to unregister the connection
	defer func() {
		instance.Unregister <- c
		c.WS.Close()
	}()

	c.WS.SetReadLimit(maxMessageSize)
	c.WS.SetReadDeadline(time.Now().Add(pongWait))

	c.WS.SetPongHandler(func(string) error { c.WS.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.WS.ReadMessage()
		if err != nil {
			break
		}

		// we need to make sure we only send the message if there is an ongoing attack
		// and if the message is a valid one
		okToSend := true

		if instance.getStatus() != AttackStatusCodeInProgress {
			okToSend = false
		}

		// only getStatus and
		okToSend = (string(message) == MessageGetTestStatus || string(message) == MessageCancelTest)

		// send the message if ok to send
		if okToSend {
			instance.Incoming <- message
		}

	}
}

// write writes a message with the given message type and payload.
func (c *WsConnection) write(mt int, payload []byte) error {
	c.WS.SetWriteDeadline(time.Now().Add(writeWait))
	return c.WS.WriteMessage(mt, payload)
}

// The WritePump method pumps messages from the hub to the websocket connection.
// As long as the connection is open, this will keep running and deliver messages to the client
func (c *WsConnection) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.WS.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}
