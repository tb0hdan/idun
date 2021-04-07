package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/idun"
	"github.com/tb0hdan/idun/webserver"
	"github.com/tb0hdan/memcache"
)

func main() { // nolint:funlen
	debugMode := flag.Bool("debug", false, "Enable colly/crawler debugging")
	targetURL := flag.String("url", "", "URL/Domain to crawl")
	serverAddr := flag.String("server", "", "Local supervisor address")
	domainsFile := flag.String("file", "", "Domains file, one domain per line")
	yacy := flag.Bool("yacy", false, "Get hosts from Yacy.net FreeWorld network and crawl them")
	yacyAddr := flag.String("yacy-addr", "http://127.0.0.1:8090", "Yacy.net address, defaults to localhost")
	single := flag.Bool("single", false, "Start with single url. For debugging.")
	//
	webserverPort := flag.Int("webserver-port", 80, "Built-in web server port")
	agentPort := flag.Int("agent-port", 8000, "Agent server port")
	agent := flag.Bool("agent", false, "Host monitor for use with consul")
	//
	customDomainsURL := flag.String("custom-domains-url", "", "Get domains from custom URL")
	//
	flag.Parse()

	logger := log.New()

	if *debugMode {
		logger.SetLevel(log.DebugLevel)
	}
	// both agent mode and workers use this
	consulURL := os.Getenv("CONSUL")
	if len(consulURL) > 0 && !strings.HasPrefix(consulURL, "http://") {
		consulURL = fmt.Sprintf("http://%s:8500", consulURL)
	}

	if *agent && len(consulURL) > 0 {
		logger.Println("Starting in agent mode. Please use only one per host.")
		idun.RunAgent(consulURL, logger, *agentPort)

		return
	}

	// configure client
	client := &idun.Client{
		Key:              idun.FreyaKey,
		Logger:           logger,
		APIBase:          idun.APIBase,
		CustomDomainsURL: *customDomainsURL,
	}

	ua, err := client.GetUA()
	if err != nil {
		panic(err)
	}

	s := &idun.S{Cache: memcache.New(logger), UserAgent: ua}

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

	server := &http.Server{
		Addr:         Address,
		Handler:      r,
		ReadTimeout:  idun.ReadTimeout,
		WriteTimeout: idun.WriteTimeout,
		IdleTimeout:  idun.IdleTimeout,
	}
	// do not start listener
	if len(*targetURL) != 0 && len(*serverAddr) != 0 {
		log.Println("Starting crawl of ", *targetURL)
		idun.CrawlURL(client, *targetURL, *debugMode, *serverAddr)

		return
	}

	go func() {
		log.Println("Starting internal listener at ", Address)

		if err := server.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	// start listener for this one and below
	if *yacy {
		log.Println("Starting Yacy.net mode")
		idun.CrawlYacyHosts(*yacyAddr, Address, *debugMode, s)

		return
	}

	if *single {
		log.Println("Starting single URL mode")
		idun.RunCrawl(*targetURL, Address, *debugMode)

		return
	}

	if len(*domainsFile) == 0 {
		log.Println("Starting normal mode")
		//
		ws := webserver.New(fmt.Sprintf(":%d", *webserverPort), idun.ReadTimeout, idun.WriteTimeout, idun.IdleTimeout)
		ws.SetBuildInfo(idun.Version, idun.GoVersion, idun.Build, idun.BuildDate)

		go ws.Run()
		//
		if len(consulURL) != 0 {
			// We have consul. Register there
			consul := idun.NewConsul(consulURL, logger)
			consul.Register()
			//
			defer consul.Deregister()
		}
		//
		idun.RunWithAPI(client, Address, *debugMode, s)

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
		idun.RunCrawl(scanner.Text(), Address, *debugMode)

		// time to empty out cache
		for {
			domain := s.Pop()
			if len(domain) == 0 {
				break
			}

			idun.RunCrawl(domain, Address, *debugMode)
		}
	}
}
