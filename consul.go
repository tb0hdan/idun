package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
)

const (
	ConsulAdvertisedPort = 80
)

var (
	Environment = "test"                         // nolint:gochecknoglobals
	ErrMsg      = "Consul registration aborted." // nolint:gochecknoglobals
)

type ConsulClient struct {
	consulURL string
	logger    *log.Logger
}

func (cc *ConsulClient) getID() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		log.Error("Could not get hostname." + ErrMsg)

		return "", err
	}

	return fmt.Sprintf("%s_%s_%s", Environment, hostName, "idun"), nil
}

func (cc *ConsulClient) Register() { // nolint:funlen
	retryClient := PrepareClient(cc.logger)
	addrs, err := net.InterfaceAddrs()
	//
	if err != nil {
		log.Error("Could not get interface list." + ErrMsg)

		return
	}

	validAddrs := make([]string, 0)
	//
	for _, addr := range addrs {
		if addr.Network() != "ip+net" {
			continue
		}
		//
		if addr.String() == "127.0.0.1/8" {
			continue
		}
		//
		ipAddr, _, err := net.ParseCIDR(addr.String())
		//
		if err != nil {
			continue
		}

		validAddrs = append(validAddrs, ipAddr.String())
	}
	//
	ID, err := cc.getID()
	//
	if err != nil {
		log.Error("Could not get host ID." + ErrMsg)

		return
	}
	//
	data := url.Values{
		"ID":   []string{ID},
		"Name": []string{"idun"},
		// Use first one. Works for Docker. Maybe will be fixed later for host systems.
		"Address": []string{validAddrs[0]},
		"Port":    []string{fmt.Sprintf("%d", ConsulAdvertisedPort)},
		"Tags":    []string{fmt.Sprintf("%s,%s", Environment, "worker")},
	}

	req, err := retryablehttp.NewRequest("PUT", cc.consulURL+"/v1/agent/service/register",
		strings.NewReader(data.Encode()))
	if err != nil {
		log.Error("Could not prepare retryable client." + ErrMsg)

		return
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	resp, err := retryClient.Do(req)
	if err != nil {
		log.Error("Could not process request." + ErrMsg)

		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Println("Consul registration succeeded")

		return
	}
	//
	log.Errorf("Got error code %d while registering."+ErrMsg, resp.StatusCode)
}

func (cc *ConsulClient) Deregister() {
	ID, err := cc.getID()
	if err != nil {
		log.Error("Could not get host ID." + ErrMsg)

		return
	}
	//
	retryClient := PrepareClient(cc.logger)
	req, err := retryablehttp.NewRequest("PUT",
		fmt.Sprintf(cc.consulURL+"/v1/agent/service/deregister/%s", ID), nil)
	//
	if err != nil {
		log.Error("Could not prepare retryable client." + ErrMsg)

		return
	}

	resp, err := retryClient.Do(req)
	if err != nil {
		log.Error("Could not process request." + ErrMsg)

		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Println("Consul deregistration succeeded")

		return
	}
	//
	log.Errorf("Got error code %d while deregestering."+ErrMsg, resp.StatusCode)
}

func NewConsul(consulURL string, logger *log.Logger) *ConsulClient {
	return &ConsulClient{
		consulURL: consulURL,
		logger:    logger,
	}
}
