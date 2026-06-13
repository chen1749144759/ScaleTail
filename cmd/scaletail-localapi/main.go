// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// scaletail-localapi is a small helper used by the Windows Electron client.
// It talks to ScaleTail through the official Go LocalAPI client so Windows
// named-pipe authentication uses the same path as the upstream CLI.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: scaletail-localapi request|watch [flags]")
	}

	switch os.Args[1] {
	case "request":
		runRequest(os.Args[2:])
	case "watch":
		runWatch(os.Args[2:])
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func runRequest(args []string) {
	fs := flag.NewFlagSet("request", flag.ExitOnError)
	method := fs.String("method", "GET", "HTTP method")
	path := fs.String("path", "", "LocalAPI path")
	expect := fs.Int("expect", http.StatusOK, "expected HTTP status")
	timeoutMS := fs.Int("timeout-ms", 15000, "request timeout in milliseconds")
	fs.Parse(args)

	if *path == "" || !strings.HasPrefix(*path, "/") {
		fatalf("invalid LocalAPI path %q", *path)
	}

	bodyBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("read request body: %v", err)
	}

	var body io.Reader
	if len(bodyBytes) > 0 {
		body = bytes.NewReader(bodyBytes)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutMS)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, *method, "http://"+apitype.LocalAPIHost+*path, body)
	if err != nil {
		fatalf("create request: %v", err)
	}
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := new(local.Client).DoLocalRequest(req)
	if err != nil {
		fatalf("%v", err)
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		fatalf("read response body: %v", err)
	}
	if res.StatusCode != *expect {
		fatalf("HTTP %d: %s", res.StatusCode, bestErrorMessage(resBody, res.Status))
	}
	if len(resBody) > 0 {
		if _, err := os.Stdout.Write(resBody); err != nil {
			fatalf("write response body: %v", err)
		}
	}
}

func runWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	path := fs.String("path", "/localapi/v0/watch-ipn-bus?mask=0", "LocalAPI watch path")
	fs.Parse(args)

	if *path == "" || !strings.HasPrefix(*path, "/") {
		fatalf("invalid LocalAPI path %q", *path)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+apitype.LocalAPIHost+*path, nil)
	if err != nil {
		fatalf("create watch request: %v", err)
	}
	res, err := new(local.Client).DoLocalRequest(req)
	if err != nil {
		fatalf("%v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		resBody, _ := io.ReadAll(res.Body)
		fatalf("HTTP %d: %s", res.StatusCode, bestErrorMessage(resBody, res.Status))
	}
	if _, err := io.Copy(os.Stdout, res.Body); err != nil {
		fatalf("stream watch response: %v", err)
	}
}

func bestErrorMessage(body []byte, fallback string) string {
	type errorJSON struct {
		Error string
	}
	var parsed errorJSON
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != "" {
		return parsed.Error
	}
	if msg := strings.TrimSpace(string(body)); msg != "" {
		return msg
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
