package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Vondel-Media/vondel-plugins/catalog"
)

const githubAPIOrigin = "https://api.github.com"

var releaseBinaryNames = []string{"plugin-darwin-arm64", "plugin-linux-amd64", "plugin-linux-arm64"}
var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var sha256Digest = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

var retainedPlugins = map[string]string{
	"Vondel-Media/vondel-plugin-audiobook-metadata": "silo.audiobook-metadata",
	"Vondel-Media/vondel-plugin-autoscan-arr":       "silo.autoscan.arr",
	"Vondel-Media/vondel-plugin-ebook-metadata":     "silo.ebook-metadata",
	"Vondel-Media/vondel-plugin-manga-metadata":     "silo.manga-metadata",
	"Vondel-Media/vondel-plugin-tmdb":               "silo.tmdb",
	"Vondel-Media/vondel-plugin-tvdb":               "silo.tvdb",
}

func main() {
	var catalogPath, outputPath string
	flag.StringVar(&catalogPath, "catalog", "manifest.json", "private catalog JSON to materialize")
	flag.StringVar(&outputPath, "output", "private-staging", "static output directory")
	flag.Parse()
	if err := materialize(context.Background(), &http.Client{Timeout: 2 * time.Minute}, os.Getenv("GITHUB_TOKEN"), githubAPIOrigin, catalogPath, outputPath); err != nil {
		fmt.Fprintln(os.Stderr, "materialize private catalog:", err)
		os.Exit(1)
	}
}

func materialize(ctx context.Context, client *http.Client, token, apiOrigin, catalogPath, outputPath string) error {
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if client == nil {
		return fmt.Errorf("HTTP client is required")
	}
	if err := requireRegularFile(catalogPath); err != nil {
		return fmt.Errorf("catalog input: %w", err)
	}
	if err := validateOutputPath(outputPath); err != nil {
		return err
	}
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	var index catalog.RepositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("decode catalog: %w", err)
	}
	if len(index.Plugins) != len(retainedPlugins) {
		return fmt.Errorf("catalog must contain exactly the six retained plugins")
	}

	parent := filepath.Dir(outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(outputPath)+".materialize-")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	stagingLive := true
	defer func() {
		if stagingLive {
			_ = os.RemoveAll(staging)
		}
	}()

	sort.Slice(index.Plugins, func(i, j int) bool {
		return index.Plugins[i].Manifest.GetPluginId() < index.Plugins[j].Manifest.GetPluginId()
	})
	seen := make(map[string]struct{}, len(index.Plugins))
	outputIndex := catalog.RepositoryIndex{Plugins: make([]catalog.CatalogPackage, 0, len(index.Plugins))}
	for _, pkg := range index.Plugins {
		repo, version, err := validateCatalogPackage(pkg, seen)
		if err != nil {
			return err
		}
		release, err := fetchRelease(ctx, client, token, apiOrigin, repo, "v"+version)
		if err != nil {
			return fmt.Errorf("fetch %s release: %w", repo, err)
		}
		assets, err := validateRelease(repo, version, apiOrigin, release)
		if err != nil {
			return fmt.Errorf("validate %s release: %w", repo, err)
		}
		contents := make(map[string][]byte, len(assets))
		for _, name := range append([]string{"checksums.txt"}, releaseBinaryNames...) {
			contents[name], err = downloadAsset(ctx, client, token, apiOrigin, repo, assets[name])
			if err != nil {
				return fmt.Errorf("download %s %s: %w", repo, name, err)
			}
		}
		checksums, err := verifyContents(contents)
		if err != nil {
			return fmt.Errorf("verify %s release: %w", repo, err)
		}

		relDir := path.Join("plugins", pkg.Manifest.GetPluginId(), version)
		absoluteDir := filepath.Join(staging, filepath.FromSlash(relDir))
		if err := os.MkdirAll(absoluteDir, 0o755); err != nil {
			return fmt.Errorf("create plugin output: %w", err)
		}
		if err := os.WriteFile(filepath.Join(absoluteDir, "checksums.txt"), contents["checksums.txt"], 0o644); err != nil {
			return fmt.Errorf("write checksums: %w", err)
		}
		localBinaries := make(map[string]catalog.PlatformBinary, len(pkg.Binaries))
		for _, name := range releaseBinaryNames {
			platform := strings.ReplaceAll(strings.TrimPrefix(name, "plugin-"), "-", "/")
			catalogBinary := pkg.Binaries[platform]
			if !strings.EqualFold(catalogBinary.Checksum, checksums[name]) {
				return fmt.Errorf("catalog checksum does not match verified release for %s %s", repo, platform)
			}
			if err := os.WriteFile(filepath.Join(absoluteDir, name), contents[name], 0o755); err != nil {
				return fmt.Errorf("write binary: %w", err)
			}
			localBinaries[platform] = catalog.PlatformBinary{URL: path.Join(relDir, name), Checksum: checksums[name]}
		}
		pkg.ChecksumsURL = path.Join(relDir, "checksums.txt")
		pkg.Binaries = localBinaries
		outputIndex.Plugins = append(outputIndex.Plugins, pkg)
	}

	catalogData, err := json.MarshalIndent(outputIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("encode static catalog: %w", err)
	}
	catalogData = append(catalogData, '\n')
	if err := os.WriteFile(filepath.Join(staging, "catalog.json"), catalogData, 0o644); err != nil {
		return fmt.Errorf("write static catalog: %w", err)
	}
	if err := scanTreeForToken(staging, token); err != nil {
		return err
	}
	if err := publishTree(staging, outputPath); err != nil {
		return err
	}
	stagingLive = false
	return nil
}

