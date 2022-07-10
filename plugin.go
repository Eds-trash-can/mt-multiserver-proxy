package proxy

import (
	"log"
	"os"
	"plugin"
	"sync"
)

var pluginsOnce sync.Once

var loadingPlugin string

func loadPlugins() {
	pluginsOnce.Do(openPlugins)
}

func openPlugins() {
	path := Path("plugins")
	os.Mkdir(path, 0777)

	dir, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range dir {
		loadingPlugin = file.Name()
		_, err := plugin.Open(path + "/" + file.Name())
		if err != nil {
			log.Print(err)
			continue
		}
	}

	loadingPlugin = ""

	log.Print("load plugins")
}
