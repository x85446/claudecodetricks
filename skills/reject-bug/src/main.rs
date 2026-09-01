use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(
    name = "reject-bug",
    about = "Bounce a bug back to chopper2 for rerouting."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long)]
    suggested_repo: Option<String>,
    #[arg(long)]
    reason: String,
    #[arg(long, default_value = "from-chopper")]
    from_dir: String,
    #[arg(long)]
    actor: Option<String>,
    #[arg(long, default_value_t = false)]
    dry_run: bool,
    #[arg(long)]
    cwd: Option<PathBuf>,
}

fn dry_run_enabled(arg: bool) -> bool {
    arg || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false)
}

fn agent_name() -> String {
    std::env::var("AGENT_NAME").unwrap_or_else(|_| "unknown-leaf".to_string())
}

fn find_git_root(start: &Path) -> Option<PathBuf> {
    let mut cur = start.to_path_buf();
    loop {
        if cur.join(".git").exists() {
            return Some(cur);
        }
        if !cur.pop() {
            return None;
        }
    }
}

fn run_git_mv(repo: &Path, from: &Path, to: &Path) -> Result<()> {
    let from_rel = from.strip_prefix(repo).unwrap_or(from);
    let to_rel = to.strip_prefix(repo).unwrap_or(to);
    if let Some(p) = to.parent() {
        fs::create_dir_all(p).ok();
    }
    let status = Command::new("git")
        .arg("mv")
        .arg(from_rel)
        .arg(to_rel)
        .current_dir(repo)
        .status()
        .with_context(|| format!("spawn git mv in {}", repo.display()))?;
    if !status.success() {
        bail!("git mv failed: {} -> {}", from.display(), to.display());
    }
    Ok(())
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry_run = dry_run_enabled(args.dry_run);
    let cwd = args
        .cwd
        .clone()
        .unwrap_or_else(|| std::env::current_dir().expect("cwd"));

    let from_dir = cwd.join(&args.from_dir);
    let bug_path = from_dir.join(format!("{}.json", args.bug_id));
    if !bug_path.exists() {
        bail!("bug not found: {}", bug_path.display());
    }
    if dry_run {
        println!(
            "DRY_RUN: would set rejected_by_leaf on {} and git mv to to-chopper/",
            args.bug_id
        );
        return Ok(());
    }

    let mut bug: Value = serde_json::from_str(&fs::read_to_string(&bug_path)?)?;
    let now = jiff::Timestamp::now().to_string();
    bug["ghlstate"] = json!("rejected_by_leaf");
    bug["ghlstatereason"] = json!(args.reason);
    if let Some(repo) = &args.suggested_repo {
        bug["suggested_repo"] = json!(repo);
    }
    bug["current_state"] = json!({"since": now, "note": "rejected by leaf"});

    let mut event = json!({
        "ts": now,
        "type": "rejected_by_leaf",
        "by": agent_name(),
        "reason": args.reason,
    });
    if let Some(repo) = &args.suggested_repo {
        event["suggested_repo"] = json!(repo);
    }
    if let Some(actor) = &args.actor {
        event["actor"] = json!(actor);
    }
    bug.as_object_mut()
        .unwrap()
        .entry("events")
        .or_insert_with(|| Value::Array(vec![]))
        .as_array_mut()
        .unwrap()
        .push(event);

    let tmp = bug_path.with_extension("json.tmp");
    fs::write(&tmp, serde_json::to_vec_pretty(&bug)?)?;
    fs::rename(&tmp, &bug_path)?;

    let to_path = cwd.join("to-chopper").join(format!("{}.json", args.bug_id));
    let repo = find_git_root(&cwd).unwrap_or(cwd.clone());
    run_git_mv(&repo, &bug_path, &to_path)?;

    println!("rejected {}", args.bug_id);
    Ok(())
}

#[cfg(test)]
mod tests {
    #[test]
    fn smoke() {
        // Reject-bug has no parser-only logic worth testing; integration tests live in tests/.
        let _ = "ok";
    }
}
