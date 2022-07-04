package utils

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
	log "github.com/sirupsen/logrus"

	"github.com/tb0hdan/idun/pkg/types"
)

func DeduplicateSlice(incoming []string) (outgoing []string) {
	hash := make(map[string]int)
	outgoing = make([]string, 0)
	//
	for _, value := range incoming {
		if _, ok := hash[value]; !ok {
			hash[value] = 1

			outgoing = append(outgoing, value)
		}
	}
	//
	return
}

func KillPid(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	//
	time.Sleep(types.KillSleep)
	//
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func WaitAndKill(sleepTime time.Duration, pid int) {
	time.Sleep(sleepTime)
	log.Println("Run time exceeded, sending signal to ", pid)
	KillPid(pid)
}

func PIDWatcher(pid int) {
	ticker := time.NewTicker(types.TickEvery)
	for t := range ticker.C {
		pm := sigar.ProcMem{}
		err := pm.Get(pid)
		//
		if err != nil && err.Error() != "no such process" {
			log.Error("PIDWatcher ", err)

			break
		}

		if err != nil {
			// process doesn't exit
			break
		}

		log.Printf("Parent tick for %d at %s: %v\n", pid, t, pm.Resident/types.OneGig)

		if pm.Resident > types.TwoGigs {
			log.Printf("Killing subprocess, memory used %d Kb > %d Kb memory allowed\n", pm.Resident/types.OneK, types.TwoGigs/types.OneK)
			KillPid(pid)

			break
		}
	}

	ticker.Stop()
}

func HeadCheck(domain string, ua string) bool {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: tr,
	}
	ctx, cancel := context.WithTimeout(context.Background(), types.HeadCheckTimeout)

	defer cancel()

	target := domain
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		target = fmt.Sprintf("http://%s", domain)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	//
	if err != nil {
		return false
	}

	req.Header.Add("User-Agent", ua)

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

func HeadCheckDomains(domains []string, ua string) map[string]bool {
	results := make(map[string]bool)
	wg := &sync.WaitGroup{}
	lock := &sync.RWMutex{}

	for _, domain := range DeduplicateSlice(domains) {
		wg.Add(1)

		go func(domain string, wg *sync.WaitGroup) {
			result := HeadCheck(domain, ua)

			lock.Lock()
			results[domain] = result
			lock.Unlock()
			wg.Done()
		}(domain, wg)
	}

	wg.Wait()

	return results
}
