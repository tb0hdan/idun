package robots

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/temoto/robotstxt"
)

const (
	RobotsTimeout = 10 * time.Second
)

type RoboTester struct {
	robots    *robotstxt.RobotsData
	userAgent string
	fullURL   string
	gotRobots bool
}

func (rt *RoboTester) GetRobots(path string) (robots *robotstxt.RobotsData, err error) {
	parsed, err := url.Parse(path)
	if err != nil {
		return &robotstxt.RobotsData{}, err
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsed.Scheme, parsed.Host)

	client := &http.Client{}

	ctx, cancel := context.WithTimeout(context.Background(), RobotsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return &robotstxt.RobotsData{}, err
	}

	req.Header.Set("User-Agent", rt.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return &robotstxt.RobotsData{}, err
	}

	defer resp.Body.Close()

	robots, err = robotstxt.FromResponse(resp)
	resp.Body.Close()

	return robots, err
}

func (rt *RoboTester) Test(path string) bool {
	if rt.gotRobots {
		return !rt.robots.TestAgent(path, "domainsproject.org") || !rt.robots.TestAgent(path, "Domains Project")
	}

	return true
}

// GetDelay - be as careful as possible, if there are two definitions - sum them up and use both
func (rt *RoboTester) GetDelay() time.Duration {
	if rt.gotRobots {
		group1 := rt.robots.FindGroup("domainsproject.org")
		group2 := rt.robots.FindGroup("Domains Project")
		return group1.CrawlDelay + group2.CrawlDelay
	}
	return 0 * time.Second
}

func (rt *RoboTester) InitWithUA(ua string) {
	rt.userAgent = ua
	robots, err := rt.GetRobots(rt.fullURL)

	if err == nil {
		rt.robots = robots
		rt.gotRobots = true
	}
}

func NewRoboTester(fullURL string) *RoboTester {
	return &RoboTester{fullURL: fullURL}
}
