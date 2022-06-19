package proxy

import (
	"errors"
	"log"
	"net"
	"sync"

	"github.com/anon55555/mt"
	"github.com/anon55555/mt/rudp"
)

type ClientState uint8

const (
	CsCreated ClientState = iota
	CsInit
	CsActive
	CsSudo
)

// A ClientConn is a connection to a minetest client.
type ClientConn struct {
	mt.Peer
	srv ServerConn
	mu  sync.RWMutex

	logger *log.Logger

	cstate   ClientState
	cstateMu sync.RWMutex
	name     string
	initCh   chan struct{}
	aoReady  sync.Once
	hopMu    sync.Mutex

	auth struct {
		method                       mt.AuthMethods
		salt, srpA, srpB, srpM, srpK []byte
	}

	lang string

	major, minor, patch uint8
	reservedVer         uint8
	versionStr          string
	formspecVer         uint16

	itemDefs []mt.ItemDef
	aliases  []struct{ Alias, Orig string }
	nodeDefs []mt.NodeDef
	p0Map    param0Map
	p0SrvMap param0SrvMap
	media    []MediaFile

	playerCAO, currentCAO mt.AOID

	playerListInit bool

	modChs   map[string]struct{}
	modChsMu sync.RWMutex
}

// Name returns the player name of the ClientConn.
func (cc *ClientConn) Name() string { return cc.name }

func (cc *ClientConn) server() ServerConn {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.srv
}

// ServerName returns the name of the current upstream server
// of the ClientConn. It is empty if there is no upstream connection.
func (cc *ClientConn) ServerName() string {
	srv := cc.server()
	if srv != nil {
		return srv.GetName()
	}

	return ""
}

func (cc *ClientConn) state() ClientState {
	cc.cstateMu.RLock()
	defer cc.cstateMu.RUnlock()

	return cc.cstate
}

func (cc *ClientConn) setState(state ClientState) {
	cc.cstateMu.Lock()

	oldState := cc.cstate
	cc.cstate = state

	cc.cstateMu.Unlock()

	if oldState != state {
		handleClientStateChange(cc, oldState, state)
	}
}

// Init returns a channel that is closed
// when the ClientConn enters the csActive state.
func (cc *ClientConn) Init() <-chan struct{} { return cc.initCh }

// Log logs an interaction with the ClientConn.
// dir indicates the direction of the interaction.
func (cc *ClientConn) Log(dir string, v ...interface{}) {
	cc.logger.Println(append([]interface{}{dir}, v...)...)
}

func handleClt(cc *ClientConn) {
	for {
		pkt, err := cc.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				if errors.Is(cc.WhyClosed(), rudp.ErrTimedOut) {
					cc.Log("<->", "timeout")
				} else {
					handleClientLeave(cc, &Leave{
						Type: Exit,
					})
					cc.Log("<->", "disconnect")
				}

				if cc.Name() != "" {
					playersMu.Lock()
					delete(players, cc.Name())
					playersMu.Unlock()
				}

				if cc.server() != nil {
					cc.server().Close()

					cc.server().nilClt()
					
					cc.mu.Lock()
					cc.srv = nil
					cc.mu.Unlock()
				}

				break
			}

			cc.Log("->", err)
			continue
		}

		cc.process(pkt)
	}
}
