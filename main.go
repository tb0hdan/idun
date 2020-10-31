package main

import (
	"bufio"
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/memcache"
)

const (
	OneGig          = 1 << 20
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
)

var (
	FreyaKey = os.Getenv("FREYA")
	APIBase  = "https://api.domainsproject.org/api/vo"
)

type JSONResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type DomainsResponse struct {
	Domains []string `json:"domains"`
}

func RunCrawl(target, serverAddr string, debugMode bool) {
	ctx := context.Background()

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
	out, _ := cmd.StdoutPipe()
	err := cmd.Start()
	//
	if err != nil {
		log.Error(err)

		return
	}

	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		ucl := strings.ToUpper(scanner.Text())
		log.Println(ucl)
	}

	err = cmd.Wait()

	if err != nil {
		log.Errorf("Could not start crawler: %+v\n", err)
	}
}

func main() {
	debugMode := flag.Bool("debug", false, "Enable colly/crawler debugging")
	targetURL := flag.String("url", "", "URL/Domain to crawl")
	serverAddr := flag.String("server", "", "Local supervisor address")
	flag.Parse()

	logger := log.New()

	if *debugMode {
		logger.SetLevel(log.DebugLevel)
	}

	client := &Client{
		Key:     FreyaKey,
		Logger:  logger,
		APIBase: APIBase,
	}

	if len(*targetURL) != 0 {
		CrawlURL(client, *targetURL, *debugMode, *serverAddr)

		return
	}

	s := &S{cache: memcache.New(logger)}

	r := mux.NewRouter()
	r.HandleFunc("/upload", s.UploadDomains).Methods(http.MethodPost)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	Address := listener.Addr().String()
	_ = listener.Close()

	server := &http.Server{
		Addr:         Address,
		Handler:      r,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}

	go func() {
		log.Println("Starting internal listener at ", Address)

		if err := server.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	for {
		domains, err := client.GetDomains()
		if err != nil {
			time.Sleep(GetDomainsRetry)

			continue
		}

		for _, domain := range domains {
			RunCrawl(domain, Address, *debugMode)
		}
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
