package proxy

import (
	"github.com/anon55555/mt"

	"sync"
)

type PacketHandler struct {
	CltHandler func(*ClientConn, *mt.Pkt) bool
	SrvHandler func(*ServerConn, *mt.Pkt) bool
}

var packetHandlers []*PacketHandler
var packetHandlersMu sync.RWMutex

func RegisterPacketHandler(h *PacketHandler) {
	packetHandlersMu.Lock()
	defer packetHandlersMu.Unlock()

	packetHandlers = append(packetHandlers, h)
}

func handleSrvPacket(sc *ServerConn, pkt *mt.Pkt) bool {
	packetHandlersMu.RLock()
	defer packetHandlersMu.RUnlock()

	var handled bool
	for _, handler := range packetHandlers {
		if handler.SrvHandler != nil && handler.SrvHandler(sc, pkt) {
			handled = true
		}
	}

	return handled
}

func handleCltPacket(cc *ClientConn, pkt *mt.Pkt) bool {
	packetHandlersMu.RLock()
	defer packetHandlersMu.RUnlock()

	var handled bool
	for _, handler := range packetHandlers {
		if handler.CltHandler != nil && handler.CltHandler(cc, pkt) {
			handled = true
		}
	}

	return handled
}
