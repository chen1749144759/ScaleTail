// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
	"scaletail.com/clientupdate/scaletailota"
	"scaletail.com/util/cmpver"
	"scaletail.com/util/httpm"
)

const maxOTAInstallerSize = 1024 << 20

var (
	otaMarkerPattern  = regexp.MustCompile(`^[a-f0-9]{32}$`)
	otaVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)
	otaInstallMu      sync.Mutex
	otaInstallRunning bool
)

type scaleTailUpdateInstallRequest struct {
	InstallerPath string `json:"installer_path"`
	Version       string `json:"version"`
	Platform      string `json:"platform"`
	SHA256        string `json:"sha256"`
	FileSize      int64  `json:"file_size"`
	Signature     string `json:"signature"`
	MarkerID      string `json:"marker_id"`
}

func init() {
	Register("scaletail-update/install", (*Handler).serveScaleTailUpdateInstall)
}

func (h *Handler) serveScaleTailUpdateInstall(w http.ResponseWriter, r *http.Request) {
	if !h.PermitWrite {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	if r.Method != httpm.POST {
		http.Error(w, "want POST", http.StatusBadRequest)
		return
	}

	var req scaleTailUpdateInstallRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid update request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateScaleTailUpdateRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if current := installedScaleTailUIVersion(); current != "" && cmpver.Compare(req.Version, current) <= 0 {
		http.Error(w, fmt.Sprintf("update version %s is not newer than installed version %s", req.Version, current), http.StatusConflict)
		return
	}

	otaInstallMu.Lock()
	if otaInstallRunning {
		otaInstallMu.Unlock()
		http.Error(w, "another ScaleTail update is already running", http.StatusConflict)
		return
	}
	otaInstallRunning = true
	otaInstallMu.Unlock()

	installerPath, err := stageAndVerifyInstaller(req)
	if err != nil {
		setOTAInstallRunning(false)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := launchScaleTailInstaller(installerPath, req.MarkerID); err != nil {
		setOTAInstallRunning(false)
		http.Error(w, "start installer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logf("OTA: accepted signed ScaleTail %s installer %s", req.Version, installerPath)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"accepted": true})
}

func validateScaleTailUpdateRequest(req *scaleTailUpdateInstallRequest) error {
	req.InstallerPath = filepath.Clean(strings.TrimSpace(req.InstallerPath))
	req.Version = strings.TrimSpace(req.Version)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.SHA256 = strings.ToLower(strings.TrimSpace(req.SHA256))
	req.Signature = strings.TrimSpace(req.Signature)
	req.MarkerID = strings.ToLower(strings.TrimSpace(req.MarkerID))

	if !filepath.IsAbs(req.InstallerPath) || !strings.EqualFold(filepath.Ext(req.InstallerPath), ".exe") {
		return fmt.Errorf("installer_path must be an absolute .exe path")
	}
	if !otaVersionPattern.MatchString(req.Version) {
		return fmt.Errorf("invalid update version")
	}
	if req.Platform != currentOTAPlatform() {
		return fmt.Errorf("update platform %q does not match %q", req.Platform, currentOTAPlatform())
	}
	if len(req.SHA256) != sha256.Size*2 {
		return fmt.Errorf("invalid SHA-256")
	}
	if _, err := hex.DecodeString(req.SHA256); err != nil {
		return fmt.Errorf("invalid SHA-256")
	}
	if req.FileSize <= 0 || req.FileSize > maxOTAInstallerSize {
		return fmt.Errorf("invalid installer size")
	}
	if !otaMarkerPattern.MatchString(req.MarkerID) {
		return fmt.Errorf("invalid OTA marker")
	}
	return nil
}

func stageAndVerifyInstaller(req scaleTailUpdateInstallRequest) (string, error) {
	source, err := os.Open(req.InstallerPath)
	if err != nil {
		return "", fmt.Errorf("open installer: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return "", fmt.Errorf("stat installer: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != req.FileSize {
		return "", fmt.Errorf("installer size does not match signed metadata")
	}

	updateDir, err := otaUpdateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(updateDir, 0700); err != nil {
		return "", fmt.Errorf("create OTA directory: %w", err)
	}
	cleanupOldOTAArtifacts(updateDir)
	tempFile, err := os.CreateTemp(updateDir, ".verify-*.exe")
	if err != nil {
		return "", fmt.Errorf("create staged installer: %w", err)
	}
	tempPath := tempFile.Name()
	keep := false
	defer func() {
		tempFile.Close()
		if !keep {
			os.Remove(tempPath)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tempFile, hash), io.LimitReader(source, maxOTAInstallerSize+1))
	if err != nil {
		return "", fmt.Errorf("stage installer: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return "", fmt.Errorf("flush staged installer: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close staged installer: %w", err)
	}
	if written != req.FileSize || hex.EncodeToString(hash.Sum(nil)) != req.SHA256 {
		return "", fmt.Errorf("installer SHA-256 does not match signed metadata")
	}
	if err := scaletailota.Verify(req.Version, req.Platform, req.SHA256, req.FileSize, req.Signature); err != nil {
		return "", err
	}

	targetPath := filepath.Join(updateDir, fmt.Sprintf("ScaleTail-%s-%s.exe", req.Version, req.SHA256[:12]))
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("replace staged installer: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", fmt.Errorf("commit staged installer: %w", err)
	}
	keep = true
	return targetPath, nil
}

func launchScaleTailInstaller(installerPath, markerID string) error {
	markerDir, err := otaMarkerDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		return fmt.Errorf("create OTA marker directory: %w", err)
	}
	markerPath := filepath.Join(markerDir, markerID+".done")
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale OTA marker: %w", err)
	}
	logPath := filepath.Join(filepath.Dir(installerPath), markerID+".log")
	cmd := exec.Command(installerPath,
		"/VERYSILENT",
		"/SUPPRESSMSGBOXES",
		"/NORESTART",
		"/CLOSEAPPLICATIONS",
		"/SP-",
		"/OTAUPDATE=1",
		"/OTAMARKER="+markerID,
		"/LOG="+logPath,
	)
	cmd.Dir = filepath.Dir(installerPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	windows.MoveFileEx(windows.StringToUTF16Ptr(installerPath), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	go func() {
		err := cmd.Wait()
		setOTAInstallRunning(false)
		_ = err
	}()
	return nil
}

func cleanupOldOTAArtifacts(updateDir string) {
	entries, err := os.ReadDir(updateDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasPrefix(name, ".verify-") &&
			!(strings.HasPrefix(name, "ScaleTail-") && strings.HasSuffix(strings.ToLower(name), ".exe"))) {
			continue
		}
		_ = os.Remove(filepath.Join(updateDir, name))
	}
}

func currentOTAPlatform() string {
	if runtime.GOARCH == "arm64" {
		return "windows-arm64"
	}
	return "windows-amd64"
}

func otaUpdateDir() (string, error) {
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		return "", fmt.Errorf("ProgramData is not configured")
	}
	return filepath.Join(programData, "ScaleTail", "Updates"), nil
}

func otaMarkerDir() (string, error) {
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		return "", fmt.Errorf("ProgramData is not configured")
	}
	return filepath.Join(programData, "ScaleTailOTA"), nil
}

func installedScaleTailUIVersion() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "resources", "app", "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return ""
	}
	return strings.TrimSpace(pkg.Version)
}

func setOTAInstallRunning(running bool) {
	otaInstallMu.Lock()
	otaInstallRunning = running
	otaInstallMu.Unlock()
}
