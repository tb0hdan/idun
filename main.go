package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"idun/webserver"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/hydra"
	"github.com/tb0hdan/memcache"
)

const (
	OneK            = 1 << 10
	OneMeg          = 1 << 20
	HalfGig         = 512 * OneMeg
	OneGig          = 1 << 30
	TwoGigs         = OneGig * 2
	MaxDomainsInMap = 32
	TickEvery       = 10 * time.Second
	Parallelism     = 2
	RandomDelay     = 15 * time.Second
	APIRetryMax     = 3
	//
	ReadTimeout  = 30 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 60 * time.Second
	//
	GetDomainsRetry = 60 * time.Second
	// process control.
	CrawlerExtra = 10 * time.Second
	KillSleep    = 3 * time.Second
)

var (
	FreyaKey = os.Getenv("FREYA")                      // nolint:gochecknoglobals
	APIBase  = "https://api.domainsproject.org/api/vo" // nolint:gochecknoglobals
	// Version Build info.
	Version   = "unset" // nolint:gochecknoglobals
	GoVersion = "unset" // nolint:gochecknoglobals
	Build     = "unset" // nolint:gochecknoglobals
	BuildDate = "unset" // nolint:gochecknoglobals
)

type JSONResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type DomainsResponse struct {
	Domains []string `json:"domains"`
}

func KillPid(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	//
	time.Sleep(KillSleep)
	//
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func WaitAndKill(sleepTime time.Duration, pid int) {
	time.Sleep(sleepTime)
	log.Println("Run time exceeded, sending signal to ", pid)
	KillPid(pid)
}

func PIDWatcher(pid int) {
	ticker := time.NewTicker(TickEvery)
	for t := range ticker.C {
		pm := sigar.ProcMem{}
		err := pm.Get(pid)
		//
		if err != nil && err.Error() != "no such process" {
			log.Error("PIDWatcher ", err)

			break
		}

		if err != nil {
			// process doesn't exit
			break
		}

		log.Printf("Parent tick for %d at %s: %v\n", pid, t, pm.Resident/OneGig)

		if pm.Resident > TwoGigs {
			log.Printf("Killing subprocess, memory used %d Kb > %d Kb memory allowed\n", pm.Resident/OneK, TwoGigs/OneK)
			KillPid(pid)

			break
		}
	}

	ticker.Stop()
}

func RunCrawl(target, serverAddr string, debugMode bool) {
	// this will terminate process without chance to handle signal correctly
	ctx, cancel := context.WithTimeout(context.Background(), CrawlerMaxRunTime+CrawlerExtra)

	defer cancel()

	args := []string{
		"-url",
		target,
		"-server",
		serverAddr,
	}

	if debugMode {
		args = append(args, "-debug")
	}

	cmd := exec.CommandContext(ctx, os.Args[:1][0], args...) // nolint:gosec
	sout, _ := cmd.StdoutPipe()
	serr, _ := cmd.StderrPipe()
	err := cmd.Start()
	//
	if err != nil {
		log.Error(err)

		return
	}

	if cmd.Process != nil {
		log.Printf("PIDs: parent - %d, child - %d\n", os.Getpid(), cmd.Process.Pid)

		// Monitor memory usage
		go PIDWatcher(cmd.Process.Pid)

		// Process is up, start countdown
		go WaitAndKill(CrawlerMaxRunTime, cmd.Process.Pid)
		//
	}

	pipes := io.MultiReader(sout, serr)
	scanner := bufio.NewScanner(pipes)
	//
	for scanner.Scan() {
		ucl := strings.ToUpper(scanner.Text())
		log.Println(ucl)
	}

	err = cmd.Wait()

	if err != nil {
		log.Errorf("Could not start crawler: %+v\n", err)
	}
}

func RunWithAPI(client *Client, address string, debugMode bool, srvr *S) {
	workerCount, err := CalculateMaxWorkers()
	if err != nil {
		client.Logger.Fatal("Could not calculate worker amount")
	}
	client.Logger.Debugf("Will use up to %d workers", workerCount)
	wn := WorkerNode{
		serverAddr: address,
		srvr:       srvr,
		debugMode:  debugMode,
		client:     client,
	}
	pool := hydra.New(context.Background(), int(workerCount), wn, client.Logger)
	pool.Run()
}

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
		RunAgent(consulURL, logger, *agentPort)

		return
	}

	// configure client
	client := &Client{
		Key:              FreyaKey,
		Logger:           logger,
		APIBase:          APIBase,
		CustomDomainsURL: *customDomainsURL,
	}

	ua, err := client.GetUA()
	if err != nil {
		panic(err)
	}

	s := &S{cache: memcache.New(logger), userAgent: ua}

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
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}
	// do not start listener
	if len(*targetURL) != 0 && len(*serverAddr) != 0 {
		log.Println("Starting crawl of ", *targetURL)
		CrawlURL(client, *targetURL, *debugMode, *serverAddr)

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
		CrawlYacyHosts(*yacyAddr, Address, *debugMode, s)

		return
	}

	if *single {
		log.Println("Starting single URL mode")
		RunCrawl(*targetURL, Address, *debugMode)

		return
	}

	if len(*domainsFile) == 0 {
		log.Println("Starting normal mode")
		//
		ws := webserver.New(fmt.Sprintf(":%d", *webserverPort), ReadTimeout, WriteTimeout, IdleTimeout)
		ws.SetBuildInfo(Version, GoVersion, Build, BuildDate)

		go ws.Run()
		//
		if len(consulURL) != 0 {
			// We have consul. Register there
			consul := NewConsul(consulURL, logger)
			consul.Register()
			//
			defer consul.Deregister()
		}
		//
		RunWithAPI(client, Address, *debugMode, s)

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
		RunCrawl(scanner.Text(), Address, *debugMode)

		// time to empty out cache
		for {
			domain := s.Pop()
			if len(domain) == 0 {
				break
			}

			RunCrawl(domain, Address, *debugMode)
		}
	}
}
