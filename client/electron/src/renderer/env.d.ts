import type { ScaleTailAPI } from "../shared/types";

declare global {
  interface Window {
    scaletail: ScaleTailAPI;
  }
}

export {};
