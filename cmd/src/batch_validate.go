package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/output"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/sourcegraph/src-cli/internal/batches/ui"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func init() {
	usage := `
'src batch validate' validates the given batch spec.

Usage:

    src batch validate -f FILE

Examples:

    $ src batch validate -f batch.spec.yaml

`

	flagSet := flag.NewFlagSet("validate", flag.ExitOnError)
	fileFlag := flagSet.String("f", "", "The batch spec file to read.")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		svc := service.New(&service.Opts{})

		out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
		if _, _, err := batchParseSpec(fileFlag, svc); err != nil {
			(&ui.TUI{Out: out}).ParsingBatchSpecFailure(err)
			return err
		}

		out.WriteLine(output.Line("\u2705", output.StyleSuccess, "Batch spec successfully validated."))
		return nil
	}

	batchCommands = append(batchCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src batch %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}
