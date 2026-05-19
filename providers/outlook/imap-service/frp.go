package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func startFRP(ctx context.Context, errCh chan<- error) error {
	if !envBool("FRP_ENABLE", false) {
		return nil
	}
	configFile := envStr("FRP_CONFIG_FILE", defaultFRPConfigFile)
	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("read FRP_CONFIG_FILE %s: %w", configFile, err)
	}

	binary := envStr("FRP_BINARY", defaultFRPBinary)
	cmd := exec.CommandContext(ctx, binary, "-c", configFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start frpc: %w", err)
	}

	logInfo("started embedded frpc pid=%d config=%s", cmd.Process.Pid, configFile)
	go func() {
		err := cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			errCh <- fmt.Errorf("frpc exited: %w", err)
			return
		}
		errCh <- errors.New("frpc exited")
	}()
	return nil
}
