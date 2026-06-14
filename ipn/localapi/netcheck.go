// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"scaletail.com/net/netcheck"
	"scaletail.com/net/netmon"
	"scaletail.com/types/logger"
	"scaletail.com/util/eventbus"
	"scaletail.com/util/httpm"
)

func (h *Handler) serveNetcheck(w http.ResponseWriter, r *http.Request) {
	if !h.PermitRead {
		http.Error(w, "netcheck access denied", http.StatusForbidden)
		return
	}
	if r.Method != httpm.GET && r.Method != httpm.POST {
		http.Error(w, "want GET or POST", http.StatusMethodNotAllowed)
		return
	}

	dm := h.b.DERPMap()
	if dm == nil || len(dm.Regions) == 0 {
		http.Error(w, "DERP map is empty; connect to a control server before running netcheck", http.StatusServiceUnavailable)
		return
	}

	bus := eventbus.New()
	defer bus.Close()
	netMon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		WriteErrorJSON(w, err)
		return
	}

	c := &netcheck.Client{
		NetMon:      netMon,
		UseDNSCache: false,
		Logf:        logger.Discard,
	}
	if err := c.Standalone(r.Context(), ""); err != nil {
		h.logf("netcheck UDP setup: %v", err)
	}
	report, err := c.GetReport(r.Context(), dm, nil)
	if err != nil {
		WriteErrorJSON(w, errors.New("netcheck: "+err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	e := json.NewEncoder(w)
	e.SetIndent("", "\t")
	e.Encode(report)
}
