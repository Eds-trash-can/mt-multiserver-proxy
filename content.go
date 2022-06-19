package proxy

import (
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/HimbeerserverDE/srp"
	"github.com/anon55555/mt"
	"github.com/anon55555/mt/rudp"
)

var disallowedChars = regexp.MustCompile("[^a-zA-Z0-9-_.:]")

var b64 = base64.StdEncoding

//go:embed textures/*
var textures embed.FS

type MediaFile struct {
	name       string
	base64SHA1 string
	data       []byte
}

type contentConn interface {
	Log(string, ...interface{})

	Done() <-chan struct{}

	GetMediaPool() string
	GetName() string

	getItemDefs() []mt.ItemDef
	getAliases()  []struct{ Alias, Orig string }

	getNodeDefs() []mt.NodeDef

	addMedia(...MediaFile)
	getMedia() []MediaFile
	getRemotes() []string
}


func addDefaultTextures(cc contentConn) error {
	dir, err := textures.ReadDir("textures")
	if err != nil {
		return err
	}

	files := make([]MediaFile, 0, len(dir))
	for _, f := range dir {
		data, err := textures.ReadFile("textures/" + f.Name())
		if err != nil {
			return err
		}

		sum := sha1.Sum(data)

		files = append(files, MediaFile{
			name:       f.Name(),
			base64SHA1: b64.EncodeToString(sum[:]),
			data:       data,
		})
	}

	cc.addMedia(files...)

	return nil
}



func (cc *contentConnUDP) handleContent() {
	defer close(cc.doneCh)

	go func() {
		init := make(chan struct{})
		defer close(init)

		go func(init <-chan struct{}) {
			select {
			case <-init:
			case <-time.After(10 * time.Second):
				cc.Log("->", "timeout")
				cc.Close()
			}
		}(init)

		for cc.state() == CsCreated {
			cc.SendCmd(&mt.ToSrvInit{
				SerializeVer: serializeVer,
				MinProtoVer:  protoVer,
				MaxProtoVer:  protoVer,
				PlayerName:   cc.userName,
			})
			time.Sleep(500 * time.Millisecond)
		}
	}()

	for {
		pkt, err := cc.Recv()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				if errors.Is(cc.WhyClosed(), rudp.ErrTimedOut) {
					cc.Log("<->", "timeout")
				}

				cc.setState(CsInit)
				break
			}

			cc.Log("<-", err)
			continue
		}

		switch cmd := pkt.Cmd.(type) {
		case *mt.ToCltHello:
			if cc.auth.method != 0 {
				cc.Log("<-", "unexpected authentication")
				cc.Close()
				break
			}

			cc.setState(CsActive)
			if cmd.AuthMethods&mt.FirstSRP != 0 {
				cc.auth.method = mt.FirstSRP
			} else {
				cc.auth.method = mt.SRP
			}

			if cmd.SerializeVer != serializeVer {
				cc.Log("<-", "invalid serializeVer")
				break
			}

			switch cc.auth.method {
			case mt.SRP:
				cc.auth.srpA, cc.auth.a, err = srp.InitiateHandshake()
				if err != nil {
					cc.Log("->", err)
					break
				}

				cc.SendCmd(&mt.ToSrvSRPBytesA{
					A:      cc.auth.srpA,
					NoSHA1: true,
				})
			case mt.FirstSRP:
				id := strings.ToLower(cc.userName)

				salt, verifier, err := srp.NewClient([]byte(id), []byte{})
				if err != nil {
					cc.Log("->", err)
					break
				}

				cc.SendCmd(&mt.ToSrvFirstSRP{
					Salt:        salt,
					Verifier:    verifier,
					EmptyPasswd: true,
				})
			default:
				cc.Log("<->", "invalid auth method")
				cc.Close()
			}
		case *mt.ToCltSRPBytesSaltB:
			if cc.auth.method != mt.SRP {
				cc.Log("<-", "multiple authentication attempts")
				break
			}

			id := strings.ToLower(cc.userName)

			cc.auth.srpK, err = srp.CompleteHandshake(cc.auth.srpA, cc.auth.a, []byte(id), []byte{}, cmd.Salt, cmd.B)
			if err != nil {
				cc.Log("->", err)
				break
			}

			M := srp.ClientProof([]byte(cc.userName), cmd.Salt, cc.auth.srpA, cmd.B, cc.auth.srpK)
			if M == nil {
				cc.Log("<-", "SRP safety check fail")
				break
			}

			cc.SendCmd(&mt.ToSrvSRPBytesM{
				M: M,
			})
		case *mt.ToCltKick:
			cc.Log("<-", "deny access", cmd)
		case *mt.ToCltAcceptAuth:
			cc.auth.method = 0
			cc.SendCmd(&mt.ToSrvInit2{})
		case *mt.ToCltItemDefs:
			for _, def := range cmd.Defs {
				cc.itemDefs = append(cc.itemDefs, def)
			}
			cc.aliases = cmd.Aliases
		case *mt.ToCltNodeDefs:
			for _, def := range cmd.Defs {
				cc.nodeDefs = append(cc.nodeDefs, def)
			}
		case *mt.ToCltAnnounceMedia:
			var filenames []string

			for _, f := range cmd.Files {

				if FromCache(cc, f.Name, f.Base64SHA1) {
					continue
				}

				filenames = append(filenames, f.Name)

				for i, mf := range cc.media {
					if mf.name == f.Name {
						cc.media[i].base64SHA1 = f.Base64SHA1
						continue
					}
				}

				cc.addMedia(MediaFile{
					name:       f.Name,
					base64SHA1: f.Base64SHA1,
				})
			}

			cc.remotes = strings.Split(cmd.URL, ",")
			for k, v := range cc.remotes {
				cc.remotes[k] = strings.TrimSpace(v)
			}

			cc.SendCmd(&mt.ToSrvReqMedia{Filenames: filenames})
		case *mt.ToCltMedia:
			for _, f := range cmd.Files {
				for i, af := range cc.media {
					if af.name == f.Name {
						cc.media[i].data = f.Data
						break
					}
				}
			}

			if cmd.I == cmd.N-1 {
				updateCache(cc.getMedia())
				cc.Close()
			}
		}
	}
}

