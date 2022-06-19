package proxy

import (
	"github.com/anon55555/mt"

	"log"
	"sync"
)

type contentConnUDP struct {
	mt.Peer

	logger *log.Logger

	cstate         ClientState
	cstateMu       sync.RWMutex
	name, userName string
	doneCh         chan struct{}

	auth struct {
		method              mt.AuthMethods
		salt, srpA, a, srpK []byte
	}

	mediaPool string

	itemDefs []mt.ItemDef
	aliases  []struct{ Alias, Orig string }

	nodeDefs []mt.NodeDef

	media   []MediaFile
	remotes []string
}

func (cc *contentConnUDP) GetName() string {
	return cc.name
}

func (cc *contentConnUDP) getRemotes() []string {
	return cc.remotes
}

func (cc *contentConnUDP) state() ClientState {
	cc.cstateMu.RLock()
	defer cc.cstateMu.RUnlock()

	return cc.cstate
}

func (cc *contentConnUDP) setState(state ClientState) {
	cc.cstateMu.Lock()
	defer cc.cstateMu.Unlock()

	cc.cstate = state
}

func (cc *contentConnUDP) GetMediaPool() string {
	return cc.mediaPool
}

func (cc *contentConnUDP) addMedia(media ...MediaFile) {
	cc.media = append(cc.media, media...)
}

func (cc *contentConnUDP) getAliases() []struct{ Alias, Orig string } {
	return cc.aliases
}

func (cc *contentConnUDP) getItemDefs() []mt.ItemDef {
	return cc.itemDefs
}

func (cc *contentConnUDP) getNodeDefs() []mt.NodeDef {
	return cc.nodeDefs
}

func (cc *contentConnUDP) getMedia() []MediaFile {
	return cc.media
}

func (cc *contentConnUDP) Done() <-chan struct{} { return cc.doneCh }

func (cc *contentConnUDP) Log(dir string, v ...interface{}) {
	cc.logger.Println(append([]interface{}{dir}, v...)...)
}
