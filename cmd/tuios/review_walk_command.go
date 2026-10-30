package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Gaurav-Gosain/tuios/internal/review/walk"
	"github.com/spf13/cobra"
)

// newReviewWalkCommand starts the guided walkthrough: `tuios review walk
// [base]`. One surface (the walkthrough) plus one nvim pane (the real diff,
// gutter marked against the merge base), driven by a controller that owns
// both for the review's lifetime.
func newReviewWalkCommand() *cobra.Command {
	var opts struct {
		dir         string
		guide       string
		session     string
		workspace   string
		rigID       string
		surfaceOnly bool
		takeover    bool
		logPath     string
	}
	cmd := &cobra.Command{
		Use:   "walk [base]",
		Short: "Walk a reviewed change as a guided story",
		Long: `Start a tui-only guided review of the changes from base to HEAD.

Opens one TML surface (the walkthrough: prose, the step's diff block, the
story rail, the commits) and one nvim pane showing the real diff - the
gutter marked against the merge base. Walk with Next/Prev; click a commit
to see just that commit; select lines in the nvim pane and press leader n
to leave a note.

The controller owns the surface and the pane for the review's lifetime.
A second controller refuses to adopt a live review.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := "main"
			if len(args) > 0 {
				base = args[0]
			}
			if opts.dir == "" {
				d, err := os.Getwd()
				if err != nil {
					return err
				}
				opts.dir = d
			}
			if opts.rigID == "" {
				opts.rigID = filepath.Base(opts.dir) + "-review"
			}
			if opts.logPath == "" {
				opts.logPath = filepath.Join(os.TempDir(), "tui-review-"+opts.rigID+".log")
			}
			var guide []byte
			if opts.guide != "" {
				b, err := os.ReadFile(opts.guide)
				if err != nil {
					return fmt.Errorf("read guide: %w", err)
				}
				guide = b
			}
			_, err := walk.StartController(cmd.Context(), walk.Options{
				Dir:         opts.dir,
				Base:        base,
				RigID:       opts.rigID,
				Session:     opts.session,
				Workspace:   opts.workspace,
				GuideJSON:   guide,
				SurfaceOnly: opts.surfaceOnly,
				Takeover:    opts.takeover,
				LogPath:     opts.logPath,
			})
			return err
		},
	}
	cmd.Flags().StringVarP(&opts.dir, "dir", "C", "", "Worktree under review (default: here)")
	cmd.Flags().StringVar(&opts.guide, "guide", "", "Guide JSON with the story steps (story.json shape)")
	cmd.Flags().StringVarP(&opts.session, "session", "s", "", "tuios session for the surface and pane")
	cmd.Flags().StringVar(&opts.workspace, "workspace", "8", "Workspace for the surface and pane")
	cmd.Flags().StringVar(&opts.rigID, "name", "", "Rig identity (default: <dir>-review)")
	cmd.Flags().BoolVar(&opts.surfaceOnly, "surface-only", false, "Run the walkthrough without the nvim pane")
	cmd.Flags().BoolVar(&opts.takeover, "takeover", false, "Adopt the review even if another controller is live")
	cmd.Flags().StringVar(&opts.logPath, "log", "", "Controller log path")
	return cmd
}
