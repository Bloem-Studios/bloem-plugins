package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Bloem-Studios/bloem-plugins/catalog"
)

const githubAPIVersion = "2022-11-28"
const githubAPIOrigin = "https://api.github.com"

var releaseBinaryNames = []string{"plugin-darwin-arm64", "plugin-linux-amd64", "plugin-linux-arm64"}
var sha256Pattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

var allowedRepositories = map[string]struct{}{
	"Bloem-Studios/bloem-plugin-tmdb":                 {},
	"Bloem-Studios/bloem-plugin-tvdb":                 {},
	"Bloem-Studios/bloem-plugin-ebook-metadata":       {},
	"Bloem-Studios/bloem-plugin-audiobook-metadata":   {},
	"Bloem-Studios/bloem-plugin-manga-metadata":       {},
	"Bloem-Studios/bloem-plugin-autoscan-arr":         {},
	"Bloem-Studios/bloem-plugin-theintrodb":           {},
	"Bloem-Studios/bloem-plugin-sportarr-metadata":    {},
	"Bloem-Studios/bloem-plugin-watchprovider-floppy": {},
	"Bloem-Studios/bloem-plugin-requests-arr":         {},
	"Bloem-Studios/bloem-plugin-requests-seerr":       {},
}

func main() {
	var repo string
	var tag string
	var manifestPath string

	flag.StringVar(&repo, "repo", "", "GitHub repository in owner/name form")
	flag.StringVar(&tag, "tag", "", "Git tag to publish from")
	flag.StringVar(&manifestPath, "manifest", "manifest.json", "Path to the catalog manifest")
	flag.Parse()

	if strings.TrimSpace(repo) == "" {
		exitf("repo is required")
	}
	if err := validateRepository(repo); err != nil {
		exitf("validate repo: %v", err)
	}
	if strings.TrimSpace(tag) == "" {
		exitf("tag is required")
	}

	token := os.Getenv("GITHUB_TOKEN")
	client := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	release, err := fetchRelease(ctx, client, token, repo, tag)
	if err != nil {
		exitf("fetch release: %v", err)
	}

	sourceManifest, err := fetchSourceManifest(ctx, client, token, repo, tag)
	if err != nil {
		exitf("fetch source manifest: %v", err)
	}

	assets, err := validateReleaseAssets(githubAPIOrigin, repo, release)
	if err != nil {
		exitf("validate release assets: %v", err)
	}
	contents := make(map[string][]byte, len(assets))
	for name, asset := range assets {
		contents[name], err = downloadAsset(ctx, client, token, githubAPIOrigin, repo, asset)
		if err != nil {
			exitf("download release asset %s: %v", name, err)
		}
	}
	checksums, err := verifyReleaseContents(contents)
	if err != nil {
		exitf("verify release assets: %v", err)
	}
	if err := validateSourceReleaseIdentity(sourceManifest, tag); err != nil {
		exitf("validate source identity: %v", err)
	}
	nativeName := "plugin-" + runtime.GOOS + "-" + runtime.GOARCH
	if native, compatible := contents[nativeName]; compatible {
		if err := validateNativeBinary(sourceManifest, native); err != nil {
			exitf("validate native binary manifest: %v", err)
		}
	}

	pkg, err := catalog.BuildPackageFromRelease(repo, sourceManifest, release)
	if err != nil {
		exitf("build catalog package: %v", err)
	}
	for name, checksum := range checksums {
		platform := strings.ReplaceAll(strings.TrimPrefix(name, "plugin-"), "-", "/")
		binary := pkg.Binaries[platform]
		binary.Checksum = checksum
		pkg.Binaries[platform] = binary
	}

	index, err := loadIndex(manifestPath)
	if err != nil {
		exitf("load catalog: %v", err)
	}
	index = catalog.UpsertPackage(index, pkg)
	if err := writeIndex(manifestPath, index); err != nil {
		exitf("write catalog: %v", err)
	}
}

func validateRepository(repo string) error {
	if _, ok := allowedRepositories[repo]; !ok {
		return fmt.Errorf("repository %q is not allowed", repo)
	}
	return nil
}

