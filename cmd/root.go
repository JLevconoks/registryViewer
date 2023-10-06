package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/JLevconoks/registryViewer/app"
	"github.com/JLevconoks/registryViewer/registry"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "registryViewer",
	Short: "An app for displaying docker registry.",
	Run:   runRootCmd,
}

var (
	buildVersion    = ""
	buildTime       = ""
	paginationLimit int
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (%s)", buildVersion, buildTime)
	rootCmd.Flags().IntVar(&paginationLimit, "limit", 1000, "maximum number of items per request to fetch from the registry")
}

func runRootCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Println("Registry name is not provided")
		os.Exit(1)
	}
	fullUrl := strings.ToLower(args[0])

	if !strings.HasPrefix(fullUrl, "http") {
		fullUrl = "https://" + fullUrl
	}

	split := strings.Split(fullUrl, "/")
	protocol := split[0][:len(split[0])-1]
	baseUrl := split[2]

	subPath := ""
	if len(split) > 3 {
		for _, value := range split[3:] {
			subPath += "/" + value
		}
	}
	subPath = strings.TrimSuffix(subPath, "/")

	registryClient := registry.NewRegistry(protocol, baseUrl, subPath, paginationLimit)
	guiApp := app.NewApp(registryClient)
	guiApp.Run()
}
