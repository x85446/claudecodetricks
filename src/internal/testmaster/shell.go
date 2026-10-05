package testmaster

import (
	"context"
	"time"
)

// shellAdapter runs any command: exit 0 passes, 77 skips (the automake
// convention), anything else fails. This is how a language without a
// dedicated adapter — node, ruby, a make target, a bash fixture — gets in.
type shellAdapter struct{}

const shellSkipExit = 77

func (shellAdapter) identity(e *Entry) string { return e.Dir + "\x00" + e.Cmd }

func (shellAdapter) repro(_ *Project, e *Entry) string { return withDir(e.Dir, e.Cmd) }

func (shellAdapter) discover(context.Context, *Project, Source) ([]Found, error) {
	return nil, nil // shell tests are registered by hand: testmaster test add
}

func (a shellAdapter) plan(p *Project, ids []string, floor time.Duration) []Unit {
	var units []Unit
	for _, id := range ids {
		id, e := id, p.Reg.Tests[id]
		argv := append(p.Reg.Runner("shell", "bash", "-c"), e.Cmd)
		timeout := unitTimeout(floor, []*Entry{e})
		units = append(units, Unit{
			Kind: "shell", IDs: []string{id}, Display: withDir(e.Dir, e.Cmd),
			Run: func(ctx context.Context) []Result {
				r := runProc(ctx, p.DirOf(e), nil, timeout, true, argv...)
				res := Result{ID: id, Ms: r.Elapsed.Milliseconds(), Timed: true, Repro: a.repro(p, e), Exit: r.Exit}
				switch {
				case r.Interrupted:
					res.Status, res.Timed = Interrupted, false
				case r.TimedOut:
					res.Status, res.Output, res.Timed = Timeout, r.Stdout, false
					res.Note = "killed after " + timeout.String()
				case r.StartErr != nil:
					res.Status, res.Output, res.Timed = Fail, []byte(r.StartErr.Error()+"\n"), false
				case r.Exit == 0:
					res.Status = Pass
				case r.Exit == shellSkipExit:
					res.Status, res.Timed = Skip, false
				default:
					res.Status, res.Output = Fail, r.Stdout
				}
				return []Result{res}
			},
		})
	}
	return units
}
