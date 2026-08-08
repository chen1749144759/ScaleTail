const assert = require("node:assert/strict");
const test = require("node:test");
const Module = require("node:module");

const originalLoad = Module._load;
Module._load = function loadElectronStub(request, parent, isMain) {
  if (request === "electron") {
    return {
      app: {},
      BrowserWindow: class BrowserWindow {},
      screen: {},
    };
  }
  return originalLoad.call(this, request, parent, isMain);
};

const { normalizeHTTPSDownloadURL, otaMessage } = require("../dist/main/client_update");

const canonicalURL = "https://downloads.example.com/releases/ScaleTail.exe?channel=stable~1";

test("OTA v3 message matches the Go and Python vector", () => {
  const actual = otaMessage(42, "forced", "0.0.8", "WINDOWS-AMD64", "a".repeat(64), 1234, canonicalURL).toString("utf8");
  assert.equal(
    actual,
    `scaletail-update-v3\n42\nforced\n0.0.8\nwindows-amd64\n${"a".repeat(64)}\n1234\n${canonicalURL}\n`,
  );
});

test("OTA v3 URL normalizer rejects unsafe download targets", () => {
  assert.equal(
    normalizeHTTPSDownloadURL("HTTPS://Downloads.Example.Com:443/releases/ScaleTail.exe?channel=stable%7e1").canonical,
    canonicalURL,
  );
  for (const raw of [
    "https://user:secret@downloads.example.com/ScaleTail.exe",
    "https://downloads.example.com/ScaleTail.exe#ignored",
    "https://localhost/ScaleTail.exe",
    "https://127.0.0.1/ScaleTail.exe",
    "https://2130706433/ScaleTail.exe",
    "https://0x7f000001/ScaleTail.exe",
    "https://[::1]/ScaleTail.exe",
    "https://downloads.example.com/ScaleTail.exe?",
  ]) {
    assert.throws(() => normalizeHTTPSDownloadURL(raw), Error, raw);
  }
});
