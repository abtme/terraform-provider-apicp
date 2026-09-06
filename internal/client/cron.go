package client

import "fmt"

// CronJob mirrors apicp's internal/cron.CronJob definition fields plus
// the live-queried run-status fields apicp's response embeds alongside
// them (internal/api/cron.go's cronJobResponse) — apicp's own store
// never persists run history, so these three are always freshly read
// from the node on every response.
type CronJob struct {
	ID       string `json:"id"`
	VhostID  string `json:"vhost_id"`
	UnixUser string `json:"unix_user"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`

	LastRunAt    string `json:"last_run_at,omitempty"`
	LastExitCode *int64 `json:"last_exit_code,omitempty"`
	LastOutput   string `json:"last_output,omitempty"`
}

type cronJobCreateRequest struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

type cronJobPatchRequest struct {
	Schedule *string `json:"schedule,omitempty"`
	Command  *string `json:"command,omitempty"`
}

func (c *Client) CreateCronJob(vhostID, schedule, command string) (*CronJob, error) {
	var j CronJob
	path := fmt.Sprintf("/v1/vhosts/%s/cron-jobs", vhostID)
	if err := c.Post(path, cronJobCreateRequest{Schedule: schedule, Command: command}, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (c *Client) GetCronJob(vhostID, id string) (*CronJob, error) {
	var j CronJob
	path := fmt.Sprintf("/v1/vhosts/%s/cron-jobs/%s", vhostID, id)
	if err := c.Get(path, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (c *Client) UpdateCronJob(vhostID, id string, schedule, command *string) (*CronJob, error) {
	var j CronJob
	path := fmt.Sprintf("/v1/vhosts/%s/cron-jobs/%s", vhostID, id)
	if err := c.Patch(path, cronJobPatchRequest{Schedule: schedule, Command: command}, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (c *Client) DeleteCronJob(vhostID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/vhosts/%s/cron-jobs/%s", vhostID, id))
}
