package ui

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-multierror"
	"github.com/neelance/parallel"
	"github.com/pkg/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/executor"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/workspace"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

var (
	batchPendingColor = output.StylePending
	batchSuccessColor = output.StyleSuccess
	batchSuccessEmoji = output.EmojiSuccess
)

var _ ExecUI = &TUI{}

type TUI struct {
	Out *output.Output

	pending  output.Pending
	progress output.Progress

	progressPrinter *taskExecTUI
}

func (ui *TUI) ParsingBatchSpec() {
	ui.pending = batchCreatePending(ui.Out, "Parsing batch spec")
}
func (ui *TUI) ParsingBatchSpecSuccess() {
	batchCompletePending(ui.pending, "Parsing batch spec")
}

func (ui *TUI) ParsingBatchSpecFailure(err error) {
	if merr, ok := err.(*multierror.Error); ok {
		block := ui.Out.Block(output.Line("\u274c", output.StyleWarning, "Batch spec failed validation."))
		defer block.Close()

		for i, err := range merr.Errors {
			block.Writef("%d. %s", i+1, err)
		}
	}
}

func (ui *TUI) ResolvingNamespace() {
	ui.pending = batchCreatePending(ui.Out, "Resolving namespace")
}

func (ui *TUI) ResolvingNamespaceSuccess(_namespace string) {
	batchCompletePending(ui.pending, "Resolving namesapce")
}

func (ui *TUI) PreparingContainerImages() {
	ui.progress = ui.Out.Progress([]output.ProgressBar{{
		Label: "Preparing container images",
		Max:   1.0,
	}}, nil)
}

func (ui *TUI) PreparingContainerImagesProgress(percent float64) {
	ui.progress.SetValue(0, percent)
}

func (ui *TUI) PreparingContainerImagesSuccess() {
	ui.progress.Complete()
}

func (ui *TUI) DeterminingWorkspaceCreatorType() {
	ui.pending = batchCreatePending(ui.Out, "Determining workspace type")
}

func (ui *TUI) DeterminingWorkspaceCreatorTypeSuccess(wt workspace.CreatorType) {
	switch wt {
	case workspace.CreatorTypeBind:
		ui.pending.VerboseLine(output.Linef("🚧", output.StyleSuccess, "Workspace creator: bind"))
	case workspace.CreatorTypeVolume:
		ui.pending.VerboseLine(output.Linef("🚧", output.StyleSuccess, "Workspace creator: volume"))
	}

	batchCompletePending(ui.pending, "Set workspace type")
}

func (ui *TUI) ResolvingRepositories() {
	ui.pending = batchCreatePending(ui.Out, "Resolving repositories")
}
func (ui *TUI) ResolvingRepositoriesDone(repos []*graphql.Repository, unsupported batches.UnsupportedRepoSet, ignored batches.IgnoredRepoSet) {
	batchCompletePending(ui.pending, fmt.Sprintf("Resolved %d repositories", len(repos)))

	if unsupported != nil && len(unsupported) != 0 {
		block := ui.Out.Block(output.Line(" ", output.StyleWarning, "Some repositories are hosted on unsupported code hosts and will be skipped. Use the -allow-unsupported flag to avoid skipping them."))
		for repo := range unsupported {
			block.Write(repo.Name)
		}
		block.Close()
	} else if ignored != nil && len(ignored) != 0 {
		block := ui.Out.Block(output.Line(" ", output.StyleWarning, "The repositories listed below contain .batchignore files and will be skipped. Use the -force-override-ignore flag to avoid skipping them."))
		for repo := range ignored {
			block.Write(repo.Name)
		}
		block.Close()
	}
}

func (ui *TUI) DeterminingWorkspaces() {
	ui.pending = batchCreatePending(ui.Out, "Determining workspaces")
}

func (ui *TUI) DeterminingWorkspacesSuccess(num int) {
	batchCompletePending(ui.pending, fmt.Sprintf("Found %d workspaces with steps to execute", num))
}

func (ui *TUI) CheckingCache() {
	ui.pending = batchCreatePending(ui.Out, "Checking cache for changeset specs")
}

func (ui *TUI) CheckingCacheSuccess(cachedSpecsFound int, uncachedTasks int) {
	var specsFoundMessage string
	if cachedSpecsFound == 1 {
		specsFoundMessage = "Found 1 cached changeset spec"
	} else {
		specsFoundMessage = fmt.Sprintf("Found %d cached changeset specs", cachedSpecsFound)
	}
	switch uncachedTasks {
	case 0:
		batchCompletePending(ui.pending, fmt.Sprintf("%s; no tasks need to be executed", specsFoundMessage))
	case 1:
		batchCompletePending(ui.pending, fmt.Sprintf("%s; %d task needs to be executed", specsFoundMessage, uncachedTasks))
	default:
		batchCompletePending(ui.pending, fmt.Sprintf("%s; %d tasks need to be executed", specsFoundMessage, uncachedTasks))
	}
}

