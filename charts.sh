#!/bin/bash

# read the urls from charts.txt
while read url; do
    # download the .tgz file
    curl -O "$url"
    
    # extract the contents of the .tgz file
    tar -xzf "$(basename $url)"
    
    # remove the downloaded .tgz file
    rm "$(basename $url)"
done < charts.txt
