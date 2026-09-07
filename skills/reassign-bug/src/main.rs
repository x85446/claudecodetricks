//! reassign-bug — Operator skill (§9).
//!
//! Moves a bug to a different leaf inbox; sets assignee + GHLSTATE; appends event.
//! Idempotent: no-op if already assigned to target. Respects --dry-run / DRY_RUN=1.

use std::path::PathBuf;

use agent_comms_core::{
    events::{append_event, Event},
    paths::from_chopper_dir,
    state::GhlState,
};
use agent_comms_git::{git_mv, RealGit};
use anyhow::{anyhow, bail, Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(
    name = "reassign-bug",
    about = "Reassign a bug to a different leaf agent"
)]
struct Args {
    /// Bug ID, e.g. BUG-000042
    bug_id: String,
    /// Target agent in form <repo>/<role> e.g. df-chat/coder
    target: String,
    /// Reset fix_attempts to 0 (used when escalation_needed → assigned).
    #[arg(long)]
    clear_fix_attempts: bool,
    #[arg(long)]
    dry_run: bool,
    /// Repo root (defaults to walking up from cwd).
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

    let (target_repo, target_role) = args
        .target
        .split_once('/')
        .ok_or_else(|| anyhow!("--target must be <repo>/<role>"))?;

    let allowlist = operator_common::load_allowlist(&repo)?;
    if !allowlist.contains_key(target_role) {
        bail!("target role {target_role} not in role allowlist");
    }

    let (bug_path, mut bug) = operator_common::find_bug(&repo, &args.bug_id)?;
    let target_dir = from_chopper_dir(&repo, target_repo, target_role);
    let target_path = target_dir.join(format!("{}.json", args.bug_id));

    if bug.assignee.as_deref() == Some(&args.target) && bug_path == target_path {
        eprintln!("reassign-bug: already at target — no-op");
        return Ok(());
    }

    bug.assignee = Some(args.target.clone());
    bug.ghlstate = GhlState::Assigned;
    if args.clear_fix_attempts {
        bug.fix_attempts = 0;
    }
    let now = jiff::Timestamp::now().to_string();
    bug.current_state.since = now.clone();
    let mut ev = Event::new(now, "operator", "reassigned");
    ev.extra
        .insert("to".into(), serde_json::Value::String(args.target.clone()));
    if args.clear_fix_attempts {
        ev.extra
            .insert("cleared_fix_attempts".into(), serde_json::Value::Bool(true));
    }
    append_event(&mut bug, ev);

    if dry {
        println!(
            "would: write {} ; git mv {} {} ; commit \"reassign {} → {}\"",
            target_path.display(),
            bug_path.display(),
            target_path.display(),
            args.bug_id,
            args.target
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
        &format!("operator: reassign {} → {}", args.bug_id, args.target),
    )?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
