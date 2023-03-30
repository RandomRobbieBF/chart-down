package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

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

	// make HTTP request
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
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
			entries := doc["entries"].(map[interface{}]interface{})
			for _, entry := range entries {
				charts := entry.([]interface{})
				for _, chart := range charts {
					chartData := chart.(map[interface{}]interface{})
					name := chartData["name"].(string)
					description := chartData["description"].(string)
					urls := chartData["urls"].([]interface{})
					version := chartData["version"].(string)

					// print out data
					fmt.Printf("Name: %s\n", name)
					fmt.Printf("Description: %s\n", description)
					for _, u := range urls {
						url := u.(string)
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