func (ui *TUI) ExecutingTasks(verbose bool, parallelism int) executor.TaskExecutionUI {
	ui.progressPrinter = newTaskExecTUI(ui.Out, verbose, parallelism)
	return ui.progressPrinter
}

func (ui *TUI) ExecutingTasksSkippingErrors(err error) {
	printExecutionError(ui.Out, err)
	ui.Out.WriteLine(output.Line(output.EmojiWarning, output.StyleWarning, "Skipping errors because -skip-errors was used."))
}

func (ui *TUI) LogFilesKept(files []string) {
	block := ui.Out.Block(output.Line("", batchSuccessColor, "Preserving log files:"))
	defer block.Close()

	for _, file := range files {
		block.Write(file)
	}
}

func (ui *TUI) NoChangesetSpecs() {
	ui.Out.WriteLine(output.Linef(output.EmojiWarning, output.StyleWarning, `No changeset specs created`))
}

func (ui *TUI) UploadingChangesetSpecs(num int) {
	var label string
	if num == 1 {
		label = "Sending changeset spec"
	} else {
		label = fmt.Sprintf("Sending %d changeset specs", num)
	}

	ui.progress = ui.Out.Progress([]output.ProgressBar{
		{Label: label, Max: float64(num)},
	}, nil)
}

func (ui *TUI) UploadingChangesetSpecsProgress(done, total int) {
	ui.progress.SetValue(0, float64(done))
}

func (ui *TUI) UploadingChangesetSpecsSuccess() {
	ui.progress.Complete()
}

func (ui *TUI) CreatingBatchSpec() {
	ui.pending = batchCreatePending(ui.Out, "Creating batch spec on Sourcegraph")
}

func (ui *TUI) CreatingBatchSpecSuccess() {
	batchCompletePending(ui.pending, "Creating batch spec on Sourcegraph")
}

func (ui *TUI) CreatingBatchSpecError(err error) error {
	return prettyPrintBatchUnlicensedError(ui.Out, err)
}

func (ui *TUI) PreviewBatchSpec(batchSpecURL string) {
	ui.Out.Write("")
	block := ui.Out.Block(output.Line(batchSuccessEmoji, batchSuccessColor, "To preview or apply the batch spec, go to:"))
	defer block.Close()

	block.Writef("%s", batchSpecURL)

}

func (ui *TUI) ApplyingBatchSpec() {
	ui.pending = batchCreatePending(ui.Out, "Applying batch spec")
}

func (ui *TUI) ApplyingBatchSpecSuccess(batchChangeURL string) {
	batchCompletePending(ui.pending, "Applying batch spec")

	ui.Out.Write("")
	block := ui.Out.Block(output.Line(batchSuccessEmoji, batchSuccessColor, "Batch change applied!"))
	defer block.Close()

	block.Write("To view the batch change, go to:")
	block.Writef("%s", batchChangeURL)
}

func (ui *TUI) ExecutionError(err error) {
	printExecutionError(ui.Out, err)
}

