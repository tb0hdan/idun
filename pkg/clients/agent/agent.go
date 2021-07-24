package agent

import (
	"fmt"

	"github.com/tb0hdan/idun/pkg/servers/webserver"
	"github.com/tb0hdan/idun/pkg/types"

	"github.com/tb0hdan/idun/pkg/clients/consul"

	log "github.com/sirupsen/logrus"
)

func RunAgent(consulURL string, logger *log.Logger, agentPort int, Version, GoVersion, Build, BuildDate string) {
	ws := webserver.NewWebServer(fmt.Sprintf(":%d", agentPort), types.ReadTimeout, types.WriteTimeout, types.IdleTimeout)
	ws.SetBuildInfo(Version, GoVersion, Build, BuildDate)

	go ws.Run()
	//
	// We have consulClient. Register there
	consulClient := consul.NewConsul(consulURL, logger)
	consulClient.SetServiceName("agent")
	consulClient.SetAdvertisedPort(agentPort)
	consulClient.Register()
}
