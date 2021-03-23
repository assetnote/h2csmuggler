package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/assetnote/h2csmuggler/pkg/paths"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	prefix = []string{}
)

// pitchforkCmd represents the pitchfork command
var pitchforkCmd = &cobra.Command{
	Use:   "pitchfork http://base.url.com/ <paths>...",
	Short: "will modify the path of the base with your inputs",
	Long: `pitchfork will permute your base with with all the path inputs
and return full URLs. e.g. http://base.com + foo, bar, baz ->
http://base.com/foo http://base.com/bar http://base.com/baz

You can use '-' as the second argument to pipe from stdin
you can use infile flag to specify a file to take in as the paths`,
	Run: func(cmd *cobra.Command, args []string) {
		base := args[0]
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
			if len(args) < 2 {
				log.Fatalf("no infile specified and no targets provided.")
			}
			if args[1] == "-" {
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					line := scanner.Text()
					lines = append(lines, line)
				}
			} else {
				lines = args[1:]
			}
		}

		lines = append(lines, paths.Prefix(prefix, lines)...)
		res, err := paths.Pitchfork(base, lines)
		if err != nil {
			log.WithError(err).Fatalf("failed to mutate")
		}
		for _, l := range res {
			fmt.Println(l)
		}

	},
}

func init() {
	mutateCmd.AddCommand(pitchforkCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// pitchforkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// pitchforkCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	pitchforkCmd.Flags().StringVarP(&infile, "infile", "i", "", "input file to read from")
	pitchforkCmd.Flags().StringSliceVarP(&prefix, "prefix", "p", []string{}, "prefix for all the paths. Specifying multiple will cross multiply the results")
}
