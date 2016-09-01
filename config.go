package websocket

import (
	"net/http"
	"time"
)

const (
	// DefaultWebsocketWriteTimeout 15 * time.Second
	DefaultWebsocketWriteTimeout = 15 * time.Second
	// DefaultWebsocketPongTimeout 60 * time.Second
	DefaultWebsocketPongTimeout = 60 * time.Second
	// DefaultWebsocketPingPeriod (DefaultPongTimeout * 9) / 10
	DefaultWebsocketPingPeriod = (DefaultWebsocketPongTimeout * 9) / 10
	// DefaultWebsocketMaxMessageSize 1024
	DefaultWebsocketMaxMessageSize = 1024
	// DefaultWebsocketReadBufferSize 4096
	DefaultWebsocketReadBufferSize = 4096
	// DefaultWebsocketWriterBufferSize 4096
	DefaultWebsocketWriterBufferSize = 4096
)

// Config the websocket server configuration
type Config struct {
	Error       func(res http.ResponseWriter, req *http.Request, status int, reason error)
	CheckOrigin func(req *http.Request) bool
	// WriteTimeout time allowed to write a message to the connection.
	// Default value is 15 * time.Second
	WriteTimeout time.Duration
	// PongTimeout allowed to read the next pong message from the connection
	// Default value is 60 * time.Second
	PongTimeout time.Duration
	// PingPeriod send ping messages to the connection with this period. Must be less than PongTimeout
	// Default value is (PongTimeout * 9) / 10
	PingPeriod time.Duration
	// MaxMessageSize max message size allowed from connection
	// Default value is 1024
	MaxMessageSize int64
	// BinaryMessages set it to true in order to denotes binary data messages instead of utf-8 text
	// compatible if you wanna use the Connection's EmitMessage to send a custom binary data to the client, like a native server-client communication.
	// defaults to false
	BinaryMessages bool
	// ReadBufferSize is the buffer size for the underline reader
	ReadBufferSize int
	// WriteBufferSize is the buffer size for the underline writer
	WriteBufferSize int
}

func (c Config) validate() Config {
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = DefaultWebsocketWriteTimeout
	}
	if c.PongTimeout <= 0 {
		c.PongTimeout = DefaultWebsocketPongTimeout
	}
	if c.PingPeriod <= 0 {
		c.PingPeriod = DefaultWebsocketPingPeriod
	}
	if c.MaxMessageSize <= 0 {
		c.MaxMessageSize = DefaultWebsocketMaxMessageSize
	}
	if c.ReadBufferSize <= 0 {
		c.ReadBufferSize = DefaultWebsocketReadBufferSize
	}
	if c.WriteBufferSize <= 0 {
		c.WriteBufferSize = DefaultWebsocketWriterBufferSize
	}
	if c.Error == nil {
		c.Error = func(res http.ResponseWriter, req *http.Request, status int, reason error) {
			//http.Error(res, reason.Error(), status)
		}
	}
	if c.CheckOrigin == nil {
		c.CheckOrigin = func(req *http.Request) bool {
			// allow all connections by default
			return true
		}
	}

	return c
}
