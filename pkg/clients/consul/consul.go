package consul

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"

	"github.com/tb0hdan/idun/pkg/clients/apiclient"
)

const (
	ConsulAdvertisedPort    = 80
	ConsulAdvertisedService = "idun"
	Environment             = "test"
	ErrMsg                  = "Consul registration aborted."
)

type ConsulRegistration struct {
	ID      string   `json:"ID"`
	Name    string   `json:"Name,omitempty"`
	Address string   `json:"Address,omitempty"`
	Port    int      `json:"Port,omitempty"`
	Tags    []string `json:"Tags,omitempty"`
}

type Client struct {
	consulURL             string
	logger                *log.Logger
	advertisedPort        int
	advertisedServiceName string
}

func (cc *Client) getID() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		log.Error("Could not get hostname." + ErrMsg)

		return "", err
	}

	return fmt.Sprintf("%s_%s_%s", Environment, hostName, "idun"), nil
}

func (cc *Client) SetAdvertisedPort(port int) {
	cc.advertisedPort = port
}

func (cc *Client) SetServiceName(serviceName string) {
	cc.advertisedServiceName = serviceName
}

func (cc *Client) Register() { // nolint:funlen
	retryClient := apiclient.PrepareClient(cc.logger)
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
	if cc.advertisedPort == 0 {
		cc.advertisedPort = ConsulAdvertisedPort
	}

	if len(cc.advertisedServiceName) == 0 {
		cc.advertisedServiceName = ConsulAdvertisedService
	}
	//
	// Use first one. Works for Docker. Maybe will be fixed later for host systems.
	request := &ConsulRegistration{
		ID:      ID,
		Name:    cc.advertisedServiceName,
		Address: validAddrs[0],
		Port:    cc.advertisedPort,
		Tags:    []string{Environment, "worker"},
	}

	data, err := json.Marshal(request)
	if err != nil {
		log.Error("Could not marshal request." + ErrMsg)

		return
	}

	req, err := retryablehttp.NewRequest("PUT", cc.consulURL+"/v1/agent/service/register",
		strings.NewReader(string(data)))
	if err != nil {
		log.Error("Could not prepare retryable apiclient." + ErrMsg)

		return
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Length", strconv.Itoa(len(data)))

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

func (cc *Client) Deregister() {
	ID, err := cc.getID()
	if err != nil {
		log.Error("Could not get host ID." + ErrMsg)

		return
	}
	//
	retryClient := apiclient.PrepareClient(cc.logger)
	req, err := retryablehttp.NewRequest("PUT",
		fmt.Sprintf(cc.consulURL+"/v1/agent/service/deregister/%s", ID), nil)
	//
	if err != nil {
		log.Error("Could not prepare retryable apiclient." + ErrMsg)

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

func (cc *Client) GetServices() {
	retryClient := apiclient.PrepareClient(cc.logger)

	req, err := retryablehttp.NewRequest("GET", cc.consulURL+"v1/agent/services", nil)
	if err != nil {
		log.Error("Could not prepare retryable apiclient." + ErrMsg)

		return
	}
	//
	resp, err := retryClient.Do(req)
	//
	if err != nil {
		log.Error("Could not process request." + ErrMsg)

		return
	}
	defer resp.Body.Close()
}

func NewConsul(consulURL string, logger *log.Logger) *Client {
	return &Client{
		consulURL: consulURL,
		logger:    logger,
	}
}
