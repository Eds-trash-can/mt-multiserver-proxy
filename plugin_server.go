package proxy

import (
	"github.com/anon55555/mt"
)

type PluginServer interface {
	ProcessClt(cc *ClientConn, pkt mt.Pkt)
	ProcessSrv(sc *ServerConn, pkt mt.Pkt)

	Name() string

	clt() *ClientConn
}