func validateCatalogPackage(pkg catalog.CatalogPackage, seen map[string]struct{}) (string, string, error) {
	if pkg.Manifest == nil {
		return "", "", fmt.Errorf("catalog package is missing a manifest")
	}
	repo, err := repositoryFromURL(pkg.RepoURL)
	if err != nil {
		return "", "", err
	}
	wantID, ok := retainedPlugins[repo]
	if !ok {
		return "", "", fmt.Errorf("repository %q is not retained", repo)
	}
	if pkg.Manifest.GetPluginId() != wantID {
		return "", "", fmt.Errorf("repository %q has unexpected plugin identity", repo)
	}
	if _, duplicate := seen[repo]; duplicate {
		return "", "", fmt.Errorf("duplicate repository %q", repo)
	}
	seen[repo] = struct{}{}
	version := pkg.Manifest.GetVersion()
	if !stableVersion.MatchString(version) {
		return "", "", fmt.Errorf("plugin %q has unsafe version", wantID)
	}
	if pkg.Manifest.GetSiloApiVersion() != "v1" || len(pkg.Manifest.GetCapabilities()) == 0 {
		return "", "", fmt.Errorf("plugin %q has invalid manifest compatibility", wantID)
	}
	wantPlatforms := map[string]struct{}{"darwin/arm64": {}, "linux/amd64": {}, "linux/arm64": {}}
	if len(pkg.Binaries) != len(wantPlatforms) || len(pkg.Manifest.GetSupportedPlatforms()) != len(wantPlatforms) {
		return "", "", fmt.Errorf("plugin %q must contain exactly three platforms", wantID)
	}
	seenPlatforms := map[string]struct{}{}
	for _, platform := range pkg.Manifest.GetSupportedPlatforms() {
		key := platform.GetOs() + "/" + platform.GetArch()
		if _, ok := wantPlatforms[key]; !ok {
			return "", "", fmt.Errorf("plugin %q contains unsupported platform", wantID)
		}
		if _, duplicate := seenPlatforms[key]; duplicate {
			return "", "", fmt.Errorf("plugin %q contains duplicate platform", wantID)
		}
		seenPlatforms[key] = struct{}{}
	}
	for platform := range wantPlatforms {
		binary, ok := pkg.Binaries[platform]
		if !ok || !sha256Digest.MatchString(binary.Checksum) {
			return "", "", fmt.Errorf("plugin %q has invalid binary metadata", wantID)
		}
		name := "plugin-" + strings.ReplaceAll(platform, "/", "-")
		wantURL := "https://github.com/" + repo + "/releases/download/v" + version + "/" + name
		if binary.URL != wantURL {
			return "", "", fmt.Errorf("plugin %q has non-allowlisted binary URL", wantID)
		}
	}
	wantChecksums := "https://github.com/" + repo + "/releases/download/v" + version + "/checksums.txt"
	if pkg.ChecksumsURL != wantChecksums {
		return "", "", fmt.Errorf("plugin %q has non-allowlisted checksums URL", wantID)
	}
	return repo, version, nil
}

func repositoryFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("catalog repository URL is not allowlisted")
	}
	repo := strings.TrimPrefix(u.EscapedPath(), "/")
	if strings.Contains(repo, "%") || strings.Count(repo, "/") != 1 || u.Path != "/"+repo {
		return "", fmt.Errorf("catalog repository path is not allowlisted")
	}
	return repo, nil
}