func (cc *ClientConn) sendMedia(filenames []string) {
	var bunches [][]struct {
		Name string
		Data []byte
	}
	bunches = append(bunches, []struct {
		Name string
		Data []byte
	}{})

	var bunchSize int
	for _, filename := range filenames {
		var known bool
		for _, f := range cc.media {
			if f.name == filename {
				mfile := struct {
					Name string
					Data []byte
				}{
					Name: f.name,
					Data: f.data,
				}
				bunches[len(bunches)-1] = append(bunches[len(bunches)-1], mfile)

				bunchSize += len(f.data)
				if bunchSize >= bytesPerMediaBunch {
					bunches = append(bunches, []struct {
						Name string
						Data []byte
					}{})
					bunchSize = 0
				}

				known = true
				break
			}
		}

		if !known {
			cc.Log("->", "request unknown media file")
			continue
		}
	}

	for i := uint16(0); i < uint16(len(bunches)); i++ {
		cc.SendCmd(&mt.ToCltMedia{
			N:     uint16(len(bunches)),
			I:     i,
			Files: bunches[i],
		})
	}
}

type param0Map map[string]map[mt.Content]mt.Content
type param0SrvMap map[mt.Content]struct {
	name   string
	param0 mt.Content
}

func muxItemDefs(conns []contentConn) ([]mt.ItemDef, []struct{ Alias, Orig string }) {
	var itemDefs []mt.ItemDef
	var aliases []struct{ Alias, Orig string }

	itemDefs = append(itemDefs, mt.ItemDef{
		Type:       mt.ToolItem,
		InvImg:     "wieldhand.png",
		WieldScale: [3]float32{1, 1, 1},
		StackMax:   1,
		ToolCaps: mt.ToolCaps{
			NonNil: true,
		},
		PointRange: 4,
	})

	for _, cc := range conns {
		<-cc.Done()
		pool := cc.GetMediaPool()
		
		for _, def := range cc.getItemDefs() {
			if def.Name == "" {
				def.Name = "hand"
			}

			prepend(pool, &def.Name)
			prependTexture(pool, &def.InvImg)
			prependTexture(pool, &def.WieldImg)
			prepend(pool, &def.PlacePredict)
			prepend(pool, &def.PlaceSnd.Name)
			prepend(pool, &def.PlaceFailSnd.Name)
			prependTexture(pool, &def.Palette)
			prependTexture(pool, &def.InvOverlay)
			prependTexture(pool, &def.WieldOverlay)
			itemDefs = append(itemDefs, def)
		}

		for _, alias := range cc.getAliases() {
			prepend(pool, &alias.Alias)
			prepend(pool, &alias.Orig)

			aliases = append(aliases, struct{ Alias, Orig string }{
				Alias: alias.Alias,
				Orig:  alias.Orig,
			})
		}
	}

	return itemDefs, aliases
}

