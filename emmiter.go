package websocket

// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------
// --------------------------------Emmiter implementation-------------------------------
// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------

const (
	// All is the string which the Emmiter use to send a message to all
	All = ""
)

type (
	// Emmiter is the message/or/event manager
	Emmiter interface {
		// EmitMessage sends a native websocket message
		EmitMessage([]byte) error
		// Emit sends a message on a particular event
		Emit(string, interface{}) error
	}

	emmiter struct {
		server *server
		to     string
	}
)

var _ Emmiter = &emmiter{}

func newEmmiter(server *server, to string) *emmiter {
	return &emmiter{server: server, to: to}
}

func (e *emmiter) EmitMessage(nativeMessage []byte) error {
	mp := websocketMessagePayload{e.to, nativeMessage}
	e.server.messages <- mp
	return nil
}

func (e *emmiter) Emit(event string, data interface{}) error {
	message, err := websocketMessageSerialize(event, data)
	if err != nil {
		return err
	}
	e.EmitMessage([]byte(message))
	return nil
}
