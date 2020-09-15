package cmd

import (
	"io/ioutil"
	"net/http"

	"github.com/minight/h2csmuggler"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// smuggleCmd represents the smuggle command
var smuggleCmd = &cobra.Command{
	Use:   "smuggle <host> <smuggle>...",
	Short: "smuggle whether a target url is vulnerable to h2c smuggling",
	Long: `This performs a basic request against the specified host over http/1.1
and attempts to upgrade the connection to http2. The request is then replicated
over http2 and the results are compared`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		base := args[0]
		smuggle := args[1:]

		conn, err := h2csmuggler.NewConn(base, h2csmuggler.ConnectionMaxRetries(3))
		if err != nil {
			log.WithError(err).Fatalf("failed to create conn")
		}
		defer conn.Close()

		err = Request(conn, base)
		if err != nil {
			log.WithError(err).Fatalf("initial probe failed")
		}

		for _, t := range smuggle {
			err := Request(conn, t)
			if err != nil {
				log.WithField("target", t).WithError(err).Errorf("failed")
			}
		}
	},
}

func Request(c *h2csmuggler.Conn, t string) error {
	req, err := http.NewRequest("GET", t, nil)
	if err != nil {
		return errors.Wrap(err, "request creation")
	}

	res, err := c.Do(req)
	if err != nil {
		return errors.Wrap(err, "connection do")
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "body read")
	}
	if logLevelInt < 1 {
		log.WithFields(log.Fields{
			"status": res.StatusCode,
			"body":   len(body),
			"target": t,
		}).Infof("success")

	} else {
		log.WithFields(log.Fields{
			"status":  res.StatusCode,
			"headers": res.Header,
			"body":    string(body),
			"target":  t,
		}).Debugf("success")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(smuggleCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// smuggleCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// smuggleCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
