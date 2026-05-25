import type { TailscaleAPI } from "../shared/types";

declare global {
  interface Window {
    tailscale: TailscaleAPI;
  }
}

export {};