func muxNodeDefs(conns []contentConn) (nodeDefs []mt.NodeDef, p0Map param0Map, p0SrvMap param0SrvMap) {
	var param0 mt.Content

	p0Map = make(param0Map)
	p0SrvMap = param0SrvMap{
		mt.Unknown: struct {
			name   string
			param0 mt.Content
		}{
			param0: mt.Unknown,
		},
		mt.Air: struct {
			name   string
			param0 mt.Content
		}{
			param0: mt.Air,
		},
		mt.Ignore: struct {
			name   string
			param0 mt.Content
		}{
			param0: mt.Ignore,
		},
	}

	for _, cc := range conns {
		name := cc.GetName()
		pool := cc.GetMediaPool()
	
		<-cc.Done()
		for _, def := range cc.getNodeDefs() {
			if p0Map[name] == nil {
				p0Map[name] = map[mt.Content]mt.Content{
					mt.Unknown: mt.Unknown,
					mt.Air:     mt.Air,
					mt.Ignore:  mt.Ignore,
				}
			}

			p0Map[name][def.Param0] = param0
			p0SrvMap[param0] = struct {
				name   string
				param0 mt.Content
			}{
				name:   name,
				param0: def.Param0,
			}

			def.Param0 = param0
			oldName := def.Name // copy string to use later
			prepend(pool, &def.Name)
			prepend(pool, &def.Mesh)
			for i := range def.Tiles {
				prependTexture(pool, &def.Tiles[i].Texture)
			}
			for i := range def.OverlayTiles {
				prependTexture(pool, &def.OverlayTiles[i].Texture)
			}
			for i := range def.SpecialTiles {
				prependTexture(pool, &def.SpecialTiles[i].Texture)
			}
			prependTexture(pool, &def.Palette)
			for k, v := range def.ConnectTo {
				def.ConnectTo[k] = p0Map[name][v]
			}
			prepend(pool, &def.FootstepSnd.Name)
			prepend(pool, &def.DiggingSnd.Name)
			prepend(pool, &def.DugSnd.Name)
			prepend(pool, &def.DigPredict)
			nodeDefs = append(nodeDefs, def)

			param0++
			if param0 >= mt.Unknown && param0 <= mt.Ignore {
				param0 = mt.Ignore + 1
			}

			// add nodeid (if reqested)
			addNodeId(oldName, def.Param0)
		}
	}

	return
}

func muxMedia(conns []contentConn) []MediaFile {
	var media []MediaFile

	for _, cc := range conns {
		<-cc.Done()
		for _, f := range cc.getMedia() {
			prepend(cc.GetMediaPool(), &f.name)
			media = append(media, f)
		}
	}

	return media
}

