package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runInstall checks for admin privileges, checks if WinFsp is installed,
// and if not, downloads and installs it dynamically from GitHub.
func runInstall() error {
	// 1. Check for Admin privileges.
	// On Windows, opening \\.\PHYSICALDRIVE0 requires administrative privileges.
	f, err := os.Open(`\\.\PHYSICALDRIVE0`)
	if err != nil {
		return fmt.Errorf("janus install requires Administrator privileges.\nPlease open an Administrator command prompt and run this command again")
	}
	f.Close()

	// 2. Check if WinFsp is installed.
	if isWinFspInstalled() {
		fmt.Println("WinFsp is already installed. Janus is ready to use!")
		return nil
	}

	fmt.Println("WinFsp is not installed. Fetching latest release...")

	// 3. Fetch latest release from GitHub API.
	downloadURL, err := getLatestWinFspMSI()
	if err != nil {
		return fmt.Errorf("failed to fetch latest WinFsp release: %w", err)
	}

	fmt.Printf("Downloading %s...\n", downloadURL)

	// 4. Download the MSI.
	msiPath := filepath.Join(os.TempDir(), "winfsp-latest.msi")
	if err := downloadFile(msiPath, downloadURL); err != nil {
		return fmt.Errorf("failed to download MSI: %w", err)
	}
	defer os.Remove(msiPath)

	fmt.Println("Installing WinFsp (this may take a moment)...")

	// 5. Run the installer silently.
	cmd := exec.Command("msiexec.exe", "/i", msiPath, "/qn", "/norestart")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install WinFsp: %w", err)
	}

	fmt.Println("WinFsp installed successfully. Janus is ready to use!")
	return nil
}

func isWinFspInstalled() bool {
	// Standard install paths for WinFsp
	paths := []string{
		`C:\Program Files (x86)\WinFsp\bin\winfsp-x64.dll`,
		`C:\Program Files\WinFsp\bin\winfsp-x64.dll`,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func getLatestWinFspMSI() (string, error) {
	apiURL := "https://api.github.com/repos/winfsp/winfsp/releases/latest"
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".msi") {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no MSI asset found in the latest release")
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
