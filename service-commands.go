//
// Copyright (c) 2015-2024 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.
//

package madmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// ServiceRestart - restarts the MinIO cluster
func (adm *AdminClient) ServiceRestart(ctx context.Context) error {
	_, err := adm.serviceCallAction(ctx, ServiceActionOpts{Action: ServiceActionRestart})
	return err
}

// ServiceStop - stops the MinIO cluster
func (adm *AdminClient) ServiceStop(ctx context.Context) error {
	_, err := adm.serviceCallAction(ctx, ServiceActionOpts{Action: ServiceActionStop})
	return err
}

// ServiceFreeze - freezes all incoming S3 API calls on MinIO cluster
func (adm *AdminClient) ServiceFreeze(ctx context.Context) error {
	_, err := adm.serviceCallAction(ctx, ServiceActionOpts{Action: ServiceActionFreeze})
	return err
}

// ServiceUnfreeze - un-freezes all incoming S3 API calls on MinIO cluster
func (adm *AdminClient) ServiceUnfreeze(ctx context.Context) error {
	_, err := adm.serviceCallAction(ctx, ServiceActionOpts{Action: ServiceActionUnfreeze})
	return err
}

// ServiceAction - type to restrict service-action values
type ServiceAction string

const (
	// ServiceActionRestart represents restart action
	ServiceActionRestart ServiceAction = "restart"
	// ServiceActionStop represents stop action
	ServiceActionStop = "stop"
	// ServiceActionFreeze represents freeze action
	ServiceActionFreeze = "freeze"
	// ServiceActionUnfreeze represents unfreeze a previous freeze action
	ServiceActionUnfreeze = "unfreeze"
)

// ServiceActionOpts specifies the action that the service is requested
// to take, dryRun indicates if the action is a no-op, force indicates
// that server must make best effort to restart the process.
type ServiceActionOpts struct {
	Action ServiceAction
	DryRun bool
}

// ServiceActionPeerResult service peer result
type ServiceActionPeerResult struct {
	Host          string                `json:"host"`
	Err           string                `json:"err,omitempty"`
	WaitingDrives map[string]DiskStatus `json:"waitingDrives,omitempty"`
}

// ServiceActionResult service action result
type ServiceActionResult struct {
	Action  ServiceAction             `json:"action"`
	DryRun  bool                      `json:"dryRun"`
	Results []ServiceActionPeerResult `json:"results,omitempty"`
}

// ServiceAction - specify the type of service action that we are requesting the server to perform
func (adm *AdminClient) ServiceAction(ctx context.Context, opts ServiceActionOpts) (ServiceActionResult, error) {
	return adm.serviceCallAction(ctx, opts)
}

// serviceCallAction - call service restart/stop/freeze/unfreeze
func (adm *AdminClient) serviceCallAction(ctx context.Context, opts ServiceActionOpts) (ServiceActionResult, error) {
	queryValues := url.Values{}
	queryValues.Set("action", string(opts.Action))
	queryValues.Set("dry-run", strconv.FormatBool(opts.DryRun))
	queryValues.Set("type", "2")

	// Request API to Restart server
	resp, err := adm.executeMethod(ctx,
		http.MethodPost, requestData{
			relPath:     adminAPIPrefix + "/service",
			queryValues: queryValues,
		},
	)
	defer closeResponse(resp)
	if err != nil {
		return ServiceActionResult{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ServiceActionResult{}, httpRespToErrorResponse(resp)
	}

	srvRes := ServiceActionResult{}
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&srvRes); err != nil {
		return ServiceActionResult{}, err
	}

	return srvRes, nil
}
