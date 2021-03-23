package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	logLevelInt int

	output string

	logLevelMap = []log.Level{
		log.InfoLevel,
		log.DebugLevel,
		log.TraceLevel,
	}
)

var rootCmd = &cobra.Command{
	Use:   "h2csmuggler",
	Short: "h2csmuggler allows you to check if a site is vulnerable to h2csmuggling",
	Long: `h2csmuggler re-implements h2csmuggler.py from https://github.com/BishopFox/h2csmuggler.
This uses the research from Jake Miller to perform a h2csmuggling attack over http or https
`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		switch output {
		case "text":

		case "json":
			log.SetFormatter(&log.JSONFormatter{})
		default:
			log.Fatalf("Unexpected output type: %v", output)
		}

		if logLevelInt > len(logLevelMap) || logLevelInt < 0 {
			log.Fatalf("Invalid verbose level: %v", logLevelInt)
		}
		log.SetLevel(logLevelMap[logLevelInt])
		log.Debugf("Log level set to: %v", logLevelMap[logLevelInt])
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.h2csmuggler.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.PersistentFlags().IntVarP(&logLevelInt, "verbose", "v", 0, "verbosity level. 1 - debug, 2 - trace")
	rootCmd.PersistentFlags().Lookup("verbose").NoOptDefVal = "1"
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "text", "output format. text or json")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".h2csmuggler" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".h2csmuggler")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
