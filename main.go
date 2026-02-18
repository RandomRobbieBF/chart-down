package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

func main() {
	var repoURL string
	var proxyURL string
	flag.StringVar(&repoURL, "url", "", "the URL of the Helm repo")
	flag.StringVar(&proxyURL, "proxy", "", "HTTP/SOCKS5 proxy URL (e.g. http://127.0.0.1:8080)")
	flag.Parse()

	if repoURL == "" {
		fmt.Println("Please provide a URL for the Helm repo")
		fmt.Println("Usage: chart-down -url <helm-repo-url>")
		os.Exit(1)
	}

	// Check trufflehog is available
	if _, err := exec.LookPath("trufflehog"); err != nil {
		fmt.Println("Error: trufflehog is not installed or not on PATH.")
		fmt.Println("Install it with: brew install trufflehog")
		os.Exit(1)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if proxyURL != "" {
		proxyParsed, err := url.Parse(proxyURL)
		if err != nil {
			log.Fatalf("Invalid proxy URL: %v", err)
		}
		transport.Proxy = http.ProxyURL(proxyParsed)
		fmt.Printf("[*] Using proxy: %s\n", proxyURL)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: transport,
	}

	fmt.Println("=== Chart-Down: Helm Chart Secret Scanner ===")
	fmt.Println()

	var charts []chartInfo

	// Determine if the URL is a ChartMuseum API endpoint or a standard Helm repo
	isChartMuseumAPI := strings.Contains(repoURL, "/api/charts")

	// Derive the base URL for resolving relative chart download URLs
	// For ChartMuseum /api/charts, the base is the repo root (strip /api/charts)
	// For standard Helm repos, the base is the repo URL itself
	baseURL := repoURL
	if isChartMuseumAPI {
		if idx := strings.Index(baseURL, "/api/charts"); idx != -1 {
			baseURL = baseURL[:idx]
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	indexURL := repoURL
	if !isChartMuseumAPI && !strings.HasSuffix(indexURL, "/index.yaml") {
		indexURL = indexURL + "/index.yaml"
	}

	fmt.Printf("[*] Fetching index from %s\n", indexURL)

	req, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}
	req.Header.Set("User-Agent", "Helm/3.14.0")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error fetching index: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Non-200 response: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response: %v", err)
	}

	chartsFile, err := os.Create("charts.txt")
	if err != nil {
		log.Fatalf("Error creating charts.txt: %v", err)
	}
	defer chartsFile.Close()

	// Try JSON first (ChartMuseum API), fall back to YAML (standard Helm index)
	contentType := resp.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/json") || (len(body) > 0 && body[0] == '{')

	if isJSON {
		charts = parseChartMuseumJSON(body, baseURL, chartsFile)
	} else {
		charts = parseHelmIndexYAML(body, baseURL, chartsFile)
	}

	fmt.Printf("[*] Found %d chart(s) to scan\n\n", len(charts))

	if len(charts) == 0 {
		fmt.Println("No charts found. Exiting.")
		return
	}

	// Prepare directories
	workDir := "charts-extracted"
	resultsDir := "trufflehog-results"
	os.MkdirAll(workDir, 0755)
	os.MkdirAll(resultsDir, 0755)

	totalScanned := 0
	chartsWithSecrets := 0
	totalFindings := 0
	totalVerified := 0
	var allVerified []json.RawMessage

	for _, chart := range charts {
		filename := filepath.Base(chart.URL)
		chartName := strings.TrimSuffix(filename, ".tgz")

		fmt.Printf("[*] Downloading: %s\n", filename)
		tgzData, err := downloadChart(client, chart.URL)
		if err != nil {
			fmt.Printf("    Error downloading: %v\n\n", err)
			continue
		}

		chartDir := filepath.Join(workDir, chartName)
		os.MkdirAll(chartDir, 0755)

		fmt.Printf("[*] Extracting: %s\n", filename)
		if err := extractTgz(tgzData, chartDir); err != nil {
			fmt.Printf("    Error extracting: %v\n\n", err)
			continue
		}

		fmt.Printf("[*] Scanning for secrets with TruffleHog: %s\n", chartName)
		findings, verified := runTrufflehog(chartDir)
		totalScanned++

		if len(findings) > 0 {
			chartsWithSecrets++
			totalFindings += len(findings)
			totalVerified += len(verified)
			allVerified = append(allVerified, verified...)

			fmt.Printf("[!] SECRETS FOUND: %d result(s) in %s", len(findings), chartName)
			if len(verified) > 0 {
				fmt.Printf(" (%d verified)", len(verified))
			}
			fmt.Println()

			resultsPath := filepath.Join(resultsDir, chartName+".json")
			saveFindings(resultsPath, findings)
			fmt.Printf("    Results saved to: %s\n", resultsPath)
		} else {
			fmt.Printf("[+] No secrets found in %s\n", chartName)
		}
		fmt.Println()
	}

	// Write consolidated verified secrets file
	if len(allVerified) > 0 {
		verifiedPath := filepath.Join(resultsDir, "verified-secrets.json")
		saveFindings(verifiedPath, allVerified)
	}

	// Summary
	fmt.Println("=== Scan Complete ===")
	fmt.Printf("Total charts scanned: %d\n", totalScanned)
	if chartsWithSecrets > 0 {
		fmt.Printf("[!] Secrets detected in %d chart(s) (%d total finding(s))\n", chartsWithSecrets, totalFindings)
		if totalVerified > 0 {
			fmt.Printf("[!!] VERIFIED secrets: %d\n", totalVerified)
			fmt.Printf("[!!] Verified secrets saved to: %s/verified-secrets.json\n", resultsDir)
		} else {
			fmt.Println("[*] No verified secrets found (all unverified)")
		}
		fmt.Printf("[!] All results saved in: %s/\n", resultsDir)
		fmt.Println()
		fmt.Printf("Review results with:\n")
		fmt.Printf("  cat %s/*.json | jq .\n", resultsDir)
	} else {
		fmt.Println("[+] No secrets found in any charts.")
		os.Remove(resultsDir) // remove empty dir
	}
}

type chartInfo struct {
	Name    string
	Version string
	URL     string
}

func printAndCollect(name, description, chartType, version string, urls []string, baseURL string, chartsFile *os.File) []chartInfo {
	var charts []chartInfo
	fmt.Printf("Name: %s\n", name)
	if description != "" {
		fmt.Printf("Description: %s\n", description)
	}
	if chartType != "" {
		fmt.Printf("Type: %s\n", chartType)
	}
	fmt.Printf("Version: %s\n", version)

	for _, urlStr := range urls {
		if !strings.HasPrefix(urlStr, "http") {
			urlStr = baseURL + "/" + urlStr
		}
		fmt.Printf("URL: %s\n", urlStr)
		chartsFile.WriteString(urlStr + "\n")
		charts = append(charts, chartInfo{
			Name:    name,
			Version: version,
			URL:     urlStr,
		})
	}
	fmt.Println()
	return charts
}

func parseChartMuseumJSON(data []byte, baseURL string, chartsFile *os.File) []chartInfo {
	// ChartMuseum returns: { "chartName": [ {name, version, urls, ...}, ... ], ... }
	var entries map[string][]struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Description string   `json:"description"`
		Type        string   `json:"type"`
		URLs        []string `json:"urls"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	var charts []chartInfo
	for _, versions := range entries {
		for _, cv := range versions {
			if cv.Name == "" || len(cv.URLs) == 0 {
				continue
			}
			charts = append(charts, printAndCollect(cv.Name, cv.Description, cv.Type, cv.Version, cv.URLs, baseURL, chartsFile)...)
		}
	}
	return charts
}

func parseHelmIndexYAML(data []byte, baseURL string, chartsFile *os.File) []chartInfo {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	entries, ok := doc["entries"].(map[interface{}]interface{})
	if !ok {
		fmt.Println("Error: 'entries' key not found in YAML document")
		return nil
	}

	var charts []chartInfo
	for _, entry := range entries {
		chartVersions, ok := entry.([]interface{})
		if !ok {
			continue
		}
		for _, cv := range chartVersions {
			chartData, ok := cv.(map[interface{}]interface{})
			if !ok {
				continue
			}
			name, _ := chartData["name"].(string)
			description, _ := chartData["description"].(string)
			version, _ := chartData["version"].(string)
			chartType, _ := chartData["type"].(string)
			rawURLs, ok := chartData["urls"].([]interface{})
			if !ok || name == "" {
				continue
			}
			var urls []string
			for _, u := range rawURLs {
				if s, ok := u.(string); ok {
					urls = append(urls, s)
				}
			}
			charts = append(charts, printAndCollect(name, description, chartType, version, urls, baseURL, chartsFile)...)
		}
	}
	return charts
}

func downloadChart(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Helm/3.14.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func extractTgz(data []byte, destDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip open: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Zip-slip protection: reject paths with ".." components
		clean := filepath.Clean(header.Name)
		if strings.Contains(clean, "..") {
			continue
		}

		target := filepath.Join(destDir, clean)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0755)
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			f.Close()
		}
	}
	return nil
}

func runTrufflehog(dir string) (all []json.RawMessage, verified []json.RawMessage) {
	cmd := exec.Command("trufflehog", "filesystem", "--no-update", "--json", dir)
	output, _ := cmd.Output() // trufflehog exits non-zero when it finds secrets

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Only keep lines that are valid JSON with SourceMetadata (actual findings)
		var obj map[string]interface{}
		if json.Unmarshal([]byte(line), &obj) == nil {
			if _, ok := obj["SourceMetadata"]; ok {
				raw := json.RawMessage(line)
				all = append(all, raw)
				if v, ok := obj["Verified"].(bool); ok && v {
					verified = append(verified, raw)
				}
			}
		}
	}
	return
}

func saveFindings(path string, findings []json.RawMessage) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("    Error saving results: %v\n", err)
		return
	}
	defer f.Close()

	for _, finding := range findings {
		f.Write(finding)
		f.WriteString("\n")
	}
}
