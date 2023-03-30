# chart-down
Extracts all the chart lists from ChartMuseum

Install
----

```
go get -u gopkg.in/yaml.v2
go build chart-down.go
```

or

```
go install github.com/RandomRobbieBF/chart-down/@latest
```


How to run
---

```
chart-down -url http://ChartMuseum
```

Check CLI output and see charts.txt

To download and extract each chart run charts.sh after.


Example
---

```
chart-down -url https://xxx.xxx.xxx.xxx | head
Name: tvp-web-server-swagger
Description: A Helm chart for Kubernetes
Type: application
URL: https://xxx.xxx.xxx.xxx/charts/tvp-web-server-swagger-0.1.8.tgz
Version: 0.1.8

Name: tvp-web-server-swagger
Description: A Helm chart for Kubernetes
Type: application
URL: https://xxx.xxx.xxx.xxx/charts/tvp-web-server-swagger-0.1.7.tgz

```
