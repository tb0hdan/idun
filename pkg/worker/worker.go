package worker

import (
	"context"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/tb0hdan/idun/pkg/client"
	"github.com/tb0hdan/idun/pkg/server"
	"github.com/tb0hdan/idun/pkg/utils"
	"github.com/tb0hdan/idun/pkg/varstruct"
)

type WorkerNode struct {
	Srvr       *server.S
	ServerAddr string
	DebugMode  bool
	C          *client.Client
	jobItems   []string
}

func (w WorkerNode) Process(ctx context.Context, item interface{}) (interface{}, error) {
	domain := item.(string)
	utils.RunCrawl(domain, w.ServerAddr, w.DebugMode)
	return domain, nil
}

func (w WorkerNode) GetItem(ctx context.Context) (interface{}, error) {
	// try popping first
	domain := w.Srvr.Pop()
	if len(domain) > 0 {
		return domain, nil
	}

	// that didn't go well, try one of the job items
	if len(w.jobItems) > 0 {
		domain, w.jobItems = w.jobItems[0], w.jobItems[1:]

		return domain, nil
	}
	//
	domains, err := w.C.GetDomains()
	if err != nil {
		time.Sleep(varstruct.GetDomainsRetry)

		return nil, err
	}
	// Starting crawlers is expensive, do HEAD check first
	checkedMap := utils.HeadCheckDomains(domains, w.Srvr.UserAgent)

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
		w.C.Logger.Debugf("Could not parse: %s with err: %s", result, err)

		return nil
	}
	_, err = w.C.FilterDomains([]string{parsed.Host})
	w.C.Logger.Debugf("Crawling of %s completed with status: %+v", result, err)
	return nil
}
