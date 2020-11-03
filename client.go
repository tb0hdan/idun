package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
)

func PrepareClient(logger *log.Logger) *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	// DefaultClient uses DefaultTransport which in turn has idle connections and keepalives disabled.
	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.RetryMax = APIRetryMax
	retryClient.Logger = logger

	return retryClient
}

type Client struct {
	APIBase string
	Key     string
	Logger  *log.Logger
}

func (c *Client) GetUA() (string, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, c.APIBase+"/ua", nil)
	//
	if err != nil {
		return "", err
	}
	//
	req.Header.Add("X-Session-Token", c.Key)
	//
	retryClient := PrepareClient(c.Logger)
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

func (c *Client) GetDomains() ([]string, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, c.APIBase+"/domains", nil)
	//
	if err != nil {
		return nil, err
	}
	//
	req.Header.Add("X-Session-Token", c.Key)
	//
	retryClient := PrepareClient(c.Logger)
	resp, err := retryClient.Do(req)
	//
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	domainsResponse := &DomainsResponse{}

	err = json.NewDecoder(resp.Body).Decode(domainsResponse)
	if err != nil {
		return nil, err
	}

	if len(domainsResponse.Domains) == 0 {
		return nil, errors.New("empty response") // nolint:goerr113
	}

	return domainsResponse.Domains, nil
}

func (c *Client) FilterDomains(incoming []string) (outgoing []string, err error) {
	log.Println("Filter called: ", incoming)
	var (
		domainsRequest  DomainsResponse
		domainsResponse DomainsResponse
	)

	domainsRequest.Domains = DeduplicateSlice(incoming)

	data, err := json.Marshal(&domainsRequest)
	if err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, c.APIBase+"/filter", data)
	//
	if err != nil {
		return nil, err
	}
	//
	req.Header.Add("X-Session-Token", c.Key)
	//
	retryClient := PrepareClient(c.Logger)
	resp, err := retryClient.Do(req)
	//
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&domainsResponse)
	if err != nil {
		return nil, err
	}

	log.Println("Filtered domains: ", domainsResponse.Domains)
	return domainsResponse.Domains, nil
}
