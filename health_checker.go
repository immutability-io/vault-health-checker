package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	log "github.com/hashicorp/go-hclog"
)

const (
	vaultHealthCheckResponseActive        = 200
	vaultHealthCheckResponseStandby       = 429
	vaultHealthCheckResponseDRSecondary   = 472
	vaultHealthCheckResponseSealed        = 503
	vaultHealthCheckResponseUninitialized = 501
)

func statusCodeString(statusCode int64) string {
	return strconv.FormatInt(statusCode, 10)
}

type vaultHealthChecker struct {
	vaultAddr     *url.URL
	checkInterval time.Duration

	statusChange   chan<- vaultStatus
	previousStatus *vaultStatus

	client *http.Client
	logger log.Logger
}

func newVaultHealthChecker(vaultBaseAddr string, checkInterval time.Duration,
	logger log.Logger, statusChange chan<- vaultStatus) (*vaultHealthChecker, error) {
	vaultAddr, err := url.Parse(vaultBaseAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid Vault base address: %s", err)
	}
	vaultAddr.Path = "v1/sys/health"

	query := vaultAddr.Query()
	query.Set("activecode", statusCodeString(vaultHealthCheckResponseActive))
	query.Set("standbycode", statusCodeString(vaultHealthCheckResponseStandby))
	query.Set("drsecondarycode", statusCodeString(vaultHealthCheckResponseDRSecondary))
	query.Set("sealedcode", statusCodeString(vaultHealthCheckResponseSealed))
	query.Set("uninitcode", statusCodeString(vaultHealthCheckResponseUninitialized))

	return &vaultHealthChecker{
		vaultAddr:      vaultAddr,
		checkInterval:  checkInterval,
		statusChange:   statusChange,
		previousStatus: nil,
		client:         cleanhttp.DefaultClient(),
		logger:         logger,
	}, nil
}

func (hc *vaultHealthChecker) run() {
	for {
		req, err := http.NewRequest(http.MethodHead, hc.vaultAddr.String(), nil)
		if err != nil {
			hc.logger.Debug(fmt.Sprintf("error constructing request: %s", err))
			time.Sleep(hc.checkInterval)
			continue
		}

		resp, err := hc.client.Do(req)
		if err != nil {
			hc.logger.Debug(fmt.Sprintf("Error making health check request: %s", err))
			hc.sendStatus(vaultStatusUnhealthy)
			time.Sleep(hc.checkInterval)
			continue
		}
		resp.Body.Close()

		hc.logger.Debug(fmt.Sprintf("Health Check Request Status: %d", resp.StatusCode))
		switch resp.StatusCode {
		case vaultHealthCheckResponseActive:
			hc.sendStatus(vaultStatusActive)
		case vaultHealthCheckResponseStandby:
			hc.sendStatus(vaultStatusStandby)
		default:
			hc.sendStatus(vaultStatusUnhealthy)
		}

		time.Sleep(hc.checkInterval)
	}
}

func (hc *vaultHealthChecker) sendStatus(status vaultStatus) {
	if hc.previousStatus != nil && *hc.previousStatus == status {
		return
	}
	hc.previousStatus = &status
	hc.statusChange <- status
}
