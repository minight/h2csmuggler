package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/minight/h2csmuggler/internal/paths"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	prefix = []string{}
)

// appendCmd represents the append command
var appendCmd = &cobra.Command{
	Use:   "append <domains>...",
	Short: "will modify the path of the base with your inputs",
	Long: `append will permute your base with with all the path inputs
and return full URLs. e.g. http://base.com + foo, bar, baz ->
http://base.com/foo http://base.com/bar http://base.com/baz

You can use '-' as the second argument to pipe from stdin
you can use infile flag to specify a file to take in as the paths`,
	Run: func(cmd *cobra.Command, args []string) {
		domains := make([]string, 0)
		if infile != "" {
			log.WithField("filename", infile).Debugf("loading from infile")
			file, err := os.Open(infile)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				domains = append(domains, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
		} else {
			if len(args) < 1 {
				log.Fatalf("no infile specified and no targets provided.")
			}
			if args[0] == "-" {
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					domain := scanner.Text()
					domains = append(domains, domain)
				}
			} else {
				domains = args[0:]
			}
		}

		domains = append(domains, paths.Prefix(domains, prefix)...)
		for _, l := range domains {
			fmt.Println(l)
		}

	},
}

func init() {
	mutateCmd.AddCommand(appendCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// appendCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// appendCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	appendCmd.Flags().StringVarP(&infile, "infile", "i", "", "input file to read from")
	appendCmd.Flags().StringSliceVarP(&prefix, "prefix", "p", []string{}, "prefix for all the paths. Specifying multiple will cross multiply the results")
}
