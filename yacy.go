package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

const (
	PeerURL     = "/Network.xml?page=1&maxCount=1000"
	HostsURL    = "/HostBrowser.xml?admin=true&hosts="
	YacyTimeout = 60 * time.Second
)

type Peer struct {
	XMLName xml.Name `xml:"peer"`
	Address string   `xml:"address"`
}

type PeerResponse struct {
	XMLName xml.Name `xml:"peers"`
	Peers   []Peer   `xml:"peer"`
}

type Host struct {
	XMLName xml.Name `xml:"host"`
	Name    string   `xml:"name,attr"`
}

type Hosts struct {
	XMLName xml.Name `xml:"hosts"`
	Host    []Host   `xml:"host"`
}

type HostBrowserResponse struct {
	XMLName xml.Name `xml:"hostbrowser"`
	Hosts   Hosts    `xml:"hosts"`
}

func ParseXML(target string, outgoing interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), YacyTimeout)
	defer cancel()
	//
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	//
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	//
	data, err := ioutil.ReadAll(resp.Body)
	//
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	err = xml.Unmarshal(data, outgoing)
	if err != nil {
		return err
	}

	return nil
}

func GetHostURLs(target string) ([]string, error) {
	peersResponse := &PeerResponse{}
	err := ParseXML(target, peersResponse)
	//
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(peersResponse.Peers))

	for _, peer := range peersResponse.Peers {
		addr := fmt.Sprintf("http://%s", peer.Address)
		urls = append(urls, addr)
	}

	return urls, nil
}

func GetHostNames(target string) ([]string, error) {
	hostResponse := &HostBrowserResponse{}
	err := ParseXML(target, hostResponse)
	//
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(hostResponse.Hosts.Host))

	for _, host := range hostResponse.Hosts.Host {
		hosts = append(hosts, host.Name)
	}

	return hosts, nil
}

func GetAllRemoteHosts(remoteURLs []string, domainCh chan string) {
	wg := &sync.WaitGroup{}

	for _, remoteURL := range remoteURLs {
		wg.Add(1)

		go func(remoteURL string, wg *sync.WaitGroup) {
			fmt.Println("Sending request to ", remoteURL)
			hosts, err := GetHostNames(remoteURL)
			//
			if err != nil {
				return
			}

			for _, domain := range hosts {
				domainCh <- domain
			}

			wg.Done()
			fmt.Println("Done with ", remoteURL)
			fmt.Println(hosts)
		}(remoteURL, wg)
	}

	wg.Wait()
}

func CrawlYacyHosts(apiHost string, address string, debugMode bool, s *S) {
	domainsCh := make(chan string)

	target := apiHost + PeerURL
	hosts, err := GetHostURLs(target)
	//
	if err != nil {
		panic(err)
	}
	//
	hostURLs := make([]string, 0, len(hosts))

	for _, host := range hosts {
		hostURLs = append(hostURLs, fmt.Sprintf("%s%s", host, HostsURL))
	}

	go func() {
		for domain := range domainsCh {
			RunCrawl(domain, address, debugMode)

			// time to empty out cache
			for {
				domain := s.Pop()
				if len(domain) == 0 {
					break
				}

				RunCrawl(domain, address, debugMode)
			}
		}
	}()

	GetAllRemoteHosts(hostURLs, domainsCh)
	close(domainsCh)
}
