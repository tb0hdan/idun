package utils

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/idun/pkg/utils2"
	"github.com/tb0hdan/idun/pkg/varstruct"
)

func KillPid(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	//
	time.Sleep(varstruct.KillSleep)
	//
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func WaitAndKill(sleepTime time.Duration, pid int) {
	time.Sleep(sleepTime)
	log.Println("Run time exceeded, sending signal to ", pid)
	KillPid(pid)
}

func PIDWatcher(pid int) {
	ticker := time.NewTicker(varstruct.TickEvery)
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

		log.Printf("Parent tick for %d at %s: %v\n", pid, t, pm.Resident/varstruct.OneGig)

		if pm.Resident > varstruct.TwoGigs {
			log.Printf("Killing subprocess, memory used %d Kb > %d Kb memory allowed\n", pm.Resident/varstruct.OneK, varstruct.TwoGigs/varstruct.OneK)
			KillPid(pid)

			break
		}
	}

	ticker.Stop()
}

func RunCrawl(target, serverAddr string, debugMode bool) {
	// this will terminate process without chance to handle signal correctly
	ctx, cancel := context.WithTimeout(context.Background(), varstruct.CrawlerMaxRunTime+varstruct.CrawlerExtra)

	defer cancel()

	args := []string{
		"-url",
		target,
		"-server",
		serverAddr,
	}

	if debugMode {
		args = append(args, "-debug")
	}

	cmd := exec.CommandContext(ctx, os.Args[:1][0], args...) // nolint:gosec
	sout, _ := cmd.StdoutPipe()
	serr, _ := cmd.StderrPipe()
	err := cmd.Start()
	//
	if err != nil {
		log.Error(err)

		return
	}

	if cmd.Process != nil {
		log.Printf("PIDs: parent - %d, child - %d\n", os.Getpid(), cmd.Process.Pid)

		// Monitor memory usage
		go PIDWatcher(cmd.Process.Pid)

		// Process is up, start countdown
		go WaitAndKill(varstruct.CrawlerMaxRunTime, cmd.Process.Pid)
		//
	}

	pipes := io.MultiReader(sout, serr)
	scanner := bufio.NewScanner(pipes)
	//
	for scanner.Scan() {
		ucl := strings.ToUpper(scanner.Text())
		log.Println(ucl)
	}

	err = cmd.Wait()

	if err != nil {
		log.Errorf("Could not start crawler: %+v\n", err)
	}
}

func HeadCheck(domain string, ua string) bool {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: tr,
	}
	ctx, cancel := context.WithTimeout(context.Background(), varstruct.HeadCheckTimeout)

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

	for _, domain := range utils2.DeduplicateSlice(domains) {
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
