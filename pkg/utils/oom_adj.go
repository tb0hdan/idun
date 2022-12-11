package utils

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"

	log "github.com/sirupsen/logrus"
)

func AdjustOOMScore(score int, logger *log.Logger) error {
	currentUser, err := user.Current()
	if err != nil {
		return err
	}
	if uid := currentUser.Uid; uid != "0" {
		logger.Errorf("not root: %s, skipping OOM adjustment", uid)
		return nil
	}

	return os.WriteFile("/proc/self/oom_score_adj", []byte(fmt.Sprint(score)), fs.ModePerm)
}
