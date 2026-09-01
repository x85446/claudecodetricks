//! `route-to-agent` — move a triaged bug into the correct destination.
//!
//! Honors the §12 busy-signal rule: never place a file in a leaf's
//! `from-chopper/` that already has anything in it. The presence IS the
//! busy signal — the bug stays in `bugs/` until the slot frees up.

use std::fs;
use std::path::{Path, PathBuf};

use agent_comms_core::{cfg_load, paths};
use agent_comms_git::{self, RealGit};
use anyhow::{bail, Context, Result};
use clap::Parser;
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Parser)]
#[command(name = "route-to-agent")]
struct Args {
    #[arg(long)]
    bug_path: PathBuf,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
enum RouteMethod {
    AiFromChopper,
    HumanWhoCodesDir,
    BlockedQueued,
    BusySignalDeferred,
}

#[derive(Debug, Serialize, Deserialize)]
struct RouteOutcome {
    routed_to: PathBuf,
    method: RouteMethod,
}

fn from_chopper_empty(dir: &Path) -> bool {
    if !dir.exists() {
        return true;
    }
    let mut iter = match fs::read_dir(dir) {
        Ok(it) => it,
        Err(_) => return true,
    };
    iter.all(|entry| {
        entry
            .ok()
            .map(|e| {
                let name = e.file_name();
                let s = name.to_string_lossy();
                s == ".gitkeep" || s == ".gitignore"
            })
            .unwrap_or(true)
    })
}

fn run(args: &Args) -> Result<RouteOutcome> {
    let bug_bytes = fs::read(&args.bug_path)
        .with_context(|| format!("read bug at {}", args.bug_path.display()))?;
    let bug: Value = serde_json::from_slice(&bug_bytes).context("parse bug json")?;

    let bug_id = bug
        .get("id")
        .and_then(Value::as_str)
        .context("bug missing `id` field")?
        .to_string();
    let repo = bug
        .get("repo")
        .and_then(Value::as_str)
        .context("bug missing `repo` field")?
        .to_string();
    let assignee_role = bug
        .get("assignee_role")
        .and_then(Value::as_str)
        .unwrap_or("coder")
        .to_string();

    // §29 blocked guard: any unresolved `blocked_by` → quarantine before routing.
    if let Some(arr) = bug.get("blocked_by").and_then(Value::as_array) {
        if !arr.is_empty() {
            let blocked = paths::bugs_blocked_dir(&args.repo_root);
            fs::create_dir_all(&blocked).ok();
            let dst = blocked.join(format!("{bug_id}.json"));
            let runner = RealGit;
            if !args.dry_run {
                agent_comms_git::git_mv(&runner, &args.repo_root, &args.bug_path, &dst)?;
            }
            return Ok(RouteOutcome {
                routed_to: dst,
                method: RouteMethod::BlockedQueued,
            });
        }
    }

    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let human_routing = cfg
        .pointer(&format!("/repos/per_repo/{repo}/human_coder_routing"))
        .and_then(Value::as_bool)
        .or_else(|| {
            cfg.pointer("/repos/defaults/human_coder_routing")
                .and_then(Value::as_bool)
        })
        .unwrap_or(false);

    let dst_dir = if human_routing && assignee_role == "coder" {
        args.repo_root
            .join("agents/repo-agents")
            .join(&repo)
            .join("coder/human/who-codes")
    } else {
        args.repo_root
            .join("agents/repo-agents")
            .join(&repo)
            .join(&assignee_role)
            .join("from-chopper")
    };

    // §12: busy-signal — only place if dest is empty.
    if !human_routing && !from_chopper_empty(&dst_dir) {
        return Ok(RouteOutcome {
            routed_to: dst_dir,
            method: RouteMethod::BusySignalDeferred,
        });
    }
    fs::create_dir_all(&dst_dir).ok();
    let dst = dst_dir.join(format!("{bug_id}.json"));

    let runner = RealGit;
    if !args.dry_run {
        agent_comms_git::git_mv(&runner, &args.repo_root, &args.bug_path, &dst)?;
    }
    let method = if human_routing && assignee_role == "coder" {
        RouteMethod::HumanWhoCodesDir
    } else {
        RouteMethod::AiFromChopper
    };
    Ok(RouteOutcome {
        routed_to: dst,
        method,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    if !args.bug_path.exists() {
        bail!("bug path does not exist: {}", args.bug_path.display());
    }
    let outcome = run(&args)?;
    println!("{}", serde_json::to_string(&outcome)?);
    Ok(())
}
