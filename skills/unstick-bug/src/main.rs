//! unstick-bug — Operator skill (§9, IT-S5/IT-S28).
use std::path::PathBuf;

use agent_comms_core::{
    events::{append_event, Event},
    paths::from_chopper_dir,
    state::GhlState,
};
use agent_comms_git::{git_mv, RealGit};
use anyhow::{bail, Result};
use clap::Parser;
use indexmap::IndexMap;

#[derive(Parser, Debug)]
#[command(name = "unstick-bug")]
struct Args {
    bug_id: String,
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
    if bug.ghlstate != GhlState::SkillStuck {
        eprintln!("unstick-bug: bug is not skill_stuck — no-op");
        return Ok(());
    }
    let assignee = bug
        .assignee
        .clone()
        .ok_or_else(|| anyhow::anyhow!("bug has no assignee"))?;
    let (rep, role) = assignee
        .split_once('/')
        .ok_or_else(|| anyhow::anyhow!("assignee not in <repo>/<role> form"))?;

    bug.skill_retries = IndexMap::new();
    bug.ghlstate = GhlState::Assigned;
    let now = jiff::Timestamp::now().to_string();
    bug.current_state.since = now.clone();
    let mut ev = Event::new(now, "operator", "unstuck");
    ev.actor = Some("operator".into());
    ev.extra.insert(
        "from".into(),
        serde_json::Value::String("skill_stuck".into()),
    );
    append_event(&mut bug, ev);

    let target_dir = from_chopper_dir(&repo, rep, role);
    let target_path = target_dir.join(format!("{}.json", args.bug_id));

    if bug_path == target_path {
        eprintln!("unstick-bug: already at target — no-op");
        return Ok(());
    }

    if dry {
        println!(
            "would: git mv {} {} ; commit \"unstick {}\"",
            bug_path.display(),
            target_path.display(),
            args.bug_id
        );
        return Ok(());
    }
    if !target_dir.exists() {
        bail!("target dir does not exist: {target_dir:?}");
    }
    operator_common::write_bug(&bug_path, &bug)?;
    let runner = RealGit;
    git_mv(&runner, &repo, &bug_path, &target_path)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!("operator: unstick {}", args.bug_id),
    )?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
