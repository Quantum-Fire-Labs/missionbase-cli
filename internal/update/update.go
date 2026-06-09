package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Options struct {
	CurrentVersion string
	Repo           string
}

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func Run(opts Options, args []string) error {
	checkOnly := false
	force := false
	repo := opts.Repo
	for _, arg := range args {
		switch {
		case arg == "--check":
			checkOnly = true
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "--repo="):
			repo = strings.TrimPrefix(arg, "--repo=")
		default:
			return fmt.Errorf("unknown update option %q", arg)
		}
	}

	rel, err := latestRelease(repo)
	if err != nil {
		return err
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(opts.CurrentVersion, "v")
	if current == latest && !force {
		fmt.Printf("Missionbase CLI is up to date (%s)\n", opts.CurrentVersion)
		return nil
	}

	fmt.Printf("Current version: %s\nLatest version: %s\n", opts.CurrentVersion, rel.TagName)
	if checkOnly {
		return nil
	}

	asset, err := findAsset(rel.Assets)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", asset.Name)
	data, err := download(asset.URL)
	if err != nil {
		return err
	}

	tmp := exe + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	backup := exe + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(backup, exe)
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(backup)

	fmt.Printf("Updated missionbase to %s\n", rel.TagName)
	return nil
}

func latestRelease(repo string) (release, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return release{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return release{}, fmt.Errorf("GitHub release lookup failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return release{}, err
	}
	return rel, nil
}

func findAsset(assets []asset) (asset, error) {
	want := fmt.Sprintf("missionbase-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	for _, asset := range assets {
		if asset.Name == want {
			return asset, nil
		}
	}
	return asset{}, fmt.Errorf("no release asset found for %s/%s; expected %s", runtime.GOOS, runtime.GOARCH, want)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