func validateReleaseAssets(apiOrigin, repo string, release catalog.Release) (map[string]catalog.Asset, error) {
	want := map[string]struct{}{"checksums.txt": {}}
	for _, name := range releaseBinaryNames {
		want[name] = struct{}{}
	}
	if len(release.Assets) != len(want) {
		return nil, fmt.Errorf("release must contain exactly four assets")
	}
	assets := make(map[string]catalog.Asset, len(want))
	for _, asset := range release.Assets {
		if _, ok := want[asset.Name]; !ok {
			return nil, fmt.Errorf("release contains unexpected asset %q", asset.Name)
		}
		if _, exists := assets[asset.Name]; exists {
			return nil, fmt.Errorf("release contains duplicate asset %q", asset.Name)
		}
		if err := validateAssetAPIURL(apiOrigin, repo, asset); err != nil {
			return nil, err
		}
		expectedDownload := "https://github.com/" + repo + "/releases/download/" + url.PathEscape(release.TagName) + "/" + url.PathEscape(asset.Name)
		if asset.BrowserDownloadURL != expectedDownload {
			return nil, fmt.Errorf("release asset %q has non-allowlisted browser download URL", asset.Name)
		}
		assets[asset.Name] = asset
	}
	return assets, nil
}

func downloadAsset(ctx context.Context, client *http.Client, token, apiOrigin, repo string, asset catalog.Asset) ([]byte, error) {
	if err := validateAssetAPIURL(apiOrigin, repo, asset); err != nil {
		return nil, fmt.Errorf("asset metadata is not allowlisted")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build asset request")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "bloem-plugins-catalog-updater")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	safeClient := *client
	safeClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return fmt.Errorf("too many asset redirects")
		}
		next.Header.Del("Authorization")
		return nil
	}
	resp, err := safeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read asset response")
	}
	return body, nil
}

func validateAssetAPIURL(apiOrigin, repo string, asset catalog.Asset) error {
	if asset.ID <= 0 {
		return fmt.Errorf("release asset %q has invalid API ID", asset.Name)
	}
	expected := strings.TrimRight(apiOrigin, "/") + "/repos/" + repo + "/releases/assets/" + strconv.FormatInt(asset.ID, 10)
	if asset.URL != expected {
		return fmt.Errorf("release asset %q has non-allowlisted API URL", asset.Name)
	}
	return nil
}

