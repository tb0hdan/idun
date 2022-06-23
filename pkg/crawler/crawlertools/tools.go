package crawlertools

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/tb0hdan/idun/pkg/types"
	"github.com/tb0hdan/idun/pkg/utils"
)

func RunCrawl(apiBase, target, serverAddr string, debugMode bool) {
	// this will terminate process without chance to handle signal correctly
	ctx, cancel := context.WithTimeout(context.Background(), types.CrawlerMaxRunTime+types.CrawlerExtra)

	defer cancel()

	args := []string{
		"-apiBase",
		apiBase,
		"-url",
		target,
		"-servers",
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
		go utils.PIDWatcher(cmd.Process.Pid)

		// Process is up, start countdown
		go utils.WaitAndKill(types.CrawlerMaxRunTime, cmd.Process.Pid)
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
