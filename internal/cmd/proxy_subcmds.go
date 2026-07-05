package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// AnnotationMinerSafe marks a cobra command as safe for miner sandbox execution.
// Commands with this annotation are included in the output of "ms proxy-subcmds".
// Add Annotations: map[string]string{AnnotationMinerSafe: "true"} to any
// ms subcommand that miners should be permitted to run through the proxy.
const AnnotationMinerSafe = "minerSafe"

// bdSafeSubcmds lists the bd subcommands safe for miner sandbox execution.
// Unlike ms subcommands (which are auto-discovered via AnnotationMinerSafe),
// bd subcommands are listed here since bd does not embed annotations.
const bdSafeSubcmds = "create,update,close,show,list,ready,dep,export,prime,stats,blocked,doctor"

var proxySubcmdsCmd = &cobra.Command{
	Use:    "proxy-subcmds",
	Hidden: true,
	Short:  "Output allowed subcommands for the proxy server",
	Long: `Output the allowed subcommand allowlist for ms-proxy-server.

Prints a semicolon-separated "cmd:sub1,sub2,..." string listing which
subcommands miners may invoke through the mTLS proxy. The ms portion
is discovered automatically by scanning commands annotated with the
minerSafe annotation; the bd portion is a fixed list embedded here.

The proxy server calls this command at startup and falls back to its
built-in default if discovery fails.`,
	Run: func(cmd *cobra.Command, args []string) {
		var gtSubs []string
		for _, c := range rootCmd.Commands() {
			if c.Annotations[AnnotationMinerSafe] == "true" {
				gtSubs = append(gtSubs, c.Name())
			}
		}
		sort.Strings(gtSubs)
		fmt.Printf("ms:%s;bd:%s\n", strings.Join(gtSubs, ","), bdSafeSubcmds)
	},
}

func init() {
	rootCmd.AddCommand(proxySubcmdsCmd)
}
