package agent

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/idun/pkg/consul"
	"github.com/tb0hdan/idun/pkg/varstruct"

	"github.com/tb0hdan/idun/pkg/webserver"
)

func RunAgent(consulURL string, logger *log.Logger, agentPort int, Version, GoVersion, Build, BuildDate string) {
	ws := webserver.New(fmt.Sprintf(":%d", agentPort), varstruct.ReadTimeout, varstruct.WriteTimeout, varstruct.IdleTimeout)
	ws.SetBuildInfo(Version, GoVersion, Build, BuildDate)

	go ws.Run()
	//
	// We have consulClient. Register there
	consulClient := consul.NewConsul(consulURL, logger)
	consulClient.SetServiceName("agent")
	consulClient.SetAdvertisedPort(agentPort)
	consulClient.Register()
}
