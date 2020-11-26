package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	CrawlFilterRetry = 60 * time.Second
	HeadCheckTimeout = 10 * time.Second
	// process limits.
	CrawlerMaxRunTime = 300 * time.Second
)

var BannedExtensions = []string{ // nolint:gochecknoglobals
	"asc", "avi", "bmp", "dll", "doc", "docx", "exe", "iso", "jpg", "mp3", "odt",
	"pdf", "png", "rar", "rdf", "svg", "tar", "tar.gz", "tar.bz2", "tgz", "txt",
	"wav", "wmv", "xml", "xz", "zip",
}

func DeduplicateSlice(incoming []string) (outgoing []string) {
	hash := make(map[string]int)
	outgoing = make([]string, 0)
	//
	for _, value := range incoming {
		if _, ok := hash[value]; !ok {
			hash[value] = 1

			outgoing = append(outgoing, value)
		}
	}
	//
	return
}

func HeadCheck(domain string, ua string) bool {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: tr,
	}
	ctx, cancel := context.WithTimeout(context.Background(), HeadCheckTimeout)

	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("http://%s", domain), nil)
	//
	if err != nil {
		return false
	}

	req.Header.Add("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && !strings.HasPrefix(fmt.Sprintf("%d", resp.StatusCode), "3") {
		return false
	}

	return true
}

func HeadCheckDomains(domains []string, ua string) map[string]bool {
	results := make(map[string]bool)
	wg := &sync.WaitGroup{}
	lock := &sync.RWMutex{}

	for _, domain := range DeduplicateSlice(domains) {
		wg.Add(1)

		go func(domain string, wg *sync.WaitGroup) {
			result := HeadCheck(domain, ua)

			lock.Lock()
			results[domain] = result
			lock.Unlock()
			wg.Done()
		}(domain, wg)
	}

	wg.Wait()

	return results
}

func SubmitOutgoingDomains(client *Client, domains []string, serverAddr string) {
	log.Println("Submit called: ", domains)
	//
	if len(domains) == 0 {
		return
	}

	var domainsRequest DomainsResponse

	domainsRequest.Domains = DeduplicateSlice(domains)
	body, err := json.Marshal(&domainsRequest)
	//
	if err != nil {
		log.Error(err)

		return
	}

	serverURL := fmt.Sprintf("http://%s/upload", serverAddr)
	retryClient := PrepareClient(client.Logger)
	req, err := retryablehttp.NewRequest(http.MethodPost, serverURL, body)
	//
	if err != nil {
		log.Error(err)

		return
	}
	//
	resp, err := retryClient.Do(req)
	//
	if err != nil {
		log.Error(err)

		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	//
	if err != nil {
		log.Error(err)

		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Error(string(data))
	}
}

func GetUA(reqURL string, logger *log.Logger) (string, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, reqURL, nil)
	//
	if err != nil {
		return "", err
	}
	//
	// req.Header.Add("X-Session-Token", c.Key)
	//
	retryClient := PrepareClient(logger)
	resp, err := retryClient.Do(req)
	//
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	message := &JSONResponse{}
	err = json.NewDecoder(resp.Body).Decode(message)

	if err != nil {
		return "", err
	}

	if message.Code != http.StatusOK {
		return "", errors.New("non-ok response")
	}
	//
	log.Println("UA: ", message.Message)

	return message.Message, nil
}

func FilterAndSubmit(domainMap map[string]bool, client *Client, serverAddr string) {
	domains := make([]string, 0, len(domainMap))

	for domain := range domainMap {
		domains = append(domains, domain)
	}

	outgoing, err := client.FilterDomains(domains)
	if err != nil {
		log.Println("Filter failed with", err)
		time.Sleep(CrawlFilterRetry)

		return
	}

	SubmitOutgoingDomains(client, outgoing, serverAddr)
}