func verifyReleaseContents(contents map[string][]byte) (map[string]string, error) {
	if len(contents) != 4 {
		return nil, fmt.Errorf("release content must contain exactly four assets")
	}
	checksumBody, ok := contents["checksums.txt"]
	if !ok {
		return nil, fmt.Errorf("checksums.txt content is missing")
	}
	lines := strings.Split(strings.TrimSuffix(string(checksumBody), "\n"), "\n")
	if len(lines) != 3 {
		return nil, fmt.Errorf("checksums.txt must contain exactly three lines")
	}
	verified := make(map[string]string, 3)
	wanted := make(map[string]struct{}, 3)
	for _, name := range releaseBinaryNames {
		wanted[name] = struct{}{}
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || !sha256Pattern.MatchString(fields[0]) || strings.ContainsAny(fields[1], `/\\`) {
			return nil, fmt.Errorf("checksums.txt line must contain a SHA-256 and bare asset name")
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, ok := wanted[name]; !ok {
			return nil, fmt.Errorf("checksums.txt contains unexpected asset name")
		}
		if _, exists := verified[name]; exists {
			return nil, fmt.Errorf("checksums.txt contains duplicate asset name")
		}
		body, ok := contents[name]
		if !ok {
			return nil, fmt.Errorf("binary content is missing")
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(body))
		if !strings.EqualFold(actual, fields[0]) {
			return nil, fmt.Errorf("binary checksum mismatch for %s", name)
		}
		verified[name] = strings.ToLower(fields[0])
	}
	if len(verified) != len(wanted) {
		return nil, fmt.Errorf("checksums.txt does not map each binary exactly once")
	}
	for name := range contents {
		if name != "checksums.txt" {
			if _, ok := wanted[name]; !ok {
				return nil, fmt.Errorf("unexpected binary content")
			}
		}
	}
	return verified, nil
}

func validateSourceReleaseIdentity(source *catalog.SourceManifest, tag string) error {
	if source == nil {
		return fmt.Errorf("source manifest is required")
	}
	version := strings.TrimPrefix(tag, "v")
	if tag != "v"+version || source.GetVersion() != version {
		return fmt.Errorf("source manifest version does not exactly match release tag")
	}
	if source.GetSiloApiVersion() != "v1" {
		return fmt.Errorf("source manifest API version must be v1")
	}
	want := map[string]struct{}{"darwin/arm64": {}, "linux/amd64": {}, "linux/arm64": {}}
	if len(source.GetSupportedPlatforms()) != len(want) {
		return fmt.Errorf("source manifest must advertise exactly three supported platforms")
	}
	seen := map[string]struct{}{}
	for _, platform := range source.GetSupportedPlatforms() {
		key := platform.GetOs() + "/" + platform.GetArch()
		if _, ok := want[key]; !ok {
			return fmt.Errorf("source manifest advertises unsupported platform")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("source manifest advertises duplicate platform")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateNativeBinary(source *catalog.SourceManifest, binary []byte) error {
	dir, err := os.MkdirTemp("", "bloem-native-manifest-")
	if err != nil {
		return fmt.Errorf("create native validation directory")
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "plugin-linux-amd64")
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		return fmt.Errorf("write native validation binary")
	}
	command := exec.Command(path, "manifest")
	command.Env = []string{"PATH=" + os.Getenv("PATH")}
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("execute native manifest command")
	}
	return validateNativeManifestOutput(source, binary, output)
}

func validateNativeManifestOutput(source *catalog.SourceManifest, binary, output []byte) error {
	actual, err := catalog.DecodeSourceManifest(output)
	if err != nil {
		return fmt.Errorf("decode native manifest: %w", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(binary))
	if actual.GetPluginId() != source.GetPluginId() || actual.GetVersion() != source.GetVersion() || actual.GetSiloApiVersion() != source.GetSiloApiVersion() || actual.GetChecksum() != digest {
		return fmt.Errorf("native manifest identity or checksum does not match source and release")
	}
	if actual.GetPresentation().GetSourceUrl() != source.GetPresentation().GetSourceUrl() || actual.GetPresentation().GetPublisherUrl() != source.GetPresentation().GetPublisherUrl() {
		return fmt.Errorf("native manifest source or publisher does not match source manifest")
	}
	return nil
}

func fetchRelease(ctx context.Context, client *http.Client, token, repo, tag string) (catalog.Release, error) {
	var release catalog.Release
	if err := githubJSON(ctx, client, token, "https://api.github.com/repos/"+repo+"/releases/tags/"+tag, &release); err != nil {
		return catalog.Release{}, err
	}
	return release, nil
}

func fetchSourceManifest(ctx context.Context, client *http.Client, token, repo, tag string) (*catalog.SourceManifest, error) {
	requestURL := githubAPIOrigin + "/repos/" + repo + "/contents/manifest.json?ref=" + url.QueryEscape(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build source manifest request")
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("User-Agent", "bloem-plugins-catalog-updater")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	safeClient := noRedirectClient(client)
	resp, err := safeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("source manifest request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source manifest request returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read source manifest: %w", err)
	}
	manifest, err := catalog.DecodeSourceManifest(body)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func githubJSON(ctx context.Context, client *http.Client, token, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bloem-plugins-catalog-updater")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	safeClient := noRedirectClient(client)
	resp, err := safeClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API request returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode GitHub API response: %w", err)
	}
	return nil
}

func noRedirectClient(client *http.Client) *http.Client {
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &safeClient
}

func loadIndex(path string) (catalog.RepositoryIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return catalog.RepositoryIndex{}, fmt.Errorf("read %s: %w", path, err)
	}
	var index catalog.RepositoryIndex
	if len(bytes.TrimSpace(data)) == 0 {
		return index, nil
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return catalog.RepositoryIndex{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return index, nil
}

func writeIndex(path string, index catalog.RepositoryIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
