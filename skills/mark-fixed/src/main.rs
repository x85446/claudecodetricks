use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(
    name = "mark-fixed",
    about = "Signal that a fix branch is pushed; transition to awaiting_verify."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long)]
    fix_branch: Option<String>,
    #[arg(long)]
    fix_commit: String,
    #[arg(long, default_value = "from-chopper")]
    from_dir: String,
    #[arg(long, default_value_t = false)]
    as_human: bool,
    #[arg(long)]
    actor: Option<String>,
    #[arg(long)]
    target_repo_path: Option<PathBuf>,
    #[arg(long)]
    repo: Option<String>,
    #[arg(long, default_value_t = false)]
    dry_run: bool,
    #[arg(long)]
    cwd: Option<PathBuf>,
}

fn dry_run_enabled(arg: bool) -> bool {
    arg || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false)
}

fn agent_name() -> String {
    std::env::var("AGENT_NAME").unwrap_or_else(|_| "unknown-coder".to_string())
}

fn ls_remote_has_branch(repo: &Path, branch: &str) -> Result<bool> {
    let out = Command::new("git")
        .args(["ls-remote", "origin", &format!("refs/heads/{branch}")])
        .current_dir(repo)
        .output()
        .with_context(|| format!("spawn git ls-remote in {}", repo.display()))?;
    if !out.status.success() {
        bail!(
            "git ls-remote failed in {}: {}",
            repo.display(),
            String::from_utf8_lossy(&out.stderr)
        );
    }
    Ok(!out.stdout.trim_ascii().is_empty())
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
        bail!(
            "git mv failed: {} -> {} (exit {:?})",
            from.display(),
            to.display(),
            status.code()
        );
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

    let fix_branch = args
        .fix_branch
        .clone()
        .unwrap_or_else(|| format!("fix/{}", args.bug_id));
    let target_repo = match args.target_repo_path.clone() {
        Some(p) => p,
        None => {
            let r = args
                .repo
                .clone()
                .ok_or_else(|| anyhow::anyhow!("--repo or --target-repo-path required"))?;
            PathBuf::from(format!("/opt/repos/{r}"))
        }
    };

    if dry_run {
        eprintln!(
            "DRY_RUN: would verify origin has {} in {} and transition {} -> awaiting_verify",
            fix_branch,
            target_repo.display(),
            args.bug_id
        );
        return Ok(());
    }

    if !target_repo.exists() {
        bail!(
            "target repo path does not exist: {} (pass --target-repo-path)",
            target_repo.display()
        );
    }
    if !ls_remote_has_branch(&target_repo, &fix_branch)? {
        bail!(
            "fix branch {} not found on origin — push before calling mark-fixed",
            fix_branch
        );
    }

    let from_dir = cwd.join(&args.from_dir);
    let bug_path = from_dir.join(format!("{}.json", args.bug_id));
    if !bug_path.exists() {
        bail!("bug not found: {}", bug_path.display());
    }
    let text = fs::read_to_string(&bug_path)?;
    let mut bug: Value = serde_json::from_str(&text)?;
    let now = jiff::Timestamp::now().to_string();
    let by = agent_name();

    bug["ghlstate"] = json!("awaiting_verify");
    bug["fix_branch"] = json!(fix_branch);
    bug["fix_commit"] = json!(args.fix_commit);
    bug["current_state"] = json!({"since": now, "note": "fix pushed; awaiting tester"});

    let mut event = json!({
        "ts": now,
        "type": "fixed",
        "by": by,
        "fix_branch": fix_branch,
        "fix_commit": args.fix_commit,
    });
    if args.as_human {
        if let Some(actor) = &args.actor {
            event["actor"] = json!(actor);
        } else {
            event["actor"] = json!("human:unspecified");
        }
    } else if let Some(actor) = &args.actor {
        event["actor"] = json!(actor);
    }
    bug.as_object_mut()
        .unwrap()
        .entry("events")
        .or_insert_with(|| Value::Array(vec![]))
        .as_array_mut()
        .unwrap()
        .push(event);

    let parent = bug_path.parent().unwrap();
    let tmp = parent.join(format!(
        ".{}.tmp",
        bug_path.file_name().unwrap().to_string_lossy()
    ));
    fs::write(&tmp, serde_json::to_vec_pretty(&bug)?)?;
    fs::rename(&tmp, &bug_path)?;

    let to_path = cwd.join("to-chopper").join(format!("{}.json", args.bug_id));
    fs::create_dir_all(to_path.parent().unwrap()).ok();
    let repo = find_git_root(&cwd).unwrap_or(cwd.clone());
    run_git_mv(&repo, &bug_path, &to_path)?;

    println!(
        "marked-fixed {} branch={} commit={}",
        args.bug_id, fix_branch, args.fix_commit
    );
    Ok(())
}

#[cfg(test)]
mod tests {
    #[test]
    fn fix_branch_default_format() {
        let bug = "BUG-000042";
        let default = format!("fix/{bug}");
        assert_eq!(default, "fix/BUG-000042");
    }
}
