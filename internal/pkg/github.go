package pkg

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type githubEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
	ZipballURL string `json:"zipball_url"`
}

type githubTag struct {
	Name string `json:"name"`
}

type githubClient struct {
	http *http.Client
}

func newGitHubClient() *githubClient {
	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &githubClient{
		http: &http.Client{
			Transport: transport,
			Timeout:   90 * time.Second,
		},
	}
}

func (c *githubClient) githubAPIRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return c.http.Do(req)
}

func (c *githubClient) getJSON(url string, v interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		resp, err := c.githubAPIRequest(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 403 {
			return fmt.Errorf("GitHub rate limit exceeded — wait a moment and try again")
		}
		if resp.StatusCode == 404 {
			return fmt.Errorf("not found: %s", url)
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("GitHub API returned HTTP %d for %s", resp.StatusCode, url)
		}
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("GitHub API request failed after 3 attempts: %w", lastErr)
}

func (c *githubClient) ListTags(owner, repo string) ([]string, error) {
	var tags []githubTag
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=100", owner, repo)
	if err := c.getJSON(url, &tags); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return names, nil
}

func (c *githubClient) LatestRelease(owner, repo string) (*githubRelease, error) {
	var rel githubRelease
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	if err := c.getJSON(url, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (c *githubClient) Release(owner, repo, tag string) (*githubRelease, error) {
	var rel githubRelease
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	if err := c.getJSON(url, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

type installProgress struct {
	pkg       string
	total     int64
	done      int64
	bytes     int64
	startTime time.Time
}

func newInstallProgress(pkg string, total int) *installProgress {
	p := &installProgress{
		pkg:       pkg,
		total:     int64(total),
		startTime: time.Now(),
	}
	fmt.Printf("\n  Installing \033[1;36m%s\033[0m\n", pkg)
	if total > 0 {
		fmt.Printf("  %d file(s) to download\n\n", total)
	} else {
		fmt.Printf("  Resolving file list...\n\n")
	}
	return p
}

func formatBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/1024/1024)
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

func (p *installProgress) renderBar(filename string) {
	done := atomic.LoadInt64(&p.done)
	total := atomic.LoadInt64(&p.total)
	rx := atomic.LoadInt64(&p.bytes)

	elapsed := time.Since(p.startTime).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(rx) / elapsed
	}

	const barWidth = 24
	var pct float64
	filled := 0
	if total > 0 {
		pct = math.Min(float64(done)/float64(total)*100, 100)
		filled = int(math.Round(float64(barWidth) * pct / 100))
	}

	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	var line string
	if total > 0 {
		line = fmt.Sprintf(
			"  \033[36m[%s]\033[0m \033[1m%5.1f%%\033[0m  "+
				"\033[2m%d/%d files\033[0m  %s  \033[33m%s\033[0m  \033[2m%-20s\033[0m",
			bar, pct, done, total,
			formatBytes(rx), formatSpeed(speed),
			truncateFilename(filename, 20),
		)
	} else {
		line = fmt.Sprintf(
			"  \033[36m[%s]\033[0m  "+
				"\033[2m%d files\033[0m  %s  \033[33m%s\033[0m  \033[2m%-20s\033[0m",
			bar, done,
			formatBytes(rx), formatSpeed(speed),
			truncateFilename(filename, 20),
		)
	}

	fmt.Printf("\r%-100s", line)
}

func (p *installProgress) finish() {
	done := atomic.LoadInt64(&p.done)
	rx := atomic.LoadInt64(&p.bytes)
	elapsed := time.Since(p.startTime)

	fmt.Printf("\r\033[K")
	fmt.Printf(
		"  \033[1;32m✓\033[0m  \033[1m%s\033[0m — %d file(s), %s in %s\n\n",
		p.pkg, done, formatBytes(rx), elapsed.Round(time.Millisecond),
	)
}

func truncateFilename(name string, maxLen int) string {
	base := filepath.Base(name)
	if len(base) <= maxLen {
		return base
	}
	return "…" + base[len(base)-maxLen+1:]
}

type progressReader struct {
	r    io.Reader
	prog *installProgress
	name string
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		atomic.AddInt64(&pr.prog.bytes, int64(n))
		pr.prog.renderBar(pr.name)
	}
	return n, err
}

func (c *githubClient) downloadFileProgress(url, filename string, prog *installProgress) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		resp, err := c.http.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("file not found: %s", url)
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
			continue
		}

		var reader io.Reader = resp.Body
		if prog != nil && filename != "" {
			reader = &progressReader{r: resp.Body, prog: prog, name: filename}
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("download failed after 3 attempts: %w", lastErr)
}

func (c *githubClient) fetchDirRecursiveProgress(owner, repo, ref, remotePath, localDir string, prog *installProgress) error {
	cleanPath := strings.Trim(remotePath, "/")
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, cleanPath, ref)

	resp, err := c.githubAPIRequest(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		return fmt.Errorf("GitHub rate limit exceeded — wait a moment and try again")
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("path not found: %s/%s/%s (ref: %s)", owner, repo, remotePath, ref)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub Contents API returned HTTP %d for %s/%s/%s@%s", resp.StatusCode, owner, repo, remotePath, ref)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading GitHub API response: %w", err)
	}

	first := bytes.TrimSpace(body)
	if len(first) == 0 {
		return fmt.Errorf("empty response from GitHub API for %s", remotePath)
	}

	var entries []githubEntry
	if first[0] == '[' {
		if err := json.Unmarshal(body, &entries); err != nil {
			return fmt.Errorf("parsing GitHub directory listing: %w", err)
		}
	} else {
		var single githubEntry
		if err := json.Unmarshal(body, &single); err != nil {
			return fmt.Errorf("parsing GitHub file entry: %w", err)
		}
		entries = []githubEntry{single}
	}

	for _, entry := range entries {
		localPath := filepath.Join(localDir, filepath.FromSlash(entry.Name))
		switch entry.Type {
		case "file":
			if entry.DownloadURL == "" {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
				return err
			}
			data, err := c.downloadFileProgress(entry.DownloadURL, entry.Name, prog)
			if err != nil {
				return fmt.Errorf("downloading %s: %w", entry.Path, err)
			}
			if err := os.WriteFile(localPath, data, 0644); err != nil {
				return err
			}
			if prog != nil {
				atomic.AddInt64(&prog.done, 1)
				prog.renderBar(entry.Name)
			}
		case "dir":
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return err
			}
			if err := c.fetchDirRecursiveProgress(owner, repo, ref, entry.Path, localPath, prog); err != nil {
				return err
			}
		}
	}
	return nil
}

