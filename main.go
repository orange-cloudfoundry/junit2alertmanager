package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/onsi/ginkgo/reporters"
	promtpl "github.com/prometheus/alertmanager/template"
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
var version_minor int = 0
var version_build int = 0

type AppJunit struct {
	Target       string
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
			Name:   "target, t",
			EnvVar: "ALERT_MANAGER_HOST",
			Usage:  "Target your alertmanager, e.g: http://127.0.0.1:8080",
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
	if c.GlobalString("target") == "" {
		return fmt.Errorf("You must set a target")
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
		Target:       c.GlobalString("target"),
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
	b, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
	resp, err := a.Client.Post(a.Target+ALERT_API, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ = ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Error when sending alerts (code: %d): %s", resp.StatusCode, string(b))
	}
	return nil
}
func (a AppJunit) testSuite2Alerts(testSuite reporters.JUnitTestSuite) []promtpl.Alert {
	alerts := make([]promtpl.Alert, 0)
	for i, testCase := range testSuite.TestCases {
		if testCase.Skipped != nil || testCase.FailureMessage == nil {
			continue
		}
		alerts = append(alerts, a.testCase2Alert(testCase, fmt.Sprintf("-%d", i)))
	}
	return alerts
}
func (a AppJunit) testCase2Alert(testCase reporters.JUnitTestCase, suffixAlert string) promtpl.Alert {

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
func (a AppJunit) junit2TestSuite() (reporters.JUnitTestSuite, error) {
	var testSuite reporters.JUnitTestSuite
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
func (a AppJunit) generateAlertName(testCase reporters.JUnitTestCase, suffixAlert string) string {
	alertName := a.AlertName + "-" + testCase.ClassName
	return strings.Replace(alertName, " ", "-", -1) + suffixAlert
}
