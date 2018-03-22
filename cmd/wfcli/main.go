package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli"
)

// This is a prototype of the CLI (and will be integrated into the Fission CLI eventually).
func main() {
	// fetch the FISSION_URL env variable. If not set, port-forward to controller.
	var value string
	fissionUrl := os.Getenv("FISSION_URL")
	if len(fissionUrl) == 0 {
		fissionNamespace := getFissionNamespace()
		kubeConfig := getKubeConfigPath()
		localPort := setupPortForward(kubeConfig, fissionNamespace, "application=fission-api")
		value = "http://127.0.0.1:" + localPort
	} else {
		value = fissionUrl
	}

	app := cli.NewApp()
	app.Author = "Erwin van Eyk"
	app.Email = "erwin@platform9.com"
	app.EnableBashCompletion = true
	app.Usage = "Fission Workflows CLI"
	app.Description = "CLI for Fission Workflows"
	app.HideVersion = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "url, u",
			Value:  value,
			EnvVar: "FISSION_URL",
			Usage:  "Url to the Fission apiserver",
		},
		cli.BoolFlag{
			Name:   "debug, d",
			EnvVar: "WFCLI_DEBUG",
		},
	}
	app.Commands = []cli.Command{
		cmdConfig,
		cmdStatus,
		cmdParse,
		cmdWorkflow,
		cmdInvocation,
		cmdValidate,
		cmdAdmin,
		cmdVersion,
	}

	app.Run(os.Args)
}

func table(writer io.Writer, headings []string, rows [][]string) {
	w := tabwriter.NewWriter(writer, 0, 0, 5, ' ', 0)
	if headings != nil {
		fmt.Fprintln(w, strings.Join(headings, "\t")+"\t")
	}
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t")+"\t")
	}
	err := w.Flush()
	if err != nil {
		panic(err)
	}
}

//func createTransportClient(baseUrl *url.URL) *httptransport.Runtime {
//	return httptransport.New(baseUrl.Host, "/proxy/workflows-apiserver/", []string{baseUrl.Scheme})
//}

func fail(msg ...interface{}) {
	for _, line := range msg {
		fmt.Fprintln(os.Stderr, line)
	}
	os.Exit(1)
}

func parseUrl(rawUrl string) *url.URL {
	u, err := url.Parse(rawUrl)
	if err != nil {
		fail(fmt.Sprintf("Invalid url '%s': %v", rawUrl, err))
	}
	return u
}
