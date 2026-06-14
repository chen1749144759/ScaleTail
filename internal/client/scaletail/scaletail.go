// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package scaletail provides a minimal control plane API client for internal
// use. The internal client keeps tools on the local ScaleTail package without
// importing the larger public SDK.
package scaletail

import (
	"errors"
	"io"
	"net/http"

	tsclient "scaletail.com/client/scaletail"
)

// maxSize is the maximum read size (10MB) of responses from the server.
const maxReadSize = 10 << 20

func init() {
	tsclient.I_Acknowledge_This_API_Is_Unstable = true
}

// AuthMethod is an alias to scaletail.com/client/scaletail.
type AuthMethod = tsclient.AuthMethod

// APIKey is an alias to scaletail.com/client/scaletail.
type APIKey = tsclient.APIKey

// Device is an alias to scaletail.com/client/scaletail.
type Device = tsclient.Device

// DeviceFieldsOpts is an alias to scaletail.com/client/scaletail.
type DeviceFieldsOpts = tsclient.DeviceFieldsOpts

// Key is an alias to scaletail.com/client/scaletail.
type Key = tsclient.Key

// KeyCapabilities is an alias to scaletail.com/client/scaletail.
type KeyCapabilities = tsclient.KeyCapabilities

// KeyDeviceCapabilities is an alias to scaletail.com/client/scaletail.
type KeyDeviceCapabilities = tsclient.KeyDeviceCapabilities

// KeyDeviceCreateCapabilities is an alias to scaletail.com/client/scaletail.
type KeyDeviceCreateCapabilities = tsclient.KeyDeviceCreateCapabilities

// ErrResponse is an alias to scaletail.com/client/scaletail.
type ErrResponse = tsclient.ErrResponse

// NewClient is an alias to scaletail.com/client/scaletail.
func NewClient(tailnet string, auth AuthMethod) *Client {
	return &Client{
		Client: tsclient.NewClient(tailnet, auth),
	}
}

// Client is a wrapper of scaletail.com/client/scaletail.
type Client struct {
	*tsclient.Client
}

// HandleErrorResponse is an alias to scaletail.com/client/scaletail.
func HandleErrorResponse(b []byte, resp *http.Response) error {
	return tsclient.HandleErrorResponse(b, resp)
}

// SendRequest add the authentication key to the request and sends it. It
// receives the response and reads up to 10MB of it.
func SendRequest(c *Client, req *http.Request) ([]byte, *http.Response, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, resp, err
	}
	defer resp.Body.Close()

	// Read response. Limit the response to 10MB.
	// This limit is carried over from client/scaletail/scaletail.go.
	body := io.LimitReader(resp.Body, maxReadSize+1)
	b, err := io.ReadAll(body)
	if len(b) > maxReadSize {
		err = errors.New("API response too large")
	}
	return b, resp, err
}
