package notification

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"

	commonhttp "github.com/goharbor/harbor/src/common/http"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/jobservice/logger"
)

// Max retry has the same meaning as max fails.
const maxFails = "JOBSERVICE_WEBHOOK_JOB_MAX_RETRY"

// WebhookJob implements the job interface, which send notification by http or https.
type WebhookJob struct {
	client *http.Client
	logger logger.Interface
	ctx    job.Context
}

// MaxFails returns that how many times this job can fail, get this value from ctx.
func (wj *WebhookJob) MaxFails() uint {
	if maxFails, exist := os.LookupEnv(maxFails); exist {
		result, err := strconv.ParseUint(maxFails, 10, 32)
		// Unable to log error message because the logger isn't initialized when calling this function.
		if err == nil {
			return uint(result)
		}
	}

	// Default max fails count is 10, and its max retry interval is around 3h
	// Large enough to ensure most situations can notify successfully
	return 10
}

// MaxCurrency is implementation of same method in Interface.
func (wj *WebhookJob) MaxCurrency() uint {
	return 0
}

// ShouldRetry ...
func (wj *WebhookJob) ShouldRetry() bool {
	return true
}

// Validate implements the interface in job/Interface
func (wj *WebhookJob) Validate(params job.Parameters) error {
	return nil
}

// Run implements the interface in job/Interface
func (wj *WebhookJob) Run(ctx job.Context, params job.Parameters) error {
	if err := wj.init(ctx, params); err != nil {
		return err
	}

	// does not throw err in the notification job
	if err := wj.execute(ctx, params); err != nil {
		wj.logger.Error(err)
	}

	return nil
}

// init webhook job
func (wj *WebhookJob) init(ctx job.Context, params map[string]interface{}) error {
	wj.logger = ctx.GetLogger()
	wj.ctx = ctx

	// default use insecure transport
	tr := commonhttp.GetHTTPTransport(commonhttp.InsecureTransport)
	if v, ok := params["skip_cert_verify"]; ok {
		if insecure, ok := v.(bool); ok {
			if insecure {
				tr = commonhttp.GetHTTPTransport(commonhttp.SecureTransport)
			}
		}
	}
	wj.client = &http.Client{
		Transport: tr,
	}

	return nil
}

// execute webhook job
func (wj *WebhookJob) execute(ctx job.Context, params map[string]interface{}) error {
	payload := params["payload"].(string)
	address := params["address"].(string)

	req, err := http.NewRequest(http.MethodPost, address, bytes.NewReader([]byte(payload)))
	if err != nil {
		return err
	}
	if v, ok := params["auth_header"]; ok && len(v.(string)) > 0 {
		req.Header.Set("Authorization", v.(string))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := wj.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook job(target: %s) response code is %d", address, resp.StatusCode)
	}

	return nil
}
