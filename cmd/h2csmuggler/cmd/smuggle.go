package cmd

import (
	"bufio"
	"os"
	"strings"

	"github.com/assetnote/h2csmuggler/pkg/parallel"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	headers = []string{}
	pretty  = false

	method  = "GET"
	compare = false
)

// smuggleCmd represents the smuggle command
var smuggleCmd = &cobra.Command{
	Use:   "smuggle <host> <smuggle>...",
	Short: "smuggle whether a target url is vulnerable to h2c smuggling",
	Long: `This performs a basic request against the specified host over http/1.1
and attempts to upgrade the connection to http2. The request is then replicated
over http2 and the results are compared

if '-' is the second argument, the smuggled targets will be piped in from stdin
if infile is specified as an argument, `,
	Args: cobra.MinimumNArgs(1),
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

		c := parallel.New()
		c.MaxConnPerHost = concurrency

		hs := parseHeaders(headers)
		opts := []parallel.ParallelOption{}
		for _, h := range hs {
			opts = append(opts, parallel.RequestHeader(h.key, h.value))
		}
		opts = append(opts, parallel.RequestMethod(method))
		opts = append(opts, parallel.PrettyPrint(pretty))

		var err error
		if !compare {
			err = c.GetPathsOnHost(base, lines, opts...)
		} else {
			err = c.GetPathDiffOnHost(base, lines, opts...)
		}
		if err != nil {
			log.WithError(err).Errorf("failed")
		}
	},
}

type header struct {
	key   string
	value string
}

func parseHeaders(headers []string) (ret []header) {
	for _, h := range headers {
		v := strings.SplitN(h, ": ", 2)
		if len(v) != 2 {
			log.WithField("input", "v").Errorf("failed to parse header")
		}
		ret = append(ret, header{
			key:   v[0],
			value: v[1],
		})
	}
	return ret
}

func init() {
	rootCmd.AddCommand(smuggleCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// smuggleCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	smuggleCmd.Flags().BoolVarP(&pretty, "pretty", "P", false, "pretty print the results difference")
	smuggleCmd.Flags().BoolVarP(&compare, "compare", "C", false, "Compare the results from h2c with a basic http2 request. log any differences")
	smuggleCmd.Flags().StringSliceVarP(&headers, "header", "H", []string{}, "Headers to send in each request. These will clobber existing headers. Expected in normal formatting: e.g. `Host: foobar.com`")
	smuggleCmd.Flags().StringVarP(&method, "method", "X", "GET", "Method to send in the smuggled request. This will affect the initial request as well")
	smuggleCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 10, "Number of concurrent threads to use")
}
