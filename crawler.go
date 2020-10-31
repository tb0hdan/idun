package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
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
)

func HeadCheck(domain string) bool {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, fmt.Sprintf("http://%s", domain), nil)
	if err != nil {
		return false
	}

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

func HeadCheckDomains(domains []string) map[string]bool {
	results := make(map[string]bool)
	wg := &sync.WaitGroup{}
	for _, domain := range domains {
		go func(domain string) {
			wg.Add(1)
			results[domain] = HeadCheck(domain)
			wg.Done()
		}(domain)
	}
	wg.Wait()
	return results
}

func SubmitOutgoingDomains(client *Client, domains []string, serverAddr string) {
	log.Println("Submit called: ", domains)
	if len(domains) == 0 {
		return
	}

	var domainsRequest DomainsResponse

	domainsRequest.Domains = domains
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
		return "", errors.New("non-ok response") // nolint:goerr113
	}
	log.Println("UA: ", message.Message)
	return message.Message, nil
}

func CrawlURL(client *Client, targetURL string, debugMode bool, serverAddr string) {
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

	defaultOptions := []func(collector *colly.Collector){
		colly.Async(true),
		colly.UserAgent(ua),
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

	c.Limit(&colly.LimitRule{
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
		if !strings.HasSuffix(parsedHost, allowedDomain) {
			// external links
			if len(domainMap) < MaxDomainsInMap {
				if _, ok := domainMap[parsedHost]; !ok {
					domainMap[parsedHost] = true
				}

				return
			}
			domains := make([]string, 0, len(domainMap))

			for domain, _ := range domainMap {
				domains = append(domains, domain)
			}

			outgoing, err := client.FilterDomains(domains)
			if err != nil {
				log.Println("Filter failed with", err)
				time.Sleep(CrawlFilterRetry)

				return
			}
			SubmitOutgoingDomains(client, outgoing, serverAddr)

			domainMap = make(map[string]bool)

			return
		}

		if !robo.Test(absolute) {
			log.Errorf("Crawling of %s is disallowed by robots.txt", absolute)

			return
		}

		c.Visit(absolute)
	})

	c.OnRequest(func(r *colly.Request) {
		if debugMode {
			fmt.Println("Visiting", r.URL.String())
		}
	})

	ticker := time.NewTicker(TickEvery)

	go func() {
		c.Wait()
		done <- true
	}()

	go func() {
		for t := range ticker.C {
			mem := sigar.ProcMem{}
			mem.Get(os.Getpid())
			fmt.Println("Tick at", t, mem.Resident/OneGig)
			runtime.GC()

			if mem.Resident > TwoGigs {
				// 2Gb MAX
				done <- true
			}
		}
	}()

	if !robo.Test(targetURL + "/") {
		log.Errorf("Crawling of / for %s is disallowed by robots.txt", targetURL)

		return
	}
	c.Visit(targetURL)
	<-done
	ticker.Stop()
}
