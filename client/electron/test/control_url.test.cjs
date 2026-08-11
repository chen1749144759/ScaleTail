const assert = require("node:assert/strict");
const { test } = require("node:test");
const Module = require("node:module");

const originalLoad = Module._load;
Module._load = function loadElectronStub(request, parent, isMain) {
  if (request === "electron") {
    return { app: { getAppPath: () => process.cwd() } };
  }
  return originalLoad.call(this, request, parent, isMain);
};

const { buildControlURL } = require("../dist/main/localapi");

test("remote HTTP control URL is accepted and normalized", () => {
  assert.equal(buildControlURL({
    serverIP: "http://211.137.214.34:60090/",
    serverPort: "",
    useHTTPS: false,
  }), "http://211.137.214.34:60090");
});

test("control URL rejects credentials and non-root URL components", () => {
  for (const serverIP of [
    "http://user:password@control.example.com",
    "http://control.example.com/path",
    "http://control.example.com?next=x",
    "http://control.example.com#fragment",
  ]) {
    assert.throws(() => buildControlURL({ serverIP, serverPort: "", useHTTPS: false }));
  }
});
