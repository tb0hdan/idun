package main

import (
	"fmt"
	"idun/webserver"

	log "github.com/sirupsen/logrus"
)

func RunAgent(consulURL string, logger *log.Logger, agentPort int) {
	ws := webserver.New(fmt.Sprintf(":%d", agentPort), ReadTimeout, WriteTimeout, IdleTimeout)
	ws.SetBuildInfo(Version, GoVersion, Build, BuildDate)

	go ws.Run()
	//
	// We have consul. Register there
	consul := NewConsul(consulURL, logger)
	consul.SetServiceName("agent")
	consul.SetAdvertisedPort(agentPort)
	consul.Register()
}
