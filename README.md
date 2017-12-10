# Junit2Alertmanager

Junit2Alertmanager is a small utility to be able to send junit xml file as alerts in a [prometheus alertmanager](https://github.com/prometheus/alertmanager).

This has been created preliminary to run [cloud foundry smoke tests](https://github.com/cloudfoundry/cf-smoke-tests) 
periodically and send alert to alertmanager when one of this test failed.

## Installation

For now you must have golang and run `go get github.com/orange-cloudfoundry/junit2Alertmanager`, this will install tools in `$GOPATH/bin/junit2Alertmanager`.

## Usage

```
NAME:
   junit2alertmanager - A simple cli program to send junit xml to a prometheus alertmanager

USAGE:
   junit2alertmanager [global options] command [command options] [arguments...]

VERSION:
   1.1.0

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --targets value, -t value        Target one or a list of alertmanager(s) (e.g: http://127.0.0.1:8080,http://127.0.0.1:8080), 
   it will assume that alertmanager are in cluster and will only fallback to next alertmanager when first failed [$ALERT_MANAGER_HOSTS]
   --junit value, -f value          path to a junit xml file (default: "junit.xml")
   --alert-name value, -n value     prefix to alertname label in alert
   --generator-url value, -g value  url to set as generator url
   --expire value, -e value         set expiration for alerts (default: 3m0s)
   --skip-insecure, -k              use it to skip insecure certificate on your target (not recommeded)
   --help, -h                       show help
   --version, -v                    print the version
```