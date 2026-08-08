const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { after, test } = require("node:test");
const Module = require("node:module");

const userDataPath = fs.mkdtempSync(path.join(os.tmpdir(), "scaletail-credential-test-"));
const originalLoad = Module._load;
Module._load = function loadElectronStub(request, parent, isMain) {
  if (request === "electron") {
    return {
      app: {
        getPath(name) {
          assert.equal(name, "userData");
          return userDataPath;
        },
      },
      safeStorage: {
        isEncryptionAvailable: () => true,
        encryptString: (value) => Buffer.from(value, "utf8"),
        decryptString: (value) => value.toString("utf8"),
      },
    };
  }
  return originalLoad.call(this, request, parent, isMain);
};

const {
  canonicalCredentialControlURL,
  clearAccountCredential,
  readAccountCredential,
  saveAccountCredential,
  validateAccountPassword,
} = require("../dist/main/credential_store");

after(() => {
  fs.rmSync(userDataPath, { recursive: true, force: true });
});

test("credential is restored only for its canonical control URL", () => {
  clearAccountCredential();
  saveAccountCredential({
    controlURL: "HTTPS://Control.Example:443/",
    username: "alice",
    password: "correct horse battery staple",
  });

  assert.deepEqual(readAccountCredential("https://control.example"), {
    controlURL: "https://control.example",
    username: "alice",
    password: "correct horse battery staple",
  });
  assert.equal(readAccountCredential("https://other.example"), undefined);
});

test("unscoped schema v1 credential is not restored", () => {
  clearAccountCredential();
  const encrypted = Buffer.from(JSON.stringify({ username: "alice", password: "legacy" }), "utf8");
  fs.writeFileSync(
    path.join(userDataPath, "account-credential.json"),
    JSON.stringify({ version: 1, encrypted: encrypted.toString("base64") }),
    "utf8",
  );
  assert.equal(readAccountCredential("https://control.example"), undefined);
});

test("credential control URL canonicalization accepts only server roots", () => {
  assert.equal(canonicalCredentialControlURL("HTTPS://Control.Example:443/"), "https://control.example");
  assert.equal(canonicalCredentialControlURL("http://127.0.0.1:80"), "http://127.0.0.1");
  assert.equal(canonicalCredentialControlURL("https://control.example/path"), undefined);
  assert.equal(canonicalCredentialControlURL("https://user@control.example"), undefined);
});

test("account password rejects HTTP header control characters", () => {
  assert.equal(validateAccountPassword("密码-安全"), "密码-安全");
  for (const password of ["left\nright", "left\tright", "left\0right", "left\x7fright", "left\u0085right"]) {
    assert.throws(() => validateAccountPassword(password), /控制字符/);
  }
});