func fetchRelease(ctx context.Context, client *http.Client, token, apiOrigin, repo, tag string) (catalog.Release, error) {
	requestURL := strings.TrimRight(apiOrigin, "/") + "/repos/" + repo + "/releases/tags/" + url.PathEscape(tag)
	req, err := authenticatedRequest(ctx, requestURL, token, "application/vnd.github+json")
	if err != nil {
		return catalog.Release{}, err
	}
	safe := *client
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := safe.Do(req)
	if err != nil {
		return catalog.Release{}, fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return catalog.Release{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var release catalog.Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return catalog.Release{}, fmt.Errorf("decode release metadata: %w", err)
	}
	return release, nil
}

func validateRelease(repo, version, apiOrigin string, release catalog.Release) (map[string]catalog.Asset, error) {
	if release.TagName != "v"+version || release.Draft || release.Prerelease || release.PublishedAt == "" {
		return nil, fmt.Errorf("release identity or state is invalid")
	}
	wanted := map[string]struct{}{"checksums.txt": {}}
	for _, name := range releaseBinaryNames {
		wanted[name] = struct{}{}
	}
	if len(release.Assets) != len(wanted) {
		return nil, fmt.Errorf("release must contain exactly four assets")
	}
	assets := make(map[string]catalog.Asset, len(wanted))
	for _, asset := range release.Assets {
		if _, ok := wanted[asset.Name]; !ok {
			return nil, fmt.Errorf("unexpected asset")
		}
		if _, duplicate := assets[asset.Name]; duplicate {
			return nil, fmt.Errorf("duplicate asset")
		}
		if asset.ID <= 0 {
			return nil, fmt.Errorf("asset has invalid ID")
		}
		wantAPI := strings.TrimRight(apiOrigin, "/") + "/repos/" + repo + "/releases/assets/" + strconv.FormatInt(asset.ID, 10)
		wantBrowser := "https://github.com/" + repo + "/releases/download/v" + version + "/" + asset.Name
		if asset.URL != wantAPI || asset.BrowserDownloadURL != wantBrowser {
			return nil, fmt.Errorf("asset URL is not allowlisted")
		}
		assets[asset.Name] = asset
	}
	return assets, nil
}

func downloadAsset(ctx context.Context, client *http.Client, token, apiOrigin, repo string, asset catalog.Asset) ([]byte, error) {
	want := strings.TrimRight(apiOrigin, "/") + "/repos/" + repo + "/releases/assets/" + strconv.FormatInt(asset.ID, 10)
	if asset.URL != want {
		return nil, fmt.Errorf("asset API path is not allowlisted")
	}
	req, err := authenticatedRequest(ctx, asset.URL, token, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	safe := *client
	safe.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		next.Header.Del("Authorization")
		return nil
	}
	resp, err := safe.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	limit := int64(512 << 20)
	if asset.Name == "checksums.txt" {
		limit = 1 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response")
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("asset exceeds size limit")
	}
	return body, nil
}

func authenticatedRequest(ctx context.Context, requestURL, token, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "vondel-private-staging-materializer")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

func verifyContents(contents map[string][]byte) (map[string]string, error) {
	if len(contents) != 4 {
		return nil, fmt.Errorf("release must contain exactly four downloaded assets")
	}
	lines := strings.Split(strings.TrimSuffix(string(contents["checksums.txt"]), "\n"), "\n")
	if len(lines) != 3 {
		return nil, fmt.Errorf("checksums must contain exactly three lines")
	}
	verified := map[string]string{}
	wanted := map[string]struct{}{}
	for _, name := range releaseBinaryNames {
		wanted[name] = struct{}{}
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || !sha256Digest.MatchString(fields[0]) || strings.ContainsAny(fields[1], `/\\`) {
			return nil, fmt.Errorf("checksum entry is invalid")
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, ok := wanted[name]; !ok {
			return nil, fmt.Errorf("checksum asset is unexpected")
		}
		if _, duplicate := verified[name]; duplicate {
			return nil, fmt.Errorf("checksum asset is duplicated")
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(contents[name]))
		if !strings.EqualFold(fields[0], actual) {
			return nil, fmt.Errorf("checksum mismatch for %s", name)
		}
		verified[name] = strings.ToLower(fields[0])
	}
	if len(verified) != 3 {
		return nil, fmt.Errorf("checksums do not map every binary")
	}
	return verified, nil
}

func requireRegularFile(name string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path must be a regular non-symlink file")
	}
	return nil
}

func validateOutputPath(name string) error {
	if name == "" || !filepath.IsAbs(name) && filepath.Clean(name) == "." {
		return fmt.Errorf("output path is invalid")
	}
	info, err := os.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("output must be a real directory path")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output: %w", err)
	}
	return nil
}

func scanTreeForToken(root, token string) error {
	return filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("staging tree contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), token) {
			return fmt.Errorf("credential canary detected in staging output")
		}
		return nil
	})
}

func publishTree(staging, output string) error {
	if _, err := os.Lstat(output); os.IsNotExist(err) {
		if err := os.Rename(staging, output); err != nil {
			return fmt.Errorf("publish staging tree: %w", err)
		}
		return nil
	}
	backup, err := os.MkdirTemp(filepath.Dir(output), "."+filepath.Base(output)+".previous-")
	if err != nil {
		return fmt.Errorf("reserve previous tree: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare previous tree: %w", err)
	}
	if err := os.Rename(output, backup); err != nil {
		return fmt.Errorf("preserve previous tree: %w", err)
	}
	if err := os.Rename(staging, output); err != nil {
		_ = os.Rename(backup, output)
		return fmt.Errorf("publish staging tree: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous tree: %w", err)
	}
	return nil
}

func isSafeRelativeURL(raw string) bool {
	if raw == "" || strings.Contains(raw, "\\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return !path.IsAbs(u.Path) && path.Clean(u.Path) == u.Path && u.Path != "." && !strings.HasPrefix(u.Path, "../")
}
