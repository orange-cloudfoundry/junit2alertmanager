package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	promtpl "github.com/prometheus/alertmanager/template"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ALERT_API = "/api/v1/alerts"
)

var version_major int = 1
var version_minor int = 1
var version_build int = 1

type AppJunit struct {
	Targets      []string
	JunitFile    string
	AlertName    string
	GeneratorUrl string
	Expire       time.Duration
	Client       *http.Client
}

func main() {
	app := cli.NewApp()
	app.Name = "junit2alertmanager"
	app.Usage = "A simple cli program to send junit xml to a prometheus alertmanager"
	app.Version = fmt.Sprintf("%d.%d.%d", version_major, version_minor, version_build)
	defaultDuration, _ := time.ParseDuration("3m")
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "targets, t",
			EnvVar: "ALERT_MANAGER_HOSTS",
			Usage:  "Target one or a list of alertmanager(s) (e.g: http://127.0.0.1:8080,http://127.0.0.1:8080), it will assume that alertmanager are in cluster and will only fallback to next alertmanager when first failed",
		},
		cli.StringFlag{
			Name:  "junit, f",
			Value: "junit.xml",
			Usage: "path to a junit xml file",
		},
		cli.StringFlag{
			Name:  "alert-name, n",
			Value: "",
			Usage: "prefix to alertname label in alert",
		},
		cli.StringFlag{
			Name:  "generator-url, g",
			Value: "",
			Usage: "url to set as generator url",
		},
		cli.DurationFlag{
			Name:  "expire, e",
			Value: defaultDuration,
			Usage: "set expiration for alerts",
		},
		cli.BoolFlag{
			Name:  "skip-insecure, k",
			Usage: "use it to skip insecure certificate on your target (not recommeded)",
		},
	}
	app.Action = run
	app.Run(os.Args)
}
func run(c *cli.Context) error {
	if c.GlobalString("targets") == "" {
		return fmt.Errorf("You must set, at least, one target")
	}
	if c.GlobalString("junit") == "" {
		return fmt.Errorf("You must set a junit path file")
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: c.Bool("skip-insecure"),
			},
		},
	}
	appJunit := &AppJunit{
		Targets:      strings.Split(c.GlobalString("targets"), ","),
		JunitFile:    c.GlobalString("junit"),
		AlertName:    c.GlobalString("alert-name"),
		GeneratorUrl: c.GlobalString("generator-url"),
		Expire:       c.GlobalDuration("expire"),
		Client:       client,
	}
	return appJunit.sendAlerts()
}
func (a AppJunit) sendAlerts() error {
	testSuite, err := a.junit2TestSuite()
	if err != nil {
		return err
	}
	alerts := a.testSuite2Alerts(testSuite)
	return a.sendAlertsToTargets(alerts)
}
func (a AppJunit) sendAlertsToTargets(alerts []promtpl.Alert) error {
	b, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
	errMessages := ""
	for _, target := range a.Targets {
		target = strings.TrimSpace(target)
		entry := log.WithField("target", target)
		entry.Info("Sending alerts...")
		resp, err := a.Client.Post(target+ALERT_API, "application/json", bytes.NewBuffer(b))
		if err != nil {
			errMessage := fmt.Sprintf("Error on target '%s': %s\n", target, err.Error())
			entry.Errorf("%s, trying next target.", errMessage)
			errMessages += errMessage
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b, _ = ioutil.ReadAll(resp.Body)
			errMessage := fmt.Sprintf("Error on target '%s' when sending alerts (code: %d): %s\n",
				target, resp.StatusCode, string(b))
			entry.Errorf("%s, trying next target.", errMessage)
			errMessages += errMessage
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		break
	}
	if errMessages != "" {
		return fmt.Errorf(errMessages)
	}
	log.Info("Finished sending alerts")
	return nil
}
func (a AppJunit) testSuite2Alerts(testSuite JUnitTestSuite) []promtpl.Alert {
	alerts := make([]promtpl.Alert, 0)
	for i, testCase := range testSuite.TestCases {
		if testCase.Skipped != nil || testCase.FailureMessage == nil {
			continue
		}
		alert := a.testCase2Alert(testCase, fmt.Sprintf("-%d", i))
		log.Infof("Alert %s created.", alert.Labels["alertname"])
		alerts = append(alerts, alert)
	}
	return alerts
}
func (a AppJunit) testCase2Alert(testCase JUnitTestCase, suffixAlert string) promtpl.Alert {

	endsAt := time.Unix(0, 0)
	if a.Expire != time.Duration(0) {
		endsAt = time.Now().Add(a.Expire)
	}
	description := "no failure"
	if testCase.FailureMessage != nil {
		description = testCase.FailureMessage.Message
	}
	return promtpl.Alert{
		StartsAt: time.Now(),
		EndsAt:   endsAt,
		Labels: promtpl.KV{
			"alertname": a.generateAlertName(testCase, suffixAlert),
		},
		GeneratorURL: a.GeneratorUrl,
		Annotations: promtpl.KV{
			"summary":     testCase.Name,
			"description": description,
		},
	}
}
func (a AppJunit) junit2TestSuite() (JUnitTestSuite, error) {
	var testSuite JUnitTestSuite
	b, err := ioutil.ReadFile(a.JunitFile)
	if err != nil {
		return testSuite, err
	}
	err = xml.Unmarshal(b, &testSuite)
	if err != nil {
		return testSuite, err
	}
	return testSuite, nil
}
func (a AppJunit) generateAlertName(testCase JUnitTestCase, suffixAlert string) string {
	alertName := a.AlertName + "-" + testCase.ClassName
	return strings.Replace(alertName, " ", "-", -1) + suffixAlert
}
