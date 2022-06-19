package proxy

import (
	"github.com/anon55555/mt"

	"log"
	"sync"
	"errors"
)

type UDPServer struct {
	Address string

	MediaPool string
	Fallbacks []string

	dynamic bool
}

func (s UDPServer) GetFallbacks() []string {
	return s.Fallbacks
}

func (s UDPServer) GetMediaPool() string {
	return s.MediaPool
}

func (s UDPServer) isDynamic() bool {
	return s.dynamic
}

func (s UDPServer) contentConn() (contentConn, error) {
	addr, err = net.ResolveUDPAddr("udp", srv.Addr)
	if err != nil {
		return nil, err
	}

	var conn *net.UDPConn
	conn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	var cc *contentConn

	logPrefix := fmt.Sprintf("[content %s] ", name)
	cc := &contentConnUDP{
		Peer:      mt.Connect(conn),
		logger:    log.New(logWriter, logPrefix, log.LstdFlags|log.Lmsgprefix),
		doneCh:    make(chan struct{}),
		name:      name,
		userName:  userName,
		mediaPool: mediaPool,
	}

	if err := cc.addDefaultTextures(); err != nil {
		return nil, err
	}

	go handleContent(cc)
	
	defer cc.Close()

	conns = append(conns, cc)
}

type ServerConnUDP struct {
	mt.Peer
	clt *ClientConn
	mu  sync.RWMutex

	logger *log.Logger

	cstate   ClientState
	cstateMu sync.RWMutex
	name     string
	initCh   chan struct{}

	auth struct {
		method              mt.AuthMethods
		salt, srpA, a, srpK []byte
	}

	mediaPool string

	inv          mt.Inv
	detachedInvs []string

	aos              map[mt.AOID]struct{}
	particleSpawners map[mt.ParticleSpawnerID]struct{}

	sounds map[mt.SoundID]struct{}

	huds map[mt.HUDID]mt.HUDType

	playerList map[string]struct{}
}

func (s *ServerConnUDP) getDetachedInvs() []string {
	return s.detachedInvs
}

func (s *ServerConnUDP) nilClt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.clt = nil
}

func (s *ServerConnUDP) swapAOID(ao *mt.AOID) {
	if sc.client() != nil {
		if *ao == sc.client().playerCAO {
			*ao = sc.client().currentCAO
		} else if *ao == sc.client().currentCAO {
			*ao = sc.client().playerCAO
		}
	}
}

func (s *ServerConnUDP) GetMediaPool() string {
	return s.MediaPool
}

func (s *ServerConnUDP) GetName() string {
	return s.Name
}

func (sc *ServerConnUDP) client() *ClientConn {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.clt
}

func (sc *ServerConnUDP) state() ClientState {
	sc.cstateMu.RLock()
	defer sc.cstateMu.RUnlock()

	return sc.cstate
}

func (sc *ServerConnUDP) setState(state ClientState) {
	sc.cstateMu.Lock()

	oldState := sc.cstate
	sc.cstate = state

	sc.cstateMu.Unlock()

	if oldState != state {
		handleServerStateChange(sc, oldState, state)
	}
}

// Init returns a channel that is closed
// when the ServerConn enters the csActive state.
func (sc *ServerConnUDP) Init() <-chan struct{} { return sc.initCh }

// Log logs an interaction with the ServerConn.
// dir indicates the direction of the interaction.
func (sc *ServerConnUDP) Log(dir string, v ...interface{}) {
	sc.logger.Println(append([]interface{}{dir}, v...)...)
}

func (sc *ServerConnUDP) handle() {
	go func() {
		init := make(chan struct{})
		defer close(init)

		go func(init <-chan struct{}) {
			select {
			case <-init:
			case <-time.After(10 * time.Second):
				sc.Log("->", "timeout")
				sc.Close()
			}
		}(init)

		for sc.state() == CsCreated && sc.client() != nil {
			sc.SendCmd(&mt.ToSrvInit{
				SerializeVer: serializeVer,
				MinProtoVer:  protoVer,
				MaxProtoVer:  protoVer,
				PlayerName:   sc.client().Name(),
			})
			time.Sleep(500 * time.Millisecond)
		}
	}()

	for {
		pkt, err := sc.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				if errors.Is(sc.WhyClosed(), rudp.ErrTimedOut) {
					sc.Log("<->", "timeout")
				} else {
					sc.Log("<->", "disconnect")
				}

				if sc.client() != nil {
					ack, _ := sc.client().SendCmd(&mt.ToCltKick{
						Reason: mt.Custom,
						Custom: "Server connection closed unexpectedly.",
					})

					select {
					case <-sc.client().Closed():
					case <-ack:
						sc.client().Close()

						sc.client().mu.Lock()
						sc.client().srv = nil
						sc.client().mu.Unlock()

						sc.mu.Lock()
						sc.clt = nil
						sc.mu.Unlock()
					}
				}

				break
			}

			sc.Log("<-", err)
			continue
		}

		sc.process(pkt)
	}
}
