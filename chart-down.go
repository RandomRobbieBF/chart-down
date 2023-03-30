package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

func main() {
	var url string
	flag.StringVar(&url, "url", "", "the URL of the YAML file")
	flag.Parse()

	if url == "" {
		fmt.Println("Please provide a URL for the YAML file")
		os.Exit(1)
	}
	urlz := url
	if !strings.HasSuffix(url, "/index.yaml") {
		url = url + "/index.yaml"
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return

	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return

	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// create output file
		f, err := os.Create("charts.txt")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		// create a new YAML decoder
		decoder := yaml.NewDecoder(resp.Body)

		// loop through YAML documents in the stream
		for {
			var doc map[string]interface{}
			err := decoder.Decode(&doc)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}

			// extract data from YAML document
			entries, ok := doc["entries"].(map[interface{}]interface{})
			if !ok {
				fmt.Println("Error: 'entries' key not found in YAML document")
				return
			}
			for _, entry := range entries {
				charts, ok := entry.([]interface{})
				if !ok {
					fmt.Println("Error: 'charts' key not found in YAML document")
					continue
				}
				for _, chart := range charts {
					chartData, ok := chart.(map[interface{}]interface{})
					if !ok {
						continue
					}
					name, ok := chartData["name"].(string)
					if !ok {
						continue
					}
					description, ok := chartData["description"].(string)
					if !ok {
						continue
					}
					urls, ok := chartData["urls"].([]interface{})
					if !ok {
						continue
					}
					version, ok := chartData["version"].(string)
					if !ok {
						continue
					}
					type2, ok := chartData["type"].(string)
					if !ok {
						continue
					}

					// print out data
					fmt.Printf("Name: %s\n", name)
					fmt.Printf("Description: %s\n", description)
					fmt.Printf("Type: %s\n", type2)
					for _, u := range urls {
						url, ok := u.(string)
						if !ok {
							fmt.Println("Error: invalid URL found in YAML document")
							continue
						}
						if !strings.HasPrefix(url, "http") {
							url = "" + urlz + "/" + url + ""
						}
						fmt.Printf("URL: %s\n", url)
						_, err := f.WriteString(url + "\n")
						if err != nil {
							log.Fatal(err)
						}
					}
					fmt.Printf("Version: %s\n\n", version)
				}
			}
		}
	} else {
		fmt.Printf("None HTTP 200 Resoponse Returned")
	}

}