func isInstallableFile(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".lx"),
		strings.HasSuffix(lower, ".nax"),
		strings.HasSuffix(lower, ".toml"),
		strings.HasSuffix(lower, ".json"),
		strings.HasSuffix(lower, ".md"):
		return true
	case lower == "license", lower == "license.txt":
		return true
	default:
		return false
	}
}

func (c *githubClient) fetchViaTreeAPIProgress(owner, repo, ref, subpath, localDir string, prog *installProgress) error {
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		owner, repo, ref)

	resp, err := c.githubAPIRequest(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		return fmt.Errorf("GitHub rate limit exceeded — wait a moment and try again")
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("repository not found: %s/%s@%s", owner, repo, ref)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub Trees API returned HTTP %d for %s/%s@%s", resp.StatusCode, owner, repo, ref)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(body, &tree); err != nil {
		return fmt.Errorf("parsing Git Trees response: %w", err)
	}
	if tree.Truncated {
		fmt.Println("  note: GitHub tree response was truncated; some files may be missing")
	}

	prefix := strings.Trim(subpath, "/")
	rawBase := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, ref)

	type blobEntry struct {
		nodePath string
		relPath  string
	}
	var blobs []blobEntry
	for _, node := range tree.Tree {
		if node.Type != "blob" {
			continue
		}
		relPath := node.Path
		if prefix != "" {
			if !strings.HasPrefix(node.Path, prefix+"/") {
				continue
			}
			relPath = strings.TrimPrefix(node.Path, prefix+"/")
		}
		if !isInstallableFile(relPath) {
			continue
		}
		blobs = append(blobs, blobEntry{nodePath: node.Path, relPath: relPath})
	}

	if len(blobs) == 0 {
		return fmt.Errorf("no .lx files found under %q in %s/%s@%s", prefix, owner, repo, ref)
	}

	if prog != nil {
		atomic.StoreInt64(&prog.total, int64(len(blobs)))
	}

	for _, blob := range blobs {
		localPath := filepath.Join(localDir, filepath.FromSlash(blob.relPath))
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}
		data, err := c.downloadFileProgress(rawBase+"/"+blob.nodePath, blob.relPath, prog)
		if err != nil {
			return fmt.Errorf("downloading %s: %w", blob.nodePath, err)
		}
		if err := os.WriteFile(localPath, data, 0644); err != nil {
			return err
		}
		if prog != nil {
			atomic.AddInt64(&prog.done, 1)
			prog.renderBar(blob.relPath)
		}
	}
	return nil
}

