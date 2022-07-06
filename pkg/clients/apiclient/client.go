package apiclient

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"

	"github.com/tb0hdan/idun/pkg/types"
	"github.com/tb0hdan/idun/pkg/utils"
)

func PrepareClient(logger *log.Logger) *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	// DefaultClient uses DefaultTransport which in turn has idle connections and keepalives disabled.
	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.RetryMax = types.APIRetryMax
	retryClient.Logger = logger

	return retryClient
}

type Client struct {
	APIBase          string
	Key              string
	Logger           *log.Logger
	CustomDomainsURL string
}

func (c *Client) Fatal(args ...interface{}) {
	c.Logger.Fatal(args...)
}

func (c *Client) Debugf(format string, args ...interface{}) {
	c.Logger.Debugf(format, args...)
}

func (c *Client) GetLogger() *log.Logger {
	return c.Logger
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

	message := &types.JSONResponse{}
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
	reqURL := c.APIBase + "/domains"
	if len(c.CustomDomainsURL) != 0 {
		reqURL = c.CustomDomainsURL
	}
	req, err := retryablehttp.NewRequest(http.MethodGet, reqURL, nil)
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

	domainsResponse := &types.DomainsResponse{}

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
	var (
		domainsRequest  types.DomainsResponse
		domainsResponse types.DomainsResponse
	)

	domainsRequest.Domains = utils.DeduplicateSlice(incoming)

	// Don't hammer API with empty requests
	if len(domainsRequest.Domains) == 0 {
		return outgoing, nil
	}

	log.Println("Filter called: ", incoming)

	data, err := json.Marshal(&domainsRequest)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(data); err != nil {
		return nil, err
	}
	if err = g.Close(); err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, c.APIBase+"/filter", &buf)
	//
	if err != nil {
		return nil, err
	}
	//
	req.Header.Add("X-Session-Token", c.Key)
	req.Header.Add("Content-Encoding", "gzip")
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

	outgoing = domainsResponse.Domains

	log.Println("Filtered domains: ", outgoing)

	return outgoing, nil
}
