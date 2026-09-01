//! reopen — Operator skill (§9, IT-S6).
use std::path::PathBuf;

use agent_comms_core::{
    events::{append_event, Event},
    paths::{bugs_dir, from_chopper_dir},
    state::GhlState,
};
use agent_comms_git::{git_mv, RealGit};
use anyhow::{anyhow, bail, Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "reopen")]
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
    if bug.ghlstate != GhlState::Closed {
        bail!(
            "reopen: bug is not closed (state={})",
            bug.ghlstate.as_str()
        );
    }

    let prior_state_str = bug
        .events
        .iter()
        .rev()
        .find_map(|ev| {
            ev.extra
                .get("prior_state")
                .and_then(|v| v.as_str().map(String::from))
        })
        .ok_or_else(|| anyhow!("could not find prior_state in events"))?;
    let prior_state = GhlState::parse(&prior_state_str)?;

    // Pick destination folder by prior_state semantics.
    let target_dir = match prior_state {
        GhlState::Assigned | GhlState::InProgress | GhlState::AwaitingVerify => {
            let assignee = bug.assignee.clone().unwrap_or_default();
            let (rep, role) = assignee.split_once('/').unwrap_or(("", "coder"));
            from_chopper_dir(&repo, rep, role)
        }
        _ => bugs_dir(&repo),
    };
    let target_path = target_dir.join(format!("{}.json", args.bug_id));

    if bug_path == target_path {
        eprintln!("reopen: already at prior state — no-op");
        return Ok(());
    }

    bug.ghlstate = prior_state;
    let now = jiff::Timestamp::now().to_string();
    bug.current_state.since = now.clone();
    let mut ev = Event::new(now, "operator", "reopened");
    ev.actor = Some("operator".into());
    ev.extra.insert(
        "reason".into(),
        serde_json::Value::String(args.reason.clone()),
    );
    ev.extra.insert(
        "restored_state".into(),
        serde_json::Value::String(prior_state_str),
    );
    append_event(&mut bug, ev);

    if dry {
        println!(
            "would: git mv {} {} ; commit \"reopen {} ({})\"",
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
        &format!("operator: reopen {} ({})", args.bug_id, args.reason),
    )?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
