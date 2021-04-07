package idun

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
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
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/debug"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	CrawlFilterRetry = 60 * time.Second
	HeadCheckTimeout = 10 * time.Second
	// process limits.
	CrawlerMaxRunTime = 600 * time.Second
)

var (
	BannedExtensions = []string{ // nolint:gochecknoglobals
		"asc", "avi", "bmp", "dll", "doc", "docx", "exe", "iso", "jpg", "mp3", "odt",
		"pdf", "png", "rar", "rdf", "svg", "tar", "tar.gz", "tar.bz2", "tgz", "txt",
		"wav", "wmv", "xml", "xz", "zip",
	}

	BannedLocalRedirects = map[string]string{ // nolint:gochecknoglobals
		"www.president.gov.ua": "1",
	}

	IgnoreNoFollow = map[string]string{ // nolint:gochecknoglobals
		"tumblr.com": "1",
	}
)

type WorkerNode struct {
	srvr       *S
	serverAddr string
	debugMode  bool
	client     *Client
	jobItems   []string
}

func (w WorkerNode) Process(ctx context.Context, item interface{}) (interface{}, error) {
	domain := item.(string)
	RunCrawl(domain, w.serverAddr, w.debugMode)
	return domain, nil
}

func (w WorkerNode) GetItem(ctx context.Context) (interface{}, error) {
	// try popping first
	domain := w.srvr.Pop()
	if len(domain) > 0 {
		return domain, nil
	}

	// that didn't go well, try one of the job items
	if len(w.jobItems) > 0 {
		domain, w.jobItems = w.jobItems[0], w.jobItems[1:]

		return domain, nil
	}
	//
	domains, err := w.client.GetDomains()
	if err != nil {
		time.Sleep(GetDomainsRetry)

		return nil, err
	}
	// Starting crawlers is expensive, do HEAD check first
	checkedMap := HeadCheckDomains(domains, w.srvr.UserAgent)

	// only add checked domains
	for d, ok := range checkedMap {
		if !ok {
			continue
		}

		w.jobItems = append(w.jobItems, d)
	}

	if len(w.jobItems) > 0 {
		domain, w.jobItems = w.jobItems[0], w.jobItems[1:]

		return domain, nil
	}

	return nil, errors.New("could not get domain")
}

func (w WorkerNode) SubmitResult(ctx context.Context, result interface{}) error {
	// convert possible url to domain
	parsed, err := url.Parse(result.(string))
	if err != nil {
		w.client.Logger.Debugf("Could not parse: %s with err: %s", result, err)

		return nil
	}
	_, err = w.client.FilterDomains([]string{parsed.Host})
	w.client.Logger.Debugf("Crawling of %s completed with status: %+v", result, err)
	return nil
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

	target := domain
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		target = fmt.Sprintf("http://%s", domain)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
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

	// Be nice on server and skip non-resolvable domains
	for domain := range domainMap {
		addrs, err := net.LookupHost(domain)
		//
		if err != nil {
			continue
		}
		//
		if len(addrs) == 0 {
			continue
		}
		// Local filter. Some ISPs have redirects / links to policies for blocked sites
		if _, banned := BannedLocalRedirects[domain]; banned {
			continue
		}
		//
		domains = append(domains, domain)
	}

	// At this point in time domain list can be empty (broken, banned domains)
	if len(domains) == 0 {
		return
	}

	outgoing, err := client.FilterDomains(domains)
	if err != nil {
		log.Println("Filter failed with", err)
		time.Sleep(CrawlFilterRetry)

		return
	}

	// Don't crawl non-responsive domains (launching subprocess is expensive!)
	ua, err := client.GetUA()
	if err != nil {
		log.Println("Could not get UA: ", err.Error())

		return
	}

	checked := HeadCheckDomains(outgoing, ua)
	toSubmit := make([]string, 0)

	for domain, okToSubmit := range checked {
		if !okToSubmit {
			continue
		}

		toSubmit = append(toSubmit, domain)
	}

	if len(toSubmit) == 0 {
		return
	}

	SubmitOutgoingDomains(client, toSubmit, serverAddr)
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

	if mem.Total < HalfGig || mem.Free < HalfGig {
		panic("Will not start without enough RAM. At least 512M free is required")
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

	defaultOptions := []colly.CollectorOption{
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

	log.Info("CrawlDelay: ", robo.GetDelay())

	c := colly.NewCollector(
		defaultOptions...,
	)

	c.WithTransport(&http.Transport{
		DisableKeepAlives: true,
	})

	_ = c.Limit(&colly.LimitRule{
		Parallelism: Parallelism,
		// Delay is the duration to wait before creating a new request to the matching domains
		Delay: robo.GetDelay(),
		// RandomDelay is the extra randomized duration to wait added to Delay before creating a new request
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
			// check ignore map
			ignore := false
			for ending := range IgnoreNoFollow {
				if strings.HasSuffix(parsedHost, ending) {
					ignore = true

					break
				}
			}

			if !ignore {
				log.Printf("Nofollow: %s\n", absolute)

				return
			}
			log.Printf("Nofollow ignored: %s\n", absolute)
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
