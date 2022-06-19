package proxy

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/anon55555/mt"
	"github.com/anon55555/mt/rudp"
)

// A ServerConn is a connection to a minetest server.
type ServerConn interface {
	client() *ClientConn
	nilClt()

	state() ClientState
	setState(ClientState)

	Init() <-chan struct{}

	Log(string, ...interface{})

	swapAOID(*mt.AOID)

	getDetachedInvs() []string
	GetMediaPool() string
	GetName() string
	Close() error

	handle()

	SendCmd(cmd mt.Cmd) (ack <-chan struct{}, err error)
}
