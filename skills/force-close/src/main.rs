//! force-close — Operator skill (§9, IT-S7).
use std::path::PathBuf;

use agent_comms_core::{
    events::{append_event, Event},
    paths::bugs_closed_dir,
    state::GhlState,
};
use agent_comms_git::{git_mv, RealGit};
use anyhow::{Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "force-close")]
struct Args {
    bug_id: String,
    #[arg(long)]
    reason: String,
    #[arg(long)]
    dry_run: bool,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry = args.dry_run || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false);
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };

    let (bug_path, mut bug) = operator_common::find_bug(&repo, &args.bug_id)?;
    if bug.ghlstate == GhlState::Closed {
        eprintln!("force-close: already closed — no-op");
        return Ok(());
    }

    let prior = bug.ghlstate;
    bug.ghlstate = GhlState::Closed;
    bug.ghlstatereason = Some(args.reason.clone());
    let now = jiff::Timestamp::now().to_string();
    bug.current_state.since = now.clone();
    let mut ev = Event::new(now, "operator", "force_closed");
    ev.actor = Some("operator".into());
    ev.extra.insert(
        "reason".into(),
        serde_json::Value::String(args.reason.clone()),
    );
    ev.extra.insert(
        "prior_state".into(),
        serde_json::Value::String(prior.as_str().into()),
    );
    append_event(&mut bug, ev);

    let target_dir = bugs_closed_dir(&repo);
    let target_path = target_dir.join(format!("{}.json", args.bug_id));
    if dry {
        println!(
            "would: write {} ; git mv {} {} ; notify(bug_closed) ; commit \"force-close {} ({})\"",
            target_path.display(),
            bug_path.display(),
            target_path.display(),
            args.bug_id,
            args.reason
        );
        return Ok(());
    }
    std::fs::create_dir_all(&target_dir).with_context(|| format!("mkdir {target_dir:?}"))?;
    operator_common::write_bug(&bug_path, &bug)?;
    let runner = RealGit;
    git_mv(&runner, &repo, &bug_path, &target_path)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!("operator: force-close {} ({})", args.bug_id, args.reason),
    )?;
    println!("notify(bug_closed): {}", args.bug_id);
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