// prettyPrintBatchUnlicensedError introspects the given error returned when
// creating a batch spec and ascertains whether it's a licensing error. If it
// is, then a better message is output. Regardless, the return value of this
// function should be used to replace the original error passed in to ensure
// that the displayed output is sensible.
func prettyPrintBatchUnlicensedError(out *output.Output, err error) error {
	// Pull apart the error to see if it's a licensing error: if so, we should
	// display a friendlier and more actionable message than the usual GraphQL
	// error output.
	if gerrs, ok := err.(api.GraphQlErrors); ok {
		// A licensing error should be the sole error returned, so we'll only
		// pretty print if there's one error.
		if len(gerrs) == 1 {
			if code, cerr := gerrs[0].Code(); cerr != nil {
				// We got a malformed value in the error extensions; at this
				// point, there's not much sensible we can do. Let's log this in
				// verbose mode, but let the original error bubble up rather
				// than this one.
				out.Verbosef("Unexpected error parsing the GraphQL error: %v", cerr)
			} else if code == "ErrCampaignsUnlicensed" || code == "ErrBatchChangesUnlicensed" {
				// OK, let's print a better message, then return an
				// exitCodeError to suppress the normal automatic error block.
				// Note that we have hand wrapped the output at 80 (printable)
				// characters: having automatic wrapping some day would be nice,
				// but this should be sufficient for now.
				block := out.Block(output.Line("🪙", output.StyleWarning, "Batch Changes is a paid feature of Sourcegraph. All users can create sample"))
				block.WriteLine(output.Linef("", output.StyleWarning, "batch changes with up to 5 changesets without a license. Contact Sourcegraph"))
				block.WriteLine(output.Linef("", output.StyleWarning, "sales at %shttps://about.sourcegraph.com/contact/sales/%s to obtain a trial", output.StyleSearchLink, output.StyleWarning))
				block.WriteLine(output.Linef("", output.StyleWarning, "license."))
				block.Write("")
				block.WriteLine(output.Linef("", output.StyleWarning, "To proceed with this batch change, you will need to create 5 or fewer"))
				block.WriteLine(output.Linef("", output.StyleWarning, "changesets. To do so, you could try adding %scount:5%s to your", output.StyleSearchAlertProposedQuery, output.StyleWarning))
				block.WriteLine(output.Linef("", output.StyleWarning, "%srepositoriesMatchingQuery%s search, or reduce the number of changesets in", output.StyleReset, output.StyleWarning))
				block.WriteLine(output.Linef("", output.StyleWarning, "%simportChangesets%s.", output.StyleReset, output.StyleWarning))
				block.Close()
				return cmderrors.ExitCode(cmderrors.GraphqlErrorsExitCode, nil)
			}
		}
	}

	// In all other cases, we'll just return the original error.
	return err
}

// printExecutionError is used to print the possible error returned by
// batchExecute.
func printExecutionError(out *output.Output, err error) {
	// exitCodeError shouldn't generate any specific output, since it indicates
	// that this was done deeper in the call stack.
	if _, ok := err.(*cmderrors.ExitCodeError); ok {
		return
	}

	out.Write("")

	writeErrs := func(errs []error) {
		var block *output.Block

		if len(errs) > 1 {
			block = out.Block(output.Linef(output.EmojiFailure, output.StyleWarning, "%d errors:", len(errs)))
		} else {
			block = out.Block(output.Line(output.EmojiFailure, output.StyleWarning, "Error:"))
		}

		for _, e := range errs {
			if taskErr, ok := e.(executor.TaskExecutionErr); ok {
				block.Write(formatTaskExecutionErr(taskErr))
			} else {
				if err == context.Canceled {
					block.Writef("%sAborting", output.StyleBold)
				} else {
					block.Writef("%s%s", output.StyleBold, e.Error())
				}
			}
		}

		if block != nil {
			block.Close()
		}
	}

	switch err := err.(type) {
	case parallel.Errors, *multierror.Error, api.GraphQlErrors:
		writeErrs(flattenErrs(err))

	default:
		writeErrs([]error{err})
	}

	out.Write("")

	block := out.Block(output.Line(output.EmojiLightbulb, output.StyleSuggestion, "The troubleshooting documentation can help to narrow down the cause of the errors:"))
	block.WriteLine(output.Line("", output.StyleSuggestion, "https://docs.sourcegraph.com/batch_changes/references/troubleshooting"))
	block.Close()
}

func flattenErrs(err error) (result []error) {
	switch errs := err.(type) {
	case parallel.Errors:
		for _, e := range errs {
			result = append(result, flattenErrs(e)...)
		}

	case *multierror.Error:
		for _, e := range errs.Errors {
			result = append(result, flattenErrs(e)...)
		}

	case api.GraphQlErrors:
		for _, e := range errs {
			result = append(result, flattenErrs(e)...)
		}

	default:
		result = append(result, errs)
	}

	return result
}

func formatTaskExecutionErr(err executor.TaskExecutionErr) string {
	if ee, ok := errors.Cause(err).(*exec.ExitError); ok && ee.String() == "signal: killed" {
		return fmt.Sprintf(
			"%s%s%s: killed by interrupt signal",
			output.StyleBold,
			err.Repository,
			output.StyleReset,
		)
	}

	return fmt.Sprintf(
		"%s%s%s:\n%s\nLog: %s\n",
		output.StyleBold,
		err.Repository,
		output.StyleReset,
		err.Err,
		err.Logfile,
	)
}

func batchCreatePending(out *output.Output, message string) output.Pending {
	return out.Pending(output.Line("", batchPendingColor, message))
}

func batchCompletePending(p output.Pending, message string) {
	p.Complete(output.Line(batchSuccessEmoji, batchSuccessColor, message))
}
