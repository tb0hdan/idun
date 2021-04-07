package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/hydra"
	"github.com/tb0hdan/idun/pkg/agent"
	"github.com/tb0hdan/idun/pkg/calculator"
	"github.com/tb0hdan/idun/pkg/client"
	"github.com/tb0hdan/idun/pkg/consul"
	"github.com/tb0hdan/idun/pkg/crawler"
	"github.com/tb0hdan/idun/pkg/server"
	"github.com/tb0hdan/idun/pkg/utils"
	"github.com/tb0hdan/idun/pkg/varstruct"
	"github.com/tb0hdan/idun/pkg/webserver"
	"github.com/tb0hdan/idun/pkg/worker"
	"github.com/tb0hdan/idun/pkg/yacy"
	"github.com/tb0hdan/memcache"
)

var (
	// Version Build info.
	Version   = "unset" // nolint:gochecknoglobals
	GoVersion = "unset" // nolint:gochecknoglobals
	Build     = "unset" // nolint:gochecknoglobals
	BuildDate = "unset" // nolint:gochecknoglobals
)

func RunWithAPI(c *client.Client, address string, debugMode bool, srvr *server.S) {
	workerCount, err := calculator.CalculateMaxWorkers()
	if err != nil {
		c.Logger.Fatal("Could not calculate worker amount")
	}
	c.Logger.Debugf("Will use up to %d workers", workerCount)
	wn := worker.WorkerNode{
		ServerAddr: address,
		Srvr:       srvr,
		DebugMode:  debugMode,
		C:          c,
	}
	pool := hydra.New(context.Background(), int(workerCount), wn, c.Logger)
	pool.Run()
}

func main() { // nolint:funlen
	debugMode := flag.Bool("debug", false, "Enable colly/crawler debugging")
	targetURL := flag.String("url", "", "URL/Domain to crawl")
	serverAddr := flag.String("httpServer", "", "Local supervisor address")
	domainsFile := flag.String("file", "", "Domains file, one domain per line")
	yacyMode := flag.Bool("yacyMode", false, "Get hosts from Yacy.net FreeWorld network and crawl them")
	yacyAddr := flag.String("yacyMode-addr", "http://127.0.0.1:8090", "Yacy.net address, defaults to localhost")
	single := flag.Bool("single", false, "Start with single url. For debugging.")
	//
	webserverPort := flag.Int("webserver-port", 80, "Built-in web httpServer port")
	agentPort := flag.Int("agentMode-port", 8000, "Agent httpServer port")
	agentMode := flag.Bool("agentMode", false, "Host monitor for use with consul")
	//
	customDomainsURL := flag.String("custom-domains-url", "", "Get domains from custom URL")
	version := flag.Bool("version", false, "Print version and exit")
	//
	flag.Parse()

	logger := log.New()

	if *version {
		fmt.Println(Version, GoVersion, Build, BuildDate)
		return
	}
	if *debugMode {
		logger.SetLevel(log.DebugLevel)
	}
	// both agentMode mode and workers use this
	consulURL := os.Getenv("CONSUL")
	if len(consulURL) > 0 && !strings.HasPrefix(consulURL, "http://") {
		consulURL = fmt.Sprintf("http://%s:8500", consulURL)
	}

	if *agentMode && len(consulURL) > 0 {
		logger.Println("Starting in agentMode mode. Please use only one per host.")
		agent.RunAgent(consulURL, logger, *agentPort, Version, GoVersion, Build, BuildDate)

		return
	}

	// configure idunClient
	idunClient := &client.Client{
		Key:              varstruct.FreyaKey,
		Logger:           logger,
		APIBase:          varstruct.APIBase,
		CustomDomainsURL: *customDomainsURL,
	}

	ua, err := idunClient.GetUA()
	if err != nil {
		panic(err)
	}

	s := &server.S{Cache: memcache.New(logger), UserAgent: ua}

	r := mux.NewRouter()
	r.HandleFunc("/upload", s.UploadDomains).Methods(http.MethodPost)
	r.HandleFunc("/ua", s.UA).Methods(http.MethodGet)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	//
	Address := listener.Addr().String()
	_ = listener.Close()

	httpServer := &http.Server{
		Addr:         Address,
		Handler:      r,
		ReadTimeout:  varstruct.ReadTimeout,
		WriteTimeout: varstruct.WriteTimeout,
		IdleTimeout:  varstruct.IdleTimeout,
	}
	// do not start listener
	if len(*targetURL) != 0 && len(*serverAddr) != 0 {
		log.Println("Starting crawl of ", *targetURL)
		crawler.CrawlURL(idunClient, *targetURL, *debugMode, *serverAddr)

		return
	}

	go func() {
		log.Println("Starting internal listener at ", Address)

		if err := httpServer.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	// start listener for this one and below
	if *yacyMode {
		log.Println("Starting Yacy.net mode")
		yacy.CrawlYacyHosts(*yacyAddr, Address, *debugMode, s)

		return
	}

	if *single {
		log.Println("Starting single URL mode")
		utils.RunCrawl(*targetURL, Address, *debugMode)

		return
	}

	if len(*domainsFile) == 0 {
		log.Println("Starting normal mode")
		//
		ws := webserver.New(fmt.Sprintf(":%d", *webserverPort), varstruct.ReadTimeout, varstruct.WriteTimeout, varstruct.IdleTimeout)
		ws.SetBuildInfo(Version, GoVersion, Build, BuildDate)

		go ws.Run()
		//
		if len(consulURL) != 0 {
			// We have consulClient. Register there
			consulClient := consul.NewConsul(consulURL, logger)
			consulClient.Register()
			//
			defer consulClient.Deregister()
		}
		//
		RunWithAPI(idunClient, Address, *debugMode, s)

		return
	}
	//
	// FALLBACK TO FILE MODE
	//
	log.Println("Starting file mode")

	f, err := os.Open(*domainsFile)
	if err != nil {
		panic(err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		utils.RunCrawl(scanner.Text(), Address, *debugMode)

		// time to empty out cache
		for {
			domain := s.Pop()
			if len(domain) == 0 {
				break
			}

			utils.RunCrawl(domain, Address, *debugMode)
		}
	}
}
