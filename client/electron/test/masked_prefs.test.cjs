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

const { buildWantRunningPrefsPatch } = require("../dist/main/localapi");

test("disconnect patch uses the flat Go MaskedPrefs shape", () => {
  const patch = buildWantRunningPrefsPatch(false);
  assert.deepEqual(patch, {
    WantRunning: false,
    WantRunningSet: true,
  });
  assert.equal(Object.hasOwn(patch, "Prefs"), false);
});

test("reconnect patch enables running without logging out", () => {
  const patch = buildWantRunningPrefsPatch(true, true);
  assert.deepEqual(patch, {
    WantRunning: true,
    WantRunningSet: true,
    LoggedOut: false,
    LoggedOutSet: true,
  });
  assert.equal(Object.hasOwn(patch, "Prefs"), false);
});