func CrawlURL(client *Client, targetURL string, debugMode bool, serverAddr string) { // nolint:funlen,gocognit
	if len(targetURL) == 0 {
		panic("Cannot start with empty url")
	}

	if !strings.HasPrefix(targetURL, "http") {
		targetURL = fmt.Sprintf("http://%s", targetURL)
	}
	// Self-checks
	mem := sigar.Mem{}
	err := mem.Get()
	//
	if err != nil {
		panic(err)
	}

	if mem.Total < TwoGigs || mem.Free < TwoGigs {
		panic("Will not start without enough RAM. At least 2Gb free is required")
	}
	//
	parsed, err := url.Parse(targetURL)
	allowedDomain := strings.ToLower(parsed.Host)

	if err != nil {
		panic(err)
	}

	done := make(chan bool)

	ua, err := GetUA(fmt.Sprintf("http://%s/ua", serverAddr), client.Logger)
	if err != nil {
		panic(err)
	}

	filters := make([]*regexp.Regexp, 0, len(BannedExtensions))
	for _, reg := range BannedExtensions {
		filters = append(filters, regexp.MustCompile(fmt.Sprintf(`.+\.%s$`, reg)))
	}

	defaultOptions := []func(collector *colly.Collector){
		colly.Async(true),
		colly.UserAgent(ua),
		colly.DisallowedURLFilters(filters...),
	}
	if debugMode {
		defaultOptions = append(defaultOptions, colly.Debugger(&debug.LogDebugger{}))
	}

	robo, err := NewRoboTester(targetURL, ua)
	if err != nil {
		panic(err)
	}

	c := colly.NewCollector(
		defaultOptions...,
	)

	c.WithTransport(&http.Transport{
		DisableKeepAlives: true,
	})

	_ = c.Limit(&colly.LimitRule{
		Parallelism: Parallelism,
		RandomDelay: RandomDelay,
	})

	domainMap := make(map[string]bool)

	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		absolute := e.Request.AbsoluteURL(link)

		parsed, err := url.Parse(absolute)
		parsedHost := strings.ToLower(parsed.Host)
		if err != nil {
			print(err)

			return
		}
		//
		if !strings.HasPrefix(absolute, "http") {
			return
		}
		// No follow check
		if strings.ToLower(e.Attr("rel")) == "nofollow" {
			log.Printf("Nofollow: %s\n", absolute)

			return
		}
		//

		if !strings.HasSuffix(parsedHost, allowedDomain) {
			// external links
			if len(domainMap) < MaxDomainsInMap {
				if _, ok := domainMap[parsedHost]; !ok {
					domainMap[parsedHost] = true
				}

				return
			}
			//
			FilterAndSubmit(domainMap, client, serverAddr)
			//
			domainMap = make(map[string]bool)

			return
		}

		if !robo.Test(link) {
			log.Errorf("Crawling of %s is disallowed by robots.txt", absolute)

			return
		}

		_ = c.Visit(absolute)
	})

	c.OnRequest(func(r *colly.Request) {
		if debugMode {
			log.Println("Visiting", r.URL.String())
		}
	})

	// catch SIGINT / SIGTERM / SIGQUIT signals & request exit
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		<-sig
		done <- true
	}()

	ts := time.Now()
	ticker := time.NewTicker(TickEvery)

	go func() {
		for t := range ticker.C {
			mem := sigar.ProcMem{}
			err := mem.Get(os.Getpid())
			//
			if err != nil {
				// something's very wrong
				log.Error(err)
				done <- true

				break
			}

			log.Println("Tick at", t, mem.Resident/OneGig)
			runtime.GC()

			if mem.Resident > TwoGigs {
				// 2Gb MAX
				log.Println("2Gb RAM limit exceeded, exiting...")
				done <- true

				break
			}

			if t.After(ts.Add(CrawlerMaxRunTime)) {
				log.Println("Max run time exceeded, exiting...")
				done <- true

				break
			}
		}
	}()

	if !robo.Test("/") {
		log.Errorf("Crawling of / for %s is disallowed by robots.txt", targetURL)

		return
	}

	// this one has to be started *AFTER* calling c.Visit()
	go func() {
		_ = c.Visit(targetURL)
		c.Wait()
		done <- true
	}()

	<-done
	// Submit remaining data
	FilterAndSubmit(domainMap, client, serverAddr)
	ticker.Stop()
	log.Println("Crawler exit")
}
