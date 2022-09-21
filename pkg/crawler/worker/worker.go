package worker

import (
	"context"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/tb0hdan/idun/pkg/crawler/connection"
	"github.com/tb0hdan/idun/pkg/crawler/crawlertools"
	"github.com/tb0hdan/idun/pkg/types"
	"github.com/tb0hdan/idun/pkg/utils"
)

type WorkerNode struct {
	ApiBase     string
	Srvr        types.APIServerInterface
	ServerAddr  string
	DebugMode   bool
	C           types.APIClientInterface
	jobItems    []string
	ConnTracker *connection.Tracker
}

func (w WorkerNode) Process(ctx context.Context, item interface{}) (interface{}, error) {
	domain := item.(string)
	/*
		if !w.ConnTracker.Check(domain) {
			w.C.Debugf("Connection check for %s exceeds limit, skipping further processing...")
			return domain, nil
		} */
	crawlertools.RunCrawl(w.ApiBase, domain, w.ServerAddr, w.DebugMode)
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
		time.Sleep(types.GetDomainsRetry)

		return nil, err
	}
	// Starting crawlers is expensive, do HEAD check first
	checkedMap := utils.HeadCheckDomains(domains, w.Srvr.GetUA())

	// only add checked domains
	for d := range checkedMap {
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
		w.C.Debugf("Could not parse: %s with err: %s", result, err)

		return nil
	}
	// might be empty
	if len(parsed.Host) == 0 {
		return nil
	}
	_, err = w.C.FilterDomains([]string{parsed.Host})
	w.C.Debugf("Crawling of %s completed with status: %+v", result, err)
	return nil
}
