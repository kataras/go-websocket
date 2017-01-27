package websocket

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

// UnderlineConnection is used for compatible with fasthttp and net/http underline websocket libraries
// we only need ~8 funcs from websocket.Conn so:
type UnderlineConnection interface {
	// SetWriteDeadline sets the write deadline on the underlying network
	// connection. After a write has timed out, the websocket state is corrupt and
	// all future writes will return an error. A zero value for t means writes will
	// not time out.
	SetWriteDeadline(t time.Time) error
	// SetReadDeadline sets the read deadline on the underlying network connection.
	// After a read has timed out, the websocket connection state is corrupt and
	// all future reads will return an error. A zero value for t means reads will
	// not time out.
	SetReadDeadline(t time.Time) error
	// SetReadLimit sets the maximum size for a message read from the peer. If a
	// message exceeds the limit, the connection sends a close frame to the peer
	// and returns ErrReadLimit to the application.
	SetReadLimit(limit int64)
	// SetPongHandler sets the handler for pong messages received from the peer.
	// The appData argument to h is the PONG frame application data. The default
	// pong handler does nothing.
	SetPongHandler(h func(appData string) error)
	// WriteMessage is a helper method for getting a writer using NextWriter,
	// writing the message and closing the writer.
	WriteMessage(messageType int, data []byte) error
	// ReadMessage is a helper method for getting a reader using NextReader and
	// reading from that reader to a buffer.
	ReadMessage() (messageType int, p []byte, err error)
	// NextWriter returns a writer for the next message to send. The writer's Close
	// method flushes the complete message to the network.
	//
	// There can be at most one open writer on a connection. NextWriter closes the
	// previous writer if the application has not already done so.
	NextWriter(messageType int) (io.WriteCloser, error)
	// Close closes the underlying network connection without sending or waiting for a close frame.
	Close() error
}

// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------
// -------------------------------Connection implementation-----------------------------
// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------

type (
	// DisconnectFunc is the callback which fires when a client/connection closed
	DisconnectFunc func()
	// ErrorFunc is the callback which fires when an error happens
	ErrorFunc (func(string))
	// NativeMessageFunc is the callback for native websocket messages, receives one []byte parameter which is the raw client's message
	NativeMessageFunc func([]byte)
	// MessageFunc is the second argument to the Emitter's Emit functions.
	// A callback which should receives one parameter of type string, int, bool or any valid JSON/Go struct
	MessageFunc interface{}
	// Connection is the front-end API that you will use to communicate with the client side
	Connection interface {
		// Emitter implements EmitMessage & Emit
		Emitter
		// ID returns the connection's identifier
		ID() string

		// Request returns the (upgraded) *http.Request of this connection
		// avoid using it, you normally don't need it,
		// websocket has everything you need to authenticate the user BUT if it's necessary
		// then  you use it to receive user information, for example: from headers
		Request() *http.Request

		// OnDisconnect registers a callback which fires when this connection is closed by an error or manual
		OnDisconnect(DisconnectFunc)
		// OnError registers a callback which fires when this connection occurs an error
		OnError(ErrorFunc)
		// EmitError can be used to send a custom error message to the connection
		//
		// It does nothing more than firing the OnError listeners. It doesn't sends anything to the client.
		EmitError(errorMessage string)
		// To defines where server should send a message
		// returns an emitter to send messages
		To(string) Emitter
		// OnMessage registers a callback which fires when native websocket message received
		OnMessage(NativeMessageFunc)
		// On registers a callback to a particular event which fires when a message to this event received
		On(string, MessageFunc)
		// Join join a connection to a room, it doesn't check if connection is already there, so care
		Join(string)
		// Leave removes a connection from a room
		Leave(string)
		// Disconnect disconnects the client, close the underline websocket conn and removes it from the conn list
		// returns the error, if any, from the underline connection
		Disconnect() error
	}

	connection struct {
		underline                UnderlineConnection
		id                       string
		messageType              int
		pinger                   *time.Ticker
		disconnected             bool
		onDisconnectListeners    []DisconnectFunc
		onErrorListeners         []ErrorFunc
		onNativeMessageListeners []NativeMessageFunc
		onEventListeners         map[string][]MessageFunc
		// these were  maden for performance only
		self      Emitter // pre-defined emitter than sends message to its self client
		broadcast Emitter // pre-defined emitter that sends message to all except this
		all       Emitter // pre-defined emitter which sends message to all clients
		// httpRequest is a long-time feature request,
		// now you have access to the *http.Request which upgraded to be able to use websocket connection
		httpRequest *http.Request
		server      *server
	}
)

var _ Connection = &connection{}

func newConnection(s *server, r *http.Request, underlineConn UnderlineConnection, id string) *connection {
	c := &connection{
		underline:                underlineConn,
		id:                       id,
		messageType:              websocket.TextMessage,
		onDisconnectListeners:    make([]DisconnectFunc, 0),
		onErrorListeners:         make([]ErrorFunc, 0),
		onNativeMessageListeners: make([]NativeMessageFunc, 0),
		onEventListeners:         make(map[string][]MessageFunc, 0),
		httpRequest:              r,
		server:                   s,
	}

	if s.config.BinaryMessages {
		c.messageType = websocket.BinaryMessage
	}

	c.self = newEmitter(c, c.id)
	c.broadcast = newEmitter(c, NotMe)
	c.all = newEmitter(c, All)

	return c
}

