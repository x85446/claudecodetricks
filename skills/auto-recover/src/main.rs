//! `auto-recover` — git-history restore for malformed `BUG-NNNNNN.json`.
//!
//! Walks history newest-first; restores the last revision whose JSON parses
//! AND passes `validate_bug`; drops a feedback file in the breaking leaf's
//! `from-chopper/`; appends an `auto_recovered` event.

use std::path::{Path, PathBuf};

use agent_comms_core::{events, schema, Clock, RealClock};
use agent_comms_git::{history_walk_for, GitRunner, RealGit};
use anyhow::{bail, Context, Result};
use clap::Parser;
use serde::Serialize;
use serde_json::{json, Value};

#[derive(Parser)]
#[command(name = "auto-recover")]
struct Args {
    #[arg(long)]
    bug_path: PathBuf,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
struct RecoverResult {
    restored_sha: String,
    broke_at_sha: String,
    feedback_path: PathBuf,
}

fn rel_path(repo_root: &Path, abs: &Path) -> Result<String> {
    let rel = abs
        .strip_prefix(repo_root)
        .with_context(|| format!("path {:?} not within repo root {:?}", abs, repo_root))?;
    Ok(rel.to_string_lossy().to_string())
}

fn run(args: &Args) -> Result<RecoverResult> {
    let runner = RealGit;
    let rel = rel_path(&args.repo_root, &args.bug_path)?;
    let rel_path_buf = PathBuf::from(&rel);

    let history =
        history_walk_for(&runner, &args.repo_root, &rel_path_buf).context("git history walk")?;
    if history.is_empty() {
        bail!("no git history for {}", rel);
    }

    let mut broke_at_sha = history[0].0.clone();
    let mut restored: Option<(String, Vec<u8>)> = None;
    for (i, (sha, content)) in history.iter().enumerate() {
        if i == 0 {
            broke_at_sha = sha.clone();
        }
        let parsed: Result<Value, _> = serde_json::from_slice(content);
        if let Ok(v) = parsed {
            if schema::validate_bug(&v).is_ok() {
                restored = Some((sha.clone(), content.clone()));
                break;
            }
        }
    }
    let (restored_sha, restored_bytes) = restored.context("no valid revision found in history")?;

    let bug_id = args
        .bug_path
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("BUG-UNKNOWN")
        .to_string();

    if !args.dry_run {
        // 3. checkout the restored sha into working tree.
        runner
            .run(&["checkout", &restored_sha, "--", &rel], &args.repo_root)
            .context("git checkout restored revision")?;

        // 7. append auto_recovered event to the bug.
        let mut bug: schema::Bug =
            serde_json::from_slice(&restored_bytes).context("parse restored bug")?;
        let clock = RealClock;
        let event = events::Event::new(
            clock.now_iso(),
            "chopper2",
            format!("auto_recovered from_sha={restored_sha}"),
        );
        events::append_event(&mut bug, event);
        std::fs::write(&args.bug_path, serde_json::to_vec_pretty(&bug)?)?;

        runner
            .run(&["add", "--", &rel], &args.repo_root)
            .context("git add restored")?;
        runner
            .run(
                &[
                    "commit",
                    "-m",
                    &format!(
                        "chopper2 auto-recovery: {} reverted to {}",
                        bug_id, restored_sha
                    ),
                ],
                &args.repo_root,
            )
            .context("git commit auto-recovery")?;
    }

    // 5. Identify breaking author via blame on the broken sha.
    let breaking_author = match runner.run(
        &["log", "-1", "--format=%an <%ae>", &broke_at_sha],
        &args.repo_root,
    ) {
        Ok(o) => String::from_utf8_lossy(&o.stdout).trim().to_string(),
        Err(_) => "unknown".to_string(),
    };

    // 6. Drop feedback in the breaking leaf's from-chopper/.
    let leaf_slug = breaking_author
        .split_once('<')
        .and_then(|(_, rest)| rest.split_once('@').map(|(local, _)| local.to_string()))
        .unwrap_or_else(|| "unknown".to_string());
    let feedback_dir = args
        .repo_root
        .join("agents/repo-agents")
        .join(&leaf_slug)
        .join("from-chopper");
    std::fs::create_dir_all(&feedback_dir).ok();
    let feedback_path = feedback_dir.join(format!("AUTO_RECOVERY-{bug_id}.json"));
    let payload = json!({
        "kind": "auto_recovery_feedback",
        "bug_id": bug_id,
        "restored_to_sha": restored_sha,
        "broke_at_sha": broke_at_sha,
        "breaking_author": breaking_author,
    });
    if !args.dry_run {
        std::fs::write(&feedback_path, serde_json::to_vec_pretty(&payload)?)?;
    }

    Ok(RecoverResult {
        restored_sha,
        broke_at_sha,
        feedback_path,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
