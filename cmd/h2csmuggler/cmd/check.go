package cmd

import (
	"bufio"
	"os"

	"github.com/assetnote/h2csmuggler/pkg/parallel"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	concurrency = 5
	infile      = ""
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check <targets>...",
	Short: "Check whether a target url is vulnerable to h2c smuggling",
	Long: `This performs a basic request against the specified host over http/1.1
and attempts to upgrade the connection to http2. The request is then replicated
over http2 and the results are compared

use "-" as first argument to recieve from stdin.
If infile is specified, then that will override CLI arguments.
Each target will have a separate connection opened. There is no optimization for batching paths to same host:port combinations`,
	Args: cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		lines := make([]string, 0)
		if infile != "" {
			log.WithField("filename", infile).Debugf("loading from infile")
			file, err := os.Open(infile)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
		} else {
			if len(args) == 0 {
				log.Fatalf("no infile specified and no arguments provided.")
			}
			if args[0] == "-" {
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					line := scanner.Text()
					lines = append(lines, line)
				}
			} else {
				lines = args
			}
		}

		c := parallel.New()
		c.MaxParallelHosts = concurrency
		err := c.GetParallelHosts(lines)
		if err != nil {
			log.WithError(err).Errorf("failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// checkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	checkCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 10, "Number of concurrent threads to use")
	checkCmd.Flags().StringVarP(&infile, "infile", "i", "", "input file to read from")

}
