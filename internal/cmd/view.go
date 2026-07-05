package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var viewPort int

var viewCmd = &cobra.Command{
	Use:     "view",
	GroupID: GroupDiag,
	Short:   "Launch the Mineshaft web view",
	Long: `Start a local web server for the Mineshaft view.

Example:
  ms view                # Start on default port 8090
  ms view --port 3000    # Start on port 3000`,
	RunE: runView,
}

func init() {
	viewCmd.Flags().IntVar(&viewPort, "port", 8090, "HTTP port to listen on")
	rootCmd.AddCommand(viewCmd)
}

func runView(cmd *cobra.Command, args []string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Mineshaft is working")
	})

	listenAddr := fmt.Sprintf("127.0.0.1:%d", viewPort)
	fmt.Printf("  ms view at http://%s  •  ctrl+c to stop\n", listenAddr)

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}
