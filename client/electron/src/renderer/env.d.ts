// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import type { ScaleTailAPI } from "../shared/types";

declare global {
  interface Window {
    scaletail: ScaleTailAPI;
  }
}

export {};
