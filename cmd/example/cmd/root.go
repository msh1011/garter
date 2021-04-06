package cmd

import (
	"fmt"
	"garter"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var addCmd = &cobra.Command{
	Use: "add",
	Run: func(cmd *cobra.Command, args []string) {
		fv, _ := cmd.Flags().GetBool("five")
		tn, _ := cmd.Flags().GetBool("ten")
		val, _ := cmd.Flags().GetInt("val")

		fmt.Println("ADD", fv, tn, val, args)

		if fv {
			val += 5
		}
		if tn {
			val += 10
		}

		fmt.Println(val)
	},
}
var longCmd = &cobra.Command{
	Use: "longest",
	Run: func(cmd *cobra.Command, args []string) {
		longest := ""

		for _, v := range args {
			if len(v) > len(longest) {
				longest = v
			}
		}

		val, _ := cmd.Flags().GetInt("val")

		if cmd.Flags().Lookup("val").Changed && len(longest) < val {
			fmt.Println("No args are longer than", val)
			return
		}
		fmt.Printf("longest of %v is %s", args, longest)
	},
}

var rootCmd = &cobra.Command{
	Use: "example",
	Run: func(cmd *cobra.Command, args []string) {
		val, _ := cmd.Flags().GetInt("val")

		fmt.Println("example", val, args)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(longCmd)
	garter.AddServerCmd(rootCmd,
		pflag.Int("port", 8000, "port to use for garter server"),
		pflag.Duration("timeout", 15*time.Minute, "w/r timeout for garter server"))

	rootCmd.PersistentFlags().Int("val", 0, "Value used by other commands")

	addCmd.Flags().Bool("five", false, "Add 5 to val")
	addCmd.Flags().Bool("ten", false, "Add 10 to val")

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
