package cmd

import (
	"io/ioutil"
	"net/http"

	"github.com/minight/h2csmuggler"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check <targets>...",
	Short: "Check whether a target url is vulnerable to h2c smuggling",
	Long: `This performs a basic request against the specified host over http/1.1
and attempts to upgrade the connection to http2. The request is then replicated
over http2 and the results are compared`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, t := range args {
			err := ConnectAndRequest(t)
			if err != nil {
				log.WithField("target", t).WithError(err).Errorf("failed")
			}
		}
	},
}

func ConnectAndRequest(t string) error {
	conn, err := h2csmuggler.NewConn(t, h2csmuggler.ConnectionMaxRetries(3))
	if err != nil {
		return errors.Wrap(err, "connect")
	}
	defer conn.Close()

	req, err := http.NewRequest("GET", t, nil)
	if err != nil {
		return errors.Wrap(err, "request creation")
	}

	res, err := conn.Do(req)
	if err != nil {
		return errors.Wrap(err, "connection do")
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "body read")
	}

	log.WithFields(log.Fields{
		"status": res.StatusCode,
		"body":   len(body),
		"target": t,
	}).Infof("success")

	log.WithFields(log.Fields{
		"status":  res.StatusCode,
		"headers": res.Header,
		"body":    string(body),
		"target":  t,
	}).Debugf("verbose")
	return nil
}

func init() {
	rootCmd.AddCommand(checkCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// checkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// checkCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
