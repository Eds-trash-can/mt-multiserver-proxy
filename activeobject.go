package proxy

import "github.com/anon55555/mt"

func srvSwapAOID(sc ServerConn, ao *mt.AOID) {
	if sc.client() != nil {
		if *ao == sc.client().playerCAO {
			*ao = sc.client().currentCAO
		} else if *ao == sc.client().currentCAO {
			*ao = sc.client().playerCAO
		}
	}
}

func srvHandleAOMsg(sc ServerConn, aoMsg mt.AOMsg) {
	mediaPool := sc.GetMediaPool()

	switch msg := aoMsg.(type) {
	case *mt.AOCmdAttach:
		sc.swapAOID(&msg.Attach.ParentID)
	case *mt.AOCmdProps:
		for j := range msg.Props.Textures {
			prependTexture(mediaPool, &msg.Props.Textures[j])
		}
		prepend(mediaPool, &msg.Props.Mesh)
		prepend(mediaPool, &msg.Props.Itemstring)
		prependTexture(mediaPool, &msg.Props.DmgTextureMod)
	case *mt.AOCmdSpawnInfant:
		sc.swapAOID(&msg.ID)
	case *mt.AOCmdTextureMod:
		prependTexture(mediaPool, &msg.Mod)
	}
}
