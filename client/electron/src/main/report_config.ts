// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import type { ClientReportConfig } from "../shared/types";

const managedConfig: Readonly<ClientReportConfig> = {
  enabled: true,
  intervalSeconds: 15,
  flowEnabled: true,
  quotaGuardEnabled: true,
};

export function readClientReportConfig(): ClientReportConfig {
  return { ...managedConfig };
}
