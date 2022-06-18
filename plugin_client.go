package proxy

import (
	"github.com/anon55555/mt"
	"sync"
)

type LeaveType uint8

const (
	Exit LeaveType = iota
	Kick
)

type Leave struct{
	Type LeaveType
	Kick *mt.ToCltKick
}

type ClientHandler struct{
	Join        func(cc *ClientConn) (destination string)
	StateChange func(cc *ClientConn, oldState, state ClientState)
	Leave       func(cc *ClientConn, l *Leave)
	Hop         func(cc *ClientConn, source, destination string)
}

var clientHandlers   []*ClientHandler
var clientHandlersMu sync.RWMutex

func RegisterClientHandler(h *ClientHandler) {
	clientHandlersMu.Lock()
	defer clientHandlersMu.Unlock()

	clientHandlers = append(clientHandlers, h)
}

func handleClientStateChange(cc *ClientConn, oldState, state ClientState) {
	clientHandlersMu.RLock()
	defer clientHandlersMu.RUnlock()

	for _, handler := range clientHandlers {
		if handler.StateChange != nil {
			handler.StateChange(cc, oldState, state)
		}
	}
}

func handleClientJoin(cc *ClientConn) string {
	clientHandlersMu.RLock()
	defer clientHandlersMu.RUnlock()

	var dest string

	for _, handler := range clientHandlers {
		if handler.Join != nil {
			if d := handler.Join(cc); d != "" {
				dest = d
			}
		}
	}

	return dest
}

func handleClientLeave(cc *ClientConn, l *Leave) {
	clientHandlersMu.RLock()
	defer clientHandlersMu.RUnlock()

	for _, handler := range clientHandlers {
		if handler.Leave != nil {
			handler.Leave(cc, l)
		}
	}	
}

func handleClientHop(cc *ClientConn, source, leave string) {
	clientHandlersMu.RLock()
	defer clientHandlersMu.RUnlock()

	for _, handler := range clientHandlers {
		if handler.Hop != nil {
			handler.Hop(cc, source, leave)
		}
	}		
}
