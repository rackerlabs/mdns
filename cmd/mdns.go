package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"

	"github.com/rackerlabs/mdns"
)

var builddate = ""
var gitref = ""

func main() {
	// Init config
	conf := mdns.InitConfig()

	// Exit if someone just wants to know version
	if conf.Version == true {
		fmt.Println(fmt.Sprintf("built from %s on %s", gitref, builddate))
		os.Exit(0)
	}

	// Logging
	mdns.InitLogging()

	// Database
	mysql := &mdns.MySQLDriver{}
	err := mysql.Open()
	if err != nil {
		log.Fatal(fmt.Sprintf("Couldn't connect to database : %s", err))
		os.Exit(1)
	}
	storage := mdns.Storage{Driver: mysql}

	handler := mdns.NewDefaultMdnsHandler(storage)

	// Listeners
	go mdns.Serve("tcp", conf.BindAddress, conf.BindPort, handler)
	go mdns.Serve("udp", conf.BindAddress, conf.BindPort, handler)
	mdns.Listen()
}