func muxRemotes(conns []contentConn) []string {
	remotes := make(map[string]struct{})

	for _, cc := range conns {
		<-cc.Done()
		for _, v := range cc.getRemotes() {
			remotes[v] = struct{}{}
		}
	}

	urls := make([]string, 0, len(remotes))
	for remote := range remotes {
		urls = append(urls, remote)
	}

	return urls
}

func muxContent(userName string) (itemDefs []mt.ItemDef, aliases []struct{ Alias, Orig string }, nodeDefs []mt.NodeDef, p0Map param0Map, p0SrvMap param0SrvMap, media []MediaFile, remotes []string, err error) {
	var conns []contentConn

PoolLoop:
	for _, pool := range PoolServers() {
		for _, srv := range pool {
			if conn, err := srv.contentConn(); err != nil {
				continue
			} else {
				conns = append(conns, conn)
				continue PoolLoop
			}
		}

		// There's a pool with no reachable servers.
		// We can't safely let clients join.
		return
	}

	itemDefs, aliases = muxItemDefs(conns)
	nodeDefs, p0Map, p0SrvMap = muxNodeDefs(conns)
	media = muxMedia(conns)
	remotes = muxRemotes(conns)
	return
}

func globalParam0(sc ServerConn, p0 *mt.Content) {
	clt := sc.client()
	name := sc.GetName()
	
	if clt != nil && clt.p0Map != nil {
		if clt.p0Map[name] != nil {
			*p0 = clt.p0Map[name][*p0]
		}
	}
}

func (cc *ClientConn) srvParam0(p0 *mt.Content) string {
	if cc.p0SrvMap != nil {
		srv := cc.p0SrvMap[*p0]
		*p0 = srv.param0
		return srv.name
	}

	return ""
}

func isDefaultNode(s string) bool {
	list := []string{
		"",
		"air",
		"unknown",
		"ignore",
	}

	for _, s2 := range list {
		if s == s2 {
			return true
		}
	}

	return false
}

func prependRaw(prep string, s *string, isTexture bool) {
	if !isDefaultNode(*s) {
		subs := disallowedChars.Split(*s, -1)
		seps := disallowedChars.FindAllString(*s, -1)

		for i, sub := range subs {
			if !isTexture || strings.Contains(sub, ".") {
				subs[i] = prep + "_" + sub
			}
		}

		*s = ""
		for i, sub := range subs {
			*s += sub
			if i < len(seps) {
				*s += seps[i]
			}
		}
	}
}

func prepend(prep string, s *string) {
	prependRaw(prep, s, false)
}

func prependTexture(prep string, t *mt.Texture) {
	s := string(*t)
	prependRaw(prep, &s, true)
	*t = mt.Texture(s)
}

func prependInv(mediaPool string, inv mt.Inv) {
	for k, l := range inv {
		for i := range l.Stacks {
			prepend(mediaPool, &inv[k].InvList.Stacks[i].Name)
		}
	}
}

func prependHUD(mediaPool string, t mt.HUDType, cmdIface mt.ToCltCmd) {
	pa := func(cmd *mt.ToCltAddHUD) {
		switch t {
		case mt.StatbarHUD:
			prepend(mediaPool, &cmd.Text2)
			fallthrough
		case mt.ImgHUD:
			fallthrough
		case mt.ImgWaypointHUD:
			fallthrough
		case mt.ImgWaypointHUD + 1:
			prepend(mediaPool, &cmd.Text)
		}
	}

	pc := func(cmd *mt.ToCltChangeHUD) {
		switch t {
		case mt.StatbarHUD:
			prepend(mediaPool, &cmd.Text2)
			fallthrough
		case mt.ImgHUD:
			fallthrough
		case mt.ImgWaypointHUD:
			fallthrough
		case mt.ImgWaypointHUD + 1:
			prepend(mediaPool, &cmd.Text)
		}
	}

	switch cmd := cmdIface.(type) {
	case *mt.ToCltAddHUD:
		pa(cmd)
	case *mt.ToCltChangeHUD:
		pc(cmd)
	}
}
