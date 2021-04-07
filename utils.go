package idun

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/hydra"
)

type JSONResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type DomainsResponse struct {
	Domains []string `json:"domains"`
}

func KillPid(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	//
	time.Sleep(KillSleep)
	//
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func WaitAndKill(sleepTime time.Duration, pid int) {
	time.Sleep(sleepTime)
	log.Println("Run time exceeded, sending signal to ", pid)
	KillPid(pid)
}

func PIDWatcher(pid int) {
	ticker := time.NewTicker(TickEvery)
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

		log.Printf("Parent tick for %d at %s: %v\n", pid, t, pm.Resident/OneGig)

		if pm.Resident > TwoGigs {
			log.Printf("Killing subprocess, memory used %d Kb > %d Kb memory allowed\n", pm.Resident/OneK, TwoGigs/OneK)
			KillPid(pid)

			break
		}
	}

	ticker.Stop()
}

func RunCrawl(target, serverAddr string, debugMode bool) {
	// this will terminate process without chance to handle signal correctly
	ctx, cancel := context.WithTimeout(context.Background(), CrawlerMaxRunTime+CrawlerExtra)

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
		go WaitAndKill(CrawlerMaxRunTime, cmd.Process.Pid)
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

func RunWithAPI(client *Client, address string, debugMode bool, srvr *S) {
	workerCount, err := CalculateMaxWorkers()
	if err != nil {
		client.Logger.Fatal("Could not calculate worker amount")
	}
	client.Logger.Debugf("Will use up to %d workers", workerCount)
	wn := WorkerNode{
		serverAddr: address,
		srvr:       srvr,
		debugMode:  debugMode,
		client:     client,
	}
	pool := hydra.New(context.Background(), int(workerCount), wn, client.Logger)
	pool.Run()
}