// write writes a raw websocket message with a specific type to the client
// used by ping messages and any CloseMessage types.
func (c *connection) write(websocketMessageType int, data []byte) {
	// set the write timeout based on the configuration
	c.underline.SetWriteDeadline(time.Now().Add(c.server.config.WriteTimeout))
	// .WriteMessage same as NextWriter and close (flush)
	if err := c.underline.WriteMessage(websocketMessageType, data); err != nil {
		// if failed then the connection is off, fire the disconnect
		c.Disconnect()
	}
}

// writeDefault is the same as write but the message type is the configured by c.messageType
// if BinaryMessages is enabled then it's raw []byte as you expected to work with protobufs
func (c *connection) writeDefault(data []byte) {
	c.write(c.messageType, data)
}

func (c *connection) startPinger() {
	go func() {
		// start a new timer ticker based on the configuration
		c.pinger = time.NewTicker(c.server.config.PingPeriod)
		for {
			// wait for each tick
			<-c.pinger.C
			// try to ping the client, if failed then it disconnects
			c.write(websocket.PingMessage, []byte{})
		}
	}()
}

func (c *connection) startReader() {
	go func() {
		defer func() {
			c.Disconnect()
		}()
		conn := c.underline

		conn.SetReadLimit(c.server.config.MaxMessageSize)
		conn.SetReadDeadline(time.Now().Add(c.server.config.PongTimeout))
		conn.SetPongHandler(func(s string) error {
			conn.SetReadDeadline(time.Now().Add(c.server.config.PongTimeout))
			return nil
		})

		for {
			if _, data, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
					c.EmitError(err.Error())
				}
				break
			} else {
				c.messageReceived(data)
			}

		}
	}()

}

// messageReceived checks the incoming message and fire the nativeMessage listeners or the event listeners (ws custom message)
func (c *connection) messageReceived(data []byte) {

	if bytes.HasPrefix(data, websocketMessagePrefixBytes) {
		customData := string(data)
		//it's a custom ws message
		receivedEvt := getWebsocketCustomEvent(customData)
		listeners := c.onEventListeners[receivedEvt]
		if listeners == nil { // if not listeners for this event exit from here
			return
		}
		customMessage, err := websocketMessageDeserialize(receivedEvt, customData)
		if customMessage == nil || err != nil {
			return
		}

		for i := range listeners {
			if fn, ok := listeners[i].(func()); ok { // its a simple func(){} callback
				fn()
			} else if fnString, ok := listeners[i].(func(string)); ok {

				if msgString, is := customMessage.(string); is {
					fnString(msgString)
				} else if msgInt, is := customMessage.(int); is {
					// here if server side waiting for string but client side sent an int, just convert this int to a string
					fnString(strconv.Itoa(msgInt))
				}

			} else if fnInt, ok := listeners[i].(func(int)); ok {
				fnInt(customMessage.(int))
			} else if fnBool, ok := listeners[i].(func(bool)); ok {
				fnBool(customMessage.(bool))
			} else if fnBytes, ok := listeners[i].(func([]byte)); ok {
				fnBytes(customMessage.([]byte))
			} else {
				listeners[i].(func(interface{}))(customMessage)
			}

		}
	} else {
		// it's native websocket message
		for i := range c.onNativeMessageListeners {
			c.onNativeMessageListeners[i](data)
		}
	}

}

func (c *connection) ID() string {
	return c.id
}

func (c *connection) Request() *http.Request {
	return c.httpRequest
}

func (c *connection) fireDisconnect() {
	for i := range c.onDisconnectListeners {
		c.onDisconnectListeners[i]()
	}
}

func (c *connection) OnDisconnect(cb DisconnectFunc) {
	c.onDisconnectListeners = append(c.onDisconnectListeners, cb)
}

func (c *connection) OnError(cb ErrorFunc) {
	c.onErrorListeners = append(c.onErrorListeners, cb)
}

func (c *connection) EmitError(errorMessage string) {
	for _, cb := range c.onErrorListeners {
		cb(errorMessage)
	}
}

func (c *connection) To(to string) Emitter {
	if to == NotMe { // if send to all except me, then return the pre-defined emitter, and so on
		return c.broadcast
	} else if to == All {
		return c.all
	} else if to == c.id {
		return c.self
	}
	// is an emitter to another client/connection
	return newEmitter(c, to)
}

func (c *connection) EmitMessage(nativeMessage []byte) error {
	return c.self.EmitMessage(nativeMessage)
}

func (c *connection) Emit(event string, message interface{}) error {
	return c.self.Emit(event, message)
}

func (c *connection) OnMessage(cb NativeMessageFunc) {
	c.onNativeMessageListeners = append(c.onNativeMessageListeners, cb)
}

func (c *connection) On(event string, cb MessageFunc) {
	if c.onEventListeners[event] == nil {
		c.onEventListeners[event] = make([]MessageFunc, 0)
	}

	c.onEventListeners[event] = append(c.onEventListeners[event], cb)
}

func (c *connection) Join(roomName string) {
	c.server.Join(roomName, c.id)
}

func (c *connection) Leave(roomName string) {
	c.server.Leave(roomName, c.id)
}

func (c *connection) Disconnect() error {
	return c.server.Disconnect(c.ID())
}