func (c *githubClient) fetchViaCodeload(owner, repo, ref, subpath, localDir string, prog *installProgress) error {
	branchURL := fmt.Sprintf(
		"https://codeload.github.com/%s/%s/zip/refs/heads/%s",
		owner, repo, ref)
	tagURL := fmt.Sprintf(
		"https://codeload.github.com/%s/%s/zip/refs/tags/%s",
		owner, repo, ref)

	fmt.Printf("  \033[2m→ fallback: downloading ZIP from codeload.github.com\033[0m\n")

	var zipData []byte
	var dlErr error

	for _, url := range []string{branchURL, tagURL} {
		zipData, dlErr = c.downloadZipProgress(url, owner+"/"+repo, prog)
		if dlErr == nil {
			break
		}
		if strings.Contains(dlErr.Error(), "404") {
			continue
		}
		break
	}
	if dlErr != nil {
		return fmt.Errorf("codeload download failed: %w", dlErr)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("reading ZIP archive: %w", err)
	}

	rootPrefix := repo + "-" + ref + "/"

	prefix := strings.Trim(subpath, "/")
	var filePrefix string
	if prefix != "" {
		filePrefix = rootPrefix + prefix + "/"
	} else {
		filePrefix = rootPrefix
	}

	var qualifying []*zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasPrefix(f.Name, filePrefix) {
			continue
		}
		relPath := strings.TrimPrefix(f.Name, filePrefix)
		if relPath == "" {
			continue
		}
		if !isInstallableFile(relPath) {
			continue
		}
		qualifying = append(qualifying, f)
	}

	if len(qualifying) == 0 {
		return fmt.Errorf("no .lx files found under %q in %s/%s@%s (ZIP)", prefix, owner, repo, ref)
	}

	if prog != nil {
		atomic.StoreInt64(&prog.total, int64(len(qualifying)))
		atomic.StoreInt64(&prog.done, 0)
	}

	for _, f := range qualifying {
		relPath := strings.TrimPrefix(f.Name, filePrefix)
		localPath := filepath.Join(localDir, filepath.FromSlash(relPath))

		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening ZIP entry %s: %w", f.Name, err)
		}

		var reader io.Reader = rc
		if prog != nil {
			reader = &progressReader{r: rc, prog: prog, name: relPath}
		}
		data, err := io.ReadAll(reader)
		rc.Close()
		if err != nil {
			return fmt.Errorf("reading ZIP entry %s: %w", f.Name, err)
		}

		if err := os.WriteFile(localPath, data, 0644); err != nil {
			return err
		}
		if prog != nil {
			atomic.AddInt64(&prog.done, 1)
			prog.renderBar(relPath)
		}
	}
	return nil
}

func (c *githubClient) downloadZipProgress(url, label string, prog *installProgress) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		resp, err := c.http.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("404: %s", url)
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
			continue
		}

		var reader io.Reader = resp.Body
		if prog != nil {
			reader = &progressReader{r: resp.Body, prog: prog, name: label + ".zip"}
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("ZIP download failed after 3 attempts: %w", lastErr)
}

func (c *githubClient) downloadZipToDir(url, subpath, localDir string, prog *installProgress) error {
	data, err := c.downloadZipProgress(url, filepath.Base(localDir), prog)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("reading ZIP archive: %w", err)
	}

	var rootPrefix string
	if len(zr.File) > 0 {
		first := zr.File[0].Name
		if idx := strings.Index(first, "/"); idx >= 0 {
			rootPrefix = first[:idx+1]
		}
	}

	prefix := strings.Trim(subpath, "/")

	if prog != nil {
		atomic.StoreInt64(&prog.total, int64(len(zr.File)))
		atomic.StoreInt64(&prog.done, 0)
	}

	extracted := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(f.Name, rootPrefix)
		if prefix != "" {
			if !strings.HasPrefix(rel, prefix+"/") {
				continue
			}
			rel = strings.TrimPrefix(rel, prefix+"/")
		}
		if rel == "" {
			continue
		}
		localPath := filepath.Join(localDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening ZIP entry %s: %w", f.Name, err)
		}
		out, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(localPath, out, 0644); err != nil {
			return err
		}
		extracted++
		if prog != nil {
			atomic.AddInt64(&prog.done, 1)
			prog.renderBar(rel)
		}
	}
	if extracted == 0 {
		return fmt.Errorf("no files found under %q in downloaded archive", prefix)
	}
	return nil
}
