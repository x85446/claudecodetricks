//! rebuild-index — Operator skill (§9). Idempotent.
use std::collections::BTreeMap;
use std::path::PathBuf;

use agent_comms_core::{
    index::IndexState,
    paths::{bugs_dir, index_state_path},
};
use agent_comms_git::{GitRunner, RealGit};
use anyhow::{Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "rebuild-index")]
struct Args {
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

    let mut by_status: BTreeMap<String, Vec<String>> = BTreeMap::new();
    let mut by_assignee: BTreeMap<String, Vec<String>> = BTreeMap::new();
    let mut all_ids: Vec<u32> = Vec::new();

    for path in operator_common::iter_bug_files(&repo) {
        let raw = std::fs::read_to_string(&path)?;
        let v: serde_json::Value = match serde_json::from_str(&raw) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let id = v
            .get("id")
            .and_then(|x| x.as_str())
            .unwrap_or_default()
            .to_string();
        if let Some(numeric) = id.strip_prefix("BUG-").and_then(|s| s.parse::<u32>().ok()) {
            all_ids.push(numeric);
        }
        let state = v
            .get("ghlstate")
            .and_then(|x| x.as_str())
            .unwrap_or("triaged")
            .to_string();
        let assignee = v
            .get("assignee")
            .and_then(|x| x.as_str())
            .unwrap_or("unassigned")
            .to_string();
        by_status.entry(state).or_default().push(id.clone());
        by_assignee.entry(assignee).or_default().push(id);
    }
    all_ids.sort_unstable();

    let bugs_root = bugs_dir(&repo);
    let by_status_dir = bugs_root.join("_index/by-status");
    let by_assignee_dir = bugs_root.join("_index/by-assignee");

    if dry {
        println!(
            "would: rewrite {} entries in {} and {} entries in {}",
            by_status.len(),
            by_status_dir.display(),
            by_assignee.len(),
            by_assignee_dir.display()
        );
        return Ok(());
    }

    std::fs::create_dir_all(&by_status_dir)?;
    std::fs::create_dir_all(&by_assignee_dir)?;
    let mut changed = false;
    for (k, v) in &by_status {
        let path = by_status_dir.join(format!("{k}.json"));
        let json = serde_json::to_string_pretty(&v)?;
        let prior = std::fs::read_to_string(&path).unwrap_or_default();
        if prior != json {
            std::fs::write(&path, &json)?;
            changed = true;
        }
    }
    for (k, v) in &by_assignee {
        let safe = k.replace('/', "_");
        let path = by_assignee_dir.join(format!("{safe}.json"));
        let json = serde_json::to_string_pretty(&v)?;
        let prior = std::fs::read_to_string(&path).unwrap_or_default();
        if prior != json {
            std::fs::write(&path, &json)?;
            changed = true;
        }
    }

    let state_path = index_state_path(&repo);
    let mut state = IndexState::load(&state_path).unwrap_or(IndexState {
        next_id: 1,
        all_ids: vec![],
    });
    if state.all_ids != all_ids {
        state.all_ids = all_ids;
        state.save(&state_path).context("save IndexState")?;
        changed = true;
    }

    if !changed {
        eprintln!("rebuild-index: index already correct — no-op");
        return Ok(());
    }

    let runner = RealGit;
    runner.run(&["add", "bugs/_index"], &repo)?;
    operator_common::commit_and_push(&runner, &repo, "operator: rebuild bug index")?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
