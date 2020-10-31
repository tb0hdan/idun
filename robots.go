package main

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/temoto/robotstxt"
)

type RoboTester struct {
	robots    *robotstxt.RobotsData
	userAgent string
}

func (rt *RoboTester) GetRobots(path string) (robots *robotstxt.RobotsData, err error) {
	parsed, err := url.Parse(path)
	if err != nil {
		return &robotstxt.RobotsData{}, err
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsed.Scheme, parsed.Host)

	client := &http.Client{}

	req, err := http.NewRequest("GET", robotsURL, nil)
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
	if !rt.robots.TestAgent(path, "domainsproject.org") || !rt.robots.TestAgent(path, "Domains Project") {
		return false
	}

	return true
}

func NewRoboTester(fullURL, userAgent string) (*RoboTester, error) {
	tester := &RoboTester{userAgent: userAgent}
	robots, err := tester.GetRobots(fullURL)

	if err == nil {
		tester.robots = robots
		return tester, nil
	}

	return &RoboTester{}, err
}
